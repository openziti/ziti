/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

// Package stackdump captures goroutine dumps for a diagnostic, keeping one file per distinct situation and
// counting the repeats.
//
// Written for the slow-handler diagnostics in controller/handler_ctrl and router/env, which both need the same
// capture, deduplication and retention. It exists as a package rather than a third copy of that logic.
package stackdump

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Config describes what a Recorder will and will not do.
type Config struct {
	// Dir is where dump files are written, relative to the process's working directory.
	Dir string

	// FilePrefix names the files, as <prefix>-<timestamp>.stack.
	FilePrefix string

	// Interval is the minimum time between snapshots. Snapshotting is what costs: runtime.Stack over every
	// goroutine briefly stops the world and allocates the whole dump.
	Interval time.Duration

	// MaxSignatures caps how many distinct situations a process will ever write a file for. Reaching it stops
	// files being written, not repeats being counted.
	MaxSignatures int

	// InitialBytes and MaxBytes bound the snapshot buffer, which grows until the dump fits. A truncated dump
	// hides whichever goroutine the rest are queued behind, which is the only reason to take one.
	InitialBytes int
	MaxBytes     int

	// MinGroup is how many goroutines must share a normalized stack for it to count toward the signature. See
	// Signature.
	MinGroup int
}

// Outcome is what a Capture call did.
type Outcome int

const (
	// Written means a snapshot was taken for a situation not seen before, and a file written for it.
	Written Outcome = iota

	// Repeat means a snapshot was taken and matched a situation already on file. No new file.
	Repeat

	// RateLimited means no snapshot was taken, because one was taken too recently.
	RateLimited

	// SignatureLimit means a snapshot was taken and was a new situation, but the process has already written
	// files for MaxSignatures of them.
	SignatureLimit

	// WriteFailed means a snapshot was taken but its file could not be written. Stack carries it so the caller
	// can fall back to logging it inline.
	WriteFailed
)

// Result reports what happened, for the caller to log in its own terms.
type Result struct {
	Outcome Outcome

	// Signature identifies the situation. Empty when no snapshot was taken.
	Signature string

	// Occurrences counts how many times this signature has been seen, including this one.
	Occurrences int

	// Path is the file written for this signature, on the call that wrote it and on every repeat, so a repeat
	// can point at the dump that already describes it.
	Path string

	// Bytes is the size of the snapshot taken.
	Bytes int

	// DistinctSignatures is how many situations this process has files for.
	DistinctSignatures int

	// Stack is the snapshot, populated only when Outcome is WriteFailed.
	Stack []byte

	// Err is the write error, populated only when Outcome is WriteFailed.
	Err error
}

type signatureRecord struct {
	occurrences int
	path        string
}

// Recorder captures deduplicated goroutine dumps. Safe for concurrent use.
type Recorder struct {
	cfg Config

	// snapshotFn takes the dump. Overridden in tests, since the retention logic has to be exercised against
	// known dumps: a live process's stacks differ between two calls, down to the line the caller is on, so
	// nothing about deduplication can be asserted against them.
	snapshotFn func() []byte

	lock       sync.Mutex
	lastDump   time.Time
	signatures map[string]*signatureRecord
}

// NewRecorder returns a Recorder for the given config.
func NewRecorder(cfg Config) *Recorder {
	result := &Recorder{
		cfg:        cfg,
		signatures: map[string]*signatureRecord{},
	}
	result.snapshotFn = result.snapshot
	return result
}

// Capture snapshots every goroutine and writes it, unless one was taken too recently, or the situation is one
// already on file, or the process has written as many distinct situations as it is allowed.
//
// subject is what the caller knows the dump is about and the dump itself does not say, and separates situations
// that would otherwise look alike. See Signature.
//
// Deduplicating is what makes the retention useful rather than merely bounded. A process that stays slow dumps
// the same convoy over and over, so a plain count of dumps keeps the same picture ten times and a count of
// distinct ones keeps ten different pictures. The repeats are still worth knowing about, so they are counted
// and pointed at the file that already describes them.
func (r *Recorder) Capture(subject string) Result {
	r.lock.Lock()
	if time.Since(r.lastDump) < r.cfg.Interval {
		r.lock.Unlock()
		return Result{Outcome: RateLimited}
	}
	r.lastDump = time.Now()
	r.lock.Unlock()

	stack := r.snapshotFn()
	signature := Signature(stack, r.cfg.MinGroup, subject)

	r.lock.Lock()
	defer r.lock.Unlock()

	if record, seen := r.signatures[signature]; seen {
		record.occurrences++
		return Result{
			Outcome:            Repeat,
			Signature:          signature,
			Occurrences:        record.occurrences,
			Path:               record.path,
			Bytes:              len(stack),
			DistinctSignatures: len(r.signatures),
		}
	}

	if len(r.signatures) >= r.cfg.MaxSignatures {
		return Result{
			Outcome:            SignatureLimit,
			Signature:          signature,
			Bytes:              len(stack),
			DistinctSignatures: len(r.signatures),
		}
	}

	path, err := r.write(stack, signature)
	if err != nil {
		// Not recorded, so a later capture of the same situation can try again rather than being counted as a
		// repeat of a file that does not exist.
		return Result{
			Outcome:            WriteFailed,
			Signature:          signature,
			Bytes:              len(stack),
			DistinctSignatures: len(r.signatures),
			Stack:              stack,
			Err:                err,
		}
	}

	r.signatures[signature] = &signatureRecord{occurrences: 1, path: path}
	return Result{
		Outcome:            Written,
		Signature:          signature,
		Occurrences:        1,
		Path:               path,
		Bytes:              len(stack),
		DistinctSignatures: len(r.signatures),
	}
}

// snapshot returns every goroutine's stack, growing the buffer until the dump fits so nothing is cut off, and
// giving up growing at MaxBytes so the file size stays bounded.
func (r *Recorder) snapshot() []byte {
	size := r.cfg.InitialBytes
	for {
		buf := make([]byte, size)
		n := runtime.Stack(buf, true)
		if n < len(buf) || size >= r.cfg.MaxBytes {
			return buf[:n]
		}
		size *= 2
	}
}

func (r *Recorder) write(stack []byte, signature string) (string, error) {
	if err := os.MkdirAll(r.cfg.Dir, 0o755); err != nil {
		return "", err
	}
	// Filename-safe timestamp: 2026-05-16T20-30-45.123Z (millisecond precision, ':' replaced with '-' since
	// some tooling dislikes ':' in filenames).
	//
	// The signature is in the name as well as the log, so a file can be matched to the repeats counted against
	// it without opening it, and because a timestamp alone is not unique: two situations found in the same
	// millisecond would otherwise write over each other, leaving one of them lost.
	ts := time.Now().UTC().Format("2006-01-02T15-04-05.000Z")
	path := filepath.Join(r.cfg.Dir, fmt.Sprintf("%s-%s-%s.stack", r.cfg.FilePrefix, ts, signature))
	if err := os.WriteFile(path, stack, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Signature reduces a goroutine dump to a value that compares equal for dumps of the same situation.
//
// Two dumps of one convoy are never byte-identical: goroutine ids differ, arguments are pointer values, and
// call offsets move. So each goroutine's stack is first normalized, keeping its state and its frames' functions
// and file positions and dropping the id, the blocked-for minutes, the arguments and the +0x offsets. That is
// the same equivalence goroutine-analyzer compares stacks on.
//
// The signature is then the set of normalized stacks that at least minGroup goroutines share, which is what a
// convoy looks like. Stacks held by fewer are dropped: those are the incidental difference between one moment
// and the next, and counting them would make every dump distinct, which is the thing being avoided.
//
// That filter drops the goroutine a dump is being taken for, which is normally the only one in its position.
// Rather than exempt it, which cannot be done from the dump alone without also exempting every other goroutine
// in the same position, the caller names what the dump is about in subject and that is mixed into the hash.
// Without it, two unrelated failures behind the same background hash the same, and the second is counted as a
// repeat of the first and never written.
//
// Two failures sharing a subject and a background do still collapse into one situation. The repeat count says
// how often it was seen, which is what there is to know once the dump on file already shows that position.
func Signature(dump []byte, minGroup int, subject string) string {
	if minGroup < 1 {
		minGroup = 1
	}

	counts := map[string]int{}
	for _, block := range splitGoroutines(string(dump)) {
		counts[normalizeGoroutine(block)]++
	}

	var shared []string
	for stack, count := range counts {
		if count >= minGroup {
			shared = append(shared, stack)
		}
	}
	sort.Strings(shared)

	h := fnv.New64a()
	_, _ = h.Write([]byte(subject))
	_, _ = h.Write([]byte{0})
	for _, stack := range shared {
		_, _ = h.Write([]byte(stack))
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// splitGoroutines breaks a dump into one string per goroutine.
func splitGoroutines(dump string) []string {
	var blocks []string
	var current []string

	for _, line := range strings.Split(dump, "\n") {
		if strings.HasPrefix(line, "goroutine ") {
			if len(current) > 0 {
				blocks = append(blocks, strings.Join(current, "\n"))
			}
			current = []string{line}
			continue
		}
		if len(current) > 0 {
			current = append(current, line)
		}
	}
	if len(current) > 0 {
		blocks = append(blocks, strings.Join(current, "\n"))
	}
	return blocks
}

// normalizeGoroutine strips everything from one goroutine's stack that differs between two dumps of the same
// situation, leaving its state and where its frames are.
func normalizeGoroutine(block string) string {
	var out []string
	for i, line := range strings.Split(block, "\n") {
		if line = strings.TrimRight(line, " \t\r"); line == "" {
			continue
		}
		if i == 0 {
			out = append(out, normalizeHeader(line))
			continue
		}
		if strings.HasPrefix(line, "\t") {
			out = append(out, normalizeSourceLine(line))
			continue
		}
		out = append(out, normalizeFuncLine(line))
	}
	return strings.Join(out, "\n")
}

// normalizeHeader keeps a goroutine's state and drops its id and how long it has been in that state, so the
// same blockage seen twice compares equal.
func normalizeHeader(line string) string {
	open := strings.Index(line, "[")
	close := strings.LastIndex(line, "]")
	if open < 0 || close < open {
		return "goroutine"
	}
	state := line[open+1 : close]
	// "sync.Mutex.Lock, 5 minutes" -> "sync.Mutex.Lock". How long it has been blocked is exactly what differs
	// between two samples of one blockage.
	if comma := strings.Index(state, ","); comma >= 0 {
		state = state[:comma]
	}
	return "goroutine [" + strings.TrimSpace(state) + "]"
}

// normalizeFuncLine drops a frame's argument values, which are pointers and so differ every dump.
func normalizeFuncLine(line string) string {
	if open := strings.LastIndex(line, "("); open >= 0 && strings.HasSuffix(line, ")") {
		return line[:open]
	}
	return line
}

// normalizeSourceLine drops the +0x call offset, keeping the file and line.
func normalizeSourceLine(line string) string {
	if idx := strings.Index(line, " +0x"); idx >= 0 {
		return line[:idx]
	}
	return line
}
