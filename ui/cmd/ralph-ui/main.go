// Command ralph-ui renders a ralph.sh run from its event stream.
//
//	ralph-ui live   --events <file> [--pid <n>]   what ralph.sh starts on a terminal
//	ralph-ui watch  [<events file | run dir | logs dir>]
//	ralph-ui board  --events <file>               the last board of a stream, once
//	ralph-ui replay <events file>                 the scrollback of a run, no terminal needed
//
// ralph.sh is the only one that talks to the Ralph API; this program only reads.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"

	"github.com/mbernet/ralph-wiggums/ui/internal/app"
	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
	"github.com/mbernet/ralph-wiggums/ui/internal/view"
)

const usage = `ralph-ui - renders a ralph.sh run from its event stream

  ralph-ui live   --events <file> [--pid <n>]
  ralph-ui watch  [<events file | run dir | logs dir>]   default: the newest run in .ralph/logs
  ralph-ui board  --events <file>
  ralph-ui replay <events file>

  Options
    --detail quiet|normal|verbose   how much of the agent goes up the scrollback
    --ascii                          no Unicode glyphs
    --width <n>                      replay and board: the width to draw at

  Environment
    NO_COLOR      no colours
    RALPH_ASCII   1 = no Unicode glyphs
    RALPH_LINKS   0 = no hyperlinks
    RALPH_NOTIFY  1 = desktop notifications (OSC 9), on by default in iTerm2, WezTerm and Ghostty
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "live":
		err = runLive(os.Args[2:])
	case "watch":
		err = runWatch(os.Args[2:])
	case "board":
		err = runBoard(os.Args[2:])
	case "replay":
		err = runReplay(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ralph-ui:", err)
		os.Exit(1)
	}
}

type common struct {
	detail string
	ascii  bool
	width  int
}

func flags(name string, c *common) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.detail, "detail", "normal", "")
	fs.BoolVar(&c.ascii, "ascii", os.Getenv("RALPH_ASCII") == "1" || !utf8Locale(), "")
	fs.IntVar(&c.width, "width", 0, "")
	return fs
}

func (c common) verbosity() view.Verbosity {
	switch c.detail {
	case "quiet":
		return view.Quiet
	case "verbose":
		return view.Verbose
	}
	return view.Normal
}

func utf8Locale() bool {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return strings.Contains(strings.ToUpper(v), "UTF")
		}
	}
	return true
}

// renderer is Lip Gloss's own: Bubble Tea asks the terminal for its
// background when it loads, before any program takes the keyboard, and the
// answer is kept there. A renderer of our own would ask a second time.
func renderer() *lipgloss.Renderer {
	r := lipgloss.DefaultRenderer()
	if os.Getenv("NO_COLOR") != "" {
		r.SetColorProfile(termenv.Ascii)
	}
	switch os.Getenv("RALPH_THEME") {
	case "dark":
		r.SetHasDarkBackground(true)
	case "light":
		r.SetHasDarkBackground(false)
	}
	return r
}

func interactive() bool {
	return term.IsTerminal(os.Stdout.Fd()) && term.IsTerminal(os.Stdin.Fd())
}

func notifyByDefault() bool {
	if v := os.Getenv("RALPH_NOTIFY"); v != "" {
		return v == "1"
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "ghostty":
		return true
	}
	return false
}

func config(c common, events string, mode app.Mode) app.Config {
	return app.Config{
		Events: events, Mode: mode, Verbosity: c.verbosity(), ASCII: c.ascii,
		Links: os.Getenv("RALPH_LINKS") != "0", Notify: notifyByDefault(),
		Renderer: renderer(), MaxWidth: 140,
	}
}

func runLive(args []string) error {
	var c common
	var events string
	var pid int
	fs := flags("live", &c)
	fs.StringVar(&events, "events", "", "")
	fs.IntVar(&pid, "pid", 0, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if events == "" {
		return errors.New("live needs --events")
	}
	if !interactive() {
		return errors.New("live needs a terminal")
	}
	// The terminal's own Ctrl-C would hit the whole process group, this one
	// included. Here it is a key, passed on to ralph.sh; ralph-ui ends with it.
	signal.Ignore(syscall.SIGINT)
	// ralph.sh sends SIGTERM if ralph-ui is slow to end: leave the terminal as
	// it was found.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	cfg := config(c, events, app.Live)
	cfg.PID = pid
	return app.Run(ctx, cfg)
}

func runWatch(args []string) error {
	var c common
	fs := flags("watch", &c)
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := fs.Arg(0)
	if target == "" {
		target = filepath.Join(".ralph", "logs")
	}
	events, err := findEvents(target)
	if err != nil {
		return err
	}
	if !interactive() {
		return runReplay(append(args, events))
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	return app.Run(ctx, config(c, events, app.Watch))
}

// findEvents accepts an event file, a run directory or a logs directory, where
// it takes the newest run.
func findEvents(target string) (string, error) {
	st, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return target, nil
	}
	if f := filepath.Join(target, "events.ndjson"); exists(f) {
		return f, nil
	}
	runs, _ := filepath.Glob(filepath.Join(target, "*", "events.ndjson"))
	if len(runs) == 0 {
		return "", fmt.Errorf("no run with events in %s", target)
	}
	sort.Strings(runs) // run ids are timestamps
	return runs[len(runs)-1], nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func load(path string) (*state.State, []event.Event, error) {
	evs, err := event.ReadAll(path)
	if err != nil {
		return nil, nil, err
	}
	s := state.New()
	for _, e := range evs {
		s.Apply(e)
	}
	return s, evs, nil
}

func drawWidth(c common) int {
	if c.width > 0 {
		return c.width
	}
	if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
		return min(w, 160)
	}
	if w, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && w > 0 {
		return w
	}
	return 100
}

func runBoard(args []string) error {
	var c common
	var events string
	fs := flags("board", &c)
	fs.StringVar(&events, "events", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if events == "" {
		events = fs.Arg(0)
	}
	if events == "" {
		return errors.New("board needs --events")
	}
	s, _, err := load(events)
	if err != nil {
		return err
	}
	if !s.HasBoard {
		return errors.New("the stream holds no board")
	}
	v := view.New(renderer(), c.ascii)
	fmt.Println(v.Board(s, view.Options{Width: drawWidth(c), Links: os.Getenv("RALPH_LINKS") != "0"}))
	return nil
}

func runReplay(args []string) error {
	var c common
	fs := flags("replay", &c)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.Arg(0) == "" {
		return errors.New("replay needs an events file")
	}
	events, err := findEvents(fs.Arg(0))
	if err != nil {
		return err
	}
	evs, err := event.ReadAll(events)
	if err != nil {
		return err
	}
	v := view.New(renderer(), c.ascii)
	view.Replay(os.Stdout, evs, v, view.Options{Width: drawWidth(c), Verbosity: c.verbosity()})
	return nil
}
