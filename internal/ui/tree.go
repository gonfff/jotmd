package ui

import (
	"strings"

	"github.com/gonfff/jotmd/internal/notes"
)

type Tree struct {
	snapshot notes.Snapshot
	expanded map[notes.RelPath]bool
	selected notes.RelPath
	viewport int
	height   int
}

func NewTree(snapshot notes.Snapshot) Tree {
	tree := Tree{snapshot: snapshot, expanded: make(map[notes.RelPath]bool)}
	for _, entry := range snapshot.Entries {
		if entry.Kind == notes.KindDirectory {
			tree.expanded[entry.Path] = true
		}
	}
	if entries := tree.visible(); len(entries) != 0 {
		tree.selected = entries[0].Path
	}
	return tree
}

func (t Tree) Selected() (notes.Entry, bool) {
	for _, entry := range t.visible() {
		if entry.Path == t.selected {
			return entry, true
		}
	}
	return notes.Entry{}, false
}

func (t Tree) Select(path notes.RelPath) (Tree, bool) {
	found := false
	for _, entry := range t.snapshot.Entries {
		if entry.Path == path {
			found = true
			break
		}
	}
	if !found {
		return t, false
	}
	for parent := parentPath(path); parent != ""; parent = parentPath(parent) {
		t.expanded = withExpanded(t.expanded, parent, true)
	}
	t.selected = path
	return t.keepSelectedVisible(), true
}

func (t Tree) Update(action Action) Tree {
	entries := t.visible()
	index := t.selectedIndex(entries)
	switch action {
	case ActionUp:
		if index > 0 {
			t.selected = entries[index-1].Path
		}
	case ActionDown:
		if index >= 0 && index+1 < len(entries) {
			t.selected = entries[index+1].Path
		}
	case ActionFirst:
		if len(entries) != 0 {
			t.selected = entries[0].Path
		}
	case ActionLast:
		if len(entries) != 0 {
			t.selected = entries[len(entries)-1].Path
		}
	case ActionCollapse:
		if index >= 0 && entries[index].Kind == notes.KindDirectory && t.expanded[entries[index].Path] {
			t.expanded = withExpanded(t.expanded, entries[index].Path, false)
		} else if index >= 0 {
			if parent := parentPath(entries[index].Path); parent != "" {
				t.selected = parent
			}
		}
	case ActionExpand:
		if index >= 0 && entries[index].Kind == notes.KindDirectory {
			t.expanded = withExpanded(t.expanded, entries[index].Path, true)
		}
	case ActionParent:
		if index >= 0 {
			parent := parentPath(entries[index].Path)
			if parent != "" {
				t.selected = parent
			}
		}
	case ActionOpen:
		if index >= 0 && entries[index].Kind == notes.KindDirectory {
			t.expanded = withExpanded(t.expanded, entries[index].Path, !t.expanded[entries[index].Path])
		}
	}
	return t.keepSelectedVisible()
}

func (t Tree) SetViewport(height int) Tree {
	if height < 1 {
		height = 1
	}
	t.height = height
	return t.keepSelectedVisible()
}

func (t Tree) Scroll(delta int) Tree {
	t.viewport = min(max(0, len(t.visible())-t.height), max(0, t.viewport+delta))
	return t
}

func (t Tree) Reconcile(snapshot notes.Snapshot) Tree {
	previousPath := t.selected
	previousEntries := t.visible()
	previousIndex := t.selectedIndex(previousEntries)
	var previous notes.Entry
	if previousIndex >= 0 {
		previous = previousEntries[previousIndex]
	}
	t.snapshot = snapshot
	t.expanded = retainedExpansions(t.expanded, snapshot)
	if _, ok := t.Selected(); ok {
		return t.keepSelectedVisible()
	}
	entries := t.visible()
	if len(entries) == 0 {
		t.selected = ""
		return t.keepSelectedVisible()
	}
	if previous.Identity != (notes.FileIdentity{}) {
		matches := make([]notes.RelPath, 0, 1)
		for _, entry := range entries {
			if entry.Identity == previous.Identity {
				matches = append(matches, entry.Path)
			}
		}
		if len(matches) == 1 {
			t.selected = matches[0]
			return t.keepSelectedVisible()
		}
		if len(matches) > 1 {
			t.selected = ""
			return t.keepSelectedVisible()
		}
	}
	previousParent := parentPath(previousPath)
	newPaths := make(map[notes.RelPath]struct{}, len(entries))
	for _, entry := range entries {
		newPaths[entry.Path] = struct{}{}
	}
	for index := previousIndex + 1; index < len(previousEntries); index++ {
		candidate := previousEntries[index].Path
		if parentPath(candidate) == previousParent {
			if _, exists := newPaths[candidate]; exists {
				t.selected = candidate
				return t.keepSelectedVisible()
			}
		}
	}
	for index := previousIndex - 1; index >= 0; index-- {
		candidate := previousEntries[index].Path
		if parentPath(candidate) == previousParent {
			if _, exists := newPaths[candidate]; exists {
				t.selected = candidate
				return t.keepSelectedVisible()
			}
		}
	}
	if previousParent != "" {
		if selected, ok := t.Select(previousParent); ok {
			return selected
		}
	}
	t.selected = entries[0].Path
	return t.keepSelectedVisible()
}

func (t Tree) keepSelectedVisible() Tree {
	if t.height < 1 {
		return t
	}
	entries := t.visible()
	index := t.selectedIndex(entries)
	if index < 0 {
		t.viewport = 0
		return t
	}
	if index < t.viewport {
		t.viewport = index
	}
	if index >= t.viewport+t.height {
		t.viewport = index - t.height + 1
	}
	if maximum := len(entries) - t.height; maximum < t.viewport {
		t.viewport = maximum
	}
	if t.viewport < 0 {
		t.viewport = 0
	}
	return t
}

func (t Tree) visible() []notes.Entry {
	directories := make(map[notes.RelPath]struct{})
	for _, entry := range t.snapshot.Entries {
		if entry.Kind == notes.KindDirectory {
			directories[entry.Path] = struct{}{}
		}
	}
	visible := make([]notes.Entry, 0, len(t.snapshot.Entries))
	for _, entry := range t.snapshot.Entries {
		if t.hasCollapsedParent(entry.Path, directories) {
			continue
		}
		visible = append(visible, entry)
	}
	return visible
}

func (t Tree) hasCollapsedParent(path notes.RelPath, directories map[notes.RelPath]struct{}) bool {
	for parent := parentPath(path); parent != ""; parent = parentPath(parent) {
		if _, isDirectory := directories[parent]; isDirectory && !t.expanded[parent] {
			return true
		}
	}
	return false
}

func (t Tree) selectedIndex(entries []notes.Entry) int {
	for index, entry := range entries {
		if entry.Path == t.selected {
			return index
		}
	}
	return -1
}

func retainedExpansions(expanded map[notes.RelPath]bool, snapshot notes.Snapshot) map[notes.RelPath]bool {
	directories := make(map[notes.RelPath]struct{})
	for _, entry := range snapshot.Entries {
		if entry.Kind == notes.KindDirectory {
			directories[entry.Path] = struct{}{}
		}
	}
	retained := make(map[notes.RelPath]bool, len(directories))
	for path := range directories {
		retained[path] = true
	}
	for path, isExpanded := range expanded {
		if _, exists := directories[path]; exists {
			retained[path] = isExpanded
		}
	}
	return retained
}

func withExpanded(expanded map[notes.RelPath]bool, path notes.RelPath, value bool) map[notes.RelPath]bool {
	updated := make(map[notes.RelPath]bool, len(expanded)+1)
	for currentPath, isExpanded := range expanded {
		updated[currentPath] = isExpanded
	}
	updated[path] = value
	return updated
}

func parentPath(path notes.RelPath) notes.RelPath {
	parent := strings.LastIndex(string(path), "/")
	if parent < 0 {
		return ""
	}
	return path[:parent]
}
