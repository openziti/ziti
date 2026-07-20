package mesh

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

// newTestCertChain creates a self-signed root and a leaf issued by it, returning [leaf, root].
func newTestCertChain(name string) ([]*x509.Certificate, error) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name + "-root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	rootDer, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	rootCert, err := x509.ParseCertificate(rootDer)
	if err != nil {
		return nil, err
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: name + "-leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	leafDer, err := x509.CreateCertificate(rand.Reader, leafTemplate, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	leafCert, err := x509.ParseCertificate(leafDer)
	if err != nil {
		return nil, err
	}

	return []*x509.Certificate{leafCert, rootCert}, nil
}

func Test_signingCertsFromHeaders(t *testing.T) {
	chain, err := newTestCertChain("mesh-test")
	require.NoError(t, err)

	t.Run("chain header yields the full chain", func(t *testing.T) {
		req := require.New(t)
		headers := map[int32][]byte{
			SigningCertChainHeader: ConcatDer([][]byte{chain[0].Raw, chain[1].Raw}),
			SigningCertHeader:      chain[0].Raw,
		}

		certs := signingCertsFromHeaders(headers)
		req.Len(certs, 2)
		req.True(certs[0].Equal(chain[0]))
		req.True(certs[1].Equal(chain[1]))
	})

	t.Run("falls back to the single-cert header when no chain header present", func(t *testing.T) {
		req := require.New(t)
		headers := map[int32][]byte{
			SigningCertHeader: chain[0].Raw,
		}

		certs := signingCertsFromHeaders(headers)
		req.Len(certs, 1)
		req.True(certs[0].Equal(chain[0]))
	})

	t.Run("falls back to the legacy single-cert header", func(t *testing.T) {
		req := require.New(t)
		headers := map[int32][]byte{
			LegacySigningCertHeader: chain[0].Raw,
		}

		certs := signingCertsFromHeaders(headers)
		req.Len(certs, 1)
		req.True(certs[0].Equal(chain[0]))
	})

	t.Run("unparseable chain header falls back to the single-cert header", func(t *testing.T) {
		req := require.New(t)
		headers := map[int32][]byte{
			SigningCertChainHeader: []byte("not a certificate"),
			SigningCertHeader:      chain[0].Raw,
		}

		certs := signingCertsFromHeaders(headers)
		req.Len(certs, 1)
		req.True(certs[0].Equal(chain[0]))
	})

	t.Run("no cert headers yields nil", func(t *testing.T) {
		req := require.New(t)
		certs := signingCertsFromHeaders(map[int32][]byte{})
		req.Nil(certs)
	})
}
