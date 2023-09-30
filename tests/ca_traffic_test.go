//go:build apitests

package tests

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	id "github.com/openziti/identity"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/eid"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"
)

func Test_CA_Auth_Two_Identities_Diff_Certs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	time.Sleep(2 * time.Second)

	t.Run("setup", func(t *testing.T) {
		ctx.testContextChanged(t)

		// create ca
		caCert, caPrivate, caPEM := newTestCaCert()

		caCreate := &rest_model.CaCreate{
			CertPem: S(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           I(0),
				Location:        S(rest_model.ExternalIDClaimLocationCOMMONNAME),
				Matcher:         S(rest_model.ExternalIDClaimMatcherALL),
				MatcherCriteria: S(""),
				Parser:          S(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  S(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             B(true),
			IsAutoCaEnrollmentEnabled: B(true),
			IsOttCaEnrollmentEnabled:  B(true),
			Name:                      S(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caCreateResult)
		ctx.NotNil(caCreateResult.Data)
		ctx.NotEmpty(caCreateResult.Data.ID)

		//verify ca

		caValues := ctx.AdminManagementSession.requireQuery("cas/" + caCreateResult.Data.ID)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.Req.NotEmpty(verificationToken)

		verificationKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		var verificationTemplate = &x509.Certificate{
			NotBefore:    time.Now(),
			NotAfter:     time.Now().AddDate(1, 0, 0),
			SerialNumber: big.NewInt(123456789),
			Subject: pkix.Name{
				Country:      []string{"US"},
				SerialNumber: "123456789",
				CommonName:   verificationToken,
			},
			KeyUsage:              x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			BasicConstraintsValid: true,
		}

		verificationRawCert, err := x509.CreateCertificate(rand.Reader, verificationTemplate, caCert, &verificationKey.PublicKey, caPrivate)
		ctx.Req.NoError(err)

		verificationCert, err := x509.ParseCertificate(verificationRawCert)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(verificationCert)

		verificationPem := nfpem.EncodeToBytes(verificationCert)

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().
			SetHeader("content-type", "text/plain").
			SetBody(verificationPem).
			Post("cas/" + caCreateResult.Data.ID + "/verify")

		ctx.Req.NoError(err)
		ctx.logJson(resp.Body())
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		//create child cert 1

		const commonName = "TEST_COMMON_NAME"

		clientKey1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		clientPrivateDer1, _ := x509.MarshalECPrivateKey(clientKey1)
		clientKeyPem1 := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientPrivateDer1})

		var clientTemplate1 = &x509.Certificate{
			NotBefore:    time.Now(),
			NotAfter:     time.Now().AddDate(1, 0, 0),
			SerialNumber: big.NewInt(123456789),
			Subject: pkix.Name{
				Country:      []string{"US"},
				SerialNumber: "123456789",
				CommonName:   commonName,
			},
			KeyUsage:              x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			BasicConstraintsValid: true,
		}

		clientRawCert1, err := x509.CreateCertificate(rand.Reader, clientTemplate1, caCert, &clientKey1.PublicKey, caPrivate)
		ctx.Req.NoError(err)

		clientCert1, err := x509.ParseCertificate(clientRawCert1)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(clientCert1)

		clientCertPem1 := nfpem.EncodeToString(clientCert1)

		//create child cert 2
		clientKey2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		clientPrivateDer2, _ := x509.MarshalECPrivateKey(clientKey2)
		clientKeyPem2 := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientPrivateDer2})

		var clientTemplate2 = &x509.Certificate{
			NotBefore:    time.Now(),
			NotAfter:     time.Now().AddDate(1, 0, 0),
			SerialNumber: big.NewInt(4567894596),
			Subject: pkix.Name{
				Country:      []string{"US"},
				SerialNumber: "4567894596",
				CommonName:   commonName,
			},
			KeyUsage:              x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			BasicConstraintsValid: true,
		}

		clientRawCert2, err := x509.CreateCertificate(rand.Reader, clientTemplate2, caCert, &clientKey2.PublicKey, caPrivate)
		ctx.Req.NoError(err)

		clientCert2, err := x509.ParseCertificate(clientRawCert2)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(clientCert2)

		clientCertPem2 := nfpem.EncodeToString(clientCert2)

		// create test identity w/ externalId set to the common name
		idType := rest_model.IdentityTypeUser
		identityPost := &rest_model.IdentityCreate{
			Enrollment: &rest_model.IdentityCreateEnrollment{
				Ott: true,
			},
			Name:       S(uuid.NewString()),
			IsAdmin:    B(false),
			Type:       &idType,
			ExternalID: S(commonName),
		}

		identityCreateEnv := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(identityCreateEnv).SetBody(identityPost).Post("/identities")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(identityCreateEnv)
		ctx.NotNil(identityCreateEnv.Data)
		ctx.NotEmpty(identityCreateEnv.Data.ID)

		certAuth1 := certAuthenticator{
			cert: clientCert1,
			key:  clientKey1,
		}

		certAuth2 := certAuthenticator{
			cert: clientCert2,
			key:  clientKey2,
		}

		session1, err := certAuth1.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(session1)

		session2, err := certAuth2.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(session2)

		//create service
		service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

		t.Run("start server & client", func(t *testing.T) {
			ctx.testContextChanged(t)

			//start server
			_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
			defer context.Close()

			listener, err := context.Listen(service.Name)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(listener)
			defer func() {
				ctx.Req.NoError(listener.Close())
			}()

			listenerEstablished := make(chan struct{}, 1)
			listener.(edge.SessionListener).SetConnectionChangeHandler(func(conn []edge.Listener) {
				if len(conn) > 0 {
					close(listenerEstablished)
				}
			})
			select {
			case <-listenerEstablished:
			case <-time.After(5 * time.Second):
				ctx.Fail("timed out waiting for listener to be established")
			}

			errC := make(chan error, 1)
			done := make(chan struct{})

			go func() {
				defer func() {
					val := recover()
					if val != nil {
						err := val.(error)
						errC <- err
					}
					close(errC)
				}()

				go func() {
					<-done
					_ = listener.Close()
				}()

				for {
					edgeCon, err := listener.AcceptEdge()

					if err != nil {
						return
					}

					go func() {
						conn := ctx.WrapNetConn(edgeCon, err)

						if listener.IsClosed() || conn.IsClosed() {
							return
						}
						conn.WriteString("hello", time.Second)
						conn.RequireClose()
					}()
				}

			}()

			doneClient1 := make(chan string)
			doneClient2 := make(chan string)

			go func() {
				//connect client 1
				client1Config := &ziti.Config{
					ZtAPI: "https://" + ctx.ApiHost + EdgeClientApiPath,
					ID: id.Config{
						Key:            id.StoragePem + ":" + string(clientKeyPem1),
						Cert:           id.StoragePem + ":" + clientCertPem1,
						ServerCert:     "",
						ServerKey:      "",
						AltServerCerts: nil,
						CA:             id.StoragePem + ":" + string(ctx.EdgeController.AppEnv.Config.CaPems()),
					},
					ConfigTypes: nil,
				}
				client1Context, err := ziti.NewContext(client1Config)
				ctx.Req.NoError(err)

				err = client1Context.Authenticate()
				ctx.Req.NoError(err)

				defer func() {
					client1Context.Close()
					close(doneClient1)
				}()

				conn, err := client1Context.Dial(service.Name)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(conn)

				bytes, err := io.ReadAll(conn)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(bytes)

				doneClient1 <- string(bytes)
			}()

			go func() {
				//connect client 2
				client2Config := &ziti.Config{
					ZtAPI: "https://" + ctx.ApiHost + EdgeClientApiPath,
					ID: id.Config{
						Key:            id.StoragePem + ":" + string(clientKeyPem2),
						Cert:           id.StoragePem + ":" + clientCertPem2,
						ServerCert:     "",
						ServerKey:      "",
						AltServerCerts: nil,
						CA:             id.StoragePem + ":" + string(ctx.EdgeController.AppEnv.Config.CaPems()),
					},
					ConfigTypes: nil,
				}

				client2Context, err := ziti.NewContext(client2Config)
				ctx.Req.NoError(err)

				err = client2Context.Authenticate()
				ctx.Req.NoError(err)

				defer func() {
					client2Context.Close()
					close(doneClient2)
				}()

				conn, err := client2Context.Dial(service.Name)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(conn)

				bytes, err := io.ReadAll(conn)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(bytes)

				doneClient2 <- string(bytes)
			}()

			t.Run("wait for service data", func(t *testing.T) {
				ctx.testContextChanged(t)

				select {
				case resp := <-doneClient1:
					ctx.Req.Equal("hello", resp)
				case <-time.After(5 * time.Second):
				}

				select {
				case resp := <-doneClient2:
					ctx.Req.Equal("hello", resp)
				case <-time.After(5 * time.Second):
				}

				close(done)
			})
		})
	})

}
