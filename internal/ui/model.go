package ui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/editor"
	"github.com/gonfff/jotmd/internal/notes"
	"github.com/gonfff/jotmd/internal/theme"
)

type Model struct {
	store             *notes.Store
	cfg               config.Config
	theme             theme.Theme
	themes            []theme.Theme
	themeCatalog      []theme.Theme
	bindings          []Binding
	configReloader    *config.Reloader
	resolveTheme      func(config.Config) (theme.Theme, error)
	editorCommand     editor.Command
	themePicker       themePickerState
	renderStylePicker renderStylePickerState
	search            searchState
	palette           paletteState
	input             textinput.Model
	mode              Mode
	modalEntry        notes.Entry
	modalError        string
	pendingSelect     notes.RelPath
	mutationStatus    string
	tree              Tree
	preview           Preview
	document          notes.Document
	directoryExcerpts map[notes.RelPath]string
	ctx               context.Context
	cancel            context.CancelFunc
	scanCtx           context.Context
	watchCancel       context.CancelFunc

	scanCancel, readCancel         context.CancelFunc
	scanGeneration, readGeneration uint64
	watchGeneration                uint64
	configReloadGeneration         uint64

	width, height int
	focus         Context
	status        string
	editorWarning string
	watchWarning  string
	configWarning string
	previewRatio  float64
	scanned       bool
	loading       bool
	help          bool
	helpOffset    int
	hasDocument   bool
	fullPreview   bool
	restoreRatio  bool
	forceRead     bool
}

func (m *Model) SetEditor(command editor.Command) {
	m.editorCommand = command
}

func (m *Model) SetConfigReloader(reloader *config.Reloader, resolveTheme func(config.Config) (theme.Theme, error)) {
	m.configReloader = reloader
	m.resolveTheme = resolveTheme
}

type scanResult struct {
	generation uint64
	snapshot   notes.Snapshot
	err        error
}

type readResult struct {
	generation uint64
	path       notes.RelPath
	document   notes.Document
	err        error
}

type directoryPreviewResult struct {
	generation uint64
	path       notes.RelPath
	excerpts   map[notes.RelPath]string
	err        error
}

type watchStarted struct {
	generation uint64
	changes    <-chan notes.Change
	err        error
}

type watchResult struct {
	generation uint64
	changes    <-chan notes.Change
	change     notes.Change
	open       bool
}

type configWatchStarted struct {
	results <-chan config.ReloadResult
	err     error
}

type configWatchResult struct {
	results <-chan config.ReloadResult
	result  config.ReloadResult
	open    bool
}

type resolvedConfig struct {
	config.Config
	keymap config.Keymap
	theme  theme.Theme
}

type configReloadResult struct {
	generation uint64
	resolved   resolvedConfig
	err        error
}

func NewModelWithOptions(store *notes.Store, cfg config.Config, th theme.Theme, keymap config.Keymap, themes []theme.Theme) Model {
	preview := NewPreview()
	preview.SetMaxBytes(cfg.Preview.MaxBytes)
	preview.SetWrap(cfg.Preview.Wrap)
	preview.SetRenderStyle(cfg.Preview.RenderStyle)
	ctx, cancel := context.WithCancel(context.Background())
	catalog := pickerThemes(th, themes, false)
	model := Model{store: store, cfg: cfg, theme: th, themes: pickerThemes(th, catalog, cfg.NoColor), themeCatalog: catalog, bindings: BindingsForKeymap(keymap), preview: preview, focus: ContextTree, ctx: ctx, cancel: cancel}
	model.startScan()
	return model
}

func (m Model) Init() tea.Cmd {
	scan := m.scanCommand(m.scanCtx, m.scanGeneration)
	if m.configReloader == nil {
		return scan
	}
	return tea.Batch(scan, m.startConfigWatch())
}

func (m *Model) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	m.cancelPending()
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.scanCancel, m.readCancel = nil, nil
	m.startScan()
}

func (m Model) scanCommand(ctx context.Context, generation uint64) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := m.store.Scan(ctx)
		return scanResult{generation: generation, snapshot: snapshot, err: err}
	}
}

func (m Model) readCommand(ctx context.Context, path notes.RelPath, generation uint64) tea.Cmd {
	return func() tea.Msg {
		document, err := m.store.Read(ctx, path)
		return readResult{generation: generation, path: path, document: document, err: err}
	}
}

func (m *Model) startScan() tea.Cmd {
	if m.scanCancel != nil {
		m.scanCancel()
	}
	m.scanGeneration++
	ctx, cancel := context.WithCancel(m.ctx)
	m.scanCtx, m.scanCancel = ctx, cancel
	m.loading = true
	return m.scanCommand(ctx, m.scanGeneration)
}

func (m *Model) startRead(path notes.RelPath) tea.Cmd {
	if m.readCancel != nil {
		m.readCancel()
	}
	m.readGeneration++
	ctx, cancel := context.WithCancel(m.ctx)
	m.readCancel = cancel
	return m.readCommand(ctx, path, m.readGeneration)
}

func (m *Model) startDirectoryPreview(directory notes.Entry) tea.Cmd {
	if m.readCancel != nil {
		m.readCancel()
	}
	m.readGeneration++
	ctx, cancel := context.WithCancel(m.ctx)
	m.readCancel = cancel
	entries := m.directoryEntries(directory)
	if len(entries) == 0 {
		m.directoryExcerpts = map[notes.RelPath]string{}
		return nil
	}
	generation := m.readGeneration
	return func() tea.Msg {
		excerpts := make(map[notes.RelPath]string, len(entries))
		for _, entry := range entries {
			document, err := m.store.Read(ctx, entry.Path)
			if err != nil {
				if ctx.Err() != nil {
					return directoryPreviewResult{generation: generation, path: directory.Path, err: ctx.Err()}
				}
				excerpts[entry.Path] = "Unavailable"
				continue
			}
			excerpts[entry.Path] = noteExcerpt(document.Content)
		}
		return directoryPreviewResult{generation: generation, path: directory.Path, excerpts: excerpts}
	}
}

func (m *Model) startWatch(snapshot notes.Snapshot) tea.Cmd {
	if !m.cfg.Watch {
		return nil
	}
	if m.watchCancel != nil {
		m.watchCancel()
	}
	m.watchGeneration++
	generation := m.watchGeneration
	ctx, cancel := context.WithCancel(m.ctx)
	m.watchCancel = cancel
	return func() tea.Msg {
		changes, err := m.store.Watch(ctx, snapshot, 100*time.Millisecond)
		return watchStarted{generation: generation, changes: changes, err: err}
	}
}

func waitForWatch(generation uint64, changes <-chan notes.Change) tea.Cmd {
	return func() tea.Msg {
		change, open := <-changes
		return watchResult{generation: generation, changes: changes, change: change, open: open}
	}
}

func (m Model) startConfigWatch() tea.Cmd {
	return func() tea.Msg {
		results, err := m.configReloader.Watch(m.ctx, 100*time.Millisecond)
		return configWatchStarted{results: results, err: err}
	}
}

func waitForConfigWatch(results <-chan config.ReloadResult) tea.Cmd {
	return func() tea.Msg {
		result, open := <-results
		return configWatchResult{results: results, result: result, open: open}
	}
}

func (m Model) resolveConfig(generation uint64, resolved config.Resolved) tea.Cmd {
	return func() tea.Msg {
		th, err := m.resolveTheme(resolved.Config)
		return configReloadResult{generation: generation, resolved: resolvedConfig{Config: resolved.Config, keymap: resolved.Keymap, theme: th}, err: err}
	}
}

func (m *Model) clearDocument() {
	m.document = notes.Document{}
	m.hasDocument = false
	m.previewRatio = 0
	m.restoreRatio = false
	m.preview.Clear()
}

func (m Model) cancelPending() {
	if m.scanCancel != nil {
		m.scanCancel()
	}
	if m.readCancel != nil {
		m.readCancel()
	}
	if m.watchCancel != nil {
		m.watchCancel()
	}
	if m.search.cancel != nil {
		m.search.cancel()
	}
	if m.cancel != nil {
		m.cancel()
	}
}

func (m Model) View() tea.View {
	content := m.render()
	if m.cfg.NoColor {
		content = theme.StripStyles(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	if m.focus != ContextPreview {
		view.MouseMode = tea.MouseModeCellMotion
	}
	view.ReportFocus = true
	view.KeyboardEnhancements.ReportAlternateKeys = true
	view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	view.KeyboardEnhancements.ReportAssociatedText = true
	return view
}
