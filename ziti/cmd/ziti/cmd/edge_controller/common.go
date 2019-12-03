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

package edge_controller

import (
	"fmt"
	"github.com/google/uuid"
	"strings"
)

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
			list, err := filterEntitiesOfType(entityType, fmt.Sprintf("name=\"%s\"", name), false)
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
	ids, err := mapNamesToIDs("identities", nameOrId)

	if err != nil {
		return "", err
	}

	if len(ids) == 0 {
		return "", fmt.Errorf("invalid identity name: %s", nameOrId)
	}

	if len(ids) > 1 {
		return "", fmt.Errorf("too many identities with name: %s, use id", nameOrId)
	}

	return ids[0], nil
}

func mapCaNameToID(nameOrId string) (string, error) {
	ids, err := mapNamesToIDs("cas", nameOrId)

	if err != nil {
		return "", err
	}

	if len(ids) == 0 {
		return "", fmt.Errorf("invalid CA name: %s", nameOrId)
	}

	if len(ids) > 1 {
		return "", fmt.Errorf("too many CAs with name: %s, use id", nameOrId)
	}

	return ids[0], nil
}
