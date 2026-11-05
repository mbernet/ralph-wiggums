// Package view draws the state: the lines that go up the scrollback, the live
// panel at the bottom, the board and the final report. Every function takes a
// width and returns plain strings, so it can be tested without a terminal.
package view

import "github.com/charmbracelet/lipgloss"

type Glyphs struct {
	OK, Bad, Warn, Reject, Sick, Lost, Drop, Arrow, Dot, Bullet, Tool, Flag  string
	Clock, Retry, Pause, Stop, Full, Empty, Rule, Pipe, Pending, Block, Play string
	Bar                                                                      string
	Spin                                                                     []string
}

var unicodeGlyphs = Glyphs{
	OK: "✓", Bad: "✗", Warn: "⚠", Reject: "↺", Sick: "✚", Lost: "⇄", Drop: "⏏",
	Arrow: "→", Dot: "·", Bullet: "●", Tool: "▸", Flag: "⚑", Clock: "◷", Retry: "↻",
	Pause: "⏸", Stop: "■", Full: "▰", Empty: "▱", Rule: "─", Pipe: "━", Pending: "○",
	Block: "⊘", Play: "▶", Bar: "│",
	Spin: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
}

var asciiGlyphs = Glyphs{
	OK: "ok", Bad: "x", Warn: "!", Reject: "<-", Sick: "+", Lost: "<>", Drop: "^",
	Arrow: "->", Dot: "-", Bullet: "*", Tool: ">", Flag: "!", Clock: "@", Retry: "~",
	Pause: "||", Stop: "#", Full: "#", Empty: ".", Rule: "-", Pipe: "=", Pending: "o",
	Block: "/", Play: ">", Bar: "|",
	Spin: []string{"-", "\\", "|", "/"},
}

// Theme holds the styles. Colours are semantic and adapt to a light or dark
// terminal; with no colour support lipgloss drops them on its own.
type Theme struct {
	G                                     Glyphs
	Accent, Strong, Muted, Faint          lipgloss.Style
	OK, Warn, Bad, Info, Badge, Key, Done lipgloss.Style
	tiers                                 map[string]lipgloss.Style
}

func color(dark, light string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Dark: dark, Light: light}
}

func newTheme(r *lipgloss.Renderer, ascii bool) Theme {
	g := unicodeGlyphs
	if ascii {
		g = asciiGlyphs
	}
	accent := color("#A78BFA", "#6D28D9")
	st := func() lipgloss.Style { return r.NewStyle() }
	return Theme{
		G:      g,
		Accent: st().Foreground(accent),
		Strong: st().Bold(true),
		Muted:  st().Foreground(color("#A1A1AA", "#52525B")),
		Faint:  st().Foreground(color("#71717A", "#A1A1AA")),
		OK:     st().Foreground(color("#34D399", "#047857")),
		Warn:   st().Foreground(color("#FBBF24", "#B45309")),
		Bad:    st().Foreground(color("#F87171", "#B91C1C")),
		Info:   st().Foreground(color("#67E8F9", "#0E7490")),
		Badge:  st().Bold(true).Foreground(color("#18181B", "#FAFAFA")).Background(accent).Padding(0, 1),
		Key:    st().Foreground(color("#D4D4D8", "#3F3F46")).Bold(true),
		Done:   st().Foreground(color("#34D399", "#047857")).Bold(true),
		tiers: map[string]lipgloss.Style{
			"low":  st().Foreground(color("#67E8F9", "#0E7490")),
			"med":  st().Foreground(color("#60A5FA", "#1D4ED8")),
			"high": st().Foreground(color("#F472B6", "#BE185D")),
		},
	}
}

func (t Theme) Tier(tier string) string {
	if s, ok := t.tiers[tier]; ok {
		return s.Render(tier)
	}
	return t.Muted.Render(tier)
}
