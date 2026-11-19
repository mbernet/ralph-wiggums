package view

import (
	"fmt"
	"strings"

	"github.com/mbernet/ralph-wiggums/ui/internal/state"
)

// status is what the box at the bottom says: what happens now and what it has
// cost, then the meters and what needs a person. The keys go under the box.
func (v *View) status(s *state.State, o Options, w int) []string {
	t, g := v.t, v.t.G
	act := v.activity(s, o, w)
	if o.Stopping && s.Exit == nil {
		act += "   " + t.Warn.Render(g.Stop+" stopping after this stage")
	}
	var right []string
	if pr := PR(s); pr != "" {
		right = append(right, v.prLink(o, pr))
	}
	if len(s.History) > 0 {
		right = append(right, t.Strong.Render(USD(s.CostU)))
	}
	cost := strings.Join(right, t.Faint.Render(" "+g.Dot+" "))

	needs := ""
	if ns := s.NeedsYou(); len(ns) > 0 {
		var ids []string
		for _, n := range ns {
			ids = append(ids, n.ID)
		}
		needs = t.Bad.Render(fmt.Sprintf("%s %d needs you: %s", g.Flag, len(ns), strings.Join(ids, " ")))
	}
	meters := v.meters(s, false)
	if full := v.meters(s, true); width(full)+width(needs)+3 <= w {
		meters = full
	}
	return []string{spread(act, cost, w), spread(meters, needs, w)}
}

// keyLine is the keys there are now, as caps, and where the body is scrolled.
func (v *View) keyLine(o Options, w int) string {
	t, g := v.t, v.t.G
	var left string
	switch {
	case o.Board || o.Help:
		left = v.keys(w, "b", "board", "?", "help", "q", "back")
	case o.Watching != "":
		left = t.Faint.Render("watching "+o.Watching+"   ") + v.keys(w, "q", "quit", "v", "detail", "b", "board", "?", "help")
	case o.Live && o.Stopping:
		left = v.keys(w, "q", "cancel the stop", "v", "detail", "b", "board", "z", "saver", "?", "help")
	case o.Live:
		left = v.keys(w, "q", "stop", "s", "skip", "v", "detail", "b", "board", "z", "saver", "?", "help")
	}
	if o.HasPR && !o.Board && !o.Help {
		left = v.keys(w, "o", "PR") + "  " + left
	}
	right := ""
	if o.Scroll > 0 && !o.Board && !o.Help {
		right = t.Warn.Render(fmt.Sprintf("%s %d back", g.Retry, o.Scroll)) + t.Faint.Render(" "+g.Dot+" ") + v.keys(w, "End", "follow")
	}
	return spread(left, right, w)
}

func (v *View) keys(w int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, v.t.KeyCap.Render(pairs[i])+" "+v.t.Muted.Render(pairs[i+1]))
	}
	return trunc(strings.Join(parts, "  "), w)
}

// activity is the first thing the status says: what this worker does now.
func (v *View) activity(s *state.State, o Options, w int) string {
	t, g := v.t, v.t.G
	switch {
	case s.Exit != nil || s.End != nil:
		state := "ended"
		if s.End != nil {
			state = s.End.State
		}
		return v.stateStyle(state).Render(g.Stop + " session " + state)

	case s.Wait != nil:
		wt := s.Wait
		left := max(wt.Until-o.Now, 0)
		done := 0
		if wt.Seconds > 0 {
			done = int(int64(wt.Seconds)-left) * 100 / wt.Seconds
		}
		icon := g.Clock
		switch wt.Kind {
		case "throttle", "window", "quota":
			icon = g.Pause
		}
		line := t.Warn.Render(icon+" "+wt.Kind) + " " + v.bar(done, 10, t.Warn) + " " +
			t.Strong.Render(Dur(int(left)))
		if why := waitWhy(s, wt.Kind); why != "" {
			line += t.Faint.Render(" " + g.Dot + " " + why)
		}
		return line

	case s.Stage != nil:
		st := s.Stage
		line := t.Accent.Render(g.Spin[o.Spin%len(g.Spin)] + " " + st.Stage)
		if st.Errors > 0 {
			line += "  " + t.Bad.Render(fmt.Sprintf("%s %d", g.Bad, st.Errors))
		}
		// The claim is only worth a look when it runs short.
		if ttl := int(st.ClaimExpires - o.Now); st.ClaimExpires > 0 && ttl < 1800 {
			line += t.Faint.Render(" "+g.Dot+" ") + t.Warn.Render("claim "+Dur(ttl))
		}
		return line

	case s.Idle:
		line := t.Muted.Render(g.Dot + " waiting for work")
		if n := len(s.Others()); n > 0 {
			line += t.Faint.Render(fmt.Sprintf("  %d held by other workers", n))
		}
		return line
	}
	if !s.Started {
		return t.Faint.Render(g.Dot + " starting")
	}
	return t.Muted.Render(g.Dot + " asking for work")
}

func waitWhy(s *state.State, kind string) string {
	switch kind {
	case "idle":
		return "nothing eligible right now"
	case "retry", "backoff":
		if s.Ended != nil {
			return "then " + s.Ended.Task.ID + " " + s.Ended.Stage + " again"
		}
	case "quota":
		return "no quota: not asking for work"
	case "api":
		return "the Ralph API did not answer"
	}
	return ""
}

// meters are the progress of the project and the quota window, with bars
// when full, as bare numbers when not.
func (v *View) meters(s *state.State, full bool) string {
	t, g := v.t, v.t.G
	c := s.Counts
	if c.Total == 0 && !s.Window.Known() {
		return ""
	}
	pct := 0
	if c.Total > 0 {
		pct = c.Done * 100 / c.Total
	}
	var parts []string
	if c.Total > 0 {
		bar := ""
		if full {
			bar = v.bar(pct, 10, t.OK) + " "
		}
		parts = append(parts, t.Faint.Render("tasks ")+bar+fmt.Sprintf("%d/%d", c.Done, c.Total))
	}
	win := s.Window
	if win.Known() {
		style, calm := t.OK, false
		budget := win.BudgetPct
		if budget == 0 {
			budget = 85
		}
		switch {
		case win.FiveHour.Pct >= budget:
			style = t.Bad
		case win.FiveHour.Pct*100 >= budget*80:
			style = t.Warn
		default:
			calm = true
		}
		five := t.Faint.Render("5h ")
		if full {
			five += v.bar(win.FiveHour.Pct, 10, style) + " "
		}
		five += style.Render(fmt.Sprintf("%d%%", win.FiveHour.Pct))
		// When the window resets, and the week, matter only near their limits.
		if full && !calm && win.FiveHour.ResetsAt > 0 {
			five += t.Faint.Render(" " + g.Retry + Clock(win.FiveHour.ResetsAt))
		}
		parts = append(parts, five)
		guard := win.WeekGuardPct
		if guard == 0 {
			guard = 90
		}
		if win.SevenDay.Pct*100 >= guard*80 {
			parts = append(parts, t.Faint.Render("7d ")+t.Warn.Render(fmt.Sprintf("%d%%", win.SevenDay.Pct)))
		}
	}
	return strings.Join(parts, "   ")
}

func (v *View) bar(pct, n int, style interface{ Render(...string) string }) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	full := pct * n / 100
	return style.Render(strings.Repeat(v.t.G.Full, full)) + v.t.Faint.Render(strings.Repeat(v.t.G.Empty, n-full))
}

func (v *View) stateStyle(state string) interface{ Render(...string) string } {
	switch state {
	case "complete":
		return v.t.OK
	case "incomplete", "exhausted", "stopped":
		return v.t.Warn
	}
	return v.t.Bad
}
