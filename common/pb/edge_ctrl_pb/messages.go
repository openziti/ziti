package edge_ctrl_pb

import (
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/sdk-golang/ziti"
)

func (m *ClientHello) GetContentType() int32 {
	return int32(ContentType_ClientHelloType)
}

func (m *ServerHello) GetContentType() int32 {
	return int32(ContentType_ServerHelloType)
}

func (m *RequestClientReSync) GetContentType() int32 {
	return int32(ContentType_RequestClientReSyncType)
}

func (m *CreateCircuitRequest) GetContentType() int32 {
	return int32(ContentType_CreateCircuitRequestType)
}

func (m *CreateCircuitResponse) GetContentType() int32 {
	return int32(ContentType_CreateCircuitResponseType)
}

func (request *CreateTerminatorRequest) GetContentType() int32 {
	return int32(ContentType_CreateTerminatorRequestType)
}

func (request *CreateTerminatorRequest) GetXtPrecedence() xt.Precedence {
	if request.GetPrecedence() == TerminatorPrecedence_Failed {
		return xt.Precedences.Failed
	}
	if request.GetPrecedence() == TerminatorPrecedence_Required {
		return xt.Precedences.Required
	}
	return xt.Precedences.Default
}

func (request *CreateTerminatorV2Request) GetContentType() int32 {
	return int32(ContentType_CreateTerminatorV2RequestType)
}

func (request *CreateTerminatorV2Request) GetXtPrecedence() xt.Precedence {
	if request.GetPrecedence() == TerminatorPrecedence_Failed {
		return xt.Precedences.Failed
	}
	if request.GetPrecedence() == TerminatorPrecedence_Required {
		return xt.Precedences.Required
	}
	return xt.Precedences.Default
}

func (request *CreateTerminatorV2Response) GetContentType() int32 {
	return int32(ContentType_CreateTerminatorV2ResponseType)
}

func (request *Error) GetContentType() int32 {
	return int32(ContentType_ErrorType)
}

func (request *UpdateTerminatorRequest) GetContentType() int32 {
	return int32(ContentType_UpdateTerminatorRequestType)
}

func (request *RemoveTerminatorRequest) GetContentType() int32 {
	return int32(ContentType_RemoveTerminatorRequestType)
}

func (request *ValidateSessionsRequest) GetContentType() int32 {
	return int32(ContentType_ValidateSessionsRequestType)
}

func (request *HealthEventRequest) GetContentType() int32 {
	return int32(ContentType_HealthEventType)
}

func (request *CreateApiSessionRequest) GetContentType() int32 {
	return int32(ContentType_CreateApiSessionRequestType)
}

func (request *CreateApiSessionResponse) GetContentType() int32 {
	return int32(ContentType_CreateApiSessionResponseType)
}

func (m *CreateCircuitForServiceRequest) GetContentType() int32 {
	return int32(ContentType_CreateCircuitForServiceRequestType)
}

func (m *CreateCircuitForServiceResponse) GetContentType() int32 {
	return int32(ContentType_CreateCircuitForServiceResponseType)
}

func (m *ServicesList) GetContentType() int32 {
	return int32(ContentType_ServiceListType)
}

func (request *CreateTunnelTerminatorRequest) GetContentType() int32 {
	return int32(ContentType_CreateTunnelTerminatorRequestType)
}

func (request *EnrollmentExtendRouterVerifyRequest) GetContentType() int32 {
	return int32(ContentType_EnrollmentExtendRouterVerifyRequestType)
}

func (request *CreateTunnelTerminatorRequest) GetXtPrecedence() xt.Precedence {
	if request.GetPrecedence() == TerminatorPrecedence_Failed {
		return xt.Precedences.Failed
	}
	if request.GetPrecedence() == TerminatorPrecedence_Required {
		return xt.Precedences.Required
	}
	return xt.Precedences.Default
}

func (request *CreateTunnelTerminatorResponse) GetContentType() int32 {
	return int32(ContentType_CreateTunnelTerminatorResponseType)
}

func (request *UpdateTunnelTerminatorRequest) GetContentType() int32 {
	return int32(ContentType_UpdateTunnelTerminatorRequestType)
}

func GetPrecedence(p ziti.Precedence) TerminatorPrecedence {
	if p == ziti.PrecedenceRequired {
		return TerminatorPrecedence_Required
	}
	if p == ziti.PrecedenceFailed {
		return TerminatorPrecedence_Failed
	}
	return TerminatorPrecedence_Default
}

func (self TerminatorPrecedence) GetZitiLabel() rest_model.TerminatorPrecedence {
	if self == TerminatorPrecedence_Default {
		return rest_model.TerminatorPrecedenceDefault
	}

	if self == TerminatorPrecedence_Required {
		return rest_model.TerminatorPrecedenceRequired
	}

	if self == TerminatorPrecedence_Failed {
		return rest_model.TerminatorPrecedenceFailed
	}

	return rest_model.TerminatorPrecedenceDefault
}
