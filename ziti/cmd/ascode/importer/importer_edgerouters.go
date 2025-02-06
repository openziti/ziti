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

package importer

import (
	"errors"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/ascode"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"slices"
)

func (importer *Importer) IsEdgeRouterImportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "edge-router") ||
		slices.Contains(args, "er")
}

func (importer *Importer) ProcessEdgeRouters(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["edgeRouters"] {
		create := FromMap(data, rest_model.EdgeRouterCreate{})

		// see if the router already exists
		existing := mgmt.EdgeRouterFromFilter(importer.Client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			log.WithFields(map[string]interface{}{
				"name":         *create.Name,
				"edgeRouterId": *existing.ID,
			}).
				Info("Found existing EdgeRouter, skipping create")
			_, _ = internal.FPrintfReusingLine(importer.Err, "Skipping EdgeRouter %s\r", *create.Name)
			continue
		}

		// do the actual create since it doesn't exist
		_, _ = internal.FPrintfReusingLine(importer.Err, "Creating EdgeRouterPolicy %s\r", *create.Name)
		log.WithField("name", *create.Name).Debug("Creating EdgeRouter")
		created, createErr := importer.Client.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{EdgeRouter: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
				}).Error("Unable to create EdgeRouter")
			} else {
				log.WithField("err", createErr).Error("Unable to create EdgeRouter")
				return nil, createErr
			}
		}
		log.WithFields(map[string]interface{}{
			"name":         *create.Name,
			"edgeRouterId": created.Payload.Data.ID,
		}).
			Info("Created EdgeRouter")

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}

func (importer *Importer) lookupEdgeRouters(roles []string) ([]string, error) {
	edgeRouterRoles := []string{}
	for _, role := range roles {
		if role[0:1] == "@" {
			value := role[1:]
			edgeRouter, _ := ascode.GetItemFromCache(importer.edgeRouterCache, value, func(name string) (interface{}, error) {
				return mgmt.EdgeRouterFromFilter(importer.Client, mgmt.NameFilter(name)), nil
			})
			if edgeRouter == nil {
				return nil, errors.New("error reading EdgeRouter: " + value)
			}
			edgeRouterId := edgeRouter.(*rest_model.EdgeRouterDetail).ID
			edgeRouterRoles = append(edgeRouterRoles, "@"+*edgeRouterId)
		} else {
			edgeRouterRoles = append(edgeRouterRoles, role)
		}
	}
	return edgeRouterRoles, nil
}
