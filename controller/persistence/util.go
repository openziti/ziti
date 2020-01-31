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
	"fmt"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"github.com/pkg/errors"
	"strings"
)

const (
	RolePrefix   = "#"
	EntityPrefix = "@"
	AllRole      = "#all"
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

func validateRolesAndIds(field string, values []string) error {
	if len(values) > 1 && stringz.Contains(values, AllRole) {
		return validation.NewFieldError(fmt.Sprintf("if using %v, it should be the only role specified", AllRole), field, values)
	}

	var invalidKeys []string
	for _, entry := range values {
		if !strings.HasPrefix(entry, RolePrefix) && ! strings.HasPrefix(entry, EntityPrefix) {
			invalidKeys = append(invalidKeys, entry)
		}
	}
	if len(invalidKeys) > 0 {
		return validation.NewFieldError("role entries must prefixed with # (to indicate role attributes) or @ (to indicate a name or id)", field, invalidKeys)
	}
	return nil
}

func splitRolesAndIds(values []string) ([]string, []string, error) {
	var roles []string
	var ids []string
	for _, entry := range values {
		if strings.HasPrefix(entry, RolePrefix) {
			entry = strings.TrimPrefix(entry, RolePrefix)
			roles = append(roles, entry)
		} else if strings.HasPrefix(entry, EntityPrefix) {
			entry = strings.TrimPrefix(entry, EntityPrefix)
			ids = append(ids, entry)
		} else {
			return nil, nil, errors.Errorf("'%v' is neither role attribute (prefixed with %v) or an entity id or name (prefixed with %v)",
				entry, RolePrefix, EntityPrefix)
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

func roleRef(val string) string {
	return RolePrefix + val
}

func entityRef(val string) string {
	return EntityPrefix + val
}
