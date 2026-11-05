package view

import (
	"fmt"
	"strings"

	"github.com/mbernet/ralph-wiggums/ui/internal/state"
)

// Footer is the live panel under the scrollback: where the pipeline is, what
// is happening now, the meters, who else is working and what needs a person.
func (v *View) Footer(s *state.State, o Options) string {
	t, g, w := v.t, v.t.G, o.Width
	if w < 20 {
		w = 20
	}
	lines := []string{v.rule(s, w), v.pipeline(s, w), v.activity(s, o, w), v.meters(s, w)}
	if l := v.people(s, w); l != "" {
		lines = append(lines, l)
	}
	switch {
	case o.Stopping && s.Exit == nil:
		lines = append(lines, trunc(t.Warn.Render(g.Stop+" stopping after this stage")+t.Faint.Render("  q cancels"), w))
	case o.Watching != "":
		lines = append(lines, trunc(t.Faint.Render("watching "+o.Watching+"  ")+v.keys(w, "q", "quit", "v", "detail", "b", "board"), w))
	case o.Live:
		if o.Help {
			lines = append(lines, v.keys(w, "q", "stop after this stage", "s", "skip the wait", "v", "detail "+o.Verbosity.String(),
				"b", "board", "^C", "interrupt, twice aborts"))
		} else {
			lines = append(lines, v.keys(w, "q", "stop", "s", "skip wait", "v", "detail", "b", "board", "?", "help"))
		}
	}
	for i := range lines {
		lines[i] = trunc(lines[i], w)
	}
	return strings.Join(lines, "\n")
}

func (v *View) keys(w int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, v.t.Key.Render(pairs[i])+" "+v.t.Faint.Render(pairs[i+1]))
	}
	return trunc(strings.Join(parts, "   "), w)
}

func (v *View) rule(s *state.State, w int) string {
	t, g := v.t, v.t.G
	left := g.Rule + g.Rule + " " + t.Accent.Render("ralph")
	if s.Session.Project != "" {
		left += " " + t.Strong.Render(s.Session.Project)
	}
	right := ""
	if s.Session.Worker != "" {
		right = s.Session.Worker
	}
	if st := s.Stage; st != nil && st.MaxIterations > 0 {
		right += fmt.Sprintf(" %s iter %d/%d", g.Dot, st.Iter, st.MaxIterations)
	}
	right = t.Faint.Render(right) + " " + g.Rule + g.Rule
	fill := w - width(left) - width(right) - 2
	if fill < 1 {
		return trunc(left, w)
	}
	return left + " " + t.Faint.Render(strings.Repeat(g.Rule, fill)) + " " + right
}

// pipeline draws the stages as a line, with the stage in progress lit. With
// nothing in progress it shows how many tasks wait in each stage.
func (v *View) pipeline(s *state.State, w int) string {
	t, g := v.t, v.t.G
	stages := s.Session.Pipeline
	if len(stages) == 0 {
		return ""
	}
	pos := 0
	if st := s.Stage; st != nil && st.Stage != "doctor" {
		pos = st.Pos
	}
	count := map[string]int{}
	for _, task := range s.Board {
		count[task.Stage]++
	}
	draw := func(compact bool) string {
		var parts []string
		for i, name := range stages {
			label := name
			if compact {
				label = g.Pending
			}
			switch {
			case pos > 0 && i+1 < pos:
				parts = append(parts, t.OK.Render(label))
			case pos > 0 && i+1 == pos:
				if compact {
					parts = append(parts, t.Accent.Render(g.Bullet))
				} else {
					parts = append(parts, t.Accent.Bold(true).Render(g.Bullet+" "+label))
				}
			default:
				if n := count[name]; n > 0 && pos == 0 && !compact {
					label += t.Faint.Render(fmt.Sprintf(" %d", n))
				}
				parts = append(parts, t.Muted.Render(label))
			}
		}
		line := "  " + strings.Join(parts, t.Faint.Render(" "+g.Pipe+g.Pipe+" "))
		if n := count["done"]; n > 0 && !compact {
			line += t.Faint.Render(" "+g.Pipe+g.Pipe+" ") + t.OK.Render(fmt.Sprintf("done %d", n))
		}
		if st := s.Stage; st != nil && st.Stage == "doctor" {
			line += "   " + t.Bad.Render(g.Sick+" doctor")
		}
		if compact && pos > 0 {
			line += "  " + t.Accent.Render(stages[pos-1]) + t.Faint.Render(fmt.Sprintf(" %d/%d", pos, len(stages)))
		}
		return line
	}
	if line := draw(false); width(line) <= w {
		return line
	}
	return draw(true)
}

func (v *View) activity(s *state.State, o Options, w int) string {
	t, g := v.t, v.t.G
	switch {
	case s.Exit != nil || s.End != nil:
		state := "ended"
		if s.End != nil {
			state = s.End.State
		}
		return "  " + v.stateStyle(state).Render(g.Stop+" session "+state)

	case s.Wait != nil:
		wt := s.Wait
		left := wt.Until - o.Now
		if left < 0 {
			left = 0
		}
		done := 0
		if wt.Seconds > 0 {
			done = int(int64(wt.Seconds)-left) * 100 / wt.Seconds
		}
		why := waitWhy(s, wt.Kind)
		line := "  " + t.Warn.Render(g.Clock+" "+wt.Kind) + "  " + v.bar(done, 14, t.Warn) + "  " +
			t.Strong.Render(Dur(int(left))) + t.Faint.Render(" left")
		if why != "" {
			line += t.Faint.Render("  " + why)
		}
		return line

	case s.Stage != nil:
		st := s.Stage
		spin := g.Spin[o.Spin%len(g.Spin)]
		elapsed := int(o.Now - st.StartedAt)
		left := "  " + t.Accent.Render(spin) + " " + t.Strong.Render(st.Task.ID) + " " + t.Accent.Render(st.Stage) +
			"  " + t.Strong.Render(Dur(elapsed))
		if st.Attempt > 1 {
			left += "  " + t.Warn.Render(fmt.Sprintf("attempt %d", st.Attempt))
		}
		var tools []string
		for _, tc := range st.Tools {
			tools = append(tools, fmt.Sprintf("%s %d", tc.Name, tc.N))
		}
		if len(tools) > 0 {
			left += "  " + t.Muted.Render(strings.Join(tools, " "))
		}
		if st.Errors > 0 {
			left += "  " + t.Bad.Render(fmt.Sprintf("%s%d", g.Bad, st.Errors))
		}
		right := ""
		if st.ClaimExpires > 0 {
			ttl := int(st.ClaimExpires - o.Now)
			style := t.Faint
			if ttl < 1800 {
				style = t.Warn
			}
			right = style.Render(g.Clock + " claim " + Dur(ttl))
		}
		return spread(left, right, w)

	case s.Idle:
		line := "  " + t.Muted.Render(g.Dot+" waiting for work")
		if n := len(s.Others()); n > 0 {
			line += t.Faint.Render(fmt.Sprintf("  %d held by other workers", n))
		}
		return line
	}
	if !s.Started {
		return "  " + t.Faint.Render(g.Dot+" starting")
	}
	return "  " + t.Muted.Render(g.Dot+" asking for work")
}

func waitWhy(s *state.State, kind string) string {
	switch kind {
	case "throttle", "window":
		if s.Pace != nil {
			return s.Pace.Why
		}
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

func (v *View) meters(s *state.State, w int) string {
	t, g := v.t, v.t.G
	c := s.Counts
	pct := 0
	if c.Total > 0 {
		pct = c.Done * 100 / c.Total
	}
	parts := []string{t.Faint.Render("tasks ") + v.bar(pct, 10, t.OK) + " " + fmt.Sprintf("%d/%d", c.Done, c.Total)}
	win := s.Window
	if win.Known() {
		style := t.OK
		budget := win.BudgetPct
		if budget == 0 {
			budget = 85
		}
		switch {
		case win.FiveHour.Pct >= budget:
			style = t.Bad
		case win.FiveHour.Pct*100 >= budget*80:
			style = t.Warn
		}
		five := t.Faint.Render("5h ") + v.bar(win.FiveHour.Pct, 10, style) + " " +
			style.Render(fmt.Sprintf("%d%%", win.FiveHour.Pct))
		if win.FiveHour.ResetsAt > 0 {
			five += t.Faint.Render(" " + g.Retry + Clock(win.FiveHour.ResetsAt))
		}
		parts = append(parts, five, t.Faint.Render("7d ")+fmt.Sprintf("%d%%", win.SevenDay.Pct))
	} else if win.Src != "" {
		parts = append(parts, t.Faint.Render("5h ?"))
	}
	left := "  " + strings.Join(parts, "   ")
	right := ""
	if n := len(s.History); n > 0 {
		right = t.Strong.Render(USD(s.CostU)) + t.Faint.Render(" "+g.Dot+" "+plural(n, "run", "runs"))
	}
	return spread(left, right, w)
}

func (v *View) people(s *state.State, w int) string {
	t, g := v.t, v.t.G
	others, needs := s.Others(), s.NeedsYou()
	if len(others) == 0 && len(needs) == 0 {
		return ""
	}
	left := ""
	if len(others) > 0 {
		left = "  " + t.Faint.Render("others ") + t.Info.Render(othersText(others, g))
	}
	right := ""
	if len(needs) > 0 {
		var ids []string
		for _, n := range needs {
			ids = append(ids, n.ID)
		}
		right = t.Bad.Render(fmt.Sprintf("%s %d needs you: %s", g.Flag, len(needs), strings.Join(ids, " ")))
	}
	return spread(left, right, w)
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
