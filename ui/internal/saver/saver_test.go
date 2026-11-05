package saver

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
)

var t0 = time.Unix(1_800_000_000, 0)

func TestFrameFillsTheWholeScreen(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {1, 1}, {3, 2}, {200, 60}, {37, 5}} {
		w, h := sz[0], sz[1]
		s := New(termenv.TrueColor, 1)
		lines := strings.Split(s.Frame(state.New(), w, h, t0), "\n")
		if len(lines) != h {
			t.Fatalf("%dx%d: %d lines", w, h, len(lines))
		}
		for i, l := range lines {
			if got := ansi.StringWidth(l); got != w {
				t.Fatalf("%dx%d: line %d is %d wide", w, h, i, got)
			}
		}
	}
}

func run(t *testing.T, lines ...string) *state.State {
	t.Helper()
	s := state.New()
	for _, l := range lines {
		e, err := event.Decode([]byte(l))
		if err != nil {
			t.Fatal(err)
		}
		s.Apply(e)
	}
	return s
}

func lastLine(frame string) string {
	lines := strings.Split(frame, "\n")
	return ansi.Strip(lines[len(lines)-1])
}

func TestTheStatusLineSaysWhereTheRunIs(t *testing.T) {
	s := run(t,
		`{"v":1,"ts":1,"type":"session.start","project":"myapp","worker":"w"}`,
		`{"v":1,"ts":2,"type":"stage.end","task":"US-001","stage":"implement","outcome":"pass","runCostU":1230000}`,
		`{"v":1,"ts":3,"type":"stage.start","task":{"id":"US-002","title":"Badge"},"stage":"review_code","pos":3,"count":4}`,
	)
	line := lastLine(New(termenv.TrueColor, 1).Frame(s, 120, 10, t0))
	for _, want := range []string{"myapp", "US-002", "review_code", "$1.23"} {
		if !strings.Contains(line, want) {
			t.Fatalf("%q is not in the status line %q", want, line)
		}
	}

	s = run(t, `{"v":1,"ts":1,"type":"wait.start","kind":"throttle","seconds":600,"until":`+
		fmt.Sprint(t0.Unix()+125)+`}`)
	if line := lastLine(New(termenv.TrueColor, 1).Frame(s, 120, 10, t0)); !strings.Contains(line, "throttle") || !strings.Contains(line, "2m 05s") {
		t.Fatalf("a wait is not in the status line %q", line)
	}
}

func ev(t *testing.T, line string) event.Event {
	t.Helper()
	e, err := event.Decode([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// frames draws a second of animation, feeding the events half way.
func frames(t *testing.T, events ...string) string {
	s, st := New(termenv.TrueColor, 1), state.New()
	var out string
	for i := 0; i <= 10; i++ {
		if i == 5 {
			for _, l := range events {
				s.Feed(ev(t, l))
			}
		}
		out = s.Frame(st, 60, 20, t0.Add(time.Duration(i)*100*time.Millisecond))
	}
	return out
}

func TestTheArtMovesWithTime(t *testing.T) {
	s, st := New(termenv.TrueColor, 1), state.New()
	if s.Frame(st, 60, 20, t0) == s.Frame(st, 60, 20, t0.Add(time.Second)) {
		t.Fatal("two frames a second apart are the same")
	}
}

func TestTheAgentsActivityChangesTheArt(t *testing.T) {
	calm := frames(t)
	tool := `{"v":1,"ts":1,"type":"agent.tool","id":"t1","name":"Edit","arg":"a.go"}`
	if frames(t, tool, tool, tool) == calm {
		t.Fatal("tool calls leave the art as it was")
	}
}

func TestTheEndOfAStageShowsItsOutcome(t *testing.T) {
	end := func(outcome string) string {
		return `{"v":1,"ts":1,"type":"stage.end","task":"US-1","stage":"review","outcome":"` + outcome + `"}`
	}
	calm, pass, reject := frames(t), frames(t, end("pass")), frames(t, end("reject"))
	if pass == calm || reject == calm || pass == reject {
		t.Fatal("a pass and a reject do not look different, or not different from nothing")
	}
}

func TestWithoutColourItDrawsWithCharacters(t *testing.T) {
	frame := New(termenv.Ascii, 1).Frame(state.New(), 60, 20, t0)
	if strings.Contains(frame, "\x1b") {
		t.Fatal("escape sequences on a terminal with no colour")
	}
	art := strings.Join(strings.Split(frame, "\n")[:19], "")
	if strings.Count(art, " ") > len([]rune(art))/2 {
		t.Fatal("the art is mostly blank")
	}
	for i, l := range strings.Split(frame, "\n") {
		if ansi.StringWidth(l) != 60 {
			t.Fatalf("line %d is %d wide", i, ansi.StringWidth(l))
		}
	}
}

// BenchmarkFrame is a large screen at work. bytes/frame is what goes to the
// terminal on each frame, as the cost of the saver is mostly there.
func BenchmarkFrame(b *testing.B) {
	st := state.New()
	st.Stage = &state.Stage{}
	s := New(termenv.TrueColor, 1)
	n := 0
	for i := 0; i < b.N; i++ {
		n += len(s.Frame(st, 220, 60, t0.Add(time.Duration(i)*66*time.Millisecond)))
	}
	b.ReportMetric(float64(n/b.N), "bytes/frame")
}
