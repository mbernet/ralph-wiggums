package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mbernet/ralph-wiggums/ui/internal/state"
)

// Report is the end of the session: how it ended, what it cost, what became
// of each task, and what is left for a person.
func (v *View) Report(s *state.State, o Options) string {
	t, g, w := v.t, v.t.G, o.Width
	end := s.End
	if end == nil {
		return ""
	}
	inner := w - 4
	if inner > 96 {
		inner = 96
	}
	style := v.stateStyle(end.State)
	title := style.Render(strings.ToUpper("ralph " + g.Dot + " " + end.State))
	right := t.Faint.Render(fmt.Sprintf("%s %s %s", plural(end.Ran, "run", "runs"), g.Dot, Dur(end.WallS)))

	c := s.Counts
	pct := 0
	if c.Total > 0 {
		pct = c.Done * 100 / c.Total
	}
	row := func(k, val string) string { return trunc(t.Faint.Render(fmt.Sprintf("%-8s", k))+val, inner) }
	rows := []string{
		spread(title, right, inner),
		"",
		row("tasks", v.bar(pct, 20, t.OK)+fmt.Sprintf("  %d/%d done", c.Done, c.Total)),
		row("runs", fmt.Sprintf("%d registered %s %d without a result %s %d retried", end.Registered, g.Dot, end.Failed, g.Dot, end.Retried)),
	}
	if end.CostU > 0 {
		cost := t.Strong.Render(USD(end.CostU))
		if end.Ran > 0 {
			cost += t.Faint.Render(" " + g.Dot + " avg " + USD(end.CostU/int64(end.Ran)))
		}
		for _, tier := range []string{"low", "med", "high"} {
			if tt, ok := end.Tiers[tier]; ok && tt.Runs > 0 {
				cost += t.Faint.Render(" "+g.Dot+" ") + t.Tier(tier) + t.Faint.Render(fmt.Sprintf(" %d %s", tt.Runs, USD(tt.CostU)))
			}
		}
		rows = append(rows, row("cost", cost))
		tk := end.Tokens
		rows = append(rows, row("tokens", fmt.Sprintf("in %s %s out %s %s cache %s", Tok(tk.In), g.Dot, Tok(tk.Out), g.Dot, Tok(tk.CacheRead))))
	}
	rows = append(rows, row("time", fmt.Sprintf("wall %s %s in agent %s %s throttled %s", Dur(end.WallS), g.Dot, Dur(end.AgentS), g.Dot, Dur(end.ThrottledS))))
	if s.Window.Known() {
		rows = append(rows, row("window", fmt.Sprintf("5h %s %d%% %s7d %d%%", v.bar(s.Window.FiveHour.Pct, 10, t.Warn), s.Window.FiveHour.Pct, g.Dot+" ", s.Window.SevenDay.Pct)))
	}

	box := v.r.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Dark: "#52525B", Light: "#A1A1AA"}).
		Padding(0, 1).Width(inner + 2)
	out := strings.Split(box.Render(strings.Join(rows, "\n")), "\n")

	if tl := s.Timeline(); len(tl) > 0 {
		out = append(out, "", "  "+t.Muted.Bold(true).Render("tasks this session"))
		for _, line := range tl {
			var steps []string
			dur, cost := 0, int64(0)
			for _, st := range line.Steps {
				steps = append(steps, v.step(st.Outcome, st.Stage))
				dur += st.DurS
				cost += st.CostU
			}
			meta := t.Faint.Render(Dur(dur))
			if cost > 0 {
				meta += t.Faint.Render(" " + USD(cost))
			}
			out = append(out, spread("  "+t.Strong.Render(fmt.Sprintf("%-8s", line.Task))+strings.Join(steps, " "), meta, w))
		}
	}
	if needs := s.NeedsYou(); len(needs) > 0 {
		out = append(out, "", "  "+t.Bad.Bold(true).Render(g.Flag+" needs you"))
		for _, n := range needs {
			out = append(out, trunc("  "+t.Strong.Render(n.ID)+"  "+t.Muted.Render(n.Why), w))
			out = append(out, "    "+t.Faint.Render("fix the cause, then  ")+t.Key.Render("./ralph.sh --story "+n.ID))
		}
	}
	out = append(out, "")
	if end.Next != "" {
		out = append(out, "  "+t.Faint.Render("next    ")+t.Key.Render(end.Next))
	}
	out = append(out, "  "+t.Faint.Render("logs    "+end.Logs))
	if bell := notify(o, "ralph: "+end.State); bell != "" {
		out[len(out)-1] += bell
	}
	for i := range out {
		out[i] = trunc(out[i], w)
	}
	return strings.Join(out, "\n")
}

func (v *View) step(outcome, stage string) string {
	t, g := v.t, v.t.G
	switch outcome {
	case "pass":
		return t.OK.Render(g.OK + stage)
	case "reject":
		return t.Warn.Render(g.Reject + stage)
	case "sick":
		return t.Bad.Render(g.Sick + stage)
	case "conflict":
		return t.Warn.Render(g.Lost + stage)
	case "dropped":
		return t.Warn.Render(g.Drop + stage)
	}
	return t.Faint.Render(g.Bad + stage)
}
