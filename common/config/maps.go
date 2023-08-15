package config

import (
	"github.com/pkg/errors"
)

func ToJsonCompatibleMap(m map[any]any) (map[string]any, error) {
	result := map[string]any{}
	for k, v := range m {
		if subMap, ok := v.(map[any]any); ok {
			val, err := ToJsonCompatibleMap(subMap)
			if err != nil {
				return nil, err
			}
			v = val
		}

		if subSlice, ok := v.([]any); ok {
			for idx, sliceVal := range subSlice {
				if subMap, ok := sliceVal.(map[any]any); ok {
					val, err := ToJsonCompatibleMap(subMap)
					if err != nil {
						return nil, err
					}
					subSlice[idx] = val
				}
			}
		}

		if s, ok := k.(string); ok {
			result[s] = v
		} else {
			return nil, errors.Errorf("map contains invalid key of type %T", k)
		}
	}
	return result, nil
}
