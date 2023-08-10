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

package events

import (
	"github.com/openziti/foundation/v2/cowslice"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

type testEvent struct {
	val int
}

func (event *testEvent) Handle() {
	for _, handler := range getTestHandlers() {
		handler(event)
	}
}

type testEventHandler func(*testEvent)

var testHandlers = cowslice.NewCowSlice(make([]testEventHandler, 0))

func getTestHandlers() []testEventHandler {
	return testHandlers.Value().([]testEventHandler)
}

func addTestHandler(handler testEventHandler) {
	cowslice.Append(testHandlers, handler)
}

func TestDispatcherImpl_Dispatch(t *testing.T) {
	closeNotify := make(chan struct{})
	defer close(closeNotify)
	dispatcher := NewDispatcher(closeNotify)

	latch := sync.WaitGroup{}
	latch.Add(1000)

	errC := make(chan error, 1)

	current := 0

	handler := func(event *testEvent) {
		if current != event.val {
			select {
			case errC <- errors.Errorf("expected %v, got %v", current, event.val):
			default:
			}
		}
		current++
		latch.Done()
	}

	addTestHandler(handler)

	for i := 0; i < 1000; i++ {
		dispatcher.Dispatch(&testEvent{val: i})
	}

	doneC := make(chan struct{})
	go func() {
		latch.Wait()
		close(doneC)
	}()

	req := require.New(t)

	select {
	case <-doneC:
	case <-time.After(time.Second):
		req.Fail("timed out waiting for all events to be processed")
	}

	select {
	case err := <-errC:
		req.NoError(err)
	default:
	}
}
