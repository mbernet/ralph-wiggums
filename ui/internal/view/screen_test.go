package view

import (
	"strings"
	"testing"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
)

// feed applies the events to a fresh state and body, as the app does, and
// stops after the first event the predicate accepts.
func feed(t *testing.T, evs []event.Event, o Options, until func(event.Event) bool) (*state.State, *Body) {
	t.Helper()
	s, b := state.New(), &Body{}
	for _, e := range evs {
		s.Apply(e)
		o.Now = e.Ts
		b.Add(plainView(), s, e, o)
		if until != nil && until(e) {
			break
		}
	}
	return s, b
}

func TestEveryScreenFillsTheTerminalExactly(t *testing.T) {
	v := plainView()
	for _, scenario := range []string{"mixed", "reject"} {
		s, b := state.New(), &Body{}
		for _, e := range dryRun(t, scenario) {
			s.Apply(e)
			b.Add(v, s, e, Options{Width: 100, Now: e.Ts, Verbosity: Normal})
			for _, size := range [][2]int{{60, 14}, {80, 24}, {132, 40}} {
				o := Options{Width: size[0], Height: size[1], Now: e.Ts, Live: true, Verbosity: Normal}
				out := v.Screen(s, b, o)
				fits(t, scenario+" screen after "+e.Type, out, size[0])
				if n := strings.Count(out, "\n") + 1; n != size[1] {
					t.Errorf("%s: the screen is %d lines on a terminal of %d", e.Type, n, size[1])
				}
			}
		}
	}
}

func TestTheScreenShowsTheStageInProgressWithItsMessages(t *testing.T) {
	v := plainView()
	o := Options{Width: 100, Height: 30, Verbosity: Normal}
	s, b := feed(t, dryRun(t, "mixed"), o, func(e event.Event) bool {
		tool, ok := e.Body.(event.AgentTool)
		return ok && tool.Name == "Edit"
	})
	o.Now = s.LastTs
	out := v.Screen(s, b, o)
	for _, want := range []string{s.Session.Project, s.Stage.Task.ID, s.Stage.Task.Title, s.Stage.Stage,
		"src/components/TaskEditModal.tsx"} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen lacks %q:\n%s", want, out)
		}
	}
}

func TestAFinishedStageFoldsIntoItsCard(t *testing.T) {
	v := plainView()
	o := Options{Width: 100, Height: 40, Verbosity: Normal}
	s, b := state.New(), &Body{}
	for _, line := range []string{
		`{"v":1,"ts":1,"type":"session.start","project":"p","worker":"w","pipeline":["implement","review"]}`,
		`{"v":1,"ts":2,"type":"stage.start","task":{"id":"US-7","title":"Filters"},"stage":"review","pos":2,"count":2,"attempt":1}`,
		`{"v":1,"ts":3,"type":"agent.tool","id":"t1","name":"Read","arg":"src/very/particular/file.ts"}`,
		`{"v":1,"ts":9,"type":"stage.end","task":"US-7","stage":"review","class":"ok","outcome":"reject","next":"implement","durS":7,"body":"The empty-filter case has no test."}`,
	} {
		e := mustDecode(t, line)
		s.Apply(e)
		b.Add(v, s, e, o)
	}
	out := v.Screen(s, b, o)
	if strings.Contains(out, "src/very/particular/file.ts") {
		t.Errorf("the messages of a finished stage are still shown:\n%s", out)
	}
	for _, want := range []string{"US-7", "implement", "The empty-filter case has no test."} {
		if !strings.Contains(out, want) {
			t.Errorf("the card lacks %q:\n%s", want, out)
		}
	}
}

func TestTheBodyFollowsTheEndAndScrollsBack(t *testing.T) {
	v := plainView()
	o := Options{Width: 80, Height: 14, Verbosity: Normal}
	s, b := state.New(), &Body{}
	for _, line := range []string{
		`{"v":1,"ts":1,"type":"session.start","project":"p","worker":"w","pipeline":["implement"]}`,
		`{"v":1,"ts":2,"type":"stage.start","task":{"id":"US-1","title":"First"},"stage":"implement","pos":1,"count":1,"attempt":1}`,
	} {
		e := mustDecode(t, line)
		s.Apply(e)
		b.Add(v, s, e, o)
	}
	for i := 0; i < 40; i++ {
		e := mustDecode(t, `{"v":1,"ts":3,"type":"agent.tool","id":"x","name":"Read","arg":"file-`+string(rune('A'+i%26))+string(rune('a'+i/26))+`.go"}`)
		s.Apply(e)
		b.Add(v, s, e, o)
	}
	if out := v.Screen(s, b, o); !strings.Contains(out, "file-Nb.go") || strings.Contains(out, "file-Aa.go") {
		t.Errorf("the body does not follow the newest message:\n%s", out)
	}
	o.Scroll = 1000
	if out := v.Screen(s, b, o); !strings.Contains(out, "US-1") || !strings.Contains(out, "file-Aa.go") {
		t.Errorf("scrolled back, the body does not show the start:\n%s", out)
	}
}

func TestTheStatusCountsDownAWait(t *testing.T) {
	v := plainView()
	s := state.New()
	s.Apply(mustDecode(t, `{"v":1,"ts":1000,"type":"wait.start","kind":"throttle","seconds":300,"until":1300}`))
	out := v.Screen(s, &Body{}, Options{Width: 100, Height: 20, Now: 1060})
	if !strings.Contains(out, "throttle") || !strings.Contains(out, "4m 00s") {
		t.Errorf("the status does not count down:\n%s", out)
	}
}

func TestEachKindOfToolHasItsIcon(t *testing.T) {
	v := plainView()
	g := v.t.G
	icons := map[string]string{"Read": g.Read, "Grep": g.Read, "Edit": g.Edit, "Write": g.Edit, "Bash": g.Run, "Task": g.Tool}
	seen := map[string]bool{}
	for name, want := range icons {
		if got := v.toolIcon(name); got != want {
			t.Errorf("%s: icon %q, want %q", name, got, want)
		}
		seen[want] = true
	}
	if len(seen) != 4 {
		t.Errorf("reads, edits, commands and the rest do not have four different icons")
	}
}

func TestTheKeysSitApartFromTheState(t *testing.T) {
	v := plainView()
	g := v.t.G
	s := state.New()
	s.Apply(mustDecode(t, `{"v":1,"ts":1,"type":"session.start","project":"p","worker":"w","pipeline":["implement"]}`))
	s.Apply(mustDecode(t, `{"v":1,"ts":2,"type":"stage.start","task":{"id":"US-1","title":"T"},"stage":"implement","pos":1,"count":1,"attempt":1}`))
	for _, h := range []int{8, 24} {
		lines := strings.Split(v.Screen(s, &Body{}, Options{Width: 80, Height: h, Now: 3, Live: true}), "\n")
		keys := lines[len(lines)-1]
		if strings.HasPrefix(keys, g.Bar) || strings.HasPrefix(keys, g.Corners[2]) {
			t.Errorf("height %d: the keys are inside the status box: %q", h, keys)
		}
		if !strings.Contains(keys, "z") || strings.Contains(keys, "implement") {
			t.Errorf("height %d: the last line is not the keys alone: %q", h, keys)
		}
		state := lines[len(lines)-3] // the state, the meters, the keys
		if h >= 12 {
			state = lines[len(lines)-4] // and the bottom of the box
		}
		if !strings.Contains(state, "implement") {
			t.Errorf("height %d: the state is not right above the keys: %q", h, state)
		}
	}
}

func TestThePROfTheTaskIsAlwaysInSightAndALink(t *testing.T) {
	v := plainView()
	url := "https://github.com/acme/app/pull/12"
	s, b := state.New(), &Body{}
	o := Options{Width: 100, Height: 24, Verbosity: Normal, Links: true}
	for _, line := range []string{
		`{"v":1,"ts":1,"type":"session.start","project":"p","worker":"w","pipeline":["implement","review"]}`,
		`{"v":1,"ts":2,"type":"stage.start","task":{"id":"US-7","title":"T","pr":"` + url + `"},"stage":"review","pos":2,"count":2,"attempt":1}`,
	} {
		e := mustDecode(t, line)
		s.Apply(e)
		b.Add(v, s, e, o)
	}
	for i := 0; i < 40; i++ { // enough to push the stage header out of the body
		e := mustDecode(t, `{"v":1,"ts":3,"type":"agent.tool","name":"Read","arg":"f.go"}`)
		s.Apply(e)
		b.Add(v, s, e, o)
	}
	lines := strings.Split(v.Screen(s, b, o), "\n")
	status := strings.Join(lines[len(lines)-5:], "\n")
	if !strings.Contains(status, "#12") || !strings.Contains(status, url) {
		t.Errorf("the status lacks the PR as a link:\n%q", status)
	}
}

func TestEveryCardOfATaskCarriesItsPR(t *testing.T) {
	v := plainView()
	s, b := state.New(), &Body{}
	o := Options{Width: 100, Height: 30, Verbosity: Normal}
	for _, line := range []string{
		`{"v":1,"ts":1,"type":"session.start","project":"p","worker":"w","pipeline":["implement","review"]}`,
		`{"v":1,"ts":2,"type":"stage.start","task":{"id":"US-7","title":"T"},"stage":"implement","pos":1,"count":2,"attempt":1}`,
		`{"v":1,"ts":3,"type":"stage.end","task":"US-7","stage":"implement","class":"ok","outcome":"pass","next":"review","pr":"https://github.com/acme/app/pull/12"}`,
		`{"v":1,"ts":4,"type":"stage.start","task":{"id":"US-7","title":"T","pr":"https://github.com/acme/app/pull/12"},"stage":"review","pos":2,"count":2,"attempt":1}`,
		`{"v":1,"ts":5,"type":"stage.end","task":"US-7","stage":"review","class":"ok","outcome":"pass","next":"done","pr":"https://github.com/acme/app/pull/12"}`,
	} {
		e := mustDecode(t, line)
		s.Apply(e)
		b.Add(v, s, e, o)
	}
	if n := strings.Count(strings.Join(b.done, "\n"), "#12"); n != 2 {
		t.Errorf("%d cards of 2 carry the PR:\n%s", n, strings.Join(b.done, "\n"))
	}
}
