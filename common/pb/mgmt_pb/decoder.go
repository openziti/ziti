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

package mgmt_pb

import (
	"fmt"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
)

type Decoder struct{}

const DECODER = "mgmt"

func (d Decoder) Decode(msg *channel.Message) ([]byte, bool) {
	switch msg.ContentType {
	case int32(ContentType_StreamEventsRequestType):
		meta := channel.NewTraceMessageDecode(DECODER, "Stream Events Request")
		meta["request"] = string(msg.Body)
		data, err := meta.MarshalTraceMessageDecode()
		if err != nil {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}
		return data, true
	case int32(ContentType_StreamEventsEventType):
		meta := channel.NewTraceMessageDecode(DECODER, "Stream Events Event")
		meta["event"] = string(msg.Body)
		data, err := meta.MarshalTraceMessageDecode()
		if err != nil {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}
		return data, true
	case int32(ContentType_StreamTracesRequestType):
		data, err := channel.NewTraceMessageDecode(DECODER, "Stream Traces Request").MarshalTraceMessageDecode()
		if err != nil {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}
		return data, true
	}

	return nil, false
}

func (self *Path) CalculateDisplayPath() string {
	if self == nil {
		return ""
	}
	out := ""
	for i := 0; i < len(self.Nodes); i++ {
		if i < len(self.Links) {
			out += fmt.Sprintf("[r/%s]->{l/%s}->", self.Nodes[i], self.Links[i])
		} else {
			out += fmt.Sprintf("[r/%s%s]\n", self.Nodes[i], func() string {
				if self.TerminatorLocalAddress == "" {
					return ""
				}
				return fmt.Sprintf(" (%s)", self.TerminatorLocalAddress)
			}())
		}
	}
	return out
}
