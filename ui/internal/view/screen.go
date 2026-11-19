package view

import (
	"fmt"
	"strings"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
)

// Body is what the middle of the full screen shows: the stages that ended,
// folded into their cards, the lines of the session such as a throttle, and
// the messages of the stage in progress. Feed it every event, after the event
// has been applied to the state.
type Body struct {
	done []string // cards and one-off lines, oldest first
	cur  []string // the messages of the stage in progress
	last string   // the task of the last card: the cards of a task go together
}

const (
	margin   = 2 // left margin of the body
	maxDone  = 5000
	maxCur   = 1000
	nameCols = 8 // the column of a tool's name
)

func (b *Body) Add(v *View, s *state.State, e event.Event, o Options) {
	t, g := v.t, v.t.G
	o.Width -= margin
	o.Notify = false // not inside a full screen: the app notifies on its own
	w := o.Width - 4 // the messages sit under the stage, 4 in
	msg := func(icon, text string) {
		b.cur = append(b.cur, icon+"  "+text)
		if len(b.cur) > maxCur {
			b.cur = b.cur[len(b.cur)-maxCur:]
		}
	}
	switch m := e.Body.(type) {
	case event.SessionStart, event.SessionEnd:
		// The header and the report at the exit carry them.
	case event.StageStart:
		b.cur = nil
	case event.AgentText:
		if o.Verbosity < Normal {
			return
		}
		if o.Verbosity == Verbose {
			for i, l := range wrap(m.Text, w-3, 6) {
				if i == 0 {
					msg(t.Info.Render(g.Say), l)
				} else {
					msg(" ", l)
				}
			}
			return
		}
		msg(t.Info.Render(g.Say), trunc(m.Text, w-3))
	case event.AgentLine:
		if o.Verbosity >= Normal && strings.TrimSpace(m.Text) != "" {
			msg(t.Faint.Render(g.Bar), trunc(m.Text, w-3))
		}
	case event.AgentTool:
		if o.Verbosity < Normal {
			return
		}
		arg := m.Arg
		room := w - 3 - nameCols
		if strings.Contains(arg, "/") && !strings.Contains(arg, " ") {
			arg = truncLeft(arg, room)
		}
		msg(t.Accent.Render(v.toolIcon(m.Name)), trunc(fmt.Sprintf("%-*s", nameCols, m.Name)+t.Muted.Render(arg), w-3))
	case event.AgentResult:
		if m.OK || o.Verbosity < Normal {
			return
		}
		tool := ""
		if s.Stage != nil {
			tool = s.Stage.LastTool
		}
		msg(t.Bad.Render(g.Bad), trunc(t.Bad.Render(fmt.Sprintf("%-*s", nameCols, tool))+t.Muted.Render(m.Error), w-3))
	case event.AgentCompact:
		msg(t.Warn.Render(g.Compact), t.Warn.Render("context compacted"))
	case event.AgentRateLimit:
		if m.Status != "allowed" {
			msg(t.Warn.Render(g.Warn), t.Warn.Render(fmt.Sprintf("rate limit %s (%s)", m.Status, m.Kind)))
		}
	case event.StageEnd:
		b.cur = nil
		card := v.card(s, e.Ts, m, o)
		if m.Task != b.last {
			card = append([]string{""}, card...)
		}
		b.last = m.Task
		b.push(card)
	default:
		if lines := v.Scroll(s, e, o); len(lines) > 0 {
			b.last = ""
			b.push(lines)
		}
	}
}

// Note is a line of the UI's own, such as the answer to a key.
func (b *Body) Note(v *View, text string) {
	b.push([]string{v.Notice(text)})
}

func (b *Body) push(lines []string) {
	b.done = append(b.done, lines...)
	if len(b.done) > maxDone {
		b.done = b.done[len(b.done)-maxDone:]
	}
}

// toolIcon tells reading, editing and running apart at a glance.
func (v *View) toolIcon(name string) string {
	g := v.t.G
	switch name {
	case "Read", "Grep", "Glob", "LS", "NotebookRead":
		return g.Read
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return g.Edit
	case "Bash", "BashOutput":
		return g.Run
	}
	return g.Tool
}

// lines is the whole body at width w, oldest first.
func (b *Body) lines(v *View, s *state.State, o Options, w int) []string {
	out := make([]string, 0, len(b.done)+len(b.cur)+4)
	pad := strings.Repeat(" ", margin)
	for _, l := range b.done {
		out = append(out, pad+l)
	}
	if st := s.Stage; st != nil && s.Exit == nil {
		t, g := v.t, v.t.G
		right := t.Faint.Render("(") + t.Accent.Render(st.Stage) + t.Faint.Render(")") + "  " + t.Strong.Render(Dur(int(o.Now-st.StartedAt)))
		if st.Task.PR != "" {
			right = v.prLink(o, st.Task.PR) + "  " + right
		}
		if st.Attempt > 1 {
			right = t.Warn.Render(fmt.Sprintf("attempt %d", st.Attempt)) + "  " + right
		}
		left := t.Accent.Render(g.Bullet) + " " + t.Strong.Render(st.Task.ID) + "  " + st.Task.Title
		inner := w - margin
		out = append(out, "", pad+spread(left, right, inner), pad+t.Faint.Render(strings.Repeat(g.Rule, inner)))
		for _, l := range b.cur {
			out = append(out, pad+"  "+l)
		}
	}
	return out
}

// Screen is the whole terminal: the header, the body and the status box,
// exactly o.Height lines of at most o.Width. o.Scroll moves the body back.
func (v *View) Screen(s *state.State, b *Body, o Options) string {
	w, h := o.Width, o.Height
	if w < 20 {
		w = 20
	}
	if h < 4 {
		h = 4
	}
	head := []string{v.headLine(s, w-4)}
	status := v.status(s, o, w-4)
	if boxed(o) {
		head, status = v.box(head, w), v.box(status, w)
	}
	status = append(status, strings.Repeat(" ", margin)+v.keyLine(o, w-margin))
	room := h - len(head) - len(status)

	var body []string
	switch {
	case o.Board:
		body = strings.Split(v.Board(s, Options{Width: w - margin}), "\n")
		for i := range body {
			body[i] = strings.Repeat(" ", margin) + body[i]
		}
	case o.Help:
		body = v.help(s, w)
	default:
		all := b.lines(v, s, o, w)
		end := len(all) - clamp(o.Scroll, 0, max(0, len(all)-room))
		body = all[max(0, end-room):end]
	}
	if len(body) > room {
		body = body[:room]
	}
	for len(body) < room {
		body = append(body, "")
	}
	lines := append(append(head, body...), status...)
	for i := range lines {
		lines[i] = trunc(lines[i], w)
	}
	return strings.Join(lines[:h], "\n")
}

// MaxScroll is how far back the body can go on this screen.
func (v *View) MaxScroll(s *state.State, b *Body, o Options) int {
	chrome := 1 + 2 + 1 // the header, the status and the keys
	if boxed(o) {
		chrome += 4 // the borders of two boxes
	}
	return max(0, len(b.lines(v, s, o, o.Width))-(o.Height-chrome))
}

// boxed is whether the header and the status have a border: not when the
// terminal is too short to spare the rows.
func boxed(o Options) bool { return o.Height >= 12 }

func clamp(n, lo, hi int) int {
	return min(max(n, lo), hi)
}

func (v *View) box(lines []string, w int) []string {
	t, g := v.t, v.t.G
	c := g.Corners
	out := []string{t.Faint.Render(c[0] + strings.Repeat(g.Rule, w-2) + c[1])}
	for _, l := range lines {
		l = trunc(l, w-4)
		out = append(out, t.Faint.Render(g.Bar)+" "+l+strings.Repeat(" ", w-4-width(l))+" "+t.Faint.Render(g.Bar))
	}
	return append(out, t.Faint.Render(c[2]+strings.Repeat(g.Rule, w-2)+c[3]))
}

// headLine is the project on the left; who works and on what branch on the right.
func (v *View) headLine(s *state.State, w int) string {
	t, g := v.t, v.t.G
	left := t.Accent.Bold(true).Render("RALPH")
	if s.Session.Project != "" {
		left += t.Faint.Render(" — ") + t.Strong.Render(s.Session.Project)
	}
	var parts []string
	if br := s.Session.Branch; br != "" {
		if s.Session.BaseBranch != "" {
			br += " " + g.Arrow + " " + s.Session.BaseBranch
		}
		parts = append(parts, br)
	}
	if st := s.Stage; st != nil && st.MaxIterations > 0 {
		parts = append(parts, fmt.Sprintf("iter %d/%d", st.Iter, st.MaxIterations))
	}
	// The right side gives way from its start: the iteration matters more
	// than the branch.
	for len(parts) > 0 {
		right := t.Faint.Render(strings.Join(parts, " "+g.Dot+" "))
		if width(left)+width(right)+2 <= w {
			return spread(left, right, w)
		}
		parts = parts[1:]
	}
	return trunc(left, w)
}

// help is what the header leaves out, and every key.
func (v *View) help(s *state.State, w int) []string {
	t, g := v.t, v.t.G
	ss := s.Session
	pad := strings.Repeat(" ", margin)
	out := []string{""}
	if ss.Description != "" {
		for _, l := range wrap(ss.Description, w-margin*2, 3) {
			out = append(out, pad+t.Muted.Render(l))
		}
		out = append(out, "")
	}
	tool := ss.Tool
	if ss.ToolVersion != "" {
		tool += " " + ss.ToolVersion
	}
	pace := "off"
	if ss.Pace.Mode != "off" && ss.Pace.Mode != "" {
		pace = fmt.Sprintf("%s %d%%", ss.Pace.Mode, ss.Pace.BudgetPct)
	}
	run := ss.Run
	if ss.DryRun {
		run += " " + g.Dot + " dry run " + ss.Scenario
	}
	for _, kv := range [][2]string{
		{"run", run}, {"worker", ss.Worker}, {"branch", ss.Branch}, {"base", ss.BaseBranch},
		{"pipeline", strings.Join(ss.Pipeline, " "+g.Pipe+" ")}, {"tool", tool}, {"pacing", pace},
		{"only", ss.Story}, {"worktree", ss.Worktree}, {"logs", ss.Logs},
	} {
		if kv[1] != "" {
			out = append(out, pad+t.Faint.Render(fmt.Sprintf("%-10s", kv[0]))+kv[1])
		}
	}
	out = append(out, "")
	for _, kv := range [][2]string{
		{"q", "stop after this stage (again to cancel)"}, {"s", "skip the wait"},
		{"o", "open the PR of the task in the browser"},
		{"v", "detail: quiet, normal, verbose"}, {"b", "board"}, {"z", "screensaver"},
		{"↑ ↓ PgUp PgDn", "scroll the messages; End follows them again"},
		{"^C", "interrupt, twice aborts"}, {"?", "back"},
	} {
		out = append(out, pad+t.Key.Render(fmt.Sprintf("%-16s", kv[0]))+t.Faint.Render(kv[1]))
	}
	return out
}
