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

package tcp

import (
	"fmt"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"sync"
)

var log = logrus.StandardLogger()

// packet queues (outbound) keyed by client address
type tcpQItem struct {
	segment TCP    // the actual segment
	release func() // a closure that returns the underlying packet to the rx packet pool when called.
}

var queues = make(map[string]chan *tcpQItem)
var queuesMtx = sync.Mutex{}

// handled service names keyed by intercept address
var services = make(map[string]string)
var servicesMtx = sync.Mutex{}

func key(ip net.IP, port int) string {
	return fmt.Sprintf("%s:%d", ip.String(), port)
}

func RegisterService(service *entities.Service, interceptIP net.IP) {
	key := key(interceptIP, service.ClientConfig.Port)
	servicesMtx.Lock()
	defer servicesMtx.Unlock()
	_, exists := services[key]
	if !exists {
		services[key] = service.Name
	}
}

func UnregisterService(serviceName string) {
	var key string
	servicesMtx.Lock()
	defer servicesMtx.Unlock()

	for addr, svcName := range services {
		if svcName == serviceName {
			key = addr
			break
		}
	}
	if key == "" {
		log.Warnf("service %s is not registered", serviceName)
		return
	}

	delete(services, key)
}

func Enqueue(context ziti.Context, srcIP, dstIP net.IP, pdu []byte, dev io.ReadWriter, tunMTU uint, release func()) bool {
	segment := TCP(pdu)
	svcKey := key(dstIP, int(segment.DestinationPort()))
	service := services[svcKey]
	if service == "" {
		log.Debugf("no service registered for %s", svcKey)
		return false
	}

	clientKey := key(srcIP, int(segment.SourcePort()))
	queuesMtx.Lock()
	defer queuesMtx.Unlock()
	queue := queues[clientKey]
	if queue == nil {
		if segment.HasFlags(TCPFlagSyn) {
			queue = make(chan *tcpQItem, 64)
			queues[clientKey] = queue
			clientConn, err := NewClientConn(clientKey, svcKey, queue, dev, tunMTU)
			if err != nil {
				log.Errorf("failed to create client connection for %s, %s: %v", clientKey, svcKey, err)
				release()
				return true // this packet is effectively handled here, even though we're dropping it
			}
			go tunnel.DialAndRun(context, service, clientConn, true)
		} else {
			log.Debugf("packet from %s lacks TCP syn", clientKey)
			release()
			return true
		}
	}

	var x tcpQItem
	x.release = release
	x.segment = segment
	queue <- &x
	return true
}
