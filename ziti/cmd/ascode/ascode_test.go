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
	"context"
	"crypto/tls"
	"fmt"
	"github.com/antchfx/jsonquery"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/ziti/cmd/ascode/exporter"
	"github.com/openziti/ziti/ziti/cmd/ascode/importer"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/stretchr/testify/assert"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var log = pfxlog.Logger()

func TestYamlUploadAndDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmdComplete := make(chan bool)
	qsCmd := edge.NewQuickStartCmd(os.Stdout, os.Stderr, ctx)

	qsCmd.SetArgs([]string{})

	go func() {
		err := qsCmd.Execute()
		if err != nil {
			log.Fatal(err)
		}
		cmdComplete <- true
	}()

	c := make(chan struct{})
	go waitForController("https://127.0.0.1:1280", c)

	select {
	case <-c:
		//completed normally
		log.Info("controller online")
	case <-time.After(30 * time.Second):
		cancel()
		panic("timed out waiting for controller")
	}

	performTest(t)

	cancel() //terminate the running ctrl/router

	<-cmdComplete
	fmt.Println("Operation completed")
}

func waitForController(ctrlUrl string, done chan struct{}) {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: tr}
	for {
		r, e := client.Get(ctrlUrl)
		if e != nil || r == nil || r.StatusCode != 200 {
			time.Sleep(50 * time.Millisecond)
		} else {
			break
		}
	}
	done <- struct{}{}

}

func performTest(t *testing.T) {
	errWriter := strings.Builder{}

	uploadWriter := strings.Builder{}
	importCmd := importer.NewImportCmd(&uploadWriter, &errWriter)
	importCmd.SetArgs([]string{"--yaml", "./test.yaml", "--yes", "--controller-url=localhost:1280", "--username=admin", "--password=admin"})

	err := importCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	// Create a temporary file in the default temporary directory
	tempFile, err := os.CreateTemp("", "ascode-output-*.json")
	if err != nil {
		t.Fatalf("error creating temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	log.Info("output file: ", tempFile.Name())

	exportCmd := exporter.NewExportCmd(os.Stdout, os.Stderr)
	exportCmd.SetArgs([]string{"all", "--yes", "--controller-url=localhost:1280", "--username=admin", "--password=admin", "--output-file=" + tempFile.Name()})
	err = exportCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Error reading file: %v", err)
	}
	sresult := string(result)
	log.Infof("export result: %s", sresult)

	doc, err := jsonquery.Parse(strings.NewReader(sresult))
	if err != nil {
		t.Fatalf("error parsing json: %v", err)
	}

	assert.NotEqual(t, 0, len(doc.ChildNodes()))
	if len(doc.ChildNodes()) == 0 {
		// no data, nothing to test
		return
	}

	externalJwtSigner1 := jsonquery.FindOne(doc, "//externalJwtSigners/*[name='NetFoundry Console Integration External JWT Signer']")
	assert.NotNil(t, externalJwtSigner1)
	assert.Equal(t, "https://gateway.staging.netfoundry.io/network-auth/v1/public/.well-known/NYFw7IGJKNP9AaG45iwCj/jwks.json", jsonquery.FindOne(externalJwtSigner1, "/jwksEndpoint").Value())
	assert.Equal(t, "https://gateway.staging.netfoundry.io/cloudziti/25ba1aa3-4468-445a-910e-93f5b425f2c1", jsonquery.FindOne(externalJwtSigner1, "/audience").Value())

	authPolicy1 := jsonquery.FindOne(doc, "//authPolicies/*[name='NetFoundry Console Integration Auth Policy']")
	assert.NotNil(t, authPolicy1)
	assert.Equal(t, "@NetFoundry Console Integration External JWT Signer", jsonquery.FindOne(authPolicy1, "/primary/extJwt/allowedSigners/*[1]").Value())
	authPolicy2 := jsonquery.FindOne(doc, "//authPolicies/*[name='Test123']")
	assert.NotNil(t, authPolicy2)
	assert.True(t, jsonquery.FindOne(authPolicy2, "/primary/updb/allowed").Value().(bool))
	authPolicy3 := jsonquery.FindOne(doc, "//authPolicies/*[name='ott+secondary']")
	assert.NotNil(t, authPolicy3)
	assert.True(t, jsonquery.FindOne(authPolicy3, "/primary/cert/allowed").Value().(bool))
	assert.False(t, jsonquery.FindOne(authPolicy3, "/primary/extJwt/allowed").Value().(bool))
	assert.False(t, jsonquery.FindOne(authPolicy3, "/primary/updb/allowed").Value().(bool))
	assert.Equal(t, "@Auth0a", jsonquery.FindOne(authPolicy3, "/secondary/requireExtJwtSigner").Value().(string))
	assert.False(t, jsonquery.FindOne(authPolicy3, "/secondary/requireTotp").Value().(bool))

	identity1 := jsonquery.FindOne(doc, "//identities/*[externalId='f1505b76-38ec-470b-9819-75984623c23d']")
	assert.NotNil(t, identity1)
	assert.Equal(t, "Vinay Lakshmaiah", jsonquery.FindOne(identity1, "/name").Value())
	assert.Empty(t, jsonquery.FindOne(identity1, "/roleAttributes").Value())
	assert.Equal(t, "@NetFoundry Console Integration Auth Policy", jsonquery.FindOne(identity1, "/authPolicy").Value())

	config1 := jsonquery.FindOne(doc, "//configs/*[name='service2-intercept-config']")
	assert.NotNil(t, config1)
	assert.Equal(t, "@intercept.v1", jsonquery.FindOne(config1, "/configType").Value())
	config2 := jsonquery.FindOne(doc, "//configs/*[name='ssssimple-intercept-config']")
	assert.NotNil(t, config2)
	assert.Equal(t, "@intercept.v1", jsonquery.FindOne(config2, "/configType").Value())
	config3 := jsonquery.FindOne(doc, "//configs/*[name='test-123-host-config']")
	assert.NotNil(t, config3)
	assert.Equal(t, "@host.v1", jsonquery.FindOne(config3, "/configType").Value())
	config4 := jsonquery.FindOne(doc, "//configs/*[name='service1-host-config']")
	assert.NotNil(t, config4)
	assert.Equal(t, "@host.v1", jsonquery.FindOne(config4, "/configType").Value())

	postureCheck1 := jsonquery.FindOne(doc, "//postureChecks/*[name='Mac']")
	assert.NotNil(t, postureCheck1)
	assert.Equal(t, "MAC", jsonquery.FindOne(postureCheck1, "/typeId").Value())
	assert.Equal(t, "0123456789ab", jsonquery.FindOne(postureCheck1, "/macAddresses/*[1]").Value())
	assert.Equal(t, "mac", jsonquery.FindOne(postureCheck1, "/roleAttributes/*[1]").Value())
	postureCheck2 := jsonquery.FindOne(doc, "//postureChecks/*[name='MFA']")
	assert.NotNil(t, postureCheck2)
	assert.Equal(t, "MFA", jsonquery.FindOne(postureCheck2, "/typeId").Value())
	assert.Equal(t, "mfa", jsonquery.FindOne(postureCheck2, "/roleAttributes/*[1]").Value())
	postureCheck3 := jsonquery.FindOne(doc, "//postureChecks/*[name='Process']")
	assert.NotNil(t, postureCheck3)
	assert.Equal(t, "PROCESS", jsonquery.FindOne(postureCheck3, "/typeId").Value())
	assert.Equal(t, "Linux", jsonquery.FindOne(postureCheck3, "/process/osType").Value())
	assert.Equal(t, "/path/something", jsonquery.FindOne(postureCheck3, "/process/path").Value())
	assert.Empty(t, jsonquery.Find(postureCheck3, "/process/hashes/*[1]"))
	assert.Equal(t, "process", jsonquery.FindOne(postureCheck3, "/roleAttributes/*[1]").Value())

	router1 := jsonquery.FindOne(doc, "//edgeRouters/*[name='custroutet2']")
	assert.NotNil(t, router1)
	assert.Equal(t, "vis-bind", jsonquery.FindOne(router1, "/roleAttributes/*[1]").Value())
	router2 := jsonquery.FindOne(doc, "//edgeRouters/*[name='asd']")
	assert.NotNil(t, router2)
	assert.Empty(t, jsonquery.Find(router2, "/roleAttributes/*[1]"))
	router3 := jsonquery.FindOne(doc, "//edgeRouters/*[name='public-router1']")
	assert.NotNil(t, router3)
	assert.Equal(t, "public", jsonquery.FindOne(router3, "/roleAttributes/*[1]").Value())
	router4 := jsonquery.FindOne(doc, "//edgeRouters/*[name='enroll']")
	assert.NotNil(t, router4)
	assert.Empty(t, jsonquery.FindOne(router4, "/roleAttributes").Value())
	router5 := jsonquery.FindOne(doc, "//edgeRouters/*[name='nfhosted']")
	assert.NotNil(t, router5)
	assert.Equal(t, "public", jsonquery.FindOne(router5, "/roleAttributes/*[1]").Value())
	router6 := jsonquery.FindOne(doc, "//edgeRouters/*[name='appdata']")
	assert.NotNil(t, router6)
	assert.Empty(t, jsonquery.FindOne(router6, "/roleAttributes").Value())
	assert.Equal(t, "er", jsonquery.FindOne(router6, "/appData/my").Value())
	router7 := jsonquery.FindOne(doc, "//edgeRouters/*[name='vis-customer-router']")
	assert.NotNil(t, router7)
	assert.Equal(t, "vis-bind", jsonquery.FindOne(router7, "/roleAttributes/*").Value())

	serviceRouterPolicy1 := jsonquery.FindOne(doc, "//serviceEdgeRouterPolicies/*[name='ssep2']")
	assert.NotNil(t, serviceRouterPolicy1)
	assert.Equal(t, "@custroutet2", jsonquery.FindOne(serviceRouterPolicy1, "/edgeRouterRoles/*[1]").Value())
	assert.Equal(t, "@ssssimple", jsonquery.FindOne(serviceRouterPolicy1, "/serviceRoles/*[1]").Value())
	serviceRouterPolicy2 := jsonquery.FindOne(doc, "//serviceEdgeRouterPolicies/*[name='sep1']")
	assert.NotNil(t, serviceRouterPolicy2)
	assert.Equal(t, "@public-router1", jsonquery.FindOne(serviceRouterPolicy2, "/edgeRouterRoles/*[1]").Value())
	assert.Equal(t, "@ssssimple", jsonquery.FindOne(serviceRouterPolicy2, "/serviceRoles/*[1]").Value())

	servicePolicy1 := jsonquery.FindOne(doc, "//servicePolicies/*[name='ssssimple-bind-policy']")
	assert.NotNil(t, servicePolicy1)
	assert.Equal(t, "@public-router1", jsonquery.FindOne(servicePolicy1, "/identityRoles/*[1]").Value())
	assert.Equal(t, "@ssssimple", jsonquery.FindOne(servicePolicy1, "/serviceRoles/*[1]").Value())
	assert.Equal(t, "Bind", jsonquery.FindOne(servicePolicy1, "/type").Value())
	servicePolicy2 := jsonquery.FindOne(doc, "//servicePolicies/*[name='ssssimple-dial-policy']")
	assert.NotNil(t, servicePolicy2)
	assert.Equal(t, "@identity12", jsonquery.FindOne(servicePolicy2, "/identityRoles/*[1]").Value())
	assert.Equal(t, "@ssssimple", jsonquery.FindOne(servicePolicy2, "/serviceRoles/*[1]").Value())
	assert.Equal(t, "Dial", jsonquery.FindOne(servicePolicy2, "/type").Value())

	routerPolicy1 := jsonquery.FindOne(doc, "//edgeRouterPolicies/*[name='routerpolicy1']")
	assert.NotNil(t, routerPolicy1)
	assert.Equal(t, "@public-router1", jsonquery.FindOne(routerPolicy1, "/edgeRouterRoles/*[1]").Value())
	assert.Equal(t, "@identity12", jsonquery.FindOne(routerPolicy1, "/identityRoles/*[1]").Value())
	routerPolicy2 := jsonquery.FindOne(doc, "//edgeRouterPolicies/*[name='edge-router-D98X8WmjYH-system']")
	assert.NotNil(t, routerPolicy2)
	assert.Equal(t, "@custroutet2", jsonquery.FindOne(routerPolicy2, "/edgeRouterRoles/*[1]").Value())
	assert.Equal(t, "@custroutet2", jsonquery.FindOne(routerPolicy2, "/identityRoles/*[1]").Value())

	service1 := jsonquery.FindOne(doc, "//services/*[name='ssssimple']")
	assert.NotNil(t, service1)
	assert.Equal(t, "abcd", jsonquery.FindOne(service1, "/roleAttributes/*[1]").Value())
	assert.Equal(t, "service", jsonquery.FindOne(service1, "/roleAttributes/*[2]").Value())
	configs1 := []string{}
	for _, node := range jsonquery.Find(service1, "/configs/*") {
		configs1 = append(configs1, node.Value().(string))
	}
	assert.Contains(t, configs1, "@ssssimple-intercept-config")
	assert.Contains(t, configs1, "@ssssimple-host-config")
	service2 := jsonquery.FindOne(doc, "//services/*[name='asdfasdf']")
	assert.NotNil(t, service2)
	assert.Equal(t, "bcde", jsonquery.FindOne(service2, "/roleAttributes/*[1]").Value())
	assert.Empty(t, jsonquery.Find(service2, "/configs/*"))

}
