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
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport/udp"
	"net"
	"time"
)

type HelloHandler interface {
	HandleHello(linkId *identity.TokenId, conn *net.UDPConn, addr *net.UDPAddr)
}

type MessageHandler interface {
	HandlePing(sequence int32, replyFor int32, conn *net.UDPConn, addr *net.UDPAddr)
	HandlePayload(p *xgress.Payload, sequence int32, conn *net.UDPConn, addr *net.UDPAddr)
	HandleAcknowledgement(a *xgress.Acknowledgement, sequence int32, conn *net.UDPConn, addr *net.UDPAddr)
	HandleWindowReport(lowWater, highWater, gaps, count int32, conn *net.UDPConn, addr *net.UDPAddr)
	HandleWindowSizeRequest(newWindowSize int32, conn *net.UDPConn, addr *net.UDPAddr)
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

type messageType uint8

const (
	Hello messageType = iota
	Ping
	Payload
	Acknowledgement
	WindowReport
	WindowSizeRequest
)

const timeoutSeconds = 5
const mss = 1472
const noReplyFor = -1

func writeHello(linkId *identity.TokenId, conn *net.UDPConn, peer *net.UDPAddr) error {
	payload := new(bytes.Buffer)
	payload.Write([]byte(linkId.Token))

	data, err := encodeMessage(&message{
		sequence:    -1,
		fragment:    0,
		ofFragments: 1,
		messageType: Hello,
		payload:     payload.Bytes(),
	})
	if err != nil {
		return fmt.Errorf("error creating message (%w)", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(timeoutSeconds * time.Second)); err != nil {
		return fmt.Errorf("unable to set write deadline (%w)", err)
	}

	if _, err := conn.WriteToUDP(data, peer); err != nil {
		return err
	}

	return nil
}

func writePing(sequence int32, conn *net.UDPConn, peer *net.UDPAddr, replyFor int32) error {
	payload := new(bytes.Buffer)
	if err := binary.Write(payload, binary.LittleEndian, replyFor); err != nil {
		return fmt.Errorf("reply for write (%w)", err)
	}

	data, err := encodeMessage(&message{
		sequence:    sequence,
		fragment:    0,
		ofFragments: 1,
		messageType: Ping,
		payload:     payload.Bytes(),
	})
	if err != nil {
		return fmt.Errorf("error creating message (%w)", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(timeoutSeconds * time.Second)); err != nil {
		return fmt.Errorf("unable to set write deadline (%w)", err)
	}

	if _, err := conn.WriteToUDP(data, peer); err != nil {
		return err
	}

	return nil
}

func writePayload(sequence int32, p *xgress.Payload, conn *net.UDPConn, peer *net.UDPAddr) error {
	m, err := encodePayload(p, sequence)
	if err != nil {
		return err
	}

	data, err := encodeMessage(m)
	if err != nil {
		return fmt.Errorf("error creating message (%w)", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(timeoutSeconds * time.Second)); err != nil {
		return fmt.Errorf("unable to set write deadline (%w)", err)
	}

	if _, err := conn.WriteToUDP(data, peer); err != nil {
		return err
	}

	return nil
}

func writeAcknowledgement(sequence int32, a *xgress.Acknowledgement, conn *net.UDPConn, peer *net.UDPAddr) error {
	m, err := encodeAcnowledgement(a, sequence)
	if err != nil {
		return err
	}

	data, err := encodeMessage(m)
	if err != nil {
		return fmt.Errorf("error creating message (%w)", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(timeoutSeconds * time.Second)); err != nil {
		return fmt.Errorf("unable to set write deadline (%w)", err)
	}

	if _, err := conn.WriteToUDP(data, peer); err != nil {
		return err
	}

	return nil
}

func writeWindowReport(sequence int32, lowWater, highWater, gaps, count int32, conn *net.UDPConn, peer *net.UDPAddr) error {
	m, err := encodeWindowReport(sequence, lowWater, highWater, gaps, count)
	if err != nil {
		return err
	}

	data, err := encodeMessage(m)
	if err != nil {
		return fmt.Errorf("error creating message (%w)", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(timeoutSeconds * time.Second)); err != nil {
		return fmt.Errorf("unable to set write deadline (%w)", err)
	}

	if _, err := conn.WriteToUDP(data, peer); err != nil {
		return err
	}

	return nil
}

func writeWindowSizeRequest(sequence, newWindowSize int32, conn *net.UDPConn, peer *net.UDPAddr) error {
	m, err := encodeWindowSizeRequest(sequence, newWindowSize)
	if err != nil {
		return err
	}

	data, err := encodeMessage(m)
	if err != nil {
		return fmt.Errorf("error creating message (%w)", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(timeoutSeconds * time.Second)); err != nil {
		return fmt.Errorf("unable to set write deadline (%w)", err)
	}

	if _, err := conn.WriteToUDP(data, peer); err != nil {
		return err
	}

	return nil
}

func readMessage(conn *net.UDPConn) (*message, *net.UDPAddr, error) {
	data := make([]byte, udp.MaxPacketSize)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, nil, fmt.Errorf("error setting read deadline (%w)", err)
	}
	if n, peer, err := conn.ReadFromUDP(data); err == nil {
		if m, err := decodeMessage(data[:n]); err == nil {
			return m, peer, nil
		} else {
			return nil, nil, fmt.Errorf("error decoding message from [%s] (%w)", peer, err)
		}
	} else {
		return nil, nil, err
	}
}

func handleHello(m *message, conn *net.UDPConn, peer *net.UDPAddr, handler HelloHandler) error {
	if m != nil {
		switch m.messageType {
		case Hello:
			if m.sequence != -1 {
				return fmt.Errorf("hello expects sequence -1 [%s]", peer)
			}
			if m.fragment != 0 || m.ofFragments != 1 {
				return fmt.Errorf("hello expects single fragment [%s]", peer)
			}
			linkId := &identity.TokenId{Token: string(m.payload)}
			handler.HandleHello(linkId, conn, peer)

			return nil

		default:
			return fmt.Errorf("expected hello, not [%d] from [%s]", m.messageType, peer)
		}
	} else {
		return fmt.Errorf("nil message")
	}
}

func handleMessage(m *message, conn *net.UDPConn, peer *net.UDPAddr, handler MessageHandler) error {
	if m.fragment != 0 || m.ofFragments != 1 {
		return fmt.Errorf("ping expects single fragment [%s]", peer)
	}

	switch m.messageType {
	case Ping:
		replyFor, err := readInt32(m.payload)
		if err != nil {
			return fmt.Errorf("ping expects replyFor in payload [%s] (%w)", peer, err)
		}
		handler.HandlePing(m.sequence, replyFor, conn, peer)

		return nil

	case Payload:
		p, err := decodePayload(m)
		if err != nil {
			return fmt.Errorf("error decoding payload for peer [%s] (%w)", peer, err)
		}
		handler.HandlePayload(p, m.sequence, conn, peer)

		return nil

	case Acknowledgement:
		a, err := decodeAcknowledgement(m)
		if err != nil {
			return fmt.Errorf("error decoding acknowledgement for peer [%s] (%w)", peer, err)
		}
		handler.HandleAcknowledgement(a, m.sequence, conn, peer)

		return nil

	case WindowReport:
		lowWater, highWater, gaps, count, err := decodeWindowReport(m)
		if err != nil {
			return fmt.Errorf("error decoding window report for peer [%s] (%w)", peer, err)
		}
		handler.HandleWindowReport(lowWater, highWater, gaps, count, conn, peer)

		return nil

	case WindowSizeRequest:
		newWindowSize, err := decodeWindowSizeRequest(m)
		if err != nil {
			return fmt.Errorf("error decoding window size request for peer [%s] (%w)", peer, err)
		}
		handler.HandleWindowSizeRequest(newWindowSize, conn, peer)

		return nil

	default:
		return fmt.Errorf("unexpected message type [%d] from [%s]", m.messageType, peer)
	}
}

func encodeMessage(m *message) ([]byte, error) {
	data := new(bytes.Buffer)

	data.Write(magicV1)
	if err := binary.Write(data, binary.LittleEndian, m.sequence); err != nil {
		return nil, fmt.Errorf("sequence write (%w)", err)
	}
	data.Write([]byte{m.fragment, m.ofFragments, uint8(m.messageType)})
	if err := binary.Write(data, binary.LittleEndian, uint16(0)); err != nil { // headers length
		return nil, fmt.Errorf("headers length write (%w)", err)
	}
	if err := binary.Write(data, binary.LittleEndian, uint16(len(m.payload))); err != nil {
		return nil, fmt.Errorf("payload length write (%w)", err)
	}
	data.Write(m.payload)

	buffer := make([]byte, data.Len())
	_, err := data.Read(buffer)
	if err != nil {
		return nil, fmt.Errorf("error reading buffer (%w)", err)
	}
	/*
		if n > mss {
			return nil, fmt.Errorf("message too long [%d]", n)
		}
	*/

	return buffer, nil
}

/*
 * TRANSWARP v1 Headers Wire Format
 *
 * <key:uint8>                                  0
 * <length:uint8>                               1
 * <data>                                       2 -> (2 + length)
 */
func encodeHeaders(headers map[uint8][]byte) ([]byte, error) {
	data := new(bytes.Buffer)
	for k, v := range headers {
		if _, err := data.Write([]byte{k}); err != nil {
			return nil, err
		}
		if err := binary.Write(data, binary.LittleEndian, uint8(len(v))); err != nil {
			return nil, err
		}
		if n, err := data.Write(v); err == nil {
			if n != len(v) {
				return nil, fmt.Errorf("short header write")
			}
		} else {
			return nil, err
		}
	}
	return data.Bytes(), nil
}

func decodeHeaders(data []byte) (map[uint8][]byte, error) {
	headers := make(map[uint8][]byte)
	if len(data) > 0 && len(data) < 2 {
		return nil, fmt.Errorf("truncated header data")
	}
	i := 0
	for i < len(data) {
		key := data[i]
		length := data[i+1]
		if i+2+int(length) > len(data) {
			return nil, fmt.Errorf("short header data (%d > %d)", i+2+int(length), len(data))
		}
		headerData := data[i+2 : i+2+int(length)]
		headers[key] = headerData
		i += 2 + int(length)
	}
	return headers, nil
}

func decodeMessage(data []byte) (*message, error) {
	m := &message{}
	if len(data) < messageSectionLength {
		return nil, fmt.Errorf("short read")
	}
	for i := 0; i < len(magicV1); i++ {
		if data[i] != magicV1[i] {
			return nil, fmt.Errorf("bad magic")
		}
	}
	sequence, err := readInt32(data[4:8])
	if err != nil {
		return nil, fmt.Errorf("error reading sequence (%w)", err)
	}
	m.sequence = sequence

	m.fragment = data[8]
	m.ofFragments = data[9]
	m.messageType = messageType(data[10])

	headersLength, err := readUint16(data[11:13])
	if err != nil {
		return nil, fmt.Errorf("error reading headers length (%w)", err)
	}
	if headers, err := decodeHeaders(data[15 : 15+headersLength]); err == nil {
		m.headers = headers
	} else {
		return nil, fmt.Errorf("headers error (%w)", err)
	}

	payloadLength, err := readUint16(data[13:15])
	if err != nil {
		return nil, fmt.Errorf("error reading payload length (%w)", err)
	}
	m.payload = data[15+headersLength : 15+headersLength+payloadLength]

	return m, nil
}

func readInt32(data []byte) (ret int32, err error) {
	buf := bytes.NewBuffer(data)
	err = binary.Read(buf, binary.LittleEndian, &ret)
	return
}

func writeInt32(value int32) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, value)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readUint32(data []byte) (ret uint32, err error) {
	buf := bytes.NewBuffer(data)
	err = binary.Read(buf, binary.LittleEndian, &ret)
	return
}

func readUint16(data []byte) (ret uint16, err error) {
	buf := bytes.NewBuffer(data)
	err = binary.Read(buf, binary.LittleEndian, &ret)
	return
}

type message struct {
	sequence    int32
	fragment    uint8
	ofFragments uint8
	messageType messageType
	headers     map[uint8][]byte
	payload     []byte
}
