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

package controller

import (
	"crypto/tls"
	gotls "crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	transporttls "github.com/openziti/transport/v2/tls"
	"github.com/pkg/errors"
)

const (
	ZitiCtrlAddressHeader = "ziti-ctrl-address"
)

// UnderlayBindPoint represents the interface:port address of where a http.Server should listen for a ServerConfig and the public
// address that should be used to address it.
type UnderlayBindPoint struct {
	InterfaceAddress string //<interface>:<port>
	Address          string //<ip/host>:<port>
	NewAddress       string //<ip/host>:<port> sent out as a header for clients to alternatively swap to (ip -> hostname moves)
}

func (u *UnderlayBindPoint) BeforeHandler(next http.Handler) http.Handler {
	wrappedHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if u.NewAddress != "" {
			address := "https://" + u.NewAddress
			writer.Header().Set(ZitiCtrlAddressHeader, address)
		}

		next.ServeHTTP(writer, request)
	})

	return wrappedHandler
}
func (u *UnderlayBindPoint) AfterHandler(prev http.Handler) http.Handler {
	return prev
}
func (u *UnderlayBindPoint) Listener(serverName string, tlsConfig *gotls.Config) (net.Listener, error) {
	ln, err := transporttls.ListenTLS(u.InterfaceAddress, serverName, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("error listening: %s", err)
	}
	return ln, nil
}
func (u *UnderlayBindPoint) ServerAddress() string {
	return u.Address
}
func (u *UnderlayBindPoint) Configure(config []interface{}) error {
	for _, v := range config {
		if m, ok := v.(map[interface{}]interface{}); ok {
			if v, ok := m["interface"].(string); ok {
				u.InterfaceAddress = v
			}
			if v, ok := m["address"].(string); ok {
				u.Address = v
			}
			if v, ok := m["newAddress"].(string); ok {
				u.NewAddress = v
			}
		}
	}
	return nil
}

// IdentityConfig represents the BindPointConfig when an identity is supplied as opposed to an address
type OverlayBindPoint struct {
	Identity       []byte //an openziti identity
	Service        string //name of the service to bind
	ClientAuthType tls.ClientAuthType
	ServeTLS       bool
	Name           string
}

func (o *OverlayBindPoint) BeforeHandler(next http.Handler) http.Handler {
	return next
}
func (o *OverlayBindPoint) AfterHandler(prev http.Handler) http.Handler {
	return prev
}
func (o *OverlayBindPoint) Configure(config []interface{}) error {
	for _, v := range config {
		m, ok := v.(map[interface{}]interface{})
		if !ok {
			continue
		}
		identVal, ok := m["identity"]
		if !ok {
			continue
		}
		identCfg, ok := identVal.(map[interface{}]interface{})
		if !ok {
			return errors.New("identity config must be a map")
		}

		if fileVal, ok := identCfg["file"].(string); ok {
			data, err := os.ReadFile(fileVal)
			if err != nil {
				return err
			}
			o.Identity = data
		}

		if envName, ok := identCfg["env"].(string); ok {
			b64Id := os.Getenv(envName)
			idReader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(b64Id))
			data, err := io.ReadAll(idReader)
			if err != nil {
				return err
			}
			o.Identity = data
		}

		if len(o.Identity) < 1 {
			return errors.New("no identity configured: file or env required")
		}

		if svc, ok := identCfg["service"].(string); ok {
			o.Service = svc
		} else {
			return errors.New("service must be supplied when using an identity binding")
		}

		if certRequired, ok := identCfg["tlsClientAuthenticationPolicy"].(string); ok {
			switch strings.ToLower(certRequired) {
			case "noclientcert":
				o.ClientAuthType = tls.NoClientCert
			case "requestclientcert":
				o.ClientAuthType = tls.RequestClientCert
			case "requireanyclientcert":
				o.ClientAuthType = tls.RequireAnyClientCert
			case "verifyclientcertifgiven":
				o.ClientAuthType = tls.VerifyClientCertIfGiven
			case "requireandverifyclientcert":
				o.ClientAuthType = tls.RequireAndVerifyClientCert
			default:
				o.ClientAuthType = tls.VerifyClientCertIfGiven
			}
		}

		if serveTLS, ok := identCfg["serveTLS"].(bool); ok {
			o.ServeTLS = serveTLS
		} else {
			o.ServeTLS = true
		}
	}
	return nil
}

func (o *OverlayBindPoint) Listener(_ string, tlsConfig *gotls.Config) (net.Listener, error) {
	var listener net.Listener
	cfg := ziti.Config{}
	err := json.Unmarshal(o.Identity, &cfg)
	if err != nil {
		return nil, err
	}

	ctx, err := ziti.NewContext(&cfg)
	if err != nil {
		return nil, err
	}

	listener, err = ctx.Listen(o.Service)
	if err != nil {
		return nil, fmt.Errorf("error listening on overlay: %s", err)
	}

	if o.ServeTLS {
		listener = gotls.NewListener(listener, tlsConfig)
		if tlsConfig.ClientAuth < gotls.VerifyClientCertIfGiven {
			pfxlog.Logger().WithError(err).Warnf("The configured certificate verification method [%d] will not support mutual TLS", tlsConfig.ClientAuth)
		}
	} else {
		pfxlog.Logger().Warn("API not configured for TLS - use with caution")
	}
	return listener, nil
}
func (o *OverlayBindPoint) ServerAddress() string {
	return o.Name
}
func (o *OverlayBindPoint) Validate() error {
	return nil
}

// Validate this configuration object.
func (u *UnderlayBindPoint) Validate() error {
	// required
	if err := validateHostPort(u.InterfaceAddress); err != nil {
		return fmt.Errorf("invalid interface address [%s]: %v", u.InterfaceAddress, err)
	}

	// required
	if err := validateHostPort(u.Address); err != nil {
		return fmt.Errorf("invalid advertise address [%s]: %v", u.Address, err)
	}

	//optional
	if u.NewAddress != "" {
		if err := validateHostPort(u.NewAddress); err != nil {
			return fmt.Errorf("invalid new address [%s]: %v", u.NewAddress, err)
		}
	}

	return nil
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

// wrapSetCtrlAddressHeader will check to see if the bindPoint is configured to advertise a "new address". If so
// the value is added to the ZitiCtrlAddressHeader which will be sent out on every response. Clients can check this
// header to be notified that the controller is or will be moving from one ip/hostname to another. When the
// new address value is set, both the old and new addresses should be valid as the clients will begin using the
// new address on their next connect.
func (u *UnderlayBindPoint) wrapSetCtrlAddressHeader(handler http.Handler) http.Handler {
	wrappedHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if u.NewAddress != "" {
			address := "https://" + u.NewAddress
			writer.Header().Set(ZitiCtrlAddressHeader, address)
		}

		handler.ServeHTTP(writer, request)
	})

	return wrappedHandler
}
