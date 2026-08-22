package ui

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/notes"
)

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
		m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
		command := m.renderDocument()
		return m, command
	case scanResult:
		if message.generation != m.scanGeneration {
			return m, nil
		}
		m.loading = false
		if message.err != nil {
			m.status = fmt.Sprintf("scan: %v", message.err)
			return m, nil
		}
		selectedBefore, hadSelection := m.tree.Selected()
		wasScanned := m.scanned
		forceRead := m.forceRead
		m.forceRead = false
		preserveRatio := m.hasDocument && hadSelection && selectedBefore.Path == m.document.Path
		previousRatio := m.preview.viewport.ScrollPercent()
		if m.scanned {
			m.tree = m.tree.Reconcile(message.snapshot)
		} else {
			m.tree = NewTree(message.snapshot).SetViewport(m.tree.height)
			m.scanned = true
		}
		if m.pendingSelect != "" {
			m.tree, _ = m.tree.Select(m.pendingSelect)
			m.pendingSelect = ""
		}
		selectedAfter, stillSelected := m.tree.Selected()
		sameEntry := stillSelected && (selectedAfter.Path == selectedBefore.Path ||
			(selectedBefore.Identity != (notes.FileIdentity{}) && selectedAfter.Identity == selectedBefore.Identity))
		m.restoreRatio = preserveRatio && sameEntry
		if m.restoreRatio {
			m.previewRatio = previousRatio
		}
		if m.mutationStatus != "" {
			m.status = m.mutationStatus
			m.mutationStatus = ""
		} else {
			m.status = m.editorWarning
		}
		var read tea.Cmd
		selectionChanged := hadSelection != stillSelected ||
			(hadSelection && stillSelected && !sameSnapshotEntry(selectedBefore, selectedAfter))
		if forceRead || !wasScanned || selectionChanged ||
			(stillSelected && selectedAfter.Kind == notes.KindMarkdown && (!m.hasDocument || m.document.Path != selectedAfter.Path)) ||
			(!stillSelected && m.hasDocument) ||
			(stillSelected && selectedAfter.Kind != notes.KindMarkdown && m.hasDocument) {
			read = m.readSelected()
		} else if stillSelected && selectedAfter.Kind == notes.KindDirectory {
			m.renderDocument()
			read = m.startDirectoryPreview(selectedAfter)
		}
		return m, tea.Batch(read, m.startWatch(message.snapshot))
	case readResult:
		if message.generation != m.readGeneration {
			return m, nil
		}
		selected, ok := m.tree.Selected()
		if !ok || selected.Kind != notes.KindMarkdown || selected.Path != message.path {
			return m, nil
		}
		if message.err != nil {
			m.clearDocument()
			var tooLarge *notes.TooLargeError
			if errors.As(message.err, &tooLarge) {
				m.status = fmt.Sprintf("too large to preview: %d bytes (limit %d bytes)", tooLarge.Size, tooLarge.Limit)
			} else {
				m.status = fmt.Sprintf("read: %v", message.err)
			}
			return m, nil
		}
		m.document, m.hasDocument = message.document, true
		command := m.renderDocument()
		return m, command
	case directoryPreviewResult:
		if message.generation != m.readGeneration || message.err != nil {
			return m, nil
		}
		selected, ok := m.tree.Selected()
		if !ok || selected.Kind != notes.KindDirectory || selected.Path != message.path {
			return m, nil
		}
		m.directoryExcerpts = message.excerpts
		return m, m.renderDocument()
	case PreviewRenderedMsg:
		applied := m.preview.hasCurrent && message.Key == m.preview.current && message.Rendered
		command, err := m.preview.Update(message)
		if applied && err == nil && m.restoreRatio {
			maximum := max(0, m.preview.viewport.TotalLineCount()-m.preview.viewport.VisibleLineCount())
			m.preview.viewport.SetYOffset(int(m.previewRatio * float64(maximum)))
			m.restoreRatio = false
		}
		if err != nil {
			if !message.Rendered {
				m.clearDocument()
			}
			m.status = fmt.Sprintf("render: %v", err)
		}
		return m, command
	case watchStarted:
		if message.generation != m.watchGeneration {
			return m, nil
		}
		if message.err != nil {
			m.watchWarning = fmt.Sprintf("watch: %v", message.err)
			return m, nil
		}
		return m, waitForWatch(message.generation, message.changes)
	case watchResult:
		if message.generation != m.watchGeneration {
			return m, nil
		}
		if !message.open {
			if m.ctx.Err() == nil {
				m.watchWarning = "watch: stopped"
			}
			return m, nil
		}
		if message.change.Err != nil {
			m.watchWarning = fmt.Sprintf("watch: %v", message.change.Err)
		}
		return m, tea.Batch(m.startScan(), waitForWatch(message.generation, message.changes))
	case configWatchStarted:
		if message.err != nil {
			return m, m.setConfigWarning(fmt.Sprintf("config reload: watch: %v", message.err))
		}
		return m, waitForConfigWatch(message.results)
	case configWatchResult:
		if !message.open {
			m.configReloadGeneration++
			if m.ctx.Err() == nil {
				return m, m.setConfigWarning("config reload: watch stopped")
			}
			return m, nil
		}
		wait := waitForConfigWatch(message.results)
		m.configReloadGeneration++
		if message.result.Err != nil {
			return m, tea.Batch(wait, m.setConfigWarning(fmt.Sprintf("config reload: %v", message.result.Err)))
		}
		return m, tea.Batch(wait, m.resolveConfig(m.configReloadGeneration, message.result.Resolved))
	case configReloadResult:
		if message.generation != 0 && message.generation != m.configReloadGeneration {
			return m, nil
		}
		if message.err != nil {
			return m, m.setConfigWarning(fmt.Sprintf("config reload: %v", message.err))
		}
		watchChanged := m.cfg.Watch != message.resolved.Watch
		m.cfg.Theme = message.resolved.Theme
		m.cfg.ThemeColors = message.resolved.ThemeColors
		m.cfg.TreeWidth = message.resolved.TreeWidth
		m.cfg.NoColor = message.resolved.NoColor
		m.cfg.StatusBar = message.resolved.StatusBar
		m.cfg.Watch = message.resolved.Watch
		m.cfg.Preview.Wrap = message.resolved.Preview.Wrap
		m.cfg.Preview.RenderStyle = message.resolved.Preview.RenderStyle
		m.theme = message.resolved.theme
		m.themes = pickerThemes(m.theme, m.themeCatalog, m.cfg.NoColor)
		m.bindings = BindingsForKeymap(message.resolved.keymap)
		m.preview.SetRenderStyle(m.cfg.Preview.RenderStyle)
		if m.mode == ThemePicker {
			m.openThemePickerWithQuery(m.themePicker.input.Value())
		} else if m.mode == RenderStylePicker {
			m.openRenderStylePickerWithQuery(m.renderStylePicker.input.Value())
		} else if m.mode == CommandPalette {
			m.refreshPalette()
		}
		m.preview.SetWrap(m.cfg.Preview.Wrap)
		m.configWarning = ""
		m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
		if !m.cfg.Watch && m.watchCancel != nil {
			m.watchCancel()
			m.watchCancel = nil
			m.watchGeneration++
		}
		var scan tea.Cmd
		if watchChanged && m.cfg.Watch {
			scan = m.startScan()
		}
		return m, tea.Batch(scan, m.renderDocument())
	case searchReady:
		if m.mode != SearchPrompt || message.generation != m.search.generation || message.query != m.input.Value() {
			return m, nil
		}
		return m, m.searchContentCommand(m.search.ctx, message.generation, message.query, searchMaxResults-len(m.search.paths))
	case searchContentResult:
		if m.mode != SearchPrompt || message.generation != m.search.generation {
			return m, nil
		}
		if message.err != nil {
			if !errors.Is(message.err, context.Canceled) {
				m.search.err = fmt.Sprintf("search: %v", message.err)
			}
			return m, nil
		}
		m.search.matches = append(append([]notes.Match(nil), m.search.paths...), message.matches...)
		return m, nil
	case editorFinished:
		m.editorWarning = ""
		if message.err != nil {
			m.editorWarning = fmt.Sprintf("editor: %v", message.err)
		}
		m.status = m.editorWarning
		m.forceRead = true
		return m, m.startScan()
	case mutationResult:
		m.pendingSelect = message.path
		m.mutationStatus = ""
		if message.err != nil {
			m.mutationStatus = fmt.Sprintf("mutation: %v", message.err)
		}
		m.status = m.mutationStatus
		return m, m.startScan()
	case tea.MouseClickMsg:
		return m.updateMouseClick(message)
	case tea.MouseWheelMsg:
		return m.updateMouseWheel(message)
	case tea.KeyPressMsg:
		return m.updateKey(message)
	case tea.FocusMsg:
		return m, m.startScan()
	default:
		if m.focus == ContextPreview && m.mode == Browse && !m.help {
			command, err := m.preview.Update(message)
			if err != nil {
				m.status = fmt.Sprintf("preview: %v", err)
			}
			return m, command
		}
		return m, nil
	}
}

func (m Model) updateMouseClick(message tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := tea.Mouse(message)
	if m.mode != Browse || m.help || m.height < tinyHeight ||
		mouse.Button != tea.MouseLeft || mouse.X < 0 || mouse.X >= m.width || mouse.Y < 0 || mouse.Y >= bodyHeight(m.height, m.statusBarVisible()) {
		return m, nil
	}
	if m.fullPreview {
		if m.width < wideWidth {
			return m, nil
		}
		if mouse.X < treeWidth(m.width, m.cfg.TreeWidth) {
			selected := m.preview.tocIndex
			if m.focus != ContextTOC {
				selected = m.preview.headingAtOffset()
			}
			m.focus = ContextTOC
			height := max(1, bodyHeight(m.height, m.statusBarVisible())-2)
			row := mouse.Y - 1
			index := max(0, selected-height+1) + row
			if row >= 0 && row < height && index >= 0 && index < len(m.preview.toc) {
				m.preview.tocIndex = index
				m.preview.viewport.SetYOffset(m.preview.toc[index].line)
			}
			return m, nil
		}
		m.focus = ContextPreview
		return m, nil
	}
	treeHovered := m.focus == ContextTree
	if m.width >= wideWidth {
		treeHovered = mouse.X < treeWidth(m.width, m.cfg.TreeWidth)
	}
	if !treeHovered {
		m.focus = ContextPreview
		return m.clickDirectoryCard(mouse)
	}
	m.focus = ContextTree
	if mouse.Y == 0 || mouse.Y >= bodyHeight(m.height, m.statusBarVisible())-1 {
		return m, nil
	}
	index := m.tree.viewport + mouse.Y - 1
	entries := m.tree.visible()
	if index < 0 || index >= len(entries) {
		return m, nil
	}
	before, _ := m.tree.Selected()
	m.tree, _ = m.tree.Select(entries[index].Path)
	if entries[index].Path != before.Path {
		return m, m.readSelected()
	}
	return m, nil
}

func (m Model) clickDirectoryCard(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	directory, ok := m.tree.Selected()
	if !ok || directory.Kind != notes.KindDirectory {
		return m, nil
	}
	width := max(1, m.previewWidthForLayout())
	height := max(1, paneHeight(m.height, m.statusBarVisible())-2)
	originX := 2
	if m.width >= wideWidth {
		originX += treeWidth(m.width, m.cfg.TreeWidth)
	}
	x, y := mouse.X-originX, mouse.Y-2
	if x < 0 || y < 0 || x >= width || y >= height {
		return m, nil
	}
	entries := m.directoryEntries(directory)
	columns, cardWidth, cardHeight := directoryGridSize(width, height, len(entries))
	column := x / cardWidth
	if column >= columns || x >= columns*cardWidth {
		return m, nil
	}
	index := ((m.preview.viewport.YOffset()+y)/cardHeight)*columns + column
	if index < 0 || index >= len(entries) {
		return m, nil
	}
	m.tree, _ = m.tree.Select(entries[index].Path)
	return m, m.readSelected()
}

func (m Model) updateMouseWheel(message tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	mouse := tea.Mouse(message)
	if m.help {
		switch mouse.Button {
		case tea.MouseWheelUp:
			return m.scrollHelp(-m.preview.viewport.MouseWheelDelta), nil
		case tea.MouseWheelDown:
			return m.scrollHelp(m.preview.viewport.MouseWheelDelta), nil
		default:
			return m, nil
		}
	}
	if m.mode != Browse || mouse.X < 0 || mouse.X >= m.width || mouse.Y < 0 || mouse.Y >= bodyHeight(m.height, m.statusBarVisible()) {
		return m, nil
	}
	treeHovered := !m.fullPreview && m.focus == ContextTree
	if !m.fullPreview && m.width >= wideWidth {
		treeHovered = mouse.X < treeWidth(m.width, m.cfg.TreeWidth)
	}
	if treeHovered {
		delta := m.preview.viewport.MouseWheelDelta
		switch mouse.Button {
		case tea.MouseWheelUp:
			delta = -delta
		case tea.MouseWheelDown:
		default:
			return m, nil
		}
		m.tree = m.tree.Scroll(delta)
		return m, nil
	}
	command, err := m.preview.Update(message)
	if err != nil {
		m.status = fmt.Sprintf("preview: %v", err)
	}
	return m, command
}

func (m *Model) setConfigWarning(warning string) tea.Cmd {
	wasVisible := m.statusBarVisible()
	m.configWarning = warning
	if wasVisible == m.statusBarVisible() {
		return nil
	}
	m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
	return m.renderDocument()
}

func sameSnapshotEntry(left, right notes.Entry) bool {
	return left.Path == right.Path && left.Kind == right.Kind && left.Size == right.Size &&
		left.Identity == right.Identity && left.Modified.Equal(right.Modified)
}

func (m Model) updateKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := physicalKey(message).String()
	if m.mode == CommandPalette {
		return m.updatePalette(message)
	}
	if m.mode == SearchPrompt {
		if action, ok := actionForKeyPress(m.bindings, message, ContextSearch); ok && action == ActionPalette {
			m.openPalette()
			return m, nil
		}
		return m.updateSearch(message)
	}
	if isConfirmation(m.mode) {
		return m.updateConfirmation(message)
	}
	if m.mode == ThemePicker {
		action, ok := pickerAction(m.bindings, message)
		result := pickerResult{}
		if ok {
			result = m.updateThemePicker(action)
		} else {
			result = m.updateThemePickerInput(message)
		}
		if result.render {
			return m, m.renderDocument()
		}
		return m, nil
	}
	if m.mode == RenderStylePicker {
		action, ok := pickerAction(m.bindings, message)
		result := pickerResult{}
		if ok {
			result = m.updateRenderStylePicker(action)
		} else {
			result = m.updateRenderStylePickerInput(message)
		}
		if result.render {
			return m, m.renderDocument()
		}
		return m, nil
	}
	if m.mode != Browse {
		return m.updatePrompt(message)
	}
	if m.help {
		if action, ok := actionForKeyPress(m.bindings, message, ContextHelp); ok && action == ActionClose {
			m.help = false
			return m, nil
		}
		switch key {
		case "k", "up":
			m = m.scrollHelp(-1)
		case "j", "down":
			m = m.scrollHelp(1)
		}
		return m, nil
	}
	if m.fullPreview {
		if action, ok := actionForKey(m.bindings, message.String(), m.focus); ok {
			return m.dispatchAction(action)
		}
		if key == "esc" {
			return m, m.setFullPreview(false)
		}
		if m.focus == ContextPreview && (key == "left" || key == "h") {
			m.focus = ContextTOC
			m.preview.syncHeading()
			return m, nil
		}
	}
	if action, ok := actionForKeyPress(m.bindings, message, m.focus); ok {
		return m.dispatchAction(action)
	}
	if m.focus == ContextPreview {
		command, err := m.preview.Update(physicalKey(message))
		if err != nil {
			m.status = fmt.Sprintf("preview: %v", err)
		}
		return m, command
	}
	return m, nil
}

func pickerAction(bindings []Binding, message tea.KeyPressMsg) (Action, bool) {
	if message.Key().Text != "" {
		return "", false
	}
	action, ok := actionForKeyPress(bindings, message, ContextTheme)
	return action, ok
}

func (m Model) dispatchAction(action Action) (tea.Model, tea.Cmd) {
	if m.focus == ContextTOC {
		switch action {
		case ActionUp:
			m.preview.jumpHeading(-1)
		case ActionDown:
			m.preview.jumpHeading(1)
		case ActionExpand, ActionOpen:
			m.focus = ContextPreview
		case ActionCollapse:
			return m, m.setFullPreview(false)
		}
		if action == ActionUp || action == ActionDown || action == ActionExpand || action == ActionOpen {
			return m, nil
		}
	}
	switch action {
	case ActionFocus:
		if m.fullPreview {
			return m, m.setFullPreview(false)
		}
		if m.focus == ContextTree {
			m.focus = ContextPreview
		} else {
			m.focus = ContextTree
		}
	case ActionReload:
		return m, m.startScan()
	case ActionHelp:
		m.help = true
		m.helpOffset = 0
	case ActionThemeSelect:
		m.openThemePicker()
	case ActionSearch:
		m.openSearch()
	case ActionPalette:
		m.openPalette()
	case ActionPreviewRaw:
		m.preview.SetRaw(!m.preview.raw)
		return m, m.renderDocument()
	case ActionPreviewWrap:
		m.preview.SetWrap(!m.preview.wrap)
		return m, m.renderDocument()
	case ActionPreviewStyle:
		m.openRenderStylePicker()
	case ActionEdit:
		return m, m.editSelected()
	case ActionNewNote, ActionNewDirectory, ActionRename, ActionCopy, ActionMove, ActionTrash, ActionDelete:
		entry, ok := m.tree.Selected()
		if !ok && action != ActionNewNote && action != ActionNewDirectory {
			return m, nil
		}
		if (action == ActionCopy || action == ActionMove) && entry.Kind != notes.KindMarkdown {
			return m, nil
		}
		if action == ActionNewNote && isOnlyRootDirectory(m.tree.snapshot, entry) {
			entry = notes.Entry{}
		}
		m.mutationStatus = ""
		m.status = ""
		m.modalEntry = entry
		switch action {
		case ActionNewNote:
			m.mode = NewNotePrompt
		case ActionNewDirectory:
			m.mode = NewDirectoryPrompt
		case ActionRename:
			m.mode = RenamePrompt
		case ActionCopy:
			m.mode = CopyPrompt
		case ActionMove:
			m.mode = MovePrompt
		case ActionTrash:
			m.mode = TrashConfirm
		case ActionDelete:
			m.mode = PermanentDeleteConfirm
		}
		if isTextPrompt(m.mode) {
			m.input = newPrompt(m.mode, entry, m.tree.snapshot)
		}
		if isConfirmation(m.mode) {
			m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
		}
	case ActionQuit:
		m.cancelPending()
		return m, tea.Quit
	default:
		if m.focus == ContextTree {
			selected, ok := m.tree.Selected()
			if ok && selected.Kind == notes.KindMarkdown && (action == ActionOpen || action == ActionExpand) {
				return m, m.setFullPreview(true)
			}
			before, _ := m.tree.Selected()
			m.tree = m.tree.Update(action)
			after, ok := m.tree.Selected()
			if ok && after.Path != before.Path {
				return m, m.readSelected()
			}
		}
	}
	return m, nil
}

func isOnlyRootDirectory(snapshot notes.Snapshot, entry notes.Entry) bool {
	if entry.Kind != notes.KindDirectory || parentPath(entry.Path) != "" {
		return false
	}
	for _, candidate := range snapshot.Entries {
		if candidate.Path != entry.Path && parentPath(candidate.Path) == "" {
			return false
		}
	}
	return true
}

func (m *Model) setFullPreview(full bool) tea.Cmd {
	ratio := m.preview.viewport.ScrollPercent()
	m.fullPreview = full
	if full {
		m.focus = ContextTOC
		m.preview.syncHeading()
	} else {
		m.focus = ContextTree
	}
	command := m.renderDocument()
	if command != nil {
		m.previewRatio = ratio
		m.restoreRatio = true
	}
	return command
}

func (m *Model) readSelected() tea.Cmd {
	entry, ok := m.tree.Selected()
	if !ok || entry.Kind != notes.KindMarkdown {
		m.fullPreview = false
		m.focus = ContextTree
		if m.readCancel != nil {
			m.readCancel()
			m.readCancel = nil
		}
		m.clearDocument()
		m.directoryExcerpts = nil
		m.renderDocument()
		if ok && entry.Kind == notes.KindDirectory {
			return m.startDirectoryPreview(entry)
		}
		return nil
	}
	m.directoryExcerpts = nil
	return m.startRead(entry.Path)
}

func (m *Model) renderDocument() tea.Cmd {
	if m.width < 40 || m.height < 10 {
		return nil
	}
	width := max(1, m.previewWidthForLayout())
	height := max(1, paneHeight(m.height, m.statusBarVisible())-2)
	if entry, ok := m.tree.Selected(); ok && entry.Kind == notes.KindDirectory {
		m.preview.SetContent(m.directoryPreview(entry, width, height), width, height)
		return nil
	}
	if !m.hasDocument {
		return nil
	}
	return m.preview.RenderCommand(m.document, width, height, m.theme)
}
