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
	"net"
	"time"
)

/**
 * TRANSWARP v1 Wire Format
 *
 * // --- message section --------------------------------------------------------------------------------- //
 *
 * <version:[]byte>								0  1  2  3
 * <sequence:int32> 							4  5  6  7
 * <frame:uint8>								8
 * <of_frames:uint8>							9
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

const messageLen = 15

type contentType uint8

const (
	Hello contentType = iota
	Payload
	Acknowledgement
)

const versionLen = 4

func writeHello(linkId *identity.TokenId, conn *net.UDPConn, peer *net.UDPAddr) error {
	payload := new(bytes.Buffer)
	payload.Write([]byte(linkId.Token))

	data := new(bytes.Buffer)
	data.Write(magicV1)                                                        // version
	if err := binary.Write(data, binary.LittleEndian, int32(-1)); err != nil { // sequence
		return err
	}
	data.Write([]byte{0x01, 0x01})                                             // frame, of_frames
	data.Write([]byte{byte(Hello)})                                            // content_type
	if err := binary.Write(data, binary.LittleEndian, uint16(0)); err != nil { // headers_length
		return err
	}
	if err := binary.Write(data, binary.LittleEndian, uint16(4)); err != nil { // payload_length
		return err
	}
	data.Write(payload.Bytes())

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("unable to set deadline (%w)", err)
	}
	if _, err := conn.WriteToUDP(data.Bytes()[:15+4], peer); err != nil {
		return err
	}

	return nil
}

func readHello(conn *net.UDPConn) (*identity.TokenId, *net.UDPAddr, error) {
	data := make([]byte, 10240)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, nil, fmt.Errorf("unable to set deadline (%w)", err)
	}
	n, peer, err := conn.ReadFromUDP(data)
	if err != nil {
		return nil, nil, err
	}
	if n < messageLen {
		return nil, peer, fmt.Errorf("short read")
	}
	for i := 0; i < len(magicV1); i++ {
		if data[i] != magicV1[i] {
			return nil, peer, fmt.Errorf("bad magic")
		}
	}
	if sequence, err := readInt32(data[4:8]); err == nil {
		if sequence != -1 {
			return nil, peer, fmt.Errorf("invalid hello sequence [%d]", sequence)
		}
	} else {
		return nil, peer, fmt.Errorf("error reading sequence (%w)", err)
	}
	if data[8] != 0x01 || data[9] != 0x01 {
		return nil, peer, fmt.Errorf("framing error")
	}
	if data[10] != byte(Hello) {
		return nil, peer, fmt.Errorf("unexpected content type [%d]", data[10])
	}
	headersLength, err := readUint16(data[11:13])
	if err != nil {
		return nil, peer, fmt.Errorf("error reading headers length (%w)", err)
	}
	if headersLength != 0 {
		return nil, peer, fmt.Errorf("unexpected headers [%d]", headersLength)
	}
	payloadLength, err := readUint16(data[13:15])
	if err != nil {
		return nil, peer, fmt.Errorf("error reading payload length (%w)", err)
	}
	if payloadLength != 4 {
		return nil, peer, fmt.Errorf("unexpected payload length [%d]", payloadLength)
	}
	expectedLength := int(15 + headersLength + payloadLength)
	if n != expectedLength {
		return nil, peer, fmt.Errorf("long/short packet [%d] != [%d]", n, expectedLength)
	}
	linkId := string(data[15+headersLength:15+headersLength+payloadLength])
	return &identity.TokenId{Token: linkId}, peer, nil
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
