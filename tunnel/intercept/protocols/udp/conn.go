/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-edge/tunnel/intercept/protocols/ip"
	"github.com/sirupsen/logrus"
	"io"
	"net"
)

const (
	maxPacketSize  = 65535
	maxPayloadSize = 65507

	ProtocolNumber int = 17
)

type ClientConn struct {
	net.Conn

	svcKey  string
	dstAddr *net.UDPAddr
	txq     chan []byte // queue of layer 3 packets for peer
	dev     io.ReadWriter

	log *logrus.Entry
}

func NewClientConn(interceptAddr string, dev io.ReadWriter) (*ClientConn, error) {
	dstAddr, err := net.ResolveUDPAddr("udp", interceptAddr)
	if err != nil {
		return nil, err
	}
	txq := make(chan []byte, 16)
	for i := 0; i < cap(txq); i++ {
		txq <- make([]byte, maxPacketSize)
	}
	return &ClientConn{
		svcKey:  interceptAddr,
		dstAddr: dstAddr,
		txq:     txq,
		dev:     dev,
		log:     log.WithFields(logrus.Fields{"dst": dstAddr.String()}),
	}, nil
}

// Write to the local client.
func (conn *ClientConn) udpSend(payload []byte, addr *net.UDPAddr) {
	dataLen := len(payload)
	log.Infof("Getting next packet from pool")
	packet := <-conn.txq
	log.Infof("Received packet from pool")
	defer func() {
		conn.txq <- packet
	}()
	packet = packet[:ip.IPv4MinimumSize+headerSize+dataLen]
	ipHeader := ip.IPv4(packet[0:ip.IPv4MinimumSize])
	ipHeader.Encode(&ip.IPv4Fields{
		IHL:         ip.IPv4MinimumSize,
		TotalLength: uint16(len(packet)),
		Protocol:    uint8(ProtocolNumber),
		SrcAddr:     conn.dstAddr.IP.To4(),
		DstAddr:     addr.IP.To4(),
		TTL:         64,
	})
	ipHeader.SetChecksum(^ipHeader.CalculateChecksum())

	udpHeader := UDP(packet[ip.IPv4MinimumSize : ip.IPv4MinimumSize+headerSize])
	udpHeader.Encode(&udpFields{
		SrcPort:  uint16(conn.dstAddr.Port),
		DstPort:  uint16(addr.Port),
		Length:   headerSize + uint16(dataLen),
		Checksum: 0,
	})

	sum := ip.PseudoHeaderChecksum(ProtocolNumber, conn.dstAddr.IP.To4(), addr.IP.To4())
	sum = ip.ChecksumCombine(sum, udpHeader.Length()) // need to UDP length field to IP pseudo header
	sum = ip.Checksum(payload, sum)
	sum = ip.Checksum(udpHeader, sum)
	udpHeader.SetChecksum(^sum)

	copy(packet[ip.IPv4MinimumSize+headerSize:], payload)
	conn.log.Infof("sending len %d", dataLen)
	nOut, err := conn.dev.Write(packet)
	if nOut != len(packet) {
		conn.log.Errorf("short write %d of %d bytes %v", nOut, len(packet), err)
	}
}

func (conn *ClientConn) WriteTo(payload []byte, addr net.Addr) (int, error) {
	n := uint(len(payload))
	log.Infof("Received %v bytes from Ziti", n)
	var chunkLen uint
	for i := uint(0); i < n; i += maxPayloadSize {
		if i+maxPayloadSize > n {
			chunkLen = n % maxPayloadSize
		} else {
			chunkLen = maxPayloadSize
		}
		chunk := payload[i : i+chunkLen]
		log.Infof("Sending bytes [%v, %v] back", i, i+chunkLen)
		conn.udpSend(chunk, addr.(*net.UDPAddr))
	}

	return int(n), nil
}
