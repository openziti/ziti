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

package xlink_transport

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"google.golang.org/protobuf/proto"
)

type linkStateReporter struct {
	ctrls env.NetworkControllers
	msgs  []*ctrl_pb.LinkStateUpdate
}

func (reporter *linkStateReporter) AcceptMetrics(message *ctrl_pb.LinkStateUpdate) {
	reporter.msgs = append(reporter.msgs, message)

	for len(reporter.msgs) > 0 {
		message = reporter.msgs[0]

		ctrlCh := reporter.ctrls.AnyCtrlChannel()
		log := pfxlog.Logger().WithField("ctrlId", ctrlCh.Id())

		bytes, err := proto.Marshal(message)
		if err != nil {
			log.WithError(err).Error("failed to encode link state update message")

			// drop message, since it's invalid somehow
			reporter.msgs[0] = nil
			reporter.msgs = reporter.msgs[1:]
			continue
		}

		chMsg := channel.NewMessage(int32(ctrl_pb.ContentType_LinkState), bytes)

		if err = chMsg.WithTimeout(reporter.ctrls.DefaultRequestTimeout()).SendAndWaitForWire(ctrlCh); err != nil {
			log.WithError(err).Error("failed to send link state update message")
		} else {
			log.Trace("reported link state update to controller")

			reporter.msgs[0] = nil
			reporter.msgs = reporter.msgs[1:]
		}
	}
}

func newLinkStateReporter(ctrls env.NetworkControllers) *linkStateReporter {
	return &linkStateReporter{
		ctrls: ctrls,
	}
}
