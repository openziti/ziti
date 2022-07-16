package events

import (
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/v2/cowslice"
	"github.com/openziti/storage/boltz"
	"time"
)

func AddSessionEventHandler(handler SessionEventHandler) {
	cowslice.Append(sessionEventHandlerRegistry, handler)
}

func RemoveSessionEventHandler(handler SessionEventHandler) {
	cowslice.Delete(sessionEventHandlerRegistry, handler)
}

func AddApiSessionEventHandler(handler ApiSessionEventHandler) {
	cowslice.Append(apiSessionEventHandlerRegistry, handler)
}

func RemoveApiSessionEventHandler(handler ApiSessionEventHandler) {
	cowslice.Delete(apiSessionEventHandlerRegistry, handler)
}

func Init(n *network.Network, dbProvider persistence.DbProvider, stores *persistence.Stores, closeNotify <-chan struct{}) {
	n.GetEventDispatcher().RegisterEventType(ApiSessionEventNS, registerApiSessionEventHandler)
	n.GetEventDispatcher().RegisterEventType(SessionEventNS, registerSessionEventHandler)
	n.GetEventDispatcher().RegisterEventType(EntityCountEventNS, registerEntityCountEventHandler)

	n.GetEventDispatcher().RegisterEventHandlerFactory("file", edgeFileEventLoggerFactory{})
	n.GetEventDispatcher().RegisterEventHandlerFactory("stdout", edgeStdOutLoggerFactory{})

	stores.ApiSession.AddListener(boltz.EventCreate, apiSessionCreated)
	stores.ApiSession.AddListener(boltz.EventDelete, apiSessionDeleted)

	stores.Session.AddListener(boltz.EventCreate, sessionCreated)
	stores.Session.AddListener(boltz.EventDelete, sessionDeleted)

	entityCountEventGenerator = func(interval time.Duration, handler EntityCountEventHandler) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				generateEntityCountEvent(dbProvider, stores, handler)
			case <-closeNotify:
				return
			}
		}
	}
}
