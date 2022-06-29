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
	"github.com/openziti/channel"
	"google.golang.org/protobuf/proto"
)

type Decoder struct{}

const DECODER = "mgmt"

func (d Decoder) Decode(msg *channel.Message) ([]byte, bool) {
	switch msg.ContentType {
	case int32(ContentType_ListServicesRequestType):
		data, err := channel.NewTraceMessageDecode(DECODER, "List Services Request").MarshalTraceMessageDecode()
		if err != nil {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

		return data, true

	case int32(ContentType_ListServicesResponseType):
		listServices := &ListServicesResponse{}
		if err := proto.Unmarshal(msg.Body, listServices); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "List Services Response")
			meta["services"] = len(listServices.Services)

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

	case int32(ContentType_CreateServiceRequestType):
		createService := &CreateServiceRequest{}
		if err := proto.Unmarshal(msg.Body, createService); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Create Service Request")
			meta["service"] = serviceToString(createService.Service)

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

	case int32(ContentType_RemoveServiceRequestType):
		removeService := &RemoveServiceRequest{}
		if err := proto.Unmarshal(msg.Body, removeService); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Remove Service Request")
			meta["serviceId"] = removeService.ServiceId

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

	case int32(ContentType_GetServiceRequestType):
		getService := &GetServiceRequest{}
		if err := proto.Unmarshal(msg.Body, getService); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Get Service Request")
			meta["serviceId"] = getService.ServiceId

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

	case int32(ContentType_GetServiceResponseType):
		getService := &GetServiceResponse{}
		if err := proto.Unmarshal(msg.Body, getService); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Get Service Response")
			meta["service"] = serviceToString(getService.Service)

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

		// terminator messages
	case int32(ContentType_ListTerminatorsRequestType):
		data, err := channel.NewTraceMessageDecode(DECODER, "List Terminators Request").MarshalTraceMessageDecode()
		if err != nil {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

		return data, true

	case int32(ContentType_ListTerminatorsResponseType):
		listTerminators := &ListTerminatorsResponse{}
		if err := proto.Unmarshal(msg.Body, listTerminators); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "List Terminators Response")
			meta["terminators"] = len(listTerminators.Terminators)

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

	case int32(ContentType_CreateTerminatorRequestType):
		createTerminator := &CreateTerminatorRequest{}
		if err := proto.Unmarshal(msg.Body, createTerminator); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Create Terminator Request")
			meta["terminator"] = terminatorToString(createTerminator.Terminator)

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
			meta := channel.NewTraceMessageDecode(DECODER, "Remove Terminator Request")
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

	case int32(ContentType_GetTerminatorRequestType):
		getTerminator := &GetTerminatorRequest{}
		if err := proto.Unmarshal(msg.Body, getTerminator); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Get Terminator Request")
			meta["terminatorId"] = getTerminator.TerminatorId

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

	case int32(ContentType_GetTerminatorResponseType):
		getTerminator := &GetTerminatorResponse{}
		if err := proto.Unmarshal(msg.Body, getTerminator); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Get Terminator Response")
			meta["terminator"] = terminatorToString(getTerminator.Terminator)

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

	case int32(ContentType_SetTerminatorCostRequestType):
		request := &SetTerminatorCostRequest{}
		if err := proto.Unmarshal(msg.Body, request); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Set Terminator Weight Request")
			meta["terminatorId"] = request.TerminatorId
			meta["staticCost"] = request.StaticCost
			meta["precedence"] = request.Precedence.String()
			meta["dynamicCost"] = request.DynamicCost
			meta["changeMask"] = request.UpdateMask

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
		// router messages
	case int32(ContentType_ListRoutersRequestType):
		data, err := channel.NewTraceMessageDecode(DECODER, "List Routers Request").MarshalTraceMessageDecode()
		if err != nil {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

		return data, true

	case int32(ContentType_ListRoutersResponseType):
		listRouters := &ListRoutersResponse{}
		if err := proto.Unmarshal(msg.Body, listRouters); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "List Routers Response")
			meta["routers"] = len(listRouters.Routers)

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

	case int32(ContentType_CreateRouterRequestType):
		createRouter := &CreateRouterRequest{}
		if err := proto.Unmarshal(msg.Body, createRouter); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Create Router Request")
			meta["router"] = routerToString(createRouter.Router)

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

	case int32(ContentType_RemoveRouterRequestType):
		removeRouter := &RemoveRouterRequest{}
		if err := proto.Unmarshal(msg.Body, removeRouter); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Remove Router Request")
			meta["routerId"] = removeRouter.RouterId

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

	case int32(ContentType_ListLinksRequestType):
		data, err := channel.NewTraceMessageDecode(DECODER, "List Links Request").MarshalTraceMessageDecode()
		if err != nil {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

		return data, true

	case int32(ContentType_ListLinksResponseType):
		listLinks := &ListLinksResponse{}
		if err := proto.Unmarshal(msg.Body, listLinks); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "List Links Response")
			meta["links"] = len(listLinks.Links)

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

	case int32(ContentType_SetLinkCostRequestType):
		setLinkCost := &SetLinkCostRequest{}
		if err := proto.Unmarshal(msg.Body, setLinkCost); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Set Link Cost Request")
			meta["linkId"] = setLinkCost.LinkId
			meta["cost"] = setLinkCost.Cost

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

	case int32(ContentType_SetLinkDownRequestType):
		setLinkDown := &SetLinkDownRequest{}
		if err := proto.Unmarshal(msg.Body, setLinkDown); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "Set Link Down Request")
			meta["linkId"] = setLinkDown.LinkId
			meta["down"] = setLinkDown.Down

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

	case int32(ContentType_ListCircuitsRequestType):
		data, err := channel.NewTraceMessageDecode(DECODER, "List Circuits Request").MarshalTraceMessageDecode()
		if err != nil {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

		return data, true

	case int32(ContentType_ListCircuitsResponseType):
		listCircuits := &ListCircuitsResponse{}
		if err := proto.Unmarshal(msg.Body, listCircuits); err == nil {
			meta := channel.NewTraceMessageDecode(DECODER, "List Circuits Response")
			meta["circuits"] = len(listCircuits.Circuits)

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
	}

	return nil, false
}

func serviceToString(service *Service) string {
	return fmt.Sprintf("{id=[%s]}", service.Id)
}

func terminatorToString(terminator *Terminator) string {
	return fmt.Sprintf("{id=[%s]}", terminator.Id)
}

func routerToString(router *Router) string {
	return fmt.Sprintf("{id=[%s] fingerprint=[%s] listener=[%s] connected=[%t]}", router.Id, router.Fingerprint, router.ListenerAddress, router.Connected)
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
