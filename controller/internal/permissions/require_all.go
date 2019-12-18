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

package permissions

type RequireAll struct {
	required []string
}

func NewRequireAll(perms ...string) *RequireAll {
	ra := &RequireAll{
		required: perms,
	}
	return ra
}

func (ra *RequireAll) IsAllowed(identityPerms ...string) bool {
	ps := map[string]bool{}

	if len(identityPerms) < len(ra.required) {
		return false
	}

	for _, p := range identityPerms {
		ps[p] = true

		//short cut admins
		if p == AdminPermission {
			return true
		}
	}

	for _, r := range ra.required {
		if _, ok := ps[r]; !ok {
			return false
		}
	}
	return true
}
