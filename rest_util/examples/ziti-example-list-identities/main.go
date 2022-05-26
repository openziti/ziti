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

package main

import (
	"context"
	"crypto/x509"
	"github.com/openziti/edge/rest_management_api_client/identity"
	"github.com/openziti/edge/rest_util"
	log "github.com/sirupsen/logrus"
)

// main is an example usage of the rest_util package. This example is missing the concepts of enrollment.
// Part of enrollment is being delivered a signed JWT that can be used to verify the controller server certificate
// That step is missing from this example. The CA bundle from the well-known endpoint is verified as a sanity
// check against the controller. However, this does not add any extra security, just sanity.
func main() {
	ctrlAddress := "https://localhost:1280"
	caCerts, err := rest_util.GetControllerWellKnownCas(ctrlAddress)

	if err != nil {
		log.Fatal(err)
	}

	caPool := x509.NewCertPool()

	for _, ca := range caCerts {
		caPool.AddCert(ca)
	}

	ok, err := rest_util.VerifyController(ctrlAddress, caPool)

	if err != nil {
		log.Fatal(err)
	}

	if !ok {
		log.Fatal("controller failed CA validation")
	}

	client, err := rest_util.NewEdgeManagementClientWithUpdb("admin", "admin", ctrlAddress, caPool)

	if err != nil {
		log.Fatal(err)
	}

	params := &identity.ListIdentitiesParams{
		Context: context.Background(),
	}

	resp, err := client.Identity.ListIdentities(params, nil)

	if err != nil {
		log.Fatal(err)
	}

	println("\n=== Identity List ===")
	for _, identityItem := range resp.GetPayload().Data {
		println(*identityItem.Name)
	}
}
