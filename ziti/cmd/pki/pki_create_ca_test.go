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

func TestTrustDomain(t *testing.T) {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)
	name := uuid.New().String()
	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", name),
		fmt.Sprintf("--ca-name=%s", name),
		fmt.Sprintf("--trust-domain=%s", "spiffe://"+trustDomain),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), "spiffe://"+trustDomain)
}

func TestNoTrustDomain(t *testing.T) {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)
	name := uuid.New().String()
	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", name),
		fmt.Sprintf("--ca-name=%s", name),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Empty(t, bundle.Cert.URIs)
}

func TestTrustDomainSpiffeAppended(t *testing.T) {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)
	name := uuid.New().String()
	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", name),
		fmt.Sprintf("--ca-name=%s", name),
		fmt.Sprintf("--trust-domain=%s", trustDomain),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), "spiffe://"+trustDomain)
}

func TestTrustDomainWithPath(t *testing.T) {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)
	name := uuid.New().String()
	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", name),
		fmt.Sprintf("--ca-name=%s", name),
		fmt.Sprintf("--trust-domain=%s", "spiffe://"+trustDomain+"/path"),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), "spiffe://"+trustDomain)
}
