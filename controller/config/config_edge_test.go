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

func Test_loadExternalJwtSignersSection(t *testing.T) {
	t.Run("an absent section yields defaults", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		req.NoError(c.loadExternalJwtSignersSection(map[any]any{}))

		req.False(c.ExternalJwtSigners.JwksFetch.BlockPrivateAddresses)
		req.Empty(c.ExternalJwtSigners.JwksFetch.DeniedIPs)
		req.Empty(c.ExternalJwtSigners.JwksFetch.AllowedIPs)
		req.Empty(c.ExternalJwtSigners.JwksFetch.DeniedHostnames)
		req.Empty(c.ExternalJwtSigners.JwksFetch.AllowedHostnames)
		req.Equal(DefaultJwksFetchTimeout, c.ExternalJwtSigners.JwksFetch.Timeout)
		req.Equal(DefaultJwksFetchMaxRedirects, c.ExternalJwtSigners.JwksFetch.MaxRedirects)
	})

	t.Run("an absent jwksFetch sub-section yields defaults", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		req.NoError(c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{},
		}))

		req.False(c.ExternalJwtSigners.JwksFetch.BlockPrivateAddresses)
		req.Equal(DefaultJwksFetchTimeout, c.ExternalJwtSigners.JwksFetch.Timeout)
		req.Equal(DefaultJwksFetchMaxRedirects, c.ExternalJwtSigners.JwksFetch.MaxRedirects)
	})

	t.Run("a fully specified section is parsed", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		req.NoError(c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"blockPrivateAddresses": true,
					"deniedIPs":             []any{"10.0.0.0/8", "203.0.113.5"},
					"allowedIPs":            []any{"192.168.10.0/24", "fd00:1234::1"},
					"timeout":               "12s",
					"maxRedirects":          2,
				},
			},
		}))

		jwksFetch := c.ExternalJwtSigners.JwksFetch

		req.True(jwksFetch.BlockPrivateAddresses)
		req.Equal(12*time.Second, jwksFetch.Timeout)
		req.Equal(2, jwksFetch.MaxRedirects)

		req.Len(jwksFetch.DeniedIPs, 2)
		req.Equal("10.0.0.0/8", jwksFetch.DeniedIPs[0].String())
		req.Equal("203.0.113.5/32", jwksFetch.DeniedIPs[1].String(), "a bare IPv4 address should be treated as a /32")

		req.Len(jwksFetch.AllowedIPs, 2)
		req.Equal("192.168.10.0/24", jwksFetch.AllowedIPs[0].String())
		req.Equal("fd00:1234::1/128", jwksFetch.AllowedIPs[1].String(), "a bare IPv6 address should be treated as a /128")
	})

	t.Run("host lists are parsed and normalized", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		req.NoError(c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"deniedHostnames":  []any{"Blocked.Example.COM", "internal.example.com."},
					"allowedHostnames": []any{"idp.example.com", "*.idp.example.org"},
				},
			},
		}))

		jwksFetch := c.ExternalJwtSigners.JwksFetch

		req.Equal([]string{"blocked.example.com", "internal.example.com"}, jwksFetch.DeniedHostnames,
			"host entries should be lower-cased with any trailing dot removed")
		req.Equal([]string{"idp.example.com", "*.idp.example.org"}, jwksFetch.AllowedHostnames)
	})

	t.Run("a host list entry that is an IP address is an error", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		err := c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"deniedHostnames": []any{"10.0.0.5"},
				},
			},
		})

		req.Error(err, "an IP address in a host list would be a name comparison, not an address check")
		req.Contains(err.Error(), "deniedIPs")
	})

	t.Run("a host list entry with a scheme, port or path is an error", func(t *testing.T) {
		entries := []string{"https://idp.example.com", "idp.example.com:443", "idp.example.com/jwks", "idp.*.example.com", "*.", ""}

		for _, entry := range entries {
			t.Run(entry, func(t *testing.T) {
				req := require.New(t)

				c := NewEdgeConfig()

				err := c.loadExternalJwtSignersSection(map[any]any{
					"externalJwtSigners": map[any]any{
						"jwksFetch": map[any]any{
							"allowedHostnames": []any{entry},
						},
					},
				})

				req.Error(err)
				req.Contains(err.Error(), "allowedHostnames")
			})
		}
	})

	t.Run("blockPrivateAddresses accepts a string boolean", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		req.NoError(c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"blockPrivateAddresses": "true",
				},
			},
		}))

		req.True(c.ExternalJwtSigners.JwksFetch.BlockPrivateAddresses)
	})

	t.Run("maxRedirects of zero is allowed and disables redirects", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		req.NoError(c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"maxRedirects": 0,
				},
			},
		}))

		req.Equal(0, c.ExternalJwtSigners.JwksFetch.MaxRedirects)
	})

	t.Run("an invalid CIDR is an error", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		err := c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"deniedIPs": []any{"not-an-address"},
				},
			},
		})

		req.Error(err)
		req.Contains(err.Error(), "deniedIPs")
	})

	t.Run("a hostname entry is an error", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		err := c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"allowedIPs": []any{"idp.example.com"},
				},
			},
		})

		req.Error(err, "hostnames are not valid entries, they cannot be enforced at dial time")
		req.Contains(err.Error(), "allowedIPs")
	})

	t.Run("a non-list address value is an error", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		err := c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"deniedIPs": "10.0.0.0/8",
				},
			},
		})

		req.Error(err)
	})

	t.Run("an invalid duration is an error", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		err := c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"timeout": "not-a-duration",
				},
			},
		})

		req.Error(err)
	})

	t.Run("a non-positive timeout is an error", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		err := c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"timeout": "0s",
				},
			},
		})

		req.Error(err, "an unbounded fetch is exactly what this section exists to prevent")
	})

	t.Run("a negative maxRedirects is an error", func(t *testing.T) {
		req := require.New(t)

		c := NewEdgeConfig()

		err := c.loadExternalJwtSignersSection(map[any]any{
			"externalJwtSigners": map[any]any{
				"jwksFetch": map[any]any{
					"maxRedirects": -1,
				},
			},
		})

		req.Error(err)
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
