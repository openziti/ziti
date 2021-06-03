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

package ctrl_pb

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"strconv"
	"strings"
)

type Decoder struct{}

const DECODER = "ctrl"

func (d Decoder) Decode(msg *channel2.Message) ([]byte, bool) {
	switch msg.ContentType {
	case int32(ContentType_SessionRequestType):
		sessionRequest := &SessionRequest{}
		if err := proto.Unmarshal(msg.Body, sessionRequest); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Session Request")
			meta["ingressId"] = sessionRequest.IngressId
			meta["serviceId"] = sessionRequest.ServiceId
			headers := make([]string, 0)
			for k := range sessionRequest.PeerData {
				headers = append(headers, strconv.Itoa(int(k)))
			}
			meta["peerData"] = strings.Join(headers, ",")

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_CreateTerminatorRequestType):
		createTerminator := &CreateTerminatorRequest{}
		if err := proto.Unmarshal(msg.Body, createTerminator); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Create Terminator Request")
			meta["terminator"] = terminatorToString(createTerminator)

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				pfxlog.Logger().Errorf("unexpected error (%s)", err)
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_RemoveTerminatorRequestType):
		removeTerminator := &RemoveTerminatorRequest{}
		if err := proto.Unmarshal(msg.Body, removeTerminator); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Remove Terminator Request")
			meta["terminatorId"] = removeTerminator.TerminatorId

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				pfxlog.Logger().Errorf("unexpected error (%s)", err)
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_ValidateTerminatorsRequestType):
		request := &ValidateTerminatorsRequest{}
		if err := proto.Unmarshal(msg.Body, request); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Validate Terminators")

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				pfxlog.Logger().Errorf("unexpected error (%s)", err)
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ctrl_msg.SessionSuccessType):
		meta := channel2.NewTraceMessageDecode(DECODER, "Session Success Response")
		meta["sessionId"] = string(msg.Body)
		meta["address"] = string(msg.Headers[ctrl_msg.SessionSuccessAddressHeader])

		data, err := meta.MarshalTraceMessageDecode()
		if err != nil {
			return nil, true
		}

		return data, true

	case int32(ctrl_msg.SessionFailedType):
		meta := channel2.NewTraceMessageDecode(DECODER, "Session Failed Response")
		message := string(msg.Body)
		if message != "" {
			meta["message"] = message
		}

		data, err := meta.MarshalTraceMessageDecode()
		if err != nil {
			return nil, true
		}

		return data, true

	case int32(ContentType_DialType):
		connect := &Dial{}
		if err := proto.Unmarshal(msg.Body, connect); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Dial")
			meta["linkId"] = connect.LinkId
			meta["address"] = connect.Address
			meta["routerId"] = connect.RouterId

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_LinkType):
		link := &Link{}
		if err := proto.Unmarshal(msg.Body, link); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Link")
			meta["id"] = link.Id

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_FaultType):
		fault := &Fault{}
		if err := proto.Unmarshal(msg.Body, fault); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Fault")
			meta["subject"] = fault.Subject.String()
			meta["id"] = fault.Id

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_RouteType):
		route := &Route{}
		if err := proto.Unmarshal(msg.Body, route); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Route")
			meta["sessionId"] = route.SessionId
			if route.Egress != nil {
				meta["egress.address"] = route.Egress.Address
				meta["egress.destination"] = route.Egress.Destination
			}
			for i, forward := range route.Forwards {
				meta[fmt.Sprintf("forward[%d].srcAddress", i)] = forward.SrcAddress
				meta[fmt.Sprintf("forward[%d].dstAddress", i)] = forward.DstAddress
			}

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_UnrouteType):
		unroute := &Unroute{}
		if err := proto.Unmarshal(msg.Body, unroute); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Unroute")
			meta["sessionId"] = unroute.SessionId

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
		}

	case int32(ContentType_MetricsType):
		metricsMsg := &metrics_pb.MetricsMessage{}
		if err := proto.Unmarshal(msg.Body, metricsMsg); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Metrics")

			for name, metric := range metricsMsg.Histograms {
				meta[name+".min"] = metric.Min
				meta[name+".mean"] = metric.Mean
				meta[name+".max"] = metric.Max
				meta[name+".p95"] = metric.P95
				meta[name+".p99"] = metric.P99
			}

			for name, metric := range metricsMsg.Meters {
				meta[name+".count"] = metric.Count
				meta[name+".meanRate"] = metric.MeanRate
				meta[name+".m1Rate"] = metric.M1Rate
				meta[name+".m5Rate"] = metric.M5Rate
				meta[name+".m15Rate"] = metric.M15Rate
			}

			for name, counter := range metricsMsg.IntervalCounters {
				for _, bucket := range counter.Buckets {
					for key, val := range bucket.Values {
						meta[name+"."+key+"["+strconv.FormatInt(bucket.IntervalStartUTC, 10)+"]"] = val
					}
				}
			}

			data, err := meta.MarshalTraceMessageDecode()
			if err != nil {
				return nil, true
			}

			return data, true

		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
		}

	case int32(ctrl_msg.RouteResultType):
		meta := channel2.NewTraceMessageDecode(DECODER, "Route Result")
		meta["sessionId"] = string(msg.Body)
		meta["attempt"], _ = msg.GetUint32Header(ctrl_msg.RouteResultAttemptHeader)
		success, _ := msg.GetBoolHeader(ctrl_msg.RouteResultSuccessHeader)
		meta["success"] = success
		if !success {
			meta["errormsg"], _ = msg.GetStringHeader(ctrl_msg.RouteResultErrorHeader)
		}

		data, err := meta.MarshalTraceMessageDecode()
		if err != nil {
			return nil, true
		}

		return data, true
	}

	return nil, false
}

func terminatorToString(request *CreateTerminatorRequest) string {
	return fmt.Sprintf("{serviceId=[%s], binding=[%s], address=[%v]}", request.ServiceId, request.Binding, request.Address)
}
