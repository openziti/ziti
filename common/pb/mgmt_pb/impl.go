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
