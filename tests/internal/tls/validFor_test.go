package tls_test

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"

	"github.com/openziti/identity"
	ziti_tls "github.com/openziti/ziti/internal/tls"
)

// mockIdentity implements the Identity interface for testing
type mockIdentity struct {
	serverCerts []*tls.Certificate
	clientCert  *tls.Certificate
}

func (m *mockIdentity) Cert() *tls.Certificate         { return m.clientCert }
func (m *mockIdentity) ServerCert() []*tls.Certificate { return m.serverCerts }
func (m *mockIdentity) CA() *x509.CertPool             { return nil }
func (m *mockIdentity) CaPool() *identity.CaPool       { return nil }
func (m *mockIdentity) ServerTLSConfig() *tls.Config   { return nil }
func (m *mockIdentity) ClientTLSConfig() *tls.Config   { return nil }
func (m *mockIdentity) Reload() error                  { return nil }
func (m *mockIdentity) WatchFiles() error              { return nil }
func (m *mockIdentity) StopWatchingFiles()             {}
func (m *mockIdentity) SetCert(pem string) error       { return nil }
func (m *mockIdentity) SetServerCert(pem string) error { return nil }
func (m *mockIdentity) GetConfig() *identity.Config    { return nil }
func (m *mockIdentity) ValidFor(address string) bool   { return true }

const (
	validDNS   = "example.com"
	invalidDNS = "invalid.com"
	validIP4   = "192.168.1.1"
	invalidIP4 = "10.0.0.1"
	validIP6   = "::1"
	invalidIP6 = "fe80::1"
	validPort  = "443"
)

// Helper to create a mock identity with certs
func createmockIdentity(dnsNames []string, ipAddresses []string) *identity.TokenId {
	leaf := &x509.Certificate{}
	for _, dns := range dnsNames {
		leaf.DNSNames = append(leaf.DNSNames, dns)
	}
	for _, ip := range ipAddresses {
		leaf.IPAddresses = append(leaf.IPAddresses, net.ParseIP(ip))
	}

	tlsCert := &tls.Certificate{Leaf: leaf}
	mi := &mockIdentity{
		serverCerts: []*tls.Certificate{tlsCert},
		clientCert:  tlsCert,
	}
	id := &identity.TokenId{
		Identity: mi,
		Token:    "",
		Data:     nil,
	}
	return id
}

func TestValidFor_ValidHostname(t *testing.T) {
	id := createmockIdentity([]string{validDNS}, []string{})

	err := ziti_tls.ValidFor(id, validDNS+":"+validPort)
	if err != nil {
		t.Errorf("Expected valid hostname, got error: %v", err)
	}
}

func TestValidFor_InvalidHostname(t *testing.T) {
	id := createmockIdentity([]string{validDNS}, []string{})

	err := ziti_tls.ValidFor(id, invalidDNS+":"+validPort)
	if err == nil {
		t.Errorf("Expected error for invalid hostname, got nil")
	}
	assert.Equal(t, "identity is not valid for provided host: ["+invalidDNS+"]. is valid for: ["+validDNS+"]", err.Error())
}

func TestValidFor_ValidIPv4(t *testing.T) {
	id := createmockIdentity([]string{}, []string{validIP4})

	err := ziti_tls.ValidFor(id, validIP4+":"+validPort)
	if err != nil {
		t.Errorf("Expected valid IP, got error: %v", err)
	}
}

func TestValidFor_InvalidIPv4(t *testing.T) {
	id := createmockIdentity([]string{}, []string{validIP4})

	err := ziti_tls.ValidFor(id, invalidIP4+":"+validPort)
	if err == nil {
		t.Errorf("Expected error for invalid IP, got nil")
	}
	assert.Equal(t, "identity is not valid for provided host: ["+invalidIP4+"]. is valid for: ["+validIP4+"]", err.Error())
}

func TestValidFor_ValidIPv6(t *testing.T) {
	id := createmockIdentity([]string{}, []string{validIP6})

	err := ziti_tls.ValidFor(id, "["+validIP6+"]:"+validPort)
	if err != nil {
		t.Errorf("Expected valid IPv6, got error: %v", err)
	}
}

func TestValidFor_InvalidIPv6(t *testing.T) {
	id := createmockIdentity([]string{}, []string{validIP6})

	err := ziti_tls.ValidFor(id, "["+invalidIP6+"]:"+validPort)
	if err == nil {
		t.Errorf("Expected error for invalid IPv6, got nil")
	}
	assert.Equal(t, "identity is not valid for provided host: ["+invalidIP6+"]. is valid for: ["+validIP6+"]", err.Error())
}

func TestValidFor_ValidMixed(t *testing.T) {
	id := createmockIdentity([]string{validDNS}, []string{validIP4})

	err1 := ziti_tls.ValidFor(id, validDNS+":"+validPort)
	err2 := ziti_tls.ValidFor(id, validIP4+":"+validPort)

	if err1 != nil {
		t.Errorf("Expected valid hostname, got error: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Expected valid IP, got error: %v", err2)
	}
}

func TestValidFor_InvalidMixed(t *testing.T) {
	id := createmockIdentity([]string{validDNS}, []string{validIP4})

	err1 := ziti_tls.ValidFor(id, invalidDNS+":"+validPort)
	err2 := ziti_tls.ValidFor(id, invalidIP4+":"+validPort)

	if err1 == nil {
		t.Errorf("Expected error for invalid hostname, got nil")
	}
	assert.Equal(t, "identity is not valid for provided host: ["+invalidDNS+"]. is valid for: ["+validIP4+", "+validDNS+"]", err1.Error())
	if err2 == nil {
		t.Errorf("Expected error for invalid IP, got nil")
	}
	assert.Equal(t, "identity is not valid for provided host: ["+invalidIP4+"]. is valid for: ["+validIP4+", "+validDNS+"]", err2.Error())
}

func TestValidFor_NoCerts(t *testing.T) {
	id := createmockIdentity([]string{}, []string{})

	err := ziti_tls.ValidFor(id, validDNS+":"+validPort)
	if err == nil {
		t.Errorf("Expected error for no valid certs, got nil")
	}
	assert.Equal(t, "identity is not valid for provided host: ["+validDNS+"]. is valid for: []", err.Error())
}
