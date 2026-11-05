package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Dur is a length of time the way the plain output writes it: 45s, 4m 11s, 2h 05m.
func Dur(s int) string {
	if s < 0 {
		s = 0
	}
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm %02ds", s/60, s%60)
	default:
		return fmt.Sprintf("%dh %02dm", s/3600, s%3600/60)
	}
}

// USD reads micro-dollars, the unit ralph.sh keeps money in.
func USD(u int64) string {
	return fmt.Sprintf("$%d.%02d", u/1_000_000, u%1_000_000/10_000)
}

func Tok(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%d.%dk", n/1000, n%1000/100)
	default:
		return fmt.Sprintf("%d.%02dM", n/1_000_000, n%1_000_000/10_000)
	}
}

// Clock is a unix time as local HH:MM.
func Clock(epoch int64) string {
	if epoch <= 0 {
		return "?"
	}
	return time.Unix(epoch, 0).Format("15:04")
}

func Stamp(epoch int64) string {
	if epoch <= 0 {
		return "        "
	}
	return time.Unix(epoch, 0).Format("15:04:05")
}

func width(s string) int { return ansi.StringWidth(s) }

func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// truncLeft keeps the end of s, where a path carries its meaning.
func truncLeft(s string, w int) string {
	if width(s) <= w || w <= 1 {
		return trunc(s, w)
	}
	r := []rune(s)
	return "…" + string(r[len(r)-w+1:])
}

// spread puts left and right at the two ends of a line of width w. When both
// do not fit, the left side gives way.
func spread(left, right string, w int) string {
	lw, rw := width(left), width(right)
	if rw == 0 {
		return trunc(left, w)
	}
	if lw+rw+2 > w {
		left = trunc(left, w-rw-2)
		lw = width(left)
	}
	if lw+rw+2 > w {
		return trunc(left, w)
	}
	return left + strings.Repeat(" ", w-lw-rw) + right
}

// wrap word-wraps plain text into at most max lines of width w.
func wrap(text string, w, max int) []string {
	if w < 10 {
		w = 10
	}
	lines := strings.Split(ansi.Wordwrap(strings.TrimSpace(text), w, ""), "\n")
	var out []string
	for _, l := range lines {
		out = append(out, ansi.Hardwrap(l, w, true))
	}
	out = strings.Split(strings.Join(out, "\n"), "\n")
	if max > 0 && len(out) > max {
		out = out[:max]
		out[max-1] = trunc(out[max-1]+" …", w)
	}
	return out
}

func prNumber(url string) string {
	if url == "" {
		return ""
	}
	i := strings.LastIndex(url, "/")
	return "#" + url[i+1:]
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
