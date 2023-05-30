/*
	(c) Copyright NetFoundry Inc.

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

package tests

import (
	"fmt"
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/model"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

var hashes = map[string]string{
	"1KB":   "0f343b0931126a20f133d67c2b018a3b",
	"100KB": "4c6426ac7ef186464ecbb0d81cbfcb1e",
	"20MB":  "8f4e33f3dc3e414ff94e5fb6905cba8c",
}

var timeouts = map[string]time.Duration{
	"1KB":   5 * time.Second,
	"100KB": 5 * time.Second,
	"20MB":  30 * time.Second,
}

type httpClient string

const (
	ClientCurl httpClient = "curl"
	ClientWget httpClient = "wget"
)

func TestCurlFiles(t *testing.T) {
	for _, clientType := range []string{"ert", "zet"} { // add zet back
		for _, hostType := range []string{"ert", "zet"} { // add zet back
			for _, client := range []httpClient{ClientCurl, ClientWget} {
				for _, encrypted := range []bool{true, false} {
					for _, size := range []string{"1KB", "100KB", "20MB"} {
						if clientType == hostType || encrypted {
							testFileDownload(t, clientType, client, hostType, encrypted, size)
						}
					}
				}
			}
		}
	}
}

func testFileDownload(t *testing.T, hostSelector string, client httpClient, hostType string, encrypted bool, fileSize string) {
	encDesk := "encrypted"
	if !encrypted {
		encDesk = "unencrypted"
	}

	t.Run(fmt.Sprintf("%v-(%s->%s)-%s-%v", client, hostSelector, hostType, fileSize, encDesk), func(t *testing.T) {
		host, err := model.GetModel().SelectHost("." + hostSelector + "-client")
		req := require.New(t)
		req.NoError(err)

		urlExtra := ""
		if !encrypted {
			urlExtra = "-unencrypted"
		}

		url := fmt.Sprintf("https://ziti-files-%s%s.s3-us-west-1.amazonaws.ziti/%s.zip", hostType, urlExtra, fileSize)
		sshConfigFactory := lib.NewSshConfigFactory(host)

		var cmd string
		if client == ClientCurl {
			cmd = fmt.Sprintf(`set -o pipefail; curl -k --header "Host: ziti-smoketest-files.s3-us-west-1.amazonaws.com" -fSL -o - %s | md5sum`, url)
		} else if client == ClientWget {
			cmd = fmt.Sprintf(`set -o pipefail; wget --no-check-certificate --header "Host: ziti-smoketest-files.s3-us-west-1.amazonaws.com" -O - -t 5 -T 5 %s | md5sum`, url)
		}

		timeout := timeouts[fileSize]
		o, err := lib.RemoteExecAllWithTimeout(sshConfigFactory, timeout, cmd)
		req.NoError(err)
		req.Equal(hashes[fileSize], o[0:32])
	})
}
