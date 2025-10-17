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
package util

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/ziti/constants"
)

func NewZitifiedTransportFromSlice(bytes []byte) (*http.Transport, error) {
	cfg := &ziti.Config{}
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		return nil, err
	}
	cfg.ConfigTypes = append(cfg.ConfigTypes, "all")

	zc, zce := ziti.NewContext(cfg)
	if zce != nil {
		return nil, fmt.Errorf("failed to create ziti context: %v", zce)
	}
	zitiCliContextCollection.Add(zc)
	zitiTransport := http.DefaultTransport.(*http.Transport).Clone() // copy default transport
	zitiTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := zitiCliContextCollection.NewDialerWithFallback(ctx, &net.Dialer{})
		return dialer.Dial(network, addr)
	}

	_, se := zc.GetServices() // loads all the services
	if se != nil {
		return nil, fmt.Errorf("failed to get ziti services: %v", se)
	}
	return zitiTransport, nil
}

func ZitifiedTransportFromEnv() (*http.Transport, error) {
	return ZitifiedTransportFromEnvByName(constants.ZitiCliNetworkIdVarName)
}

func ZitifiedTransportFromEnvByName(envVarName string) (*http.Transport, error) {
	b64Zid := os.Getenv(envVarName)
	if b64Zid == "" {
		return nil, nil
	}
	idReader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(b64Zid))
	data, err := io.ReadAll(idReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read and decode ziti identity: %v", err)
	}
	return NewZitifiedTransportFromSlice(data)
}
