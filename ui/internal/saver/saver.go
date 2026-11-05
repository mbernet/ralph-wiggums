// Package saver is the screensaver: a full-screen plasma that lives off the
// run. The agent's activity is its energy, the stage its palette, the end of a
// stage a ripple of the colour of its outcome. A status line at the bottom says
// where the run is. Frame returns the whole screen as a string for a size and a
// moment, so it can be tested without a terminal.
package saver

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/mbernet/ralph-wiggums/ui/internal/event"
	"github.com/mbernet/ralph-wiggums/ui/internal/state"
	"github.com/mbernet/ralph-wiggums/ui/internal/view"
)

type rgb struct{ r, g, b float64 }

func (c rgb) add(o rgb, k float64) rgb { return rgb{c.r + o.r*k, c.g + o.g*k, c.b + o.b*k} }

func hexRGB(h string) rgb {
	var r, g, b int
	_, _ = fmt.Sscanf(h, "#%02x%02x%02x", &r, &g, &b)
	return rgb{float64(r) / 255, float64(g) / 255, float64(b) / 255}
}

// A pulse is a ring that grows from a point and fades: the end of a stage, or
// the spark of a tool call. Its place is in the plasma's own units.
type pulse struct {
	x, y, age, life, speed, power float64
	col                           rgb
}

// A particle drifts along the flow of the field and leaves a trail behind.
type particle struct{ x, y, life float64 }

type Saver struct {
	p     termenv.Profile
	rng   *rand.Rand
	last  time.Time
	phase float64 // the animation clock; it runs faster with more energy

	energy float64 // the agent's activity, fed by events, fading by itself
	bright float64
	mood   string   // the gradient the run asks for now
	from   gradient // the one on screen as the last change began
	to     gradient
	mix    float64  // how far from from to to
	g      gradient // the blend of this frame
	pulses []pulse

	px, py int
	parts  []particle
	trail  []float64

	fg, bg map[uint32]string // escape sequences, by quantized colour
}

func New(p termenv.Profile, seed int64) *Saver {
	return &Saver{
		p:      p,
		rng:    rand.New(rand.NewSource(seed)),
		bright: 0.6, mix: 1,
		fg: map[uint32]string{}, bg: map[uint32]string{},
	}
}

var outcomeColor = map[string]string{
	"pass": "#34D399", "reject": "#FBBF24", "sick": "#F87171",
	"conflict": "#F472B6", "dropped": "#F472B6",
}

// Feed lets an event touch the art. Every event can go through it; most do
// nothing.
func (sv *Saver) Feed(e event.Event) {
	switch b := e.Body.(type) {
	case event.AgentTool:
		sv.energy += 0.25
		sv.spark(hexRGB("#67E8F9"))
	case event.AgentResult:
		if !b.OK {
			sv.spark(hexRGB("#F87171"))
		}
	case event.AgentText, event.AgentLine, event.AgentThinking:
		sv.energy += 0.06
	case event.StageStart:
		sv.ring(hexRGB("#A78BFA"), 0.5)
	case event.StageEnd:
		c, ok := outcomeColor[b.Outcome]
		if !ok {
			c = "#A1A1AA"
		}
		sv.ring(hexRGB(c), 0.8)
	case event.SessionEnd:
		sv.ring(hexRGB("#FAFAFA"), 0.6)
	}
	sv.energy = math.Min(sv.energy, 1.5)
}

func (sv *Saver) ring(c rgb, power float64) {
	sv.pulses = append(sv.pulses, pulse{life: 3.5, speed: 1.6, power: power, col: c})
}

func (sv *Saver) spark(c rgb) {
	sv.pulses = append(sv.pulses, pulse{
		x: sv.rng.Float64()*5 - 2.5, y: sv.rng.Float64()*3 - 1.5,
		life: 1.2, speed: 1.1, power: 0.4, col: c,
	})
}

// Frame draws the screen: art on every line but the last, the status line on
// the last one. Each line is exactly w cells wide.
func (sv *Saver) Frame(s *state.State, w, h int, now time.Time) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	sv.resize(w, 2*(h-1)) // a half block is two square-ish pixels
	sv.step(s, now)
	lines := make([]string, 0, h)
	for row := 0; row < h-1; row++ {
		lines = append(lines, sv.row(row))
	}
	lines = append(lines, sv.status(s, w, now))
	return strings.Join(lines, "\n")
}

func (sv *Saver) resize(px, py int) {
	if px == sv.px && py == sv.py {
		return
	}
	sv.px, sv.py = px, py
	sv.trail = make([]float64, px*py)
	n := min(max(px*py/30, 8), 500)
	if px*py == 0 {
		n = 0
	}
	sv.parts = make([]particle, n)
	for i := range sv.parts {
		sv.respawn(&sv.parts[i])
	}
}

func (sv *Saver) respawn(p *particle) {
	p.x, p.y = sv.rng.Float64()*float64(sv.px), sv.rng.Float64()*float64(sv.py)
	p.life = 2 + sv.rng.Float64()*5
}

// ease moves cur towards to, by a share of the way that grows with dt.
func ease(cur, to, dt, rate float64) float64 { return cur + (to-cur)*math.Min(1, dt*rate) }

func (sv *Saver) step(s *state.State, now time.Time) {
	dt := 0.0
	if !sv.last.IsZero() {
		dt = math.Min(math.Max(now.Sub(sv.last).Seconds(), 0), 0.25)
	}
	sv.last = now
	sv.energy *= math.Exp(-dt / 4)

	// The mood of the run: a stage working is vivid, in the gradient of its
	// place in the pipeline; a wait is cold and breathes; the rest is grey.
	mood, bright, speed := "rest", 0.8, 0.3
	e := math.Min(sv.energy, 1)
	switch {
	case s.Stage != nil:
		mood = work[max(s.Stage.Pos-1, 0)%len(work)]
		bright, speed = 0.7+0.25*e, 0.35+1.5*e
	case s.Wait != nil:
		mood = "wait"
		bright, speed = 0.8+0.15*math.Sin(sv.phase*3), 0.2
	}
	if mood != sv.mood {
		if sv.mood == "" {
			sv.from, sv.mix = moods[mood], 1
		} else {
			sv.from, sv.mix = sv.blend(), 0
		}
		sv.mood, sv.to = mood, moods[mood]
	}
	sv.mix = math.Min(1, sv.mix+dt/2.5)
	sv.g = sv.blend()
	sv.bright = ease(sv.bright, bright, dt, 1.5)
	sv.phase += dt * speed

	live := sv.pulses[:0]
	for _, p := range sv.pulses {
		if p.age += dt; p.age < p.life {
			live = append(live, p)
		}
	}
	sv.pulses = live

	fade := math.Exp(-dt * 2.5)
	for i := range sv.trail {
		sv.trail[i] *= fade
	}
	pxs := 3 + 28*e
	for i := range sv.parts {
		p := &sv.parts[i]
		u, v := sv.uv(p.x, p.y)
		a := 2 * math.Pi * math.Sin(u*0.7+sv.phase*0.4) * math.Cos(v*0.9-sv.phase*0.3)
		p.x += math.Cos(a) * pxs * dt
		p.y += math.Sin(a) * pxs * dt
		if p.life -= dt; p.life <= 0 || p.x < 0 || p.y < 0 || p.x >= float64(sv.px) || p.y >= float64(sv.py) {
			sv.respawn(p)
			continue
		}
		sv.trail[int(p.y)*sv.px+int(p.x)] = 1
	}
}

// uv is a pixel in the plasma's units: the centre is 0, the long side spans 6.
func (sv *Saver) uv(x, y float64) (float64, float64) {
	scale := math.Max(float64(sv.px), float64(sv.py)) / 6
	return (x - float64(sv.px)/2) / scale, (y - float64(sv.py)/2) / scale
}

// ramp draws brightness with characters, for a terminal with no colour.
var ramp = []rune(" .:-=+*#%@")

func (sv *Saver) row(row int) string {
	var b strings.Builder
	if sv.p == termenv.Ascii {
		for x := 0; x < sv.px; x++ {
			c1, c2 := sv.pixel(x, 2*row), sv.pixel(x, 2*row+1)
			l := clamp01((c1.r + c1.g + c1.b + c2.r + c2.g + c2.b) / 6 / 0.7)
			b.WriteRune(ramp[int(l*float64(len(ramp)-1)+0.5)])
		}
		return b.String()
	}
	var lastF, lastB uint32 = 1 << 31, 1 << 31
	for x := 0; x < sv.px; x++ {
		f, k := quant(sv.pixel(x, 2*row)), quant(sv.pixel(x, 2*row+1))
		// One SGR for what changed: the top pixel is the glyph, the bottom one
		// the cell behind it.
		switch nf, nb := f != lastF, k != lastB; {
		case nf && nb:
			b.WriteString("\x1b[" + sv.seq(sv.fg, f, false) + ";" + sv.seq(sv.bg, k, true) + "m")
		case nf:
			b.WriteString("\x1b[" + sv.seq(sv.fg, f, false) + "m")
		case nb:
			b.WriteString("\x1b[" + sv.seq(sv.bg, k, true) + "m")
		}
		lastF, lastB = f, k
		b.WriteString("▀")
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

// pixel is the plasma at one point: four interfering waves read through a
// cosine palette, then the rings of the pulses and the trails of the particles.
func (sv *Saver) pixel(x, y int) rgb {
	u, v := sv.uv(float64(x), float64(y))
	ph := sv.phase
	cx, cy := u+0.9*math.Sin(ph/5), v+0.9*math.Cos(ph/3)
	val := math.Sin(u*1.3+ph) +
		math.Sin(1.7*(u*math.Sin(ph/2)+v*math.Cos(ph/3))+ph) +
		math.Sin(math.Sqrt(cx*cx+cy*cy)*2.2+ph) +
		math.Sin(v*1.1-ph*0.7)
	t := val/8 + 0.5
	g := sv.g
	c := g.at(t).scale(sv.bright)
	// Contour lines, faint, as on a map.
	if d := math.Abs(frac(t*9+sv.phase*0.05)-0.5) * 2; d > 0.88 {
		c = c.add(g.at(1), (d-0.88)/0.12*0.18)
	}

	for _, p := range sv.pulses {
		d := math.Hypot(u-p.x, v-p.y) - p.age*p.speed
		k := math.Exp(-d*d/0.06) * p.power * (1 - p.age/p.life)
		if k > 0.004 {
			c = c.add(p.col, k)
		}
	}
	if tr := sv.trail[y*sv.px+x]; tr > 0.01 {
		glow := g.at(1)
		c = c.add(rgb{0.5 + glow.r/2, 0.5 + glow.g/2, 0.5 + glow.b/2}, tr*0.6)
	}
	return c
}

// A gradient runs from the deep of the plasma to its crests.
type gradient [5]rgb

func grad(hex ...string) gradient {
	var g gradient
	for i, h := range hex {
		g[i] = hexRGB(h)
	}
	return g
}

var moods = map[string]gradient{
	"synth":  grad("#0B1026", "#3B0764", "#7C3AED", "#F472B6", "#FDE68A"),
	"aurora": grad("#03161D", "#0E4A5C", "#0E7490", "#34D399", "#ECFCCB"),
	"nebula": grad("#14051F", "#4C1D95", "#2563EB", "#22D3EE", "#F0ABFC"),
	"ember":  grad("#1A0A00", "#7C2D12", "#EA580C", "#FBBF24", "#FEF9C3"),
	"wait":   grad("#020617", "#0B1B3F", "#1E3A8A", "#2563EB", "#7DD3FC"),
	"rest":   grad("#09090B", "#18181B", "#3F3F46", "#71717A", "#D4D4D8"),
}

// work are the gradients of the stages, by their place in the pipeline.
var work = []string{"synth", "aurora", "nebula", "ember"}

func (g gradient) at(t float64) rgb {
	t = clamp01(t) * float64(len(g)-1)
	i := min(int(t), len(g)-2)
	f := t - float64(i)
	f = f * f * (3 - 2*f)
	a, b := g[i], g[i+1]
	return rgb{a.r + (b.r-a.r)*f, a.g + (b.g-a.g)*f, a.b + (b.b-a.b)*f}
}

// blend is the gradient on screen: part way from the last mood to this one.
func (sv *Saver) blend() gradient {
	var g gradient
	for i := range g {
		a, b := sv.from[i], sv.to[i]
		g[i] = rgb{a.r + (b.r-a.r)*sv.mix, a.g + (b.g-a.g)*sv.mix, a.b + (b.b-a.b)*sv.mix}
	}
	return g
}

func (c rgb) scale(k float64) rgb { return rgb{c.r * k, c.g * k, c.b * k} }

func frac(f float64) float64 { return f - math.Floor(f) }

func clamp01(f float64) float64 { return math.Min(math.Max(f, 0), 1) }

// quant packs a colour into 6 bits a channel, so the sequence cache stays small.
func quant(c rgb) uint32 {
	q := func(f float64) uint32 { return uint32(clamp01(f)*63+0.5) << 2 }
	return q(c.r)<<16 | q(c.g)<<8 | q(c.b)
}

func (sv *Saver) seq(cache map[uint32]string, c uint32, bg bool) string {
	if s, ok := cache[c]; ok {
		return s
	}
	if len(cache) > 1<<16 {
		clear(cache)
	}
	hex := "#" + hex2(c>>16) + hex2(c>>8) + hex2(c)
	s := sv.p.Color(hex).Sequence(bg) // "38;2;r;g;b", without the CSI
	cache[c] = s
	return s
}

func hex2(v uint32) string {
	const d = "0123456789abcdef"
	v &= 0xff
	return string([]byte{d[v>>4], d[v&0xf]})
}

// status is one line: the run on the left, the way back on the right.
func (sv *Saver) status(s *state.State, w int, now time.Time) string {
	parts := []string{"ralph"}
	if s.Session.Project != "" {
		parts = append(parts, s.Session.Project)
	}
	switch {
	case s.Stage != nil:
		st := s.Stage
		at := st.Task.ID + " " + st.Stage
		if st.Count > 0 {
			at += fmt.Sprintf(" %d/%d", st.Pos, st.Count)
		}
		parts = append(parts, at, view.Dur(int(now.Unix()-st.StartedAt)))
	case s.Wait != nil:
		parts = append(parts, s.Wait.Kind+" "+view.Dur(int(s.Wait.Until-now.Unix())))
	case s.End != nil:
		parts = append(parts, s.End.State)
	case s.Idle:
		parts = append(parts, "no work")
	}
	if s.Counts.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d done", s.Counts.Done, s.Counts.Total))
	}
	if s.CostU > 0 {
		parts = append(parts, view.USD(s.CostU))
	}
	left := " " + strings.Join(parts, " · ")
	right := "any key returns "
	gap := w - ansi.StringWidth(left) - ansi.StringWidth(right)
	text := left + strings.Repeat(" ", max(gap, 0)) + right
	if gap < 2 {
		text = left
	}
	text = ansi.Truncate(text, w, "…")
	text += strings.Repeat(" ", w-ansi.StringWidth(text))
	return sv.p.String(text).Foreground(sv.p.Color("#E4E4E7")).Background(sv.p.Color("#18181B")).String()
}
