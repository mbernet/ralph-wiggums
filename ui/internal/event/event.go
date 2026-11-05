// Package event decodes the event stream ralph.sh writes to
// $LOG_DIR/events.ndjson: one JSON object per line, described in
// docs/events.md. Each type has its own payload struct; a type this build does
// not know is kept as Unknown, so a newer ralph.sh never breaks an older UI.
package event

import (
	"encoding/json"
	"fmt"
)

// Event is one line of the stream.
type Event struct {
	V    int
	Ts   int64 // unix seconds
	Type string
	Body any
}

type envelope struct {
	V    int    `json:"v"`
	Ts   int64  `json:"ts"`
	Type string `json:"type"`
}

type Counts struct {
	Total      int `json:"total"`
	Done       int `json:"done"`
	Claimed    int `json:"claimed"`
	Sick       int `json:"sick"`
	NeedsHuman int `json:"needsHuman"`
	Blocked    int `json:"blocked"`
}

type Tokens struct {
	In         int64 `json:"in"`
	Out        int64 `json:"out"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

type SessionStart struct {
	Run         string            `json:"run"`
	Project     string            `json:"project"`
	Description string            `json:"description"`
	Worker      string            `json:"worker"`
	API         string            `json:"api"`
	Branch      string            `json:"branch"`
	Pipeline    []string          `json:"pipeline"`
	Tool        string            `json:"tool"`
	ToolVersion string            `json:"toolVersion"`
	Tiers       map[string]string `json:"tiers"`
	Pace        struct {
		Mode         string `json:"mode"`
		BudgetPct    int    `json:"budgetPct"`
		WeekGuardPct int    `json:"weekGuardPct"`
		Reason       string `json:"reason"`
	} `json:"pace"`
	MaxIterations int    `json:"maxIterations"`
	DryRun        bool   `json:"dryRun"`
	Scenario      string `json:"scenario"`
	Story         string `json:"story"`
	Logs          string `json:"logs"`
	PID           int    `json:"pid"`
	Control       struct {
		Stop string `json:"stop"`
		Skip string `json:"skip"`
	} `json:"control"`
}

type Project struct {
	Counts Counts `json:"counts"`
}

type Claim struct {
	Worker    string `json:"worker"`
	ExpiresAt int64  `json:"expiresAt"`
}

// Task is a card of the board: what the panel of the spec (section 9) shows.
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Priority    int    `json:"priority"`
	Stage       string `json:"stage"`
	Runs        int    `json:"runs"`
	Sick        bool   `json:"sick"`
	NeedsHuman  bool   `json:"needsHuman"`
	Claim       *Claim `json:"claim"`
	PR          string `json:"pr"`
	Rejects     int    `json:"rejects"`
	Assumptions int    `json:"assumptions"`
	DoneAt      int64  `json:"doneAt"`
	Why         string `json:"why"`
	BlockedBy   string `json:"blockedBy"`
}

type Board struct {
	Tasks []Task `json:"tasks"`
}

type WindowPart struct {
	Pct      int   `json:"pct"`
	ResetsAt int64 `json:"resetsAt"`
}

type Window struct {
	Src          string     `json:"src"`
	FiveHour     WindowPart `json:"fiveHour"`
	SevenDay     WindowPart `json:"sevenDay"`
	Fresh        bool       `json:"fresh"`
	FetchedAt    int64      `json:"fetchedAt"`
	SpendU       int64      `json:"spendU"`
	BudgetPct    int        `json:"budgetPct"`
	WeekGuardPct int        `json:"weekGuardPct"`
}

// Known says whether any source could read the window.
func (w Window) Known() bool { return w.Src != "" && w.Src != "none" }

type Pace struct {
	Action      string `json:"action"`
	SleepS      int    `json:"sleepS"`
	Why         string `json:"why"`
	EstimatePct int    `json:"estimatePct"`
	Tier        string `json:"tier"`
}

type WaitStart struct {
	Kind    string `json:"kind"`
	Seconds int    `json:"seconds"`
	Until   int64  `json:"until"`
}

type WaitEnd struct {
	Kind    string `json:"kind"`
	Skipped bool   `json:"skipped"`
}

type WorkNone struct {
	Finished bool   `json:"finished"`
	IdleS    int    `json:"idleS"`
	Counts   Counts `json:"counts"`
}

type StageTask struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Priority int    `json:"priority"`
	Tier     string `json:"tier"`
	PR       string `json:"pr"`
}

type StageStart struct {
	Iter          int       `json:"iter"`
	MaxIterations int       `json:"maxIterations"`
	Attempt       int       `json:"attempt"`
	Task          StageTask `json:"task"`
	Stage         string    `json:"stage"`
	Pos           int       `json:"pos"`
	Count         int       `json:"count"`
	TaskStage     string    `json:"taskStage"`
	Model         string    `json:"model"`
	Tool          string    `json:"tool"`
	ClaimExpires  int64     `json:"claimExpires"`
	Stream        string    `json:"stream"`
}

type Commit struct {
	Sha     string `json:"sha"`
	Subject string `json:"subject"`
}

type StageEnd struct {
	Iter        int      `json:"iter"`
	Attempt     int      `json:"attempt"`
	Task        string   `json:"task"`
	Stage       string   `json:"stage"`
	Class       string   `json:"class"`
	Subtype     string   `json:"subtype"`
	Outcome     string   `json:"outcome"`
	Next        string   `json:"next"`
	ByScript    bool     `json:"byScript"`
	Error       string   `json:"error"`
	DurS        int      `json:"durS"`
	Turns       int      `json:"turns"`
	CostU       int64    `json:"costU"`
	RunCostU    int64    `json:"runCostU"`
	Tokens      Tokens   `json:"tokens"`
	Model       string   `json:"model"`
	Commit      *Commit  `json:"commit"`
	WinDelta    int      `json:"winDelta"`
	PR          string   `json:"pr"`
	Body        string   `json:"body"`
	Assumptions []string `json:"assumptions"`
	Summary     string   `json:"summary"`
}

type AgentInit struct {
	Session string `json:"session"`
	Model   string `json:"model"`
}

type AgentText struct {
	Text string `json:"text"`
}

type AgentThinking struct{}

type AgentTool struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Arg  string `json:"arg"`
}

type AgentResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

type AgentCompact struct{}

type AgentRateLimit struct {
	Status   string `json:"status"`
	Kind     string `json:"kind"`
	ResetsAt int64  `json:"resetsAt"`
	Pct      *int   `json:"pct"`
}

type AgentLine struct {
	Text string `json:"text"`
}

type Notice struct {
	Level string `json:"level"` // info | ok | warn | error
	Text  string `json:"text"`
}

type TierTally struct {
	Runs  int   `json:"runs"`
	CostU int64 `json:"costU"`
}

type SessionEnd struct {
	State      string               `json:"state"`
	Ran        int                  `json:"ran"`
	Registered int                  `json:"registered"`
	Failed     int                  `json:"failed"`
	Retried    int                  `json:"retried"`
	CostU      int64                `json:"costU"`
	Tokens     Tokens               `json:"tokens"`
	WallS      int                  `json:"wallS"`
	AgentS     int                  `json:"agentS"`
	ThrottledS int                  `json:"throttledS"`
	Tiers      map[string]TierTally `json:"tiers"`
	Ledger     string               `json:"ledger"`
	Logs       string               `json:"logs"`
	Next       string               `json:"next"`
}

type SessionExit struct {
	Code int `json:"code"`
}

// Unknown is the payload of a type this build does not know.
type Unknown struct{}

func as[T any](line []byte) (any, error) {
	var v T
	err := json.Unmarshal(line, &v)
	return v, err
}

var decoders = map[string]func([]byte) (any, error){
	"session.start":   as[SessionStart],
	"project":         as[Project],
	"board":           as[Board],
	"window":          as[Window],
	"pace":            as[Pace],
	"wait.start":      as[WaitStart],
	"wait.end":        as[WaitEnd],
	"work.none":       as[WorkNone],
	"stage.start":     as[StageStart],
	"stage.end":       as[StageEnd],
	"agent.init":      as[AgentInit],
	"agent.text":      as[AgentText],
	"agent.thinking":  as[AgentThinking],
	"agent.tool":      as[AgentTool],
	"agent.result":    as[AgentResult],
	"agent.compact":   as[AgentCompact],
	"agent.ratelimit": as[AgentRateLimit],
	"agent.line":      as[AgentLine],
	"notice":          as[Notice],
	"session.end":     as[SessionEnd],
	"session.exit":    as[SessionExit],
}

// Decode reads one line. The body is the payload struct by value.
func Decode(line []byte) (Event, error) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Event{}, fmt.Errorf("not an event: %w", err)
	}
	e := Event{V: env.V, Ts: env.Ts, Type: env.Type, Body: Unknown{}}
	decode, ok := decoders[env.Type]
	if !ok {
		return e, nil
	}
	body, err := decode(line)
	if err != nil {
		return Event{}, fmt.Errorf("%s: %w", env.Type, err)
	}
	e.Body = body
	return e, nil
}
