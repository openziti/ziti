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
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/pkg/errors"
	"os"
	"strings"
)

func mapNameToID(entityType string, val string, o api.Options) (string, error) {
	list, _, err := filterEntitiesOfType(entityType, fmt.Sprintf("id=\"%s\"", val), false, nil, o.Timeout, o.Verbose)
	if err != nil {
		return "", err
	}

	if len(list) > 0 {
		return val, nil
	}

	list, _, err = filterEntitiesOfType(entityType, fmt.Sprintf("name=\"%s\"", val), false, nil, o.Timeout, o.Verbose)
	if err != nil {
		return "", err
	}

	if len(list) < 1 {
		return "", errors.Errorf("no %v found with id or name %v", entityType, val)
	}

	if len(list) > 1 {
		return "", errors.Errorf("multiple %v found for name %v, please use id instead", entityType, val)
	}

	entity := list[0]
	entityId, _ := entity.Path("id").Data().(string)
	if val, found := os.LookupEnv("ZITI_CLI_DEBUG"); found && strings.EqualFold("true", val) {
		fmt.Printf("Found %v with id %v for name %v\n", entityType, entityId, val)
	}
	return entityId, nil
}

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

func mapNamesToIDs(entityType string, o api.Options, list ...string) ([]string, error) {
	var result []string
	for _, val := range list {
		if strings.HasPrefix(val, "id") {
			id := strings.TrimPrefix(val, "id:")
			result = append(result, id)
		} else {
			query := fmt.Sprintf(`id = "%s" or name="%s"`, val, val)
			if strings.HasPrefix(val, "name") {
				name := strings.TrimPrefix(val, "name:")
				query = fmt.Sprintf(`name="%s"`, name)
			}
			list, _, err := filterEntitiesOfType(entityType, query, false, nil, o.Timeout, o.Verbose)
			if err != nil {
				return nil, err
			}

			if len(list) > 1 {
				fmt.Printf("Found multiple %v matching %v. Please specify which you want by prefixing with id: or name:\n", entityType, val)
				return nil, errors.Errorf("ambigous if %v is id or name", val)
			}

			for _, entity := range list {
				entityId, _ := entity.Path("id").Data().(string)
				result = append(result, entityId)
				if val, found := os.LookupEnv("ZITI_CLI_DEBUG"); found && strings.EqualFold("true", val) {
					fmt.Printf("Found %v with id %v for name %v\n", entityType, entityId, val)
				}
			}
		}
	}
	return result, nil
}

func mapIdentityNameToID(nameOrId string, o api.Options) (string, error) {
	return mapNameToID("identities", nameOrId, o)
}

func mapCaNameToID(nameOrId string, o api.Options) (string, error) {
	return mapNameToID("cas", nameOrId, o)
}
