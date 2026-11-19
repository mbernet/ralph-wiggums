package app

import (
	"fmt"
	"strings"

	"bytes"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
	"github.com/mbernet/ralph-wiggums/ui/internal/view"
)

func liveModel(t *testing.T, mode Mode) (*model, string, string) {
	t.Helper()
	dir := t.TempDir()
	stop, skip := filepath.Join(dir, "stop"), filepath.Join(dir, "skip")
	m := &model{
		cfg:  Config{Mode: mode},
		v:    view.New(lipgloss.NewRenderer(&bytes.Buffer{}), false),
		s:    state.New(),
		body: &view.Body{},
		verb: view.Normal,
	}
	e, err := event.Decode([]byte(`{"v":1,"ts":1,"type":"session.start","project":"p","worker":"w",
		"control":{"stop":"` + stop + `","skip":"` + skip + `"}}`))
	if err != nil {
		t.Fatal(err)
	}
	m.s.Apply(e)
	return m, stop, skip
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestQAsksTheLoopToStopAndAgainTakesItBack(t *testing.T) {
	m, stop, _ := liveModel(t, Live)
	m.key("q")
	if !exists(stop) || !m.stopping {
		t.Fatal("q does not ask for a stop")
	}
	m.key("q")
	if exists(stop) || m.stopping {
		t.Fatal("a second q does not take the stop back")
	}
}

func TestSSkipsOnlyAWait(t *testing.T) {
	m, _, skip := liveModel(t, Live)
	m.key("s")
	if exists(skip) {
		t.Fatal("s asks for a skip with nothing to skip")
	}
	e, _ := event.Decode([]byte(`{"v":1,"ts":2,"type":"wait.start","kind":"throttle","seconds":60,"until":62}`))
	m.s.Apply(e)
	m.key("s")
	if !exists(skip) {
		t.Fatal("s does not skip the wait")
	}
}

func TestAWatcherCannotAct(t *testing.T) {
	m, stop, _ := liveModel(t, Watch)
	m.key("q")
	if exists(stop) {
		t.Fatal("a watcher asked the loop to stop")
	}
	if !m.done {
		t.Fatal("q does not quit a watcher")
	}
}

func send(t *testing.T, m *model, line string) {
	t.Helper()
	e, err := event.Decode([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	m.Update(evMsg{e: e, ok: true})
}

func TestZOpensTheSaverOnTheWholeScreenAndAnyKeyCloses(t *testing.T) {
	m, stop, _ := liveModel(t, Live)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 18})
	m.Update(key("z"))
	if got := len(strings.Split(m.View(), "\n")); got != 18 {
		t.Fatalf("the saver draws %d lines on a screen of 18", got)
	}
	m.Update(key("q"))
	if exists(stop) {
		t.Fatal("the key that closes the saver also asked the loop to stop")
	}
	if m.saver != nil {
		t.Fatal("the saver is still open")
	}
	m.Update(key("q"))
	if !exists(stop) {
		t.Fatal("after the saver, q does not stop the loop")
	}
}

func TestWhatHappensUnderTheSaverIsThereOnReturn(t *testing.T) {
	m, _, _ := liveModel(t, Live)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 18})
	m.Update(key("z"))
	send(t, m, `{"v":1,"ts":5,"type":"notice","level":"warn","text":"disk-nearly-full"}`)
	m.Update(key("x"))
	if !strings.Contains(m.View(), "disk-nearly-full") {
		t.Fatalf("a notice under the saver is not on the screen after it:\n%s", m.View())
	}
}

func TestTheEndOfTheSessionLeavesTheReportForTheTerminal(t *testing.T) {
	m, _, _ := liveModel(t, Live)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 18})
	m.Update(key("z"))
	send(t, m, `{"v":1,"ts":8,"type":"session.end","state":"complete","ran":3}`)
	send(t, m, `{"v":1,"ts":9,"type":"session.exit","code":0}`)
	if m.saver != nil || !m.done {
		t.Fatal("the session ended and the screen stayed")
	}
	if !strings.Contains(strings.ToLower(m.after), "complete") {
		t.Fatalf("the report is not left to print after the screen:\n%s", m.after)
	}
}

func TestTheBodyScrollsBackAndEndFollowsAgain(t *testing.T) {
	m, _, _ := liveModel(t, Live)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 14})
	send(t, m, `{"v":1,"ts":2,"type":"stage.start","task":{"id":"US-1","title":"First"},"stage":"implement","pos":1,"count":1,"attempt":1}`)
	for i := 0; i < 30; i++ {
		send(t, m, fmt.Sprintf(`{"v":1,"ts":3,"type":"agent.tool","name":"Read","arg":"f%02d.go"}`, i))
	}
	if strings.Contains(m.View(), "US-1") {
		t.Fatalf("the body does not follow the newest messages:\n%s", m.View())
	}
	for i := 0; i < 10; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	}
	back := m.View()
	if !strings.Contains(back, "US-1") {
		t.Fatalf("PgUp does not go back to the start:\n%s", back)
	}
	send(t, m, `{"v":1,"ts":4,"type":"agent.tool","name":"Read","arg":"newest.go"}`)
	if strings.Contains(m.View(), "newest.go") || !strings.Contains(m.View(), "US-1") {
		t.Fatalf("a new message moves a body scrolled back:\n%s", m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !strings.Contains(m.View(), "newest.go") {
		t.Fatalf("End does not follow again:\n%s", m.View())
	}
}

func TestTheBoardOpensInPlaceAndQClosesIt(t *testing.T) {
	m, stop, _ := liveModel(t, Live)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	send(t, m, `{"v":1,"ts":2,"type":"board","tasks":[{"id":"US-42","title":"Board card","stage":"implement"}]}`)
	m.Update(key("b"))
	if !strings.Contains(m.View(), "US-42") {
		t.Fatalf("b does not show the board:\n%s", m.View())
	}
	m.Update(key("q"))
	if exists(stop) || strings.Contains(m.View(), "US-42") {
		t.Fatal("q on the board does not just close it")
	}
}

func key(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func TestOOpensThePROfTheTask(t *testing.T) {
	m, _, _ := liveModel(t, Live)
	var opened string
	m.open = func(url string) error { opened = url; return nil }
	m.key("o")
	if opened != "" {
		t.Fatalf("o opened %q with no PR", opened)
	}
	send(t, m, `{"v":1,"ts":2,"type":"stage.start","task":{"id":"US-7","title":"T","pr":"https://github.com/acme/app/pull/12"},"stage":"review","pos":2,"count":2,"attempt":1}`)
	if cmd := m.key("o"); cmd != nil {
		cmd()
	}
	if opened != "https://github.com/acme/app/pull/12" {
		t.Fatalf("o opened %q", opened)
	}
}
