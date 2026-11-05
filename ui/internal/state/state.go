// Package state folds the event stream into what the views draw: the session,
// the stage in progress, the wait in progress, the board and the history.
// Apply is pure bookkeeping; nothing here knows about terminals.
package state

import "github.com/mbernet/ralph-wiggums/ui/internal/event"

type ToolCount struct {
	Name string
	N    int
}

// Stage is the stage in progress, with what its agent has done so far.
type Stage struct {
	event.StageStart
	StartedAt  int64
	AgentModel string
	Tools      []ToolCount // in order of first use
	Calls      int
	Errors     int
	LastTool   string
	LastText   string
}

type Wait struct {
	event.WaitStart
	Started int64
}

type Step struct {
	Stage   string
	Outcome string
	Class   string
	Attempt int
	DurS    int
	CostU   int64
}

// TaskLine is what happened to one task in this session, in order.
type TaskLine struct {
	Task  string
	Steps []Step
}

type State struct {
	Session  event.SessionStart
	Started  bool
	Counts   event.Counts
	Board    []event.Task
	HasBoard bool
	Window   event.Window
	Stage    *Stage
	Ended    *Stage // the stage the last stage.end closed
	Wait     *Wait
	Pace     *event.Pace
	Idle     bool
	IdleRun  int // work.none answers in a row
	History  []event.StageEnd
	CostU    int64
	End      *event.SessionEnd
	Exit     *int
	LastTs   int64
}

func New() *State { return &State{} }

func (s *State) Apply(e event.Event) {
	if e.Ts > s.LastTs {
		s.LastTs = e.Ts
	}
	switch b := e.Body.(type) {
	case event.SessionStart:
		s.Session, s.Started = b, true
	case event.Project:
		s.Counts = b.Counts
	case event.Board:
		s.Board, s.HasBoard = b.Tasks, true
	case event.Window:
		s.Window = b
	case event.Pace:
		p := b
		s.Pace = &p
	case event.WaitStart:
		s.Wait = &Wait{WaitStart: b, Started: e.Ts}
	case event.WaitEnd:
		s.Wait = nil
	case event.WorkNone:
		s.Counts, s.Idle = b.Counts, true
		s.IdleRun++
	case event.StageStart:
		s.Stage = &Stage{StageStart: b, StartedAt: e.Ts}
		s.Idle, s.IdleRun, s.Pace = false, 0, nil
	case event.AgentInit:
		if s.Stage != nil {
			s.Stage.AgentModel = b.Model
		}
	case event.AgentTool:
		if s.Stage != nil {
			s.Stage.count(b.Name)
		}
	case event.AgentResult:
		if s.Stage != nil && !b.OK {
			s.Stage.Errors++
		}
	case event.AgentText:
		if s.Stage != nil {
			s.Stage.LastText = b.Text
		}
	case event.StageEnd:
		s.History = append(s.History, b)
		s.CostU = b.RunCostU
		s.Ended, s.Stage = s.Stage, nil
	case event.SessionEnd:
		end := b
		s.End = &end
	case event.SessionExit:
		code := b.Code
		s.Exit = &code
		s.Stage, s.Wait = nil, nil
	}
}

func (st *Stage) count(name string) {
	st.Calls++
	st.LastTool = name
	for i := range st.Tools {
		if st.Tools[i].Name == name {
			st.Tools[i].N++
			return
		}
	}
	st.Tools = append(st.Tools, ToolCount{Name: name, N: 1})
}

// Others are the tasks other workers hold right now.
func (s *State) Others() []event.Task {
	var out []event.Task
	for _, t := range s.Board {
		if t.Claim != nil && t.Claim.Worker != s.Session.Worker && t.Stage != "done" {
			out = append(out, t)
		}
	}
	return out
}

// NeedsYou are the tasks only a person can move.
func (s *State) NeedsYou() []event.Task {
	var out []event.Task
	for _, t := range s.Board {
		if t.NeedsHuman {
			out = append(out, t)
		}
	}
	return out
}

// Blocked are the tasks that hang from one that needs a person.
func (s *State) Blocked() []event.Task {
	var out []event.Task
	for _, t := range s.Board {
		if !t.NeedsHuman && t.BlockedBy != "" {
			out = append(out, t)
		}
	}
	return out
}

// Timeline groups the history by task, in the order the tasks were first seen.
func (s *State) Timeline() []TaskLine {
	var out []TaskLine
	index := map[string]int{}
	for _, h := range s.History {
		i, ok := index[h.Task]
		if !ok {
			i = len(out)
			index[h.Task] = i
			out = append(out, TaskLine{Task: h.Task})
		}
		out[i].Steps = append(out[i].Steps, Step{
			Stage: h.Stage, Outcome: h.Outcome, Class: h.Class,
			Attempt: h.Attempt, DurS: h.DurS, CostU: h.CostU,
		})
	}
	return out
}
