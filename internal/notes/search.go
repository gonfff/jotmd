package notes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sahilm/fuzzy"
)

type MatchKind uint8

const (
	MatchPath MatchKind = iota
	MatchContent
)

type Match struct {
	Path    RelPath
	Kind    MatchKind
	Score   int
	Line    int
	Snippet string
}

type SearchStats struct {
	FilesScanned int
	FilesSkipped int
	Truncated    bool
}

func RankPaths(snapshot Snapshot, query string, limit int) []Match {
	limit = searchLimit(limit)
	if query == "" || limit == 0 {
		return nil
	}
	paths := make([]string, len(snapshot.Entries))
	for index, entry := range snapshot.Entries {
		paths[index] = string(entry.Path)
	}
	sort.Strings(paths)
	targets := make([]string, len(paths))
	for index, path := range paths {
		targets[index] = caseFoldKey(path)
	}
	ranks := fuzzy.Find(caseFoldKey(query), targets)
	sort.Stable(ranks)
	if len(ranks) > limit {
		ranks = ranks[:limit]
	}
	matches := make([]Match, len(ranks))
	for index, rank := range ranks {
		matches[index] = Match{Path: RelPath(paths[rank.Index]), Kind: MatchPath, Score: len(ranks) - index}
	}
	return matches
}

func (s *Store) SearchContent(ctx context.Context, query string, maxFileBytes int64, limit int) ([]Match, SearchStats, error) {
	limit = searchLimit(limit)
	if err := ctx.Err(); err != nil {
		return nil, SearchStats{}, err
	}
	if query == "" || limit == 0 {
		return nil, SearchStats{}, nil
	}
	snapshot, err := s.Scan(ctx)
	if err != nil {
		return nil, SearchStats{}, err
	}
	normalized := caseFoldKey(query)
	matches := make([]Match, 0, min(limit+1, 16))
	stats := SearchStats{}
	for _, entry := range snapshot.Entries {
		if err := ctx.Err(); err != nil {
			return nil, stats, err
		}
		if entry.Kind != KindMarkdown {
			continue
		}
		if maxFileBytes < 1 || entry.Size > maxFileBytes {
			stats.FilesSkipped++
			continue
		}
		contents, err := s.readSearchFile(entry.Path, maxFileBytes)
		if errors.Is(err, fs.ErrNotExist) || isSymlinkError(err) {
			continue
		}
		if err != nil {
			return nil, stats, fmt.Errorf("search note %q: %w", entry.Path, err)
		}
		if int64(len(contents)) > maxFileBytes {
			stats.FilesSkipped++
			continue
		}
		stats.FilesScanned++
		for lineIndex, line := range strings.Split(string(contents), "\n") {
			if err := ctx.Err(); err != nil {
				return nil, stats, err
			}
			normalizedLine := caseFoldKey(line)
			if matchByte := strings.Index(normalizedLine, normalized); matchByte >= 0 {
				matches = append(matches, Match{Path: entry.Path, Kind: MatchContent, Line: lineIndex + 1, Snippet: searchSnippet(line, normalizedLine, matchByte)})
				if len(matches) > limit {
					stats.Truncated = true
					return matches[:limit], stats, nil
				}
			}
		}
	}
	return matches, stats, nil
}

func searchSnippet(line, normalizedLine string, matchByte int) string {
	const maxRunes = 200
	runes := []rune(line)
	if len(runes) <= maxRunes {
		return strings.TrimSpace(line)
	}
	matchRune := utf8.RuneCountInString(normalizedLine[:matchByte])
	start := max(0, matchRune-maxRunes/4)
	end := min(len(runes), start+maxRunes)
	if end-start < maxRunes {
		start = max(0, end-maxRunes)
	}
	snippet := strings.TrimSpace(string(runes[start:end]))
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}

func (s *Store) readSearchFile(path RelPath, maxBytes int64) ([]byte, error) {
	file, err := openRelative(s.rootFD, path, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maxBytes+1))
}

func searchLimit(limit int) int {
	if limit < 1 {
		return 0
	}
	return min(limit, 200)
}
