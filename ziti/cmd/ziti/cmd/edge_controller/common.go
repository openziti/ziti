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

package edge_controller

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"strings"
)

func mapNameToID(entityType string, val string) (string, error) {
	// If we can parse it as a UUID, treat it as such
	_, err := uuid.Parse(val)
	if err == nil {
		return val, nil
	}

	// Allow UUID formatted names to be recognized with a name: prefix
	name := strings.TrimPrefix(val, "name:")
	list, err := filterEntitiesOfType(entityType, fmt.Sprintf("name=\"%s\"", name), false, nil)
	if err != nil {
		return "", err
	}

	if len(list) < 1 {
		return "", errors.Errorf("no %v found for name %v", entityType, val)
	}

	if len(list) > 1 {
		return "", errors.Errorf("multiple %v found for name %v, please use id instead", entityType, val)
	}

	entity := list[0]
	entityId, _ := entity.Path("id").Data().(string)
	fmt.Printf("Found %v with id %v for name %v\n", entityType, entityId, val)
	return entityId, nil
}

func mapIdToName(entityType string, val string) (string, error) {
	list, err := filterEntitiesOfType(entityType, fmt.Sprintf(`id="%s"`, val), false, nil)
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

func mapNamesToIDs(entityType string, list ...string) ([]string, error) {
	var result []string
	for _, val := range list {

		// If we can parse it as a UUID, treat it as such
		_, err := uuid.Parse(val)
		if err == nil {
			result = append(result, val)
		} else {
			// Allow UUID formatted names to be recognized with a name: prefix
			name := strings.TrimPrefix(val, "name:")
			list, err := filterEntitiesOfType(entityType, fmt.Sprintf("name=\"%s\"", name), false, nil)
			if err != nil {
				return nil, err
			}

			for _, entity := range list {
				entityId, _ := entity.Path("id").Data().(string)
				result = append(result, entityId)
				fmt.Printf("Found %v with id %v for name %v\n", entityType, entityId, val)
			}
		}
	}
	return result, nil
}

func mapIdentityNameToID(nameOrId string) (string, error) {
	return mapNameToID("identities", nameOrId)
}

func mapCaNameToID(nameOrId string) (string, error) {
	return mapNameToID("cas", nameOrId)
}
