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

package common

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/openziti/metrics"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// queueWait is how long a test holds the subscriber goroutine, so queued work accrues a queue time
// well clear of scheduling noise.
const queueWait = 50 * time.Millisecond

// blockingEvent holds the subscriber goroutine until released, so later work queues up behind it.
// Once it has started, every event queued before it has been processed and recorded, and its own
// queue time is recorded but not its processing time.
type blockingEvent struct {
	started chan struct{}
	release chan struct{}
}

func newBlockingEvent() *blockingEvent {
	return &blockingEvent{started: make(chan struct{}), release: make(chan struct{})}
}

func (self *blockingEvent) process(*RouterDataModel) {
	close(self.started)
	<-self.release
}

// waitStarted waits up to 5s for the subscriber goroutine to start processing the event.
func (self *blockingEvent) waitStarted() error {
	select {
	case <-self.started:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("timed out waiting for the blocking event to start")
	}
}

type noopEvent struct{}

func (noopEvent) process(*RouterDataModel) {}

// metricsTestRdm is a receiver data model that records to its own subscriber metrics.
type metricsTestRdm struct {
	*RouterDataModel
	metrics     *SubscriberMetrics
	closeNotify chan struct{}
}

// newMetricsTestRdm returns a running receiver data model recording to fresh subscriber metrics. The
// caller closes it.
func newMetricsTestRdm() *metricsTestRdm {
	closeNotify := make(chan struct{})
	rdm := NewReceiverRouterDataModel(closeNotify)
	m := NewSubscriberMetrics(metrics.NewRegistry("test", nil))
	rdm.SetSubscriberMetrics(m)
	return &metricsTestRdm{RouterDataModel: rdm, metrics: m, closeNotify: closeNotify}
}

// Close stops the data model's subscriber goroutine.
func (self *metricsTestRdm) Close() {
	close(self.closeNotify)
}

// waitForScans waits up to 5s for m to record count scans. A scan has no completion signal, so this
// polls ScanIdentities, the last metric a scan records.
func waitForScans(m *SubscriberMetrics, count int64) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(5 * time.Second)
	for m.ScanIdentities.Count() < count {
		select {
		case <-ticker.C:
		case <-deadline:
			return fmt.Errorf("recorded %d scans, waited for %d", m.ScanIdentities.Count(), count)
		}
	}
	return nil
}

func Test_SubscriberMetrics(t *testing.T) {
	t.Run("an event records its processing time and how long it waited in the queue", func(t *testing.T) {
		req := require.New(t)

		// arrange: hold the subscriber goroutine so an event queues behind it
		rdm := newMetricsTestRdm()
		defer rdm.Close()
		block := newBlockingEvent()
		rdm.queueEvent(block)
		req.NoError(block.waitStarted())

		// act
		rdm.queueEvent(noopEvent{})
		queued := time.Now()
		time.Sleep(queueWait)
		held := time.Since(queued)
		close(block.release)
		fence := newBlockingEvent()
		defer close(fence.release)
		rdm.queueEvent(fence)
		err := fence.waitStarted()

		// assert
		req.NoError(err)
		req.Equal(int64(2), rdm.metrics.Event.Count(), "the blocking and queued events")
		req.Equal(int64(3), rdm.metrics.EventQueueTime.Count(), "the blocking, queued and fence events")
		req.GreaterOrEqual(rdm.metrics.EventQueueTime.Max(), held.Nanoseconds(),
			"the event queued behind the blocked one records the time it waited")
	})

	t.Run("a full sync records its own timer and is not counted as an event", func(t *testing.T) {
		req := require.New(t)

		// arrange
		rdm := newMetricsTestRdm()
		defer rdm.Close()

		// act
		rdm.SyncAllSubscribers()
		fullSyncsOnReturn := rdm.metrics.FullSync.Count()
		fence := newBlockingEvent()
		defer close(fence.release)
		rdm.queueEvent(fence)
		err := fence.waitStarted()

		// assert
		req.NoError(err)
		req.Equal(int64(1), fullSyncsOnReturn, "the full sync is recorded before SyncAllSubscribers returns")
		req.Zero(rdm.metrics.Event.Count(), "the full sync has its own timer")
		req.Equal(int64(2), rdm.metrics.EventQueueTime.Count(), "the full sync and fence events")
	})

	t.Run("a scan records how many identities it processed and how long its request waited", func(t *testing.T) {
		req := require.New(t)

		// arrange: hold the subscriber goroutine so the scan request waits
		rdm := newMetricsTestRdm()
		defer rdm.Close()
		block := newBlockingEvent()
		rdm.queueEvent(block)
		req.NoError(block.waitStarted())
		rdm.markIdentityForCheck("i1", IdentityUpdated)
		rdm.markIdentityForCheck("i2", IdentityUpdated)

		// act
		rdm.requestScan()
		requested := time.Now()
		time.Sleep(queueWait)
		held := time.Since(requested)
		close(block.release)
		err := waitForScans(rdm.metrics, 1)

		// assert
		req.NoError(err)
		req.Equal(int64(1), rdm.metrics.Scan.Count())
		req.Equal(int64(1), rdm.metrics.ScanQueueTime.Count())
		req.Equal(int64(2), rdm.metrics.ScanIdentities.Max())
		req.GreaterOrEqual(rdm.metrics.ScanQueueTime.Max(), held.Nanoseconds(),
			"the scan records how long its request waited behind the blocked event")
	})

	t.Run("a data model without metrics processes events, scans and full syncs", func(t *testing.T) {
		req := require.New(t)

		// arrange
		closeNotify := make(chan struct{})
		defer close(closeNotify)
		rdm := NewReceiverRouterDataModel(closeNotify)

		// act
		rdm.queueEvent(noopEvent{})
		rdm.markIdentityForCheck("i1", IdentityUpdated)
		rdm.requestScan()
		rdm.SyncAllSubscribers()
		fence := newBlockingEvent()
		defer close(fence.release)
		rdm.queueEvent(fence)
		err := fence.waitStarted()

		// assert
		req.NoError(err, "the subscriber goroutine processed everything queued before the fence")
	})
}

// Test_SubscriberQueueOverflow covers the fallback that makes the scan metrics mean anything: once the
// event queue is full, identity checks are marked and scanned instead. None of these data models
// runs a subscriber goroutine, so queued events stay put.
func Test_SubscriberQueueOverflow(t *testing.T) {
	t.Run("a check with room in the queue is queued as an event and requests no scan", func(t *testing.T) {
		req := require.New(t)

		// arrange
		rdm := &RouterDataModel{
			subscriptions:     cmap.New[*IdentitySubscription](),
			updatedIdentities: cmap.New[byte](),
			events:            make(chan queuedSubscriberEvent, 1),
			scanSubsNotify:    make(chan struct{}, 1),
		}
		rdm.subscriptions.Set("i1", &IdentitySubscription{})

		// act
		rdm.queueSyncCheck("i1")

		// assert
		req.Equal(1, rdm.SubscriberEventQueueSize())
		req.Zero(rdm.scanRequestedAt.Load())
		req.Empty(rdm.scanSubsNotify)
	})

	t.Run("a check on a full queue marks the identity and requests a scan", func(t *testing.T) {
		req := require.New(t)

		// arrange
		rdm := &RouterDataModel{
			subscriptions:     cmap.New[*IdentitySubscription](),
			updatedIdentities: cmap.New[byte](),
			events:            make(chan queuedSubscriberEvent, 1),
			scanSubsNotify:    make(chan struct{}, 1),
		}
		rdm.subscriptions.Set("i1", &IdentitySubscription{})
		rdm.subscriptions.Set("i2", &IdentitySubscription{})
		rdm.queueSyncCheck("i1")

		// act
		rdm.queueSyncCheck("i2")

		// assert
		req.Equal(1, rdm.SubscriberEventQueueSize(), "the full queue took nothing more")
		req.True(rdm.updatedIdentities.Has("i2"), "the overflowed identity is marked for the scan")
		req.Len(rdm.scanSubsNotify, 1)
		req.NotZero(rdm.scanRequestedAt.Load())
	})

	t.Run("a scan request keeps the time of the oldest unserved request", func(t *testing.T) {
		req := require.New(t)

		// arrange
		rdm := &RouterDataModel{
			scanSubsNotify: make(chan struct{}, 1),
		}
		rdm.scanRequestedAt.Store(1)

		// act
		rdm.requestScan()

		// assert
		req.Equal(int64(1), rdm.scanRequestedAt.Load())
		req.Len(rdm.scanSubsNotify, 1)
	})
}
