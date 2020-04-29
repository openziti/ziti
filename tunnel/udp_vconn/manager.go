/*
	Copyright NetFoundry, Inc.

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

package udp_vconn

import (
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/tunnel"
	"github.com/netfoundry/ziti-foundation/util/mempool"
	"github.com/netfoundry/ziti-sdk-golang/ziti"
	"io"
	"net"
	"time"
)

type manager struct {
	eventC           chan Event
	context          ziti.Context
	connMap          map[string]*udpConn
	newConnPolicy    NewConnPolicy
	expirationPolicy ConnExpirationPolicy
}

func (manager *manager) QueueEvent(event Event) {
	manager.eventC <- event
}

func (manager *manager) QueueError(err error) {
	manager.QueueEvent(&errorEvent{err})
}

func (manager *manager) run() {
	log := pfxlog.Logger()
	defer log.Info("shutting down udp listener")

	timer := time.NewTicker(manager.expirationPolicy.PollFrequency())
	defer timer.Stop()

	for {
		select {
		case event, ok := <-manager.eventC:
			if !ok {
				return
			}

			err := event.Handle(manager)
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Errorf("error while handling udp event: %v", err)
			}
		case <-timer.C:
			manager.dropExpired()
		}
	}
}

func (manager *manager) GetWriteQueue(srcAddr net.Addr) WriteQueue {
	pfxlog.Logger().Debugf("Looking up address %v", srcAddr.String())
	result := manager.connMap[srcAddr.String()]
	if result == nil {
		return nil
	}
	return result
}

func (manager *manager) CreateWriteQueue(srcAddr net.Addr, service string, writeConn UDPWriterTo) (WriteQueue, error) {
	switch manager.newConnPolicy.NewConnection(uint32(len(manager.connMap))) {
	case AllowDropLRU:
		manager.dropLRU()
	case Deny:
		return nil, errors.New("max connections exceeded")
	}
	conn := &udpConn{
		readC:     make(chan mempool.PooledBuffer),
		service:   service,
		srcAddr:   srcAddr,
		manager:   manager,
		writeConn: writeConn,
	}
	conn.markUsed()
	manager.connMap[srcAddr.String()] = conn
	pfxlog.Logger().WithField("udpConnId", srcAddr.String()).Debug("created new virtual UDP connection")
	go tunnel.Run(manager.context, service, conn)
	return conn, nil
}

func (manager *manager) dropLRU() {
	if len(manager.connMap) < 1 {
		return
	}
	var oldest *udpConn
	for _, value := range manager.connMap {
		if oldest == nil {
			oldest = value
		} else if oldest.GetLastUsed().After(value.GetLastUsed()) {
			oldest = value
		}
	}
	manager.close(oldest)
}

func (manager *manager) dropExpired() {
	log := pfxlog.Logger()
	now := time.Now()
	for key, value := range manager.connMap {
		if manager.expirationPolicy.IsExpired(now, value.GetLastUsed()) {
			log.WithField("udpConnId", key).Debug("connection expired. removing from UDP vconn manager")
			manager.close(value)
		}
	}
}

func (manager *manager) queueClose(conn *udpConn) {
	// this will likely get called from the event loop, so make sure we don't deadlock
	go manager.QueueEvent(&closeEvent{manager, conn})
}

func (manager *manager) close(conn *udpConn) {
	if !conn.closed {
		conn.closed = true
		delete(manager.connMap, conn.srcAddr.String())
		close(conn.readC)
	}
}

type closeEvent struct {
	manager *manager
	conn    *udpConn
}

func (event *closeEvent) Handle(manager Manager) error {
	event.manager.close(event.conn)
	return nil
}

type errorEvent struct {
	error
}

func (event *errorEvent) Handle(Manager) error {
	return event
}
