package ui

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
	"github.com/gonfff/jotmd/internal/theme"
)

var updateGoldens = flag.Bool("update", false, "update UI golden files")

func TestViewReleasesMouseOnlyInPreview(t *testing.T) {
	model := testModel(t)
	for _, focus := range []Context{ContextTree, ContextTOC} {
		model.focus = focus
		if mode := model.View().MouseMode; mode != tea.MouseModeCellMotion {
			t.Fatalf("%s MouseMode = %v, want cell motion", focus, mode)
		}
	}

	model.focus = ContextPreview
	if mode := model.View().MouseMode; mode != tea.MouseModeNone {
		t.Fatalf("preview MouseMode = %v, want none", mode)
	}
}

func TestViewRequestsLayoutIndependentKeyEvents(t *testing.T) {
	enhancements := testModel(t).View().KeyboardEnhancements
	if !enhancements.ReportAlternateKeys || !enhancements.ReportAllKeysAsEscapeCodes || !enhancements.ReportAssociatedText {
		t.Fatalf("KeyboardEnhancements = %+v, want alternate keys, escaped keys, and associated text", enhancements)
	}
}

func TestFooterKeepsPriorityHintsCompleteAtNarrowWidths(t *testing.T) {
	model := testModel(t)
	got := ansi.Strip(model.statusView(24, ContextTree))
	if ansi.StringWidth(got) != 24 || !strings.HasSuffix(got, "? help  ") {
		t.Fatalf("statusView() = %q, want inset ? help", got)
	}

	model.status = "error"
	got = ansi.Strip(model.statusView(24, ContextTree))
	if ansi.StringWidth(got) != 24 || !strings.HasPrefix(got, "! error") || !strings.HasSuffix(got, "? help  ") {
		t.Fatalf("diagnostic statusView() = %q, want left diagnostic and right help", got)
	}
}

func TestBrowseStatusShowsSelectedPathBeforeDiagnostic(t *testing.T) {
	model := loadedModel(t)
	model.tree, _ = model.tree.Select("docs/guide.md")

	if got := ansi.Strip(model.statusView(80, ContextTree)); !strings.HasPrefix(got, "docs/guide.md") {
		t.Fatalf("statusView() = %q, want selected relative path on the left", got)
	}

	model.status = "error"
	if got := ansi.Strip(model.statusView(80, ContextTree)); !strings.HasPrefix(got, "docs/guide.md  ! error") {
		t.Fatalf("diagnostic statusView() = %q, want path before diagnostic", got)
	}
}

func TestBrowseViewHasNoHeaderAndFooterOnlyShowsHelp(t *testing.T) {
	model := sizedLoadedModel(t)
	view := ansi.Strip(model.View().Content)
	if strings.HasPrefix(view, "jotmd\n") {
		t.Fatalf("view retains header: %q", view)
	}
	if got := ansi.Strip(model.statusView(120, ContextTree)); ansi.StringWidth(got) != 120 || !strings.HasSuffix(got, "? help  ") {
		t.Fatalf("statusView() = %q, want right-aligned ? help", got)
	}
}

func TestBrowsePanesKeepContentOffBorders(t *testing.T) {
	view := ansi.Strip(goldenModel(t, "jotmd", false, 120, 18).View().Content)
	lines := strings.Split(view, "\n")
	if strings.Contains(lines[1], "Jot") {
		t.Fatalf("preview has no top padding:\n%s", view)
	}
	if !strings.Contains(lines[2], "│ Jot") || strings.Contains(lines[2], "│   Jot") {
		t.Fatalf("preview first line is shifted:\n%s", view)
	}
}

func TestBrowsePanePaintsConfiguredThemeBackground(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "jotmd", want: false},
		{name: "dracula", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := testModel(t)
			model.theme = builtinTheme(t, test.name)
			view := model.pane([]string{"note"}, 20, 4, false, false)
			if got := strings.Contains(view, "\x1b[48;2;40;42;54m"); got != test.want {
				t.Fatalf("pane paints configured background = %t, want %t: %q", got, test.want, view)
			}
			if got := strings.Contains(strings.SplitN(view, "\n", 2)[0], "48;2;40;42;54"); got != test.want {
				t.Fatalf("pane border paints configured background = %t, want %t: %q", got, test.want, view)
			}
		})
	}
}

func TestPaneRestoresConfiguredBackgroundAfterNestedStyleReset(t *testing.T) {
	model := testModel(t)
	model.theme = builtinTheme(t, "colorblind-light")
	marker := lipgloss.NewStyle().Foreground(lipgloss.Color(model.theme.Palette.Directory)).Render("• ")
	view := model.pane([]string{marker + "note"}, 20, 4, false, false)
	background := "\x1b[48;2;243;243;243m"
	if strings.Contains(view, "\x1b[mnote") || !strings.Contains(view, "\x1b[m"+background+"note") {
		t.Fatalf("pane does not restore background after nested reset: %q", view)
	}
}

func TestPopupsPaintConfiguredThemeBackground(t *testing.T) {
	model := testModel(t)
	model.theme.Palette.Background = "#282A36"
	model.openThemePicker()
	model.help = true
	background := regexp.MustCompile("\\x1b\\[[0-9;]*48;2;40;42;54(?:;[0-9;]*)?m")
	for name, view := range map[string]string{
		"picker": model.themePickerPopup(40, 8),
		"popup":  model.popupContent(40, 8),
	} {
		if !background.MatchString(view) {
			t.Errorf("%s does not paint configured background: %q", name, view)
		}
		if !strings.Contains(strings.SplitN(view, "\n", 2)[0], "48;2;40;42;54") {
			t.Errorf("%s border does not paint configured background: %q", name, view)
		}
	}
}

func TestPickerUsesBottomPopupInsteadOfCenteredPopup(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("t"))
	if popup := model.popupContent(40, 8); popup != "" {
		t.Fatalf("theme picker remains centered popup: %q", popup)
	}
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Theme: ") || !strings.Contains(view, "> jotmd") {
		t.Fatalf("bottom picker view = %q", view)
	}
}

func TestHelpUsesTitledWindowedGroupedLayout(t *testing.T) {
	model := testModel(t)
	model.width, model.height = 120, 30
	model.help = true
	width, height := model.popupSize()
	popup := model.popupContent(width, height)
	if lipgloss.Width(popup) != 98 || lipgloss.Height(popup) != 26 {
		t.Fatalf("help popup = %dx%d, want 98x26", lipgloss.Width(popup), lipgloss.Height(popup))
	}
	if first := strings.SplitN(ansi.Strip(popup), "\n", 2)[0]; !strings.Contains(first, "help") {
		t.Fatalf("help border lacks title: %q", first)
	}

	help := model.helpView(width, height)
	stripped := ansi.Strip(help)
	for _, group := range []string{"Navigation", "Notes", "Preview", "Application"} {
		if !strings.Contains(stripped, group) {
			t.Errorf("help lacks %q group:\n%s", group, stripped)
		}
	}
	key := lipgloss.NewStyle().Foreground(lipgloss.Color(model.theme.Palette.Heading)).Render("k/up       ")
	label := lipgloss.NewStyle().Foreground(lipgloss.Color(model.theme.Palette.Foreground)).Render("up")
	if !strings.Contains(help, key) || !strings.Contains(help, label) {
		t.Fatalf("help does not style key and label separately:\n%q", help)
	}
	if !strings.Contains(stripped, "esc close") {
		t.Fatalf("help lacks close hint:\n%s", stripped)
	}
	lines := strings.Split(stripped, "\n")
	positions := map[string]int{}
	for _, line := range lines {
		for _, group := range []string{"Navigation", "Notes", "Preview", "Application"} {
			if column := strings.Index(line, group); column >= 0 {
				positions[group] = column
			}
		}
	}
	if positions["Navigation"] != positions["Preview"] || positions["Notes"] != positions["Application"] || positions["Navigation"] == positions["Notes"] {
		t.Fatalf("help groups are not arranged in a 2x2 grid: %#v\n%s", positions, stripped)
	}
	upColumn, downColumn := -1, -2
	for _, line := range lines {
		if strings.Contains(line, "k/up") {
			upColumn = strings.LastIndex(line, "up")
		}
		if strings.Contains(line, "j/down") {
			downColumn = strings.LastIndex(line, "down")
		}
	}
	if upColumn < 0 || upColumn != downColumn {
		t.Fatalf("help descriptions are not aligned: up=%d down=%d\n%s", upColumn, downColumn, stripped)
	}
}

func TestHelpUsesOneHintPerSectionRow(t *testing.T) {
	model := loadedModel(t)
	bindings := make([]Binding, 0, 2)
	for _, binding := range model.bindings {
		if binding.Name == "note.new" || binding.Name == "directory.new" {
			bindings = append(bindings, binding)
		}
	}
	model.bindings = bindings
	for _, line := range model.helpLines(94) {
		line = ansi.Strip(line)
		if strings.Contains(line, "new note") && strings.Contains(line, "new directory") {
			t.Fatalf("help joins two hints in one row: %q", line)
		}
	}
}

func TestHelpFitsAllBindingsAtTwentyFourRowsAndCloseRemainsConfigurable(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{Bindings: []config.Binding{{Action: "help.close", Keys: []string{"x"}}}})
	model.width, model.height, model.help = 120, 26, true
	width, height := model.popupSize()
	help := ansi.Strip(model.helpView(width, height))
	if !strings.Contains(help, "x close") || strings.Contains(help, "esc close") {
		t.Fatalf("help footer does not use configured close key:\n%s", help)
	}
	for _, binding := range model.bindings {
		if len(binding.Keys) == 0 || (!hasContext(binding.Contexts, ContextTree) && !hasContext(binding.Contexts, ContextPreview)) {
			continue
		}
		if !strings.Contains(help, strings.Join(binding.Keys, "/")) || !strings.Contains(help, binding.Label) {
			t.Fatalf("24-row help lacks %q/%q:\n%s", binding.Keys, binding.Label, help)
		}
	}
	model = updateModel(t, model, key("esc"))
	if !model.help {
		t.Fatal("default escape closed help despite configured close key")
	}
	model = updateModel(t, model, key("x"))
	if model.help {
		t.Fatal("configured close key did not close help")
	}
}

func TestHelpScrollsAtShortTerminalWithCompactFooter(t *testing.T) {
	model := sizedLoadedModel(t)
	model.width, model.height = 80, 10
	model = updateModel(t, model, key("?"))
	width, height := model.popupSize()
	if first := ansi.Strip(model.helpView(width, height)); strings.Contains(first, "quit") {
		t.Fatalf("short help unexpectedly fits without scrolling:\n%s", first)
	}
	for range 20 {
		model = updateModel(t, model, key("down"))
	}
	last := ansi.Strip(model.helpView(width, height))
	if !strings.Contains(last, "quit") || !strings.Contains(last, "esc close") {
		t.Fatalf("scrolled help lacks final binding or close hint:\n%s", last)
	}
}

func TestTemporaryInterfacesOverlayBrowse(t *testing.T) {
	for _, test := range temporaryInterfaceCases() {
		if test.name == "commands" {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			model := sizedLoadedModel(t)
			model = updateModel(t, model, key(test.key))
			view := ansi.Strip(model.View().Content)
			for _, want := range []string{"docs", "note.md", test.title} {
				if !strings.Contains(view, want) {
					t.Fatalf("temporary view lacks %q:\n%s", want, view)
				}
			}
		})
	}
}

func TestWindowedMenusKeepBrowseChrome(t *testing.T) {
	for _, test := range []struct{ name, key, title, browse string }{
		{name: "commands", key: "P", title: "commands (", browse: "note.md"},
		{name: "help", key: "?", title: "help", browse: "╔"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := sizedLoadedModel(t)
			model = updateModel(t, model, key(test.key))
			view := ansi.Strip(model.View().Content)
			if !strings.Contains(view, test.title) || !strings.Contains(view, test.browse) {
				t.Fatalf("windowed menu hid browse chrome:\n%s", view)
			}
		})
	}
}

func TestPopupClipsToTerminal(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 24, Height: 6})
	model = updateModel(t, model, key("?"))
	view := ansi.Strip(model.View().Content)
	lines := strings.Split(view, "\n")
	if len(lines) > model.height {
		t.Fatalf("popup height = %d, want at most %d:\n%s", len(lines), model.height, view)
	}
	for index, line := range lines {
		if width := ansi.StringWidth(line); width > model.width {
			t.Fatalf("popup line %d width = %d, want at most %d: %q", index, width, model.width, line)
		}
	}
}

func TestTreeWidthUsesConfiguredPercentageWithinSafePaneBounds(t *testing.T) {
	for _, test := range []struct {
		percentage int
		want       int
	}{
		{percentage: 1, want: 24},
		{percentage: 32, want: 38},
		{percentage: 40, want: 48},
		{percentage: 100, want: 48},
	} {
		if got := treeWidth(120, test.percentage); got != test.want {
			t.Errorf("treeWidth(120, %d) = %d, want %d", test.percentage, got, test.want)
		}
	}
}

func TestNoColorViewMarksFocusSelectionAndErrorsWithTextOrShape(t *testing.T) {
	model := loadedModel(t)
	model.cfg.NoColor = true
	model.status = "read: failed"
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 18})
	view := model.View().Content
	for _, marker := range []string{"╔", "╭", "> ", "! read: failed"} {
		if !strings.Contains(view, marker) {
			t.Errorf("no-color view lacks %q marker:\n%s", marker, view)
		}
	}
}

func TestNoColorViewPreservesTerminalHyperlinks(t *testing.T) {
	model := sizedLoadedModel(t)
	model.cfg.NoColor = true
	target := "https://example.com/docs"
	link := ansi.SetHyperlink(target) + "docs" + ansi.ResetHyperlink()
	model.preview.SetContent(link, model.previewWidthForLayout(), 3)

	view := model.View().Content
	if strings.Contains(view, "\x1b[") {
		t.Fatalf("no-color view contains SGR styling: %q", view)
	}
	if !strings.Contains(view, target) || !strings.Contains(view, "\x1b]8;") {
		t.Fatalf("no-color view removed OSC-8 hyperlink: %q", view)
	}
}

func TestResponsiveStatesRemainExplicit(t *testing.T) {
	loading := testModel(t)
	loading = updateModel(t, loading, tea.WindowSizeMsg{Width: 80, Height: 12})
	if got := ansi.Strip(loading.View().Content); !strings.Contains(got, "Loading notes…") {
		t.Errorf("loading view = %q, want loading message", got)
	}

	root := t.TempDir()
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	empty := newModel(store, config.Defaults(), jotmdTheme(t))
	empty = updateModel(t, empty, tea.WindowSizeMsg{Width: 80, Height: 12})
	empty = updateModel(t, empty, run(empty.Init()))
	if got := ansi.Strip(empty.View().Content); !strings.Contains(got, "No notes found.") {
		t.Errorf("empty view = %q, want empty message", got)
	}

	failure := loadedModel(t)
	failure.status = "scan: permission denied"
	failure = updateModel(t, failure, tea.WindowSizeMsg{Width: 80, Height: 12})
	if got := ansi.Strip(failure.View().Content); !strings.Contains(got, "! scan: permission denied") {
		t.Errorf("error view = %q, want visible error marker", got)
	}

	search := loadedModel(t)
	search.openSearch()
	search.input.SetValue("missing")
	search.refreshSearch()
	search.width, search.height = 80, 12
	if got := ansi.Strip(search.View().Content); !strings.Contains(got, "Search") || !strings.Contains(got, "No matches.") {
		t.Errorf("search view = %q, want search and no-match states", got)
	}
}

func TestResponsiveThemeGoldens(t *testing.T) {
	themes := []struct {
		name    string
		noColor bool
	}{
		{name: "jotmd"},
		{name: "nord"},
		{name: "catppuccin-latte"},
		{name: "jotmd", noColor: true},
	}
	sizes := []struct {
		name          string
		width, height int
	}{
		{name: "wide", width: 120, height: 18},
		{name: "narrow", width: 80, height: 18},
		{name: "tiny", width: 39, height: 18},
	}

	for _, themeCase := range themes {
		for _, size := range sizes {
			name := themeCase.name
			if themeCase.noColor {
				name = "no-color"
			}
			t.Run(name+"/"+size.name, func(t *testing.T) {
				model := goldenModel(t, themeCase.name, themeCase.noColor, size.width, size.height)
				content := model.View().Content
				if themeCase.noColor {
					lines := strings.Split(content, "\n")
					for index := range lines {
						lines[index] = strings.TrimRight(lines[index], " ")
					}
					content = strings.Join(lines, "\n")
				}
				assertGolden(t, name+"_"+size.name+".golden", []byte(content))
			})
		}
	}
}

func goldenModel(t *testing.T, themeName string, noColor bool, width, height int) Model {
	t.Helper()
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "archive", "2025.md"), "# Archive\n")
	writeNote(t, filepath.Join(root, "projects", "jot.md"), "# Jot\n\nFast terminal notes.\n\n- [x] semantic themes\n- [x] live preview\n- [ ] ship it\n")
	writeNote(t, filepath.Join(root, "welcome.md"), "# Welcome\n")
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Watch = false
	cfg.NoColor = noColor
	th, err := theme.Builtin(themeName)
	if err != nil {
		t.Fatal(err)
	}
	th, err = theme.Apply(th, nil, noColor)
	if err != nil {
		t.Fatal(err)
	}
	model := newModel(store, cfg, th)
	model = updateModel(t, model, run(model.Init()))
	model.tree, _ = model.tree.Select("projects/jot.md")
	model = updateModel(t, model, run(model.readSelected()))
	model = updateModel(t, model, run(model.renderDocument()))
	model, command := updateModelCommand(t, model, tea.WindowSizeMsg{Width: width, Height: height})
	if command != nil {
		model = updateModel(t, model, run(command))
	}
	return model
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGoldens {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run go test ./internal/ui -run TestResponsiveThemeGoldens -update)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s mismatch (-want +got):\n%s", path, firstDifference(want, got))
	}
}

func firstDifference(want, got []byte) string {
	limit := min(len(want), len(got))
	for index := range limit {
		if want[index] != got[index] {
			return fmt.Sprintf("byte %d: %q != %q", index, want[index:], got[index:])
		}
	}
	return fmt.Sprintf("length %d != %d", len(want), len(got))
}
