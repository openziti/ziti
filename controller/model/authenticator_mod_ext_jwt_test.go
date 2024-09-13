package model

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/jwks"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/db"
	"github.com/stretchr/testify/require"
	"math/big"
	"net"
	"strconv"
	"testing"
	"time"
)

func Test_signerRecord_Resolve(t *testing.T) {
	t.Run("can resolve and parse a valid JWKS response", func(t *testing.T) {
		req := require.New(t)

		testRootCa := newRootCa()
		leaf1KeyPair := testRootCa.NewLeafWithAKID()
		leaf2KeyPair := testRootCa.NewLeafWithAKID()

		jwksEndpoint := "https://example.com/.well-known/jwks"

		jwksResolver, err := newTestJwksResolver()
		req.NoError(err)
		
		leaf1Key, err := newKey(leaf1KeyPair.cert, []*x509.Certificate{leaf1KeyPair.cert, testRootCa.cert})
		req.NoError(err)

		jwksResolver.AddKey(leaf1Key, leaf1KeyPair.key)

		leaf2Key, err := newKey(leaf2KeyPair.cert, []*x509.Certificate{leaf2KeyPair.cert, testRootCa.cert})
		req.NoError(err)

		jwksResolver.AddKey(leaf2Key, leaf2KeyPair.key)

		signerRec := &signerRecord{
			kidToPubKey: map[string]pubKey{},
			externalJwtSigner: &db.ExternalJwtSigner{
				BaseExtEntity: boltz.BaseExtEntity{
					Id:        "fake-id",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				Name:         "test1",
				JwksEndpoint: &jwksEndpoint,
				Enabled:      true,
			},
			jwksResolver: jwksResolver,
		}

		req.NoError(signerRec.Resolve(false))
		req.Equal(1, jwksResolver.callCount)
		req.Equal(jwksEndpoint, jwksResolver.callUrls[0])
		req.Len(signerRec.kidToPubKey, 2)

		t.Run("adding a new key, trigger resolve, increases call count and keys available", func(t *testing.T) {
			req := require.New(t)
			leaf3KeyPair := testRootCa.NewLeafWithAKID()

			leaf3Key, err := newKey(leaf3KeyPair.cert, []*x509.Certificate{leaf3KeyPair.cert, testRootCa.cert})
			req.NoError(err)

			jwksResolver.AddKey(leaf3Key, leaf3KeyPair.key)

			time.Sleep(JwksQueryTimeout)

			req.NoError(signerRec.Resolve(false))

			req.Equal(2, jwksResolver.callCount)
			req.Len(signerRec.kidToPubKey, 3)
		})

		t.Run("asking to resolve twice in succession w/o force does not trigger multiple calls", func(t *testing.T) {
			req := require.New(t)

			time.Sleep(JwksQueryTimeout)

			existingCallCount := jwksResolver.callCount

			req.NoError(signerRec.Resolve(false))
			req.NoError(signerRec.Resolve(false))

			req.Equal(existingCallCount+1, jwksResolver.callCount)
		})

		t.Run("asking to resolve twice in succession with force does trigger multiple calls", func(t *testing.T) {
			req := require.New(t)

			time.Sleep(JwksQueryTimeout)

			existingCallCount := jwksResolver.callCount

			req.NoError(signerRec.Resolve(true))
			req.NoError(signerRec.Resolve(true))

			req.Equal(existingCallCount+2, jwksResolver.callCount)
		})
	})
}

var currentSerial int64 = 1

type certPair struct {
	cert *x509.Certificate
	key  any
}

type testCa struct {
	certPair
}

func newRootCa() *testCa {
	currentSerial++

	rootKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		panic(err)
	}

	root := &x509.Certificate{
		SerialNumber: big.NewInt(currentSerial),
		Subject: pkix.Name{
			CommonName:    "root-" + strconv.FormatInt(currentSerial, 10),
			Organization:  []string{"FAKE, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Nowhere"},
			StreetAddress: []string{"Nowhere Road"},
			PostalCode:    []string{"55555"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		MaxPathLen:            -1,
	}

	rootBytes, err := x509.CreateCertificate(rand.Reader, root, root, &rootKey.PublicKey, rootKey)

	if err != nil {
		panic(err)
	}

	root, err = x509.ParseCertificate(rootBytes)

	if err != nil {
		panic(err)
	}

	return &testCa{
		certPair{
			cert: root,
			key:  rootKey,
		},
	}
}

func (ca *testCa) NewIntermediateWithAKID() *testCa {
	currentSerial++

	intermediate := &x509.Certificate{
		SerialNumber: big.NewInt(currentSerial),
		Subject: pkix.Name{
			CommonName:    "intermediate-" + strconv.FormatInt(currentSerial, 10),
			Organization:  []string{"FAKE, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Nowhere"},
			StreetAddress: []string{"Nowhere Road"},
			PostalCode:    []string{"55555"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		AuthorityKeyId:        ca.cert.SubjectKeyId,
		MaxPathLen:            5,
	}

	intermediateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		panic(err)
	}

	intermediateBytes, err := x509.CreateCertificate(rand.Reader, intermediate, ca.cert, &intermediateKey.PublicKey, ca.key)

	if err != nil {
		panic(err)
	}

	intermediate, err = x509.ParseCertificate(intermediateBytes)

	if err != nil {
		panic(err)
	}

	return &testCa{
		certPair{
			cert: intermediate,
			key:  intermediateKey,
		},
	}
}

func (ca *testCa) NewIntermediateWithoutAKID() *testCa {
	currentSerial++

	intermediate := &x509.Certificate{
		SerialNumber: big.NewInt(currentSerial),
		Subject: pkix.Name{
			CommonName:    "intermediate-" + strconv.FormatInt(currentSerial, 10),
			Organization:  []string{"FAKE, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Nowhere"},
			StreetAddress: []string{"Nowhere Road"},
			PostalCode:    []string{"55555"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		MaxPathLen:            5,
	}

	intermediateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		panic(err)
	}

	intermediateBytes, err := x509.CreateCertificate(rand.Reader, intermediate, ca.cert, &intermediateKey.PublicKey, ca.key)

	if err != nil {
		panic(err)
	}

	intermediate, err = x509.ParseCertificate(intermediateBytes)

	if err != nil {
		panic(err)
	}

	return &testCa{
		certPair{
			cert: intermediate,
			key:  intermediateKey,
		},
	}
}

func (ca *testCa) NewLeafWithAKID() *certPair {
	currentSerial++

	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			CommonName:    "leaf-" + strconv.FormatInt(currentSerial, 10),
			Organization:  []string{"FAKE, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Nowhere"},
			StreetAddress: []string{"Nowhere Road"},
			PostalCode:    []string{"55555"},
		},
		IPAddresses:    []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:      time.Now(),
		NotAfter:       time.Now().AddDate(10, 0, 0),
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:       x509.KeyUsageDigitalSignature,
		AuthorityKeyId: ca.cert.SubjectKeyId,
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		panic(err)
	}

	leafBytes, err := x509.CreateCertificate(rand.Reader, leaf, ca.cert, &leafKey.PublicKey, ca.key)

	if err != nil {
		panic(err)
	}

	leaf, err = x509.ParseCertificate(leafBytes)

	if err != nil {
		panic(err)
	}

	return &certPair{
		cert: leaf,
		key:  leafKey,
	}
}

func (ca *testCa) NewLeaf(leafKey *rsa.PrivateKey, alterCertFuncs ...func(certificate *x509.Certificate)) *certPair {
	currentSerial++

	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			CommonName:    "leaf-" + strconv.FormatInt(currentSerial, 10),
			Organization:  []string{"FAKE, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Nowhere"},
			StreetAddress: []string{"Nowhere Road"},
			PostalCode:    []string{"55555"},
		},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(10, 0, 0),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	for _, f := range alterCertFuncs {
		f(leaf)
	}

	leafBytes, err := x509.CreateCertificate(rand.Reader, leaf, ca.cert, &leafKey.PublicKey, ca.key)

	if err != nil {
		panic(err)
	}

	leaf, err = x509.ParseCertificate(leafBytes)

	if err != nil {
		panic(err)
	}

	return &certPair{
		cert: leaf,
		key:  leafKey,
	}
}
func (ca *testCa) NewLeafWithoutAKID() *certPair {
	currentSerial++

	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			CommonName:    "leaf-" + strconv.FormatInt(currentSerial, 10),
			Organization:  []string{"FAKE, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Nowhere"},
			StreetAddress: []string{"Nowhere Road"},
			PostalCode:    []string{"55555"},
		},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(10, 0, 0),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		panic(err)
	}

	leafBytes, err := x509.CreateCertificate(rand.Reader, leaf, ca.cert, &leafKey.PublicKey, ca.key)

	if err != nil {
		panic(err)
	}

	leaf, err = x509.ParseCertificate(leafBytes)

	if err != nil {
		panic(err)
	}

	return &certPair{
		cert: leaf,
		key:  leafKey,
	}
}

func newKey(cert *x509.Certificate, certChain []*x509.Certificate) (*jwks.Key, error) {
	kid := eid.New()
	key, err := jwks.NewKey(kid, cert, certChain)

	if err != nil {
		return nil, nil
	}

	return key, nil
}

type testJwksProvider struct {
	callCount int
	callUrls  []string

	response    *jwks.Response
	privateKeys map[string]any
}

func newTestJwksResolver() (*testJwksProvider, error) {
	result := &testJwksProvider{
		callUrls:    make([]string, 0),
		privateKeys: make(map[string]any),
		response: &jwks.Response{
			Keys: []jwks.Key{},
		},
	}

	return result, nil
}

func (s *testJwksProvider) Get(url string) (*jwks.Response, []byte, error) {
	s.callCount = s.callCount + 1
	s.callUrls = append(s.callUrls, url)

	responseBytes, err := json.Marshal(s.response)

	if err != nil {
		return nil, nil, fmt.Errorf("error marshalling jwks response: %v", err)
	}

	return s.response, responseBytes, nil
}

func (s *testJwksProvider) AddKey(key *jwks.Key, privateKey crypto.PrivateKey) {
	s.response.Keys = append(s.response.Keys, *key)
	s.privateKeys[key.KeyId] = privateKey
}

func (s *testJwksProvider) SignJwt(kid string, claims jwt.Claims) (string, *jwt.Token, error) {
	privateKey, ok := s.privateKeys[kid]

	if !ok {
		return "", nil, fmt.Errorf("could not find private key for kid %s", kid)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(privateKey)

	if err != nil {
		return "", nil, fmt.Errorf("could not sign token using private key from kid %s: %w", kid, err)
	}

	return signedToken, token, nil
}
