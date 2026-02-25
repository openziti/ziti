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

	"github.com/stretchr/testify/require"
)

func TestSdkSim(t *testing.T) {
	waitForTerminators(t, 30*time.Second, "sim")

	components := run.GetModel().SelectComponents("#loop4-client")

	req := require.New(t)
	req.True(len(components) > 0)

	for _, c := range components {
		user := c.GetHost().GetSshUser()
		binaryPath := fmt.Sprintf("/home/%s/fablab/bin/ziti-traffic-test", user)
		configPath := fmt.Sprintf("/home/%s/fablab/cfg/%s.yml", user, c.Id)

		cmd := fmt.Sprintf("%s loop4 dialer %s 2>&1", binaryPath, configPath)
		t.Logf("running: %s", cmd)

		output, err := c.GetHost().ExecLoggedWithTimeout(2*time.Minute, cmd)
		t.Logf("output:\n%s", output)
		req.NoError(err, "loop4 dialer failed")
	}
}
