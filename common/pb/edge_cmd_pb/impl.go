package edge_cmd_pb

func (x *CreateEdgeTerminatorCommand) GetCommandType() int32 {
	return int32(CommandType_CreateEdgeTerminatorType)
}

func (x *ReplaceEnrollmentWithAuthenticatorCmd) GetCommandType() int32 {
	return int32(CommandType_ReplaceEnrollmentWithAuthenticatorType)
}

func (x *CreateEdgeRouterCmd) GetCommandType() int32 {
	return int32(CommandType_CreateEdgeRouterType)
}

func (x *CreateTransitRouterCmd) GetCommandType() int32 {
	return int32(CommandType_CreateTransitRouterType)
}

func (x *CreateIdentityWithEnrollmentsCmd) GetCommandType() int32 {
	return int32(CommandType_CreateIdentityWithEnrollmentsType)
}

func (x *CreateIdentityWithAuthenticatorsCmd) GetCommandType() int32 {
	return int32(CommandType_CreateIdentityWithAuthenticatorsType)
}

func (x *ReEnrollEdgeRouterCmd) GetCommandType() int32 {
	return int32(CommandType_ReEnrollEdgeRouterType)
}

func (x *UpdateServiceConfigsCmd) GetCommandType() int32 {
	return int32(CommandType_UpdateServiceConfigsType)
}

func EncodeTags(tags map[string]interface{}) (map[string]*TagValue, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	result := map[string]*TagValue{}

	for k, v := range tags {
		if v == nil {
			result[k] = &TagValue{
				Value: &TagValue_NilValue{
					NilValue: true,
				},
			}
		} else {
			switch val := v.(type) {
			case string:
				result[k] = &TagValue{
					Value: &TagValue_StringValue{
						StringValue: val,
					},
				}
			case bool:
				result[k] = &TagValue{
					Value: &TagValue_BoolValue{
						BoolValue: val,
					},
				}
			case float64:
				result[k] = &TagValue{
					Value: &TagValue_FpValue{
						FpValue: val,
					},
				}
			}
		}
	}
	return result, nil
}

func DecodeTags(tags map[string]*TagValue) map[string]interface{} {
	if len(tags) == 0 {
		return nil
	}
	result := map[string]interface{}{}

	for k, v := range tags {
		switch v.Value.(type) {
		case *TagValue_NilValue:
			result[k] = nil
		case *TagValue_BoolValue:
			result[k] = v.GetBoolValue()
		case *TagValue_StringValue:
			result[k] = v.GetStringValue()
		case *TagValue_FpValue:
			result[k] = v.GetFpValue()
		}
	}

	return result
}

func EncodeCtrlChanListeners(listeners map[string][]string) map[string]*CtrlChanListenerDetail {
	if len(listeners) == 0 {
		return nil
	}
	result := make(map[string]*CtrlChanListenerDetail, len(listeners))
	for addr, groups := range listeners {
		result[addr] = &CtrlChanListenerDetail{Groups: groups}
	}
	return result
}

func DecodeCtrlChanListeners(listeners map[string]*CtrlChanListenerDetail) map[string][]string {
	if len(listeners) == 0 {
		return nil
	}
	result := make(map[string][]string, len(listeners))
	for addr, detail := range listeners {
		result[addr] = detail.GetGroups()
	}
	return result
}
