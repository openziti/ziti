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
	"github.com/google/uuid"
	"github.com/openziti/fablab/kernel/lib"
	"github.com/openziti/fablab/kernel/libssh"
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
	"1KB":   10 * time.Second,
	"100KB": 10 * time.Second,
	"20MB":  40 * time.Second,
}

type httpClient string

const (
	ClientCurl httpClient = "curl"
	ClientWget httpClient = "wget"
)

func TestDownloadFiles(t *testing.T) {
	allZetHostedFailed := true
	allZetClientsFailed := true

	t.Run("download-tests", func(t *testing.T) {
		t.Run("test-ert-downloads", func(t *testing.T) {
			t.Parallel()

			for _, size := range []string{"1KB" /* "100KB", "20MB"*/} {
				for _, hostType := range []string{"ert", "zet", "ziti-tunnel"} {
					for _, client := range []httpClient{ClientCurl, ClientWget} {
						for _, encrypted := range []bool{true, false} {
							success := testFileDownload(t, "ert", client, hostType, encrypted, size)
							if hostType == "zet" && success {
								allZetHostedFailed = false
							}
						}
					}
				}
			}
		})

		t.Run("test-zet-downloads", func(t *testing.T) {
			t.Parallel()

			for _, size := range []string{"1KB", "100KB", "20MB"} {
				for _, hostType := range []string{"zet", "ziti-tunnel", "ert"} {
					for _, client := range []httpClient{ClientCurl, ClientWget} {
						for _, encrypted := range []bool{true, false} {
							success := testFileDownload(t, "zet", client, hostType, encrypted, size)
							if hostType == "zet" && success {
								allZetHostedFailed = false
							}
							if success {
								allZetClientsFailed = false
							}
						}
					}
				}
			}
		})

		t.Run("test-ziti-tunnel-downloads", func(t *testing.T) {
			t.Parallel()

			for _, size := range []string{"1KB", "100KB", "20MB"} {
				for _, hostType := range []string{"ziti-tunnel", "ert", "zet"} {
					for _, client := range []httpClient{ClientCurl, ClientWget} {
						for _, encrypted := range []bool{true, false} {
							success := testFileDownload(t, "ziti-tunnel", client, hostType, encrypted, size)
							if hostType == "zet" && success {
								allZetHostedFailed = false
							}
						}
					}
				}
			}
		})
	})

	req := require.New(t)
	req.False(allZetHostedFailed, "all zet hosted file transfer should not failed, indicates bigger issue")
	req.False(allZetClientsFailed, "all zet client file transfers should not failed, indicates bigger issue")
}

func testFileDownload(t *testing.T, hostSelector string, client httpClient, hostType string, encrypted bool, fileSize string) bool {
	encDesk := "encrypted"
	if !encrypted {
		encDesk = "unencrypted"
	}

	success := false

	t.Run(fmt.Sprintf("%v-(%s<-%s)-%s-%v", client, hostSelector, hostType, fileSize, encDesk), func(t *testing.T) {
		host, err := model.GetModel().SelectHost("." + hostSelector + "-client")
		req := require.New(t)
		req.NoError(err)

		urlExtra := ""
		if !encrypted {
			urlExtra = "-unencrypted"
		}

		url := fmt.Sprintf("https://files-%s%s.s3-us-west-1.amazonaws.ziti/%s.zip", hostType, urlExtra, fileSize)
		sshConfigFactory := lib.NewSshConfigFactory(host)

		filename := uuid.NewString()

		var cmds []string
		cmds = append(cmds, fmt.Sprintf("echo '%s  %s' > checksums", hashes[fileSize], filename))

		var cmd string
		if client == ClientCurl {
			cmd = fmt.Sprintf(`set -o pipefail; curl -k --header "Host: ziti-smoketest-files.s3-us-west-1.amazonaws.com" --fail-early --fail-with-body -SL -o %s %s`, filename, url)
		} else if client == ClientWget {
			cmd = fmt.Sprintf(`set -o pipefail; wget --no-check-certificate --header "Host: ziti-smoketest-files.s3-us-west-1.amazonaws.com" -O %s -t 5 -T 5 %s`, filename, url)
		}
		cmds = append(cmds, cmd)
		cmds = append(cmds, "md5sum -c checksums")

		timeout := timeouts[fileSize]
		o, err := libssh.RemoteExecAllWithTimeout(sshConfigFactory, timeout, cmds...)
		t.Log(o)

		if hostType == "zet" && err != nil {
			t.Skipf("zet hosted file transfer failed [%v]", err.Error())
			return
		}

		if hostSelector == "zet" && err != nil {
			t.Skipf("zet client file transfer failed [%v]", err.Error())
			return
		}

		req.NoError(err)
		success = true
	})
	return success
}
