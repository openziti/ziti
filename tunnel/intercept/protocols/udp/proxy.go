/*
	Copyright 2019 Netfoundry, Inc.

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

package udp

import (
	"fmt"
	"github.com/netfoundry/ziti-edge/tunnel/udp_vconn"
	"github.com/netfoundry/ziti-sdk-golang/ziti"
	"github.com/netfoundry/ziti-sdk-golang/ziti/edge"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"sync"
)

var log = logrus.StandardLogger()

// packet queues (outbound) keyed by client address
type udpQItem struct {
	segment      UDP // the actual segment
	srcIP, dstIP net.IP
	tunManager   *TunUDPManager
	release      func() // a closure that returns the underlying packet to the rx packet pool when called.
}

func (item *udpQItem) GetPayload() []byte {
	return item.segment.Payload()
}

func (item *udpQItem) Release() {
	item.release()
}

func NewManager(dev io.ReadWriter) *TunUDPManager {
	return &TunUDPManager{
		dev:      dev,
		services: make(map[string]string),
	}
}

type TunUDPManager struct {
	dev         io.ReadWriter
	services    map[string]string
	servicesMtx sync.Mutex
}

func (manager *TunUDPManager) RegisterService(service edge.Service, interceptIP net.IP) {
	addr := &net.UDPAddr{IP: interceptIP, Port: service.Dns.Port}
	key := addr.String()
	log.Infof("registered udp service %s", key)
	manager.servicesMtx.Lock()
	defer manager.servicesMtx.Unlock()
	_, exists := manager.services[key]
	if !exists {
		manager.services[key] = service.Name
	}
}

func (manager *TunUDPManager) UnregisterService(service string) {
	var key string
	manager.servicesMtx.Lock()
	defer manager.servicesMtx.Unlock()

	for addr, svcName := range manager.services {
		if svcName == service {
			key = addr
			break
		}
	}
	if key == "" {
		log.Warnf("service %s is not registered", service)
		return
	}

	delete(manager.services, key)
}

func (manager *TunUDPManager) CreateEvent(context ziti.Context, srcIP, dstIP net.IP, pdu []byte, dev io.ReadWriter, release func()) udp_vconn.Event {
	udpQItem := &udpQItem{
		segment:    UDP(pdu),
		srcIP:      srcIP,
		dstIP:      dstIP,
		tunManager: manager,
		release:    release,
	}
	return udpQItem
}

func (item *udpQItem) Handle(proxyManager udp_vconn.Manager) error {
	tunManager := item.tunManager
	segment := item.segment

	srcAddr := &net.UDPAddr{IP: item.srcIP, Port: int(segment.SourcePort())}

	writeQueue := proxyManager.GetWriteQueue(srcAddr)
	if writeQueue == nil {
		dstAddr := &net.UDPAddr{IP: item.dstIP, Port: int(segment.DestinationPort())}
		svcKey := dstAddr.String()
		tunManager.servicesMtx.Lock()
		defer tunManager.servicesMtx.Unlock()
		service, found := tunManager.services[svcKey]
		if !found {
			return fmt.Errorf("no service found for intercept %v", svcKey)
		}

		clientConn, err := NewClientConn(svcKey, tunManager.dev)
		if err != nil {
			return err
		}
		writeQueue, err = proxyManager.CreateWriteQueue(srcAddr, service, clientConn)
		if err != nil {
			return err
		}
	}

	log.Infof("received %v bytes for manager %v -> %v", len(segment.Payload()), writeQueue.LocalAddr(), writeQueue.Service())
	writeQueue.Accept(item)

	return nil
}
