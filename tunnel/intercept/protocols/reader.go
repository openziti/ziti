// +build linux

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

package protocols

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/intercept/protocols/ip"
	"github.com/openziti/edge/tunnel/intercept/protocols/tcp"
	"github.com/openziti/edge/tunnel/intercept/protocols/udp"
	"github.com/openziti/edge/tunnel/udp_vconn"
	"github.com/sirupsen/logrus"
	"io"
)

type rxPacket struct {
	data    []byte
	release func()
}

// Read packets from the tun interface
func HandleInboundPackets(provider tunnel.FabricProvider, dev io.ReadWriter, mtu uint, udpManager *udp.TunUDPManager) {
	log := pfxlog.Logger()

	// initialize a buffer pool for incoming packets
	// TEST with small MTU
	rxq := make(chan *rxPacket, 16)
	for i := 0; i < cap(rxq); i++ {
		packet := rxPacket{
			data: make([]byte, mtu),
		}
		packet.release = func() {
			rxq <- &packet
		}
		rxq <- &packet
	}

	vconnMgr := udp_vconn.NewManager(provider, udp_vconn.NewUnlimitedConnectionPolicy(), udp_vconn.NewDefaultExpirationPolicy())

	for {
		packet := <-rxq
		n, err := dev.Read(packet.data)
		if err != nil {
			log.Fatal("failed reading from tun: ", err)
		}

		src, dst, proto, payload, err := ip.Decode(packet.data)
		if err != nil {
			log.Error("failed parsing layer 3 packet: ", err)
		}

		log.WithFields(logrus.Fields{"src": src, "dst": dst, "proto": proto}).Debugf("read %d bytes", n)
		enqueued := false
		switch proto {
		case tcp.TCPProtocolNumber:
			enqueued = tcp.Enqueue(provider, src, dst, payload, dev, mtu, packet.release)
		case udp.ProtocolNumber:
			log.Infof("Received udp packet %v, %v", src, dst)
			event := udpManager.CreateEvent(src, dst, payload, packet.release)
			vconnMgr.QueueEvent(event)
			enqueued = true
		default:
			log.Errorf("protocol %d is not supported", proto)
		}

		if !enqueued {
			log.Debug("dropping packet!") // TODO better logging
			packet.release()
		}
	}
}
