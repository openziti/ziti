//go:build apitests
// +build apitests

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

package tests

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"github.com/Jeffail/gabs"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/identity/certtools"
	"net/http"
	"os"
	"testing"
)

func Test_Api_Session_Certs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	t.Run("as the default admin, session certs", func(t *testing.T) {
		var createdResponseBody *gabs.Container //used across multiple subtests, set in first
		var createdId string                    //used across multiple subtests, set in first

		t.Run("can be created", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.RequireAdminManagementApiLogin()
			ctx.RequireAdminClientApiLogin()
			csr, _, err := generateCsr()
			ctx.Req.NoError(err)

			csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})
			ctx.Req.NotEmpty(csrPem)

			request := ctx.AdminClientSession.newAuthenticatedRequest()

			body := gabs.New()
			body.Set(string(csrPem), "csr")
			bodyStr := body.String()
			request.SetBody(bodyStr)

			resp, err := request.Post("current-api-session/certificates")

			ctx.Req.NoError(err)
			standardJsonResponseTests(resp, http.StatusCreated, t)

			createdResponseBody, err = gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)

			var ok bool
			createdId, ok = createdResponseBody.Path("data.id").Data().(string)
			ctx.Req.True(ok)
			ctx.Req.NotEmpty(createdId)

			certificate := createdResponseBody.Path("data.certificate").Data().(string)
			ctx.Req.NotEmpty(certificate)

			x509Cert, err := certtools.LoadCert([]byte(certificate))
			ctx.Req.NotEmpty(x509Cert)
			ctx.Req.NoError(err)

		})

		t.Run("can be listed", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminClientSession.newAuthenticatedRequest().Get("current-api-session/certificates")
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			t.Run("list contains the API session cert", func(t *testing.T) {
				ctx.testContextChanged(t)

				bodyStr := resp.Body()
				body, err := gabs.ParseJSON(bodyStr)
				ctx.Req.NoError(err)

				data := body.Path("data")
				ctx.Req.NotEmpty(data)

				children, err := data.Children()
				ctx.Req.NoError(err)

				ctx.Req.Len(children, 1)

				subject := children[0].Path("subject").Data().(string)
				ctx.Req.NotEmpty(subject)

				fingerprint := children[0].Path("fingerprint").Data().(string)
				ctx.Req.NotEmpty(fingerprint)

				validFrom := children[0].Path("validFrom").Data().(string)
				ctx.Req.NotEmpty(validFrom)

				validTo := children[0].Path("validTo").Data().(string)
				ctx.Req.NotEmpty(validTo)

				certificate := children[0].Path("certificate").Data().(string)
				ctx.Req.NotEmpty(certificate)

				x509Cert, err := certtools.LoadCert([]byte(certificate))
				ctx.Req.NotEmpty(x509Cert)
				ctx.Req.NoError(err)
			})
		})

		t.Run("can be detailed", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminClientSession.newAuthenticatedRequest().Get("current-api-session/certificates/" + createdId)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			t.Run("contains proper values", func(t *testing.T) {
				ctx.testContextChanged(t)

				bodyStr := resp.Body()
				body, err := gabs.ParseJSON(bodyStr)
				ctx.Req.NoError(err)

				data := body.Path("data")
				ctx.Req.NotEmpty(data)

				subject := data.Path("subject").Data().(string)
				ctx.Req.NotEmpty(subject)

				fingerprint := data.Path("fingerprint").Data().(string)
				ctx.Req.NotEmpty(fingerprint)

				validFrom := data.Path("validFrom").Data().(string)
				ctx.Req.NotEmpty(validFrom)

				validTo := data.Path("validTo").Data().(string)
				ctx.Req.NotEmpty(validTo)

				certificate := data.Path("certificate").Data().(string)
				ctx.Req.NotEmpty(certificate)

				x509Cert, err := certtools.LoadCert([]byte(certificate))
				ctx.Req.NotEmpty(x509Cert)
				ctx.Req.NoError(err)
			})
		})

		t.Run("can be deleted", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminClientSession.newAuthenticatedRequest().Delete("current-api-session/certificates/" + createdId)
			ctx.Req.NoError(err)

			standardJsonResponseTests(resp, http.StatusOK, t)
		})
	})
}

func generateCsr() ([]byte, crypto.PrivateKey, error) {
	p384 := elliptic.P384()
	pfxlog.Logger().Infof("generating %s EC key", p384.Params().Name)
	privateKey, err := ecdsa.GenerateKey(p384, rand.Reader)

	if err != nil {
		return nil, nil, err
	}

	hostname, err := os.Hostname()

	if err != nil {
		return nil, nil, err
	}

	request, err := certtools.NewCertRequest(map[string]string{
		"C": "US", "O": "TEST", "CN": hostname,
	}, nil)

	if err != nil {
		return nil, nil, err
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, request, privateKey)

	if err != nil {
		return nil, nil, err
	}

	return csr, privateKey, nil
}
