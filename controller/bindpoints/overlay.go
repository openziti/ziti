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
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/xweb/v3"
)

var _ xweb.BindPoint = (*OverlayBindPoint)(nil)

// OverlayBindPoint represents the BindPointConfig when an identity is supplied as opposed to an address
type OverlayBindPoint struct {
	Identity       []byte //an openziti identity
	Service        string //name of the service to bind
	ClientAuthType tls.ClientAuthType
	Name           string
	Opts           ziti.ListenOptions
	cfg            ziti.Config
	ctx            ziti.Context
}

func (o OverlayBindPoint) BeforeHandler(next http.Handler) http.Handler {
	return next
}
func (o OverlayBindPoint) AfterHandler(prev http.Handler) http.Handler {
	return prev
}
func (o OverlayBindPoint) ServerAddress() string {
	return o.Name
}
func newOverlayBindPoint(conf map[interface{}]interface{}) (OverlayBindPoint, error) {
	o := OverlayBindPoint{}

	identVal, ok := conf["identity"]
	if !ok {
		return o, errors.New("missing identity section")
	}

	identCfg, ok := identVal.(map[interface{}]interface{})
	if !ok {
		return o, errors.New("identity config must be a map")
	}

	if fileVal, ok := identCfg["file"].(string); ok {
		data, err := os.ReadFile(fileVal)
		if err != nil {
			return o, err
		}
		o.Identity = data
	}

	if envName, ok := identCfg["env"].(string); ok {
		b64Id := os.Getenv(envName)
		idReader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(b64Id))
		data, err := io.ReadAll(idReader)
		if err != nil {
			return o, err
		}
		o.Identity = data
	}

	if len(o.Identity) < 1 {
		return o, errors.New("no identity configured: file or env required")
	}

	if svc, ok := identCfg["service"].(string); ok {
		o.Service = svc
	} else {
		return o, errors.New("service must be supplied when using an identity binding")
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
	if listenOptsCfg, ok := identCfg["listenOptions"]; ok {
		optsCfg := listenOptsCfg.(map[interface{}]interface{})
		if asId, ok := optsCfg["bindUsingEdgeIdentity"].(bool); ok {
			o.Opts.BindUsingEdgeIdentity = asId
		}
	}

	o.cfg = ziti.Config{}
	unMarshallErr := json.Unmarshal(o.Identity, &o.cfg)
	if unMarshallErr != nil {
		return o, unMarshallErr
	}

	if ctx, newCtxErr := ziti.NewContext(&o.cfg); newCtxErr != nil {
		return o, newCtxErr
	} else {
		o.ctx = ctx
	}

	return o, nil
}

func (o OverlayBindPoint) Listener(_ string, tlsConfig *tls.Config) (net.Listener, error) {
	var ln net.Listener

	ln, err := o.ctx.ListenWithOptions(o.Service, &o.Opts)
	if err != nil {
		return nil, fmt.Errorf("error listening on overlay: %s", err)
	}

	ln = tls.NewListener(ln, tlsConfig)
	if tlsConfig.ClientAuth < tls.RequestClientCert {
		pfxlog.Logger().WithError(err).Warnf("The configured certificate verification method [%d] will not support mutual TLS", tlsConfig.ClientAuth)
	}

	return ln, nil
}

func (o OverlayBindPoint) Validate(_ identity.Identity) error {
	return nil // much of the validation happens before this func is invoked in newOverlayBindPoint
}
