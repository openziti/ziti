package sync_strats

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

func Test_newPublicKey(t *testing.T) {
	data := []byte("anchor-cert-der")
	intermediate1 := []byte("intermediate-1-der")
	intermediate2 := []byte("intermediate-2-der")

	t.Run("sets kid from data fingerprint and carries usages and intermediates", func(t *testing.T) {
		req := require.New(t)

		publicKey := newPublicKey(data, edge_ctrl_pb.DataState_PublicKey_X509CertDer, firstPartyCaUsages, intermediate1, intermediate2)

		req.Equal(data, publicKey.Data)
		req.Equal(fmt.Sprintf("%x", sha1.Sum(data)), publicKey.Kid)
		req.Equal(firstPartyCaUsages, publicKey.Usages)
		req.Equal(edge_ctrl_pb.DataState_PublicKey_X509CertDer, publicKey.Format)
		req.Equal([][]byte{intermediate1, intermediate2}, publicKey.Intermediates)
	})

	t.Run("no intermediates yields empty intermediates", func(t *testing.T) {
		req := require.New(t)

		publicKey := newPublicKey(data, edge_ctrl_pb.DataState_PublicKey_X509CertDer, thirdPartyCaUsages)

		req.Empty(publicKey.Intermediates)
	})

	t.Run("usage sets pair the deprecated usage with the party-specific usage", func(t *testing.T) {
		req := require.New(t)

		req.Equal([]edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_JWTValidation}, controllerCertUsages)

		req.Contains(firstPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation)
		req.Contains(firstPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation)
		req.NotContains(firstPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_ThirdPartyX509CertValidation)

		req.Contains(thirdPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation)
		req.Contains(thirdPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_ThirdPartyX509CertValidation)
		req.NotContains(thirdPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation)
	})
}

func Test_firstPartyCaPublicKeys(t *testing.T) {
	root, rootKey := newTestCa(t, "root", nil, nil)
	intermediate, intermediateKey := newTestCa(t, "intermediate", root, rootKey)
	otherRoot, _ := newTestCa(t, "other-root", nil, nil)
	leaf := newTestLeaf(t, "leaf", intermediate, intermediateKey)

	t.Run("publishes each root as a first-party anchor with no intermediates", func(t *testing.T) {
		req := require.New(t)

		keys := firstPartyCaPublicKeys([]*x509.Certificate{intermediate, root, leaf, otherRoot})

		req.Len(keys, 2)
		req.ElementsMatch([][]byte{root.Raw, otherRoot.Raw}, [][]byte{keys[0].Data, keys[1].Data})
		for _, key := range keys {
			req.Equal(firstPartyCaUsages, key.Usages)
			req.Empty(key.Intermediates)
		}
	})

	t.Run("a bundle with no roots publishes nothing", func(t *testing.T) {
		require.Empty(t, firstPartyCaPublicKeys([]*x509.Certificate{intermediate, leaf}))
	})
}

// newTestCa issues a CA certificate named name, self-signed when parent is nil.
func newTestCa(t *testing.T, name string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	return newTestCert(t, name, true, parent, parentKey)
}

// newTestLeaf issues an end-entity certificate named name from the given CA.
func newTestLeaf(t *testing.T, name string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) *x509.Certificate {
	cert, _ := newTestCert(t, name, false, parent, parentKey)
	return cert
}

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
