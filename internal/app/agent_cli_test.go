package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gonfff/jotmd/internal/notes"
)

func TestParseAgentInvocationAcceptsFlagsAroundOperands(t *testing.T) {
	tests := []struct {
		args       []string
		command    string
		operands   []string
		notesDir   string
		jsonOutput bool
		limit      int
		ifRevision string
	}{
		{args: []string{"get", "docs/a.md", "--json"}, command: "get", operands: []string{"docs/a.md"}, jsonOutput: true, limit: 20},
		{args: []string{"--notes-dir", "./vault", "search", "auth", "--limit", "7", "--json"}, command: "search", operands: []string{"auth"}, notesDir: "./vault", jsonOutput: true, limit: 7},
		{args: []string{"write", "docs/a.md", "--if-revision", "sha256:abc"}, command: "write", operands: []string{"docs/a.md"}, ifRevision: "sha256:abc", limit: 20},
		{args: []string{"delete", "--if-revision=sha256:def", "docs/a.md"}, command: "delete", operands: []string{"docs/a.md"}, ifRevision: "sha256:def", limit: 20},
	}

	for _, tt := range tests {
		got, handled, err := parseAgentInvocation(tt.args)
		if err != nil || !handled {
			t.Fatalf("parseAgentInvocation(%q) = (%#v, %v, %v)", tt.args, got, handled, err)
		}
		gotNotesDir, gotRevision := "", ""
		if got.notesDir != nil {
			gotNotesDir = *got.notesDir
		}
		if got.ifRevision != nil {
			gotRevision = *got.ifRevision
		}
		if got.command != tt.command || !slices.Equal(got.operands, tt.operands) ||
			gotNotesDir != tt.notesDir || got.jsonOutput != tt.jsonOutput ||
			got.limit != tt.limit || gotRevision != tt.ifRevision {
			t.Errorf("parseAgentInvocation(%q) = %#v", tt.args, got)
		}
	}
}

func TestParseAgentInvocationLeavesLegacyArgumentsAlone(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--help"},
		{"--version"},
		{"--theme", "nord"},
		{"config", "check"},
		{"rm", "note.md"},
		{"trash", "note.md"},
	} {
		if got, handled, err := parseAgentInvocation(args); err != nil || handled {
			t.Errorf("parseAgentInvocation(%q) = (%#v, %v, %v), want legacy", args, got, handled, err)
		}
	}
}

func TestParseAgentInvocationRejectsInvalidAgentArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"get", "note.md", "--unknown"}, want: "unknown flag"},
		{name: "missing option value", args: []string{"search", "note", "--limit"}, want: "requires a value"},
		{name: "two operands", args: []string{"get", "one.md", "two.md"}, want: "exactly one"},
		{name: "zero limit", args: []string{"search", "note", "--limit", "0"}, want: "between 1 and 200"},
		{name: "large limit", args: []string{"search", "note", "--limit=201"}, want: "between 1 and 200"},
		{name: "invalid limit", args: []string{"search", "note", "--limit", "many"}, want: "integer"},
		{name: "limit on get", args: []string{"get", "note.md", "--limit", "2"}, want: "only valid with search"},
		{name: "revision on get", args: []string{"get", "note.md", "--if-revision", "sha256:abc"}, want: "only valid with write or delete"},
		{name: "delete without revision", args: []string{"delete", "note.md"}, want: "requires --if-revision"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, handled, err := parseAgentInvocation(tt.args)
			if !handled || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseAgentInvocation(%q) = (handled=%v, err=%v), want error containing %q", tt.args, handled, err, tt.want)
			}
		})
	}
}

func TestParseAgentInvocationDoubleDashStopsFlagParsing(t *testing.T) {
	got, handled, err := parseAgentInvocation([]string{"search", "--", "--json"})
	if err != nil || !handled {
		t.Fatalf("parseAgentInvocation() = (%#v, %v, %v)", got, handled, err)
	}
	if got.jsonOutput || !slices.Equal(got.operands, []string{"--json"}) {
		t.Fatalf("parseAgentInvocation() = %#v", got)
	}
}

func TestRunAgentSearchJSON(t *testing.T) {
	root := t.TempDir()
	writeAgentNote(t, root, "projects/foo.md", "refresh token redis pitfall\n")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer

	code := Run(context.Background(), []string{
		"--notes-dir", root, "search", "refresh token redis", "--json",
	}, strings.NewReader(""), &stdout, &stderr)

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("Run(search) = (%d, %q), stderr = %q", code, stdout.String(), stderr.String())
	}
	var got struct {
		Query   string `json:"query"`
		Results []struct {
			Path    string `json:"path"`
			Line    int    `json:"line"`
			Snippet string `json:"snippet"`
		} `json:"results"`
		Truncated bool `json:"truncated"`
		Stats     struct {
			FilesScanned int `json:"files_scanned"`
			FilesSkipped int `json:"files_skipped"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Query != "refresh token redis" || got.Truncated || len(got.Results) != 1 {
		t.Fatalf("search JSON = %#v", got)
	}
	result := got.Results[0]
	if result.Path != "projects/foo.md" || result.Line != 1 || result.Snippet != "refresh token redis pitfall" {
		t.Errorf("search result = %#v", result)
	}
	if got.Stats.FilesScanned != 1 || got.Stats.FilesSkipped != 0 {
		t.Errorf("search stats = %#v", got.Stats)
	}
}

func TestRunAgentSearchReportsEmptyAndTruncatedResults(t *testing.T) {
	root := t.TempDir()
	writeAgentNote(t, root, "note.md", "needle one\nneedle two\n")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"search", "missing", "--notes-dir", root, "--json"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"results":[]`) {
		t.Fatalf("Run(empty search) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	code = Run(context.Background(), []string{"search", "needle", "--limit", "1", "--notes-dir", root, "--json"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), `"truncated":true`) {
		t.Fatalf("Run(truncated search) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
}

func TestRunAgentSearchPlainOutput(t *testing.T) {
	root := t.TempDir()
	writeAgentNote(t, root, "docs/a.md", "before\nauth token\nafter")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer

	code := Run(context.Background(), []string{"--notes-dir", root, "search", "auth"}, strings.NewReader(""), &stdout, &stderr)

	if code != 0 || stdout.String() != "docs/a.md:2:  auth token\n" || stderr.Len() != 0 {
		t.Fatalf("Run(search) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
}

func TestRunAgentGetPlainAndJSON(t *testing.T) {
	root := t.TempDir()
	writeAgentNote(t, root, "docs/a.md", "# Note\n")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"get", "docs/a.md", "--notes-dir", root}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stdout.String() != "# Note\n" || stderr.Len() != 0 {
		t.Fatalf("Run(get plain) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	code = Run(context.Background(), []string{"get", "docs/a.md", "--notes-dir", root, "--json"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("Run(get JSON) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["path"] != "docs/a.md" || got["content"] != "# Note\n" || !strings.HasPrefix(got["revision"].(string), "sha256:") || len(got) != 3 {
		t.Fatalf("get JSON = %#v", got)
	}
}

func TestRunAgentGetRejectsInvalidPathsAndEncoding(t *testing.T) {
	root := t.TempDir()
	writeAgentNote(t, root, "invalid.md", string([]byte{'a', 0xff}))
	if err := os.Mkdir(filepath.Join(root, "directory.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	for _, path := range []string{"directory.md", "note.txt", "../note.md", "/note.md", `folder\note.md`} {
		t.Run(path, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{"get", path, "--notes-dir", root}, strings.NewReader(""), &stdout, &stderr)
			if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid_input") {
				t.Fatalf("Run(get %q) = (%d, %q, %q)", path, code, stdout.String(), stderr.String())
			}
		})
	}

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"get", "invalid.md", "--notes-dir", root, "--json"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"invalid_encoding"`) {
		t.Fatalf("Run(get invalid UTF-8) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
}

func TestRunAgentErrorsUseStableChannelAndFormat(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	missingRoot := filepath.Join(t.TempDir(), "missing")

	for _, tt := range []struct {
		name     string
		args     []string
		wantCode int
		wantText string
	}{
		{name: "missing root", args: []string{"--notes-dir", missingRoot, "search", "note"}, wantCode: 3, wantText: "jotmd: not_found:"},
		{name: "invalid flag JSON", args: []string{"get", "note.md", "--unknown", "--json"}, wantCode: 2, wantText: `"code":"invalid_input"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, strings.NewReader(""), &stdout, &stderr)
			if code != tt.wantCode || stdout.Len() != 0 || !strings.Contains(stderr.String(), tt.wantText) {
				t.Fatalf("Run() = (%d, %q, %q), want code %d and %q", code, stdout.String(), stderr.String(), tt.wantCode, tt.wantText)
			}
			if strings.Contains(tt.name, "JSON") {
				var envelope struct {
					Error struct {
						Code    string         `json:"code"`
						Message string         `json:"message"`
						Details map[string]any `json:"details"`
					} `json:"error"`
				}
				if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil || envelope.Error.Details == nil {
					t.Fatalf("JSON error = %#v, decode error = %v", envelope, err)
				}
			}
		})
	}
}

func TestRunAgentWriteCreatesNoteAndReturnsJSON(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer

	code := Run(context.Background(), []string{
		"--notes-dir", root, "write", "note.md", "--json",
	}, strings.NewReader("# Note\n"), &stdout, &stderr)

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("Run(write) = (%d, %q), stderr = %q", code, stdout.String(), stderr.String())
	}
	var got struct {
		Path     string `json:"path"`
		Revision string `json:"revision"`
		Created  bool   `json:"created"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != "note.md" || !strings.HasPrefix(got.Revision, "sha256:") || !got.Created {
		t.Fatalf("write JSON = %#v", got)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "# Note\n" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
}

func TestRunAgentWriteUpdatesOnlyMatchingRevision(t *testing.T) {
	root := t.TempDir()
	writeAgentNote(t, root, "note.md", "old")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var getOutput, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"get", "note.md", "--notes-dir", root, "--json"}, strings.NewReader(""), &getOutput, &stderr); code != 0 {
		t.Fatalf("Run(get) = (%d, %q)", code, stderr.String())
	}
	var document struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(getOutput.Bytes(), &document); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	code := Run(context.Background(), []string{"write", "note.md", "--notes-dir", root, "--if-revision", document.Revision, "--json"}, strings.NewReader("new"), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"created":false`) {
		t.Fatalf("Run(update) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "new" {
		t.Fatalf("updated content = %q, error = %v", content, err)
	}

	stdout.Reset()
	code = Run(context.Background(), []string{"write", "note.md", "--notes-dir", root, "--if-revision", document.Revision, "--json"}, strings.NewReader("stale"), &stdout, &stderr)
	if code != 4 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"revision_conflict"`) || !strings.Contains(stderr.String(), `"expected_revision":"`+document.Revision+`"`) {
		t.Fatalf("Run(stale update) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "new" {
		t.Fatalf("content after conflict = %q, error = %v", content, err)
	}
}

func TestRunAgentWriteRejectsUnconditionalOverwriteAndMalformedRevision(t *testing.T) {
	root := t.TempDir()
	writeAgentNote(t, root, "note.md", "keep")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	for _, tt := range []struct {
		name         string
		args         []string
		wantCode     int
		wantCodeText string
	}{
		{name: "unconditional overwrite", args: []string{"write", "note.md", "--notes-dir", root, "--json"}, wantCode: 4, wantCodeText: `"code":"already_exists"`},
		{name: "malformed revision", args: []string{"write", "note.md", "--notes-dir", root, "--if-revision", "sha256:no", "--json"}, wantCode: 2, wantCodeText: `"code":"invalid_input"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, strings.NewReader("replace"), &stdout, &stderr)
			if code != tt.wantCode || stdout.Len() != 0 || !strings.Contains(stderr.String(), tt.wantCodeText) {
				t.Fatalf("Run(write) = (%d, %q, %q)", code, stdout.String(), stderr.String())
			}
		})
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "keep" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
}

func TestRunAgentWriteAcceptsEmptyInputAndDoesNotMutateOnReadError(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer

	if code := Run(context.Background(), []string{"write", "empty.md", "--notes-dir", root}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run(empty write) = (%d, %q)", code, stderr.String())
	}
	if content, err := os.ReadFile(filepath.Join(root, "empty.md")); err != nil || len(content) != 0 {
		t.Fatalf("empty content = %q, error = %v", content, err)
	}

	stdout.Reset()
	if code := Run(context.Background(), []string{"write", "failed.md", "--notes-dir", root}, dataErrorReader{}, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(failed stdin) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "failed.md")); !os.IsNotExist(err) {
		t.Fatalf("failed write created target: %v", err)
	}
}

func TestRunAgentWriteReportsOutputFailureAfterCommit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stderr bytes.Buffer

	code := Run(context.Background(), []string{"write", "note.md", "--notes-dir", root}, strings.NewReader("committed"), failWriter{}, &stderr)

	if code != 1 || !strings.Contains(stderr.String(), "operation_failed") {
		t.Fatalf("Run(write with failed stdout) = (%d, %q)", code, stderr.String())
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "committed" {
		t.Fatalf("committed content = %q, error = %v", content, err)
	}
}

func TestRunAgentDeleteRejectsStaleAndInvalidRevision(t *testing.T) {
	root := t.TempDir()
	writeAgentNote(t, root, "note.md", "current")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stale := "sha256:" + strings.Repeat("0", 64)

	for _, tt := range []struct {
		name     string
		revision string
		wantCode int
		wantText string
	}{
		{name: "stale", revision: stale, wantCode: 4, wantText: `"code":"revision_conflict"`},
		{name: "malformed", revision: "sha256:no", wantCode: 2, wantText: `"code":"invalid_input"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{"delete", "note.md", "--notes-dir", root, "--if-revision", tt.revision, "--json"}, strings.NewReader(""), &stdout, &stderr)
			if code != tt.wantCode || stdout.Len() != 0 || !strings.Contains(stderr.String(), tt.wantText) {
				t.Fatalf("Run(delete) = (%d, %q, %q)", code, stdout.String(), stderr.String())
			}
		})
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "current" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
}

func TestRunAgentDeleteValidatesPathBeforeDeletion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	revision := "sha256:" + strings.Repeat("0", 64)

	for _, path := range []string{"note.txt", "../note.md"} {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{"delete", path, "--notes-dir", root, "--if-revision", revision}, strings.NewReader(""), &stdout, &stderr)
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid_input") {
			t.Fatalf("Run(delete %q) = (%d, %q, %q)", path, code, stdout.String(), stderr.String())
		}
	}
}

func TestRunDeleteReturnsJSONOnlyAfterPermanentDeletion(t *testing.T) {
	revision := "sha256:" + strings.Repeat("a", 64)
	invocation := agentInvocation{
		command:    "delete",
		jsonOutput: true,
		limit:      defaultAgentSearchLimit,
		ifRevision: &revision,
		operands:   []string{"note.md"},
	}
	called := false
	var stdout bytes.Buffer

	err := runDelete(context.Background(), invocation, &stdout, func(_ context.Context, path notes.RelPath, got notes.Revision) error {
		called = true
		if path != "note.md" || string(got) != revision {
			t.Fatalf("delete(%q, %q)", path, got)
		}
		return nil
	})

	if err != nil || !called || stdout.String() != `{"path":"note.md","revision":"`+revision+`","action":"deleted"}`+"\n" {
		t.Fatalf("runDelete() = (%q, called=%v, error=%v)", stdout.String(), called, err)
	}
}

func TestRunAgentCommandHelpDoesNotLoadConfiguration(t *testing.T) {
	t.Setenv("JOTMD_PREVIEW_MAX_BYTES", "invalid")
	tests := []struct {
		command string
		want    string
	}{
		{command: "search", want: "case-insensitive substring"},
		{command: "get", want: "content and revision"},
		{command: "write", want: "existing note requires --if-revision"},
		{command: "delete", want: "Permanently delete"},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{tt.command, "--help"}, strings.NewReader(""), &stdout, &stderr)
			if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), tt.want) {
				t.Fatalf("Run(%s --help) = (%d, %q, %q), want %q", tt.command, code, stdout.String(), stderr.String(), tt.want)
			}
		})
	}
}

func TestRunRootHelpListsOnlyImplementedAgentCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"--help"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("Run(--help) = (%d, %q, %q)", code, stdout.String(), stderr.String())
	}
	for _, command := range []string{"search", "get", "write", "delete"} {
		if !strings.Contains(stdout.String(), "jotmd [--notes-dir PATH] "+command) {
			t.Errorf("root help does not list %s: %q", command, stdout.String())
		}
	}
	for _, excluded := range []string{" create ", " list ", " context ", "--permanent", "fuzzy"} {
		if strings.Contains(stdout.String(), excluded) {
			t.Errorf("root help unexpectedly contains %q: %q", excluded, stdout.String())
		}
	}
}

func writeAgentNote(t *testing.T, root, path, content string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
