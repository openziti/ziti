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

package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalChain_RewriteReplacesPreviousContent(t *testing.T) {
	req := require.New(t)

	root := t.TempDir()
	local := &Local{Root: root}
	req.NoError(InitCADir(filepath.Join(root, "ca")))

	caCertPath := filepath.Join(root, "ca", LocalCertsDir, "ca.cert")
	serverCertPath := filepath.Join(root, "ca", LocalCertsDir, "server.cert")
	chainPath := filepath.Join(root, "ca", LocalCertsDir, "server.chain.pem")

	req.NoError(os.WriteFile(caCertPath, []byte("ca\n"), 0644))
	req.NoError(os.WriteFile(serverCertPath, []byte("a much longer original server cert\n"), 0644))
	req.NoError(local.Chain("ca", "ca", "server"))

	// A renewed cert that encodes shorter than the one it replaces must not leave the tail of
	// the previous chain behind.
	req.NoError(os.WriteFile(serverCertPath, []byte("renewed\n"), 0644))
	req.NoError(local.Chain("ca", "ca", "server"))

	chain, err := os.ReadFile(chainPath)
	req.NoError(err)
	req.Equal("renewed\nca\n", string(chain))
}
