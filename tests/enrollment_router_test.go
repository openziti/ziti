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
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/google/uuid"
	"github.com/openziti/channel"

	identity2 "github.com/openziti/foundation/identity/identity"
	"github.com/pkg/errors"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"
)

func Test_RouterEnrollment(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("an edge router that is newly created", func(t *testing.T) {
		ctx.testContextChanged(t)
		edgeRouter := ctx.AdminManagementSession.requireNewEdgeRouter()

		t.Run("is unenrolled", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/edge-routers/" + edgeRouter.id)

			ctx.Req.NoError(err)

			standardJsonResponseTests(resp, http.StatusOK, t)

			edgeRouterDetails, err := gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)

			ctx.Req.True(edgeRouterDetails.ExistsP("data.enrollmentJwt"), "expected edge router to have an enrollmentJwt property")
		})

		t.Run("connecting to the control channel with unsigned client cert and not enrolled fails", func(t *testing.T) {
			ctx.testContextChanged(t)
			cert, pk, err := generateSelfSignedClientCert("oh hello there")

			ctx.Req.NoError(err)
			ctx.Req.NotNil(cert)
			ctx.Req.NotNil(pk)

			caPems := ctx.EdgeController.AppEnv.Config.CaPems()

			caCerts, err := parsePEMBundle(caPems)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(caCerts)

			id := identity2.NewClientTokenIdentity(cert, pk, caCerts)
			ctx.Req.NotNil(id)

			ch, err := channel.NewChannel("apitest", channel.NewClassicDialer(id, ctx.ControllerConfig.Ctrl.Listener, nil), nil, nil)
			ctx.Req.Nil(ch)
			ctx.Req.Error(err, "expected remote error bad TLS certificate")
		})

		t.Run("connecting to the control channel with ca signed client cert and not enrolled fails", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.testContextChanged(t)
			cert, pk, err := generateCaSignedClientCert(ctx.EdgeController.AppEnv.ApiClientCsrSigner.Cert(), ctx.EdgeController.AppEnv.ApiClientCsrSigner.Signer(), "oh hello there")

			ctx.Req.NoError(err)
			ctx.Req.NotNil(cert)
			ctx.Req.NotNil(pk)

			caPems := ctx.EdgeController.AppEnv.Config.CaPems()

			caCerts, err := parsePEMBundle(caPems)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(caCerts)

			id := identity2.NewClientTokenIdentity(cert, pk, caCerts)
			ctx.Req.NotNil(id)

			ch, err := channel.NewChannel("apitest", channel.NewClassicDialer(id, ctx.ControllerConfig.Ctrl.Listener, nil), nil, nil)
			ctx.Req.Nil(ch)
			ctx.Req.Error(err, "expected remote error bad TLS certificate")
		})

		t.Run("enrolling with an invalid token fails", func(t *testing.T) {
			ctx.testContextChanged(t)

			randomToken, err := uuid.NewRandom()

			ctx.Req.NoError(err)
			ctx.Req.NotNil(randomToken)

			resp, err := ctx.newAnonymousClientApiRequest().SetBody("{}").Post("/enroll?method=erott&token=" + randomToken.String())
			ctx.Req.NoError(err)

			standardErrorJsonResponseTests(resp, "INVALID_ENROLLMENT_TOKEN", http.StatusBadRequest, t)

		})

		t.Run("enrolling with a valid token", func(t *testing.T) {
			ctx.testContextChanged(t)

			//get token
			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/edge-routers/" + edgeRouter.id)

			ctx.Req.NoError(err)

			standardJsonResponseTests(resp, http.StatusOK, t)

			edgeRouterDetails, err := gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)

			ctx.Req.True(edgeRouterDetails.ExistsP("data.enrollmentToken"), "expected edge router to have an enrollmentToken property")

			enrollmentToken, ok := edgeRouterDetails.Path("data.enrollmentToken").Data().(string)
			ctx.Req.True(ok, "expected data.enrollmentToken to be a string")
			ctx.Req.NotEmpty(enrollmentToken)

			t.Run("and missing server and client CSR fields fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				bodyContainer := gabs.New()

				resp, err := ctx.newAnonymousClientApiRequest().SetBody(bodyContainer.String()).Post("/enroll?method=erott&token=" + enrollmentToken)
				ctx.Req.NoError(err)

				standardErrorJsonResponseTests(resp, "MISSING_OR_INVALID_CSR", http.StatusBadRequest, t)
			})

			t.Run("and with empty server and client CSR fields fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				bodyContainer := gabs.New()

				_, _ = bodyContainer.Set("", "serverCertCsr")
				_, _ = bodyContainer.Set("", "certCsr")

				resp, err := ctx.newAnonymousClientApiRequest().SetBody(bodyContainer.String()).Post("/enroll?method=erott&token=" + enrollmentToken)
				ctx.Req.NoError(err)

				standardErrorJsonResponseTests(resp, "COULD_NOT_PROCESS_CSR", http.StatusBadRequest, t)
			})

			t.Run("and with empty server and client CSR fields fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				bodyContainer := gabs.New()

				_, _ = bodyContainer.Set("", "serverCertCsr")
				_, _ = bodyContainer.Set("", "certCsr")

				resp, err := ctx.newAnonymousClientApiRequest().SetBody(bodyContainer.String()).Post("/enroll?method=erott&token=" + enrollmentToken)
				ctx.Req.NoError(err)

				standardErrorJsonResponseTests(resp, "COULD_NOT_PROCESS_CSR", http.StatusBadRequest, t)
			})

			t.Run("and with valid client and missing server CSR fields fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				privateKey := generateEcKey()

				clientCsrPem, err := createEnrollmentClientCsrPem(edgeRouter.id, privateKey)
				ctx.Req.NoError(err)

				bodyContainer := gabs.New()

				_, _ = bodyContainer.Set(clientCsrPem, "certCsr")

				resp, err := ctx.newAnonymousClientApiRequest().SetBody(bodyContainer.String()).Post("/enroll?method=erott&token=" + enrollmentToken)
				ctx.Req.NoError(err)

				standardErrorJsonResponseTests(resp, "MISSING_OR_INVALID_CSR", http.StatusBadRequest, t)
			})

			t.Run("and with valid client and empty server CSR fields fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				privateKey := generateEcKey()

				clientCsrPem, err := createEnrollmentClientCsrPem(edgeRouter.id, privateKey)
				ctx.Req.NoError(err)

				bodyContainer := gabs.New()

				_, _ = bodyContainer.Set("", "serverCertCsr")
				_, _ = bodyContainer.Set(clientCsrPem, "certCsr")

				resp, err := ctx.newAnonymousClientApiRequest().SetBody(bodyContainer.String()).Post("/enroll?method=erott&token=" + enrollmentToken)
				ctx.Req.NoError(err)

				standardErrorJsonResponseTests(resp, "COULD_NOT_PROCESS_CSR", http.StatusBadRequest, t)
			})

			t.Run("and with valid server and missing client CSR fields fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				privateKey := generateEcKey()

				serverCsrPem, err := createEnrollmentServerCsrPem(edgeRouter.id, privateKey)
				ctx.Req.NoError(err)

				bodyContainer := gabs.New()

				_, _ = bodyContainer.Set(serverCsrPem, "serverCertCsr")

				resp, err := ctx.newAnonymousClientApiRequest().SetBody(bodyContainer.String()).Post("/enroll?method=erott&token=" + enrollmentToken)
				ctx.Req.NoError(err)

				standardErrorJsonResponseTests(resp, "MISSING_OR_INVALID_CSR", http.StatusBadRequest, t)
			})

			t.Run("and with valid server and empty client CSR fields fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				privateKey := generateEcKey()

				serverCsrPem, err := createEnrollmentServerCsrPem(edgeRouter.id, privateKey)
				ctx.Req.NoError(err)

				bodyContainer := gabs.New()

				_, _ = bodyContainer.Set(serverCsrPem, "serverCertCsr")
				_, _ = bodyContainer.Set("", "certCsr")

				resp, err := ctx.newAnonymousClientApiRequest().SetBody(bodyContainer.String()).Post("/enroll?method=erott&token=" + enrollmentToken)
				ctx.Req.NoError(err)

				standardErrorJsonResponseTests(resp, "COULD_NOT_PROCESS_CSR", http.StatusBadRequest, t)
			})

			t.Run("and with valid client and server CSR fields succeeds", func(t *testing.T) {
				ctx.testContextChanged(t)

				privateKey := generateEcKey()

				clientCsrPem, err := createEnrollmentClientCsrPem(edgeRouter.id, privateKey)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(clientCsrPem)

				serverCsrPem, err := createEnrollmentServerCsrPem(edgeRouter.id, privateKey)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(serverCsrPem)

				bodyContainer := gabs.New()

				_, _ = bodyContainer.Set(serverCsrPem, "serverCertCsr")
				_, _ = bodyContainer.Set(clientCsrPem, "certCsr")

				resp, err := ctx.newAnonymousClientApiRequest().SetBody(bodyContainer.String()).Post("/enroll?method=erott&token=" + enrollmentToken)
				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				enrollmentContainer, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.Req.True(enrollmentContainer.ExistsP("data.cert"))
				certPem := enrollmentContainer.Path("data.cert").Data().(string)
				certs, err := parsePEMBundle([]byte(certPem))
				ctx.Req.NoError(err)
				ctx.Req.Len(certs, 1)
				cert := certs[0]

				ctx.Req.True(enrollmentContainer.ExistsP("data.serverCert"))
				serverCertPem := enrollmentContainer.Path("data.serverCert").Data().(string)
				serverCerts, err := parsePEMBundle([]byte(serverCertPem))
				ctx.Req.NoError(err)
				ctx.Req.Len(serverCerts, 1)

				ctx.Req.True(enrollmentContainer.ExistsP("data.ca"))
				caCertPem := enrollmentContainer.Path("data.ca").Data().(string)
				caCerts, err := parsePEMBundle([]byte(caCertPem))
				ctx.Req.NoError(err)

				t.Run("is reported as enrolled", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/edge-routers/" + edgeRouter.id)
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

					edgeRouterContainer, err := gabs.ParseJSON(resp.Body())
					ctx.Req.NoError(err)

					ctx.Req.True(edgeRouterContainer.ExistsP("data.isVerified"))
					ctx.Req.True(edgeRouterContainer.Path("data.isVerified").Data().(bool))
				})

				t.Run("connecting to the control channel with an alternate unsigned client cert while enrolled fails", func(t *testing.T) {
					ctx.testContextChanged(t)
					cert, pk, err := generateSelfSignedClientCert("oh hello there")

					ctx.Req.NoError(err)
					ctx.Req.NotNil(cert)
					ctx.Req.NotNil(pk)

					caPems := ctx.EdgeController.AppEnv.Config.CaPems()

					caCerts, err := parsePEMBundle(caPems)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(caCerts)

					id := identity2.NewClientTokenIdentity(cert, pk, caCerts)
					ctx.Req.NotNil(id)

					ch, err := channel.NewChannel("apitest", channel.NewClassicDialer(id, ctx.ControllerConfig.Ctrl.Listener, nil), nil, nil)
					ctx.Req.Nil(ch)
					ctx.Req.Error(err, "expected remote error bad TLS certificate")
				})

				t.Run("connecting to the control channel with an alternate CA signed client cert while enrolled fails", func(t *testing.T) {
					ctx.testContextChanged(t)

					ctx.testContextChanged(t)
					cert, pk, err := generateCaSignedClientCert(ctx.EdgeController.AppEnv.ApiClientCsrSigner.Cert(), ctx.EdgeController.AppEnv.ApiClientCsrSigner.Signer(), "oh hello there")

					ctx.Req.NoError(err)
					ctx.Req.NotNil(cert)
					ctx.Req.NotNil(pk)

					caPems := ctx.EdgeController.AppEnv.Config.CaPems()

					caCerts, err := parsePEMBundle(caPems)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(caCerts)

					id := identity2.NewClientTokenIdentity(cert, pk, caCerts)
					ctx.Req.NotNil(id)

					ch, err := channel.NewChannel("apitest", channel.NewClassicDialer(id, ctx.ControllerConfig.Ctrl.Listener, nil), nil, nil)
					ctx.Req.Nil(ch)
					ctx.Req.Error(err, "expected remote error bad TLS certificate")
				})

				t.Run("connecting to the control channel with the enrollment client cert and ca pool succeeds", func(t *testing.T) {
					ctx.testContextChanged(t)

					id := identity2.NewClientTokenIdentity(cert, privateKey, caCerts)
					ctx.Req.NotNil(id)

					ch, err := channel.NewChannel("apitest", channel.NewClassicDialer(id, ctx.ControllerConfig.Ctrl.Listener, nil), nil, nil)

					defer func() {
						if ch != nil {
							_ = ch.Close()
						}
					}()

					ctx.Req.NotNil(ch)
					ctx.Req.NoError(err)
				})

				t.Run("requesting enrollment extension with", func(t *testing.T) {
					ctx.testContextChanged(t)

					t.Run("no existing router client certificate fails", func(t *testing.T) {
						ctx.testContextChanged(t)
						body := gabs.New()

						_, _ = body.Set("", "serverCertCsr")
						_, _ = body.Set("", "certCsr")

						resp, err := ctx.newAnonymousClientApiRequest().SetBody(body.String()).Post("/enroll/extend/router")
						ctx.Req.NoError(err)

						standardErrorJsonResponseTests(resp, "UNAUTHORIZED", http.StatusUnauthorized, t)
					})

					t.Run("no client or server CSR fields fails", func(t *testing.T) {
						ctx.testContextChanged(t)

						body := gabs.New()

						resp, err := ctx.newRequestWithClientCert(cert, privateKey).SetBody(body.String()).Post("/enroll/extend/router")
						ctx.Req.NoError(err)

						standardErrorJsonResponseTests(resp, "COULD_NOT_VALIDATE", http.StatusBadRequest, t)
					})

					t.Run("a client CSR but missing server CSR field fails", func(t *testing.T) {
						ctx.testContextChanged(t)

						body := gabs.New()
						_, _ = body.Set("", "certCsr")

						resp, err := ctx.newRequestWithClientCert(cert, privateKey).SetBody(body.String()).Post("/enroll/extend/router")
						ctx.Req.NoError(err)

						standardErrorJsonResponseTests(resp, "COULD_NOT_VALIDATE", http.StatusBadRequest, t)
					})

					t.Run("a server CSR but missing client CSR field fails", func(t *testing.T) {
						ctx.testContextChanged(t)

						body := gabs.New()
						_, _ = body.Set("", "serverCertCsr")

						resp, err := ctx.newRequestWithClientCert(cert, privateKey).SetBody(body.String()).Post("/enroll/extend/router")
						ctx.Req.NoError(err)

						standardErrorJsonResponseTests(resp, "COULD_NOT_VALIDATE", http.StatusBadRequest, t)
					})

					t.Run("an empty client and server CSR fields fails", func(t *testing.T) {
						ctx.testContextChanged(t)

						body := gabs.New()
						_, _ = body.Set("", "serverCertCsr")
						_, _ = body.Set("", "certCsr")

						resp, err := ctx.newRequestWithClientCert(cert, privateKey).SetBody(body.String()).Post("/enroll/extend/router")
						ctx.Req.NoError(err)

						standardErrorJsonResponseTests(resp, "COULD_NOT_PROCESS_CSR", http.StatusBadRequest, t)
					})

					t.Run("a valid client CSR and empty server CSR fields fails", func(t *testing.T) {
						ctx.testContextChanged(t)

						extensionPrivateKey := generateEcKey()

						extensionClientCsrPem, err := createEnrollmentClientCsrPem(edgeRouter.id, extensionPrivateKey)
						ctx.Req.NoError(err)
						ctx.Req.NotNil(extensionClientCsrPem)

						extensionBodyContainer := gabs.New()

						_, _ = extensionBodyContainer.Set("", "serverCertCsr")
						_, _ = extensionBodyContainer.Set(extensionClientCsrPem, "certCsr")

						resp, err := ctx.newRequestWithClientCert(cert, privateKey).SetBody(extensionBodyContainer.String()).Post("/enroll/extend/router")
						ctx.Req.NoError(err)

						standardErrorJsonResponseTests(resp, "COULD_NOT_PROCESS_CSR", http.StatusBadRequest, t)
					})

					t.Run("a valid server CSR and empty client CSR fields fails", func(t *testing.T) {
						ctx.testContextChanged(t)

						extensionPrivateKey := generateEcKey()

						extensionServerCsrPem, err := createEnrollmentServerCsrPem(edgeRouter.id, extensionPrivateKey)
						ctx.Req.NoError(err)
						ctx.Req.NotNil(extensionServerCsrPem)

						extensionBodyContainer := gabs.New()

						_, _ = extensionBodyContainer.Set(extensionServerCsrPem, "serverCertCsr")
						_, _ = extensionBodyContainer.Set("", "certCsr")

						resp, err := ctx.newRequestWithClientCert(cert, privateKey).SetBody(extensionBodyContainer.String()).Post("/enroll/extend/router")
						ctx.Req.NoError(err)

						standardErrorJsonResponseTests(resp, "COULD_NOT_PROCESS_CSR", http.StatusBadRequest, t)
					})

					t.Run("a valid server and client CSR from a new private key succeeds", func(t *testing.T) {
						ctx.testContextChanged(t)

						extensionPrivateKey := generateEcKey()

						extensionClientCsrPem, err := createEnrollmentClientCsrPem(edgeRouter.id, extensionPrivateKey)
						ctx.Req.NoError(err)
						ctx.Req.NotNil(extensionClientCsrPem)

						extensionServerCsrPem, err := createEnrollmentServerCsrPem(edgeRouter.id, extensionPrivateKey)
						ctx.Req.NoError(err)
						ctx.Req.NotNil(extensionServerCsrPem)

						extensionBodyContainer := gabs.New()

						_, _ = extensionBodyContainer.Set(extensionServerCsrPem, "serverCertCsr")
						_, _ = extensionBodyContainer.Set(extensionClientCsrPem, "certCsr")

						resp, err := ctx.newRequestWithClientCert(cert, privateKey).SetBody(extensionBodyContainer.String()).Post("/enroll/extend/router")
						ctx.Req.NoError(err)

						standardJsonResponseTests(resp, http.StatusOK, t)

						extensionContainer, err := gabs.ParseJSON(resp.Body())
						ctx.Req.NoError(err)

						ctx.Req.True(extensionContainer.ExistsP("data.cert"))
						extensionClientCertPem := extensionContainer.Path("data.cert").Data().(string)
						extensionClientCerts, err := parsePEMBundle([]byte(extensionClientCertPem))
						ctx.Req.NoError(err)
						ctx.Req.Len(extensionClientCerts, 1)
						extensionCert := extensionClientCerts[0]

						ctx.Req.True(extensionContainer.ExistsP("data.serverCert"))
						extensionServerCertPem := extensionContainer.Path("data.serverCert").Data().(string)
						extensionServerCert, err := parsePEMBundle([]byte(extensionServerCertPem))
						ctx.Req.NoError(err)
						ctx.Req.Len(extensionServerCert, 1)

						t.Run("the new client cert has its NotAfter date increased", func(t *testing.T) {
							ctx.testContextChanged(t)
							extensionClientCerts[0].NotAfter.After(cert.NotAfter)
						})

						t.Run("the new client cert has its NotBefore date before now", func(t *testing.T) {
							ctx.testContextChanged(t)
							extensionClientCerts[0].NotBefore.Before(time.Now())
						})

						t.Run("the new server cert has its NotAfter date increased", func(t *testing.T) {
							ctx.testContextChanged(t)
							extensionServerCert[0].NotAfter.After(serverCerts[0].NotAfter)
						})

						t.Run("the new server cert has its NotBefore date before now", func(t *testing.T) {
							ctx.testContextChanged(t)
							extensionServerCert[0].NotBefore.Before(time.Now())
						})

						t.Run("connecting to the control channel with the old client cert fails", func(t *testing.T) {
							ctx.testContextChanged(t)

							id := identity2.NewClientTokenIdentity(cert, privateKey, caCerts)
							ctx.Req.NotNil(id)

							ch, err := channel.NewChannel("apitest", channel.NewClassicDialer(id, ctx.ControllerConfig.Ctrl.Listener, nil), nil, nil)

							defer func() {
								if ch != nil {
									_ = ch.Close()
								}
							}()
							ctx.Req.Error(err)
							ctx.Req.Nil(ch)
						})

						t.Run("connecting to the control channel with the new client cert succeeds", func(t *testing.T) {
							ctx.testContextChanged(t)

							id := identity2.NewClientTokenIdentity(extensionCert, extensionPrivateKey, caCerts)
							ctx.Req.NotNil(id)

							ch, err := channel.NewChannel("apitestextension", channel.NewClassicDialer(id, ctx.ControllerConfig.Ctrl.Listener, nil), nil, nil)

							defer func() {
								if ch != nil {
									_ = ch.Close()
								}
							}()

							ctx.Req.NotNil(ch)
							ctx.Req.NoError(err)
						})
					})
				})
			})
		})
	})
}

func generateSelfSignedClientCert(commonName string) (*x509.Certificate, crypto.Signer, error) {
	id, _ := rand.Int(rand.Reader, big.NewInt(100000000000000000))
	cert := &x509.Certificate{
		SerialNumber: id,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"Ziti CLI Generated Cert"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Minute * 10),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)

	if err != nil {
		return nil, nil, fmt.Errorf("could not generate private key for certificate (%v)", err)
	}

	signedCertBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, privateKey.Public(), privateKey)

	if err != nil {
		return nil, nil, fmt.Errorf("could not create cert (%v)", err)
	}

	cert, _ = x509.ParseCertificate(signedCertBytes)

	return cert, privateKey, nil
}

func parsePEMBundle(bundle []byte) ([]*x509.Certificate, error) {
	var certificates []*x509.Certificate
	var certDERBlock *pem.Block

	for {
		certDERBlock, bundle = pem.Decode(bundle)
		if certDERBlock == nil {
			break
		}

		if certDERBlock.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(certDERBlock.Bytes)
			if err != nil {
				return nil, err
			}
			certificates = append(certificates, cert)
		}
	}

	if len(certificates) == 0 {
		return nil, errors.New("no certificates were found while parsing the bundle")
	}

	return certificates, nil
}

func createEnrollmentClientCsrPem(commonName string, key crypto.PrivateKey) (string, error) {
	subject := pkix.Name{
		CommonName:         commonName,
		Country:            []string{"US"},
		Province:           []string{"NC"},
		Locality:           []string{},
		Organization:       []string{"OpenZiti"},
		OrganizationalUnit: []string{"CoolCrew"},
	}

	template := x509.CertificateRequest{
		Subject:            subject,
		SignatureAlgorithm: x509.UnknownSignatureAlgorithm,
	}
	csrBytes, csrErr := x509.CreateCertificateRequest(rand.Reader, &template, key)

	if csrErr != nil {
		return "", csrErr
	}

	outBuff := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	return string(outBuff), nil
}

func createEnrollmentServerCsrPem(commonName string, key crypto.PrivateKey) (string, error) {
	subject := pkix.Name{
		CommonName:         commonName,
		Country:            []string{"US"},
		Province:           []string{"NC"},
		Locality:           []string{},
		Organization:       []string{"OpenZiti"},
		OrganizationalUnit: []string{"CoolCrew"},
	}

	localhostIp := net.ParseIP("127.0.0.1")
	template := x509.CertificateRequest{
		Subject:            subject,
		SignatureAlgorithm: x509.UnknownSignatureAlgorithm,
		IPAddresses: []net.IP{
			localhostIp,
		},
	}
	csrBytes, csrErr := x509.CreateCertificateRequest(rand.Reader, &template, key)

	if csrErr != nil {
		return "", csrErr
	}

	outBuff := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	return string(outBuff), nil
}

func generateEcKey() crypto.PrivateKey {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	return key
}
