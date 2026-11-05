package app

import (
	"bytes"
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
