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
	"github.com/openziti/ziti/zititest/models/smoke"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDownloadFiles(t *testing.T) {
	zetHostsTested := 0
	zetClientsTest := 0

	allZetHostedFailed := true
	allZetClientsFailed := true

	checkZetHost := func(hostType string, success bool) {
		if hostType == "zet" {
			zetHostsTested++
			if success {
				allZetHostedFailed = false
			}
		}
	}

	t.Run("download-tests", func(t *testing.T) {
		t.Run("test-ert-downloads", func(t *testing.T) {
			t.Parallel()

			for _, size := range smoke.FileSizes {
				for _, hostType := range []string{"ert", "zet", "ziti-tunnel"} {
					for _, client := range smoke.HttpClients {
						for _, encrypted := range []bool{true, false} {
							success := testFileDownload(t, "ert", client, hostType, encrypted, size)
							checkZetHost(hostType, success)
						}
					}
				}
			}
		})

		t.Run("test-zet-downloads", func(t *testing.T) {
			t.Parallel()

			for _, size := range smoke.FileSizes {
				for _, hostType := range []string{"zet", "ziti-tunnel", "ert"} {
					for _, client := range smoke.HttpClients {
						for _, encrypted := range []bool{true, false} {
							success := testFileDownload(t, "zet", client, hostType, encrypted, size)
							checkZetHost(hostType, success)
							if success {
								allZetClientsFailed = false
							}
							zetClientsTest++
						}
					}
				}
			}
		})

		t.Run("test-ziti-tunnel-downloads", func(t *testing.T) {
			t.Parallel()

			for _, size := range smoke.FileSizes {
				for _, hostType := range []string{"ziti-tunnel", "ert", "zet"} {
					for _, client := range smoke.HttpClients {
						for _, encrypted := range []bool{true, false} {
							success := testFileDownload(t, "ziti-tunnel", client, hostType, encrypted, size)
							checkZetHost(hostType, success)
						}
					}
				}
			}
		})
	})

	req := require.New(t)
	if zetHostsTested > 0 {
		req.False(allZetHostedFailed, "all zet hosted file transfer should not failed, indicates bigger issue")
	}

	if zetClientsTest > 0 {
		req.False(allZetClientsFailed, "all zet client file transfers should not failed, indicates bigger issue")
	}
}

func testFileDownload(t *testing.T, hostSelector string, client smoke.HttpClient, hostType string, encrypted bool, fileSize string) bool {
	encDesk := "encrypted"
	if !encrypted {
		encDesk = "unencrypted"
	}

	success := false

	t.Run(fmt.Sprintf("%v-(%s<-%s)-%s-%v", client, hostSelector, hostType, fileSize, encDesk), func(t *testing.T) {
		o, err := smoke.TestFileDownload(hostSelector, client, hostType, encrypted, fileSize)
		t.Log(o)

		if hostType == "zet" && err != nil {
			t.Skipf("zet hosted file transfer failed [%v]", err.Error())
			return
		}

		if hostSelector == "zet" && err != nil {
			t.Skipf("zet client file transfer failed [%v]", err.Error())
			return
		}

		require.NoError(t, err)
		success = true
	})
	return success
}
