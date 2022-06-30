/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package api

import (
	"fmt"
	"github.com/Jeffail/gabs"
	gabs2 "github.com/Jeffail/gabs/v2"
	"strings"
)

func SetJSONValue(container *gabs.Container, value interface{}, path ...string) {
	if _, err := container.Set(value, path...); err != nil {
		panic(err)
	}
}

func GetJsonValue(container *gabs.Container, path string) interface{} {
	if child := container.Path(path); child != nil {
		return child.Data()
	}
	return nil
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

func Wrap(c *gabs.Container) *GabsWrapper {
	return &GabsWrapper{Container: c}
}

func Wrap2(c *gabs2.Container) *Gabs2Wrapper {
	return &Gabs2Wrapper{Container: c}
}

type GabsWrapper struct {
	*gabs.Container
}

func (self *GabsWrapper) String(path string) string {
	child := self.Path(path)
	if child == nil || child.Data() == nil {
		return ""
	}
	return self.tostring(child.Data())
}

func (self *GabsWrapper) Bool(path string) bool {
	child := self.Path(path)
	if child == nil || child.Data() == nil {
		return false
	}
	if val, ok := child.Data().(bool); ok {
		return val
	}
	return strings.EqualFold("true", fmt.Sprintf("%v", child.Data()))
}

func (self *GabsWrapper) tostring(val interface{}) string {
	if val, ok := val.(string); ok {
		return val
	}
	return fmt.Sprintf("%v", val)
}

func (self *GabsWrapper) Float64(path string) float64 {
	child := self.Path(path)
	if child == nil || child.Data() == nil {
		return 0
	}
	if val, ok := child.Data().(float64); ok {
		return val
	}
	return 0
}

func (self *GabsWrapper) StringSlice(path string) []string {
	child := self.Path(path)
	if child == nil || child.Data() == nil {
		return nil
	}
	if val, ok := child.Data().([]string); ok {
		return val
	}
	if vals, ok := child.Data().([]interface{}); ok {
		var result []string
		for _, val := range vals {
			result = append(result, self.tostring(val))
		}
		return result
	}
	return nil
}

type Gabs2Wrapper struct {
	*gabs2.Container
}

func (self *Gabs2Wrapper) String(path string) string {
	child := self.Path(path)
	if child == nil || child.Data() == nil {
		return ""
	}
	return self.tostring(child.Data())
}

func (self *Gabs2Wrapper) Bool(path string) bool {
	child := self.Path(path)
	if child == nil || child.Data() == nil {
		return false
	}
	if val, ok := child.Data().(bool); ok {
		return val
	}
	return strings.EqualFold("true", fmt.Sprintf("%v", child.Data()))
}

func (self *Gabs2Wrapper) tostring(val interface{}) string {
	if val, ok := val.(string); ok {
		return val
	}
	return fmt.Sprintf("%v", val)
}

func (self *Gabs2Wrapper) Float64(path string) float64 {
	child := self.Path(path)
	if child == nil || child.Data() == nil {
		return 0
	}
	if val, ok := child.Data().(float64); ok {
		return val
	}
	return 0
}

func (self *Gabs2Wrapper) StringSlice(path string) []string {
	child := self.Path(path)
	if child == nil || child.Data() == nil {
		return nil
	}
	if val, ok := child.Data().([]string); ok {
		return val
	}
	if vals, ok := child.Data().([]interface{}); ok {
		var result []string
		for _, val := range vals {
			result = append(result, self.tostring(val))
		}
		return result
	}
	return nil
}
