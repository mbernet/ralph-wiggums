package app

import (
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
		cfg: Config{Mode: mode},
		v:   view.New(lipgloss.NewRenderer(&bytes.Buffer{}), false),
		s:   state.New(),
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
	if got := len(strings.Split(m.View(), "\n")); got >= 18 {
		t.Fatalf("the saver is still open: %d lines", got)
	}
	m.Update(key("q"))
	if !exists(stop) {
		t.Fatal("after the saver, q does not stop the loop")
	}
}

func TestWhatHappensUnderTheSaverIsPrintedOnReturn(t *testing.T) {
	m, _, _ := liveModel(t, Live)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 18})
	m.Update(key("z"))
	send(t, m, `{"v":1,"ts":5,"type":"notice","level":"warn","text":"disk-nearly-full"}`)
	if !strings.Contains(strings.Join(m.held, "\n"), "disk-nearly-full") {
		t.Fatal("a notice under the saver is not kept for the return")
	}
	if _, cmd := m.Update(key("x")); cmd == nil || len(m.held) != 0 {
		t.Fatal("the kept lines are not printed on return")
	}
}

func TestTheEndOfTheSessionClosesTheSaver(t *testing.T) {
	m, _, _ := liveModel(t, Live)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 18})
	m.Update(key("z"))
	send(t, m, `{"v":1,"ts":9,"type":"session.exit","code":0}`)
	if m.saver != nil || len(m.held) != 0 || !m.done {
		t.Fatal("the session ended under the saver and the saver stayed")
	}
}

func key(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
