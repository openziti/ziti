package router

import (
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/router/env"
)

// LifecycleEvent represents different phases in the router's lifecycle.
// Events are fired during router startup and shutdown to allow external
// components to hook into the router's lifecycle.
type LifecycleEvent int

const (
	// LifecycleEventConfigLoaded is fired when the router's Create() method is called,
	// before the router instance is created. This allows listeners to modify the
	// router configuration before the router is instantiated.
	LifecycleEventConfigLoaded LifecycleEvent = iota

	// LifecycleEventStart is fired when the router's Start() method is called,
	// before any initialization logic begins. This allows listeners to perform
	// setup operations or modify the router configuration before startup.
	LifecycleEventStart
)

// LifecycleListener defines the interface for components that want to receive
// notifications about router lifecycle events. Implementations should be
// thread-safe as they may be called concurrently.
type LifecycleListener interface {
	// OnLifecycleEvent is called when a lifecycle event occurs.
	// The event parameter indicates which lifecycle phase is occurring.
	// The router parameter provides access to the router instance. This will be nil
	// for LifecycleEventConfigLoaded since the router has not been created yet.
	// The config parameter provides access to the router configuration.
	OnLifecycleEvent(event LifecycleEvent, router *Router, config *env.Config)
}

// LifecycleNotifier manages a collection of lifecycle listeners and provides
// methods to notify them of router lifecycle events. It uses a thread-safe
// copy-on-write slice to store listeners, allowing concurrent access during
// event notification.
type LifecycleNotifier struct {
	listeners concurrenz.CopyOnWriteSlice[LifecycleListener]
}

// NewLifecycleNotifier creates a new lifecycle notifier with an empty listener list.
func NewLifecycleNotifier() *LifecycleNotifier {
	return &LifecycleNotifier{}
}

// AddListener registers a new lifecycle listener. The listener will receive
// notifications for all future lifecycle events. This method is thread-safe
// and can be called concurrently with NotifyListeners.
func (ln *LifecycleNotifier) AddListener(listener LifecycleListener) {
	ln.listeners.Append(listener)
}

// NotifyListeners sends a lifecycle event notification to all registered listeners.
// Listeners are called synchronously in the order they were registered.
// If a listener panics, the panic will propagate and prevent subsequent
// listeners from being notified.
func (ln *LifecycleNotifier) NotifyListeners(event LifecycleEvent, router *Router, config *env.Config) {
	for _, listener := range ln.listeners.Value() {
		listener.OnLifecycleEvent(event, router, config)
	}
}

// GlobalLifecycleNotifier is the default lifecycle notifier instance used by
// the router. External components can register listeners with this global
// instance to receive router lifecycle events.
//
// Example usage:
//
//	router.GlobalLifecycleNotifier.AddListener(myListener)
var GlobalLifecycleNotifier = NewLifecycleNotifier()
