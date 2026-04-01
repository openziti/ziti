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

package federation

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/identity/certtools"
	"github.com/openziti/sdk-golang/ziti/enroll"
	routerEnroll "github.com/openziti/ziti/v2/router/enroll"
	"github.com/openziti/ziti/v2/router/env"
)

type enrollPost struct {
	ServerCertCsr string `json:"serverCertCsr"`
	CertCsr       string `json:"certCsr"`
}

// EnrollWithNetwork performs CSR-based enrollment with a client network's controller using
// an enrollment JWT. It writes the resulting identity files to outputDir and returns a
// NetworkIdentity that can be used to connect to the client network.
func EnrollWithNetwork(jwtData []byte, networkId uint16, outputDir string, sans *env.Sans) (*NetworkIdentity, error) {
	log := pfxlog.Logger().WithField("networkId", networkId)

	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create network directory %s: %w", outputDir, err)
	}

	ec, _, err := enroll.ParseToken(strings.TrimSpace(string(jwtData)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse enrollment JWT: %w", err)
	}
	log.Debug("enrollment JWT parsed")

	rootCaPool := x509.NewCertPool()
	rootCaPool.AddCert(ec.SignatureCert)
	rootCas := enroll.FetchCertificates(ec.Issuer, rootCaPool)
	if len(rootCas) == 0 {
		return nil, fmt.Errorf("no valid root CAs found from issuer %s", ec.Issuer)
	}

	keyPath := filepath.Join(outputDir, "key.pem")
	key, err := certtools.GetKey(nil, keyPath, "ec:P-256")
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	subject := &pkix.Name{
		CommonName: ec.Subject,
	}

	clientCsr, err := routerEnroll.CreateCsr(key, x509.UnknownSignatureAlgorithm, subject, sans)
	if err != nil {
		return nil, fmt.Errorf("failed to generate client CSR: %w", err)
	}

	// For the existing enrollment endpoint, we need both server and client CSRs
	serverCsr, err := routerEnroll.CreateCsr(key, x509.UnknownSignatureAlgorithm, subject, sans)
	if err != nil {
		return nil, fmt.Errorf("failed to generate server CSR: %w", err)
	}

	body := &enrollPost{
		CertCsr:       clientCsr,
		ServerCertCsr: serverCsr,
	}

	client := resty.New()
	caCertPool := x509.NewCertPool()
	for _, cert := range rootCas {
		caCertPool.AddCert(cert)
	}
	client.SetTLSClientConfig(&tls.Config{
		RootCAs:    caCertPool,
		MinVersion: tls.VersionTLS12,
	})

	resp, err := client.R().SetBody(body).Post(ec.EnrolmentUrl())
	if err != nil {
		return nil, fmt.Errorf("enrollment request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("enrollment failed with HTTP %s: %s", resp.Status(), resp.Body())
	}

	envelope := rest_model.EnrollmentCertsEnvelope{}
	if err = json.Unmarshal(resp.Body(), &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse enrollment response: %w", err)
	}

	certData := envelope.Data
	if certData.Cert == "" {
		return nil, fmt.Errorf("enrollment response did not contain a cert")
	}
	if certData.Ca == "" {
		return nil, fmt.Errorf("enrollment response did not contain a CA chain")
	}

	certPath := filepath.Join(outputDir, "cert.pem")
	if err = os.WriteFile(certPath, []byte(certData.Cert), 0600); err != nil {
		return nil, fmt.Errorf("failed to write cert to %s: %w", certPath, err)
	}

	caPath := filepath.Join(outputDir, "ca.pem")
	if err = os.WriteFile(caPath, []byte(certData.Ca), 0600); err != nil {
		return nil, fmt.Errorf("failed to write CA to %s: %w", caPath, err)
	}

	// If server cert is present, write it too (the existing endpoint returns it)
	if certData.ServerCert != "" {
		serverCertPath := filepath.Join(outputDir, "server_cert.pem")
		if err = os.WriteFile(serverCertPath, []byte(certData.ServerCert), 0600); err != nil {
			return nil, fmt.Errorf("failed to write server cert to %s: %w", serverCertPath, err)
		}
	}

	// Extract controller endpoints from JWT claims
	var endpoints []string
	claims := jwt.MapClaims{}
	parser := jwt.NewParser()
	_, _, err = parser.ParseUnverified(strings.TrimSpace(string(jwtData)), claims)
	if err == nil {
		if ctrl, ok := claims["ctrl"]; ok {
			endpoints = append(endpoints, ctrl.(string))
		}
		if ctrls, ok := claims["ctrls"]; ok {
			if ctrlsSlice, ok := ctrls.([]interface{}); ok {
				for _, ctrl := range ctrlsSlice {
					endpoints = append(endpoints, ctrl.(string))
				}
			}
		}
	}
	if len(endpoints) == 0 {
		endpoints = ec.Controllers
	}

	ni := &NetworkIdentity{
		NetworkId: networkId,
		Endpoints: endpoints,
		Dir:       outputDir,
	}

	if err = SaveNetworkIdentity(ni); err != nil {
		return nil, fmt.Errorf("failed to save network identity: %w", err)
	}

	// Reload the full identity from the files we just wrote
	loaded, err := LoadNetworkIdentity(outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load enrolled identity: %w", err)
	}

	log.Info("federation enrollment complete")
	return loaded, nil
}
