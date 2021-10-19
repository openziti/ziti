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

package enroll

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/router/internal/edgerouter"
	"github.com/openziti/fabric/router"
	"github.com/openziti/foundation/identity/certtools"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/sdk-golang/ziti/enroll"
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

type Enroller interface {
	Enroll(jwt []byte, silent bool, engine string, keyAlg config.KeyAlgVar) error
	LoadConfig(cfgmap map[interface{}]interface{}) error
}

type RestEnroller struct {
	config *edgerouter.Config
}

func NewRestEnroller() Enroller {
	return &RestEnroller{}
}

func (re *RestEnroller) parseCfgMap(cfgmap map[interface{}]interface{}) (*edgerouter.Config, error) {
	routerConfig := &router.Config{}

	edgeConfig := edgerouter.NewConfig(routerConfig)
	if err := edgeConfig.LoadConfigFromMapForEnrollment(cfgmap); err != nil {
		return nil, fmt.Errorf("could not load edge router config: %v", err)
	}

	return edgeConfig, nil
}

func (re *RestEnroller) LoadConfig(cfgmap map[interface{}]interface{}) error {
	var err error
	re.config, err = re.parseCfgMap(cfgmap)

	if err != nil {
		return fmt.Errorf("error parsing configuration: %s", err)
	}

	return nil
}

func (re *RestEnroller) Enroll(jwtBuf []byte, silent bool, engine string, keyAlg config.KeyAlgVar) error {
	log := pfxlog.Logger()

	if re.config == nil {
		return errors.New("no configuration provided")
	}

	identityConfig := re.config.EnrollmentIdentityConfig

	if re.config.RouterConfig.Id != nil {
		identityConfig = re.config.RouterConfig.Id.GetConfig()
		log.Warnf("identity detected, note that any identity information will be overwritten when enrolling")
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
	var key crypto.PrivateKey
	if keyAlg.EC() {
		key, err = certtools.GetKey(engUrl, identityConfig.Key, "ec:P-256")
	} else if keyAlg.RSA() {
		key, err = certtools.GetKey(engUrl, identityConfig.Key, "rsa:4096")
	} else {
		panic(fmt.Sprintf("invalid KeyAlg specified: %s", keyAlg.Get()))
	}

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
		RootCAs:    caCertPool,
		MinVersion: tls.VersionTLS12,
	}

	client.SetTLSClientConfig(tc)

	envelope, err := re.Send(client, ec.EnrolmentUrl(), er)

	if err != nil {
		return err
	}

	resp := envelope.Data

	if resp.Cert == "" {
		return fmt.Errorf("enrollment response did not contain a cert")
	}

	if resp.ServerCert == "" {
		return fmt.Errorf("enrollment response did not contain a server cert")
	}

	if resp.Ca == "" {
		return fmt.Errorf("enrollment response did not contain a CA chain")
	}

	if err = ioutil.WriteFile(identityConfig.Cert, []byte(resp.Cert), 0600); err != nil {
		return fmt.Errorf("unable to write client cert to [%s]: %s", identityConfig.Cert, err)
	}

	if err = ioutil.WriteFile(identityConfig.ServerCert, []byte(resp.ServerCert), 0600); err != nil {
		return fmt.Errorf("unable to write server cert to [%s]: %s", identityConfig.ServerCert, err)
	}

	if err = ioutil.WriteFile(identityConfig.CA, []byte(resp.Ca), 0600); err != nil {
		return fmt.Errorf("unable to write CA certs to [%s]: %s", identityConfig.CA, err)
	}

	log.Info("registration complete")
	return nil
}

func (re *RestEnroller) Send(client *resty.Client, enrollUrl string, e *apiPost) (*rest_model.EnrollmentCertsEnvelope, error) {
	envelope := rest_model.EnrollmentCertsEnvelope{}

	resp, err := client.R().
		SetBody(e).
		Post(enrollUrl)

	if err != nil {
		return nil, err
	}

	body := resp.Body()

	err = json.Unmarshal(body, &envelope)

	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("enrollment failed recieved HTTP status [%s]: %s", resp.Status(), resp.Body())
	}

	return &envelope, nil
}
