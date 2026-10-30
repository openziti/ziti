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
	"testing"
	"time"

	"github.com/openziti/metrics"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// queueWait is long enough to stand clear of scheduling noise when asserting a recorded queue time.
const queueWait = 50 * time.Millisecond

// blockingEvent holds the subscriber goroutine until released, so later work queues up behind it.
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

type noopEvent struct{}

func (noopEvent) process(*RouterDataModel) {}

// doneEvent signals once everything queued ahead of it has been processed.
type doneEvent struct {
	done chan struct{}
}

func (self doneEvent) process(*RouterDataModel) {
	close(self.done)
}

func newMetricsTestRdm(t *testing.T) (*RouterDataModel, *SubscriberMetrics) {
	closeNotify := make(chan struct{})
	t.Cleanup(func() { close(closeNotify) })

	rdm := NewReceiverRouterDataModel(closeNotify)
	m := NewSubscriberMetrics(metrics.NewRegistry("test", nil))
	rdm.SetSubscriberMetrics(m)
	return rdm, m
}

func (rdm *RouterDataModel) drainSubscriberEvents(t *testing.T) {
	evt := doneEvent{done: make(chan struct{})}
	rdm.queueEvent(evt)
	select {
	case <-evt.done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "subscriber events were not drained")
	}
}

func Test_SubscriberMetrics_EventRecordsProcessingAndQueueTime(t *testing.T) {
	req := require.New(t)
	rdm, m := newMetricsTestRdm(t)

	block := newBlockingEvent()
	rdm.queueEvent(block)
	<-block.started
	rdm.queueEvent(noopEvent{})
	time.Sleep(queueWait)
	close(block.release)
	rdm.drainSubscriberEvents(t)

	// The done event's own timing is recorded after it signals, so wait for it to land.
	req.Eventually(func() bool { return m.Event.Count() == 3 }, time.Second, time.Millisecond)
	req.Equal(int64(3), m.EventQueueTime.Count())
	req.GreaterOrEqual(m.EventQueueTime.Max(), queueWait.Nanoseconds(),
		"the event queued behind the blocked one records the time it waited")
}

func Test_SubscriberMetrics_FullSyncIsNotCountedAsAnEvent(t *testing.T) {
	req := require.New(t)
	rdm, m := newMetricsTestRdm(t)

	rdm.SyncAllSubscribers()
	req.Equal(int64(1), m.FullSync.Count(), "recorded before SyncAllSubscribers returns")

	rdm.drainSubscriberEvents(t)
	req.Eventually(func() bool { return m.Event.Count() >= 1 }, time.Second, time.Millisecond)
	req.Equal(int64(1), m.Event.Count(), "only the drain event counts; the full sync has its own timer")
}

func Test_SubscriberMetrics_ScanRecordsSizeAndQueueTime(t *testing.T) {
	req := require.New(t)
	rdm, m := newMetricsTestRdm(t)

	block := newBlockingEvent()
	rdm.queueEvent(block)
	<-block.started

	rdm.markIdentityForCheck("i1", IdentityUpdated)
	rdm.markIdentityForCheck("i2", IdentityUpdated)
	rdm.requestScan()
	time.Sleep(queueWait)
	close(block.release)

	// The scan's metrics are recorded one after another, so wait for all of them.
	req.Eventually(func() bool {
		return m.Scan.Count() == 1 && m.ScanQueueTime.Count() == 1 && m.ScanIdentities.Count() == 1
	}, 5*time.Second, time.Millisecond)
	req.Equal(int64(2), m.ScanIdentities.Max())
	req.GreaterOrEqual(m.ScanQueueTime.Max(), queueWait.Nanoseconds(),
		"the scan records how long its request waited behind the blocked event")
}

func Test_SubscriberMetrics_NoRecordingWithoutMetrics(t *testing.T) {
	closeNotify := make(chan struct{})
	defer close(closeNotify)
	rdm := NewReceiverRouterDataModel(closeNotify)

	rdm.queueEvent(noopEvent{})
	rdm.markIdentityForCheck("i1", IdentityUpdated)
	rdm.requestScan()
	rdm.SyncAllSubscribers()
	rdm.drainSubscriberEvents(t)
}

// Test_QueueSyncCheck_OverflowRequestsAScan covers the fallback that makes the scan metrics mean
// anything: once the event queue is full, identity checks are marked and scanned instead.
func Test_QueueSyncCheck_OverflowRequestsAScan(t *testing.T) {
	req := require.New(t)

	// No subscriber goroutine, so queued events stay put.
	rdm := &RouterDataModel{
		subscriptions:     cmap.New[*IdentitySubscription](),
		updatedIdentities: cmap.New[byte](),
		events:            make(chan queuedSubscriberEvent, 1),
		scanSubsNotify:    make(chan struct{}, 1),
	}
	rdm.subscriptions.Set("i1", &IdentitySubscription{})
	rdm.subscriptions.Set("i2", &IdentitySubscription{})

	rdm.queueSyncCheck("i1")
	req.Equal(1, rdm.SubscriberEventQueueSize())
	req.Zero(rdm.scanRequestedAt.Load(), "an event with room in the queue needs no scan")
	req.Empty(rdm.scanSubsNotify)

	rdm.queueSyncCheck("i2")
	req.Equal(1, rdm.SubscriberEventQueueSize(), "the full queue took nothing more")
	req.Len(rdm.scanSubsNotify, 1)
	req.True(rdm.updatedIdentities.Has("i2"), "the overflowed identity is marked for the scan")

	firstRequest := rdm.scanRequestedAt.Load()
	req.NotZero(firstRequest)

	time.Sleep(time.Millisecond)
	rdm.requestScan()
	req.Equal(firstRequest, rdm.scanRequestedAt.Load(), "the oldest unserved request is kept")
}
