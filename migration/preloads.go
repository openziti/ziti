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

package migration

import "github.com/jinzhu/gorm"

type Preloads map[string][]interface{}

func (p Preloads) ApplyToQuery(q *gorm.DB) *gorm.DB {
	for f, c := range p {
		if c == nil {
			q = q.Preload(f)
		} else {
			// c[0] = query
			// c[1..n] = binds for c[0]
			q = q.Preload(f, c...)
		}

	}
	return q
}
