// lifted from https://github.com/google/netstack/blob/master/tcpip/header/tcp.go

// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package udp

import (
	"encoding/binary"
	"github.com/netfoundry/ziti-edge/tunnel/intercept/protocols/ip"
)

const (
	srcPortOffset  = 0
	dstPortOffset  = 2
	lengthOffset   = 4
	checksumOffset = 6
	headerSize     = 8
)

type UDP []byte

// udpFields contains the fields of a UDP packet. It is used to describe the
// fields of a packet that needs to be encoded.
type udpFields struct {
	// SrcPort is the "source port" field of a UDP packet.
	SrcPort uint16

	// DstPort is the "destination port" field of a UDP packet.
	DstPort uint16

	// Length is the "length" field of a UDP packet.
	Length uint16

	// Checksum is the "checksum" field of a UDP packet.
	Checksum uint16
}

// SourcePort returns the "source port" field of the udp header.
func (b UDP) SourcePort() uint16 {
	return binary.BigEndian.Uint16(b[srcPortOffset:])
}

// DestinationPort returns the "destination port" field of the udp header.
func (b UDP) DestinationPort() uint16 {
	return binary.BigEndian.Uint16(b[dstPortOffset:])
}

// SequenceNumber returns the "sequence number" field of the udp header.
func (b UDP) Length() uint16 {
	return binary.BigEndian.Uint16(b[lengthOffset:])
}

// Payload returns the data in the tcp packet.
func (b UDP) Payload() []byte {
	return b[headerSize:]
}

// Checksum returns the "checksum" field of the tcp header.
func (b UDP) Checksum() uint16 {
	return binary.BigEndian.Uint16(b[checksumOffset:])
}

// SetSourcePort sets the "source port" field of the tcp header.
func (b UDP) SetSourcePort(port uint16) {
	binary.BigEndian.PutUint16(b[srcPortOffset:], port)
}

// SetDestinationPort sets the "destination port" field of the tcp header.
func (b UDP) SetDestinationPort(port uint16) {
	binary.BigEndian.PutUint16(b[dstPortOffset:], port)
}

// SetChecksum sets the checksum field of the tcp header.
func (b UDP) SetChecksum(checksum uint16) {
	binary.BigEndian.PutUint16(b[checksumOffset:], checksum)
}

func (b UDP) CalculateChecksum() uint16 {
	return ip.Checksum(b, 0)
}

// Encode encodes all the fields of the udp header.
func (b UDP) Encode(fields *udpFields) {
	binary.BigEndian.PutUint16(b[srcPortOffset:], fields.SrcPort)
	binary.BigEndian.PutUint16(b[dstPortOffset:], fields.DstPort)
	binary.BigEndian.PutUint16(b[lengthOffset:], fields.Length)
	binary.BigEndian.PutUint16(b[checksumOffset:], fields.Checksum)
}
