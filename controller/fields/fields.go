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

package fields

import (
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"strings"
)

type UpdatedFields interface {
	boltz.FieldChecker
	ToSlice() []string
	AddField(field string) UpdatedFields
	AddFields(fields ...string) UpdatedFields
	RemoveFields(fields ...string) UpdatedFields
	ConcatNestedNames() UpdatedFields
	FilterMaps(mapNames ...string) UpdatedFields
}

var _ UpdatedFields = (UpdatedFieldsMap)(nil)

type UpdatedFieldsMap map[string]struct{}

func (self UpdatedFieldsMap) ToSlice() []string {
	var result []string
	for k := range self {
		result = append(result, k)
	}
	return result
}

func (self UpdatedFieldsMap) IsUpdated(key string) bool {
	_, ok := self[key]
	return ok
}

func (self UpdatedFieldsMap) AddField(key string) UpdatedFields {
	self[key] = struct{}{}
	return self
}

func (self UpdatedFieldsMap) AddFields(fields ...string) UpdatedFields {
	for _, field := range fields {
		self[field] = struct{}{}
	}
	return self
}

func (self UpdatedFieldsMap) RemoveFields(fields ...string) UpdatedFields {
	for _, field := range fields {
		delete(self, field)
	}
	return self
}

func (self UpdatedFieldsMap) ConcatNestedNames() UpdatedFields {
	for key, val := range self {
		if strings.Contains(key, ".") {
			delete(self, key)
			key = strings.ReplaceAll(key, ".", "")
			self[key] = val
		}
	}
	return self
}

func (self UpdatedFieldsMap) FilterMaps(mapNames ...string) UpdatedFields {
	nameMap := map[string]string{}
	for _, name := range mapNames {
		nameMap[name] = name + "."
	}
	for key := range self {
		for name, dotName := range nameMap {
			if strings.HasPrefix(key, dotName) {
				delete(self, key)
				self[name] = struct{}{}
				break
			}
		}
	}
	return self
}

func UpdatedFieldsToSlice(fields UpdatedFields) ([]string, error) {
	if fields == nil {
		return nil, nil
	}
	result := fields.ToSlice()
	if len(result) == 0 {
		return nil, errors.New("no fields updated, nothing to do")
	}
	return result, nil
}

func SliceToUpdatedFields(val []string) UpdatedFields {
	if len(val) == 0 {
		return nil
	}
	result := UpdatedFieldsMap{}
	for _, k := range val {
		result[k] = struct{}{}
	}
	return result
}
