/*
	Copyright NetFoundry, Inc.

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

type RequirePartiallyAuthenticated struct{}

var isPartiallyAuthenticated = &RequirePartiallyAuthenticated{}

func IsPartiallyAuthenticated() *RequirePartiallyAuthenticated {
	return isPartiallyAuthenticated
}

func (ir *RequirePartiallyAuthenticated) IsAllowed(identityPerms ...string) bool {
	for _, p := range identityPerms {
		if p == PartiallyAuthenticatePermission {
			return true
		}
	}

	return false
}