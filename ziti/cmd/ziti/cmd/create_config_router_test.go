package cmd

import (
	"github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"testing"
	"time"
)

func clearOptionsAndTemplateData() {
	routerOptions = CreateConfigRouterOptions{}
	data = &ConfigTemplateValues{}
}

func TestBlankEdgeRouterNameBecomesHostname(t *testing.T) {
	hostname, _ := os.Hostname()
	blank := ""

	// Setup options with blank router name
	clearOptionsAndTemplateData()
	routerOptions.Output = defaultOutput
	routerOptions.RouterName = blank

	// Check that template values is a blank name
	assert.Equal(t, blank, data.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigRouter()
	cmd.SetArgs([]string{"edge", "--routerName", blank})
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	// Check that the blank name was replaced with hostname in the template values
	assert.Equal(t, hostname, data.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

}

func TestBlankFabricRouterNameBecomesHostname(t *testing.T) {
	hostname, _ := os.Hostname()
	blank := ""

	// Create the options with empty router name
	clearOptionsAndTemplateData()
	routerOptions.Output = defaultOutput
	routerOptions.RouterName = blank

	// Check that template values is a blank name
	assert.Equal(t, blank, data.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigRouter()
	cmd.SetArgs([]string{"fabric", "--routerName", blank})
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	// Check that the blank name was replaced with hostname in the template values
	assert.Equal(t, hostname, data.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

}

func TestEdgeRouterCannotBeWSSAndPrivate(t *testing.T) {
	expectedErrorMsg := "Flags for private and wss configs are mutually exclusive. You must choose private or wss, not both"

	// Create the options with both flags set to true
	clearOptionsAndTemplateData()
	routerOptions.Output = defaultOutput
	routerOptions.IsPrivate = true
	routerOptions.WssEnabled = true

	err := routerOptions.runEdgeRouter(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestEdgeRouterOutputPathDoesNotExist(t *testing.T) {
	expectedErrorMsg := "stat /IDoNotExist: no such file or directory"

	// Set the router options
	clearOptionsAndTemplateData()
	routerOptions.RouterName = "MyEdgeRouter"
	routerOptions.Output = "/IDoNotExist/MyEdgeRouter.yaml"

	err := routerOptions.runEdgeRouter(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestFabricRouterOutputPathDoesNotExist(t *testing.T) {
	expectedErrorMsg := "stat /IDoNotExist: no such file or directory"

	// Set the router options
	clearOptionsAndTemplateData()
	routerOptions.RouterName = "MyFabricRouter"
	routerOptions.Output = "/IDoNotExist/MyFabricRouter.yaml"

	err := routerOptions.runFabricRouter(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestSetZitiRouterIdentityCertDefault(t *testing.T) {
	// Ensure env variable is not set
	_ = os.Setenv(constants.ZitiRouterIdentityCertVarName, "")

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

func TestSetZitiRouterIdentitySetsAllIdentitiesAndEdgeRouterRawName(t *testing.T) {
	// Setup
	expectedRawName := "MyEdgeRouterRawName"
	blank := ""
	rtv := &RouterTemplateValues{}

	// Check that they're all currently blank
	assert.Equal(t, blank, rtv.Edge.Hostname)
	assert.Equal(t, blank, rtv.IdentityCert)
	assert.Equal(t, blank, rtv.IdentityServerCert)
	assert.Equal(t, blank, rtv.IdentityKey)
	assert.Equal(t, blank, rtv.IdentityCA)

	// Set the env variable
	_ = os.Setenv(constants.ZitiEdgeRouterRawNameVarName, expectedRawName)

	SetZitiRouterIdentity(rtv, expectedRawName)

	// Check that the value matches
	assert.Equal(t, expectedRawName, rtv.Edge.Hostname)
	assert.NotEqualf(t, blank, rtv.IdentityCert, "Router.IdentityCert expected to have a value, instead it was blank")
	assert.NotEqualf(t, blank, rtv.IdentityServerCert, "Router.IdentityCert expected to have a value, instead it was blank")
	assert.NotEqualf(t, blank, rtv.IdentityKey, "Router.IdentityCert expected to have a value, instead it was blank")
	assert.NotEqualf(t, blank, rtv.IdentityCA, "Router.IdentityCert expected to have a value, instead it was blank")
}

func TestSetZitiRouterIdentitySetsAllIdentitiesAndEdgeRouterRawNameToHostWhenBlank(t *testing.T) {
	// Setup
	expectedRawName, _ := os.Hostname()
	blank := ""
	rtv := &RouterTemplateValues{}

	// Check that they're all currently blank
	assert.Equal(t, blank, rtv.Edge.Hostname)
	assert.Equal(t, blank, rtv.IdentityCert)
	assert.Equal(t, blank, rtv.IdentityServerCert)
	assert.Equal(t, blank, rtv.IdentityKey)
	assert.Equal(t, blank, rtv.IdentityCA)

	// Set the env variable to an empty value
	_ = os.Setenv(constants.ZitiEdgeRouterRawNameVarName, "")

	SetZitiRouterIdentity(rtv, expectedRawName)

	// Check that the value matches
	assert.Equal(t, expectedRawName, rtv.Edge.Hostname)
	assert.NotEqualf(t, blank, rtv.IdentityCert, "Router.IdentityCert expected to have a value, instead it was blank")
	assert.NotEqualf(t, blank, rtv.IdentityServerCert, "Router.IdentityCert expected to have a value, instead it was blank")
	assert.NotEqualf(t, blank, rtv.IdentityKey, "Router.IdentityCert expected to have a value, instead it was blank")
	assert.NotEqualf(t, blank, rtv.IdentityCA, "Router.IdentityCert expected to have a value, instead it was blank")
}

func TestExecuteCreateConfigRouterEdgeHasNonBlankTemplateValues(t *testing.T) {
	routerName := "MyEdgeRouter"
	expectedNonEmptyStringFields := []string{".ZitiHome", ".Hostname", ".Router.Name", ".Router.IdentityCert", ".Router.IdentityServerCert", ".Router.IdentityKey", ".Router.IdentityCA", ".Router.Edge.Hostname", ".Router.Edge.Port"}
	expectedNonEmptyStringValues := []*string{&data.ZitiHome, &data.Hostname, &data.Router.Name, &data.Router.IdentityCert, &data.Router.IdentityServerCert, &data.Router.IdentityKey, &data.Router.IdentityCA, &data.Router.Edge.Hostname, &data.Router.Edge.Port}
	expectedNonEmptyIntFields := []string{".Router.Listener.BindPort", ".Router.Listener.OutQueueSize", ".Router.Wss.ReadBufferSize", ".Router.Wss.WriteBufferSize", ".Router.Forwarder.XgressDialQueueLength", ".Router.Forwarder.XgressDialWorkerCount", ".Router.Forwarder.LinkDialQueueLength", ".Router.Forwarder.LinkDialWorkerCount"}
	expectedNonEmptyIntValues := []*int{&data.Router.Listener.BindPort, &data.Router.Listener.OutQueueSize, &data.Router.Wss.ReadBufferSize, &data.Router.Wss.WriteBufferSize, &data.Router.Forwarder.XgressDialQueueLength, &data.Router.Forwarder.XgressDialWorkerCount, &data.Router.Forwarder.LinkDialQueueLength, &data.Router.Forwarder.LinkDialWorkerCount}
	expectedNonEmptyTimeFields := []string{".Router.Listener.ConnectTimeout", "Router.Listener.GetSessionTimeout", ".Router.Wss.WriteTimeout", ".Router.Wss.ReadTimeout", ".Router.Wss.IdleTimeout", ".Router.Wss.PongTimeout", ".Router.Wss.PingInterval", ".Router.Wss.HandshakeTimeout", ".Router.Forwarder.LatencyProbeInterval"}
	expectedNonEmptyTimeValues := []*time.Duration{&data.Router.Listener.ConnectTimeout, &data.Router.Listener.GetSessionTimeout, &data.Router.Wss.WriteTimeout, &data.Router.Wss.ReadTimeout, &data.Router.Wss.IdleTimeout, &data.Router.Wss.PongTimeout, &data.Router.Wss.PingInterval, &data.Router.Wss.HandshakeTimeout, &data.Router.Forwarder.LatencyProbeInterval}

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigRouter()
	cmd.SetArgs([]string{"edge", "--routerName", routerName})
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	// Check that the expected string template values are not blank
	for field, value := range expectedNonEmptyStringValues {
		assert.NotEqualf(t, "", *value, expectedNonEmptyStringFields[field]+" should be a non-blank value")
	}

	// Check that the expected int template values are not zero
	for field, value := range expectedNonEmptyIntValues {
		assert.NotZero(t, *value, expectedNonEmptyIntFields[field]+" should be a non-zero value")
	}

	// Check that the expected time.Duration template values are not zero
	for field, value := range expectedNonEmptyTimeValues {
		assert.NotZero(t, *value, expectedNonEmptyTimeFields[field]+" should be a non-zero value")
	}
}

func TestExecuteCreateConfigRouterFabricHasNonBlankTemplateValues(t *testing.T) {
	routerName := "MyFabricRouter"
	expectedNonEmptyStringFields := []string{".ZitiHome", ".Hostname", ".Router.Name", ".Router.IdentityCert", ".Router.IdentityServerCert", ".Router.IdentityKey", ".Router.IdentityCA", ".Router.Edge.Hostname", ".Router.Edge.Port"}
	expectedNonEmptyStringValues := []*string{&data.ZitiHome, &data.Hostname, &data.Router.Name, &data.Router.IdentityCert, &data.Router.IdentityServerCert, &data.Router.IdentityKey, &data.Router.IdentityCA, &data.Router.Edge.Hostname, &data.Router.Edge.Port}
	expectedNonEmptyIntFields := []string{".Router.Listener.BindPort", ".Router.Listener.OutQueueSize", ".Router.Wss.ReadBufferSize", ".Router.Wss.WriteBufferSize", ".Router.Forwarder.XgressDialQueueLength", ".Router.Forwarder.XgressDialWorkerCount", ".Router.Forwarder.LinkDialQueueLength", ".Router.Forwarder.LinkDialWorkerCount"}
	expectedNonEmptyIntValues := []*int{&data.Router.Listener.BindPort, &data.Router.Listener.OutQueueSize, &data.Router.Wss.ReadBufferSize, &data.Router.Wss.WriteBufferSize, &data.Router.Forwarder.XgressDialQueueLength, &data.Router.Forwarder.XgressDialWorkerCount, &data.Router.Forwarder.LinkDialQueueLength, &data.Router.Forwarder.LinkDialWorkerCount}
	expectedNonEmptyTimeFields := []string{".Router.Listener.ConnectTimeout", "Router.Listener.GetSessionTimeout", ".Router.Wss.WriteTimeout", ".Router.Wss.ReadTimeout", ".Router.Wss.IdleTimeout", ".Router.Wss.PongTimeout", ".Router.Wss.PingInterval", ".Router.Wss.HandshakeTimeout", ".Router.Forwarder.LatencyProbeInterval"}
	expectedNonEmptyTimeValues := []*time.Duration{&data.Router.Listener.ConnectTimeout, &data.Router.Listener.GetSessionTimeout, &data.Router.Wss.WriteTimeout, &data.Router.Wss.ReadTimeout, &data.Router.Wss.IdleTimeout, &data.Router.Wss.PongTimeout, &data.Router.Wss.PingInterval, &data.Router.Wss.HandshakeTimeout, &data.Router.Forwarder.LatencyProbeInterval}

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigRouter()
	cmd.SetArgs([]string{"fabric", "--routerName", routerName})
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	// Check that the expected string template values are not blank
	for field, value := range expectedNonEmptyStringValues {
		assert.NotEqualf(t, "", *value, expectedNonEmptyStringFields[field]+" should be a non-blank value")
	}

	// Check that the expected int template values are not zero
	for field, value := range expectedNonEmptyIntValues {
		assert.NotZero(t, *value, expectedNonEmptyIntFields[field]+" should be a non-zero value")
	}

	// Check that the expected time.Duration template values are not zero
	for field, value := range expectedNonEmptyTimeValues {
		assert.NotZero(t, *value, expectedNonEmptyTimeFields[field]+" should be a non-zero value")
	}
}

func TestEdgeRouterIPOverrideIsConsumed(t *testing.T) {
	routerName := "MyFabricRouter"
	blank := ""
	externalIP := "123.456.78.9"

	// Setup options
	clearOptionsAndTemplateData()
	routerOptions.Output = defaultOutput

	// Set the env variable to non-empty value
	_ = os.Setenv(constants.ZitiEdgeRouterIPOverrideVarName, externalIP)

	// Check that template value is currently blank
	assert.Equal(t, blank, data.Router.Edge.IPOverride, "Mismatch router IP override, expected %s but got %s", blank, data.Router.Edge.IPOverride)

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigRouter()
	cmd.SetArgs([]string{"edge", "--routerName", routerName})
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	// Check that the template values now contains the custom external IP override value
	assert.Equal(t, externalIP, data.Router.Edge.IPOverride, "Mismatch router IP override, expected %s but got %s", externalIP, data.Router.Edge.IPOverride)

	// Check that the config output has the IP
	assert.True(t, strings.Contains(configOutput, externalIP), "Expected value not found; expected to find value of "+constants.ZitiEdgeRouterIPOverrideVarName+" in config output.")
}
