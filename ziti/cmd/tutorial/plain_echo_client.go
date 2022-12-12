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
	"fmt"
	"github.com/fatih/color"
	"io"
	"net/http"
	"net/url"
	"os"
)

type plainEchoClient struct {
	host string
	port uint16
}

func (self *plainEchoClient) echo(input string) error {
	input = url.QueryEscape(input)
	u := fmt.Sprintf("http://%v:%v?input=%v", self.host, self.port, input)
	resp, err := (&http.Client{}).Get(u)
	if err == nil {
		c := color.New(color.FgBlue, color.Bold)
		c.Print("\nplain-http-echo-client: ")
		_, err = io.Copy(os.Stdout, resp.Body)
	}
	return err
}
