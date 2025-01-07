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

package upload

import (
	"encoding/json"
	"github.com/antchfx/jsonquery"
	"github.com/openziti/edge-api/rest_management_api_client/posture_checks"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"strings"
)

func (u *Upload) ProcessPostureChecks(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["postureChecks"] {

		// convert to a jsonquery doc so we can query inside the json
		jsonData, _ := json.Marshal(data)
		doc, jsonQueryErr := jsonquery.Parse(strings.NewReader(string(jsonData)))
		if jsonQueryErr != nil {
			log.WithError(jsonQueryErr).Error("Unable to list ")
			return nil, jsonQueryErr
		}
		typeNode := jsonquery.FindOne(doc, "/typeId")

		var create rest_model.PostureCheckCreate
		switch strings.ToUpper(typeNode.Value().(string)) {
		case "DOMAIN":
			create = FromMap(data, rest_model.PostureCheckDomainCreate{})
		case "MAC":
			create = FromMap(data, rest_model.PostureCheckMacAddressCreate{})
		case "MFA":
			create = FromMap(data, rest_model.PostureCheckMfaCreate{})
		case "OS":
			create = FromMap(data, rest_model.PostureCheckOperatingSystemCreate{})
		case "PROCESS":
			create = FromMap(data, rest_model.PostureCheckProcessCreate{})
		case "PROCESS-MULTI":
			create = FromMap(data, rest_model.PostureCheckProcessMultiCreate{})
		default:
			log.WithFields(map[string]interface{}{
				"name":   *create.Name(),
				"typeId": create.TypeID,
			}).
				Error("Unknown PostureCheck type")
		}

		// see if the posture check already exists
		existing := mgmt.PostureCheckFromFilter(u.client, mgmt.NameFilter(*create.Name()))
		if existing != nil {
			if u.loginOpts.Verbose {
				log.WithFields(map[string]interface{}{
					"name":           *create.Name(),
					"postureCheckId": (*existing).ID(),
					"typeId":         create.TypeID(),
				}).
					Info("Found existing PostureCheck, skipping create")
			}
			_, _ = internal.FPrintFReusingLine(u.loginOpts.Err, "Skipping PostureCheck %s\r", *create.Name())
			continue
		}

		// do the actual create since it doesn't exist
		_, _ = internal.FPrintFReusingLine(u.loginOpts.Err, "Creating PostureCheck %s\r", *create.Name())
		if u.loginOpts.Verbose {
			log.WithFields(map[string]interface{}{
				"name":   *create.Name(),
				"typeId": create.TypeID(),
			}).
				Debug("Creating PostureCheck")
		}
		created, createErr := u.client.PostureChecks.CreatePostureCheck(&posture_checks.CreatePostureCheckParams{PostureCheck: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.WithFields(map[string]interface{}{
					"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
					"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
				}).
					Error("Unable to create PostureCheck")
			} else {
				log.WithError(createErr).Error("Unable to ")
				return nil, createErr
			}
		}
		if u.loginOpts.Verbose {
			log.WithFields(map[string]interface{}{
				"name":           *create.Name(),
				"postureCheckId": created.Payload.Data.ID,
				"typeId":         create.TypeID(),
			}).
				Info("Created PostureCheck")
		}

		result[*create.Name()] = created.Payload.Data.ID
	}

	return result, nil
}
