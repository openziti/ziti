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

	nfPem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/ziti/v2/common/cert"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/db"
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

// newTestCa issues a CA certificate named name, self-signed when parent is nil.
func newTestCa(t *testing.T, name string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
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

func pemOf(certs ...*x509.Certificate) string {
	result := ""
	for _, cert := range certs {
		result += nfPem.EncodeToString(cert)
	}
	return result
}

func Test_firstPartyCaPublicKeys(t *testing.T) {
	root, rootKey := newTestCa(t, "root", nil, nil)
	signingRoot, signingRootKey := newTestCa(t, "signing-root", nil, nil)

	// Per-controller intermediates: ctrl channel intermediates under root, edge signing
	// intermediates under signingRoot.
	ctrl1Int, _ := newTestCa(t, "ctrl1-intermediate", root, rootKey)
	ctrl2Int, _ := newTestCa(t, "ctrl2-intermediate", root, rootKey)
	signing1, _ := newTestCa(t, "signing1", signingRoot, signingRootKey)
	signing2, _ := newTestCa(t, "signing2", signingRoot, signingRootKey)

	// ctrl1's view: its own bundle plus the replicated controller records.
	ctrl1Bundle := []*x509.Certificate{root, signingRoot, signing1}
	ctrl2Bundle := []*x509.Certificate{root, signingRoot, signing2}
	controllers := []*db.Controller{
		{CertPem: pemOf(ctrl1Int), CaPem: pemOf(signing1)},
		{CertPem: pemOf(ctrl2Int), CaPem: pemOf(signing2)},
	}

	t.Run("publishes one key per root carrying the union of all intermediates", func(t *testing.T) {
		req := require.New(t)

		keys := firstPartyCaPublicKeys(ctrl1Bundle, controllers)

		req.Len(keys, 2)
		expected := cert.UniqueSortedDer([]*x509.Certificate{ctrl1Int, ctrl2Int, signing1, signing2})
		for _, key := range keys {
			req.Equal(firstPartyCaUsages, key.Usages)
			req.Equal(expected, key.Intermediates)
		}
		req.ElementsMatch([][]byte{root.Raw, signingRoot.Raw}, [][]byte{keys[0].Data, keys[1].Data})
	})

	t.Run("every controller computes identical content for a shared kid", func(t *testing.T) {
		req := require.New(t)

		fromCtrl1 := firstPartyCaPublicKeys(ctrl1Bundle, controllers)
		fromCtrl2 := firstPartyCaPublicKeys(ctrl2Bundle, []*db.Controller{controllers[1], controllers[0]})

		req.Len(fromCtrl2, 2)
		byKid := map[string]*edge_ctrl_pb.DataState_PublicKey{}
		for _, key := range fromCtrl2 {
			byKid[key.Kid] = key
		}
		for _, key := range fromCtrl1 {
			req.Equal(key.Intermediates, byKid[key.Kid].Intermediates)
		}
	})

	t.Run("records without CA intermediates or certs are tolerated", func(t *testing.T) {
		req := require.New(t)

		keys := firstPartyCaPublicKeys([]*x509.Certificate{root}, []*db.Controller{{}, {CertPem: "not a cert"}})

		req.Len(keys, 1)
		req.Empty(keys[0].Intermediates)
	})

	t.Run("non-root bundle entries are intermediates, not keys", func(t *testing.T) {
		req := require.New(t)

		keys := firstPartyCaPublicKeys([]*x509.Certificate{signing1}, nil)

		req.Empty(keys)
	})
}
