/*
	Copyright NetFoundry Inc.

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

package xgress

import (
	"encoding/binary"
	"github.com/openziti/channel/v4"
	"time"
)

type PayloadTransformer struct {
}

func (self PayloadTransformer) Rx(*channel.Message, channel.Channel) {}

func (self PayloadTransformer) Tx(m *channel.Message, ch channel.Channel) {
	if m.ContentType == channel.ContentTypeRaw && len(m.Body) > 1 {
		if m.Body[0]&HeartbeatFlagMask != 0 && len(m.Body) > 12 {
			now := time.Now().UnixNano()
			m.PutUint64Header(channel.HeartbeatHeader, uint64(now))
			binary.BigEndian.PutUint64(m.Body[len(m.Body)-8:], uint64(now))
		}
	}
}
