package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
)

const (
	defaultCLISearchLimit = 20
	maxCLISearchLimit     = 200
	rootUsage             = `Usage:
  jotmd [TUI options]
  jotmd [--notes-dir PATH] search [--limit N] QUERY [--json]
  jotmd [--notes-dir PATH] get PATH [--json]
  jotmd [--notes-dir PATH] write [--if-revision REVISION] PATH [--json]
  jotmd [--notes-dir PATH] delete --if-revision REVISION PATH [--json]
  jotmd config check [PATH]

TUI options: --help --version --notes-dir --theme --editor --init-config
             --init-themes --list-themes --dump-theme --dump-config --dump-keys

Run jotmd COMMAND --help for command details.
`
)

type cliInvocation struct {
	command    string
	notesDir   *string
	jsonOutput bool
	help       bool
	limit      int
	ifRevision *string
	operands   []string
}

func parseCLIInvocation(args []string) (cliInvocation, bool, error) {
	invocation := cliInvocation{jsonOutput: hasJSONOption(args), limit: defaultCLISearchLimit}
	if !containsCLICommand(args) {
		return invocation, false, nil
	}

	options := true
	limitSet := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if options && argument == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(argument, "-") {
			name, value, hasValue := strings.Cut(argument, "=")
			switch name {
			case "--json":
				if hasValue {
					return invocation, true, fmt.Errorf("flag --json does not take a value")
				}
				invocation.jsonOutput = true
			case "--help", "-h":
				if hasValue {
					return invocation, true, fmt.Errorf("flag %s does not take a value", name)
				}
				invocation.help = true
			case "--notes-dir", "--limit", "--if-revision":
				if !hasValue {
					index++
					if index >= len(args) {
						return invocation, true, fmt.Errorf("flag %s requires a value", name)
					}
					value = args[index]
				}
				switch name {
				case "--notes-dir":
					invocation.notesDir = stringPointer(value)
				case "--limit":
					parsed, err := strconv.Atoi(value)
					if err != nil {
						return invocation, true, fmt.Errorf("--limit must be an integer")
					}
					invocation.limit = parsed
					limitSet = true
				case "--if-revision":
					invocation.ifRevision = stringPointer(value)
				}
			default:
				return invocation, true, fmt.Errorf("unknown flag %s", name)
			}
			continue
		}

		if invocation.command == "" {
			invocation.command = argument
		} else {
			invocation.operands = append(invocation.operands, argument)
		}
	}

	if invocation.help {
		return invocation, true, nil
	}
	if len(invocation.operands) != 1 {
		return invocation, true, fmt.Errorf("%s requires exactly one argument", invocation.command)
	}
	if invocation.limit < 1 || invocation.limit > maxCLISearchLimit {
		return invocation, true, fmt.Errorf("--limit must be between 1 and %d", maxCLISearchLimit)
	}
	if limitSet && invocation.command != "search" {
		return invocation, true, fmt.Errorf("--limit is only valid with search")
	}
	if invocation.ifRevision != nil && invocation.command != "write" && invocation.command != "delete" {
		return invocation, true, fmt.Errorf("--if-revision is only valid with write or delete")
	}
	if invocation.command == "delete" && invocation.ifRevision == nil {
		return invocation, true, fmt.Errorf("delete requires --if-revision")
	}
	return invocation, true, nil
}

func runCLI(ctx context.Context, invocation cliInvocation, stdin io.Reader, stdout, stderr io.Writer) int {
	if invocation.help {
		if err := writeCLIHelp(stdout, invocation.command); err != nil {
			return 1
		}
		return 0
	}
	store, err := loadCLIStore(invocation.notesDir)
	if err == nil {
		switch invocation.command {
		case "search":
			err = runSearch(ctx, store, invocation, stdout)
		case "get":
			err = runGet(ctx, store, invocation, stdout)
		case "write":
			err = runWrite(ctx, store, invocation, stdin, stdout)
		case "delete":
			err = runDelete(ctx, invocation, stdout, store.DeleteRevision)
		}
	}
	if err != nil {
		return writeCLIError(stderr, invocation.jsonOutput, err)
	}
	return 0
}

func writeCLIHelp(output io.Writer, command string) error {
	var help string
	switch command {
	case "search":
		help = `Usage: jotmd [--notes-dir PATH] search [--limit N] QUERY [--json]

Search Markdown paths fuzzily, then contents using a contiguous case-insensitive substring.
--limit accepts 1..200 and defaults to 20.
`
	case "get":
		help = `Usage: jotmd [--notes-dir PATH] get PATH [--json]

Print exact note bytes, or JSON containing path, content and revision.
`
	case "write":
		help = `Usage: jotmd [--notes-dir PATH] write [--if-revision REVISION] PATH [--json]

Read Markdown from stdin. A missing note is created; an existing note requires --if-revision.
`
	case "delete":
		help = `Usage: jotmd [--notes-dir PATH] delete --if-revision REVISION PATH [--json]

Permanently delete one unchanged Markdown note. This operation cannot be undone.
`
	}
	help += "Options may appear before or after operands; -- ends option parsing.\n"
	_, err := io.WriteString(output, help)
	return err
}

func loadCLIStore(notesDir *string) (*notes.Store, error) {
	path, err := config.DefaultPath(os.LookupEnv)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	cfg, err = config.ApplyEnv(cfg, os.LookupEnv)
	if err != nil {
		return nil, err
	}
	if notesDir != nil {
		cfg = config.Merge(cfg, config.Partial{NotesDir: notesDir})
	}
	cfg, err = config.Finalize(cfg)
	if err != nil {
		return nil, err
	}
	store, err := notes.NewStore(cfg.NotesDir, cfg.Ignore, cfg.ShowHidden)
	if err != nil {
		return nil, err
	}
	if err := store.SetReadMaxBytes(cfg.Preview.MaxBytes); err != nil {
		return nil, err
	}
	return store, nil
}

type searchResultJSON struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type searchStatsJSON struct {
	FilesScanned int `json:"files_scanned"`
	FilesSkipped int `json:"files_skipped"`
}

type searchJSON struct {
	Query     string             `json:"query"`
	Results   []searchResultJSON `json:"results"`
	Truncated bool               `json:"truncated"`
	Stats     searchStatsJSON    `json:"stats"`
}

func runSearch(ctx context.Context, store *notes.Store, invocation cliInvocation, output io.Writer) error {
	query := invocation.operands[0]
	if query == "" {
		return invalidCLIInput("search query must not be empty")
	}
	snapshot, err := store.Scan(ctx)
	if err != nil {
		return err
	}
	matches := notes.RankPaths(snapshot, query, invocation.limit)
	contentMatches, stats, err := store.SearchContentSnapshot(ctx, snapshot, query, 8<<20, invocation.limit-len(matches))
	if err != nil {
		return err
	}
	matches = append(matches, contentMatches...)
	if invocation.jsonOutput {
		results := make([]searchResultJSON, len(matches))
		for index, match := range matches {
			results[index] = searchResultJSON{
				Path:    string(match.Path),
				Line:    match.Line,
				Snippet: match.Snippet,
			}
		}
		return encodeCLIJSON(output, searchJSON{
			Query:     query,
			Results:   results,
			Truncated: stats.Truncated,
			Stats: searchStatsJSON{
				FilesScanned: stats.FilesScanned,
				FilesSkipped: stats.FilesSkipped,
			},
		})
	}
	for _, match := range matches {
		if match.Kind == notes.MatchPath {
			if _, err := fmt.Fprintln(output, match.Path); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(output, "%s:%d:  %s\n", match.Path, match.Line, match.Snippet); err != nil {
			return err
		}
	}
	return nil
}

type getJSON struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Content  string `json:"content"`
}

func runGet(ctx context.Context, store *notes.Store, invocation cliInvocation, output io.Writer) error {
	path, err := parseCLINotePath(invocation.operands[0])
	if err != nil {
		return err
	}
	document, err := store.Read(ctx, path)
	if err != nil {
		return err
	}
	if invocation.jsonOutput {
		if !utf8.Valid(document.Content) {
			return &cliError{code: "invalid_encoding", message: fmt.Sprintf("note %q is not valid UTF-8", path), exitCode: 2}
		}
		return encodeCLIJSON(output, getJSON{
			Path:     string(document.Path),
			Revision: string(document.Revision),
			Content:  string(document.Content),
		})
	}
	_, err = io.Copy(output, bytes.NewReader(document.Content))
	return err
}

type writeJSON struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Created  bool   `json:"created"`
}

func runWrite(ctx context.Context, store *notes.Store, invocation cliInvocation, input io.Reader, output io.Writer) error {
	path, err := parseCLINotePath(invocation.operands[0])
	if err != nil {
		return err
	}
	var expected *notes.Revision
	if invocation.ifRevision != nil {
		parsed, err := notes.ParseRevision(*invocation.ifRevision)
		if err != nil {
			return err
		}
		expected = &parsed
	}
	content, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("read note content: %w", err)
	}
	result, err := store.Write(ctx, path, content, expected)
	if err != nil {
		switch {
		case errors.Is(err, notes.ErrRevisionConflict):
			details := map[string]any{"path": string(path)}
			if expected != nil {
				details["expected_revision"] = string(*expected)
			}
			return &cliError{code: "revision_conflict", message: "note changed since it was read", details: details, exitCode: 4, cause: err}
		case errors.Is(err, notes.ErrExists):
			return &cliError{code: "already_exists", message: "note already exists", details: map[string]any{"path": string(path)}, exitCode: 4, cause: err}
		default:
			return err
		}
	}
	if invocation.jsonOutput {
		return encodeCLIJSON(output, writeJSON{
			Path:     string(result.Path),
			Revision: string(result.Revision),
			Created:  result.Created,
		})
	}
	_, err = fmt.Fprintln(output, result.Revision)
	return err
}

type deleteJSON struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Action   string `json:"action"`
}

func runDelete(ctx context.Context, invocation cliInvocation, output io.Writer, deleteNote func(context.Context, notes.RelPath, notes.Revision) error) error {
	path, err := parseCLINotePath(invocation.operands[0])
	if err != nil {
		return err
	}
	if invocation.ifRevision == nil {
		return invalidCLIInput("delete requires --if-revision")
	}
	revision, err := notes.ParseRevision(*invocation.ifRevision)
	if err != nil {
		return err
	}
	if err := deleteNote(ctx, path, revision); err != nil {
		if errors.Is(err, notes.ErrRevisionConflict) {
			return &cliError{
				code:    "revision_conflict",
				message: "note changed since it was read",
				details: map[string]any{
					"path":              string(path),
					"expected_revision": string(revision),
				},
				exitCode: 4,
				cause:    err,
			}
		}
		return err
	}
	if !invocation.jsonOutput {
		return nil
	}
	return encodeCLIJSON(output, deleteJSON{
		Path:     string(path),
		Revision: string(revision),
		Action:   "deleted",
	})
}

func parseCLINotePath(value string) (notes.RelPath, error) {
	if value == "" || filepath.IsAbs(value) || strings.ContainsAny(value, "\\\x00") {
		return "", invalidCLIInput("invalid note path %q", value)
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", invalidCLIInput("invalid note path %q", value)
		}
	}
	if !strings.EqualFold(filepath.Ext(parts[len(parts)-1]), ".md") {
		return "", invalidCLIInput("note path %q must end in .md", value)
	}
	return notes.RelPath(value), nil
}

type cliError struct {
	code     string
	message  string
	details  map[string]any
	exitCode int
	cause    error
}

func (e *cliError) Error() string { return e.message }
func (e *cliError) Unwrap() error { return e.cause }

func invalidCLIInput(format string, args ...any) error {
	return &cliError{code: "invalid_input", message: fmt.Sprintf(format, args...), exitCode: 2}
}

type errorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func writeCLIError(output io.Writer, jsonOutput bool, err error) int {
	mapped := mapCLIError(err)
	if mapped.details == nil {
		mapped.details = map[string]any{}
	}
	var outputErr error
	if jsonOutput {
		outputErr = encodeCLIJSON(output, errorEnvelope{Error: errorBody{
			Code:    mapped.code,
			Message: mapped.message,
			Details: mapped.details,
		}})
	} else {
		_, outputErr = fmt.Fprintf(output, "jotmd: %s: %s\n", mapped.code, mapped.message)
	}
	if outputErr != nil {
		return 1
	}
	return mapped.exitCode
}

func mapCLIError(err error) *cliError {
	var mapped *cliError
	if errors.As(err, &mapped) {
		return mapped
	}
	switch {
	case errors.Is(err, notes.ErrRecoveryRequired):
		return &cliError{code: "recovery_required", message: err.Error(), exitCode: 6, cause: err}
	case errors.Is(err, notes.ErrInvalidRevision), errors.Is(err, notes.ErrNotRegularFile):
		return &cliError{code: "invalid_input", message: err.Error(), exitCode: 2, cause: err}
	case errors.Is(err, fs.ErrNotExist):
		return &cliError{code: "not_found", message: err.Error(), exitCode: 3, cause: err}
	case errors.Is(err, notes.ErrRevisionConflict):
		return &cliError{code: "revision_conflict", message: err.Error(), exitCode: 4, cause: err}
	case errors.Is(err, notes.ErrExists):
		return &cliError{code: "already_exists", message: err.Error(), exitCode: 4, cause: err}
	case errors.Is(err, fs.ErrPermission):
		return &cliError{code: "permission_denied", message: err.Error(), exitCode: 5, cause: err}
	default:
		return &cliError{code: "operation_failed", message: err.Error(), exitCode: 1, cause: err}
	}
}

func encodeCLIJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func containsCLICommand(args []string) bool {
	options := true
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if options && argument == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(argument, "-") {
			name, _, hasValue := strings.Cut(argument, "=")
			if !hasValue && (name == "--notes-dir" || name == "--limit" || name == "--if-revision") {
				index++
			}
			continue
		}
		switch argument {
		case "search", "get", "write", "delete":
			return true
		default:
			return false
		}
	}
	return false
}

func hasJSONOption(args []string) bool {
	for _, argument := range args {
		if argument == "--" {
			return false
		}
		if argument == "--json" {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string {
	return &value
}
