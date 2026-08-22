package editor

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestResolve(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		flagValue  string
		configured []string
		wantArgs   []string
		wantError  string
	}{
		{
			name:       "flag wins and preserves quoted literal arguments",
			flagValue:  executable + ` --flag "two words" 'single value' escaped\ value "$HOME" '$(echo nope)' "*.md" "~" ""`,
			configured: []string{"ignored", "argument"},
			wantArgs:   []string{"--flag", "two words", "single value", "escaped value", "$HOME", "$(echo nope)", "*.md", "~", ""},
		},
		{
			name:       "configured arguments are unchanged",
			configured: []string{executable, "two words", `$HOME`, `*.md`, `~`, ""},
			wantArgs:   []string{"two words", `$HOME`, `*.md`, `~`, ""},
		},
		{
			name:      "double quotes preserve backslash before ordinary character",
			flagValue: executable + ` "a\q"`,
			wantArgs:  []string{`a\q`},
		},
		{name: "empty flag command", flagValue: "   ", wantError: "empty"},
		{name: "unterminated quote", flagValue: executable + ` "unfinished`, wantError: "quote"},
		{name: "trailing escape", flagValue: executable + ` trailing\`, wantError: "escape"},
		{name: "empty configured executable", configured: []string{"", "arg"}, wantError: "executable"},
		{name: "missing executable", configured: []string{"jotmd-editor-that-does-not-exist"}, wantError: "not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.flagValue, tt.configured)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.wantError) {
					t.Fatalf("Resolve() error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Executable != executable {
				t.Errorf("Resolve() executable = %q, want %q", got.Executable, executable)
			}
			if !reflect.DeepEqual(got.Args, tt.wantArgs) {
				t.Errorf("Resolve() args = %#v, want %#v", got.Args, tt.wantArgs)
			}
		})
	}
}

func TestResolveDefaultsToVi(t *testing.T) {
	dir := t.TempDir()
	vi := filepath.Join(dir, "vi")
	if err := os.WriteFile(vi, []byte(""), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("EDITOR", "")

	got, err := Resolve("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Executable != vi || len(got.Args) != 0 {
		t.Fatalf("Resolve() = %#v, want vi without arguments", got)
	}
}

func TestResolveUsesEnvironmentUnlessConfigured(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", executable+` --from-env "two words"`)

	got, err := Resolve("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Executable != executable || !reflect.DeepEqual(got.Args, []string{"--from-env", "two words"}) {
		t.Fatalf("Resolve() = %#v, want $EDITOR command", got)
	}

	got, err = Resolve("", []string{executable, "--from-settings"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Executable != executable || !reflect.DeepEqual(got.Args, []string{"--from-settings"}) {
		t.Fatalf("Resolve() = %#v, want settings command", got)
	}
}

func TestExecRunsEditorAndReportsExit(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name      string
		exit      string
		wantError bool
	}{
		{name: "success", exit: "0"},
		{name: "failure", exit: "1", wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "note.md")
			if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
				t.Fatal(err)
			}
			model := &execTestModel{
				command: Command{Executable: executable, Args: []string{"-test.run=TestFakeEditorProcess", "--", tt.exit}},
				path:    path,
			}
			if _, err := tea.NewProgram(model, tea.WithInput(nil), tea.WithOutput(io.Discard)).Run(); err != nil {
				t.Fatal(err)
			}
			if got := model.err != nil; got != tt.wantError {
				t.Errorf("callback error = %v, wantError %t", model.err, tt.wantError)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != "edited" {
				t.Errorf("note contents = %q, want external edit", contents)
			}
		})
	}
}

type execDoneMsg struct{ err error }

type execTestModel struct {
	command Command
	path    string
	err     error
}

func (m *execTestModel) Init() tea.Cmd {
	return Exec(m.command, m.path, func(err error) tea.Msg { return execDoneMsg{err: err} })
}

func (m *execTestModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if message, ok := message.(execDoneMsg); ok {
		m.err = message.err
		return m, tea.Quit
	}
	return m, nil
}

func (m *execTestModel) View() tea.View { return tea.NewView("") }

func TestFakeEditorProcess(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 {
		return
	}
	arguments := os.Args[separator+1:]
	if len(arguments) != 2 {
		os.Exit(2)
	}
	if err := os.WriteFile(arguments[1], []byte("edited"), 0o600); err != nil {
		os.Exit(2)
	}
	if arguments[0] == "1" {
		os.Exit(1)
	}
}
