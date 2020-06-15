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

package gateway

import (
	"net"
	"net/url"
	"time"
)

type Memory struct {
	Path     string        `yaml:"path"`
	Interval time.Duration `yaml:"intervalMs"`
}

type Sans struct {
	DnsAddresses       []string `yaml:"dns" mapstructure:"dns"`
	IpAddresses        []string `yaml:"ip" mapstructure:"ip"`
	IpAddressesParsed  []net.IP
	EmailAddresses     []string `yaml:"email" mapstructure:"email"`
	UriAddresses       []string `yaml:"uri" mapstructure:"uri"`
	UriAddressesParsed []*url.URL
}
