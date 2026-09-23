package util

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeKeyPair writes a self-signed cert and its key as PEM files and returns both paths.
func writeKeyPair(t *testing.T, dir string) (certPath string, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "client-cert-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	keyDer, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "client.cert")
	keyPath = filepath.Join(dir, "client.key")
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer}), 0600))

	return certPath, keyPath
}

// writeIdentityFile writes an identity config whose cert and key are inlined as pem: values, the shape
// `ziti edge enroll` produces.
func writeIdentityFile(t *testing.T, dir, certPath, keyPath string) string {
	t.Helper()

	certPem, err := os.ReadFile(certPath)
	require.NoError(t, err)
	keyPem, err := os.ReadFile(keyPath)
	require.NoError(t, err)

	cfg := map[string]any{
		"ztAPI": "https://localhost:1280/edge/client/v1",
		"id": map[string]any{
			"cert": "pem:" + string(certPem),
			"key":  "pem:" + string(keyPem),
			"ca":   "pem:" + string(certPem),
		},
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	path := filepath.Join(dir, "identity.json")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}

func TestClientCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeKeyPair(t, dir)
	idPath := writeIdentityFile(t, dir, certPath, keyPath)

	t.Run("no client credential returns nothing", func(t *testing.T) {
		id := &RestClientEdgeIdentity{CaCert: "ignored"}

		cert, err := id.clientCertificate()
		require.NoError(t, err, "a username/password login has no client certificate and that is not an error")
		require.Nil(t, cert)
	})

	t.Run("identity file", func(t *testing.T) {
		id := &RestClientEdgeIdentity{ClientIdFile: idPath}

		cert, err := id.clientCertificate()
		require.NoError(t, err)
		require.NotNil(t, cert)
		require.NotEmpty(t, cert.Certificate)
	})

	t.Run("identity file is missing", func(t *testing.T) {
		id := &RestClientEdgeIdentity{ClientIdFile: filepath.Join(dir, "moved-away.json")}

		_, err := id.clientCertificate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no longer readable")
		require.Contains(t, err.Error(), "ziti edge login", "the message has to say how to recover")
	})

	t.Run("identity file is not valid json", func(t *testing.T) {
		junk := filepath.Join(dir, "junk.json")
		require.NoError(t, os.WriteFile(junk, []byte("this is not an identity"), 0600))
		id := &RestClientEdgeIdentity{ClientIdFile: junk}

		_, err := id.clientCertificate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "ziti edge login")
	})

	t.Run("identity file has no usable cert", func(t *testing.T) {
		empty := filepath.Join(dir, "empty-id.json")
		require.NoError(t, os.WriteFile(empty, []byte(`{"ztAPI":"https://localhost:1280","id":{}}`), 0600))
		id := &RestClientEdgeIdentity{ClientIdFile: empty}

		_, err := id.clientCertificate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "ziti edge login")
	})

	t.Run("cert and key pair", func(t *testing.T) {
		id := &RestClientEdgeIdentity{ClientCert: certPath, ClientKey: keyPath}

		cert, err := id.clientCertificate()
		require.NoError(t, err)
		require.NotNil(t, cert)
		require.NotEmpty(t, cert.Certificate)
	})

	t.Run("cert without key is ignored", func(t *testing.T) {
		id := &RestClientEdgeIdentity{ClientCert: certPath}

		cert, err := id.clientCertificate()
		require.NoError(t, err, "a half-configured pair cannot be loaded, and login would have rejected it")
		require.Nil(t, cert)
	})

	t.Run("key without cert is ignored", func(t *testing.T) {
		id := &RestClientEdgeIdentity{ClientKey: keyPath}

		cert, err := id.clientCertificate()
		require.NoError(t, err)
		require.Nil(t, cert)
	})

	t.Run("cert file is missing", func(t *testing.T) {
		id := &RestClientEdgeIdentity{ClientCert: filepath.Join(dir, "gone.cert"), ClientKey: keyPath}

		_, err := id.clientCertificate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no longer usable")
		require.Contains(t, err.Error(), "ziti edge login")
	})

	t.Run("key file is missing", func(t *testing.T) {
		id := &RestClientEdgeIdentity{ClientCert: certPath, ClientKey: filepath.Join(dir, "gone.key")}

		_, err := id.clientCertificate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "ziti edge login")
	})

	t.Run("key does not match cert", func(t *testing.T) {
		otherDir := t.TempDir()
		_, otherKey := writeKeyPair(t, otherDir)
		id := &RestClientEdgeIdentity{ClientCert: certPath, ClientKey: otherKey}

		_, err := id.clientCertificate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no longer usable")
	})

	t.Run("identity file wins over a cert and key pair", func(t *testing.T) {
		id := &RestClientEdgeIdentity{
			ClientIdFile: idPath,
			ClientCert:   filepath.Join(dir, "gone.cert"),
			ClientKey:    filepath.Join(dir, "gone.key"),
		}

		cert, err := id.clientCertificate()
		require.NoError(t, err, "the identity file is checked first, so the unusable pair never loads")
		require.NotNil(t, cert)
	})
}

func TestNewTlsClientConfigPresentsClientCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeKeyPair(t, dir)

	t.Run("certificate is attached", func(t *testing.T) {
		id := &RestClientEdgeIdentity{ClientCert: certPath, ClientKey: keyPath}

		cfg, err := id.NewTlsClientConfig()
		require.NoError(t, err)
		require.Len(t, cfg.Certificates, 1, "cert bound sessions fail proof of possession without this")
	})

	t.Run("no certificate when the login was not cert based", func(t *testing.T) {
		id := &RestClientEdgeIdentity{}

		cfg, err := id.NewTlsClientConfig()
		require.NoError(t, err)
		require.Empty(t, cfg.Certificates)
	})
}
