package cmd

import (
	"github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestBlankEdgeRouterNameBecomesHostname(t *testing.T) {
	hostname, _ := os.Hostname()

	templateValues := &ConfigTemplateValues{}

	// Create the options with empty router name
	options := &CreateConfigRouterEdgeOptions{}
	options.Output = defaultOutput
	options.RouterName = ""

	// Check that template values is a blank name
	assert.Equal(t, "", templateValues.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

	myCmd := NewCmdCreateConfigRouterEdge(templateValues)
	myCmd.PreRun(myCmd, options.Args)

	// Check that the blank name was replaced with hostname in the template values
	assert.Equal(t, hostname, templateValues.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

}

func TestBlankFabricRouterNameBecomesHostname(t *testing.T) {
	hostname, _ := os.Hostname()

	templateValues := &ConfigTemplateValues{}

	// Create the options with empty router name
	options := &CreateConfigRouterFabricOptions{}
	options.Output = defaultOutput
	options.RouterName = ""

	// Check that template values is a blank name
	assert.Equal(t, "", templateValues.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

	myCmd := NewCmdCreateConfigRouterFabric(templateValues)
	myCmd.PreRun(myCmd, options.Args)

	// Check that the blank name was replaced with hostname in the template values
	assert.Equal(t, hostname, templateValues.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

}

func TestEdgeRouterCannotBeWSSAndPrivate(t *testing.T) {
	expectedErrorMsg := "Flags for private and wss configs are mutually exclusive. You must choose private or wss, not both"

	// Create the options with both flags set to true
	options := &CreateConfigRouterEdgeOptions{}
	options.Output = defaultOutput
	options.IsPrivate = true
	options.WssEnabled = true

	err := options.run(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestEdgeRouterOutputPathDoesNotExist(t *testing.T) {
	expectedErrorMsg := "stat /IDoNotExist: no such file or directory"

	// Create the options with both flags set to true
	options := &CreateConfigRouterEdgeOptions{}
	options.RouterName = "MyEdgeRouter"
	options.Output = "/IDoNotExist/MyEdgeRouter.yaml"

	err := options.run(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestFabricRouterOutputPathDoesNotExist(t *testing.T) {
	expectedErrorMsg := "stat /IDoNotExist: no such file or directory"

	// Create the options with both flags set to true
	options := &CreateConfigRouterFabricOptions{}
	options.RouterName = "MyFabricRouter"
	options.Output = "/IDoNotExist/MyFabricRouter.yaml"

	err := options.run(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestSetZitiRouterIdentityCertDefault(t *testing.T) {
	// Ensure env variable is not set
	os.Setenv(constants.ZitiRouterIdentityCertVarName, "")

	routerName := "RouterTest"
	expectedDefault := workingDir + "/" + routerName + ".cert"
	rtv := &RouterTemplateValues{}
	SetZitiRouterIdentityCert(rtv, routerName)

	// Check that the default is used
	assert.Equal(t, expectedDefault, rtv.IdentityCert)
}

func TestSetZitiRouterIdentityCertCustom(t *testing.T) {
	expectedCustom := "My/Custom/Path/for/PKI/RouterTest.cert"
	// Set the env variable which is used to populate this value
	_ = os.Setenv(constants.ZitiRouterIdentityCertVarName, expectedCustom)

	rtv := &RouterTemplateValues{}
	SetZitiRouterIdentityCert(rtv, "Irrelevant")

	// Check that the custom value is used
	assert.Equal(t, expectedCustom, rtv.IdentityCert)
}

func TestSetZitiRouterIdentityServerCertDefault(t *testing.T) {
	// Ensure env variable is not set
	_ = os.Setenv(constants.ZitiRouterIdentityServerCertVarName, "")

	routerName := "RouterTest"
	expectedDefault := workingDir + "/" + routerName + ".server.chain.cert"
	rtv := &RouterTemplateValues{}
	SetZitiRouterIdentityServerCert(rtv, routerName)

	// Check that the default is used
	assert.Equal(t, expectedDefault, rtv.IdentityServerCert)
}

func TestSetZitiRouterIdentityServerCertCustom(t *testing.T) {
	expectedCustom := "My/Custom/Path/for/PKI/RouterTest.server.chain.cert"
	// Set the env variable which is used to populate this value
	_ = os.Setenv(constants.ZitiRouterIdentityServerCertVarName, expectedCustom)

	rtv := &RouterTemplateValues{}
	SetZitiRouterIdentityServerCert(rtv, "Irrelevant")

	// Check that the custom value is used
	assert.Equal(t, expectedCustom, rtv.IdentityServerCert)
}

func TestSetZitiRouterIdentityKeyCertDefault(t *testing.T) {
	// Ensure env variable is not set
	_ = os.Setenv(constants.ZitiRouterIdentityKeyVarDescription, "")

	routerName := "RouterTest"
	expectedDefault := workingDir + "/" + routerName + ".key"
	rtv := &RouterTemplateValues{}
	SetZitiRouterIdentityKey(rtv, routerName)

	// Check that the default is used
	assert.Equal(t, expectedDefault, rtv.IdentityKey)
}

func TestSetZitiRouterIdentityKeyCustom(t *testing.T) {
	expectedCustom := "My/Custom/Path/for/PKI/RouterTest.key"
	// Set the env variable which is used to populate this value
	_ = os.Setenv(constants.ZitiRouterIdentityKeyVarName, expectedCustom)

	rtv := &RouterTemplateValues{}
	SetZitiRouterIdentityKey(rtv, "Irrelevant")

	// Check that the custom value is used
	assert.Equal(t, expectedCustom, rtv.IdentityKey)
}

func TestSetZitiRouterIdentityKeyCADefault(t *testing.T) {
	// Ensure env variable is not set
	_ = os.Setenv(constants.ZitiRouterIdentityCAVarName, "")

	routerName := "RouterTest"
	expectedDefault := workingDir + "/" + routerName + ".cas"
	rtv := &RouterTemplateValues{}
	SetZitiRouterIdentityCA(rtv, routerName)

	// Check that the default is used
	assert.Equal(t, expectedDefault, rtv.IdentityCA)
}

func TestSetZitiRouterIdentityCACustom(t *testing.T) {
	expectedCustom := "My/Custom/Path/for/PKI/RouterTest.cas"
	// Set the env variable which is used to populate this value
	_ = os.Setenv(constants.ZitiRouterIdentityCAVarName, expectedCustom)

	rtv := &RouterTemplateValues{}
	SetZitiRouterIdentityCA(rtv, "Irrelevant")

	// Check that the custom value is used
	assert.Equal(t, expectedCustom, rtv.IdentityCA)
}
