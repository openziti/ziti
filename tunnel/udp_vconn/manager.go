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
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/foundation/util/mempool"
	"io"
	"net"
	"strconv"
	"time"
)

type manager struct {
	eventC           chan Event
	provider         tunnel.FabricProvider
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
				log.Errorf("EOF detected. stopping UDP event loop")
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

func (manager *manager) CreateWriteQueue(targetAddr *net.UDPAddr, srcAddr net.Addr, service *entities.Service, writeConn UDPWriterTo) (WriteQueue, error) {
	switch manager.newConnPolicy.NewConnection(uint32(len(manager.connMap))) {
	case AllowDropLRU:
		manager.dropLRU()
	case Deny:
		return nil, errors.New("max connections exceeded")
	}
	conn := &udpConn{
		readC:       make(chan mempool.PooledBuffer),
		closeNotify: make(chan struct{}),
		service:     service.Name,
		srcAddr:     srcAddr,
		manager:     manager,
		writeConn:   writeConn,
	}
	conn.markUsed()
	manager.connMap[srcAddr.String()] = conn
	pfxlog.Logger().WithField("udpConnId", srcAddr.String()).Debug("created new virtual UDP connection")

	sourceAddr := service.GetSourceAddr(srcAddr, targetAddr)
	appInfo := tunnel.GetAppInfo("udp", "", targetAddr.IP.String(), strconv.Itoa(targetAddr.Port), sourceAddr)
	identity := service.GetDialIdentity(srcAddr, targetAddr)
	go tunnel.DialAndRun(manager.provider, service, identity, conn, appInfo, false)
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
	for key, conn := range manager.connMap {
		if conn.closed.Get() {
			delete(manager.connMap, conn.srcAddr.String())
		}
		if manager.expirationPolicy.IsExpired(now, conn.GetLastUsed()) {
			log.WithField("udpConnId", key).Debug("connection expired. removing from UDP vconn manager")
			manager.close(conn)
		}
	}
}

func (manager *manager) close(conn *udpConn) {
	_ = conn.Close()
	delete(manager.connMap, conn.srcAddr.String())
}

type errorEvent struct {
	error
}

func (event *errorEvent) Handle(Manager) error {
	return event
}
