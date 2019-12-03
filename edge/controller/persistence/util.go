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