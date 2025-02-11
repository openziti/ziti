/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package oidc_auth

import (
	"crypto/x509"
	"fmt"
	"net"
	"strings"
)

type Issuer interface {
	// ValidFor parses and address (hostOrIp[:port]) and verifies it matches a given issuer's hostOrIp and port.
	// If port is unspecified the default TLS port is assumed (443)
	ValidFor(string) error

	// HostPort returns a string in the format of `host[:port]`
	HostPort() string
}

var _ Issuer = (*IssuerDns)(nil)

type IssuerDns struct {
	hostname string
	port     string
}

func (i IssuerDns) HostPort() string {
	if i.port == "" {
		return i.hostname
	}

	return net.JoinHostPort(i.hostname, i.port)
}

func (i IssuerDns) ValidFor(address string) error {
	host, port, err := getHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid host[:port]: %w", err)
	}

	cert := &x509.Certificate{
		DNSNames: []string{i.hostname},
	}

	if hostErr := cert.VerifyHostname(host); hostErr != nil {
		return fmt.Errorf("error verifying hostname: %w", hostErr)
	}

	if port != i.port {
		return fmt.Errorf("invalid port %q, expected %q", port, i.port)
	}

	return nil
}

var _ Issuer = (*IssuerIp)(nil)

type IssuerIp struct {
	ip   net.IP
	port string
}

func (i IssuerIp) HostPort() string {
	if i.port == "" {
		return i.ip.String()
	}

	return net.JoinHostPort(i.ip.String(), i.port)
}

func (i IssuerIp) ValidFor(address string) error {
	host, port, err := getHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid host[:port]: %w", err)
	}

	cert := &x509.Certificate{
		IPAddresses: []net.IP{i.ip},
	}

	if hostErr := cert.VerifyHostname(host); hostErr != nil {
		return fmt.Errorf("error verifying hostname: %w", hostErr)
	}

	if port != i.port {
		return fmt.Errorf("invalid port %q, expected %q", port, i.port)
	}

	return nil
}

func NewIssuer(address string) (Issuer, error) {
	host, port, err := getHostPort(address)

	if err != nil {
		return nil, err
	}

	ip := net.ParseIP(host)

	if ip != nil {
		return &IssuerIp{ip: ip, port: port}, nil
	} else {
		return &IssuerDns{hostname: host, port: port}, nil
	}
}

// getHostPort is similar to net.SplitHostPort but does not require a port
func getHostPort(address string) (string, string, error) {
	port := ""
	host := address
	if strings.Contains(address, ":") {
		var err error
		host, port, err = net.SplitHostPort(address)
		if err != nil {
			return "", "", err
		}
	}

	return host, port, nil
}
