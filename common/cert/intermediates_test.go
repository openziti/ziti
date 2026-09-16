package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestCert issues a certificate named name. A nil parent yields a self-signed root; isCa controls
// whether the result can itself issue certificates.
func newTestCert(t *testing.T, name string, isCa bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  isCa,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	if isCa {
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	issuer, issuerKey := template, key
	if parent != nil {
		issuer, issuerKey = parent, parentKey
	}

	der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key
}

func Test_Intermediates(t *testing.T) {
	root, rootKey := newTestCert(t, "root", true, nil, nil)
	intermediate, intermediateKey := newTestCert(t, "intermediate", true, root, rootKey)
	leaf, _ := newTestCert(t, "leaf", false, intermediate, intermediateKey)

	t.Run("keeps only non-root CA certs, in input order", func(t *testing.T) {
		req := require.New(t)
		other, _ := newTestCert(t, "other-intermediate", true, root, rootKey)

		result := Intermediates([]*x509.Certificate{leaf, other, root, intermediate})

		req.Equal([]*x509.Certificate{other, intermediate}, result)
	})

	t.Run("empty and root-only inputs yield nothing", func(t *testing.T) {
		req := require.New(t)

		req.Empty(Intermediates(nil))
		req.Empty(Intermediates([]*x509.Certificate{root, leaf}))
	})

	t.Run("IsRootCa distinguishes self-signed CAs", func(t *testing.T) {
		req := require.New(t)

		req.True(IsRootCa(root))
		req.False(IsRootCa(intermediate))
		req.False(IsRootCa(leaf))
	})
}

func Test_UniqueSortedDer(t *testing.T) {
	root, rootKey := newTestCert(t, "root", true, nil, nil)
	a, _ := newTestCert(t, "a", true, root, rootKey)
	b, _ := newTestCert(t, "b", true, root, rootKey)

	t.Run("output is independent of input order and duplicates", func(t *testing.T) {
		req := require.New(t)

		first := UniqueSortedDer([]*x509.Certificate{a, b, a})
		second := UniqueSortedDer([]*x509.Certificate{b, a})

		req.Len(first, 2)
		req.Equal(first, second)
	})

	t.Run("empty input yields nothing", func(t *testing.T) {
		require.Empty(t, UniqueSortedDer(nil))
	})
}
