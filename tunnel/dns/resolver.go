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

package dns

import "net"

// Resolver maps hostnames to intercept IPs for the tunneler's internal DNS.
type Resolver interface {
	AddHostname(string, net.IP) error
	// AddDomain registers a wildcard domain (e.g. *.ziti). The callback is
	// invoked to produce an IP for each previously unknown hostname matching
	// the domain, and must register the resulting hostname -> IP mapping via
	// AddHostname; the resolver does not register it implicitly.
	AddDomain(string, func(string) (net.IP, error)) error
	Lookup(net.IP) (string, error)
	LookupIP(string) (net.IP, bool)
	RemoveHostname(string) net.IP
	RemoveDomain(string)
	Cleanup() error
}

type domainEntry struct {
	name  string
	getIP func(string) (net.IP, error)
}
