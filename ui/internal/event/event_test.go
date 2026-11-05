package event

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecodeGivesTheTypedPayload(t *testing.T) {
	e, err := Decode([]byte(`{"v":1,"ts":100,"type":"stage.start","iter":2,"attempt":1,
		"task":{"id":"US-002","title":"Badge","priority":2,"tier":"med","pr":null},
		"stage":"review_code","pos":3,"count":4,"claimExpires":7300}`))
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != "stage.start" || e.Ts != 100 {
		t.Fatalf("envelope: %+v", e)
	}
	s, ok := e.Body.(StageStart)
	if !ok {
		t.Fatalf("body is %T", e.Body)
	}
	if s.Task.ID != "US-002" || s.Pos != 3 || s.Count != 4 || s.ClaimExpires != 7300 || s.Task.PR != "" {
		t.Fatalf("payload: %+v", s)
	}
}

func TestDecodeKeepsUnknownTypes(t *testing.T) {
	e, err := Decode([]byte(`{"v":1,"ts":1,"type":"something.new","x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := e.Body.(Unknown); !ok {
		t.Fatalf("body is %T", e.Body)
	}
}

func TestDecodeRefusesWhatIsNotJSON(t *testing.T) {
	if _, err := Decode([]byte(`{"broken`)); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFollowReadsAsTheFileGrowsAndStopsAtTheExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.ndjson")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The file does not exist yet: the worker has not started writing.
	ch := Follow(ctx, path, 10*time.Millisecond)

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	write := func(s string) {
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"v":1,"ts":1,"type":"session.start","project":"myapp"}` + "\n")
	if e := <-ch; e.Type != "session.start" {
		t.Fatalf("first: %+v", e)
	}
	// A line written in two goes is read once it is whole.
	write(`{"v":1,"ts":2,"type":"notice",`)
	time.Sleep(30 * time.Millisecond)
	write(`"level":"warn","text":"x"}` + "\n")
	if e := <-ch; e.Type != "notice" {
		t.Fatalf("second: %+v", e)
	}
	write("not json\n")
	write(`{"v":1,"ts":3,"type":"session.exit","code":0}` + "\n")
	if e := <-ch; e.Type != "session.exit" {
		t.Fatalf("a bad line is skipped, got %+v", e)
	}
	if _, open := <-ch; open {
		t.Fatal("the channel stays open after the exit")
	}
}

func TestReadAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.ndjson")
	body := `{"v":1,"ts":1,"type":"session.start","project":"myapp","pipeline":["a","b"]}` + "\n" +
		`{"v":1,"ts":2,"type":"session.exit","code":3}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	evs, err := ReadAll(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0].Body.(SessionStart).Pipeline[1] != "b" || evs[1].Body.(SessionExit).Code != 3 {
		t.Fatalf("%+v", evs)
	}
}
