package cli

import (
	"io"

	"github.com/charmbracelet/lipgloss"
)

// Styles for terminal output. lipgloss drops colors automatically when the
// output is not a terminal or NO_COLOR is set.
var (
	accent  = lipgloss.Color("#ea580c")
	sOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("#16a34a")).Bold(true)
	sWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("#d97706")).Bold(true)
	sErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("#dc2626")).Bold(true)
	sTitle  = lipgloss.NewStyle().Foreground(accent).Bold(true)
	sMuted  = lipgloss.NewStyle().Foreground(lipgloss.Color("#78716c"))
	sPath   = lipgloss.NewStyle().Foreground(lipgloss.Color("#0284c7"))
	sBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
)

// render applies a style only when w is a terminal-capable writer.
func render(w io.Writer, st lipgloss.Style, s string) string {
	r := lipgloss.NewRenderer(w)
	return st.Renderer(r).Render(s)
}
