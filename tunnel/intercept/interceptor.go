/*
	Copyright 2019 Netfoundry, Inc.

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

package intercept

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/tunnel/dns"
	"github.com/netfoundry/ziti-sdk-golang/ziti"
	"github.com/netfoundry/ziti-sdk-golang/ziti/edge"
	"net"
)

type Protocol int

const (
	TCP Protocol = 2
	UDP Protocol = 17
)

type Interceptor interface {
	Start(context ziti.Context)
	Stop()
	Intercept(service edge.Service, resolver dns.Resolver) error
	StopIntercepting(serviceName string, removeRoute bool) error
}

// Interceptors need to maintain state for services that are being intercepted.
// Intercepted services need to be looked up by:
// - intercepted address - this is all an interceptor knows when an inbound connection is made
// - service name - when a service is removed (e.g. from an appwan)

type InterceptedService struct {
	Name string
	Addr interceptAddress
	Data interface{} // interceptor-specific data attached to this intercept. intent is to provide context needed in Interceptor.StopIntercepting
}

type interceptAddress struct {
	cidr     string
	port     int
	protocol string
}

func (addr interceptAddress) Proto() string {
	return addr.protocol
}

func (addr interceptAddress) IpNet() net.IPNet {
	_, ipNet, err := net.ParseCIDR(addr.cidr)
	if err != nil {
		pfxlog.Logger().Errorf("net.ParseCIDR(%s) failed: %v", addr.cidr, err)
		// TODO return error? nil?
	}
	return *ipNet
}

func (addr interceptAddress) Port() int {
	return addr.port
}

// Return the length of a full prefix (no subnetting) for the given IP address.
// Returns 32 for ipv4 addresses, and 128 for ipv6 addresses.
func addrBits(ip net.IP) int {
	if ip == nil {
		return 0
	} else if ip.To4() != nil {
		return net.IPv4len * 8
	} else if ip.To16() != nil {
		return net.IPv6len * 8
	}

	pfxlog.Logger().Infof("invalid IP address %s", ip.String())
	return 0
}

func NewInterceptAddress(service edge.Service, protocol string, resolver dns.Resolver) (*interceptAddress, error) {
	ip, err := getInterceptIP(service.Dns.Hostname, resolver)
	if err != nil {
		return nil, fmt.Errorf("failed to get intercept IP address: %v", err)
	}

	prefixLen := addrBits(ip)
	ipNet := net.IPNet{IP: ip, Mask: net.CIDRMask(prefixLen, prefixLen)}
	addr := interceptAddress{cidr: ipNet.String(), port: service.Dns.Port, protocol: protocol}
	return &addr, nil
}

type LUT map[interceptAddress]InterceptedService

func NewLUT() LUT {
	return make(map[interceptAddress]InterceptedService)
}

// Return the service that is being intercepted at the given address, if any.
func (m LUT) GetByAddress(addr net.Addr) (*InterceptedService, error) {
	//var proto string
	var ip net.IP
	var port int
	var protocol string
	switch addrType := addr.(type) {
	case *net.TCPAddr:
		ip = addrType.IP
		port = addrType.Port
		protocol = "tcp"
	case *net.UDPAddr:
		ip = addrType.IP
		port = addrType.Port
		protocol = "udp"
	default:
		return nil, fmt.Errorf("unsupported address type: %v", addrType)
	}

	prefixLen := addrBits(ip)
	ipNet := &net.IPNet{IP: ip, Mask: net.CIDRMask(prefixLen, prefixLen)}
	cidr := ipNet.String()

	s, ok := m[interceptAddress{cidr, port, protocol}]
	if !ok {
		return nil, fmt.Errorf("address %s is not being intercepted", addr.String())
	}
	return &s, nil
}

func (m LUT) GetByName(serviceName string) (*InterceptedService, error) {
	for _, service := range m {
		if serviceName == service.Name {
			return &service, nil
		}
	}

	return nil, fmt.Errorf("service %s is not being intercepted", serviceName)
}

func (m LUT) Put(addr interceptAddress, serviceName string, data interface{}) error {
	m[addr] = InterceptedService{
		Name: serviceName,
		Addr: addr,
		Data: data,
	}
	return nil
}

func (m LUT) Remove(interceptAddr interceptAddress) {
	delete(m, interceptAddr)
}
