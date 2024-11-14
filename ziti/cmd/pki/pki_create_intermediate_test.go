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
	"github.com/google/uuid"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpiffedSetFromCa(t *testing.T) {
	out, errOut := streams()
	intermediateCmd := NewCmdPKICreateIntermediate(out, errOut)
	name := uuid.New().String()
	intermediateArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", rootCaWithSpiffeIdName),
		fmt.Sprintf("--intermediate-name=%s", name),
		fmt.Sprintf("--intermediate-file=%s", name),
		"--max-path-len=1",
	}

	intermediateCmd.SetArgs(intermediateArgs)
	pkiErr := intermediateCmd.Execute()
	if pkiErr != nil {
		t.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), "spiffe://"+rootCaWithSpiffeIdName)
}
