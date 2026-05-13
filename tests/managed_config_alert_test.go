//go:build apitests

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

package tests

import (
	"sync"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/controller/event"
)

// recordingAlertHandler captures alert events into a channel for tests.
type recordingAlertHandler struct {
	mu     sync.Mutex
	events []*event.AlertEvent
	ch     chan *event.AlertEvent
}

func newRecordingAlertHandler(buffer int) *recordingAlertHandler {
	return &recordingAlertHandler{ch: make(chan *event.AlertEvent, buffer)}
}

func (r *recordingAlertHandler) AcceptAlertEvent(e *event.AlertEvent) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
	select {
	case r.ch <- e:
	default:
		// buffer full; tests should size the buffer big enough
	}
}

// waitFor returns the next alert satisfying match, or fails the test on timeout.
func (r *recordingAlertHandler) waitFor(t *testing.T, timeout time.Duration, match func(*event.AlertEvent) bool) *event.AlertEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case e := <-r.ch:
			if match(e) {
				return e
			}
		case <-deadline:
			t.Fatalf("timed out waiting for matching alert event after %s", timeout)
			return nil
		}
	}
}

// Test_ManagedConfigAlert_EndToEnd exercises the full alert path: a
// router's managedconfig.Registry triggers an alert (handler.Apply fails
// to parse the JSON payload) → alert.Reporter queues + sends a
// ctrl_pb.Alert → controller's handler_ctrl/alert.go receives →
// event.Dispatcher.AcceptAlertEvent fires → test handler records.
//
// Uses ApplyController directly on the running router instead of going
// through controller-side Config CRUD + RDM propagation. That's adequate
// for the alert wiring; end-to-end "controller pushes bad config" is
// covered by tests that exercise the RDM path.
func Test_ManagedConfigAlert_EndToEnd(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	router := ctx.CreateEnrollAndStartEdgeRouter()

	handler := newRecordingAlertHandler(16)
	dispatcher := ctx.fabricController.GetNetwork().GetEventDispatcher()
	dispatcher.AddAlertEventHandler(handler)
	defer dispatcher.RemoveAlertEventHandler(handler)

	// Malformed JSON guarantees handler.Apply fails at parse → "initial
	// apply failed" path in the registry → AlertCallback fires.
	err := router.Router.GetRouterConfigRegistry().ApplyController("router.link.v1", `not even close to json`)
	ctx.Req.NoError(err, "ApplyController returns nil; the failure happens asynchronously in reconcile")

	alert := handler.waitFor(t, 10*time.Second, func(e *event.AlertEvent) bool {
		return e.RelatedEntities["configBaseType"] == "router.link"
	})

	ctx.Req.Equal("error", alert.Severity)
	ctx.Req.Contains(alert.Message, "router.link", "alert message should reference base type")
	ctx.Req.Equal(router.Router.GetRouterId().Token, alert.AlertSourceId, "alert source should be the router that emitted it")
}
