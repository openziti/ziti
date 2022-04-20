package mgmt_pb

func (request *ListServicesRequest) GetContentType() int32 {
	return int32(ContentType_ListServicesRequestType)
}

func (request *CreateRouterRequest) GetContentType() int32 {
	return int32(ContentType_CreateRouterRequestType)
}

func (request *InspectRequest) GetContentType() int32 {
	return int32(ContentType_InspectRequestType)
}

func (request *InspectResponse) GetContentType() int32 {
	return int32(ContentType_InspectResponseType)
}
