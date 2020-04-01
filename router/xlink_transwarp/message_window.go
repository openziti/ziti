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
)

func encodeWindowReport(sequence, lowWater, highWater, gaps, count int32) (*message, error) {
	headers := make(map[uint8][]byte)
	if data, err := writeInt32(lowWater); err == nil {
		headers[0] = data
	} else {
		return nil, err
	}
	if data, err := writeInt32(highWater); err == nil {
		headers[1] = data
	} else {
		return nil, err
	}
	if data, err := writeInt32(gaps); err != nil {
		headers[2] = data
	} else {
		return nil, err
	}
	if data, err := writeInt32(count); err != nil {
		headers[3] = data
	} else {
		return nil, err
	}

	return &message{
		sequence:    sequence,
		fragment:    0,
		ofFragments: 1,
		messageType: WindowReport,
		headers:     headers,
	}, nil
}

func decodeWindowReport(m *message) (lowWater, highWater, gaps, count int32, err error) {
	if m.headers != nil {
		if value, found := m.headers[0]; found {
			i32v, err := readInt32(value)
			if err == nil {
				lowWater = i32v
			} else {
				return 0, 0, 0, 0, fmt.Errorf("malformed lowWater (%w)", err)
			}
		} else {
			return 0, 0, 0, 0, fmt.Errorf("missing lowWater")
		}
		if value, found := m.headers[1]; found {
			i32v, err := readInt32(value)
			if err == nil {
				highWater = i32v
			} else {
				return 0, 0, 0, 0, fmt.Errorf("malformed highWater (%w)", err)
			}
		} else {
			return 0, 0, 0, 0, fmt.Errorf("missing highWater")
		}
		if value, found := m.headers[2]; found {
			i32v, err := readInt32(value)
			if err == nil {
				gaps = i32v
			} else {
				return 0, 0, 0, 0, fmt.Errorf("malformed gaps (%w)", err)
			}
		} else {
			return 0, 0, 0, 0, fmt.Errorf("missing gaps")
		}
		if value, found := m.headers[3]; found {
			i32v, err := readInt32(value)
			if err == nil {
				count = i32v
			} else {
				return 0, 0, 0, 0, fmt.Errorf("malformed count (%w)", err)
			}
		} else {
			return 0, 0, 0, 0, fmt.Errorf("missing count")
		}

		return
	} else {
		return 0, 0, 0, 0, fmt.Errorf("missing headers")
	}
}

func encodeWindowSizeRequest(sequence, newSize int32) (*message, error) {
	payload := new(bytes.Buffer)
	if err := binary.Write(payload, binary.LittleEndian, newSize); err != nil {
		return nil, err
	}

	m := &message{
		sequence:    sequence,
		fragment:    0,
		ofFragments: 1,
		messageType: WindowSizeRequest,
		payload:     payload.Bytes(),
	}

	return m, nil
}

func decodeWindowSizeRequest(m *message) (int32, error) {
	newWindowSize, err := readInt32(m.payload[0:4])
	if err != nil {
		return 0, err
	}
	return newWindowSize, nil
}
