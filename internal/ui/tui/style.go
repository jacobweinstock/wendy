package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// Colours follow the status vocabulary; every status also has its own glyph,
// so nothing depends on colour.
var (
	cPass  = lipgloss.Color("2")
	cFail  = lipgloss.Color("1")
	cErr   = lipgloss.Color("3")
	cRun   = lipgloss.Color("4")
	cPause = lipgloss.Color("5")
	cMute  = lipgloss.Color("8")

	sBold   = lipgloss.NewStyle().Bold(true)
	sMute   = lipgloss.NewStyle().Foreground(cMute)
	sTabOn  = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
	sTab    = lipgloss.NewStyle().Padding(0, 1)
	sSel    = lipgloss.NewStyle().Reverse(true)
	sTitle  = lipgloss.NewStyle().Bold(true).Foreground(cMute)
	sBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	sFail   = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(cFail).Padding(0, 1)
	sFooter = lipgloss.NewStyle().Foreground(cMute)
	sKey    = lipgloss.NewStyle().Bold(true)
)

func statusStyle(s event.Status) lipgloss.Style {
	switch vocab.StatusOf(s).Class {
	case "pass":
		return lipgloss.NewStyle().Foreground(cPass)
	case "fail":
		return lipgloss.NewStyle().Foreground(cFail)
	case "err":
		return lipgloss.NewStyle().Foreground(cErr)
	case "run":
		return lipgloss.NewStyle().Foreground(cRun)
	case "pause":
		return lipgloss.NewStyle().Foreground(cPause)
	}
	return lipgloss.NewStyle().Foreground(cMute)
}

func glyph(s event.Status) string { return statusStyle(s).Render(vocab.StatusOf(s).Glyph) }

func word(s event.Status) string { return vocab.StatusOf(s).Word }

// footer renders key hints: pairs of key and action.
func footer(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, sKey.Render(pairs[i])+" "+pairs[i+1])
	}
	return sFooter.Render(" " + strings.Join(parts, "  "))
}

// bar is a progress bar of done out of total.
func bar(done, total, width int, failed bool) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	n := done * width / total
	if n > width {
		n = width
	}
	c := cRun
	if failed {
		c = cFail
	}
	return lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("█", n)) + sMute.Render(strings.Repeat("░", width-n))
}

// fit pads or truncates s to exactly width cells.
func fit(s string, width int) string {
	w := lipgloss.Width(s)
	if w == width {
		return s
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	r := []rune(stripANSI(s))
	if width <= 1 {
		return string(r[:max(width, 0)])
	}
	if len(r) > width-1 {
		r = r[:width-1]
	}
	return string(r) + "…"
}

// stripANSI removes styling, for truncation.
func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			esc = true
		case esc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			esc = false
		case !esc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// box draws a titled pane of the given outer size.
func box(title, body string, width, height int, focused bool) string {
	st := sBox.Width(width).Height(height)
	if focused {
		st = st.BorderForeground(cRun)
	}
	if title != "" {
		body = sTitle.Render(strings.ToUpper(title)) + "\n" + body
	}
	return st.Render(clip(body, height-2))
}

// clip keeps at most n lines.
func clip(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
