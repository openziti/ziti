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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/link"
	"google.golang.org/protobuf/proto"
)

// checkStaleLinksHandler answers controller-issued CheckStaleLinks
// requests. For each established link this router holds, it reports
// whether the link is "stale" from this router's perspective — i.e.,
// whether the current link configuration could still re-establish the
// link if it were lost.
//
// The router only reports its own side per link. The controller
// aggregates the two reports (dialer side + listener side) — if either
// side reports stale, the link is stale overall.
type checkStaleLinksHandler struct {
	env env.RouterEnv
}

func newCheckStaleLinksHandler(env env.RouterEnv) *checkStaleLinksHandler {
	return &checkStaleLinksHandler{env: env}
}

func (self *checkStaleLinksHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_CheckStaleLinksRequestType)
}

func (self *checkStaleLinksHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	req := &ctrl_pb.CheckStaleLinksRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		log.WithError(err).Error("unable to unmarshal CheckStaleLinksRequest")
		return
	}

	go func() {
		reports := self.computeReports(req.Mode)

		self.respond(ch, msg, &ctrl_pb.CheckStaleLinksResponse{
			RouterId: self.env.GetRouterId().Token,
			Success:  true,
			Reports:  reports,
		})
	}()
}

// computeReports walks every established link and produces a
// per-link stale-or-not verdict for THIS router's side of the link.
// Caller (controller) aggregates with the verdict from the other side.
func (self *checkStaleLinksHandler) computeReports(wireMode ctrl_pb.StaleLinkMatchMode) []*ctrl_pb.LinkStaleReport {
	listeners := self.env.GetXlinkListeners()
	dialers := self.env.GetXlinkDialers()
	registry := self.env.GetXlinkRegistry()
	destListeners := registry.GetDestinationListeners()
	mode := link.StalenessModeFromCtrl(wireMode)

	var reports []*ctrl_pb.LinkStaleReport
	for xl := range registry.Iter() {
		if xl.IsClosed() {
			continue
		}

		report := &ctrl_pb.LinkStaleReport{LinkId: xl.Id()}
		if xl.IsDialed() {
			report.Side = ctrl_pb.StaleLinkSide_StaleLinkSideDialer
			report.Stale, report.Reason = link.CheckDialerSide(xl, dialers, destListeners, mode)
		} else {
			report.Side = ctrl_pb.StaleLinkSide_StaleLinkSideListener
			report.Stale, report.Reason = link.CheckListenerSide(xl, listeners, mode)
		}
		reports = append(reports, report)
	}
	return reports
}

func (self *checkStaleLinksHandler) respond(ch channel.Channel, in *channel.Message, body *ctrl_pb.CheckStaleLinksResponse) {
	buf, err := proto.Marshal(body)
	if err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("unable to marshal CheckStaleLinksResponse")
		return
	}
	out := channel.NewMessage(int32(ctrl_pb.ContentType_CheckStaleLinksResponseType), buf)
	out.ReplyTo(in)
	if err := ch.Send(out); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("unable to send CheckStaleLinksResponse")
	}
}
