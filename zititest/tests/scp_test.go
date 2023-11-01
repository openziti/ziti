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

func TestScp(t *testing.T) {
	allZetHostedFailed := true
	allZetClientsFailed := true

	t.Run("scp-tests", func(t *testing.T) {
		t.Run("test-ert-scp", func(t *testing.T) {
			t.Parallel()
			for _, hostType := range []string{"ert", "zet", "ziti-tunnel"} {
				for _, encrypted := range []bool{true, false} {
					success := testScp(t, "ert", hostType, encrypted)
					if hostType == "zet" && success {
						allZetHostedFailed = false
					}
				}
			}
		})

		t.Run("test-zet-scp", func(t *testing.T) {
			t.Parallel()

			for _, hostType := range []string{"zet", "ziti-tunnel", "ert"} {
				for _, encrypted := range []bool{true, false} {
					success := testScp(t, "zet", hostType, encrypted)
					if hostType == "zet" && success {
						allZetHostedFailed = false
					}
					if success {
						allZetClientsFailed = false
					}
				}
			}
		})

		t.Run("test-ziti-tunnel-scp", func(t *testing.T) {
			t.Parallel()

			for _, hostType := range []string{"ziti-tunnel", "ert", "zet"} {
				for _, encrypted := range []bool{true, false} {
					success := testScp(t, "ziti-tunnel", hostType, encrypted)
					if hostType == "zet" && success {
						allZetHostedFailed = false
					}
				}
			}
		})
	})

	req := require.New(t)
	req.False(allZetHostedFailed, "all zet hosted file transfer should not failed, indicates bigger issue")
	req.False(allZetClientsFailed, "all zet client file transfers should not failed, indicates bigger issue")
}

func testScp(t *testing.T, hostSelector string, hostType string, encrypted bool) bool {
	encDesk := "encrypted"
	if !encrypted {
		encDesk = "unencrypted"
	}

	success := false

	nameExtra := ""
	if !encrypted {
		nameExtra = "-unencrypted"
	}

	tests := []struct {
		direction string
		cmd       string
	}{
		{
			direction: "<-",
			cmd:       fmt.Sprintf("scp -o StrictHostKeyChecking=no scp://ssh-%s%s.ziti:2022/fablab/bin/ziti /tmp/ziti-%s", hostType, nameExtra, uuid.NewString()),
		}, {
			direction: "->",
			cmd:       fmt.Sprintf("scp -o StrictHostKeyChecking=no ./fablab/bin/ziti scp://ssh-%s%s.ziti:2022//tmp/ziti-%s", hostType, nameExtra, uuid.NewString()),
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("(%s%s%s)-%v", hostSelector, test.direction, hostType, encDesk), func(t *testing.T) {
			host, err := model.GetModel().SelectHost("." + hostSelector + "-client")
			req := require.New(t)
			req.NoError(err)

			sshConfigFactory := lib.NewSshConfigFactory(host)

			o, err := libssh.RemoteExecAllWithTimeout(sshConfigFactory, 50*time.Second, test.cmd)
			if hostType == "zet" && err != nil {
				t.Skipf("zet hosted ssh failed [%v]", err.Error())
			} else if hostSelector == "zet" && err != nil {
				t.Skipf("zet client ssh failed [%v]", err.Error())
			} else {
				t.Log(o)
				req.NoError(err)
				success = true
			}
		})
	}

	return success
}
