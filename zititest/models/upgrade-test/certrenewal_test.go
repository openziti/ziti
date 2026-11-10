/*
	(c) Copyright NetFoundry Inc.

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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/ziti/v2/controller/jwtsigner"
	"github.com/stretchr/testify/require"
)

// TestSessionKidMatchesControllerSigner pins certKid and sessionKid to the controller's own kid
// derivation, so verifyCtrlCertsRenewed compares like with like.
func TestSessionKidMatchesControllerSigner(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "ctrl1"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	signer, err := jwtsigner.NewTlsJwtSigner(&tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key})
	require.NoError(t, err)
	token, err := signer.Generate(jwt.MapClaims{"sub": "admin"})
	require.NoError(t, err)

	leaf, err := firstCert(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	require.NoError(t, err)

	kid, err := sessionKid(edge_apis.NewApiSessionOidc(token, ""))
	require.NoError(t, err)
	require.Equal(t, signer.KeyId(), kid)
	require.Equal(t, certKid(leaf), kid)
}
