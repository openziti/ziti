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
	"github.com/netfoundry/ziti-foundation/identity/certtools"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-edge/sdk/ziti/config"
	"github.com/netfoundry/ziti-edge/sdk/ziti/edge"
	nfpem "github.com/netfoundry/ziti-foundation/util/pem"
	"github.com/netfoundry/ziti-foundation/util/x509"
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/fullsailor/pkcs7"
	"github.com/michaelquigley/pfxlog"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type EnrollmentFlags struct {
	Token         *config.EnrollmentClaims
	JwtToken      *jwt.Token
	JwtString     string
	CertFile      string
	KeyFile       string
	IDName        string
	AdditionalCAs string
}

func ParseToken(tokenStr string) (*config.EnrollmentClaims, *jwt.Token, error) {
	parser := &jwt.Parser{
		SkipClaimsValidation: false,
	}
	enrollmentClaims := &config.EnrollmentClaims{}
	jwtToken, err := parser.ParseWithClaims(tokenStr, enrollmentClaims, ValidateToken)

	if err != nil {
		return nil, nil, err
	}

	return enrollmentClaims, jwtToken, nil
}

func ValidateToken(token *jwt.Token) (interface{}, error) {
	if token == nil {
		return nil, fmt.Errorf("could not validate token, token is nil")
	}

	claims, ok := token.Claims.(*config.EnrollmentClaims)

	if !ok {
		return nil, fmt.Errorf("could not validate token, token is not EnrollmentClaims")
	}

	if claims == nil {
		return nil, fmt.Errorf("could not validate token, EnrollmentClaims are nil")
	}

	if claims.Issuer == "" {
		return nil, fmt.Errorf("could not validate token, issues is empty")
	}

	_, err := url.Parse(claims.Issuer)

	if err != nil {
		return nil, fmt.Errorf("could not validate token, issuer [%s] is not a valid url ", claims.Issuer)
	}

	cert, err := FetchServerCert(claims.Issuer)

	claims.SignatureCert = cert

	if err != nil || cert == nil {
		return nil, fmt.Errorf("could not retrieve token URL certificate: %s", err)
	}

	return cert.PublicKey, nil
}

func Enroll(enFlags EnrollmentFlags) (*config.Config, error) {
	var key crypto.PrivateKey
	var err error
	cfg := &config.Config{
		ZtAPI: enFlags.Token.Issuer,
	}

	if strings.TrimSpace(enFlags.KeyFile) != "" {
		stat, err := os.Stat(enFlags.KeyFile)

		if !os.IsNotExist(err) {
			if stat.IsDir() {
				return nil, fmt.Errorf("specified key is a directory (%s)", enFlags.KeyFile)
			}
			if absPath, fileErr := filepath.Abs(enFlags.KeyFile); fileErr != nil {
				return nil, fileErr
			} else {
				cfg.ID.Key = "file://" + absPath
			}
		} else {
			cfg.ID.Key = enFlags.KeyFile
			fmt.Printf("using engine : %s\n", strings.Split(enFlags.KeyFile, ":")[0])
		}
	} else {
		key, err = generateKey()
		asnBytes, _ := x509.MarshalECPrivateKey(key.(*ecdsa.PrivateKey))
		keyPem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: asnBytes})
		cfg.ID.Key = "pem:" + string(keyPem)
		if err != nil {
			return nil, err
		}
	}

	caPool := x509.NewCertPool()

	allowedCerts := make([]*x509.Certificate, 0)

	if strings.TrimSpace(enFlags.AdditionalCAs) != "" {
		pfxlog.Logger().Debug("adding certificates from the provided ca override file")
		pem, _ := ioutil.ReadFile(enFlags.AdditionalCAs)
		for _, xcert := range nfpem.PemToX509(string(pem)) {
			allowedCerts = append(allowedCerts, xcert)
			caPool.AddCert(xcert)
		}
	}

	if enFlags.CertFile != "" {
		enFlags.CertFile, _ = filepath.Abs(enFlags.CertFile)
		cfg.ID.Cert = "file://" + enFlags.CertFile
	}

	enrollmentComplete := false
	shouldFetchCerts := true

	var enrollErr error

	//loop two times at most. if the correct certs are in the jwt or the overridden ca file then the enrollment will function properly
	//if not - fetch the certificates from the server - add them to the caPool and try again a second time
	for !enrollmentComplete {
		switch enFlags.Token.EnrollmentMethod {
		case "ott":
			enrollErr = enrollOTT(enFlags.Token, cfg, caPool)
		case "ottca":
			enrollErr = enrollCA(enFlags.Token, cfg, caPool)
		case "ca":
			enrollErr = enrollCAAuto(enFlags, cfg, caPool)
		default:
			enrollErr = fmt.Errorf("enrollment method '%s' is not supported", enFlags.Token.EnrollmentMethod)
			enrollmentComplete = true //no need to try again
		}

		if enrollErr == nil {
			enrollmentComplete = true //enrollment was successful
		} else {
			//determine if the failure is expected or due to tls. if tls related - retry. if not - just carry on without retrying
			urlErr, ok := enrollErr.(*url.Error)
			if ok {
				_, okx509 := urlErr.Err.(x509.UnknownAuthorityError)
				if okx509 && shouldFetchCerts {
					// don't try to fetch certs again
					shouldFetchCerts = false

					pfxlog.Logger().Debug("fetching certificates from server")
					rootCaPool := x509.NewCertPool()
					rootCaPool.AddCert(enFlags.Token.SignatureCert)

					for _, xcert := range FetchCertificates(cfg.ZtAPI, rootCaPool) {
						allowedCerts = append(allowedCerts, xcert)
						caPool.AddCert(xcert)
					}

					//certs fetched - try again
					continue
				}
			}

			// if any error other than a tls-related error occurs just return it - don't try again
			return cfg, enrollErr
		}
	}

	if len(allowedCerts) > 0 {
		var buf bytes.Buffer
		merr := nfx509.MarshalToPem(allowedCerts, &buf)
		if merr != nil {
			return nil, merr
		}
		cfg.ID.CA = "pem:" + buf.String()
	}

	return cfg, nil // success
}

func generateKey() (crypto.PrivateKey, error) {
	p384 := elliptic.P384()
	fmt.Printf("generating %s key\n", p384.Params().Name)
	return ecdsa.GenerateKey(p384, rand.Reader)
}

func useSystemCasIfEmpty(caPool *x509.CertPool) *x509.CertPool {
	if len(caPool.Subjects()) < 1 {
		pfxlog.Logger().Debugf("no cas provided in caPool. using system provided cas")
		//this means that there were no ca's in the jwt and none fetched and added... fallback to using
		//the system defined ca pool in this case
		return nil
	} else {
		return caPool
	}
}

func enrollOTT(token *config.EnrollmentClaims, cfg *config.Config, caPool *x509.CertPool) error {

	pk, err := identity.LoadKey(cfg.ID.Key)
	if err != nil {
		return fmt.Errorf("failed to load private key '%s': %s", cfg.ID.Key, err.Error())
	}

	hostname, err := os.Hostname()
	request, err := certtools.NewCertRequest(map[string]string{
		"C": "US", "O": "Netfoundry", "CN": hostname,
	}, nil)

	csr, err := x509.CreateCertificateRequest(rand.Reader, request, pk)

	if err != nil {
		return err
	}

	csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})

	caPool = useSystemCasIfEmpty(caPool)
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caPool,
			},
		},
	}
	resp, err := client.Post(token.EnrolmentUrl(), "text/plain", bytes.NewReader(csrPem))
	if err != nil {
		return err
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || err != nil {
		var edgeError edge.EdgeControllerApiError
		jsonerr := json.Unmarshal(respBody, &edgeError)

		if jsonerr != nil {
			return fmt.Errorf("enroll error: %s. raw response: %s. ", resp.Status, respBody)
		}

		//successfully parsed the json - make sure the code and cause message match and give a helpful message
		if edgeError.Error.Cause.Message == "identity not found" && edgeError.Error.Code == "INVALID_FIELD" {
			return fmt.Errorf("enroll error: %s. response: the provided token was invalid. either the token is not correct or it has already been used", resp.Status)
		} else {
			return fmt.Errorf("enroll error: %s. response: %s. ", resp.Status, edgeError.Error.Cause.Message)
		}
	}
	cfg.ID.Cert = "pem:" + string(respBody)
	return nil
}

func enrollCA(token *config.EnrollmentClaims, cfg *config.Config, caPool *x509.CertPool) error {

	if id, err := identity.LoadIdentity(cfg.ID); err != nil {
		return err
	} else {
		clientCert := id.Cert()

		caPool = useSystemCasIfEmpty(caPool)
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:      caPool,
					Certificates: []tls.Certificate{*clientCert},
				},
			},
		}
		resp, err := client.Post(token.EnrolmentUrl(), "text/plain", bytes.NewReader([]byte{}))
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusConflict {
				return fmt.Errorf("the provided identity has already been enrolled")
			} else {
				return fmt.Errorf("enroll error: %s", resp.Status)
			}
		}
		return nil
	}
}

type autoEnrollInput struct {
	Name string `json:"name"`
}

func enrollCAAuto(enFlags EnrollmentFlags, cfg *config.Config, caPool *x509.CertPool) error {
	if id, err := identity.LoadIdentity(cfg.ID); err != nil {
		return err
	} else {
		clientCert := id.Cert()

		caPool = useSystemCasIfEmpty(caPool)
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:      caPool,
					Certificates: []tls.Certificate{*clientCert},
				},
			},
		}

		var postBody []byte

		if strings.TrimSpace(enFlags.IDName) != "" {
			user := autoEnrollInput{
				Name: strings.TrimSpace(enFlags.IDName),
			}
			pb, merr := json.Marshal(user)
			if merr != nil {
				pfxlog.Logger().Warnf("problem converting name to json. Using the default name: %s", merr)
			}
			postBody = pb
		}

		resp, postErr := client.Post(enFlags.Token.EnrolmentUrl(), "text/plain", bytes.NewReader(postBody))
		if postErr != nil {
			return postErr
		}

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusConflict {
				return fmt.Errorf("the provided identity has already been enrolled")
			} else {
				return fmt.Errorf("enroll error: %s", resp.Status)
			}
		}
		return nil
	}
}

func FetchServerCert(urlRoot string) (*x509.Certificate, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Get(urlRoot)

	if err != nil {
		return nil, fmt.Errorf("could not contact remote server [%s]: %s", urlRoot, err)
	}

	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return nil, errors.New("peer certificate information is missing")
	}

	return resp.TLS.PeerCertificates[0], nil
}

// FetchCertificates will accecss the server insecurely to pull down the latest CA to be used to communicate with the
// server adding certificates to the provided pool
func FetchCertificates(urlRoot string, rootCaPool *x509.CertPool) []*x509.Certificate {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: rootCaPool},
	}
	client := &http.Client{Transport: tr}

	certStoreUrl, err := url.Parse(urlRoot)
	if err != nil {
		pfxlog.Logger().WithError(err).WithField("url", urlRoot).Panic("could not parse base url to retrieve CA store")
	}

	certStoreUrl.Path = path.Join(certStoreUrl.Path, ".well-known/est/cacerts") //specified by rfc7030

	resp, respErr := client.Get(certStoreUrl.String())

	if respErr != nil {
		//if an error occurs, log the issue and just return a nil slice of certs
		pfxlog.Logger().Errorf("unable to retrieve certificates from server at %s. %s", urlRoot, respErr)
		return nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			pfxlog.Logger().WithError(err).Error("could not close response body during certificate lookup")
		}
	}()

	pkcs7b64, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		pfxlog.Logger().Warnf("could not read response. no certificates added from %s", urlRoot)
		return nil
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {

		pkcs7Certs, _ := base64.StdEncoding.DecodeString(string(pkcs7b64))
		if pkcs7Certs != nil {
			certs, parseErr := pkcs7.Parse(pkcs7Certs)
			if parseErr != nil {
				pfxlog.Logger().Warnf("could not parse certificates. no certificates added from %s", urlRoot)
				return nil
			}
			return certs.Certificates
		}
	} else {
		pfxlog.Logger().Debugf("no certificates added from url. http response: %d, url: %s", resp.StatusCode, urlRoot)
	}
	return nil
}
