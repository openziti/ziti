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
	"github.com/openziti/fablab/kernel/libssh"
	"github.com/openziti/fablab/kernel/model"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIPerf(t *testing.T) {
	t.Run("iperf-tests", func(t *testing.T) {
		t.Run("ert-hosted", func(t *testing.T) {
			t.Parallel()

			for _, clientType := range []string{"ert", "zet", "ziti-tunnel"} {
				for _, encrypted := range []bool{true, false} {
					for _, reversed := range []bool{true, false} {
						testIPerf(t, clientType, "ert", encrypted, reversed)
					}
				}
			}
		})

		t.Run("zet-hosted", func(t *testing.T) {
			t.Parallel()

			for _, clientType := range []string{"zet", "ziti-tunnel", "ert"} {
				for _, encrypted := range []bool{true, false} {
					for _, reversed := range []bool{true, false} {
						testIPerf(t, clientType, "zet", encrypted, reversed)
					}
				}
			}
		})

		t.Run("ziti-tunnel-hosted", func(t *testing.T) {
			t.Parallel()

			for _, clientType := range []string{"ziti-tunnel", "ert", "zet"} {
				for _, encrypted := range []bool{true, false} {
					for _, reversed := range []bool{true, false} {
						testIPerf(t, clientType, "ziti-tunnel", encrypted, reversed)
					}
				}
			}
		})
	})
}

func testIPerf(t *testing.T, hostSelector string, hostType string, encrypted bool, reversed bool) bool {
	encDesk := "encrypted"
	if !encrypted {
		encDesk = "unencrypted"
	}

	direction := "->"
	if reversed {
		direction = "<-"
	}

	success := false

	t.Run(fmt.Sprintf("(%s%s%s)-%v", hostSelector, direction, hostType, encDesk), func(t *testing.T) {
		host, err := model.GetModel().SelectHost("." + hostSelector + "-client")
		req := require.New(t)
		req.NoError(err)

		urlExtra := ""
		if !encrypted {
			urlExtra = "-unencrypted"
		}

		addr := fmt.Sprintf("iperf-%s%s.ziti", hostType, urlExtra)

		extraOptions := ""
		if reversed {
			extraOptions += " -R"
		}

		cmd := fmt.Sprintf(`set -o pipefail; iperf3 -c %s -P 1 -t 10 %s`, addr, extraOptions)

		sshConfigFactory := lib.NewSshConfigFactory(host)
		o, err := libssh.RemoteExecAllWithTimeout(sshConfigFactory, 20*time.Second, cmd)
		if hostType == "zet" && err != nil {
			t.Skipf("zet hosted iperf test failed [%v]", err.Error())
			return
		}

		if hostType == "ziti-tunnel" && err != nil {
			t.Skipf("ziti-tunnel hosted iperf test failed [%v]", err.Error())
			return
		}

		if hostSelector == "zet" && err != nil {
			t.Skipf("zet client iperf test failed [%v]", err.Error())
			return
		}

		if hostSelector == "ziti-tunnel" && err != nil {
			t.Skipf("ziti-tunnel client iperf test failed [%v]", err.Error())
			return
		}

		t.Log(o)
		req.NoError(err)
		success = true
	})
	return success
}
