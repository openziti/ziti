package event

import (
	"github.com/openziti/foundation/util/cowslice"
	errors "github.com/pkg/errors"
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
