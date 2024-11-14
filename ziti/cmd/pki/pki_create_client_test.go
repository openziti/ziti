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
	"github.com/sirupsen/logrus"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientCertNoSpiffeIdFromIntermediate(t *testing.T) {
	out, errOut := streams()
	svr := NewCmdPKICreateClient(out, errOut)
	name := uuid.New().String()
	args := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", intCaNameWithoutSpiffeIdName),
		fmt.Sprintf("--client-name=%s", name),
		fmt.Sprintf("--client-file=%s", name),
	}

	svr.SetArgs(args)
	svrErr := svr.Execute()
	if svrErr != nil {
		logrus.Fatal(svrErr)
	}

	bundle, e := testPki.GetBundle(intCaNameWithoutSpiffeIdName, name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Nil(t, bundle.Cert.URIs)
}

func TestClientCertSpiffeIdFromIntermediate(t *testing.T) {
	out, errOut := streams()
	svr := NewCmdPKICreateClient(out, errOut)
	name := uuid.New().String()
	args := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", intCaNameWithSpiffeIdName),
		fmt.Sprintf("--client-name=%s", name),
		fmt.Sprintf("--client-file=%s", name),
	}

	svr.SetArgs(addSpiffeArg("/some/path", args))
	svrErr := svr.Execute()
	if svrErr != nil {
		logrus.Fatal(svrErr)
	}

	bundle, e := testPki.GetBundle(intCaNameWithSpiffeIdName, name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)
	urls := URLSlice(bundle.Cert.URIs)
	assert.Contains(t, urls.Hosts(), rootCaWithSpiffeIdName)
	assert.Contains(t, urls.Paths(), "/some/path")
}

func TestClientCertNoSpiffeIdFromIntermediateAddSpiffeId(t *testing.T) {
	out, errOut := streams()
	svr := NewCmdPKICreateClient(out, errOut)
	name := uuid.New().String()
	args := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", intCaNameWithoutSpiffeIdName),
		fmt.Sprintf("--client-name=%s", name),
		fmt.Sprintf("--client-file=%s", name),
	}

	sid := "spiffe://not-from-ca/the-path"
	svr.SetArgs(addSpiffeArg(sid, args))
	svrErr := svr.Execute()
	if svrErr != nil {
		logrus.Fatal(svrErr)
	}

	bundle, e := testPki.GetBundle(intCaNameWithoutSpiffeIdName, name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), sid)
}

func TestClientCertSpiffeIdFromIntermediateAddSpiffeId(t *testing.T) {
	out, errOut := streams()
	svr := NewCmdPKICreateClient(out, errOut)
	name := uuid.New().String()
	args := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", intCaNameWithSpiffeIdName),
		fmt.Sprintf("--client-name=%s", name),
		fmt.Sprintf("--client-file=%s", name),
	}

	sid := "spiffe://from-ca/the-path"
	svr.SetArgs(addSpiffeArg(sid, args))
	svrErr := svr.Execute()
	if svrErr != nil {
		logrus.Fatal(svrErr)
	}

	bundle, e := testPki.GetBundle(intCaNameWithSpiffeIdName, name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), sid)
}
