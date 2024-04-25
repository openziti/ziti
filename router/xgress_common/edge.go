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

package xgress_common

import "strings"

const JwtTokenPrefix = "ey"
const JwtExpectedDelimCount = 2

// IsBearerToken checks to see if a string has the basic structure required of a JWT.
func IsBearerToken(s string) bool {
	if strings.HasPrefix(s, JwtTokenPrefix) {
		sectionCount := 0
		for _, ch := range s {
			if ch == '.' {
				sectionCount++
				if sectionCount > 3 {
					return false
				}
			}
		}

		return sectionCount == JwtExpectedDelimCount
	}

	return false
}
