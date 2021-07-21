/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/dns"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/pkg/errors"
	"net"
)

type Protocol int

const (
	TCP Protocol = 2
	UDP Protocol = 17
)

type Interceptor interface {
	Start(provider tunnel.FabricProvider)
	Stop()
	Intercept(service *entities.Service, resolver dns.Resolver, tracker AddressTracker) error
	StopIntercepting(serviceName string, tracker AddressTracker) error
}

// Interceptors need to maintain state for services that are being intercepted.
// Intercepted services need to be looked up by:
// - intercepted address - this is all an interceptor knows when an inbound connection is made
// - service name - when a service is removed (e.g. from an appwan)

type InterceptAddress struct {
	cidr       *net.IPNet
	lowPort    uint16
	highPort   uint16
	protocol   string
	TproxySpec []string
	AcceptSpec []string
}

func (addr *InterceptAddress) Proto() string {
	return addr.protocol
}

func (addr *InterceptAddress) IpNet() *net.IPNet {
	return addr.cidr
}

func (addr *InterceptAddress) LowPort() uint16 {
	return addr.lowPort
}

func (addr *InterceptAddress) HighPort() uint16 {
	return addr.highPort
}

func (addr *InterceptAddress) Contains(ip net.IP, port uint16) bool {
	return addr.cidr.Contains(ip) && port >= addr.lowPort && port <= addr.highPort
}

func (addr *InterceptAddress) String() string {
	return fmt.Sprintf("cidr: %v, cidrAddr: %p, lowPort: %v, highPort: %v, protocol: %v, tproxySpec: %v, acceptSpec: %v",
		addr.cidr, addr.cidr, addr.lowPort, addr.highPort, addr.protocol, addr.TproxySpec, addr.AcceptSpec)
}

func GetInterceptAddresses(service *entities.Service, protocol string, resolver dns.Resolver) ([]*InterceptAddress, error) {
	var result []*InterceptAddress
	for _, addr := range service.InterceptV1Config.Addresses {
		_, cidr, err := getInterceptIP(service, addr, resolver)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get intercept IP address for %v", addr)
		}

		for _, portRange := range service.InterceptV1Config.PortRanges {
			result = append(result, &InterceptAddress{
				cidr:     cidr,
				lowPort:  portRange.Low,
				highPort: portRange.High,
				protocol: protocol})
		}
	}
	return result, nil
}
