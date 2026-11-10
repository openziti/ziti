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

package model

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/stretchr/testify/require"
)

func TestRetainOutgoingCert(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	newCertPem := func(serial int64, notBefore time.Time, lifetime time.Duration) string {
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: "ctrl1"},
			NotBefore:    notBefore,
			NotAfter:     notBefore.Add(lifetime),
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		require.NoError(t, err)
		cert, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		return nfpem.EncodeToString(cert)
	}

	year := 365 * 24 * time.Hour
	first := newCertPem(1, now.Add(-60*24*time.Hour), year)
	second := newCertPem(2, now.Add(-30*24*time.Hour), year)
	third := newCertPem(3, now, year)

	t.Run("a changed cert retains the outgoing leaf", func(t *testing.T) {
		result := retainOutgoingCert(nil, first, second)
		require.Equal(t, []string{first}, result)
	})

	t.Run("an unchanged cert leaves retained alone", func(t *testing.T) {
		result := retainOutgoingCert([]string{first}, second, second)
		require.Equal(t, []string{first}, result)
	})

	t.Run("reverting to a retained cert swaps it with the current one", func(t *testing.T) {
		result := retainOutgoingCert([]string{first}, second, first)
		require.Equal(t, []string{second}, result)
	})

	t.Run("retained certs accumulate oldest first", func(t *testing.T) {
		result := retainOutgoingCert([]string{second, first}, third, newCertPem(4, now.Add(time.Hour), year))
		require.Equal(t, []string{first, second, third}, result)
	})

	t.Run("a cert expired before the incoming one was issued is dropped", func(t *testing.T) {
		expired := newCertPem(5, now.Add(-2*year), year)
		result := retainOutgoingCert([]string{expired, first}, second, third)
		require.Equal(t, []string{first, second}, result)
	})

	t.Run("an unparseable incoming cert still retains the outgoing leaf", func(t *testing.T) {
		result := retainOutgoingCert(nil, first, "")
		require.Equal(t, []string{first}, result)
	})

	t.Run("past the limit the earliest issued are dropped", func(t *testing.T) {
		var retained []string
		for i := 0; i < maxRetainedCerts; i++ {
			retained = append(retained, newCertPem(int64(100+i), now.Add(time.Duration(i-maxRetainedCerts)*time.Hour), year))
		}

		outgoing := newCertPem(200, now.Add(-time.Minute), year)
		result := retainOutgoingCert(retained, outgoing, newCertPem(201, now, year))

		require.Len(t, result, maxRetainedCerts)
		require.Equal(t, retained[1:], result[:maxRetainedCerts-1])
		require.Equal(t, outgoing, result[maxRetainedCerts-1])
	})
}
