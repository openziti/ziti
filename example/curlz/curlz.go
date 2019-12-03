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

package main

import (
	"github.com/netfoundry/ziti-edge/sdk/ziti"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
)

type ZitiDialContext struct {
	context ziti.Context
}

func (dc *ZitiDialContext) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	service := strings.Split(addr, ":")[0] // will always get passed host:port
	return dc.context.Dial(service)
}

func newZitiClient() *http.Client {
	zitiDialContext := ZitiDialContext{context: ziti.NewContext()}
	zitiTransport := *http.DefaultTransport.(*http.Transport) // copy default transport
	zitiTransport.DialContext = zitiDialContext.Dial
	return &http.Client{Transport: &zitiTransport}
}

func main() {
	resp, err := newZitiClient().Get(os.Args[1])
	if err != nil {
		panic(err)
	}

	_, err = io.Copy(os.Stdout, resp.Body)
	if err != nil {
		panic(err)
	}
}
