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

package rest_util

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"github.com/fullsailor/pkcs7"
	"io/ioutil"
)

// VerifyController will attempt to use the provided x509.CertPool to connect to the provided controller.
// If successful true an no error will be returned.
func VerifyController(controllerAddr string, caPool *x509.CertPool) (bool, error) {
	tlsConfig, err := NewTlsConfig()

	if err != nil {
		return false, err
	}

	tlsConfig.RootCAs = caPool

	httpClient, err := NewHttpClientWithTlsConfig(tlsConfig)

	if err != nil {
		return false, err
	}

	_, err = httpClient.Get(controllerAddr + "/edge/client/v1/versions")

	if err != nil {
		return false, err
	}

	return true, nil
}

// GetControllerWellKnownCas will attempt to connect to a controller and retrieve its PKCS11 well-known CA bundle.
func GetControllerWellKnownCas(controllerAddr string) ([]*x509.Certificate, error) {
	tlsConfig, err := NewTlsConfig()

	if err != nil {
		return nil, err
	}

	tlsConfig.InsecureSkipVerify = true

	httpClient, err := NewHttpClientWithTlsConfig(tlsConfig)

	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Get(fmt.Sprintf("%v/.well-known/est/cacerts", controllerAddr))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	encoded, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	certData, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, err
	}
	certs, err := pkcs7.Parse(certData)
	if err != nil {
		return nil, err
	}

	return certs.Certificates, nil
}
