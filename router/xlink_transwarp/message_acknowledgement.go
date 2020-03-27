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
 * TRANSWARP v1 Acknowledgement Format
 *
 * <session_id_length:int32>					0  1  2  3
 * <flags:uint32>								4  5  6  7
 * <sequence_ids_count:int32>					8  9 10 11
 * <session_id>									12 -> (12 + session_id_length)
 * <sequence_ids:[]int32>						(12 + session_id_length) -> ((12 + session_id_length) + (4 * sequence_ids_count))
 */
func encodeAcnowledgement(a *xgress.Acknowledgement, sequence int32) (m *message, err error) {
	payload := new(bytes.Buffer)
	if err := binary.Write(payload, binary.LittleEndian, int32(len(a.SessionId))); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, a.Flags); err != nil {
		return nil, err
	}
	if err := binary.Write(payload, binary.LittleEndian, int32(len(a.Sequence))); err != nil {
		return nil, err
	}
	if _, err := payload.Write([]byte(a.SessionId)); err != nil {
		return nil, err
	}
	for _, sequenceId := range a.Sequence {
		if err := binary.Write(payload, binary.LittleEndian, sequenceId); err != nil {
			return nil, err
		}
	}

	m = &message{
		sequence:    sequence,
		fragment:    0,
		ofFragments: 1,
		messageType: Acknowledgement,
		payload:     payload.Bytes(),
	}

	return
}

func decodeAcknowledgement(m *message) (a *xgress.Acknowledgement, err error) {
	sessionIdLength, err := readInt32(m.payload[0:4])
	if err != nil {
		return nil, err
	}
	flags, err := readUint32(m.payload[4:8])
	if err != nil {
		return nil, err
	}
	sequenceIdsCount, err := readInt32(m.payload[8:12])
	if err != nil {
		return nil, err
	}
	sessionId := m.payload[12 : 12+sessionIdLength]
	nextSequenceId := 12 + sessionIdLength
	var sequenceIds []int32
	for i := 0; i < int(sequenceIdsCount); i++ {
		sequenceId, err := readInt32(m.payload[nextSequenceId : nextSequenceId+4])
		if err != nil {
			return nil, err
		}
		sequenceIds = append(sequenceIds, sequenceId)
		nextSequenceId += 4
	}

	a = &xgress.Acknowledgement{
		Header: xgress.Header{
			SessionId: string(sessionId),
			Flags:     flags,
		},
		Sequence: sequenceIds,
	}

	return
}
