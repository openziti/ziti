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
	"github.com/netfoundry/ziti-edge/tunnel/utils"
	"net"
)

func getInterceptIP(hostname string, resolver dns.Resolver) (net.IP, error) {
	log := pfxlog.Logger()
	// hostname is an ip address, return it
	parsedIP := net.ParseIP(hostname)
	if parsedIP != nil {
		return parsedIP, nil
	}

	// - apparently not an IP address, assume hostname and attempt lookup:
	//   - use first result if any
	// - pin first IP to hostname in /etc/hosts
	addrs, err := net.LookupIP(hostname)
	if err == nil {
		if len(addrs) > 0 {
			resolver.AddHostname(hostname, addrs[0].To4())
			return addrs[0], nil
		}
	} else {
		log.Debugf("net.LookupIp(%s) failed: %s", hostname, err)
	}

	ip, err := utils.NextIP(net.IP{169, 254, 1, 1}, net.IP{169, 254, 254, 254})
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address or unresolvable hostname: %s", hostname)
	}
	resolver.AddHostname(hostname, ip)
	return ip, nil
}
