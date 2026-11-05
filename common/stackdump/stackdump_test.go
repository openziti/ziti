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

package stackdump

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// convoy builds a dump holding n goroutines blocked at the same place, with the varying parts a real dump has:
// distinct goroutine ids, distinct parents, pointer arguments, call offsets, and how long each has been blocked.
func convoy(n int, blockedIn string, startId int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "goroutine %d [sync.Mutex.Lock, %d minutes]:\n", startId+i, i)
		fmt.Fprintf(&b, "sync.(*Mutex).Lock(0x%x)\n", 0xc000100000+i*0x40)
		fmt.Fprintf(&b, "\t/usr/local/go/src/sync/mutex.go:90 +0x%x\n", 0x20+i)
		fmt.Fprintf(&b, "%s(0x%x, 0x%x)\n", blockedIn, 0xc000200000+i, 0xc000300000+i)
		fmt.Fprintf(&b, "\t/home/x/blocked.go:41 +0x%x\n", 0x100+i)
		fmt.Fprintf(&b, "created by app.(*Server).accept in goroutine %d\n", startId+n+i)
		fmt.Fprintf(&b, "\t/home/x/server.go:63 +0x%x\n", 0x180+i)
	}
	return b.String()
}

// TestSignature_IgnoresWhatVariesBetweenSamples is the whole point of normalizing: two samples of one convoy
// are never byte-identical, so without it every dump is distinct and deduplicating achieves nothing.
func TestSignature_IgnoresWhatVariesBetweenSamples(t *testing.T) {
	first := convoy(10, "app.(*Store).apply", 1)
	second := convoy(10, "app.(*Store).apply", 5000)

	require.NotEqual(t, first, second, "the two samples must differ literally, or the test proves nothing")
	require.NotEqual(t, Signature(nil, 5, ""), Signature([]byte(first), 5, ""),
		"the convoy must count toward the signature, or two samples match for having nothing in it")
	require.Equal(t, Signature([]byte(first), 5, ""), Signature([]byte(second), 5, ""))
}

// TestNormalizeGoroutine_GroupsARealDump normalizes a dump the runtime took rather than one built by hand, so a
// part of the format the fixtures leave out shows up here. The goroutines are blocked in one place but started by
// different parents, as each control channel's receive goroutine is.
func TestNormalizeGoroutine_GroupsARealDump(t *testing.T) {
	const n = 4
	release := make(chan struct{})
	defer close(release)
	for i := 0; i < n; i++ {
		go func() {
			go blockedOnPurpose(release)
		}()
	}

	require.Eventually(t, func() bool {
		return len(goroutinesBlockedIn("blockedOnPurpose")) == n
	}, 5*time.Second, 10*time.Millisecond)

	groups := map[string]int{}
	for _, block := range goroutinesBlockedIn("blockedOnPurpose") {
		groups[normalizeGoroutine(block)]++
	}
	require.Len(t, groups, 1, "goroutines blocked in one place are one group, whatever started them")
}

func blockedOnPurpose(release <-chan struct{}) {
	<-release
}

// goroutinesBlockedIn returns the stacks of this process's goroutines parked on a channel receive in fn.
func goroutinesBlockedIn(fn string) []string {
	buf := make([]byte, 1<<20)
	var result []string
	for _, block := range splitGoroutines(string(buf[:runtime.Stack(buf, true)])) {
		if strings.Contains(block, "[chan receive") && strings.Contains(block, "."+fn+"(") {
			result = append(result, block)
		}
	}
	return result
}

// TestSignature_DistinguishesDifferentConvoys guards the other direction: the point of keeping distinct
// situations is that different blockages are recognised as different.
func TestSignature_DistinguishesDifferentConvoys(t *testing.T) {
	logrusConvoy := convoy(10, "logrus.(*Entry).log", 1)
	stripeConvoy := convoy(10, "concurrency.(*StripedIdLocker).LockFor", 1)

	require.NotEqual(t, Signature([]byte(logrusConvoy), 5, ""), Signature([]byte(stripeConvoy), 5, ""))
}

// TestSignature_IgnoresIncidentalGoroutines covers minGroup. A dump holds hundreds of unrelated goroutines that
// come and go, and letting those into the signature would make every sample distinct.
func TestSignature_IgnoresIncidentalGoroutines(t *testing.T) {
	base := convoy(10, "app.(*Store).apply", 1)

	withNoise := base +
		"goroutine 9001 [chan receive]:\n" +
		"app.somethingIncidental(0xc000abc000)\n" +
		"\t/home/x/other.go:12 +0x40\n"

	require.Equal(t, Signature([]byte(base), 5, ""), Signature([]byte(withNoise), 5, ""),
		"a goroutine on its own must not change the signature")
}

// TestSignature_GroupSizeChangesAreNotADifferentSituation: a convoy of 40 and a convoy of 12 are the same
// problem seen at different moments, and should share a file rather than each claiming one.
func TestSignature_GroupSizeChangesAreNotADifferentSituation(t *testing.T) {
	require.Equal(t,
		Signature([]byte(convoy(40, "app.(*Store).apply", 1)), 5, ""),
		Signature([]byte(convoy(12, "app.(*Store).apply", 900)), 5, ""))
}

// newTestRecorder returns a Recorder whose snapshots the test controls, so the retention logic is exercised
// against known dumps. Snapshotting the live process would not do: its stacks differ between two calls, down to
// the line the caller is on, so every capture would look like a new situation.
func newTestRecorder(t *testing.T, maxSignatures int, interval time.Duration, dumps func() []byte) *Recorder {
	t.Helper()
	r := NewRecorder(Config{
		Dir:           t.TempDir(),
		FilePrefix:    "test",
		Interval:      interval,
		MaxSignatures: maxSignatures,
		InitialBytes:  1 << 16,
		MaxBytes:      1 << 20,
		MinGroup:      5,
	})
	r.snapshotFn = dumps
	return r
}

// fixedDump always returns the same convoy, standing in for a process stuck in one situation.
func fixedDump(blockedIn string) func() []byte {
	return func() []byte {
		return []byte(convoy(10, blockedIn, 1))
	}
}

func TestRecorder_RateLimit(t *testing.T) {
	r := newTestRecorder(t, 10, time.Hour, fixedDump("app.(*Store).apply"))

	require.Equal(t, Written, r.Capture("subject").Outcome, "the first capture is taken")
	require.Equal(t, RateLimited, r.Capture("subject").Outcome, "a capture inside the interval is not")
}

// TestRecorder_CountsRepeatsAgainstOneFile is what replaces a plain cap on dumps: a process that stays slow
// records how often it saw each situation, and points every repeat at the file describing it.
func TestRecorder_CountsRepeatsAgainstOneFile(t *testing.T) {
	r := newTestRecorder(t, 10, 0, fixedDump("app.(*Store).apply"))

	first := r.Capture("subject")
	require.Equal(t, Written, first.Outcome)
	require.Equal(t, 1, first.Occurrences)
	require.FileExists(t, first.Path)

	// The same situation, sampled again.
	for i := 2; i <= 4; i++ {
		repeat := r.Capture("subject")
		require.Equal(t, Repeat, repeat.Outcome)
		require.Equal(t, i, repeat.Occurrences, "each repeat is counted")
		require.Equal(t, first.Path, repeat.Path, "a repeat points at the file already describing it")
		require.Equal(t, first.Signature, repeat.Signature)
	}

	files, err := os.ReadDir(r.cfg.Dir)
	require.NoError(t, err)
	require.Len(t, files, 1, "repeats must not write further files")
}

// TestRecorder_SignatureLimit covers the ceiling, which is what bounds the disk cost.
// TestRecorder_KeepsDistinctSituations is the behaviour a plain cap on dumps did not give: different blockages
// each get a file, rather than the first one being sampled over and over.
func TestRecorder_KeepsDistinctSituations(t *testing.T) {
	blockedIn := "app.first"
	r := newTestRecorder(t, 10, 0, func() []byte {
		return []byte(convoy(10, blockedIn, 1))
	})

	first := r.Capture("subject")
	require.Equal(t, Written, first.Outcome)

	blockedIn = "app.second"
	second := r.Capture("subject")
	require.Equal(t, Written, second.Outcome, "a different blockage is a different situation")
	require.NotEqual(t, first.Signature, second.Signature)
	require.NotEqual(t, first.Path, second.Path)
	require.Equal(t, 2, second.DistinctSignatures)

	files, err := os.ReadDir(r.cfg.Dir)
	require.NoError(t, err)
	require.Len(t, files, 2)
}

// TestRecorder_SignatureLimit covers the ceiling, which is what bounds the disk cost.
func TestRecorder_SignatureLimit(t *testing.T) {
	blockedIn := "app.first"
	r := newTestRecorder(t, 2, 0, func() []byte {
		return []byte(convoy(10, blockedIn, 1))
	})

	for _, name := range []string{"app.first", "app.second"} {
		blockedIn = name
		require.Equal(t, Written, r.Capture("subject").Outcome)
	}

	blockedIn = "app.third"
	result := r.Capture("subject")
	require.Equal(t, SignatureLimit, result.Outcome,
		"a new situation past the ceiling must not write a file")
	require.Equal(t, 2, result.DistinctSignatures)

	files, err := os.ReadDir(r.cfg.Dir)
	require.NoError(t, err)
	require.Len(t, files, 2, "no file for the situation past the ceiling")
}

// TestRecorder_SnapshotGrowsToFit covers why the buffer is not fixed: a truncated dump hides whichever
// goroutine the rest are queued behind, which is the only reason to take one.
func TestRecorder_SnapshotGrowsToFit(t *testing.T) {
	r := NewRecorder(Config{
		Dir:          t.TempDir(),
		FilePrefix:   "test",
		InitialBytes: 64, // far too small, so it has to grow
		MaxBytes:     1 << 20,
		MinGroup:     1,
	})

	stack := r.snapshot()
	require.Greater(t, len(stack), 64, "the buffer must have grown past its initial size")
	require.Less(t, len(stack), 1<<20, "and stopped short of the ceiling, so this dump is not truncated")
	require.Contains(t, string(stack), "goroutine ")
}

// TestRecorder_SnapshotStaysWithinMaxBytes: MaxBytes bounds the file, so growth stops at it rather than at
// the next doubling past it, and a buffer that starts above it starts at it instead.
func TestRecorder_SnapshotStaysWithinMaxBytes(t *testing.T) {
	for name, initial := range map[string]int{"growing past the max": 64, "starting above the max": 1 << 16} {
		r := NewRecorder(Config{
			Dir:          t.TempDir(),
			FilePrefix:   "test",
			InitialBytes: initial,
			MaxBytes:     96,
			MinGroup:     1,
		})

		require.LessOrEqual(t, len(r.snapshot()), 96, name)
	}
}

// activeHandler builds one goroutine inside a wrapped handler, as a busy process has many of at any moment.
func activeHandler(blockedIn string, id int) string {
	return fmt.Sprintf("goroutine %d [semacquire]:\n", id) +
		fmt.Sprintf("%s(0xc000440000)\n", blockedIn) +
		"\t/home/x/slow.go:77 +0x50\n" +
		"app.wrapHandler.func2(0xc000550000, 0xc000560000)\n" +
		"\t/home/x/handler_diagnostic.go:127 +0x84\n"
}

// TestSignature_SubjectSeparatesWhatTheDumpCannotShow is why the caller names the subject. The goroutine a dump
// is being taken for is normally alone in its position and so filtered out as incidental, which left two
// unrelated failures behind the same background hashing the same. The dump cannot supply what distinguishes
// them, because the goroutine that failed to return looks exactly like every other one currently in a handler.
func TestSignature_SubjectSeparatesWhatTheDumpCannotShow(t *testing.T) {
	background := convoy(10, "app.(*Store).apply", 1)

	first := []byte(background + activeHandler("app.(*Links).handleFault", 8001))
	second := []byte(background + activeHandler("app.(*Terminators).handleCreate", 8002))

	require.Equal(t, Signature(first, 5, ""), Signature(second, 5, ""),
		"the dumps alone are indistinguishable, which is what the subject is for")

	require.NotEqual(t, Signature(first, 5, "contentType=1"), Signature(second, 5, "contentType=2"))
}

// TestSignature_TrafficDoesNotChangeTheSituation is the constraint that rules out exempting handler goroutines
// from the group filter to solve the above. Every handler running at the moment of a snapshot looks like the one
// that triggered it, so exempting them would let ordinary traffic make each sample of one blockage distinct,
// spending the signature ceiling on the same problem and suppressing later dumps of different ones.
func TestSignature_TrafficDoesNotChangeTheSituation(t *testing.T) {
	blocked := convoy(10, "app.(*Store).apply", 1) + activeHandler("app.(*Links).handleFault", 8001)

	busier := blocked +
		activeHandler("app.(*Terminators).handleCreate", 9001) +
		activeHandler("app.(*Circuits).handleRoute", 9002) +
		activeHandler("app.(*Api).handleSession", 9003)

	require.Equal(t, Signature([]byte(blocked), 5, "contentType=1"), Signature([]byte(busier), 5, "contentType=1"),
		"handlers that merely happen to be running are not what the dump is about")
}

// TestSignature_SameSubjectAndBackgroundIsOneSituation is the deduplication working: one problem sampled twice
// shares a file rather than each sample claiming one.
func TestSignature_SameSubjectAndBackgroundIsOneSituation(t *testing.T) {
	require.Equal(t,
		Signature([]byte(convoy(10, "app.(*Store).apply", 1)), 5, "contentType=1"),
		Signature([]byte(convoy(12, "app.(*Store).apply", 700)), 5, "contentType=1"))
}

// TestRecorder_TrafficDoesNotSpendTheSignatureLimit is the same constraint at the level it would be paid at: a
// process stuck in one place, sampled while other handlers come and go, must keep writing one file rather than
// filling the ceiling and going quiet for the situations still to come.
func TestRecorder_TrafficDoesNotSpendTheSignatureLimit(t *testing.T) {
	sample := 0
	r := newTestRecorder(t, 2, 0, func() []byte {
		sample++
		// The blockage stays put; the traffic around it does not.
		return []byte(convoy(10, "app.(*Store).apply", 1) +
			activeHandler("app.(*Links).handleFault", 8001) +
			activeHandler(fmt.Sprintf("app.handler%d", sample), 9000+sample))
	})

	first := r.Capture("contentType=1")
	require.Equal(t, Written, first.Outcome)

	for i := 2; i <= 5; i++ {
		repeat := r.Capture("contentType=1")
		require.Equal(t, Repeat, repeat.Outcome, "traffic around a blockage is not a new situation")
		require.Equal(t, i, repeat.Occurrences)
	}
	require.Equal(t, 1, first.DistinctSignatures)

	// The ceiling is 2, so it is still possible to record a different handler going slow.
	other := r.Capture("contentType=2")
	require.Equal(t, Written, other.Outcome, "the limit must still have room for a genuinely new situation")
}

func TestRecorder_CheckDir_CreatesAMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	require.NoError(t, NewRecorder(Config{Dir: dir}).CheckDir())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "the write check leaves nothing behind")
}

func TestRecorder_CheckDir_ReportsADirectoryItCannotCreate(t *testing.T) {
	inTheWay := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(inTheWay, nil, 0o644))

	require.Error(t, NewRecorder(Config{Dir: filepath.Join(inTheWay, "dumps")}).CheckDir())
}

// TestRecorder_CheckDir_ReportsADirectoryItCannotWrite covers a directory that exists, which creating it does not
// test.
func TestRecorder_CheckDir_ReportsADirectoryItCannotWrite(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not stop this user writing")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	require.Error(t, NewRecorder(Config{Dir: dir}).CheckDir())
}
