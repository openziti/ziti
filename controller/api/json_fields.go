package api

import (
	"encoding/json"
	"github.com/openziti/fabric/controller/apierror"
	"strings"
)

type JsonFields map[string]bool

func (j JsonFields) IsUpdated(key string) bool {
	_, ok := j[key]
	return ok
}

func (j JsonFields) AddField(key string) {
	j[key] = true
}

func (j JsonFields) ConcatNestedNames() JsonFields {
	for key, val := range j {
		if strings.Contains(key, ".") {
			delete(j, key)
			key = strings.ReplaceAll(key, ".", "")
			j[key] = val
		}
	}
	return j
}

func (j JsonFields) FilterMaps(mapNames ...string) JsonFields {
	nameMap := map[string]string{}
	for _, name := range mapNames {
		nameMap[name] = name + "."
	}
	for key := range j {
		for name, dotName := range nameMap {
			if strings.HasPrefix(key, dotName) {
				delete(j, key)
				j[name] = true
				break
			}
		}
	}
	return j
}

func GetFields(body []byte) (JsonFields, error) {
	jsonMap := map[string]interface{}{}
	err := json.Unmarshal(body, &jsonMap)

	if err != nil {
		return nil, apierror.GetJsonParseError(err, body)
	}

	resultMap := JsonFields{}
	GetJsonFields("", jsonMap, resultMap)
	return resultMap, nil
}

func GetJsonFields(prefix string, m map[string]interface{}, result JsonFields) {
	for k, v := range m {
		name := k
		if subMap, ok := v.(map[string]interface{}); ok {
			GetJsonFields(prefix+name+".", subMap, result)
		} else {
			isSet := v != nil
			result[prefix+name] = isSet
		}
	}
}
