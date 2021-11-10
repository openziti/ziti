package api

import "github.com/Jeffail/gabs"

func SetJSONValue(container *gabs.Container, value interface{}, path ...string) {
	if _, err := container.Set(value, path...); err != nil {
		panic(err)
	}
}

func GetJsonValue(container *gabs.Container, path string) interface{} {
	return container.Path(path).Data()
}

func GetJsonString(container *gabs.Container, path string) string {
	v := GetJsonValue(container, path)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
