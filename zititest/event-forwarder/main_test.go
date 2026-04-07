package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func writeLines(t *testing.T, f *os.File, lines ...string) {
	t.Helper()
	for _, line := range lines {
		_, err := fmt.Fprintln(f, line)
		if err != nil {
			t.Fatalf("failed to write to log file: %v", err)
		}
	}
	_ = f.Sync()
}

func collectLines(ch <-chan string, timeout time.Duration) []string {
	var result []string
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return result
			}
			result = append(result, line)
		case <-deadline:
			return result
		}
	}
}

// startTestTailer creates a tailer that reads from the beginning of the file
// (for test purposes, unlike production which seeks to end).
func startTestTailer(t *testing.T, path string) *fileTailer {
	t.Helper()
	tailer := newFileTailer(path)

	// Open from beginning for tests so we see the lines we write.
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	go tailer.run(f)
	t.Cleanup(func() { tailer.stop() })
	return tailer
}

// filterLines reads from the tailer, applies the matcher, and sends matching
// lines to the output channel.
func filterLines(tailer *fileTailer, match func(string) bool, out chan<- string, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case line, ok := <-tailer.Lines:
			if !ok {
				return
			}
			if match(line) {
				select {
				case out <- line:
				case <-stop:
					return
				}
			}
		}
	}
}

func TestFilterByNamespace(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "event-*.log")
	if err != nil {
		t.Fatal(err)
	}

	tailer := startTestTailer(t, f.Name())
	match := buildMatcher([]string{"edge.apiSessions"})
	out := make(chan string, 100)
	stop := make(chan struct{})

	go filterLines(tailer, match, out, stop)

	writeLines(t, f,
		`{"namespace":"edge.apiSessions","event_type":"created","identity_id":"abc123"}`,
		`{"namespace":"fabric.routers","event_type":"connected","router_id":"r1"}`,
		`{"namespace":"edge.apiSessions","event_type":"refreshed","identity_id":"def456"}`,
		`{"namespace":"edge.sessions","event_type":"created","session_id":"s1"}`,
		`{"namespace":"edge.entityCounts","counts":{}}`,
	)

	lines := collectLines(out, 3*time.Second)
	close(stop)

	if len(lines) != 2 {
		t.Fatalf("expected 2 matching lines, got %d: %v", len(lines), lines)
	}

	if expected := `{"namespace":"edge.apiSessions","event_type":"created","identity_id":"abc123"}`; lines[0] != expected {
		t.Errorf("line 0: got %q, want %q", lines[0], expected)
	}
	if expected := `{"namespace":"edge.apiSessions","event_type":"refreshed","identity_id":"def456"}`; lines[1] != expected {
		t.Errorf("line 1: got %q, want %q", lines[1], expected)
	}
}

func TestFilterMultipleNamespaces(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "event-*.log")
	if err != nil {
		t.Fatal(err)
	}

	tailer := startTestTailer(t, f.Name())
	match := buildMatcher([]string{"edge.apiSessions", "fabric.routers"})
	out := make(chan string, 100)
	stop := make(chan struct{})

	go filterLines(tailer, match, out, stop)

	writeLines(t, f,
		`{"namespace":"edge.apiSessions","event_type":"created"}`,
		`{"namespace":"fabric.routers","event_type":"connected"}`,
		`{"namespace":"edge.sessions","event_type":"created"}`,
		`{"namespace":"fabric.terminators","event_type":"created"}`,
	)

	lines := collectLines(out, 3*time.Second)
	close(stop)

	if len(lines) != 2 {
		t.Fatalf("expected 2 matching lines, got %d: %v", len(lines), lines)
	}
}

func TestFilterEmptyNamespacesMatchesAll(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "event-*.log")
	if err != nil {
		t.Fatal(err)
	}

	tailer := startTestTailer(t, f.Name())
	match := buildMatcher(nil)
	out := make(chan string, 100)
	stop := make(chan struct{})

	go filterLines(tailer, match, out, stop)

	writeLines(t, f,
		`{"namespace":"edge.apiSessions","event_type":"created"}`,
		`{"namespace":"fabric.routers","event_type":"connected"}`,
		`{"namespace":"anything","data":"value"}`,
	)

	lines := collectLines(out, 3*time.Second)
	close(stop)

	if len(lines) != 3 {
		t.Fatalf("expected 3 matching lines (all), got %d: %v", len(lines), lines)
	}
}

func TestHandlesLogRotation(t *testing.T) {
	dir := t.TempDir()
	logPath := dir + "/event.log"

	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}

	tailer := startTestTailer(t, logPath)
	match := buildMatcher([]string{"edge.apiSessions"})
	out := make(chan string, 100)
	stop := make(chan struct{})

	go filterLines(tailer, match, out, stop)

	// Write to original file.
	writeLines(t, f,
		`{"namespace":"edge.apiSessions","event_type":"created","id":"before-rotation"}`,
	)

	lines := collectLines(out, 3*time.Second)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line before rotation, got %d", len(lines))
	}

	// Simulate rotation: rename old file, create new one at same path.
	_ = f.Close()
	if err := os.Rename(logPath, logPath+".1"); err != nil {
		t.Fatal(err)
	}

	f2, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}

	// Give the tailer time to detect the rotation.
	time.Sleep(time.Second)

	writeLines(t, f2,
		`{"namespace":"edge.apiSessions","event_type":"refreshed","id":"after-rotation"}`,
	)

	lines = collectLines(out, 3*time.Second)
	close(stop)
	_ = f2.Close()

	if len(lines) != 1 {
		t.Fatalf("expected 1 line after rotation, got %d: %v", len(lines), lines)
	}
	if expected := `{"namespace":"edge.apiSessions","event_type":"refreshed","id":"after-rotation"}`; lines[0] != expected {
		t.Errorf("got %q, want %q", lines[0], expected)
	}
}
