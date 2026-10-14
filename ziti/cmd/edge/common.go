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

package edge

import (
	"fmt"

	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/pkg/errors"
)

func mapIdToName(entityType string, val string, o api.Options) (string, error) {
	list, _, err := filterEntitiesOfType(entityType, fmt.Sprintf(`id="%s"`, val), false, nil, o.Timeout, o.Verbose)
	if err != nil {
		return "", err
	}

	if len(list) < 1 {
		return "", errors.Errorf("no %v found for id %v", entityType, val)
	}

	entity := list[0]
	name, _ := entity.Path("name").Data().(string)
	return name, nil
}
