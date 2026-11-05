package event

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"time"
)

// Follow reads the event file from the start and keeps reading as it grows,
// like tail -f. It waits for the file to appear, skips lines that are not
// events, and closes the channel after session.exit or when ctx ends.
func Follow(ctx context.Context, path string, poll time.Duration) <-chan Event {
	ch := make(chan Event, 256)
	go func() {
		defer close(ch)
		f := waitOpen(ctx, path, poll)
		if f == nil {
			return
		}
		defer f.Close()
		r := bufio.NewReader(f)
		var partial []byte
		for {
			chunk, err := r.ReadBytes('\n')
			partial = append(partial, chunk...)
			if err == nil {
				line := bytes.TrimSpace(partial)
				partial = partial[:0]
				if len(line) == 0 {
					continue
				}
				e, derr := Decode(line)
				if derr != nil {
					continue
				}
				select {
				case ch <- e:
				case <-ctx.Done():
					return
				}
				if e.Type == "session.exit" {
					return
				}
				continue
			}
			if !errors.Is(err, io.EOF) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(poll):
			}
		}
	}()
	return ch
}

func waitOpen(ctx context.Context, path string, poll time.Duration) *os.File {
	for {
		f, err := os.Open(path)
		if err == nil {
			return f
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(poll):
		}
	}
}

// ReadAll decodes a whole file, skipping the lines that are not events.
func ReadAll(path string) ([]Event, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, line := range bytes.Split(b, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if e, err := Decode(line); err == nil {
			out = append(out, e)
		}
	}
	return out, nil
}
