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

package db

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/google/uuid"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"go.etcd.io/bbolt"
	"math/big"
	"testing"
	"time"
)

func Test_ControllerStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("create controller saves all values", func(t *testing.T) {
		ctx.NextTest(t)

		testControllerCert, err := newTestSelfSignedCert()
		ctx.NoError(err)
		ctx.NotNil(testControllerCert)

		newController := &Controller{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          "test-controller-" + uuid.NewString(),
			CtrlAddress:   "test.controller.example.com:1234",
			CertPem:       testControllerCert.certPem,
			Fingerprint:   testControllerCert.fingerprint,
			IsOnline:      true,
			LastJoinedAt:  time.Now(),
			ApiAddresses: map[string][]ApiAddress{
				"v1": {
					{
						Url:     "https://test.controller.example.com:1234/v1",
						Version: "v1",
					},
				},
				"v2": {
					{
						Url:     "https://test.controller.example.com:1234/v2",
						Version: "v2",
					},
				},
			},
		}

		boltztest.RequireCreate(ctx, newController)

		t.Run("has the proper values", func(t *testing.T) {
			ctx.NextTest(t)

			boltztest.ValidateBaseline(ctx, newController)

			err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
				readController, err := ctx.stores.Controller.LoadById(tx, newController.Id)
				ctx.NoError(err)
				ctx.NotNil(readController)
				ctx.Equal(newController.Name, readController.Name)
				ctx.Equal(newController.CtrlAddress, readController.CtrlAddress)
				ctx.Equal(newController.Fingerprint, readController.Fingerprint)
				ctx.Equal(newController.IsOnline, readController.IsOnline)
				ctx.Equal(newController.LastJoinedAt.UTC(), readController.LastJoinedAt.UTC())
				ctx.Equal(newController.CertPem, readController.CertPem)
				ctx.Equal(len(newController.ApiAddresses), len(readController.ApiAddresses))

				for apiKey, newApiList := range newController.ApiAddresses {
					readApiList, ok := readController.ApiAddresses[apiKey]
					ctx.True(ok)
					ctx.Equal(len(newApiList), len(readApiList))

					for i, newApi := range newApiList {
						readApi := readApiList[i]
						ctx.Equal(newApi.Url, readApi.Url)
						ctx.Equal(newApi.Version, readApi.Version)
					}
				}

				return nil
			})

			t.Run("updating isOnline after create only updates isOnline", func(t *testing.T) {
				ctx.NextTest(t)

				updateController := &Controller{
					BaseExtEntity: boltz.BaseExtEntity{Id: newController.Id},
					IsOnline:      false,
				}

				fieldChecker := boltz.MapFieldChecker{
					FieldControllerIsOnline: struct{}{},
				}

				boltztest.RequirePatch(ctx, updateController, fieldChecker)

				t.Run("only isOnline was changed", func(t *testing.T) {
					ctx.NextTest(t)

					err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
						readController, err := ctx.stores.Controller.LoadById(tx, newController.Id)
						ctx.NoError(err)
						ctx.NotNil(readController)

						//changed
						ctx.Equal(false, readController.IsOnline)

						//no change
						ctx.Equal(newController.Name, readController.Name)
						ctx.Equal(newController.CtrlAddress, readController.CtrlAddress)
						ctx.Equal(newController.Fingerprint, readController.Fingerprint)

						ctx.Equal(newController.LastJoinedAt.UTC(), readController.LastJoinedAt.UTC())
						ctx.Equal(newController.CertPem, readController.CertPem)
						ctx.Equal(len(newController.ApiAddresses), len(readController.ApiAddresses))

						for apiKey, newApiList := range newController.ApiAddresses {
							readApiList, ok := readController.ApiAddresses[apiKey]
							ctx.True(ok)
							ctx.Equal(len(newApiList), len(readApiList))

							for i, newApi := range newApiList {
								readApi := readApiList[i]
								ctx.Equal(newApi.Url, readApi.Url)
								ctx.Equal(newApi.Version, readApi.Version)
							}
						}

						return nil
					})
				})
			})

			t.Run("updating api address after create only updates api addresses", func(t *testing.T) {
				ctx.NextTest(t)

				updateController := &Controller{
					BaseExtEntity: boltz.BaseExtEntity{Id: newController.Id},
					ApiAddresses: map[string][]ApiAddress{
						"v1000": {
							{
								Url:     "https://test.controller.example.com:1000/v1",
								Version: "v1000",
							},
						},
						"v2000": {
							{
								Url:     "https://test.controller.example.com:2000/v2",
								Version: "v2000",
							},
						},
					},
				}

				fieldChecker := boltz.MapFieldChecker{
					FieldControllerApiAddresses:      struct{}{},
					FieldControllerApiAddressUrl:     struct{}{},
					FieldControllerApiAddressVersion: struct{}{},
				}

				boltztest.RequirePatch(ctx, updateController, fieldChecker)

				t.Run("only isOnline was changed", func(t *testing.T) {
					ctx.NextTest(t)

					err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
						readController, err := ctx.stores.Controller.LoadById(tx, newController.Id)
						ctx.NoError(err)
						ctx.NotNil(readController)

						//changed
						ctx.Equal(len(updateController.ApiAddresses), len(readController.ApiAddresses))

						for updateApiKey, updateApiList := range updateController.ApiAddresses {
							readApiList, ok := readController.ApiAddresses[updateApiKey]
							ctx.True(ok)
							ctx.Equal(len(updateApiList), len(readApiList))

							for i, updateApi := range updateApiList {
								readApi := readApiList[i]
								ctx.Equal(updateApi.Url, readApi.Url)
								ctx.Equal(updateApi.Version, readApi.Version)
							}
						}

						//no change
						ctx.Equal(false, readController.IsOnline) //false from previous test setting to false
						ctx.Equal(newController.Name, readController.Name)
						ctx.Equal(newController.CtrlAddress, readController.CtrlAddress)
						ctx.Equal(newController.Fingerprint, readController.Fingerprint)
						ctx.Equal(newController.LastJoinedAt.UTC(), readController.LastJoinedAt.UTC())
						ctx.Equal(newController.CertPem, readController.CertPem)

						return nil
					})
				})
			})
		})
	})

}

type testCert struct {
	certPem     string
	fingerprint string
	key         crypto.PrivateKey
}

func newTestSelfSignedCert() (*testCert, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %s", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 1 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %s", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Controller Store Test Org"},
			CommonName:   "test.controller.store.example.com",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %s", err)
	}

	certPemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	return &testCert{
		certPem:     string(certPemBytes),
		fingerprint: fmt.Sprintf("%x", sha1.Sum(certPemBytes)),
		key:         priv,
	}, nil
}
