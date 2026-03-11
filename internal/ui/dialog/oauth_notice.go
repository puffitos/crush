package dialog

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	uv "github.com/charmbracelet/ultraviolet"
)

// OAuthNoticeID is the identifier for the OAuth notice dialog.
const OAuthNoticeID = "oauth-notice"

// OAuthNotice is a dialog that informs the user about an OAuth
// authorization URL when the browser cannot be opened automatically
// (e.g. over SSH).
type OAuthNotice struct {
	com     *common.Common
	help    help.Model
	mcpName string
	authURL string
	sshHint string
	width   int
	keyMap  struct {
		Copy  key.Binding
		Close key.Binding
	}
}

var _ Dialog = (*OAuthNotice)(nil)

// NewOAuthNotice creates a new OAuth notice dialog.
func NewOAuthNotice(com *common.Common, mcpName, authURL, sshHint string) *OAuthNotice {
	d := &OAuthNotice{
		com:     com,
		mcpName: mcpName,
		authURL: authURL,
		sshHint: sshHint,
		width:   80,
	}
	d.help = help.New()
	d.help.Styles = com.Styles.DialogHelpStyles()
	d.keyMap.Copy = key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "copy URL"),
	)
	d.keyMap.Close = CloseKey
	return d
}

// ID implements [Dialog].
func (*OAuthNotice) ID() string {
	return OAuthNoticeID
}

// HandleMsg implements [Dialog].
func (d *OAuthNotice) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.keyMap.Copy):
			return ActionCmd{common.CopyToClipboardWithCallback(
				d.authURL,
				"OAuth URL copied to clipboard",
				nil,
			)}
		case key.Matches(msg, d.keyMap.Close):
			return ActionClose{}
		}
	}
	return nil
}

// Draw implements [Dialog].
func (d *OAuthNotice) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	dialogStyle := t.Dialog.View.Width(d.width)

	titleStyle := t.Dialog.Title
	headerOffset := titleStyle.GetHorizontalFrameSize() + dialogStyle.GetHorizontalFrameSize()
	title := common.DialogTitle(
		t,
		titleStyle.Render("Authorization Required"),
		d.width-headerOffset,
		t.Primary,
		t.Secondary,
	)

	whiteStyle := lipgloss.NewStyle().Foreground(t.White)
	mutedStyle := lipgloss.NewStyle().Foreground(t.FgMuted)
	linkStyle := lipgloss.NewStyle().Foreground(t.GreenDark).Underline(true)
	warnStyle := lipgloss.NewStyle().Foreground(t.Yellow)

	innerWidth := d.width - dialogStyle.GetHorizontalFrameSize() - 2

	instruction := whiteStyle.Width(innerWidth).Margin(0, 1).Render(
		fmt.Sprintf("MCP %q needs OAuth authorization. Open this URL in your browser:", d.mcpName),
	)

	link := linkStyle.Width(innerWidth).Margin(0, 1).Render(d.authURL)

	parts := []string{
		"",
		instruction,
		"",
		link,
	}

	if d.sshHint != "" {
		hint := warnStyle.Width(innerWidth).Margin(0, 1).Render(d.sshHint)
		parts = append(parts, "", hint)
	}

	copyHint := mutedStyle.Width(innerWidth).Margin(0, 1).Render(
		"Press c to copy the URL to your clipboard.",
	)
	parts = append(parts, "", copyHint, "")

	helpView := t.Dialog.HelpView.Render(d.help.View(d))
	elements := []string{
		title,
		strings.Join(parts, "\n"),
		helpView,
	}

	content := dialogStyle.Render(strings.Join(elements, "\n"))
	DrawCenter(scr, area, content)
	return nil
}

// ShortHelp implements [help.KeyMap].
func (d *OAuthNotice) ShortHelp() []key.Binding {
	return []key.Binding{d.keyMap.Copy, d.keyMap.Close}
}

// FullHelp implements [help.KeyMap].
func (d *OAuthNotice) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}
