package edge_cmd_pb

import "github.com/pkg/errors"

func (x *CreateEdgeTerminatorCommand) GetCommandType() int32 {
	return int32(CommandType_CreateEdgeTerminatorType)
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

func EncodeJson(tags map[string]interface{}) (*JsonMap, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	result := map[string]*JsonValue{}

	for k, v := range tags {
		if v == nil {
			result[k] = &JsonValue{
				Value: &JsonValue_NilValue{
					NilValue: true,
				},
			}
		} else {
			switch val := v.(type) {
			case string:
				result[k] = &JsonValue{
					Value: &JsonValue_StringValue{
						StringValue: val,
					},
				}
			case bool:
				result[k] = &JsonValue{
					Value: &JsonValue_BoolValue{
						BoolValue: val,
					},
				}
			case float64:
				result[k] = &JsonValue{
					Value: &JsonValue_FpValue{
						FpValue: val,
					},
				}
			case map[string]interface{}:
				mapVal, err := EncodeJson(val)
				if err != nil {
					return nil, err
				}
				result[k] = &JsonValue{
					Value: &JsonValue_MapValue{
						MapValue: mapVal,
					},
				}
			default:
				return nil, errors.Errorf("unhandled json type: %T", v)
			}
		}
	}
	return &JsonMap{Value: result}, nil
}

func DecodeJson(m *JsonMap) map[string]interface{} {
	if m == nil {
		return nil
	}

	result := map[string]interface{}{}

	for k, v := range m.Value {
		switch v.Value.(type) {
		case *JsonValue_NilValue:
			result[k] = nil
		case *JsonValue_BoolValue:
			result[k] = v.GetBoolValue()
		case *JsonValue_StringValue:
			result[k] = v.GetStringValue()
		case *JsonValue_FpValue:
			result[k] = v.GetFpValue()
		case *JsonValue_MapValue:
			result[k] = DecodeJson(v.GetMapValue())
		}
	}

	return result
}
