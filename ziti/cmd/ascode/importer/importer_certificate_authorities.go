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
	"github.com/openziti/edge-api/rest_management_api_client/certificate_authority"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"slices"
)

func (importer *Importer) IsCertificateAuthorityImportRequired(args []string) bool {
	return slices.Contains(args, "all") || len(args) == 0 || // explicit all or nothing specified
		slices.Contains(args, "ca") ||
		slices.Contains(args, "certificate-authority")
}

func (importer *Importer) ProcessCertificateAuthorities(input map[string][]interface{}) (map[string]string, error) {

	var result = map[string]string{}
	for _, data := range input["certificateAuthorities"] {
		create := FromMap(data, rest_model.CaCreate{})

		// see if the CA already exists
		existing := mgmt.CertificateAuthorityFromFilter(importer.client, mgmt.NameFilter(*create.Name))
		if existing != nil {
			if importer.loginOpts.Verbose {
				log.WithFields(map[string]interface{}{
					"name":                   *create.Name,
					"certificateAuthorityId": *existing.ID,
				}).
					Info("Found existing CertificateAuthority, skipping create")
			}
			_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Skipping CertificateAuthority %s\r", *create.Name)
			continue
		}

		// do the actual create since it doesn't exist
		_, _ = internal.FPrintfReusingLine(importer.loginOpts.Err, "Creating CertificateAuthority %s\r", *create.Name)
		created, createErr := importer.client.CertificateAuthority.CreateCa(&certificate_authority.CreateCaParams{Ca: create}, nil)
		if createErr != nil {
			if payloadErr, ok := createErr.(rest_util.ApiErrorPayload); ok {
				log.
					WithError(createErr).
					WithFields(map[string]interface{}{
						"field":  payloadErr.GetPayload().Error.Cause.APIFieldError.Field,
						"reason": payloadErr.GetPayload().Error.Cause.APIFieldError.Reason,
					}).
					Error("Unable to create CertificateAuthority")
				return nil, createErr
			} else {
				log.WithError(createErr).Error("Unable to create CertificateAuthority")
				return nil, createErr
			}
		}
		if importer.loginOpts.Verbose {
			log.WithFields(map[string]interface{}{
				"name":                   *create.Name,
				"certificateAuthorityId": created.Payload.Data.ID,
			}).
				Info("Created CertificateAuthority")
		}

		result[*create.Name] = created.Payload.Data.ID
	}

	return result, nil
}
