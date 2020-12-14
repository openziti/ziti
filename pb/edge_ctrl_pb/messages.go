package edge_ctrl_pb

import "github.com/openziti/fabric/controller/xt"

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

func (request *UpdateTerminatorRequest) GetContentType() int32 {
	return int32(ContentType_UpdateTerminatorRequestType)
}

func (request *RemoveTerminatorRequest) GetContentType() int32 {
	return int32(ContentType_RemoveTerminatorRequestType)
}

func (request *ValidateSessionsRequest) GetContentType() int32 {
	return int32(ContentType_ValidateSessionsRequestType)
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
