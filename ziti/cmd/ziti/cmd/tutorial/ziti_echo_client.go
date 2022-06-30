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

package tutorial

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func NewZitiEchoClient(identityJson string) (*zitiEchoClient, error) {
	config, err := config.NewFromFile(identityJson)
	if err != nil {
		return nil, err
	}

	zitiContext := ziti.NewContextWithConfig(config)

	dial := func(_ context.Context, _ string, addr string) (net.Conn, error) {
		service := strings.Split(addr, ":")[0] // assume host is service
		return zitiContext.Dial(service)
	}

	zitiTransport := http.DefaultTransport.(*http.Transport).Clone()
	zitiTransport.DialContext = dial

	return &zitiEchoClient{
		httpClient: &http.Client{Transport: zitiTransport},
	}, nil
}

type zitiEchoClient struct {
	httpClient *http.Client
}

func (self *zitiEchoClient) echo(input string) error {
	u := fmt.Sprintf("http://echo?input=%v", url.QueryEscape(input))
	resp, err := self.httpClient.Get(u)
	if err == nil {
		c := color.New(color.FgGreen, color.Bold)
		c.Print("\nziti-http-echo-client: ")
		_, err = io.Copy(os.Stdout, resp.Body)
	}
	return err
}
