package edge_cmd_pb

import (
	"encoding/json"
	"github.com/pkg/errors"
	"strings"
)

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

func EncodeJsonMap(tags map[string]interface{}) (*JsonMap, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	result := map[string]*JsonValue{}

	for k, v := range tags {
		encoded, err := EncodeJsonValue(v)
		if err != nil {
			return nil, err
		}
		result[k] = encoded
	}
	return &JsonMap{Value: result}, nil
}

func EncodeJsonValue(v interface{}) (*JsonValue, error) {
	var result *JsonValue
	if v == nil {
		result = &JsonValue{
			Value: &JsonValue_NilValue{
				NilValue: true,
			},
		}
	}

	switch val := v.(type) {
	case string:
		result = &JsonValue{
			Value: &JsonValue_StringValue{
				StringValue: val,
			},
		}
	case bool:
		result = &JsonValue{
			Value: &JsonValue_BoolValue{
				BoolValue: val,
			},
		}
	case float64:
		result = &JsonValue{
			Value: &JsonValue_FpValue{
				FpValue: val,
			},
		}
	case int64:
		result = &JsonValue{
			Value: &JsonValue_Int64Value{
				Int64Value: val,
			},
		}
	case json.Number:
		isInt := false
		if strings.IndexByte(val.String(), '.') < 0 {
			n, err := val.Int64()
			if err == nil {
				result = &JsonValue{
					Value: &JsonValue_Int64Value{
						Int64Value: n,
					},
				}
				isInt = true
			}
		}

		if !isInt {
			n, err := val.Float64()
			if err != nil {
				return nil, errors.Errorf("unable to parse %v to int64 or float64", val.String())
			}
			result = &JsonValue{
				Value: &JsonValue_FpValue{
					FpValue: n,
				},
			}
		}
	case map[string]interface{}:
		mapVal, err := EncodeJsonMap(val)
		if err != nil {
			return nil, err
		}
		result = &JsonValue{
			Value: &JsonValue_MapValue{
				MapValue: mapVal,
			},
		}
	case []interface{}:
		l := &JsonList{}
		for _, v := range val {
			entryVal, err := EncodeJsonValue(v)
			if err != nil {
				return nil, err
			}
			l.Value = append(l.Value, entryVal)
		}
		result = &JsonValue{
			Value: &JsonValue_ListValue{
				ListValue: l,
			},
		}
	default:
		return nil, errors.Errorf("unhandled json type: %T", v)
	}
	return result, nil
}

func DecodeJsonMap(m *JsonMap) (map[string]interface{}, error) {
	if m == nil {
		return nil, nil
	}

	result := map[string]interface{}{}

	for k, v := range m.Value {
		val, err := DecodeJsonValue(v)
		if err != nil {
			return nil, err
		}
		result[k] = val
	}

	return result, nil
}

func DecodeJsonValue(v *JsonValue) (interface{}, error) {
	switch v.Value.(type) {
	case *JsonValue_NilValue:
		return nil, nil
	case *JsonValue_BoolValue:
		return v.GetBoolValue(), nil
	case *JsonValue_StringValue:
		return v.GetStringValue(), nil
	case *JsonValue_FpValue:
		return v.GetFpValue(), nil
	case *JsonValue_Int64Value:
		return v.GetInt64Value(), nil
	case *JsonValue_ListValue:
		return DecodeJsonList(v.GetListValue())
	case *JsonValue_MapValue:
		return DecodeJsonMap(v.GetMapValue())
	default:
		return nil, errors.Errorf("unhandled json type %T", v.Value)
	}
}

func DecodeJsonList(l *JsonList) ([]interface{}, error) {
	var result []interface{}
	for _, v := range l.Value {
		decoded, err := DecodeJsonValue(v)
		if err != nil {
			return nil, err
		}
		result = append(result, decoded)
	}
	return result, nil
}
