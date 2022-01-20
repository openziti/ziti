package event

import "github.com/sirupsen/logrus"

type Dispatcher interface {
	Dispatch(event Event)
}

type Event interface {
	Handle()
}

func NewDispatcher(closeNotify <-chan struct{}) Dispatcher {
	result := &dispatcherImpl{
		closeNotify: closeNotify,
		eventC:      make(chan Event, 25),
	}

	go result.eventLoop()
	return result
}

type dispatcherImpl struct {
	closeNotify <-chan struct{}
	eventC      chan Event
}

func (dispatcher *dispatcherImpl) eventLoop() {
	logrus.Info("event dispatcher: started")
	defer logrus.Info("event dispatcher: stopped")

	for {
		select {
		case event := <-dispatcher.eventC:
			event.Handle()
		case <-dispatcher.closeNotify:
			return
		}
	}
}

func (dispatcher *dispatcherImpl) Dispatch(event Event) {
	select {
	case dispatcher.eventC <- event:
	case <-dispatcher.closeNotify:
	}
}
