package tls

import (
	"fmt"
	"github.com/openziti/identity"
	"net"
	"sort"
	"strings"
)

func ValidFor(i *identity.TokenId, address string) error {
	if strings.HasPrefix(address, "tls:") {
		address = address[len("tls:"):]
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid address: %s", address)
	}
	return validForHostname(i, host)
}

// ValidForHostname checks if the identity is valid for a given hostname
func validForHostname(i *identity.TokenId, hostname string) error {
	var err error

	// Check server certificate
	if len(i.ServerCert()) > 0 {
		err = i.ServerCert()[0].Leaf.VerifyHostname(hostname)
	}
	
	// Check client certificate if server cert validation fails
	if err != nil && i.Cert() != nil && i.Cert().Leaf != nil {
		err = i.Cert().Leaf.VerifyHostname(hostname)
	}

	if err != nil {
		return fmt.Errorf("identity is not valid for provided host: [%s]. is valid for: [%v]", hostname, getUniqueAddresses(i))
	}
	return nil
}

// getUniqueAddresses extracts unique DNS names and IP addresses from the identity's certificates
func getUniqueAddresses(i *identity.TokenId) string {
	addresses := make(map[string]struct{})

	if certs := i.ServerCert(); certs != nil && len(certs) > 0 && certs[0].Leaf != nil {
		for _, dns := range certs[0].Leaf.DNSNames {
			addresses[dns] = struct{}{}
		}
		for _, ip := range certs[0].Leaf.IPAddresses {
			addresses[ip.String()] = struct{}{}
		}
	}

	if cert := i.Cert(); cert != nil && cert.Leaf != nil {
		for _, dns := range cert.Leaf.DNSNames {
			addresses[dns] = struct{}{}
		}
		for _, ip := range cert.Leaf.IPAddresses {
			addresses[ip.String()] = struct{}{}
		}
	}

	uniqueList := make([]string, 0, len(addresses))
	for addr := range addresses {
		uniqueList = append(uniqueList, addr)
	}
	sort.Strings(uniqueList) // Ensure consistent order, mostly for testing

	return strings.Join(uniqueList, ", ")
}
