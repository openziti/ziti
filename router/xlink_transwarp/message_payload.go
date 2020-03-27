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
	"github.com/netfoundry/ziti-fabric/router/xgress"
)

/*
 * TRANSWARP v1 Payload Format
 *
 * <session_id_length:int32>					0  1  2  3
 * <flags:uint32>								4  5  6  7
 * <sequence:int32>								8  9 10 11
 * <data_length:int32>						   12 13 14 15
 * <session_id>								   16 -> (16 + session_id_length)
 * <data>                                      (16 + session_id_length) -> (16 + session_id_length + data_length)
 */
func encodePayload(p *xgress.Payload, sequence int32) (m *message, err error) {
	payload := new(bytes.Buffer)
	if err := binary.Write(payload, binary.LittleEndian, int32(len(p.SessionId))); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, p.Flags); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, p.Sequence); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, int32(len(p.Data))); err != nil {
		return nil, err
	}
	if _, err := payload.Write([]byte(p.SessionId)); err != nil {
		return nil, err
	}
	if _, err := payload.Write(p.Data); err != nil {
		return nil, err
	}

	m = &message{
		sequence:    sequence,
		fragment:    0,
		ofFragments: 1,
		messageType: Payload,
		headers:     p.Headers,
		payload:     payload.Bytes(),
	}

	return
}

func decodePayload(m *message) (p *xgress.Payload, err error) {
	sessionIdLength, err := readInt32(m.payload[0:4])
	if err != nil {
		return nil, err
	}
	flags, err := readUint32(m.payload[4:8])
	if err != nil {
		return nil, err
	}
	sequence, err := readInt32(m.payload[8:12])
	if err != nil {
		return nil, err
	}
	dataLength, err := readInt32(m.payload[12:16])
	if err != nil {
		return nil, err
	}
	sessionId := m.payload[16 : 16+sessionIdLength]
	data := m.payload[16+sessionIdLength : 16+sessionIdLength+dataLength]

	p = &xgress.Payload{
		Header: xgress.Header{
			SessionId: string(sessionId),
			Flags:     flags,
		},
		Sequence: sequence,
		Headers:  m.headers,
		Data:     data,
	}

	return p, nil
}
