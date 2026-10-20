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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/fablab/kernel/model"
	"github.com/stretchr/testify/require"
)

func TestScp(t *testing.T) {
	waitForTerminators(t, 30*time.Second, hostedServices("ssh", "ert", "zet", "ziti-tunnel")...)

	t.Run("scp-tests", func(t *testing.T) {
		t.Run("test-ert-scp", func(t *testing.T) {
			t.Parallel()
			for _, hostType := range []string{"ert", "zet", "ziti-tunnel"} {
				for _, encrypted := range []bool{true, false} {
					testScp(t, "ert", hostType, encrypted)
				}
			}
		})

		t.Run("test-zet-scp", func(t *testing.T) {
			t.Parallel()
			for _, hostType := range []string{"zet", "ziti-tunnel", "ert"} {
				for _, encrypted := range []bool{true, false} {
					testScp(t, "zet", hostType, encrypted)
				}
			}
		})

		t.Run("test-ziti-tunnel-scp", func(t *testing.T) {
			t.Parallel()
			for _, hostType := range []string{"ziti-tunnel", "ert", "zet"} {
				for _, encrypted := range []bool{true, false} {
					testScp(t, "ziti-tunnel", hostType, encrypted)
				}
			}
		})
	})
}

func testScp(t *testing.T, hostSelector string, hostType string, encrypted bool) {
	encDesk := "encrypted"
	if !encrypted {
		encDesk = "unencrypted"
	}

	nameExtra := ""
	if !encrypted {
		nameExtra = "-unencrypted"
	}

	downloadPath := "/tmp/ziti-" + uuid.NewString()
	uploadPath := "/tmp/ziti-" + uuid.NewString()

	tests := []struct {
		direction string
		cmd       string
		path      string
		onClient  bool
	}{
		{
			direction: "<-",
			cmd:       fmt.Sprintf("scp -o StrictHostKeyChecking=no scp://ssh-%s%s.ziti:2022/fablab/bin/ziti %s", hostType, nameExtra, downloadPath),
			path:      downloadPath,
			onClient:  true,
		}, {
			direction: "->",
			cmd:       fmt.Sprintf("scp -o StrictHostKeyChecking=no ./fablab/bin/ziti scp://ssh-%s%s.ziti:2022/%s", hostType, nameExtra, uploadPath),
			path:      uploadPath,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("(%s%s%s)-%v", hostSelector, test.direction, hostType, encDesk), func(t *testing.T) {
			if support := pathSupport(hostSelector, hostType); !support.Supported {
				t.Skip(support.Reason)
			}
			host, err := model.GetModel().SelectHost("." + hostSelector + "-client")
			req := require.New(t)
			req.NoError(err)
			t.Cleanup(func() { removeTransferred(t, host, hostType, test.path, test.onClient) })

			o, err := host.ExecLoggedWithTimeout(50*time.Second, test.cmd)
			t.Log(o)
			req.NoError(err)
		})
	}
}

// removeTransferred deletes path from the host a transfer wrote it on: the client's host for a
// download, or for an upload, the host serving hostType's ssh service. It logs rather than fails,
// since the copy is the test's leftover, not its result.
func removeTransferred(t *testing.T, client *model.Host, hostType, path string, onClient bool) {
	host := client
	if !onClient {
		c, err := model.GetModel().SelectComponent("." + hostType + "-host")
		if err != nil {
			t.Logf("unable to find the %s host to remove %s: %v", hostType, path, err)
			return
		}
		host = c.GetHost()
	}
	if _, err := host.ExecLogged("rm -f " + path); err != nil {
		t.Logf("unable to remove %s from %s: %v", path, host.GetId(), err)
	}
}
