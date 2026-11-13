package view

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
)

// Verbosity is how much of the agent's activity goes up the scrollback.
type Verbosity int

const (
	Quiet   Verbosity = iota // the stage cards and what needs attention
	Normal                   // plus what the agent says and the tools that fail
	Verbose                  // plus every tool call
)

func (v Verbosity) String() string {
	return [...]string{"quiet", "normal", "verbose"}[v]
}

type Options struct {
	Width     int
	Now       int64 // unix seconds, for the clocks that count down
	Verbosity Verbosity
	Live      bool // the panel can act: stop, skip
	Links     bool // OSC 8 hyperlinks
	Notify    bool // OSC 9 desktop notifications
	Stopping  bool // a stop was asked for
	Help      bool
	Spin      int
	Watching  string
}

type View struct {
	t Theme
	r *lipgloss.Renderer
}

func New(r *lipgloss.Renderer, ascii bool) *View {
	return &View{t: newTheme(r, ascii), r: r}
}

const gutter = 10 // the width of the time column, and of the indent under it

func (v *View) indent(s string) string { return strings.Repeat(" ", gutter) + s }

func (v *View) stamp(ts int64) string { return v.t.Faint.Render(Stamp(ts)) + "  " }

func (v *View) link(o Options, url, text string) string {
	if !o.Links || url == "" {
		return text
	}
	return ansi.SetHyperlink(url) + text + ansi.ResetHyperlink()
}

func notify(o Options, msg string) string {
	if !o.Notify {
		return ""
	}
	return "\x1b]9;" + msg + "\x07"
}

// Notice is a line of the UI's own, such as the answer to a key.
func (v *View) Notice(text string) string {
	return strings.Repeat(" ", gutter) + v.t.Faint.Render(v.t.G.Dot+" "+text)
}

// Scroll gives the lines an event adds to the scrollback. It is called after
// the event has been applied to s.
func (v *View) Scroll(s *state.State, e event.Event, o Options) []string {
	t, g, w := v.t, v.t.G, o.Width
	switch b := e.Body.(type) {
	case event.SessionStart:
		return v.header(b, o)

	case event.StageStart:
		if b.Attempt > 1 {
			return []string{trunc(v.stamp(e.Ts)+t.Warn.Render(fmt.Sprintf("%s %s %s attempt %d", g.Retry, b.Task.ID, b.Stage, b.Attempt)), w)}
		}
		left := v.stamp(e.Ts) + t.Accent.Render(g.Play) + " " + t.Strong.Render(b.Task.ID) + "  " +
			t.Accent.Render(b.Stage) + " " + t.Faint.Render(posLabel(b))
		right := t.Tier(b.Task.Tier) + t.Faint.Render(" "+shortModel(b.Model))
		head := spread(left, right, w)
		title := v.indent(t.Muted.Render(trunc(b.Task.Title, w-gutter)))
		return []string{"", head, title}

	case event.AgentText:
		if o.Verbosity < Normal {
			return nil
		}
		if o.Verbosity == Verbose {
			var out []string
			for i, l := range wrap(b.Text, w-gutter-2, 4) {
				lead := "  "
				if i == 0 {
					lead = t.Info.Render(g.Bullet) + " "
				}
				out = append(out, v.indent(lead+t.Muted.Render(l)))
			}
			return out
		}
		return []string{v.indent(t.Info.Render(g.Bullet) + " " + t.Muted.Render(trunc(b.Text, w-gutter-2)))}

	case event.AgentLine:
		if o.Verbosity < Normal || strings.TrimSpace(b.Text) == "" {
			return nil
		}
		return []string{v.indent(t.Faint.Render(g.Bar+" ") + trunc(b.Text, w-gutter-2))}

	case event.AgentTool:
		if o.Verbosity < Verbose {
			return nil
		}
		arg := b.Arg
		if strings.Contains(arg, "/") && !strings.Contains(arg, " ") {
			arg = truncLeft(arg, w-gutter-width(b.Name)-4)
		}
		return []string{trunc(v.indent(t.Faint.Render(g.Tool)+" "+b.Name+"  "+t.Faint.Render(arg)), w)}

	case event.AgentResult:
		if b.OK || o.Verbosity < Normal {
			return nil
		}
		tool := ""
		if s.Stage != nil {
			tool = s.Stage.LastTool + "  "
		}
		return []string{trunc(v.indent(t.Bad.Render(g.Bad+" "+tool)+t.Muted.Render(b.Error)), w)}

	case event.AgentCompact:
		return []string{v.indent(t.Warn.Render(g.Dot + " context compacted"))}

	case event.AgentRateLimit:
		if b.Status == "allowed" {
			return nil
		}
		return []string{v.indent(t.Warn.Render(fmt.Sprintf("%s rate limit %s (%s)", g.Warn, b.Status, b.Kind)))}

	case event.StageEnd:
		return v.card(s, e.Ts, b, o)

	case event.Notice:
		switch b.Level {
		case "warn":
			return []string{trunc(v.stamp(e.Ts)+t.Warn.Render(g.Warn+" "+b.Text), w)}
		case "error":
			return []string{trunc(v.stamp(e.Ts)+t.Bad.Render(g.Bad+" "+b.Text), w)}
		}
		return nil

	case event.Pace:
		switch b.Action {
		case "throttle", "wait_window":
			what := "throttle " + Dur(b.SleepS)
			if b.Action == "wait_window" {
				what = "holding for the window reset"
			}
			return []string{trunc(v.stamp(e.Ts)+t.Warn.Render(g.Pause+" "+what)+t.Faint.Render("  "+b.Why), w)}
		case "stop":
			return []string{trunc(v.stamp(e.Ts)+t.Bad.Render(g.Stop+" pacing stops the run")+t.Faint.Render("  "+b.Why), w)}
		}
		return nil

	case event.WorkNone:
		if b.Finished || s.IdleRun > 1 {
			return nil
		}
		line := v.stamp(e.Ts) + t.Muted.Render(g.Dot+" nothing eligible right now")
		if others := s.Others(); len(others) > 0 {
			line += t.Faint.Render("  " + othersText(others, g))
		}
		return []string{trunc(line, w)}

	case event.SessionEnd:
		return append([]string{""}, strings.Split(v.Report(s, o), "\n")...)

	case event.SessionExit:
		if s.End != nil {
			return nil
		}
		return []string{"", t.Bad.Render(fmt.Sprintf("%s ralph exited with code %d before the end of the session", g.Bad, b.Code))}
	}
	return nil
}

func posLabel(b event.StageStart) string {
	if b.Stage == "doctor" {
		return "sick in " + b.TaskStage
	}
	if b.Count == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", b.Pos, b.Count)
}

func shortModel(m string) string {
	return strings.TrimPrefix(m, "claude-")
}

func othersText(ts []event.Task, g Glyphs) string {
	var parts []string
	for _, t := range ts {
		parts = append(parts, fmt.Sprintf("%s %s %s %s", t.Claim.Worker, g.Arrow, t.ID, t.Stage))
	}
	return strings.Join(parts, " "+g.Dot+" ")
}

func (v *View) header(b event.SessionStart, o Options) []string {
	t, g, w := v.t, v.t.G, o.Width
	right := "run " + b.Run
	if b.DryRun {
		right = "dry run " + g.Dot + " " + b.Scenario
	}
	title := t.Badge.Render("RALPH") + "  " + t.Strong.Render(b.Project) + "  " + t.Muted.Render(b.Description)
	lines := []string{spread(title, t.Faint.Render(right), w)}

	tool := b.Tool
	if b.ToolVersion != "" {
		tool += " " + b.ToolVersion
	}
	pace := "pacing off"
	if b.Pace.Mode != "off" {
		pace = fmt.Sprintf("pacing %s %d%%", b.Pace.Mode, b.Pace.BudgetPct)
	}
	meta := []string{"worker " + b.Worker}
	if b.Branch != "" {
		meta = append(meta, "branch "+b.Branch)
	}
	meta = append(meta, tool, pace)
	if b.Story != "" {
		meta = append(meta, "only "+b.Story)
	}
	lines = append(lines, trunc("  "+t.Faint.Render(strings.Join(meta, " "+g.Dot+" ")), w))
	var stages []string
	for _, s := range b.Pipeline {
		stages = append(stages, t.Muted.Render(s))
	}
	lines = append(lines, trunc("  "+strings.Join(stages, t.Faint.Render(" "+g.Pipe+" ")), w))
	lines = append(lines, trunc("  "+t.Faint.Render("logs "+b.Logs), w))
	return lines
}

// card is the end of a stage: the verdict, what it cost, and the things a
// person should read: assumptions, the feedback of a reject, a diagnosis.
func (v *View) card(s *state.State, ts int64, b event.StageEnd, o Options) []string {
	t, g, w := v.t, v.t.G, o.Width
	var verdict string
	var bell string
	switch b.Outcome {
	case "pass":
		next := t.Muted.Render(b.Next)
		if b.Next == "done" {
			next = t.Done.Render("done")
		}
		verdict = t.OK.Render(g.OK+" pass") + t.Faint.Render(" "+g.Arrow+" ") + next
	case "reject":
		verdict = t.Warn.Render(g.Reject+" reject") + t.Faint.Render(" "+g.Arrow+" ") + t.Muted.Render(b.Next)
	case "sick":
		verdict = t.Bad.Render(g.Sick + " sick")
		if b.ByScript {
			verdict += t.Faint.Render("  marked by ralph")
		}
		bell = notify(o, b.Task+" is sick")
	case "conflict":
		verdict = t.Warn.Render(g.Lost+" result dropped") + t.Faint.Render("  another worker took the task over")
	case "dropped":
		verdict = t.Warn.Render(g.Drop+" let go") + t.Faint.Render("  its claim ends too soon to retry")
	default:
		why := b.Error
		if why == "" {
			why = b.Subtype
		}
		verdict = t.Bad.Render(g.Bad+" no result") + t.Faint.Render("  "+why)
	}
	head := v.stamp(ts) + t.Strong.Render(b.Task) + "  " + b.Stage + "  " + verdict + bell
	lines := []string{trunc(head, w)}

	var meta []string
	meta = append(meta, Dur(b.DurS))
	if b.CostU > 0 {
		meta = append(meta, USD(b.CostU))
	}
	if b.Turns > 0 {
		meta = append(meta, plural(b.Turns, "turn", "turns"))
	}
	if b.WinDelta > 0 {
		meta = append(meta, fmt.Sprintf("+%d%% 5h", b.WinDelta))
	}
	if b.Commit != nil {
		meta = append(meta, b.Commit.Sha+" "+b.Commit.Subject)
	}
	if b.PR != "" && (b.Outcome == "pass" || b.Outcome == "reject") {
		meta = append(meta, v.link(o, b.PR, "PR "+prNumber(b.PR)))
	}
	lines = append(lines, trunc(v.indent(t.Faint.Render(strings.Join(meta, " "+g.Dot+" "))), w))

	text := w - gutter - 4
	switch b.Outcome {
	case "reject", "sick":
		col := t.Warn
		if b.Outcome == "sick" {
			col = t.Bad
		}
		for i, l := range wrap(b.Body, text, 6) {
			lead := "  "
			if i == 0 {
				lead = col.Render("›") + " "
			}
			lines = append(lines, v.indent(lead+l))
		}
	}
	if n := len(b.Assumptions); n > 0 {
		lines = append(lines, v.indent(t.Warn.Render(g.Flag+" "+plural(n, "assumption", "assumptions"))))
		for _, a := range b.Assumptions {
			for i, l := range wrap(a, text, 3) {
				lead := "    "
				if i == 0 {
					lead = "  " + t.Faint.Render(g.Dot) + " "
				}
				lines = append(lines, v.indent(lead+t.Muted.Render(l)))
			}
		}
	}
	if b.Summary != "" && o.Verbosity >= Normal {
		for i, l := range wrap(b.Summary, text, 3) {
			lead := "  "
			if i == 0 {
				lead = t.OK.Render("Σ") + " "
			}
			lines = append(lines, v.indent(lead+t.Muted.Render(l)))
		}
	}
	if hint := classHint(b); hint != "" {
		lines = append(lines, trunc(v.indent(t.Faint.Render("hint  "+hint)), w))
	}
	return lines
}

func classHint(b event.StageEnd) string {
	switch b.Class {
	case "turns":
		return b.Task + " hit the turn limit: it may be oversized. Split it, or raise its model tier."
	case "budget":
		return "the per-iteration budget cap was hit: raise RALPH_ITER_BUDGET_USD or unset it."
	case "crash":
		return "the agent produced no result event: see its stderr log."
	case "ratelimit":
		return "rate limited: the stage keeps its claim and is retried."
	case "unavailable":
		return "the model API failed before any work: the stage keeps its claim and is retried."
	}
	return ""
}

// Replay writes the scrollback of a whole event list, the way a live session
// would have printed it. Used by `ralph-ui replay` and by the tests.
func Replay(out io.Writer, evs []event.Event, v *View, o Options) {
	s := state.New()
	for _, e := range evs {
		s.Apply(e)
		o.Now = e.Ts
		for _, l := range v.Scroll(s, e, o) {
			fmt.Fprintln(out, l)
		}
	}
}
