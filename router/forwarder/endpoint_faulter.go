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

package forwarder

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
)

// endpointFaultSweepInterval is how often a sender re-checks its pending set without being woken.
// It paces two things: retrying faults a controller would not take, and releasing faults held past
// the retention window. It is therefore well under the retention window, so a fault gets many
// attempts before it is given up on, and expiry runs at a useful granularity.
const endpointFaultSweepInterval = 15 * time.Second

// endpointFaultKey identifies a pending fault. The subject is part of the key because one router
// can hold both ends of a circuit, and those are two distinct faults for the same circuit.
type endpointFaultKey struct {
	circuitId string
	subject   ctrl_pb.FaultSubject
}

// pendingEndpointFault is a fault waiting to be sent. queuedAt is when the fault was first
// reported and is never advanced, so retention measures from when the circuit went away rather
// than from the last attempt.
type pendingEndpointFault struct {
	key      endpointFaultKey
	queuedAt time.Time
}

// endpointFaultSender delivers circuit endpoint faults to one controller.
//
// Faults are held in a set rather than a queue so that a controller which is unreachable
// accumulates at most one entry per circuit, and so that reporting the same fault twice is free.
// One goroutine per controller owns the sending, so a controller that stops reading costs only its
// own sender; reporters never block and never wait for a controller.
//
// That goroutine also expires faults, so its sends are bounded rather than blocking: waiting
// indefinitely on one controller would stop retention from running while new faults kept
// arriving, which is the growth the retention window exists to stop.
type endpointFaultSender struct {
	ctrlId      string
	ctrls       env.NetworkControllers
	retention   time.Duration
	sendTimeout time.Duration

	lock    sync.Mutex
	pending map[endpointFaultKey]*pendingEndpointFault

	notify      chan struct{}
	stop        chan struct{}
	isStopped   atomic.Bool
	closeNotify <-chan struct{}

	sent    metrics.Meter
	expired metrics.Meter
}

func newEndpointFaultSender(ctrlId string, f *Faulter) *endpointFaultSender {
	result := &endpointFaultSender{
		ctrlId:      ctrlId,
		ctrls:       f.ctrls,
		retention:   f.endpointFaultRetention,
		sendTimeout: f.ctrls.DefaultRequestTimeout(),
		pending:     map[endpointFaultKey]*pendingEndpointFault{},
		notify:      make(chan struct{}, 1),
		stop:        make(chan struct{}),
		closeNotify: f.closeNotify,
		sent:        f.endpointFaultsSent,
		expired:     f.endpointFaultsExpired,
	}

	go result.run()

	return result
}

// report adds a fault to the pending set and wakes the sender. It does not block.
func (self *endpointFaultSender) report(key endpointFaultKey) {
	self.lock.Lock()
	if _, exists := self.pending[key]; !exists {
		self.pending[key] = &pendingEndpointFault{key: key, queuedAt: time.Now()}
	}
	self.lock.Unlock()

	self.wake()
}

func (self *endpointFaultSender) wake() {
	select {
	case self.notify <- struct{}{}:
	default:
	}
}

// shutdown stops the sender and abandons anything it still holds. Used when a controller is
// removed from the cluster, whose circuits are gone with it.
func (self *endpointFaultSender) shutdown() {
	if self.isStopped.CompareAndSwap(false, true) {
		close(self.stop)
	}
}

// hasPending return true if there are pending faults
func (self *endpointFaultSender) hasPending() bool {
	self.lock.Lock()
	defer self.lock.Unlock()
	return len(self.pending) > 0
}

func (self *endpointFaultSender) run() {
	log := pfxlog.Logger().WithField("ctrlId", self.ctrlId)
	log.Info("endpoint fault sender started")
	defer log.Info("endpoint fault sender exited")

	ticker := time.NewTicker(endpointFaultSweepInterval)
	defer ticker.Stop()

	for {
		self.expirePending()

		// Only a completed drain justifies going straight round again, and then only because
		// faults reported while it ran are already waiting. A drain cut short by a send failure
		// must wait, or an undeliverable fault spins this goroutine retrying as fast as the
		// failure comes back.
		if self.sendPending() && self.hasPending() {
			select {
			case <-self.stop:
				return
			case <-self.closeNotify:
				return
			default:
				continue
			}
		}

		select {
		case <-self.notify:
		case <-ticker.C:
		case <-self.stop:
			return
		case <-self.closeNotify:
			return
		}
	}
}

// expirePending drops faults held longer than the retention window. Expiry means the controller
// keeps a circuit this router no longer has, so it is reported as an error rather than logged
// quietly.
func (self *endpointFaultSender) expirePending() {
	if self.retention <= 0 {
		return
	}

	cutoff := time.Now().Add(-self.retention)

	self.lock.Lock()
	var expired []endpointFaultKey
	for key, fault := range self.pending {
		if fault.queuedAt.Before(cutoff) {
			expired = append(expired, key)
			delete(self.pending, key)
		}
	}
	self.lock.Unlock()

	for _, key := range expired {
		self.expired.Mark(1)
		pfxlog.Logger().
			WithField("ctrlId", self.ctrlId).
			WithField("circuitId", key.circuitId).
			WithField("subject", key.subject.String()).
			WithField("retention", self.retention).
			Error("giving up on circuit fault, controller may still hold the circuit")
	}
}

// sendPending delivers what it can and stops at the first failure, reporting whether it got
// through the whole set. A failure means the channel is unavailable, so the rest would fail too;
// they stay pending until the controller reconnects or the sweep comes round.
//
// Faults are removed as they are sent rather than taken out of the set up front, so the set
// always reflects what is still owed. Draining into a local copy would hide in-flight faults from
// expirePending, letting them age past the retention window unnoticed, and would make the set
// briefly look empty to anything else reading it.
func (self *endpointFaultSender) sendPending() bool {
	for _, fault := range self.snapshotPending() {
		select {
		case <-self.stop:
			return false
		case <-self.closeNotify:
			return false
		default:
		}

		if !self.send(fault) {
			return false
		}

		self.lock.Lock()
		delete(self.pending, fault.key)
		self.lock.Unlock()
	}

	return true
}

func (self *endpointFaultSender) snapshotPending() []*pendingEndpointFault {
	self.lock.Lock()
	defer self.lock.Unlock()

	result := make([]*pendingEndpointFault, 0, len(self.pending))
	for _, fault := range self.pending {
		result = append(result, fault)
	}
	return result
}

// send reports whether the fault was handed to the controller channel. A fault that could not be
// sent is left pending for a later attempt.
func (self *endpointFaultSender) send(fault *pendingEndpointFault) bool {
	ctrlCh := self.ctrls.GetCtrlChannel(self.ctrlId)
	if ctrlCh == nil {
		// The controller is not connected yet, or is gone. If it was removed, shutdown clears
		// this set; otherwise reconnecting wakes the sender.
		return false
	}

	msg := &ctrl_pb.Fault{Subject: fault.key.subject, Id: fault.key.circuitId}

	// Waits for the write rather than for the queue to accept it: the sender declines to retry a
	// failed write, so treating queue acceptance as delivery would drop the fault while reporting
	// success. The timeout bounds both stages, so a controller that is not draining delays this
	// sender by at most one timeout before it expires and retries.
	//
	// A successful write still is not proof the controller processed it; faults carry no reply, so
	// this is the strongest confirmation available.
	envelope := protobufs.MarshalTyped(msg).WithTimeout(self.sendTimeout)
	if err := envelope.SendAndWaitForWire(ctrlCh.GetDefaultSender()); err != nil {
		pfxlog.Logger().
			WithError(err).
			WithField("ctrlId", self.ctrlId).
			WithField("circuitId", fault.key.circuitId).
			WithField("subject", fault.key.subject.String()).
			Debug("could not send circuit fault, will retry")
		return false
	}

	self.sent.Mark(1)
	pfxlog.Logger().
		WithField("ctrlId", self.ctrlId).
		WithField("circuitId", fault.key.circuitId).
		WithField("subject", fault.key.subject.String()).
		Debug("reported circuit fault")
	return true
}
