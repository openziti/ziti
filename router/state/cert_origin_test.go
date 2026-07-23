package state

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"

	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
)

// testCa holds a CA certificate and its signing key for issuing test certificates.
type testCa struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// newTestRootCa creates a self-signed root CA.
func newTestRootCa(name string) (*testCa, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(int64(1)<<62))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	return &testCa{cert: cert, key: key}, nil
}

// newIntermediateCa creates an intermediate CA issued by this CA.
func (ca *testCa) newIntermediateCa(name string) (*testCa, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(int64(1)<<62))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	return &testCa{cert: cert, key: key}, nil
}

// issueClientCert issues a client leaf certificate from this CA.
func (ca *testCa) issueClientCert(name string) (*x509.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(int64(1)<<62))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(der)
}

// newCertOriginTestManager builds a ManagerImpl with just enough state for
// IsFirstPartyCert: a pre-seeded ctrl-channel root pool and an empty router data model.
func newCertOriginTestManager(ctrlRoot *x509.Certificate) *ManagerImpl {
	mgr := &ManagerImpl{
		certCache: cmap.New[*x509.Certificate](),
	}

	if ctrlRoot != nil {
		mgr.ctrlRootCache.roots = []*x509.Certificate{ctrlRoot}
	}
	mgr.ctrlRootCache.inited = true

	mgr.routerDataModel.Store(common.NewBareRouterDataModel())

	return mgr
}

// publishPublicKey adds a public key to the manager's router data model with the given
// usages and optional published intermediates.
func publishPublicKey(mgr *ManagerImpl, kid string, cert *x509.Certificate, usages []edge_ctrl_pb.DataState_PublicKey_Usage, intermediates ...*x509.Certificate) {
	publicKey := &edge_ctrl_pb.DataState_PublicKey{
		Data:   cert.Raw,
		Kid:    kid,
		Usages: usages,
		Format: edge_ctrl_pb.DataState_PublicKey_X509CertDer,
	}

	for _, intermediate := range intermediates {
		publicKey.Intermediates = append(publicKey.Intermediates, intermediate.Raw)
	}

	mgr.routerDataModel.Load().PublicKeys.Set(kid, publicKey)
}

func Test_IsFirstPartyCert(t *testing.T) {
	ctrlCa, err := newTestRootCa("ctrl-root")
	require.NoError(t, err)

	signingCa, err := newTestRootCa("edge-signing-root")
	require.NoError(t, err)

	thirdPartyCa, err := newTestRootCa("third-party-root")
	require.NoError(t, err)

	t.Run("cert issued by the ctrl channel root is first party", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(ctrlCa.cert)

		clientCert, err := ctrlCa.issueClientCert("client-ctrl")
		req.NoError(err)

		req.True(mgr.IsFirstPartyCert([]*x509.Certificate{clientCert}))
	})

	t.Run("cert issued by a distinct edge signing CA published as first party is first party", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(ctrlCa.cert)
		publishPublicKey(mgr, "signing-root", signingCa.cert, []edge_ctrl_pb.DataState_PublicKey_Usage{
			edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation,
			edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation,
		})

		clientCert, err := signingCa.issueClientCert("client-signing")
		req.NoError(err)

		req.True(mgr.IsFirstPartyCert([]*x509.Certificate{clientCert}))
	})

	t.Run("cert issued by a published intermediate of the edge signing CA is first party", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(ctrlCa.cert)

		intermediateCa, err := signingCa.newIntermediateCa("edge-signing-intermediate")
		req.NoError(err)

		publishPublicKey(mgr, "signing-root", signingCa.cert, []edge_ctrl_pb.DataState_PublicKey_Usage{
			edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation,
			edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation,
		}, intermediateCa.cert)

		clientCert, err := intermediateCa.issueClientCert("client-intermediate")
		req.NoError(err)

		// leaf only; the intermediate must come from the published key, not the peer chain
		req.True(mgr.IsFirstPartyCert([]*x509.Certificate{clientCert}))
	})

	t.Run("cert issued by a third party CA is not first party", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(ctrlCa.cert)
		publishPublicKey(mgr, "third-party-root", thirdPartyCa.cert, []edge_ctrl_pb.DataState_PublicKey_Usage{
			edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation,
			edge_ctrl_pb.DataState_PublicKey_ThirdPartyX509CertValidation,
		})

		clientCert, err := thirdPartyCa.issueClientCert("client-third-party")
		req.NoError(err)

		req.False(mgr.IsFirstPartyCert([]*x509.Certificate{clientCert}))
	})

	t.Run("cert issued by an unpublished CA is not first party", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(ctrlCa.cert)

		clientCert, err := signingCa.issueClientCert("client-unpublished")
		req.NoError(err)

		req.False(mgr.IsFirstPartyCert([]*x509.Certificate{clientCert}))
	})

	t.Run("old controller publishing only deprecated usage retains prior behavior", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(ctrlCa.cert)
		publishPublicKey(mgr, "signing-root", signingCa.cert, []edge_ctrl_pb.DataState_PublicKey_Usage{
			edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation,
		})

		ctrlClientCert, err := ctrlCa.issueClientCert("client-ctrl-old")
		req.NoError(err)
		req.True(mgr.IsFirstPartyCert([]*x509.Certificate{ctrlClientCert}))

		signingClientCert, err := signingCa.issueClientCert("client-signing-old")
		req.NoError(err)
		req.False(mgr.IsFirstPartyCert([]*x509.Certificate{signingClientCert}))
	})

	t.Run("no peer certs is not first party", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(ctrlCa.cert)

		req.False(mgr.IsFirstPartyCert(nil))
	})
}

// parseCertDirect is a parseCert callback for buildClientCertRoots without caching.
func parseCertDirect(_ string, data []byte) (*x509.Certificate, error) {
	return x509.ParseCertificate(data)
}

// verifyAgainstRdmPools verifies cert against the root and intermediate pools built by
// buildClientCertRoots from rdm. A nil result means the cert chained to an anchor.
func verifyAgainstRdmPools(rdm *common.RouterDataModel, cert *x509.Certificate) error {
	roots, published, count := buildClientCertRoots(rdm, parseCertDirect)
	if count == 0 {
		return errors.New("no anchors selected")
	}
	intermediates := x509.NewCertPool()
	for _, intermediate := range published {
		intermediates.AddCert(intermediate)
	}
	_, err := cert.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	return err
}

func Test_buildClientCertRoots(t *testing.T) {
	firstPartyCa, err := newTestRootCa("first-party-root")
	require.NoError(t, err)

	thirdPartyCa, err := newTestRootCa("third-party-root")
	require.NoError(t, err)

	t.Run("first and third party keys are both anchors", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(nil)
		publishPublicKey(mgr, "first", firstPartyCa.cert, firstPartyCaUsagesForTest())
		publishPublicKey(mgr, "third", thirdPartyCa.cert, thirdPartyCaUsagesForTest())
		rdm := mgr.routerDataModel.Load()

		firstPartyClient, err := firstPartyCa.issueClientCert("client-first")
		req.NoError(err)
		req.NoError(verifyAgainstRdmPools(rdm, firstPartyClient))

		thirdPartyClient, err := thirdPartyCa.issueClientCert("client-third")
		req.NoError(err)
		req.NoError(verifyAgainstRdmPools(rdm, thirdPartyClient))
	})

	t.Run("deprecated-only key is ignored when new usages are present", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(nil)
		publishPublicKey(mgr, "first", firstPartyCa.cert, firstPartyCaUsagesForTest())
		publishPublicKey(mgr, "deprecated", thirdPartyCa.cert, []edge_ctrl_pb.DataState_PublicKey_Usage{
			edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation,
		})
		rdm := mgr.routerDataModel.Load()

		thirdPartyClient, err := thirdPartyCa.issueClientCert("client-third")
		req.NoError(err)
		req.Error(verifyAgainstRdmPools(rdm, thirdPartyClient))
	})

	t.Run("deprecated keys are anchors when no key has the new usages", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(nil)
		publishPublicKey(mgr, "deprecated", firstPartyCa.cert, []edge_ctrl_pb.DataState_PublicKey_Usage{
			edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation,
		})
		rdm := mgr.routerDataModel.Load()

		firstPartyClient, err := firstPartyCa.issueClientCert("client-first")
		req.NoError(err)
		req.NoError(verifyAgainstRdmPools(rdm, firstPartyClient))
	})

	t.Run("published intermediates chain leaf certs to the anchor", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(nil)

		intermediateCa, err := firstPartyCa.newIntermediateCa("first-party-intermediate")
		req.NoError(err)

		publishPublicKey(mgr, "first", firstPartyCa.cert, firstPartyCaUsagesForTest(), intermediateCa.cert)
		rdm := mgr.routerDataModel.Load()

		leaf, err := intermediateCa.issueClientCert("client-intermediate")
		req.NoError(err)
		req.NoError(verifyAgainstRdmPools(rdm, leaf))
	})

	t.Run("jwt-only key is not an anchor", func(t *testing.T) {
		req := require.New(t)
		mgr := newCertOriginTestManager(nil)
		publishPublicKey(mgr, "jwt", firstPartyCa.cert, []edge_ctrl_pb.DataState_PublicKey_Usage{
			edge_ctrl_pb.DataState_PublicKey_JWTValidation,
		})
		rdm := mgr.routerDataModel.Load()

		firstPartyClient, err := firstPartyCa.issueClientCert("client-first")
		req.NoError(err)
		req.Error(verifyAgainstRdmPools(rdm, firstPartyClient))
	})
}

// firstPartyCaUsagesForTest returns the usage set a current controller publishes for
// first-party CA roots.
func firstPartyCaUsagesForTest() []edge_ctrl_pb.DataState_PublicKey_Usage {
	return []edge_ctrl_pb.DataState_PublicKey_Usage{
		edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation,
		edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation,
	}
}

// thirdPartyCaUsagesForTest returns the usage set a current controller publishes for
// third-party CAs.
func thirdPartyCaUsagesForTest() []edge_ctrl_pb.DataState_PublicKey_Usage {
	return []edge_ctrl_pb.DataState_PublicKey_Usage{
		edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation,
		edge_ctrl_pb.DataState_PublicKey_ThirdPartyX509CertValidation,
	}
}
