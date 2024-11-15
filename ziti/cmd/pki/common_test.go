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
	"bytes"
	"fmt"
	"github.com/openziti/ziti/ziti/pki/pki"
	"github.com/openziti/ziti/ziti/pki/store"
	"github.com/sirupsen/logrus"
	"net"
	"net/url"
	"os"
	"testing"
)

var where = "/tmp/pki-test"
var trustDomain = "pki-test-domain"
var testPki *pki.ZitiPKI

var rootCaWithSpiffeIdName = "root-ca-with-spiffe-id"
var rootCaWithoutSpiffeIdName = "root-ca-without-spiffe-id"
var intCaNameWithSpiffeIdName = "intermediate-ca-with-spiffe-id"
var intCaNameWithoutSpiffeIdName = "intermediate-ca-without-spiffe-id"

func streams() (*bytes.Buffer, *bytes.Buffer) {
	return new(bytes.Buffer), new(bytes.Buffer)
}

type URLSlice []*url.URL

func (u URLSlice) Paths() []string {
	paths := make([]string, len(u))
	for i, uri := range u {
		paths[i] = uri.Path
	}
	return paths
}
func (u URLSlice) Hosts() []string {
	hosts := make([]string, len(u))
	for i, uri := range u {
		hosts[i] = uri.Host
	}
	return hosts
}

func urisAsStrings(uris []*url.URL) []string {
	urisAsStrings := make([]string, len(uris))
	for i, uri := range uris {
		urisAsStrings[i] = uri.String()
	}
	return urisAsStrings
}

func ipsAsStrings(ips []net.IP) []string {
	ipsAsStrings := make([]string, len(ips))
	for i, ip := range ips {
		ipsAsStrings[i] = ip.String()
	}
	return ipsAsStrings
}

func TestMain(m *testing.M) {
	var code int
	if setup() {
		// Run tests
		code = m.Run()
	}
	teardown()

	// Exit with the code from the test run
	os.Exit(code)
}

func setup() bool {
	where, _ = os.MkdirTemp("", "pki-test")
	testPki = &pki.ZitiPKI{Store: &store.Local{}}
	local := testPki.Store.(*store.Local)
	local.Root = where
	if !createTestCaWithSpiffeId() {
		return false
	}
	if !createTestCaWithoutSpiffeId() {
		return false
	}
	if !createTestIntermediateWithSpiffeId() {
		return false
	}
	if !createTestIntermediateWithoutSpiffeId() {
		return false
	}

	return true
}

func createTestCaWithSpiffeId() bool {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)

	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", rootCaWithSpiffeIdName),
		fmt.Sprintf("--ca-name=%s", rootCaWithSpiffeIdName),
		fmt.Sprintf("--trust-domain=%s", "spiffe://"+rootCaWithSpiffeIdName),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Error(pkiErr)
		return false
	}
	return true
}
func createTestCaWithoutSpiffeId() bool {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)

	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", rootCaWithoutSpiffeIdName),
		fmt.Sprintf("--ca-name=%s", rootCaWithoutSpiffeIdName),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Error(pkiErr)
		return false
	}
	return true
}
func createTestIntermediateWithSpiffeId() bool {
	out, errOut := streams()
	intermediateCmd := NewCmdPKICreateIntermediate(out, errOut)
	intermediateArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", rootCaWithSpiffeIdName),
		fmt.Sprintf("--intermediate-name=%s", intCaNameWithSpiffeIdName),
		fmt.Sprintf("--intermediate-file=%s", intCaNameWithSpiffeIdName),
		"--max-path-len=1",
	}

	intermediateCmd.SetArgs(intermediateArgs)
	pkiErr := intermediateCmd.Execute()
	if pkiErr != nil {
		logrus.Error(pkiErr)
		return false
	}
	return true
}
func createTestIntermediateWithoutSpiffeId() bool {
	out, errOut := streams()
	intermediateCmd := NewCmdPKICreateIntermediate(out, errOut)
	intermediateArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", rootCaWithoutSpiffeIdName),
		fmt.Sprintf("--intermediate-name=%s", intCaNameWithoutSpiffeIdName),
		fmt.Sprintf("--intermediate-file=%s", intCaNameWithoutSpiffeIdName),
		"--max-path-len=1",
	}

	intermediateCmd.SetArgs(intermediateArgs)
	pkiErr := intermediateCmd.Execute()
	if pkiErr != nil {
		logrus.Error(pkiErr)
		return false
	}
	return true
}

func teardown() {
	fmt.Printf("removing temp directory: %s\n", where)
	_ = os.RemoveAll(where)
}

func addSpiffeArg(id string, args []string) []string {
	args = append(args, "--spiffe-id="+id)
	return args
}
