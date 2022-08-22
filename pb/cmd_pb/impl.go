package cmd_pb

import (
	"encoding/binary"
	"google.golang.org/protobuf/proto"
)

// TypedMessage instances are protobuf messages which know their command type
type TypedMessage interface {
	proto.Message
	GetCommandType() int32
}

// EncodeProtobuf returns the encoded message, prefixed with the command type
func EncodeProtobuf(v TypedMessage) ([]byte, error) {
	b, err := proto.Marshal(v)
	if err != nil {
		return nil, err
	}
	result := make([]byte, len(b)+4)
	binary.BigEndian.PutUint32(result, uint32(v.GetCommandType()))
	copy(result[4:], b)
	return result, nil
}

func (x *CreateEntityCommand) GetCommandType() int32 {
	return int32(CommandType_CreateEntityType)
}

func (x *UpdateEntityCommand) GetCommandType() int32 {
	return int32(CommandType_UpdateEntityType)
}

func (x *DeleteEntityCommand) GetCommandType() int32 {
	return int32(CommandType_DeleteEntityType)
}

func (x *SyncSnapshotCommand) GetCommandType() int32 {
	return int32(CommandType_SyncSnapshot)
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
