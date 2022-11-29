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

package cmd

import (
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestInstallZitiApp(t *testing.T) {
	t.Skip("skipping test for now")

	oldPath := os.Getenv("PATH")
	err := os.Setenv("PATH", "")
	assert.Nil(t, err)
	defer os.Setenv("PATH", oldPath)

	defer os.Unsetenv("HOME")
	err = os.Setenv("HOME", "/tmp/"+uuid.New())
	assert.Nil(t, err)
	err = (&CommonOptions{}).installZitiApp("main", c.ZITI_CONTROLLER, true, "0.0.0-0")

	assert.FileExists(t, os.Getenv("HOME")+"/bin/"+c.ZITI_CONTROLLER)
}
