/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/foundation/util/goroutines"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/handler_xgress"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type routeHandler struct {
	id        *identity.TokenId
	ctrl      xgress.CtrlChannel
	dialerCfg map[string]xgress.OptionsData
	forwarder *forwarder.Forwarder
	pool      goroutines.Pool
}

func newRouteHandler(id *identity.TokenId, ctrl xgress.CtrlChannel, dialerCfg map[string]xgress.OptionsData, forwarder *forwarder.Forwarder, pool goroutines.Pool) *routeHandler {
	handler := &routeHandler{
		id:        id,
		ctrl:      ctrl,
		dialerCfg: dialerCfg,
		forwarder: forwarder,
		pool:      pool,
	}

	return handler
}

func (rh *routeHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_RouteType)
}

func (rh *routeHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	route := &ctrl_pb.Route{}
	if err := proto.Unmarshal(msg.Body, route); err == nil {
		var ctx logcontext.Context
		if route.Context != nil {
			ctx = logcontext.NewContextWith(route.Context.ChannelMask, route.Context.Fields)
		} else {
			ctx = logcontext.NewContext()
		}
		log := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx).
			WithField("context", ch.Label()).
			WithField("circuitId", route.CircuitId).
			WithField("attempt", route.Attempt)

		if route.Egress != nil {
			log = log.WithField("binding", route.Egress.Binding).WithField("destination", route.Egress.Destination)
		}

		log.Debugf("attempt [#%d] for [s/%s]", route.Attempt, route.CircuitId)

		if route.Egress != nil {
			if rh.forwarder.HasDestination(xgress.Address(route.Egress.Address)) {
				log.Warnf("destination exists for [%s]", route.Egress.Address)
				rh.success(msg, int(route.Attempt), route, nil, log)
				return
			} else {
				rh.connectEgress(msg, int(route.Attempt), ch, route, ctx)
				return
			}
		} else {
			rh.success(msg, int(route.Attempt), route, nil, log)
		}
	} else {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("error unmarshaling")
	}
}

func (rh *routeHandler) success(msg *channel.Message, attempt int, route *ctrl_pb.Route, peerData xt.PeerData, log *logrus.Entry) {
	rh.forwarder.Route(route)
	log.Debug("forwarder updated with route")

	response := ctrl_msg.NewRouteResultSuccessMsg(route.CircuitId, attempt)
	for k, v := range peerData {
		response.Headers[int32(k)] = v
	}

	response.ReplyTo(msg)

	log.Debug("sending sucess response")
	if err := rh.ctrl.Channel().Send(response); err == nil {
		log.Debug("handled route")
	} else {
		log.WithError(err).Error("send response failed")
	}
}

func (rh *routeHandler) fail(msg *channel.Message, attempt int, route *ctrl_pb.Route, err error, errorHeader *byte, log *logrus.Entry) {
	log.WithError(err).Error("failed to connect egress")

	response := ctrl_msg.NewRouteResultFailedMessage(route.CircuitId, attempt, err.Error())
	if errorHeader != nil {
		response.PutByteHeader(ctrl_msg.RouteResultErrorCodeHeader, *errorHeader)
	}
	response.ReplyTo(msg)
	if err := rh.ctrl.Channel().Send(response); err != nil {
		log.WithError(err).Error("send failure response failed")
	}
}

func (rh *routeHandler) connectEgress(msg *channel.Message, attempt int, ch channel.Channel, route *ctrl_pb.Route, ctx logcontext.Context) {
	log := pfxlog.ChannelLogger(logcontext.EstablishPath).Wire(ctx).
		WithField("context", ch.Label()).
		WithField("circuitId", route.CircuitId).
		WithField("binding", route.Egress.Binding).
		WithField("destination", route.Egress.Destination).
		WithField("attempt", route.Attempt)

	log.Debug("route request received")

	dialF := func() {
		if factory, err := xgress.GlobalRegistry().Factory(route.Egress.Binding); err == nil {
			if dialer, err := factory.CreateDialer(rh.dialerCfg[route.Egress.Binding]); err == nil {
				circuitId := &identity.TokenId{Token: route.CircuitId, Data: route.Egress.PeerData}

				bindHandler := handler_xgress.NewBindHandler(
					handler_xgress.NewReceiveHandler(rh.forwarder),
					handler_xgress.NewCloseHandler(rh.ctrl, rh.forwarder),
					rh.forwarder)

				if rh.forwarder.Options.XgressDialDwellTime > 0 {
					log.Infof("dwelling [%s] on dial", rh.forwarder.Options.XgressDialDwellTime)
					time.Sleep(rh.forwarder.Options.XgressDialDwellTime)
				}

				if peerData, err := dialer.Dial(route.Egress.Destination, circuitId, xgress.Address(route.Egress.Address), bindHandler, ctx); err == nil {
					rh.success(msg, attempt, route, peerData, log)
				} else {
					var headerError *byte
					if _, ok := err.(xgress.InvalidTerminatorError); ok {
						var errCode byte = ctrl_msg.ErrorTypeInvalidTerminator
						headerError = &errCode
						switch {
						case errors.Is(err, syscall.ECONNREFUSED):
							errCode = ctrl_msg.ErrorTypeConnectionRefused
						case errors.Is(err, syscall.ETIMEDOUT):
							errCode = ctrl_msg.ErrorTypeDialTimedOut
						case errors.As(err, &xgress.MisconfiguredTerminatorError{}):
							errCode = ctrl_msg.ErrorTypeMisconfiguredTerminator
						case errors.As(err, &xgress.InvalidTerminatorError{}):
							errCode = ctrl_msg.ErrorTypeInvalidTerminator
						}
						headerError = &errCode
					}
					rh.fail(msg, attempt, route, errors.Wrapf(err, "error creating route for [c/%s]", route.CircuitId), headerError, log)
				}
			} else {
				var errCode byte = ctrl_msg.ErrorTypeMisconfiguredTerminator
				rh.fail(msg, attempt, route, errors.Wrapf(err, "unable to create dialer for [c/%s]", route.CircuitId), &errCode, log)
			}
		} else {
			var errCode byte = ctrl_msg.ErrorTypeMisconfiguredTerminator
			rh.fail(msg, attempt, route, errors.Wrapf(err, "error creating route for [c/%s]", route.CircuitId), &errCode, log)
		}
	}

	if err := rh.pool.QueueWithTimeout(dialF, time.Second*15); err != nil {
		log.WithError(err).Error("error queuing xgress dial to pool")
		rh.fail(msg, attempt, route, errors.Wrapf(err, "error creating route for [c/%s]", route.CircuitId), nil, log)
	}
}
