package view

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
)

// The fixtures are real: a dry run of ralph.sh, in a temporary directory, so
// these tests also catch the two sides of the event stream drifting apart.
var (
	runs   = map[string][]event.Event{}
	runsMu sync.Mutex
)

func dryRun(t *testing.T, scenario string) []event.Event {
	t.Helper()
	if testing.Short() {
		t.Skip("runs ralph.sh")
	}
	for _, bin := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not installed", bin)
		}
	}
	runsMu.Lock()
	defer runsMu.Unlock()
	if evs, ok := runs[scenario]; ok {
		return evs
	}
	dir, err := os.MkdirTemp("", "ralph-ui-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	script, _ := filepath.Abs("../../../ralph.sh")
	cmd := exec.Command("bash", script, "--dry-run", "--fast", "--no-pace", "--scenario", scenario, "--tool", "claude", "12")
	cmd.Env = append(os.Environ(), "RALPH_DIR="+dir, "RALPH_UI=plain", "RALPH_MIN_DELAY_S=0")
	_ = cmd.Run() // a dry run ends with the exit code of its scenario
	files, _ := filepath.Glob(filepath.Join(dir, "logs", "*", "events.ndjson"))
	if len(files) != 1 {
		t.Fatalf("expected one event file, found %v", files)
	}
	evs, err := event.ReadAll(files[0])
	if err != nil {
		t.Fatal(err)
	}
	runs[scenario] = evs
	return evs
}

func plainView() *View {
	return New(lipgloss.NewRenderer(&bytes.Buffer{}), false)
}

func fits(t *testing.T, what string, out string, width int) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("%s: a line is %d wide, over %d:\n%q", what, w, width, line)
		}
	}
}

func TestEveryFooterFitsTheTerminal(t *testing.T) {
	v := plainView()
	for _, scenario := range []string{"mixed", "reject"} {
		s := state.New()
		for _, e := range dryRun(t, scenario) {
			s.Apply(e)
			for _, w := range []int{60, 80, 132} {
				o := Options{Width: w, Now: e.Ts, Live: true}
				out := v.Footer(s, o)
				fits(t, scenario+" footer after "+e.Type, out, w)
				if n := strings.Count(out, "\n") + 1; n > 7 {
					t.Errorf("the footer is %d lines tall after %s", n, e.Type)
				}
			}
		}
	}
}

func TestTheFooterShowsTheStageInProgress(t *testing.T) {
	v := plainView()
	s := state.New()
	for _, e := range dryRun(t, "mixed") {
		s.Apply(e)
		if e.Type == "agent.tool" {
			break
		}
	}
	out := v.Footer(s, Options{Width: 100, Now: s.LastTs})
	for _, want := range []string{s.Stage.Task.ID, s.Stage.Stage, "Read"} {
		if !strings.Contains(out, want) {
			t.Errorf("the footer lacks %q:\n%s", want, out)
		}
	}
}

func TestTheFooterCountsDownAWait(t *testing.T) {
	v := plainView()
	s := state.New()
	s.Apply(mustDecode(t, `{"v":1,"ts":1000,"type":"wait.start","kind":"throttle","seconds":300,"until":1300}`))
	out := v.Footer(s, Options{Width: 100, Now: 1060})
	if !strings.Contains(out, "throttle") || !strings.Contains(out, "4m 00s") {
		t.Errorf("the footer does not count down:\n%s", out)
	}
}

func TestTheScrollbackTellsTheWholeRun(t *testing.T) {
	v := plainView()
	var buf bytes.Buffer
	evs := dryRun(t, "mixed")
	Replay(&buf, evs, v, Options{Width: 100, Verbosity: Normal})
	out := buf.String()
	fits(t, "replay", out, 100)
	s := state.New()
	for _, e := range evs {
		s.Apply(e)
	}
	for _, h := range s.History {
		if !strings.Contains(out, h.Task) {
			t.Errorf("the replay never names %s", h.Task)
		}
	}
	if s.End == nil || !strings.Contains(strings.ToLower(out), s.End.State) {
		t.Errorf("the replay does not end with the state of the session %+v:\n%s", s.End, out[max(0, len(out)-1500):])
	}
	// A failed tool call is worth a line; a successful one is not, at this level.
	if !strings.Contains(out, "FAIL src/hooks/useTask.test.ts") {
		t.Errorf("the error of a tool is missing")
	}
	if strings.Contains(out, "src/components/TaskEditModal.tsx") {
		t.Errorf("tool calls are shown at the normal level")
	}
}

func TestTheVerboseScrollbackShowsEveryToolCall(t *testing.T) {
	v := plainView()
	var buf bytes.Buffer
	Replay(&buf, dryRun(t, "mixed"), v, Options{Width: 100, Verbosity: Verbose})
	if !strings.Contains(buf.String(), "src/components/TaskEditModal.tsx") {
		t.Error("the tool calls are missing")
	}
}

func TestTheQuietScrollbackKeepsToTheCards(t *testing.T) {
	v := plainView()
	var buf bytes.Buffer
	Replay(&buf, dryRun(t, "mixed"), v, Options{Width: 100, Verbosity: Quiet})
	if strings.Contains(buf.String(), "The fixture needs the new default") {
		t.Error("what the agent says is shown when quiet")
	}
}

func TestAStageCardCarriesWhatAPersonMustRead(t *testing.T) {
	v := plainView()
	s := state.New()
	s.Apply(mustDecode(t, `{"v":1,"ts":1,"type":"session.start","project":"p","worker":"w","pipeline":["implement","review","merge"]}`))
	s.Apply(mustDecode(t, `{"v":1,"ts":2,"type":"stage.start","task":{"id":"US-7","title":"T"},"stage":"implement","pos":1,"count":3,"attempt":1}`))
	end := mustDecode(t, `{"v":1,"ts":9,"type":"stage.end","task":"US-7","stage":"implement","class":"ok","outcome":"pass","next":"review",
		"durS":251,"costU":1870000,"pr":"https://github.com/acme/app/pull/12","assumptions":["Archived tasks show the badge too."]}`)
	s.Apply(end)
	out := strings.Join(v.Scroll(s, end, Options{Width: 90, Verbosity: Normal}), "\n")
	for _, want := range []string{"US-7", "review", "#12", "Archived tasks show the badge too.", "$1.87", "4m 11s"} {
		if !strings.Contains(out, want) {
			t.Errorf("the card lacks %q:\n%s", want, out)
		}
	}

	reject := mustDecode(t, `{"v":1,"ts":20,"type":"stage.end","task":"US-7","stage":"review","class":"ok","outcome":"reject","next":"implement",
		"body":"The empty-filter case has no test."}`)
	s.Apply(reject)
	out = strings.Join(v.Scroll(s, reject, Options{Width: 90, Verbosity: Quiet}), "\n")
	if !strings.Contains(out, "The empty-filter case has no test.") {
		t.Errorf("the feedback of a reject is missing, even when quiet:\n%s", out)
	}
}

func TestTheBoardHasAColumnPerStage(t *testing.T) {
	v := plainView()
	s := state.New()
	for _, e := range dryRun(t, "reject") {
		s.Apply(e)
	}
	for _, w := range []int{60, 100, 160} {
		out := v.Board(s, Options{Width: w})
		fits(t, "board", out, w)
		for _, task := range s.Board {
			if !strings.Contains(out, task.ID) {
				t.Errorf("width %d: the board lacks %s", w, task.ID)
			}
		}
	}
	wide := strings.ToLower(v.Board(s, Options{Width: 160}))
	for _, stage := range append(s.Session.Pipeline, "done") {
		if !strings.Contains(wide, strings.ToLower(stage)) {
			t.Errorf("no column for %s", stage)
		}
	}
}

func TestTheBoardPointsAtWhatNeedsAPerson(t *testing.T) {
	v := plainView()
	s := state.New()
	s.Apply(mustDecode(t, `{"v":1,"ts":1,"type":"session.start","project":"p","worker":"w","pipeline":["implement","merge"]}`))
	s.Apply(mustDecode(t, `{"v":1,"ts":2,"type":"board","tasks":[
		{"id":"US-3","title":"Broken","stage":"implement","runs":4,"sick":true,"needsHuman":true,"why":"The database is not reachable."},
		{"id":"US-4","title":"After","stage":"implement","blockedBy":"US-3"}]}`))
	out := v.Board(s, Options{Width: 100})
	for _, want := range []string{"The database is not reachable.", "--story US-3", "US-3"} {
		if !strings.Contains(out, want) {
			t.Errorf("the board lacks %q:\n%s", want, out)
		}
	}
}

func mustDecode(t *testing.T, line string) event.Event {
	t.Helper()
	e, err := event.Decode([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestTruncLeftKeepsTheEndWithinTheWidth(t *testing.T) {
	for _, s := range []string{
		strings.Repeat("漢", 40), // wide runes: width 80
		"src/components/a/very/long/path/to/TaskEditModal.tsx",
		"short",
	} {
		for _, w := range []int{1, 5, 20, 60} {
			got := truncLeft(s, w)
			if width(got) > w {
				t.Errorf("truncLeft(%q, %d) = %q, %d wide", s, w, got, width(got))
			}
		}
	}
	if got := truncLeft("src/a/b/TaskEditModal.tsx", 18); !strings.HasSuffix(got, "TaskEditModal.tsx") {
		t.Errorf("the end of a path is lost: %q", got)
	}
}

func TestAStageThePipelineDidNotHaveDoesNotBreakTheFooter(t *testing.T) {
	v := plainView()
	s := state.New()
	s.Apply(mustDecode(t, `{"v":1,"ts":1,"type":"session.start","project":"p","worker":"w","pipeline":["implement","review_functional"]}`))
	// The project gained a third stage while this worker ran.
	s.Apply(mustDecode(t, `{"v":1,"ts":2,"type":"stage.start","task":{"id":"US-9"},"stage":"security","pos":3,"count":3,"attempt":1}`))
	for _, w := range []int{20, 40, 100} {
		fits(t, "footer", v.Footer(s, Options{Width: w, Now: 3}), w)
	}
}
