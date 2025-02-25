package mgmt_pb

func (request *InspectRequest) GetContentType() int32 {
	return int32(ContentType_InspectRequestType)
}

func (request *InspectResponse) GetContentType() int32 {
	return int32(ContentType_InspectResponseType)
}

func (request *RaftMemberListResponse) GetContentType() int32 {
	return int32(ContentType_RaftListMembersResponseType)
}

func (request *ValidateTerminatorsRequest) GetContentType() int32 {
	return int32(ContentType_ValidateTerminatorsRequestType)
}

func (request *ValidateTerminatorsResponse) GetContentType() int32 {
	return int32(ContentType_ValidateTerminatorResponseType)
}

func (request *TerminatorDetail) GetContentType() int32 {
	return int32(ContentType_ValidateTerminatorResultType)
}

func (request *ValidateRouterLinksRequest) GetContentType() int32 {
	return int32(ContentType_ValidateRouterLinksRequestType)
}

func (request *ValidateRouterLinksResponse) GetContentType() int32 {
	return int32(ContentType_ValidateRouterLinksResponseType)
}

func (request *RouterLinkDetails) GetContentType() int32 {
	return int32(ContentType_ValidateRouterLinksResultType)
}

func (request *ValidateRouterSdkTerminatorsRequest) GetContentType() int32 {
	return int32(ContentType_ValidateRouterSdkTerminatorsRequestType)
}

func (request *ValidateRouterSdkTerminatorsResponse) GetContentType() int32 {
	return int32(ContentType_ValidateRouterSdkTerminatorsResponseType)
}

func (request *RouterSdkTerminatorsDetails) GetContentType() int32 {
	return int32(ContentType_ValidateRouterSdkTerminatorsResultType)
}

func (request *ValidateRouterErtTerminatorsRequest) GetContentType() int32 {
	return int32(ContentType_ValidateRouterErtTerminatorsRequestType)
}

func (request *ValidateRouterErtTerminatorsResponse) GetContentType() int32 {
	return int32(ContentType_ValidateRouterErtTerminatorsResponseType)
}

func (request *RouterErtTerminatorsDetails) GetContentType() int32 {
	return int32(ContentType_ValidateRouterErtTerminatorsResultType)
}

func (request *ValidateRouterDataModelRequest) GetContentType() int32 {
	return int32(ContentType_ValidateRouterDataModelRequestType)
}

func (request *ValidateRouterDataModelResponse) GetContentType() int32 {
	return int32(ContentType_ValidateRouterDataModelResponseType)
}

func (request *RouterDataModelDetails) GetContentType() int32 {
	return int32(ContentType_ValidateRouterDataModelResultType)
}

func (request *ValidateIdentityConnectionStatusesRequest) GetContentType() int32 {
	return int32(ContentType_ValidateIdentityConnectionStatusesRequestType)
}

func (request *ValidateIdentityConnectionStatusesResponse) GetContentType() int32 {
	return int32(ContentType_ValidateIdentityConnectionStatusesResponseType)
}

func (request *RouterIdentityConnectionStatusesDetails) GetContentType() int32 {
	return int32(ContentType_ValidateIdentityConnectionStatusesResultType)
}

func (x *InitRequest) GetContentType() int32 {
	return int32(ContentType_RaftInit)
}
