package create

import (
	"github.com/openziti/ziti/ziti/constants"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecuteCreateConfigRouterFabricHasNonBlankTemplateValues(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()
	routerName := "MyFabricRouter"

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	_, data := createRouterConfig([]string{"fabric", "--routerName", routerName}, routerOptions, nil)

	expectedNonEmptyStringFields := []string{".Router.Listener.BindPort", ".ZitiHome", ".Hostname", ".Router.Name", ".Router.IdentityCert", ".Router.IdentityServerCert", ".Router.IdentityKey", ".Router.IdentityCA", ".Router.Edge.Port"}
	expectedNonEmptyStringValues := []*string{&data.Router.Edge.ListenerBindPort, &data.ZitiHome, &data.Hostname, &data.Router.Name, &data.Router.IdentityCert, &data.Router.IdentityServerCert, &data.Router.IdentityKey, &data.Router.IdentityCA, &data.Router.Edge.Port}
	expectedNonEmptyIntFields := []string{".Router.Listener.OutQueueSize", ".Router.Wss.ReadBufferSize", ".Router.Wss.WriteBufferSize", ".Router.Forwarder.XgressDialQueueLength", ".Router.Forwarder.XgressDialWorkerCount", ".Router.Forwarder.LinkDialQueueLength", ".Router.Forwarder.LinkDialWorkerCount"}
	expectedNonEmptyIntValues := []*int{&data.Router.Listener.OutQueueSize, &data.Router.Wss.ReadBufferSize, &data.Router.Wss.WriteBufferSize, &data.Router.Forwarder.XgressDialQueueLength, &data.Router.Forwarder.XgressDialWorkerCount, &data.Router.Forwarder.LinkDialQueueLength, &data.Router.Forwarder.LinkDialWorkerCount}
	expectedNonEmptyTimeFields := []string{".Router.Listener.ConnectTimeout", "Router.Listener.GetSessionTimeout", ".Router.Wss.WriteTimeout", ".Router.Wss.ReadTimeout", ".Router.Wss.IdleTimeout", ".Router.Wss.PongTimeout", ".Router.Wss.PingInterval", ".Router.Wss.HandshakeTimeout"}
	expectedNonEmptyTimeValues := []*time.Duration{&data.Router.Listener.ConnectTimeout, &data.Router.Listener.GetSessionTimeout, &data.Router.Wss.WriteTimeout, &data.Router.Wss.ReadTimeout, &data.Router.Wss.IdleTimeout, &data.Router.Wss.PongTimeout, &data.Router.Wss.PingInterval, &data.Router.Wss.HandshakeTimeout}

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
	routerOptions := clearEnvAndInitializeTestData()
	routerName := "MyFabricRouter"
	externalIP := "123.456.78.9"

	// Set the env variable to non-empty value
	_ = os.Setenv(constants.ZitiEdgeRouterIPOverrideVarName, externalIP)

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	config, data := createRouterConfig([]string{"fabric", "--routerName", routerName}, routerOptions, nil)

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
	routerOptions := clearEnvAndInitializeTestData()

	// Create and run the CLI command
	config, _ := createRouterConfig([]string{"fabric", "--routerName", "myRouter"}, routerOptions, nil)

	// Expect that the config values are represented correctly
	assert.Equal(t, 0, len(config.Listeners), "Expected zero listeners for fabric router, found a non-zero value")
}

func TestBlankFabricRouterNameBecomesHostname(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()
	hostname, _ := os.Hostname()
	blank := ""

	// Create the options with empty router name
	clearEnvAndInitializeTestData()
	routerOptions.Output = defaultOutput
	routerOptions.RouterName = blank

	// Create and run the CLI command
	_, data := createRouterConfig([]string{"fabric", "--routerName", blank}, routerOptions, nil)

	// Check that the blank name was replaced with hostname in the template values
	assert.Equal(t, hostname, data.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

}

func TestFabricRouterOutputPathDoesNotExist(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()

	// Set the router options
	clearEnvAndInitializeTestData()
	routerOptions.RouterName = "MyFabricRouter"
	routerOptions.Output = "/IDoNotExist/MyFabricRouter.yaml"

	err := routerOptions.runFabricRouter(&ConfigTemplateValues{})

	assert.Error(t, err)
	assert.Equal(t, errors.Unwrap(err), syscall.ENOENT)
}

func TestDefaultZitiFabricRouterListenerBindPort(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()
	expectedDefaultPortStr := strconv.Itoa(testDefaultRouterListenerPort)

	// Make sure the related env vars are unset
	_ = os.Unsetenv("ZITI_ROUTER_LISTENER_BIND_PORT")

	// Create and run the CLI command
	config, data := createRouterConfig([]string{"fabric", "--routerName", "testRouter"}, routerOptions, nil)

	// Check that the template data has been updated as expected
	assert.Equal(t, expectedDefaultPortStr, data.Router.Edge.ListenerBindPort)

	// Check that the actual config output has the correct port
	for i := 1; i < len(config.Link.Listeners); i++ {
		if config.Link.Listeners[i].Binding == "transport" {
			// Assert Bind and Advertise use Bind port value
			assert.Equal(t, expectedDefaultPortStr, strings.Split(config.Link.Listeners[i].Bind, ":")[1])
			assert.Equal(t, expectedDefaultPortStr, strings.Split(config.Link.Listeners[i].Address, ":")[1])
			break
		}
	}
}

func TestSetZitiFabricRouterListenerBindPort(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()
	myPortValue := "1234"

	// Set the port manually
	_ = os.Setenv("ZITI_ROUTER_LISTENER_BIND_PORT", myPortValue)

	// Create and run the CLI command
	config, data := createRouterConfig([]string{"fabric", "--routerName", "testRouter"}, routerOptions, nil)

	assert.Equal(t, myPortValue, data.Router.Edge.ListenerBindPort)

	// Check that the actual config output has the correct port
	for i := 1; i < len(config.Link.Listeners); i++ {
		if config.Link.Listeners[i].Binding == "transport" {
			// Assert Bind and Advertise use Bind port value
			assert.Equal(t, myPortValue, strings.Split(config.Link.Listeners[i].Bind, ":")[1])
			assert.Equal(t, myPortValue, strings.Split(config.Link.Listeners[i].Address, ":")[1])
			break
		}
	}
}
