package notes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRankPathsIsDeterministicAndUnicodeCaseInsensitive(t *testing.T) {
	snapshot := Snapshot{Entries: []Entry{
		{Path: "zeta.md", Kind: KindMarkdown},
		{Path: "Привет.md", Kind: KindMarkdown},
		{Path: "projects", Kind: KindDirectory},
		{Path: "alpha-project.md", Kind: KindMarkdown},
	}}

	matches := RankPaths(snapshot, "ПРИВ", 10)
	if len(matches) != 1 || matches[0].Path != "Привет.md" || matches[0].Kind != MatchPath {
		t.Fatalf("RankPaths() Unicode matches = %#v, want Привет.md path match", matches)
	}
	matches = RankPaths(snapshot, "proj", 10)
	if got := matchPaths(matches); fmt.Sprint(got) != "[projects alpha-project.md]" {
		t.Fatalf("RankPaths() = %v, want deterministic fuzzy order", got)
	}
	if got := RankPaths(snapshot, "proj", 1); len(got) != 1 || got[0].Path != "projects" {
		t.Fatalf("RankPaths(limit=1) = %#v", got)
	}
}

func TestRankPathsUsesUnicodeCaseFolding(t *testing.T) {
	snapshot := Snapshot{Entries: []Entry{
		{Path: "ΣΑ.md", Kind: KindMarkdown},
		{Path: "Kelvin.md", Kind: KindMarkdown},
	}}

	for _, test := range []struct {
		query string
		want  RelPath
	}{
		{query: "ςα", want: "ΣΑ.md"},
		{query: "k", want: "Kelvin.md"},
	} {
		matches := RankPaths(snapshot, test.query, 10)
		if len(matches) != 1 || matches[0].Path != test.want {
			t.Fatalf("RankPaths(%q) = %#v, want %q", test.query, matches, test.want)
		}
	}
}

func TestSearchContentUsesScannedMarkdownAndReportsLines(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{
		{path: "a.md", content: "first\nNeedle in haystack\nlast", mode: 0o600},
		{path: "b.md", content: "NEEDLE again", mode: 0o600},
		{path: "plain.txt", content: "needle", mode: 0o600},
		{path: ".hidden.md", content: "needle", mode: 0o600},
		{path: "ignored/skip.md", content: "needle", mode: 0o600},
	})
	store, err := NewStore(root, []string{"ignored"}, false)
	if err != nil {
		t.Fatal(err)
	}

	matches, stats, err := store.SearchContent(context.Background(), "needle", 1024, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got := matchPaths(matches); fmt.Sprint(got) != "[a.md b.md]" {
		t.Fatalf("SearchContent() paths = %v, want scanned Markdown only", got)
	}
	if matches[0].Kind != MatchContent || matches[0].Line != 2 || matches[0].Snippet != "Needle in haystack" {
		t.Fatalf("first content match = %#v", matches[0])
	}
	if stats.FilesScanned != 2 || stats.FilesSkipped != 0 {
		t.Fatalf("SearchContent() stats = %#v, want 2 scanned", stats)
	}
}

func TestSearchContentSnapshotOnlyReadsProvidedEntries(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{
		{path: "agent-memory/global/topic.md", content: "needle", mode: 0o600},
		{path: "user.md", content: "needle", mode: 0o600},
	})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	visible := Snapshot{Root: snapshot.Root}
	for _, entry := range snapshot.Entries {
		if entry.Path == "user.md" {
			visible.Entries = append(visible.Entries, entry)
		}
	}

	matches, stats, err := store.SearchContentSnapshot(context.Background(), visible, "needle", 1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := matchPaths(matches); fmt.Sprint(got) != "[user.md]" || stats.FilesScanned != 1 {
		t.Fatalf("SearchContentSnapshot() = %v, %#v, want user.md only", got, stats)
	}
}

func TestSearchContentUsesUnicodeCaseFoldingAndPreservesSnippet(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "unicode.md", content: "ΣΑ\nKelvin", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		query   string
		line    int
		snippet string
	}{
		{query: "ςα", line: 1, snippet: "ΣΑ"},
		{query: "k", line: 2, snippet: "Kelvin"},
	} {
		matches, _, err := store.SearchContent(context.Background(), test.query, 1024, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 || matches[0].Line != test.line || matches[0].Snippet != test.snippet {
			t.Fatalf("SearchContent(%q) = %#v, want line %d snippet %q", test.query, matches, test.line, test.snippet)
		}
	}
}

func TestSearchContentSkipsLargeFilesCapsResultsAndCancels(t *testing.T) {
	root := t.TempDir()
	lines := ""
	for index := range 250 {
		lines += fmt.Sprintf("needle %03d\n", index)
	}
	if err := os.WriteFile(filepath.Join(root, "large.md"), []byte(strings.Repeat("needle-too-large", len(lines))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "many.md"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	matches, stats, err := store.SearchContent(context.Background(), "needle", int64(len(lines)+1), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 200 || stats.FilesSkipped != 1 {
		t.Fatalf("SearchContent() = %d matches, %#v stats; want cap 200 and one skip", len(matches), stats)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := store.SearchContent(ctx, "needle", 1<<20, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("SearchContent(canceled) error = %v", err)
	}
}

func TestSearchContentReportsOnlyRealTruncation(t *testing.T) {
	for _, tt := range []struct {
		name      string
		lines     int
		truncated bool
	}{
		{name: "exact limit", lines: 2, truncated: false},
		{name: "more than limit", lines: 3, truncated: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, nil, []treeFile{{path: "note.md", content: strings.Repeat("needle\n", tt.lines), mode: 0o600}})
			store, err := NewStore(root, nil, false)
			if err != nil {
				t.Fatal(err)
			}

			matches, stats, err := store.SearchContent(context.Background(), "needle", 1024, 2)
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 2 || stats.Truncated != tt.truncated {
				t.Fatalf("SearchContent() = %d matches, %#v", len(matches), stats)
			}
		})
	}
}

func TestSearchContentSkipsFilesAboveEightMiB(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.md")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate((8 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	matches, stats, err := store.SearchContent(context.Background(), "needle", 8<<20, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 || stats.FilesScanned != 0 || stats.FilesSkipped != 1 {
		t.Fatalf("8 MiB capped search = %#v, %#v", matches, stats)
	}
}

func TestSearchContentBoundsUnicodeSnippetAroundMatch(t *testing.T) {
	root := t.TempDir()
	line := strings.Repeat("я", 300) + " needle " + strings.Repeat("界", 300)
	if err := os.WriteFile(filepath.Join(root, "long.md"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	matches, _, err := store.SearchContent(context.Background(), "needle", 8<<20, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || utf8.RuneCountInString(matches[0].Snippet) > 202 || !strings.Contains(matches[0].Snippet, "needle") {
		t.Fatalf("bounded snippet = %q (%d runes)", matches[0].Snippet, utf8.RuneCountInString(matches[0].Snippet))
	}
}

func BenchmarkRankPaths(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			entries := make([]Entry, count)
			for index := range entries {
				entries[index] = Entry{Path: RelPath(fmt.Sprintf("folder/note-%05d.md", index)), Kind: KindMarkdown}
			}
			snapshot := Snapshot{Entries: entries}
			b.ResetTimer()
			for b.Loop() {
				RankPaths(snapshot, "n123", 200)
			}
		})
	}
}

func matchPaths(matches []Match) []RelPath {
	paths := make([]RelPath, len(matches))
	for index, match := range matches {
		paths[index] = match.Path
	}
	return paths
}
