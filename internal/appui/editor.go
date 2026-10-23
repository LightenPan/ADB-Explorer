package appui

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"
	"github.com/oldj/switchhosts/internal/store"
)

// hostsLanguage highlights hosts files: comments are muted and addresses
// stand out, as the web editor colors them.
var hostsLanguage = &editor.Language{
	Name:        "Hosts",
	Extensions:  []string{".hosts"},
	LineComment: "#",
	TabSize:     4,
	Tokenizer: editor.TokenizerFunc(func(line string, state uint64, tokens []editor.Token) ([]editor.Token, uint64) {
		trimmed := strings.TrimLeft(line, " \t")
		prefix := len(line) - len(trimmed)
		if strings.HasPrefix(trimmed, "#") {
			return append(tokens, editor.Token{End: prefix}, editor.Token{End: len(line), Kind: editor.TokenComment}), 0
		}
		if prefix > 0 {
			tokens = append(tokens, editor.Token{End: prefix})
		}
		address, rest := splitAddress(trimmed)
		if address == "" {
			return append(tokens, editor.Token{End: len(line)}), 0
		}
		addressEnd := prefix + len(address)
		tokens = append(tokens, editor.Token{End: addressEnd, Kind: editor.TokenConstant})
		// A comment after the mapping stays a comment.
		if index := strings.Index(rest, "#"); index >= 0 {
			commentStart := addressEnd + index
			tokens = append(tokens, editor.Token{End: commentStart})
			tokens = append(tokens, editor.Token{End: len(line), Kind: editor.TokenComment})
			return tokens, 0
		}
		return append(tokens, editor.Token{End: len(line)}), 0
	}),
}

// editorState is the code editor of one entry, kept between frames.
type editorState struct {
	entryID string
	buf     *editor.Buffer
	state   *editor.State
	// applied is the content the buffer last received from the app.
	applied string
}

// editorFor returns the editor of the selected entry, rebuilding it when
// the selection changed.
func (a *App) editorFor() *editorState {
	id := a.currentID
	if id == "" {
		id = "0"
	}
	if a.editor != nil && a.editor.entryID == id {
		return a.editor
	}
	buf := editor.NewBuffer(a.editorContent)
	buf.SetLanguage(hostsLanguage)
	buf.MarkSaved()
	st := &editor.State{
		Buffer:      buf,
		LineNumbers: editor.LineNumbersOn,
		ReadOnly:    a.IsReadOnly(),
		Label:       a.T("hosts_content"),
	}
	a.editor = &editorState{entryID: id, buf: buf, state: st, applied: a.editorContent}
	return a.editor
}

// discardEditor forgets the editor, so the next frame rebuilds it from the
// selected entry's saved content.
func (a *App) discardEditor() { a.editor = nil }

// scheduleEditorSave saves the entry once typing pauses, as the web editor's
// debounced save does. It uses a timer of its own rather than a frame: a
// frame only comes while the editor is drawing, and an edit made while the
// window is idle must still be written.
func (a *App) scheduleEditorSave() {
	if a.saveScheduled {
		return
	}
	a.saveScheduled = true
	go func() {
		time.Sleep(editorSaveDelay)
		a.mu.Lock()
		a.saveScheduled = false
		due := a.editorDirty
		a.mu.Unlock()
		if !due {
			return
		}
		a.flushEditor()
		a.refresh()
	}()
}

// flushEditor writes the editor's text, then reapplies the selection that
// uses the entry, so an edit reaches the system file without switching the
// entry again.
func (a *App) flushEditor() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.editorDirty {
		return
	}
	id := a.editorID
	a.SaveEditor()
	if a.editorError != "" || id == "" || id == "0" {
		return
	}
	// The editor's buffer is saved now, so it stops reporting changes and
	// does not schedule another save.
	if a.editor != nil && a.editor.state != nil {
		a.editor.state.Buffer.MarkSaved()
		a.editor.applied = a.editorContent
	}
	// Only a switched-on entry contributes to the system file; the web
	// interface reapplies for exactly that case.
	node := store.FindNode(a.list, id)
	if node == nil || !store.NodeOn(node) {
		return
	}
	a.reapplyAfterEdit()
}

// reapplyAfterEdit writes the system hosts file again after an entry's
// content changed. The caller holds the app's lock.
func (a *App) reapplyAfterEdit() {
	if a.apply.Pending() {
		a.addToast(a.T("fail"), a.T("hosts_application_unknown"), true)
		return
	}
	go func() {
		a.mu.Lock()
		err := a.applyList()
		a.mu.Unlock()
		if err != nil {
			a.addToast(a.T("fail"), err.Error(), true)
		}
		a.refresh()
	}()
}

// editorSaveDelay is how long typing must pause before the entry is saved,
// matching the web editor's debounce.
const editorSaveDelay = 600 * time.Millisecond
