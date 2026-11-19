// Package app is the interactive ralph-ui: a Bubble Tea program on the whole
// screen. A header names the project, the body holds the stages and what the
// agents say, and a status box at the bottom has the meters and the keys. When
// it ends, the report of the session is left on the terminal. It never talks
// to the API and never runs the loop: it reads the events ralph.sh writes and,
// in live mode, asks the loop to stop or to skip a wait through two files.
package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/saver"
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

// frameMsg asks for the next frame of the saver that was open as gen.
type frameMsg struct{ gen int }

type model struct {
	cfg      Config
	v        *view.View
	s        *state.State
	ch       <-chan event.Event
	width    int
	height   int
	verb     view.Verbosity
	stopping bool
	help     bool
	spin     int
	done     bool
	gone     bool

	body   *view.Body
	scroll int // how many lines the body is scrolled back; 0 follows the end
	board  bool
	after  string // printed on the terminal once the screen is given back
	open   func(url string) error

	// The saver takes the place of the screen while it is open.
	saver  *saver.Saver
	gen    int
	screen string // the last frame; drawn on the saver's clock, not per message
}

const tickEvery = 120 * time.Millisecond

const frameEvery = 66 * time.Millisecond

func frame(gen int) tea.Cmd {
	return tea.Tick(frameEvery, func(time.Time) tea.Msg { return frameMsg{gen} })
}

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
	h := m.height
	if h <= 0 {
		h = 24
	}
	o := view.Options{
		Width: w, Height: h, Scroll: m.scroll, Board: m.board, HasPR: view.PR(m.s) != "", Now: time.Now().Unix(), Verbosity: m.verb,
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

func (m *model) openSaver() tea.Cmd {
	p := termenv.Ascii
	if m.cfg.Renderer != nil {
		p = m.cfg.Renderer.ColorProfile()
	}
	m.saver = saver.New(p, time.Now().UnixNano())
	m.gen++
	m.draw()
	return frame(m.gen)
}

func (m *model) draw() {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		w, h = 80, 24
	}
	m.screen = m.saver.Frame(m.s, w, h, time.Now())
}

func (m *model) closeSaver() {
	m.saver, m.screen = nil, ""
	m.gen++
}

// finish ends the program; what is in after goes on the terminal then.
func (m *model) finish(after ...string) tea.Cmd {
	m.done = true
	m.closeSaver()
	m.after = strings.Join(after, "\n")
	return tea.Quit
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if debug != nil {
		fmt.Fprintf(debug, "%s %T %v\n", time.Now().Format("15:04:05.000"), msg, msg)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.saver != nil {
			m.draw()
		}
		return m, nil

	case frameMsg:
		if m.saver == nil || msg.gen != m.gen {
			return m, nil
		}
		m.draw()
		return m, frame(m.gen)

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
		if m.saver != nil {
			m.saver.Feed(msg.e)
		}
		o := m.opts()
		switch msg.e.Type {
		case "session.end":
			m.after = strings.Join(m.v.Scroll(m.s, msg.e, o), "\n")
		case "session.exit":
			return m, m.finish(append([]string{m.after}, m.v.Scroll(m.s, msg.e, o)...)...)
		}
		// Scrolled back, the body stays where it is as lines come in.
		before := 0
		if m.scroll > 0 {
			before = m.v.MaxScroll(m.s, m.body, o)
		}
		m.body.Add(m.v, m.s, msg.e, o)
		if m.scroll > 0 {
			m.scroll += m.v.MaxScroll(m.s, m.body, o) - before
		}
		return m, tea.Batch(m.notify(msg.e), tea.SetWindowTitle(title(m.s)), next(m.ch))

	case tea.KeyMsg:
		k := msg.String()
		if m.saver != nil {
			// Any key brings the normal view back, and does nothing else.
			// Ctrl-C keeps its meaning.
			m.closeSaver()
			if k != "ctrl+c" {
				return m, nil
			}
			return m, m.key(k)
		}
		return m, m.key(k)
	}
	return m, nil
}

func (m *model) key(k string) tea.Cmd {
	live := m.cfg.Mode == Live
	ctl := m.s.Session.Control
	if m.board || m.help {
		// The board and the help take the body: the keys that open them,
		// q and Esc give it back.
		switch k {
		case "b":
			m.board, m.help = !m.board, false
		case "?":
			m.help, m.board = !m.help, false
		case "q", "esc":
			m.board, m.help = false, false
		case "ctrl+c":
			m.board, m.help = false, false
			return m.key(k)
		}
		return nil
	}
	page := m.height - 10 // the body, less a line kept from the last page
	if page < 1 {
		page = 1
	}
	switch k {
	case "up", "k":
		m.scrollBy(1)
	case "down", "j":
		m.scrollBy(-1)
	case "pgup":
		m.scrollBy(page)
	case "pgdown", " ":
		m.scrollBy(-page)
	case "home", "g":
		m.scrollBy(1 << 30)
	case "end", "G":
		m.scroll = 0
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
	case "o":
		if url := view.PR(m.s); url != "" && m.open != nil {
			open := m.open
			return func() tea.Msg { _ = open(url); return nil }
		}
	case "s":
		if live && ctl.Skip != "" && m.s.Wait != nil {
			_ = os.WriteFile(ctl.Skip, nil, 0o644)
		}
	case "v":
		m.verb = (m.verb + 1) % 3
		m.body.Note(m.v, "detail: "+m.verb.String()+", from the next message on")
	case "b":
		m.board = true
	case "?":
		m.help = !m.help
	case "z":
		return m.openSaver()
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
	if m.saver != nil {
		return m.screen
	}
	return m.v.Screen(m.s, m.body, m.opts())
}

func (m *model) scrollBy(n int) {
	o := m.opts()
	o.Scroll = 0
	m.scroll = max(0, min(m.scroll+n, m.v.MaxScroll(m.s, m.body, o)))
}

// notify sends a desktop notification (OSC 9) when a task falls sick. The
// screen is Bubble Tea's, so it goes straight to the terminal.
func (m *model) notify(e event.Event) tea.Cmd {
	end, ok := e.Body.(event.StageEnd)
	if !m.cfg.Notify || !ok || end.Outcome != "sick" {
		return nil
	}
	return func() tea.Msg {
		fmt.Fprintf(os.Stdout, "\x1b]9;%s is sick\x07", end.Task)
		return nil
	}
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

// openURL hands a URL to the desktop, as a click on it would.
func openURL(url string) error {
	bin := "xdg-open"
	if runtime.GOOS == "darwin" {
		bin = "open"
	}
	return exec.Command(bin, url).Run()
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
		body: &view.Body{},
		open: openURL,
	}
	// The terminal is opened by name: on macOS the reader Bubble Tea uses sees
	// no keys from a tty that arrived as a redirected stdin.
	opts := []tea.ProgramOption{tea.WithContext(ctx), tea.WithInputTTY(), tea.WithAltScreen()}
	if cfg.Mode == Live {
		// Ctrl-C reaches the program as a key, and is passed on to ralph.sh.
		opts = append(opts, tea.WithoutSignalHandler())
	}
	_, err := tea.NewProgram(m, opts...).Run()
	if m.after != "" {
		fmt.Println(m.after)
	}
	if err == tea.ErrProgramKilled && ctx.Err() != nil {
		return nil
	}
	return err
}
