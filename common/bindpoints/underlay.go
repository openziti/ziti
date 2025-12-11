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

package bindpoints

import (
	gotls "crypto/tls"
	goerrs "errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/openziti/identity"
	transporttls "github.com/openziti/transport/v2/tls"
	"github.com/openziti/xweb/v3"
	"github.com/pkg/errors"
)

const (
	ZitiCtrlAddressHeader = "ziti-ctrl-address"
)

// UnderlayBindPoint represents the interface:port address of where a http.Server should listen for a ServerConfig and the public
// address that should be used to address it.
type UnderlayBindPoint struct {
	InterfaceAddress   string //<interface>:<port>
	Address            string //<ip/host>:<port>
	NewAddress         string //<ip/host>:<port> sent out as a header for clients to alternatively swap to (ip -> hostname moves)
	allowLegacyAddress bool
}

func (u UnderlayBindPoint) BeforeHandler(next http.Handler) http.Handler {
	return u.wrapSetCtrlAddressHeader(next)
}
func (u UnderlayBindPoint) AfterHandler(prev http.Handler) http.Handler {
	return prev
}
func (u UnderlayBindPoint) Listener(serverName string, tlsConfig *gotls.Config) (net.Listener, error) {
	ln, err := transporttls.ListenTLS(u.InterfaceAddress, serverName, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("error listening: %s", err)
	}

	return ln, nil
}

func (u UnderlayBindPoint) ServerAddress() string {
	return u.Address
}

func newUnderlayBindPoint(conf map[interface{}]interface{}, legacyValidation bool) (xweb.BindPoint, error) {
	u := UnderlayBindPoint{}
	u.allowLegacyAddress = legacyValidation
	if v, ok := conf["interface"].(string); ok {
		u.InterfaceAddress = v
	}
	if v, ok := conf["address"].(string); ok {
		u.Address = v
	}
	if v, ok := conf["newAddress"].(string); ok {
		u.NewAddress = v
	}
	return u, nil
}

// Validate this configuration object.
func (u UnderlayBindPoint) Validate(id identity.Identity) error {
	var errs []error

	// required
	if err := validateHostPort(u.InterfaceAddress); err != nil {
		errs = append(errs, fmt.Errorf("invalid interface address [%s]: %v", u.InterfaceAddress, err))
	}

	// required
	if err := validateHostPort(u.Address); err != nil {
		errs = append(errs, fmt.Errorf("invalid advertise address [%s]: %v", u.Address, err))
	}

	//optional
	if u.NewAddress != "" {
		if err := validateHostPort(u.NewAddress); err != nil {
			errs = append(errs, fmt.Errorf("invalid new address [%s]: %v", u.NewAddress, err))
		}
	}

	if h, _, err := net.SplitHostPort(u.Address); err == nil {
		if ve := id.ValidFor(normalizeIp(h)); ve != nil {
			if !u.allowLegacyAddress {
				errs = append(errs, fmt.Errorf("address not valid %s: %v", u.Address, ve))
			}
		}
	}

	return goerrs.Join(errs...)
}

func normalizeIp(s string) string {
	s = strings.Trim(s, "[]")
	var ip net.IP
	if i := strings.IndexByte(s, '%'); i != -1 {
		ip = net.ParseIP(s[:i])
	} else {
		ip = net.ParseIP(s)
	}
	if ip == nil {
		return s
	} else {
		return ip.String()
	}
}

// wrapSetCtrlAddressHeader will check to see if the bindPoint is configured to advertise a "new address". If so
// the value is added to the ZitiCtrlAddressHeader which will be sent out on every response. Clients can check this
// header to be notified that the controller is or will be moving from one ip/hostname to another. When the
// new address value is set, both the old and new addresses should be valid as the clients will begin using the
// new address on their next connect.
func (u UnderlayBindPoint) wrapSetCtrlAddressHeader(handler http.Handler) http.Handler {
	wrappedHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if u.NewAddress != "" {
			address := "https://" + u.NewAddress
			writer.Header().Set(ZitiCtrlAddressHeader, address)
		}

		handler.ServeHTTP(writer, request)
	})

	return wrappedHandler
}

func validateHostPort(address string) error {
	address = strings.TrimSpace(address)

	if address == "" {
		return errors.New("must not be an empty string or unspecified")
	}

	host, port, err := net.SplitHostPort(address)

	if err != nil {
		return errors.Errorf("could not split host and port: %v", err)
	}

	if host == "" {
		return errors.New("host must be specified")
	}

	if port == "" {
		return errors.New("port must be specified")
	}

	if port, err := strconv.ParseInt(port, 10, 32); err != nil {
		return errors.New("invalid port, must be a integer")
	} else if port < 1 || port > 65535 {
		return errors.New("invalid port, must 1-65535")
	}

	return nil
}
