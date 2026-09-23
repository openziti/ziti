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

package xlink_transport

import (
	"sync"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/stretchr/testify/require"
)

// settings builds a generation with intervals whose sum stays under the
// timeout, the relationship the real config validation enforces.
func settings(gen uint64, send, check, closeTimeout time.Duration) xlink.HeartbeatSettings {
	return xlink.HeartbeatSettings{
		Generation:               gen,
		SendInterval:             send,
		CheckInterval:            check,
		CloseUnresponsiveTimeout: closeTimeout,
	}
}

type recordingHeartbeatControl struct {
	send  time.Duration
	check time.Duration
	calls int
}

func (self *recordingHeartbeatControl) UpdateIntervals(send, check time.Duration) {
	self.send = send
	self.check = check
	self.calls++
}

func Test_heartbeatControl_UpdatesCurrent(t *testing.T) {
	req := require.New(t)

	var hc heartbeatControl
	a := &recordingHeartbeatControl{}
	hc.SetHeartbeatControl(a, settings(1, 10*time.Second, time.Second, time.Minute))

	next := settings(2, 2*time.Second, 200*time.Millisecond, 30*time.Second)
	hc.UpdateHeartbeat(next)

	req.Equal(1, a.calls)
	req.Equal(2*time.Second, a.send)
	req.Equal(200*time.Millisecond, a.check)
	req.Equal(next, hc.HeartbeatSettings(), "the link adopts the pushed generation")
}

// A split link binds a control per channel, payload and ack. Both must be
// re-tuned: every channel's callback reads the close-unresponsive timeout live,
// so one left at an older, longer interval can close a healthy link when the
// timeout shortens.
func Test_heartbeatControl_RetunesEveryRegisteredControl(t *testing.T) {
	req := require.New(t)

	var hc heartbeatControl
	payload := &recordingHeartbeatControl{}
	ack := &recordingHeartbeatControl{}
	base := settings(1, 10*time.Second, time.Second, time.Minute)
	hc.SetHeartbeatControl(payload, base)
	hc.SetHeartbeatControl(ack, base)

	hc.UpdateHeartbeat(settings(2, 3*time.Second, 300*time.Millisecond, 30*time.Second))

	for name, ctrl := range map[string]*recordingHeartbeatControl{"payload": payload, "ack": ack} {
		req.Equal(1, ctrl.calls, "%s channel must be re-tuned", name)
		req.Equal(3*time.Second, ctrl.send, "%s channel", name)
		req.Equal(300*time.Millisecond, ctrl.check, "%s channel", name)
	}

	// A second retune reaches both again, rather than only the newest.
	hc.UpdateHeartbeat(settings(3, time.Second, time.Second, 10*time.Second))
	req.Equal(2, payload.calls)
	req.Equal(2, ack.calls)
}

func Test_heartbeatControl_IgnoresNilControl(t *testing.T) {
	req := require.New(t)

	var hc heartbeatControl
	hc.SetHeartbeatControl(nil, settings(1, 10*time.Second, time.Second, time.Minute))
	a := &recordingHeartbeatControl{}
	hc.SetHeartbeatControl(a, settings(1, 10*time.Second, time.Second, time.Minute))

	// A nil handle must not be stored, or the retune loop would panic on it.
	hc.UpdateHeartbeat(settings(2, time.Second, time.Second, 10*time.Second))
	req.Equal(1, a.calls)
}

func Test_heartbeatControl_NoControlIsNoop(t *testing.T) {
	var hc heartbeatControl
	// must not panic with nothing registered
	hc.UpdateHeartbeat(settings(1, time.Second, time.Second, 10*time.Second))
}

func Test_heartbeatControl_IgnoresStaleGeneration(t *testing.T) {
	// Change notifications run on independent goroutines, so an older one can
	// land after a newer. It must not roll the link back onto older settings.
	req := require.New(t)

	var hc heartbeatControl
	a := &recordingHeartbeatControl{}
	hc.SetHeartbeatControl(a, settings(1, 10*time.Second, time.Second, time.Minute))

	newer := settings(5, time.Second, 100*time.Millisecond, 10*time.Second)
	hc.UpdateHeartbeat(newer)
	req.Equal(newer, hc.HeartbeatSettings())

	older := settings(4, 30*time.Second, 20*time.Second, 5*time.Minute)
	hc.UpdateHeartbeat(older)
	req.Equal(newer, hc.HeartbeatSettings(), "an overtaken update must not be applied")
	req.Equal(1, a.calls, "and must not retune the ticker either")

	// The same generation is also a no-op, which is what makes the
	// post-registration reconcile free in the common case.
	hc.UpdateHeartbeat(newer)
	req.Equal(1, a.calls)
}

// gatedControl records the generation implied by the intervals it was re-tuned
// to, and holds the first caller inside UpdateIntervals until released. That is
// how a second update gets the chance to overtake one already dispatching.
type gatedControl struct {
	lock     sync.Mutex
	recorded []uint64
	entered  chan struct{}
	gate     chan struct{}
	seen     bool
}

func newGatedControl() *gatedControl {
	return &gatedControl{
		entered: make(chan struct{}),
		gate:    make(chan struct{}),
	}
}

func (self *gatedControl) UpdateIntervals(send, _ time.Duration) {
	self.lock.Lock()
	first := !self.seen
	self.seen = true
	self.lock.Unlock()

	if first {
		close(self.entered)
		<-self.gate
	}

	self.lock.Lock()
	defer self.lock.Unlock()
	self.recorded = append(self.recorded, uint64(send/time.Millisecond))
}

func (self *gatedControl) lastRecorded() uint64 {
	self.lock.Lock()
	defer self.lock.Unlock()
	if len(self.recorded) == 0 {
		return 0
	}
	return self.recorded[len(self.recorded)-1]
}

// generationSettings encodes the generation into the send interval so a control
// can report which generation its ticker was last re-tuned to.
func generationSettings(gen uint64) xlink.HeartbeatSettings {
	return xlink.HeartbeatSettings{
		Generation:               gen,
		SendInterval:             time.Duration(gen) * time.Millisecond,
		CheckInterval:            time.Millisecond,
		CloseUnresponsiveTimeout: 5 * time.Minute,
	}
}

func Test_heartbeatControl_OvertakenUpdateCannotDragControlsBack(t *testing.T) {
	// Config changes are notified on independent goroutines, so two updates can
	// be in flight at once. Whichever generation the callbacks end up reporting
	// must be the one the tickers were last re-tuned to: an update that publishes
	// its generation and dispatches separately can be overtaken in between and
	// then retune the controls back to its own intervals.
	req := require.New(t)

	var hc heartbeatControl
	ctrl := newGatedControl()
	hc.SetHeartbeatControl(ctrl, generationSettings(1))

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		hc.UpdateHeartbeat(generationSettings(2))
	}()
	<-ctrl.entered // generation 2 is now inside the dispatch

	thirdDone := make(chan struct{})
	go func() {
		defer close(thirdDone)
		hc.UpdateHeartbeat(generationSettings(3))
	}()

	// Give generation 3 the chance to overtake. With the dispatch serialized it
	// cannot, which is the property under test; without that it runs to
	// completion here and generation 2 then overwrites its intervals.
	select {
	case <-thirdDone:
	case <-time.After(50 * time.Millisecond):
	}

	close(ctrl.gate)
	<-secondDone
	<-thirdDone

	reported := hc.HeartbeatSettings().Generation
	req.Equal(reported, ctrl.lastRecorded(),
		"callbacks report generation %d while the ticker was last re-tuned to %d",
		reported, ctrl.lastRecorded())
	req.Equal(uint64(3), reported, "the newest generation must win")
}

func Test_heartbeatControl_FirstRegistrationAdoptsGenerationZero(t *testing.T) {
	// A router with no managed link config hands out the real defaults carrying
	// generation 0. Treating that as "nothing registered" leaves the link on
	// zero-valued settings, and a zero timeout condemns the first late
	// heartbeat instead of waiting out the configured minute.
	req := require.New(t)

	var hc heartbeatControl
	defaults := settings(0, 10*time.Second, time.Second, time.Minute)
	hc.SetHeartbeatControl(&recordingHeartbeatControl{}, defaults)

	req.Equal(defaults, hc.HeartbeatSettings())
}

func Test_heartbeatControl_LaterChannelReconcilesGenerations(t *testing.T) {
	// A split link binds its payload and ack channels one at a time, so config
	// can move in between and the two arrive carrying different generations.
	// Whichever order that happens in, the link must end up holding the newer
	// generation with every channel tuned to it.
	older := settings(1, 30*time.Second, 5*time.Second, 2*time.Minute)
	newer := settings(2, 2*time.Second, 500*time.Millisecond, 30*time.Second)

	t.Run("second channel is newer", func(t *testing.T) {
		req := require.New(t)
		var hc heartbeatControl
		first, second := &recordingHeartbeatControl{}, &recordingHeartbeatControl{}
		hc.SetHeartbeatControl(first, older)
		hc.SetHeartbeatControl(second, newer)

		req.Equal(newer, hc.HeartbeatSettings())
		req.Equal(newer.SendInterval, first.send, "the already-bound channel must be brought forward")
		req.Equal(newer.CheckInterval, first.check)

		// An update at the adopted generation is a no-op, so registration is the only
		// chance to bring the first channel forward.
		hc.UpdateHeartbeat(newer)
		req.Equal(newer.SendInterval, first.send)
	})

	t.Run("second channel is older", func(t *testing.T) {
		req := require.New(t)
		var hc heartbeatControl
		first, second := &recordingHeartbeatControl{}, &recordingHeartbeatControl{}
		hc.SetHeartbeatControl(first, newer)
		hc.SetHeartbeatControl(second, older)

		req.Equal(newer, hc.HeartbeatSettings(), "an older registration must not roll the link back")
		req.Equal(newer.SendInterval, second.send, "the late channel must be brought up to current")
		req.Equal(newer.CheckInterval, second.check)
		req.Equal(0, first.calls, "and the channel already on that generation is left alone")
	})
}
