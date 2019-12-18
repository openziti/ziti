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

package enroll

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/gateway/internal/gateway"
	"github.com/netfoundry/ziti-foundation/identity/certtools"
	"github.com/netfoundry/ziti-sdk-golang/ziti/enroll"
	"gopkg.in/resty.v1"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type apiPost struct {
	ServerCertCsr string `json:"serverCertCsr"`
	CertCsr       string `json:"certCsr"`
}

type apiResponse struct {
	ServerCert string `json:"serverCert"`
	CA         string `json:"ca"`
	Cert       string `json:"cert"`
}

type Enroller interface {
	Enroll(jwt []byte, silent bool, engine string) error
	LoadConfig(cfgmap map[interface{}]interface{}) error
}

type RestEnroller struct {
	config *gateway.Config
}

func NewRestEnroller() Enroller {
	return &RestEnroller{}
}

func (re *RestEnroller) parseCfgMap(cfgmap map[interface{}]interface{}) (*gateway.Config, error) {
	config := gateway.NewConfig()
	if err := config.LoadConfigFromMap(cfgmap); err != nil {
		return nil, err
	}

	return config, nil
}

func (re *RestEnroller) LoadConfig(cfgmap map[interface{}]interface{}) error {
	var err error
	re.config, err = re.parseCfgMap(cfgmap)

	if err != nil {
		return fmt.Errorf("error parsing configuraiton: %s", err)
	}

	return nil
}

func (re *RestEnroller) Enroll(jwtBuf []byte, silent bool, engine string) error {
	log := pfxlog.Logger()

	if re.config == nil {
		return errors.New("no configuration provided")
	}

	_, err := re.config.LoadIdentity()

	if err == nil {
		log.Warnf("identity detected, note that any identity information will be overwritten when enrolling [%s]", re.config.IdentityConfig)
	}

	ec, _, err := enroll.ParseToken(strings.TrimSpace(string(jwtBuf)))
	if err != nil {
		log.WithField("cause", err).Fatal("failed to parse JWT")
	}

	log.Debug("JWT parsed")

	rootCaPool := x509.NewCertPool()
	rootCaPool.AddCert(ec.SignatureCert)

	rootCas := enroll.FetchCertificates(ec.Issuer, rootCaPool)

	if len(rootCas) == 0 {
		log.Fatal("no valid root CAs were found")
	}

	var engUrl *url.URL

	if engine != "" {
		if engUrl, err = url.Parse(engine); err != nil {
			return fmt.Errorf("could not parse engine string: %s", err)
		}
	}

	//writes key if it is file based
	key, err := certtools.GetKey(engUrl, re.config.IdentityConfig.Key, "ec:P-256")

	if err != nil {
		return fmt.Errorf("could not obtain private key: %s", err)
	}

	subject := &pkix.Name{
		CommonName:         ec.Subject,
		Country:            []string{re.config.Csr.Country},
		Province:           []string{re.config.Csr.Province},
		Locality:           []string{re.config.Csr.Locality},
		Organization:       []string{re.config.Csr.Organization},
		OrganizationalUnit: []string{re.config.Csr.OrganizationalUnit},
	}

	serverCsr, err := CreateCsr(key, x509.UnknownSignatureAlgorithm, subject, re.config.Csr.Sans)

	if err != nil {
		return fmt.Errorf("failed to generate server CSR: %s", err)
	}

	clientCsr, err := CreateCsr(key, x509.UnknownSignatureAlgorithm, subject, re.config.Csr.Sans)

	if err != nil {
		return fmt.Errorf("failed to generate client CSR: %s", err)
	}

	er := &apiPost{
		CertCsr:       clientCsr,
		ServerCertCsr: serverCsr,
	}

	client := resty.New()

	caCertPool := x509.NewCertPool()
	for _, cert := range rootCas {
		caCertPool.AddCert(cert)
	}

	tc := &tls.Config{
		RootCAs: caCertPool,
	}

	client.SetTLSClientConfig(tc)

	resp, err := re.Send(client, ec.EnrolmentUrl(), er)

	if err != nil {
		return err
	}

	if resp.Cert == "" {
		return fmt.Errorf("enrollment response did not contain a cert")
	}

	if resp.ServerCert == "" {
		return fmt.Errorf("enrollment response did not contain a server cert")
	}

	if resp.CA == "" {
		return fmt.Errorf("enrollment response did not contain a CA chain")
	}

	if err = ioutil.WriteFile(re.config.IdentityConfig.Cert, []byte(resp.Cert), 0600); err != nil {
		return fmt.Errorf("unable to write client cert to [%s]: %s", re.config.IdentityConfig.Cert, err)
	}

	if err = ioutil.WriteFile(re.config.IdentityConfig.ServerCert, []byte(resp.ServerCert), 0600); err != nil {
		return fmt.Errorf("unable to write server cert to [%s]: %s", re.config.IdentityConfig.ServerCert, err)
	}
	if err = ioutil.WriteFile(re.config.IdentityConfig.CA, []byte(resp.CA), 0600); err != nil {
		return fmt.Errorf("unable to write CA certs to [%s]: %s", re.config.IdentityConfig.CA, err)
	}

	log.Info("registration complete")
	return nil
}

func (re *RestEnroller) Send(client *resty.Client, enrollUrl string, e *apiPost) (*apiResponse, error) {
	data := &apiResponse{}
	apiResponse := response.NewApiResponseBody(data, nil)

	resp, err := client.R().
		SetBody(e).
		SetResult(apiResponse).
		Post(enrollUrl)

	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("enrollment failed recieved HTTP status [%s]: %s", resp.Status(), resp.Body())
	}

	return data, nil
}
