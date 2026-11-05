// Package app is the interactive ralph-ui: a Bubble Tea program in inline
// mode. What happens goes up the scrollback, as plain output does, and a panel
// of a few lines stays at the bottom, redrawn as the run goes. It never talks
// to the API and never runs the loop: it reads the events ralph.sh writes and,
// in live mode, asks the loop to stop or to skip a wait through two files.
package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
	"github.com/mbernet/ralph-wiggums/ui/internal/view"
)

type Mode int

const (
	Live  Mode = iota // started by ralph.sh, owns the terminal, can act
	Watch             // follows a run from another terminal, read-only
)

type Config struct {
	Events    string
	Mode      Mode
	PID       int // the ralph.sh to watch over in live mode
	Verbosity view.Verbosity
	ASCII     bool
	Links     bool
	Notify    bool
	Renderer  *lipgloss.Renderer
	MaxWidth  int
}

type evMsg struct {
	e  event.Event
	ok bool
}

type tickMsg time.Time

type model struct {
	cfg      Config
	v        *view.View
	s        *state.State
	ch       <-chan event.Event
	width    int
	verb     view.Verbosity
	stopping bool
	help     bool
	spin     int
	done     bool
	gone     bool
}

const tickEvery = 120 * time.Millisecond

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func next(ch <-chan event.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		return evMsg{e: e, ok: ok}
	}
}

func (m *model) Init() tea.Cmd { return tea.Batch(next(m.ch), tick()) }

func (m *model) opts() view.Options {
	w := m.width
	if w <= 0 {
		w = 100
	}
	if m.cfg.MaxWidth > 0 && w > m.cfg.MaxWidth {
		w = m.cfg.MaxWidth
	}
	o := view.Options{
		Width: w, Now: time.Now().Unix(), Verbosity: m.verb,
		Live: m.cfg.Mode == Live, Links: m.cfg.Links, Notify: m.cfg.Notify,
		Stopping: m.stopping, Help: m.help, Spin: m.spin,
	}
	if m.cfg.Mode == Watch {
		o.Watching = m.s.Session.Run
		if o.Watching == "" {
			o.Watching = "a run"
		}
	}
	return o
}

func printLines(lines []string) tea.Cmd {
	if len(lines) == 0 {
		return nil
	}
	return tea.Println(strings.Join(lines, "\n"))
}

func (m *model) finish(lines ...string) tea.Cmd {
	m.done = true
	return tea.Sequence(printLines(lines), tea.Quit)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if debug != nil {
		fmt.Fprintf(debug, "%s %T %v\n", time.Now().Format("15:04:05.000"), msg, msg)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tickMsg:
		m.spin++
		if m.cfg.Mode == Live && m.cfg.PID > 0 && m.spin%8 == 0 && !alive(m.cfg.PID) && m.s.Exit == nil {
			// Killed without a chance to say so. Give the last events a moment.
			if m.gone {
				return m, m.finish("", "ralph.sh is gone without closing its session")
			}
			m.gone = true
		}
		return m, tick()

	case evMsg:
		if !msg.ok {
			return m, m.finish()
		}
		m.s.Apply(msg.e)
		print := printLines(m.v.Scroll(m.s, msg.e, m.opts()))
		if msg.e.Type == "session.exit" {
			m.done = true
			return m, tea.Sequence(print, tea.Quit)
		}
		return m, tea.Sequence(print, tea.SetWindowTitle(title(m.s)), next(m.ch))

	case tea.KeyMsg:
		return m, m.key(msg.String())
	}
	return m, nil
}

func (m *model) key(k string) tea.Cmd {
	live := m.cfg.Mode == Live
	ctl := m.s.Session.Control
	switch k {
	case "q":
		if !live {
			return m.finish()
		}
		if ctl.Stop == "" {
			return nil
		}
		if m.stopping {
			_ = os.Remove(ctl.Stop)
		} else {
			_ = os.WriteFile(ctl.Stop, nil, 0o644)
		}
		m.stopping = !m.stopping
	case "s":
		if live && ctl.Skip != "" && m.s.Wait != nil {
			_ = os.WriteFile(ctl.Skip, nil, 0o644)
		}
	case "v":
		m.verb = (m.verb + 1) % 3
		return tea.Println(m.v.Notice("detail: " + m.verb.String()))
	case "b":
		return tea.Println(m.v.Board(m.s, m.opts()))
	case "?":
		m.help = !m.help
	case "ctrl+c":
		if !live {
			return m.finish()
		}
		// What Ctrl-C does to a plain run: the loop and its agent get SIGINT.
		// ralph.sh decides; twice in a row aborts it, and this ends with it.
		interrupt(m.cfg.PID)
	}
	return nil
}

func (m *model) View() string {
	if m.done {
		return ""
	}
	return m.v.Footer(m.s, m.opts())
}

func title(s *state.State) string {
	parts := []string{"ralph"}
	if s.Session.Project != "" {
		parts = append(parts, s.Session.Project)
	}
	if s.Counts.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", s.Counts.Done, s.Counts.Total))
	}
	switch {
	case s.Stage != nil:
		parts = append(parts, s.Stage.Task.ID+" "+s.Stage.Stage)
	case s.Wait != nil:
		parts = append(parts, s.Wait.Kind)
	case s.End != nil:
		parts = append(parts, s.End.State)
	}
	return strings.Join(parts, " · ")
}

func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func interrupt(pid int) {
	if pid <= 0 {
		return
	}
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid > 0 {
		_ = syscall.Kill(-pgid, syscall.SIGINT)
		return
	}
	_ = syscall.Kill(pid, syscall.SIGINT)
}

// debug, when RALPH_UI_DEBUG names a file, gets every message the program sees.
var debug *os.File

// Run follows the event file until the session exits, or the user quits.
func Run(ctx context.Context, cfg Config) error {
	if p := os.Getenv("RALPH_UI_DEBUG"); p != "" {
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			debug = f
			defer f.Close()
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := &model{
		cfg:  cfg,
		v:    view.New(cfg.Renderer, cfg.ASCII),
		s:    state.New(),
		ch:   event.Follow(ctx, cfg.Events, 100*time.Millisecond),
		verb: cfg.Verbosity,
	}
	// The terminal is opened by name: on macOS the reader Bubble Tea uses sees
	// no keys from a tty that arrived as a redirected stdin.
	opts := []tea.ProgramOption{tea.WithContext(ctx), tea.WithInputTTY()}
	if cfg.Mode == Live {
		// Ctrl-C reaches the program as a key, and is passed on to ralph.sh.
		opts = append(opts, tea.WithoutSignalHandler())
	}
	_, err := tea.NewProgram(m, opts...).Run()
	if err == tea.ErrProgramKilled && ctx.Err() != nil {
		return nil
	}
	return err
}
