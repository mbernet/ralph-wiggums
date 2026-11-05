package state

import (
	"testing"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
)

func ev(t *testing.T, line string) event.Event {
	t.Helper()
	e, err := event.Decode([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func apply(t *testing.T, s *State, lines ...string) {
	t.Helper()
	for _, l := range lines {
		s.Apply(ev(t, l))
	}
}

const start = `{"v":1,"ts":100,"type":"session.start","project":"myapp","worker":"mac","pipeline":["implement","review","merge"]}`

func TestAStageInProgressCountsTheToolsItUses(t *testing.T) {
	s := New()
	apply(t, s, start,
		`{"v":1,"ts":110,"type":"stage.start","iter":1,"attempt":1,"task":{"id":"US-1"},"stage":"implement","pos":1,"count":3}`,
		`{"v":1,"ts":111,"type":"agent.init","model":"opus"}`,
		`{"v":1,"ts":112,"type":"agent.tool","name":"Read","arg":"a.go"}`,
		`{"v":1,"ts":113,"type":"agent.tool","name":"Edit","arg":"a.go"}`,
		`{"v":1,"ts":114,"type":"agent.tool","name":"Read","arg":"b.go"}`,
		`{"v":1,"ts":115,"type":"agent.tool","name":"Bash","arg":"go test"}`,
		`{"v":1,"ts":116,"type":"agent.result","ok":false,"error":"FAIL a_test.go"}`,
	)
	st := s.Stage
	if st == nil || st.Task.ID != "US-1" || st.StartedAt != 110 {
		t.Fatalf("stage: %+v", st)
	}
	got := ""
	for _, tc := range st.Tools {
		got += tc.Name + ":" + string(rune('0'+tc.N)) + " "
	}
	if got != "Read:2 Edit:1 Bash:1 " {
		t.Fatalf("tools in order of first use: %q", got)
	}
	if st.Errors != 1 || st.LastTool != "Bash" || st.AgentModel != "opus" {
		t.Fatalf("errors %d, last tool %q, model %q", st.Errors, st.LastTool, st.AgentModel)
	}
}

func TestAStageEndGoesToTheHistory(t *testing.T) {
	s := New()
	apply(t, s, start,
		`{"v":1,"ts":110,"type":"stage.start","iter":1,"attempt":1,"task":{"id":"US-1"},"stage":"implement","pos":1,"count":3}`,
		`{"v":1,"ts":150,"type":"stage.end","iter":1,"attempt":1,"task":"US-1","stage":"implement","outcome":"pass","next":"review","durS":40,"costU":1500000,"runCostU":1500000}`,
		`{"v":1,"ts":160,"type":"stage.start","iter":2,"attempt":1,"task":{"id":"US-1"},"stage":"review","pos":2,"count":3}`,
		`{"v":1,"ts":190,"type":"stage.end","iter":2,"attempt":1,"task":"US-1","stage":"review","outcome":"reject","next":"implement","durS":30,"costU":500000,"runCostU":2000000}`,
	)
	if s.Stage != nil {
		t.Fatal("no stage is in progress after its end")
	}
	if len(s.History) != 2 || s.CostU != 2000000 {
		t.Fatalf("history %d, cost %d", len(s.History), s.CostU)
	}
	tl := s.Timeline()
	if len(tl) != 1 || tl[0].Task != "US-1" || len(tl[0].Steps) != 2 || tl[0].Steps[1].Outcome != "reject" {
		t.Fatalf("timeline: %+v", tl)
	}
}

func TestAWaitLastsUntilItsEnd(t *testing.T) {
	s := New()
	apply(t, s, start, `{"v":1,"ts":200,"type":"wait.start","kind":"throttle","seconds":60,"until":260}`)
	if s.Wait == nil || s.Wait.Kind != "throttle" || s.Wait.Until != 260 || s.Wait.Started != 200 {
		t.Fatalf("wait: %+v", s.Wait)
	}
	apply(t, s, `{"v":1,"ts":230,"type":"wait.end","kind":"throttle","skipped":true}`)
	if s.Wait != nil {
		t.Fatal("the wait is over")
	}
}

func TestTheBoardSaysWhoElseIsWorkingAndWhatNeedsAPerson(t *testing.T) {
	s := New()
	apply(t, s, start, `{"v":1,"ts":300,"type":"board","tasks":[
		{"id":"US-1","stage":"review","claim":{"worker":"mac","expiresAt":9000}},
		{"id":"US-2","stage":"merge","claim":{"worker":"vps-1","expiresAt":9000}},
		{"id":"US-3","stage":"implement","sick":true,"needsHuman":true,"why":"db down"},
		{"id":"US-4","stage":"implement","blockedBy":"US-3"},
		{"id":"US-5","stage":"done","doneAt":250}]}`)
	others := s.Others()
	if len(others) != 1 || others[0].ID != "US-2" {
		t.Fatalf("others: %+v", others)
	}
	needs := s.NeedsYou()
	if len(needs) != 1 || needs[0].ID != "US-3" {
		t.Fatalf("needs you: %+v", needs)
	}
	if b := s.Blocked(); len(b) != 1 || b[0].ID != "US-4" {
		t.Fatalf("blocked: %+v", b)
	}
}

func TestTheSessionEndsAndExits(t *testing.T) {
	s := New()
	apply(t, s, start,
		`{"v":1,"ts":400,"type":"session.end","state":"complete","ran":4}`,
		`{"v":1,"ts":401,"type":"session.exit","code":0}`)
	if s.End == nil || s.End.State != "complete" || s.Exit == nil || *s.Exit != 0 {
		t.Fatalf("end %+v exit %v", s.End, s.Exit)
	}
}

func TestTheProjectCountsAndTheWindowFollowTheirEvents(t *testing.T) {
	s := New()
	apply(t, s, start,
		`{"v":1,"ts":10,"type":"project","counts":{"total":5,"done":2}}`,
		`{"v":1,"ts":11,"type":"window","src":"config","fiveHour":{"pct":38,"resetsAt":999}}`,
		`{"v":1,"ts":12,"type":"work.none","finished":false,"counts":{"total":5,"done":3}}`)
	if s.Counts.Done != 3 || s.Window.FiveHour.Pct != 38 || !s.Idle {
		t.Fatalf("counts %+v window %+v idle %v", s.Counts, s.Window, s.Idle)
	}
	apply(t, s, `{"v":1,"ts":20,"type":"stage.start","task":{"id":"US-1"},"stage":"implement"}`)
	if s.Idle {
		t.Fatal("not idle once a stage starts")
	}
}
