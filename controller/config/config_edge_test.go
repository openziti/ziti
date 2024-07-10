package config

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"github.com/google/uuid"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

func Test_validateHostPortString(t *testing.T) {
	t.Run("a hostname and port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:123")

		req.NoError(err)
	})

	t.Run("an ipv4 localhost and port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("127.0.0.1:123")

		req.NoError(err)
	})

	t.Run("an ipv6 localhost and port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("[::1]:123")

		req.NoError(err)
	})

	t.Run("a hostname and a blank port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:")

		req.Error(err)
	})

	t.Run("a blank hostname and a port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString(":123")

		req.Error(err)
	})

	t.Run("a blank hostname and a blank port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString(":")

		req.Error(err)
	})

	t.Run("too many colons with blank host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("::")

		req.Error(err)
	})

	t.Run("extra trailing colons with host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("myhost:123:")

		req.Error(err)
	})

	t.Run("extra middle colon with host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("myhost::123")

		req.Error(err)
	})

	t.Run("extra leading colon with host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString(":myhost::123")

		req.Error(err)
	})

	t.Run("extra leading colon with host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("")

		req.Error(err)
	})

	t.Run("host with scheme should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("http://myhost:80")

		req.Error(err)
	})

	t.Run("host with scheme should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("http://myhost:80")

		req.Error(err)
	})

	t.Run("host with non-integer port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:nooooooooo")

		req.Error(err)
	})

	t.Run("host with negative port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:-1")

		req.Error(err)
	})

	t.Run("host with 0 port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:0")

		req.Error(err)
	})

	t.Run("host with 1 port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:1")

		req.NoError(err)
	})

	t.Run("host with 65535 port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:65535")

		req.NoError(err)
	})

	t.Run("host with 65535 port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:65535")

		req.NoError(err)
	})

	t.Run("host and port with trailing space pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("   hello.com:65535   ")

		req.NoError(err)
	})

	t.Run("white space host with port with fails", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("   :65535   ")

		req.Error(err)
	})

	t.Run("white space post with host fails", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("myhost:           ")

		req.Error(err)
	})
}

func Test_CalculateCaPems(t *testing.T) {
	ca1, _ := newSelfSignedCert(uuid.NewString(), true)
	ca2, _ := newSelfSignedCert(uuid.NewString(), true)
	ca3, _ := newSelfSignedCert(uuid.NewString(), true)

	notCaSelfSigned, _ := newSelfSignedCert(uuid.NewString(), false)

	ca1Pem := nfpem.EncodeToBytes(ca1)
	ca2Pem := nfpem.EncodeToBytes(ca2)
	ca3Pem := nfpem.EncodeToBytes(ca3)
	notCaSelfSignedPem := nfpem.EncodeToBytes(notCaSelfSigned)

	inCas := []*x509.Certificate{
		ca1,
		ca2,
		ca3,
	}

	t.Run("1 non-ca in, 0 out", func(t *testing.T) {
		req := require.New(t)

		buf := bytes.NewBuffer([]byte{})

		buf.Write(notCaSelfSignedPem)

		outBuf := CalculateCaPems(buf)

		outCerts := nfpem.PemBytesToCertificates(outBuf.Bytes())

		req.Len(outCerts, 0)
	})

	t.Run("1 non-ca + 3 ca in, 3 out", func(t *testing.T) {
		req := require.New(t)

		buf := bytes.NewBuffer([]byte{})

		buf.Write(notCaSelfSignedPem)
		buf.Write(ca1Pem)
		buf.Write(ca2Pem)
		buf.Write(ca3Pem)

		outBuf := CalculateCaPems(buf)

		outCerts := nfpem.PemBytesToCertificates(outBuf.Bytes())

		req.Len(outCerts, 3)

		for _, inCert := range inCas {
			found := false
			for _, outCert := range outCerts {
				if bytes.Equal(inCert.Raw, outCert.Raw) {
					req.Falsef(found, "certificate %s was found multiple times, expected once instance in output", inCert.Subject.String())

					found = true
				}
			}
			req.Truef(found, "certificate %s was provided as input but not found as output", inCert.Subject.String())
		}
	})

	t.Run("three unique CAs in, three out", func(t *testing.T) {
		req := require.New(t)

		buf := bytes.NewBuffer([]byte{})

		buf.Write(ca1Pem)
		buf.Write(ca2Pem)
		buf.Write(ca3Pem)

		outBuf := CalculateCaPems(buf)

		outCerts := nfpem.PemBytesToCertificates(outBuf.Bytes())

		req.Len(outCerts, 3)

		for _, inCert := range inCas {
			found := false
			for _, outCert := range outCerts {
				if bytes.Equal(inCert.Raw, outCert.Raw) {
					req.Falsef(found, "certificate %s was found multiple times, expected once instance in output", inCert.Subject.String())

					found = true
				}
			}
			req.Truef(found, "certificate %s was provided as input but not found as output", inCert.Subject.String())
		}
	})

	t.Run("0 unique CAs in, 0 out", func(t *testing.T) {
		req := require.New(t)

		buf := bytes.NewBuffer([]byte{})

		outBuf := CalculateCaPems(buf)

		outCerts := nfpem.PemBytesToCertificates(outBuf.Bytes())

		req.Len(outCerts, 0)
	})

	t.Run("1 unique CAs in, 1 out", func(t *testing.T) {
		req := require.New(t)

		buf := bytes.NewBuffer([]byte{})

		buf.Write(ca1Pem)

		outBuf := CalculateCaPems(buf)

		outCerts := nfpem.PemBytesToCertificates(outBuf.Bytes())

		req.Len(outCerts, 1)

		req.True(bytes.Equal(outCerts[0].Raw, ca1.Raw), "the in ca did not match the out ca")
	})

	t.Run("2 duplicate CAs in, 1 out", func(t *testing.T) {
		req := require.New(t)

		buf := bytes.NewBuffer([]byte{})

		buf.Write(ca1Pem)
		buf.Write(ca1Pem)

		outBuf := CalculateCaPems(buf)

		outCerts := nfpem.PemBytesToCertificates(outBuf.Bytes())

		req.Len(outCerts, 1)

		req.True(bytes.Equal(outCerts[0].Raw, ca1.Raw), "the in ca did not match the out ca")
	})

	t.Run("2 sets of 2 duplicate CAs in and 1 unique, 3 unique out", func(t *testing.T) {
		req := require.New(t)

		buf := bytes.NewBuffer([]byte{})

		buf.Write(ca1Pem) //uniques
		buf.Write(ca2Pem)
		buf.Write(ca3Pem)

		buf.Write(ca1Pem) //dupe 1
		buf.Write(ca2Pem) //dupe 2

		outBuf := CalculateCaPems(buf)

		outCerts := nfpem.PemBytesToCertificates(outBuf.Bytes())

		req.Len(outCerts, 3)

		for _, inCert := range inCas {
			found := false
			for _, outCert := range outCerts {
				if bytes.Equal(inCert.Raw, outCert.Raw) {
					req.Falsef(found, "certificate %s was found multiple times, expected once instance in output", inCert.Subject.String())

					found = true
				}
			}
			req.Truef(found, "certificate %s was provided as input but not found as output", inCert.Subject.String())
		}
	})

}

func newSelfSignedCert(commonName string, isCas bool) (*x509.Certificate, crypto.PrivateKey) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"API Test Co"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 180),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	if isCas {
		template.IsCA = true
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		panic(err)
	}
	cert, err := x509.ParseCertificate(der)

	if err != nil {
		panic(err)
	}

	return cert, priv
}
