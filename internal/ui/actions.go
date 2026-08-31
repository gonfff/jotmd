package ui

import "github.com/gonfff/jotmd/internal/config"

type Action string

const (
	ActionUp           Action = "up"
	ActionDown         Action = "down"
	ActionFirst        Action = "first"
	ActionLast         Action = "last"
	ActionCollapse     Action = "collapse"
	ActionExpand       Action = "expand"
	ActionParent       Action = "parent"
	ActionOpen         Action = "open"
	ActionEdit         Action = "edit"
	ActionNewNote      Action = "new-note"
	ActionNewDirectory Action = "new-directory"
	ActionRename       Action = "rename"
	ActionCopy         Action = "copy"
	ActionMove         Action = "move"
	ActionTrash        Action = "trash"
	ActionDelete       Action = "delete"
	ActionFocus        Action = "focus"
	ActionReload       Action = "reload"
	ActionHelp         Action = "help"
	ActionThemeSelect  Action = "theme-select"
	ActionSearch       Action = "search"
	ActionPalette      Action = "palette"
	ActionPreviewRaw   Action = "preview-raw"
	ActionPreviewWrap  Action = "preview-wrap"
	ActionPreviewStyle Action = "preview-style"
	ActionAgentMemory  Action = "agent-memory"
	ActionQuit         Action = "quit"
	ActionClose        Action = "close"
)

const (
	ActionThemeUp    Action = "theme-up"
	ActionThemeDown  Action = "theme-down"
	ActionThemeApply Action = "theme-apply"
	ActionThemeClose Action = "theme-close"
)

type Context string

const (
	ContextTree    Context = "tree"
	ContextTOC     Context = "toc"
	ContextPreview Context = "preview"
	ContextHelp    Context = "help"
	ContextTheme   Context = "theme"
	ContextSearch  Context = "search"
)

type Binding struct {
	Name     string
	Action   Action
	Keys     []string
	Label    string
	Contexts []Context
}

func ActionRegistry() []config.Action {
	bindings := DefaultBindings()
	actions := make([]config.Action, len(bindings))
	for index, binding := range bindings {
		contexts := make([]string, len(binding.Contexts))
		for contextIndex, context := range binding.Contexts {
			contexts[contextIndex] = string(context)
		}
		actions[index] = config.Action{Name: binding.Name, Keys: append([]string(nil), binding.Keys...), Contexts: contexts}
	}
	return actions
}
