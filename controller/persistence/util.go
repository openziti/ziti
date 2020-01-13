/*
	Copyright 2019 Netfoundry, Inc.

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

package persistence

import (
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"strings"
)

func ToInFilter(ids ...string) string {
	builder := strings.Builder{}
	builder.WriteString("id in [")
	if len(ids) > 0 {
		builder.WriteRune('"')
		builder.WriteString(ids[0])
		builder.WriteRune('"')
	}
	for _, id := range ids[1:] {
		builder.WriteString(", ")
		builder.WriteRune('"')
		builder.WriteString(id)
		builder.WriteRune('"')
	}
	builder.WriteString("]")
	return builder.String()
}

func Field(elements ...string) string {
	return strings.Join(elements, ".")
}

func splitRolesAndIds(values []string) ([]string, []string, error) {
	var roles []string
	var ids []string
	for _, entry := range values {
		if strings.HasPrefix(entry, "#") {
			entry = strings.TrimPrefix(entry, "#")
			roles = append(roles, entry)
		} else if strings.HasPrefix(entry, "@") {
			entry = strings.TrimPrefix(entry, "@")
			ids = append(ids, entry)
		} else {
			return nil, nil, errors.Errorf("'%v' is neither role attribute (prefixed with #) or an entity id or name (prefixed with @)", entry)
		}
	}
	return roles, ids, nil
}

func FieldValuesToIds(new []boltz.FieldTypeAndValue) []string {
	var entityRoles []string
	for _, fv := range new {
		entityRoles = append(entityRoles, string(fv.Value))
	}
	return entityRoles
}

type UpdateTimeOnlyFieldChecker struct{}

func (u UpdateTimeOnlyFieldChecker) IsUpdated(string) bool {
	return false
}
