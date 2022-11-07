package cmd

import (
	"github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

func TestExecuteCreateConfigRouterFabricHasNonBlankTemplateValues(t *testing.T) {
	routerName := "MyFabricRouter"
	expectedNonEmptyStringFields := []string{".ZitiHome", ".Hostname", ".Router.Name", ".Router.IdentityCert", ".Router.IdentityServerCert", ".Router.IdentityKey", ".Router.IdentityCA", ".Router.Edge.Hostname", ".Router.Edge.Port"}
	expectedNonEmptyStringValues := []*string{&data.ZitiHome, &data.Hostname, &data.Router.Name, &data.Router.IdentityCert, &data.Router.IdentityServerCert, &data.Router.IdentityKey, &data.Router.IdentityCA, &data.Router.Edge.Hostname, &data.Router.Edge.Port}
	expectedNonEmptyIntFields := []string{".Router.Listener.BindPort", ".Router.Listener.OutQueueSize", ".Router.Wss.ReadBufferSize", ".Router.Wss.WriteBufferSize", ".Router.Forwarder.XgressDialQueueLength", ".Router.Forwarder.XgressDialWorkerCount", ".Router.Forwarder.LinkDialQueueLength", ".Router.Forwarder.LinkDialWorkerCount"}
	expectedNonEmptyIntValues := []*int{&data.Router.Listener.BindPort, &data.Router.Listener.OutQueueSize, &data.Router.Wss.ReadBufferSize, &data.Router.Wss.WriteBufferSize, &data.Router.Forwarder.XgressDialQueueLength, &data.Router.Forwarder.XgressDialWorkerCount, &data.Router.Forwarder.LinkDialQueueLength, &data.Router.Forwarder.LinkDialWorkerCount}
	expectedNonEmptyTimeFields := []string{".Router.Listener.ConnectTimeout", "Router.Listener.GetSessionTimeout", ".Router.Wss.WriteTimeout", ".Router.Wss.ReadTimeout", ".Router.Wss.IdleTimeout", ".Router.Wss.PongTimeout", ".Router.Wss.PingInterval", ".Router.Wss.HandshakeTimeout", ".Router.Forwarder.LatencyProbeInterval"}
	expectedNonEmptyTimeValues := []*time.Duration{&data.Router.Listener.ConnectTimeout, &data.Router.Listener.GetSessionTimeout, &data.Router.Wss.WriteTimeout, &data.Router.Wss.ReadTimeout, &data.Router.Wss.IdleTimeout, &data.Router.Wss.PongTimeout, &data.Router.Wss.PingInterval, &data.Router.Wss.HandshakeTimeout, &data.Router.Forwarder.LatencyProbeInterval}

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	_ = createRouterConfig([]string{"fabric", "--routerName", routerName})

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

func TestFabricRouterIPOverrideIsConsumed(t *testing.T) {
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
	config := createRouterConfig([]string{"fabric", "--routerName", routerName})

	// Check that the template values now contains the custom external IP override value
	assert.Equal(t, externalIP, data.Router.Edge.IPOverride, "Mismatch router IP override, expected %s but got %s", externalIP, data.Router.Edge.IPOverride)

	// Check that the config output has the IP
	found := false
	for i := 1; i < len(config.Csr.Sans.Ip); i++ {
		if config.Csr.Sans.Ip[i] == externalIP {
			found = true
		}
	}
	assert.True(t, found, "Expected value not found; expected to find value of "+constants.ZitiEdgeRouterIPOverrideVarName+" in fabric router config output.")
}

func TestFabricRouterHasNoListeners(t *testing.T) {
	clearOptionsAndTemplateData()

	// Create and run the CLI command
	config := createRouterConfig([]string{"fabric", "--routerName", "myRouter"})

	// Expect that the config values are represented correctly
	assert.Equal(t, 0, len(config.Listeners), "Expected zero listeners for fabric router, found a non-zero value")
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

	// Create and run the CLI command
	_ = createRouterConfig([]string{"fabric", "--routerName", blank})

	// Check that the blank name was replaced with hostname in the template values
	assert.Equal(t, hostname, data.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

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
