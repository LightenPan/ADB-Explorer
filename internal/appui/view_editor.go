package appui

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// hostsTheme colors the hosts editor: comments muted, addresses in the
// purple the app uses, and the gutter's numbers faint.

func hostsTheme(p palette) *editor.Theme {
	th := &editor.Theme{
		Background:        p.MainBG,
		Text:              p.EditorText,
		LineNumber:        p.LineNumber,
		ActiveLineNumber:  p.EditorText,
		ActiveLine:        ui.RGBA(0, 0, 0, 0.035),
		Selection:         p.AccentSoft,
		InactiveSelection: p.AccentSoft,
		Cursor:            p.EditorIP,
		MatchingBracket:   p.AccentIndicator,
		Match:             ui.RGBA(255, 200, 0, 0.3),
		CurrentMatch:      ui.RGBA(255, 140, 0, 0.45),
		Occurrence:        ui.RGBA(0, 0, 0, 0.07),
		IndentGuide:       p.Border,
		ActiveIndentGuide: p.TextFaint,
		WrapGuide:         p.Border,
		Whitespace:        p.TextFaint,
		Error:             p.EditorError,
		Warning:           ui.Hex("#9a6700"),
		Info:              ui.Hex("#0969da"),
		Hint:              p.TextMuted,
	}
	if p.Dark {
		th.ActiveLine = ui.RGBA(255, 255, 255, 0.05)
		th.Occurrence = ui.RGBA(255, 255, 255, 0.1)
		th.Match = ui.RGBA(255, 210, 60, 0.22)
		th.CurrentMatch = ui.RGBA(255, 160, 40, 0.5)
		th.Warning = ui.Hex("#d29922")
		th.Info = ui.Hex("#58a6ff")
	}
	th.Tokens[editor.TokenComment] = editor.TokenStyle{Color: p.EditorComment}
	th.Tokens[editor.TokenConstant] = editor.TokenStyle{Color: p.EditorIP}
	th.Tokens[editor.TokenNumber] = editor.TokenStyle{Color: p.EditorIP}
	th.Tokens[editor.TokenInvalid] = editor.TokenStyle{Color: p.EditorError}
	return th
}

// editorView draws the hosts text editor for the selected entry.
func (a *App) editorView(c *ui.Context, p palette, readOnly bool) {
	st := a.editorFor()
	st.state.ReadOnly = readOnly
	st.state.Theme = hostsTheme(p)
	syncEditorText(st, a.editorContent)
	editor.View(c, st.state).FillWidth().FillHeight()
	if st.state.Buffer.Modified() && !readOnly {
		a.SetEditorContent(st.state.Buffer.String())
	}
}

// syncEditorText replaces the buffer when the saved content changed under
// the editor, as after selecting another entry or reflecting a refresh.
func syncEditorText(st *editorState, content string) {
	if st.applied == content {
		return
	}
	st.applied = content
	if st.state.Buffer.String() != content {
		st.state.Buffer.Reset(content)
	}
}
