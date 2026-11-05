package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
)

// Board is the panel of the spec (section 9) in a terminal: a column per stage
// of the pipeline and one for the tasks that are done, then what needs a person.
func (v *View) Board(s *state.State, o Options) string {
	t, g, w := v.t, v.t.G, o.Width
	cols := append(append([]string{}, s.Session.Pipeline...), "done")
	byStage := map[string][]event.Task{}
	for _, task := range s.Board {
		stage := task.Stage
		if _, known := indexOf(cols, stage); !known {
			stage = cols[0]
		}
		byStage[stage] = append(byStage[stage], task)
	}
	for stage, ts := range byStage {
		sortTasks(ts, stage == "done")
		byStage[stage] = ts
	}

	c := s.Counts
	head := t.Badge.Render("BOARD") + "  " + t.Strong.Render(s.Session.Project) + "  " + t.Muted.Render(s.Session.Description)
	summary := fmt.Sprintf("%d tasks %s %d done %s %d claimed", c.Total, g.Dot, c.Done, g.Dot, c.Claimed)
	if c.NeedsHuman > 0 {
		summary += " " + g.Dot + " " + t.Bad.Render(fmt.Sprintf("%d needs you", c.NeedsHuman))
	}
	lines := []string{spread(head, t.Faint.Render(summary), w), ""}

	gap := 2
	cw := (w - gap*(len(cols)-1)) / len(cols)
	if cw >= 16 {
		var blocks []string
		for i, stage := range cols {
			block := []string{v.columnHead(stage, len(byStage[stage]), cw), t.Faint.Render(strings.Repeat(g.Rule, cw))}
			for _, task := range byStage[stage] {
				block = append(block, v.cardLines(s, task, cw)...)
				block = append(block, "")
			}
			style := v.r.NewStyle().Width(cw)
			if i < len(cols)-1 {
				style = style.MarginRight(gap)
			}
			blocks = append(blocks, style.Render(strings.Join(block, "\n")))
		}
		lines = append(lines, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, blocks...), "\n")...)
	} else {
		// Too narrow for columns: one group per stage, one task per line.
		for _, stage := range cols {
			if len(byStage[stage]) == 0 {
				continue
			}
			lines = append(lines, v.columnHead(stage, len(byStage[stage]), w))
			for _, task := range byStage[stage] {
				mark := v.mark(s, task)
				left := "  " + t.Strong.Render(task.ID) + "  " + t.Muted.Render(task.Title)
				lines = append(lines, spread(left, mark, w))
			}
		}
	}

	if needs := s.NeedsYou(); len(needs) > 0 {
		lines = append(lines, "", t.Bad.Bold(true).Render(g.Flag+" NEEDS YOU"))
		for _, n := range needs {
			lines = append(lines, trunc("  "+t.Strong.Render(n.ID)+"  "+t.Bad.Render("sick in "+n.Stage)+"  "+t.Muted.Render(n.Title), w))
			for _, l := range wrap(n.Why, w-6, 3) {
				lines = append(lines, "    "+t.Muted.Render(l))
			}
			lines = append(lines, "    "+t.Faint.Render("fix the cause, then  ")+t.Key.Render("./ralph.sh --story "+n.ID))
		}
		for _, b := range s.Blocked() {
			lines = append(lines, trunc("  "+t.Faint.Render(fmt.Sprintf("%s %s waits for %s", g.Block, b.ID, b.BlockedBy)), w))
		}
	}
	for i := range lines {
		lines[i] = trunc(strings.TrimRight(lines[i], " "), w)
	}
	return strings.Join(lines, "\n")
}

func (v *View) columnHead(stage string, n, w int) string {
	style := v.t.Muted.Bold(true)
	if stage == "done" {
		style = v.t.OK.Bold(true)
	}
	return trunc(style.Render(strings.ToUpper(stage))+v.t.Faint.Render(fmt.Sprintf(" %d", n)), w)
}

// cardLines: the id and who holds it, the title, and the marks of attention.
func (v *View) cardLines(s *state.State, task event.Task, w int) []string {
	t, g := v.t, v.t.G
	lines := []string{spread(t.Strong.Render(task.ID), v.mark(s, task), w)}
	lines = append(lines, t.Muted.Render(trunc(task.Title, w)))
	var marks []string
	if task.Rejects > 0 {
		marks = append(marks, t.Warn.Render(fmt.Sprintf("%s%d", g.Reject, task.Rejects)))
	}
	if task.Assumptions > 0 {
		marks = append(marks, t.Warn.Render(fmt.Sprintf("%s%d", g.Flag, task.Assumptions)))
	}
	if task.PR != "" {
		marks = append(marks, t.Faint.Render(prNumber(task.PR)))
	}
	if task.Claim != nil && s.LastTs > 0 {
		marks = append(marks, t.Faint.Render(g.Clock+Dur(int(task.Claim.ExpiresAt-s.LastTs))))
	}
	if len(marks) > 0 {
		lines = append(lines, trunc(strings.Join(marks, " "), w))
	}
	return lines
}

// mark says who holds a task, or why nobody can.
func (v *View) mark(s *state.State, task event.Task) string {
	t, g := v.t, v.t.G
	var mark string
	switch {
	case task.NeedsHuman:
		mark = t.Bad.Render(g.Flag + " you")
	case task.Sick:
		mark = t.Bad.Render(g.Sick + " sick")
	case task.Stage == "done":
		mark = t.OK.Render(g.OK + " " + Clock(task.DoneAt))
	case task.Claim != nil && task.Claim.Worker == s.Session.Worker:
		mark = t.Accent.Render(g.Bullet + " " + task.Claim.Worker)
	case task.Claim != nil:
		mark = t.Info.Render(g.Bullet + " " + task.Claim.Worker)
	case task.BlockedBy != "":
		mark = t.Faint.Render(g.Block + " " + task.BlockedBy)
	case task.Runs == 0:
		mark = t.Faint.Render(g.Pending)
	}
	return mark
}

func indexOf(list []string, s string) (int, bool) {
	for i, x := range list {
		if x == s {
			return i, true
		}
	}
	return 0, false
}

// sortTasks orders a column as work is handed out: priority, then id. Done
// tasks go by the time they closed, the latest first.
func sortTasks(ts []event.Task, done bool) {
	sort.SliceStable(ts, func(i, j int) bool {
		if done {
			return ts[i].DoneAt > ts[j].DoneAt
		}
		if ts[i].Priority != ts[j].Priority {
			return ts[i].Priority < ts[j].Priority
		}
		return ts[i].ID < ts[j].ID
	})
}
