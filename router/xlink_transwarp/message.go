/*
	(c) Copyright NetFoundry, Inc.

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

package xlink_transwarp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport/udp"
	"net"
	"time"
)

type MessageHandler interface {
	HandleHello(linkId *identity.TokenId, addr *net.UDPAddr)
}

/**
 * TRANSWARP v1 Wire Format
 *
 * // --- message section --------------------------------------------------------------------------------- //
 *
 * <version:[]byte>								0  1  2  3
 * <sequence:int32> 							4  5  6  7
 * <fragment:uint8>								8
 * <of_fragments:uint8>							9
 * <content_type:uint8>							10
 * <headers_length:uint16>						11 12
 * <payload_length:uint16> 						13 14
 *
 * // --- data section ------------------------------------------------------------------------------------ //
 *
 * <headers>									15 -> (15 + headers_length)
 * <body>										(15 + headers_length) -> (15 + headers_length + body_length)
 */
var magicV1 = []byte{0x01, 0x02, 0x02, 0x00}

const messageSectionLength = 15

type contentType uint8

const (
	Hello contentType = iota
	Payload
	Acknowledgement
)

const versionLen = 4
const mss = 1472

func createMessage(sequence int32, frame uint8, ofFrames uint8, ct uint8, headers map[int32][]byte, payload []byte) ([]byte, error) {
	data := new(bytes.Buffer)

	data.Write(magicV1)
	if err := binary.Write(data, binary.LittleEndian, sequence); err != nil {
		return nil, fmt.Errorf("sequence write (%w)", err)
	}
	data.Write([]byte{ frame, ofFrames, ct })
	if err := binary.Write(data, binary.LittleEndian, uint16(0)); err != nil {	// headers length
		return nil, fmt.Errorf("headers length write (%w)", err)
	}
	if err := binary.Write(data, binary.LittleEndian, uint16(len(payload))); err != nil {
		return nil, fmt.Errorf("payload length write (%w)", err)
	}
	data.Write(payload)

	buffer := make([]byte, data.Len())
	n, err := data.Read(buffer)
	if err != nil {
		return nil, fmt.Errorf("error reading buffer (%w)", err)
	}
	if n > mss {
		return nil, fmt.Errorf("message too long [%d]", n)
	}

	return buffer, nil
}

func writeHello(linkId *identity.TokenId, conn *net.UDPConn, peer *net.UDPAddr) error {
	payload := new(bytes.Buffer)
	payload.Write([]byte(linkId.Token))

	data, err := createMessage(-1, 0, 1, uint8(Hello), nil, payload.Bytes())
	if err != nil {
		return fmt.Errorf("error creating message (%w)", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("unable to set deadline (%w)", err)
	}

	if _, err := conn.WriteToUDP(data, peer); err != nil {
		return err
	}

	return nil
}

func readMessage(conn *net.UDPConn, handler MessageHandler) error {
	data := make([]byte, udp.MaxPacketSize)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("error setting read deadline (%w)", err)
	}
	if n, peer, err := conn.ReadFromUDP(data); err == nil {
		return handleMessage(data[:n], peer, handler)
	} else {
		return err
	}
}

func handleMessage(data []byte, peer *net.UDPAddr, handler MessageHandler) error {
	if len(data) < messageSectionLength {
		return fmt.Errorf("short read [%s]", peer)
	}
	for i := 0; i < len(magicV1); i++ {
		if data[i] != magicV1[i] {
			return fmt.Errorf("bad magic [%s]", peer)
		}
	}
	sequence, err := readInt32(data[4:8])
	if err != nil {
		return fmt.Errorf("error reading sequence [%s] (%w)", peer, err)
	}
	fragment := data[8]
	ofFragments := data[9]
	contentType := data[10]
	headersLength, err := readUint16(data[11:13])
	if err != nil {
		return fmt.Errorf("error reading headers length [%s] (%w)", peer, err)
	}
	if headersLength != 0 {
		return fmt.Errorf("headers error [%s]", peer)
	}
	payloadLength, err := readUint16(data[13:15])
	if err != nil {
		return fmt.Errorf("error reading payload length [%s] (%w)", peer, err)
	}
	payload := data[15 + headersLength:15+headersLength+payloadLength]

	switch contentType {
	case uint8(Hello):
		if sequence != -1 {
			return fmt.Errorf("hello expects sequence -1 [%s]", peer)
		}
		if fragment != 0 || ofFragments != 1 {
			return fmt.Errorf("hello expects single fragment [%s]", peer)
		}
		linkId := &identity.TokenId{Token: string(payload)}
		handler.HandleHello(linkId, peer)

	default:
		return fmt.Errorf("no handler for content type [%d]", contentType)
	}

	return nil
}


func readInt32(data []byte) (ret int32, err error) {
	buf := bytes.NewBuffer(data)
	err = binary.Read(buf, binary.LittleEndian, &ret)
	return
}

func readUint16(data []byte) (ret uint16, err error) {
	buf := bytes.NewBuffer(data)
	err = binary.Read(buf, binary.LittleEndian, &ret)
	return
}
