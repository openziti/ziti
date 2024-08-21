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
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestSdkEcho(t *testing.T) {
	components := run.GetModel().SelectComponents("#zcat")

	req := require.New(t)
	req.True(len(components) > 0)

	data := ""
	for i := 0; i < 1000; i++ {
		data += uuid.NewString()
	}

	for _, c := range components {
		remoteConfigFile := "/home/ubuntu/fablab/cfg/" + c.Id + ".json"

		ha := ""
		if len(run.GetModel().SelectComponents(".ctrl")) > 1 {
			ha = "--ha"
		}
		echoClientCmd := fmt.Sprintf(`echo "%s" | /home/%s/fablab/bin/ziti demo zcat %s --identity %s ziti:echo 2>&1`,
			data, c.GetHost().GetSshUser(), ha, remoteConfigFile)
		t.Logf("running: %s", echoClientCmd)
		output, err := c.GetHost().ExecLoggedWithTimeout(10*time.Second, echoClientCmd)
		t.Logf("test output:\n%s", output)
		req.NoError(err)
		//trim the newline ssh added
		output = strings.TrimRight(output, "\n")
		req.Equal(string(data), output)
	}
}
