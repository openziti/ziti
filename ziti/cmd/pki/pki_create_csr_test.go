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
package pki

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/openziti/ziti/v2/ziti/pki/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateCSRHonorsFlags is a regression test for openziti/ziti#1185. The
// 'pki create csr' subcommand used to ignore its --csr-file and --key-name
// flags (it read viper keys that were never bound, then fell back to an
// interactive prompt). This test passes both flags non-interactively and
// asserts the command both consumes them and writes its output where the
// flags say it should.
func TestCreateCSRHonorsFlags(t *testing.T) {
	out, errOut := streams()
	csr := NewCmdPKICreateCSR(out, errOut)
	csrFile := uuid.New().String()

	// rootCaWithoutSpiffeIdName was created in setup() with --ca-file == --ca-name,
	// so its private key lives at <root>/<name>/keys/<name>.key -- exactly where
	// 'create csr' looks when given --key-name.
	args := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--key-name=%s", rootCaWithoutSpiffeIdName),
		fmt.Sprintf("--csr-file=%s", csrFile),
		"--csr-name=regression-1185",
	}

	csr.SetArgs(args)
	require.NoError(t, csr.Execute())

	// The CSR is stored under the key-name's CA dir, named after --csr-file.
	csrPath := filepath.Join(where, rootCaWithoutSpiffeIdName, store.LocalCertsDir, csrFile+".cert")
	_, statErr := os.Stat(csrPath)
	assert.NoError(t, statErr, "expected CSR written to %s", csrPath)
}

// TestCreateCSRRejectsBadKeyName ensures --key-name is actually consulted: a
// key-name that does not exist must fail rather than silently succeed against
// some other key.
func TestCreateCSRRejectsBadKeyName(t *testing.T) {
	out, errOut := streams()
	csr := NewCmdPKICreateCSR(out, errOut)

	args := []string{
		fmt.Sprintf("--pki-root=%s", where),
		"--key-name=does-not-exist",
		fmt.Sprintf("--csr-file=%s", uuid.New().String()),
		"--csr-name=regression-1185",
	}

	csr.SetArgs(args)
	assert.Error(t, csr.Execute())
}
