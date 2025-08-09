package router

import (
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/router/env"
)

type LifecycleEvent int

const (
	LifecycleEventStart LifecycleEvent = iota
)

type LifecycleListener interface {
	OnLifecycleEvent(event LifecycleEvent, env env.RouterEnv)
}

type LifecycleNotifier struct {
	listeners concurrenz.CopyOnWriteSlice[LifecycleListener]
}

func NewLifecycleNotifier() *LifecycleNotifier {
	return &LifecycleNotifier{}
}

func (ln *LifecycleNotifier) AddListener(listener LifecycleListener) {
	ln.listeners.Append(listener)
}

func (ln *LifecycleNotifier) NotifyListeners(event LifecycleEvent, env env.RouterEnv) {
	for _, listener := range ln.listeners.Value() {
		listener.OnLifecycleEvent(event, env)
	}
}

var GlobalLifecycleNotifier = NewLifecycleNotifier()
