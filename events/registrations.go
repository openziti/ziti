package events

import (
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/v2/cowslice"
	"github.com/pkg/errors"
	"reflect"
)

func init() {
	RegisterEventType("metrics", registerMetricsEventHandler)
	RegisterEventType("services", registerServiceEventHandler)
	RegisterEventType("fabric.usage", registerUsageEventHandler)
	RegisterEventType("fabric.circuits", registerCircuitEventHandler)
	RegisterEventType("fabric.terminators", registerTerminatorEventHandler)
	RegisterEventType("fabric.routers", registerRouterEventHandler)
	RegisterEventType("fabric.traces", func(val interface{}, _ map[interface{}]interface{}) error {
		handler, ok := val.(trace.EventHandler)
		if !ok {
			return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/trace/EventHandler interface.", reflect.TypeOf(val))
		}
		AddTraceEventHandler(handler)
		return nil
	})
}

func AddCircuitEventHandler(handler network.CircuitEventHandler) {
	cowslice.Append(network.CircuitEventHandlerRegistry, handler)
}

func RemoveCircuitEventHandler(handler network.CircuitEventHandler) {
	cowslice.Delete(network.CircuitEventHandlerRegistry, handler)
}

func AddTraceEventHandler(handler trace.EventHandler) {
	cowslice.Append(trace.EventHandlerRegistry, handler)
}

func RemoveTraceEventHandler(handler trace.EventHandler) {
	cowslice.Delete(trace.EventHandlerRegistry, handler)
}

func AddTerminatorEventHandler(handler TerminatorEventHandler) {
	cowslice.Append(terminatorEventHandlerRegistry, handler)
}

func RemoveTerminatorEventHandler(handler TerminatorEventHandler) {
	cowslice.Delete(terminatorEventHandlerRegistry, handler)
}

func AddServiceEventHandler(handler ServiceEventHandler) {
	cowslice.Append(serviceEventHandlerRegistry, handler)
}

func RemoveServiceEventHandler(handler ServiceEventHandler) {
	cowslice.Delete(serviceEventHandlerRegistry, handler)
}

func AddRouterEventHandler(handler RouterEventHandler) {
	cowslice.Append(routerEventHandlerRegistry, handler)
}

func RemoveRouterEventHandler(handler RouterEventHandler) {
	cowslice.Delete(routerEventHandlerRegistry, handler)
}
