package create

import (
	"github.com/openziti/ziti/ziti/constants"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

var defaultArgs = []string{"edge", "--routerName", "test-router"}
var testHostname, _ = os.Hostname()

func TestEdgeRouterAdvertisedAddress(t *testing.T) {
	routerOpts := clearEnvAndInitializeTestData()
	routerAdvHostIp := "192.168.10.10"
	routerAdvHostDns := "controller01.zitinetwork.example.org"
	keys := map[string]string{
		"ZITI_CTRL_ADVERTISED_PORT": "80",
		"ZITI_ROUTER_PORT":          "443",
	}
	// Defaults to hostname if nothing is set
	_, data := createRouterConfig(defaultArgs, routerOpts, keys)
	require.Equal(t, testHostname, data.Router.Edge.AdvertisedHost, nil)

	// If IP override set, uses that value over hostname
	keys["ZITI_ROUTER_IP_OVERRIDE"] = routerAdvHostIp
	_, data2 := createRouterConfig(defaultArgs, routerOpts, keys)
	require.Equal(t, routerAdvHostIp, data2.Router.Edge.AdvertisedHost, nil)

	// If advertised address set, uses that over IP override or hostname
	keys["ZITI_ROUTER_ADVERTISED_ADDRESS"] = routerAdvHostDns
	keys["ZITI_ROUTER_IP_OVERRIDE"] = routerAdvHostIp
	_, data3 := createRouterConfig(defaultArgs, routerOpts, keys)
	require.Equal(t, routerAdvHostDns, data3.Router.Edge.AdvertisedHost, nil)
}

func TestTunnelerEnabledByDefault(t *testing.T) {
	// Setup options
	routerOptions := clearEnvAndInitializeTestData()

	// Create and run the CLI command without the tunnel flag
	config, _ := createRouterConfig([]string{"edge", "--routerName", "myRouter"}, routerOptions, nil)

	// Confirm tunneler is enabled in config output
	foundTunnel := false
	for i := 0; i < len(config.Listeners); i++ {
		if config.Listeners[i].Binding == "tunnel" {
			foundTunnel = true
		}
	}
	assert.True(t, foundTunnel, "Expected to find tunnel listener binding but it was not found")
}

func TestTunnelerNoneMode(t *testing.T) {
	// Setup options
	routerOptions := clearEnvAndInitializeTestData()

	// Create and run the CLI command with the disable tunnel flag
	config, data := createRouterConfig([]string{"edge", "--routerName", "myRouter", "--tunnelerMode", "none"}, routerOptions, nil)

	// Expect tunneler mode to be "none" mode
	assert.Equal(t, noneTunMode, data.Router.TunnelerMode, "Expected tunneler mode to be %s but found %s", noneTunMode, data.Router.TunnelerMode)

	// Confirm tunneler is disabled in config output
	foundTunnel := false
	for i := 0; i < len(config.Listeners); i++ {
		if config.Listeners[i].Binding == "tunnel" {
			foundTunnel = true
		}
	}
	assert.False(t, foundTunnel, "Tunnel listener binding was not expected but it was found")
}

func TestTunnelerHostModeIsDefault(t *testing.T) {
	// Setup options
	routerOptions := clearEnvAndInitializeTestData()

	// Create and run the CLI command without the tunnel flag
	config, data := createRouterConfig([]string{"edge", "--routerName", "myRouter"}, routerOptions, nil)

	// Expect tunneler mode to be "host" mode
	assert.Equal(t, hostTunMode, data.Router.TunnelerMode, "Expected tunneler mode to be %s but found %s", hostTunMode, data.Router.TunnelerMode)

	// Confirm tunneler mode in config is set to host mode
	for i := 0; i < len(config.Listeners); i++ {
		if config.Listeners[i].Binding == "tunnel" {
			assert.Equal(t, hostTunMode, config.Listeners[i].Options.Mode, "Expected tunneler mode to be %s but found %s in config output", hostTunMode, data.Router.TunnelerMode)
		}
	}
}

func TestTunnelerTproxyMode(t *testing.T) {
	// Setup options
	routerOptions := clearEnvAndInitializeTestData()

	// Create and run the CLI command without the tunnel flag
	config, data := createRouterConfig([]string{"edge", "--routerName", "myRouter", "--tunnelerMode", tproxyTunMode}, routerOptions, nil)

	// Expect tunneler mode to be "host" mode
	assert.Equal(t, tproxyTunMode, data.Router.TunnelerMode, "Expected tunneler mode to be %s but found %s", tproxyTunMode, data.Router.TunnelerMode)

	// Confirm tunneler mode in config is set to host mode
	for i := 0; i < len(config.Listeners); i++ {
		if config.Listeners[i].Binding == "tunnel" {
			assert.Equal(t, tproxyTunMode, config.Listeners[i].Options.Mode, "Expected tunneler mode to be %s but found %s in config output", tproxyTunMode, data.Router.TunnelerMode)
		}
	}
}

func TestTunnelerInvalidMode(t *testing.T) {
	invalidMode := "invalidMode"

	expectedErrorMsg := "Unknown tunneler mode [" + invalidMode + "] provided, should be \"" + noneTunMode + "\", \"" + hostTunMode + "\", or \"" + tproxyTunMode + "\""

	// Create the options with both flags set to true
	routerOptions := clearEnvAndInitializeTestData()
	routerOptions.TunnelerMode = invalidMode

	err := routerOptions.runEdgeRouter(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestPrivateEdgeRouterNotAdvertising(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()

	// Create and run the CLI command
	config, _ := createRouterConfig([]string{"edge", "--routerName", "myRouter", "--private"}, routerOptions, nil)

	// Expect that the config values are represented correctly
	assert.Equal(t, 0, len(config.Link.Listeners), "Expected zero link listeners for private edge router, found a non-zero value")
}

func TestBlankEdgeRouterNameBecomesHostname(t *testing.T) {
	hostname, _ := os.Hostname()
	blank := ""

	// Setup options with blank router name
	routerOptions := clearEnvAndInitializeTestData()
	routerOptions.RouterName = blank

	// Check that template values is a blank name
	//xx how does this work? assert.Equal(t, blank, data.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

	// Create and run the CLI command
	_, data := createRouterConfig([]string{"edge", "--routerName", blank}, routerOptions, nil)

	// Check that the blank name was replaced with hostname in the template values
	assert.Equal(t, hostname, data.Router.Name, "Mismatch router name, expected %s but got %s", "", hostname)

}

func TestDefaultZitiEdgeRouterListenerBindPort(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()
	expectedDefaultPortStr := strconv.Itoa(testDefaultRouterListenerPort)

	// Make sure the related env vars are unset
	_ = os.Unsetenv("ZITI_ROUTER_LISTENER_BIND_PORT")

	// Create and run the CLI command
	config, data := createRouterConfig([]string{"edge", "--routerName", "testRouter"}, routerOptions, nil)

	// Check that the template data has been updated as expected
	assert.Equal(t, expectedDefaultPortStr, data.Router.Edge.ListenerBindPort)

	// Check that the actual config output has the correct port
	for i := 1; i < len(config.Link.Listeners); i++ {
		if config.Link.Listeners[i].Binding == "transport" {
			// Assert Bind and Advertise use Bind port value
			require.Equal(t, expectedDefaultPortStr, strings.Split(config.Link.Listeners[i].Bind, ":")[1])
			require.Equal(t, expectedDefaultPortStr, strings.Split(config.Link.Listeners[i].Address, ":")[1])
			break
		}
	}
}

func TestSetZitiEdgeRouterListenerBindPort(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()
	myPortValue := "1234"

	// Set the port manually
	_ = os.Setenv("ZITI_ROUTER_LISTENER_BIND_PORT", myPortValue)

	// Create and run the CLI command
	config, data := createRouterConfig([]string{"edge", "--routerName", "testRouter"}, routerOptions, nil)

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

func TestEdgeRouterCannotBeWSSAndPrivate(t *testing.T) {
	expectedErrorMsg := "Flags for private and wss configs are mutually exclusive. You must choose private or wss, not both"

	// Create the options with both flags set to true
	routerOptions := clearEnvAndInitializeTestData()
	routerOptions.IsPrivate = true
	routerOptions.WssEnabled = true

	err := routerOptions.runEdgeRouter(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestEdgeRouterOutputPathDoesNotExist(t *testing.T) {
	// Set the router options
	routerOptions := clearEnvAndInitializeTestData()
	routerOptions.TunnelerMode = defaultTunnelerMode
	routerOptions.RouterName = "MyEdgeRouter"
	routerOptions.Output = "/IDoNotExist/MyEdgeRouter.yaml"

	err := routerOptions.runEdgeRouter(&ConfigTemplateValues{})

	assert.Error(t, err)
	assert.Equal(t, errors.Unwrap(err), syscall.ENOENT)
}

func TestExecuteCreateConfigRouterEdgeHasNonBlankTemplateValues(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()

	routerName := "MyEdgeRouter"

	// Create and run the CLI command
	_, data := createRouterConfig([]string{"edge", "--routerName", routerName}, routerOptions, nil)

	expectedNonEmptyStringFields := []string{".Router.Edge.ListenerBindPort", ".ZitiHome", ".Hostname", ".Router.Name", ".Router.IdentityCert", ".Router.IdentityServerCert", ".Router.IdentityKey", ".Router.IdentityCA", ".Router.Edge.Port"}
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

func TestEdgeRouterIPOverrideIsConsumed(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()

	routerName := "MyFabricRouter"
	//useful?    blank := ""
	externalIP := "123.456.78.9"

	// Set the env variable to non-empty value
	_ = os.Setenv(constants.ZitiEdgeRouterIPOverrideVarName, externalIP)

	//useful?	// Check that template value is currently blank
	//useful?	assert.Equal(t, blank, data.Router.Edge.IPOverride, "Mismatch router IP override, expected %s but got %s", blank, data.Router.Edge.IPOverride)

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	config, data := createRouterConfig([]string{"edge", "--routerName", routerName}, routerOptions, nil)

	// Check that the template values now contains the custom external IP override value
	assert.Equal(t, externalIP, data.Router.Edge.IPOverride, "Mismatch router IP override, expected %s but got %s", externalIP, data.Router.Edge.IPOverride)

	// Check that the config output has the IP
	found := false
	for i := 1; i < len(config.Edge.Csr.Sans.Ip); i++ {
		if config.Edge.Csr.Sans.Ip[i] == externalIP {
			found = true
		}
	}
	assert.True(t, found, "Expected value not found; expected to find value of "+constants.ZitiEdgeRouterIPOverrideVarName+" in edge router config output.")
}

func TestEdgeRouterCsrFields(t *testing.T) {
	routerOptions := clearEnvAndInitializeTestData()
	routerName := "CstTest"
	config1, data := createRouterConfig([]string{"edge", "--routerName", routerName}, routerOptions, nil)
	// Check that the template values now contains the custom external IP override value
	assert.Equal(t, "US", data.Router.Edge.CsrC)
	assert.Equal(t, "US", config1.Edge.Csr.Country)
	assert.Equal(t, "NC", data.Router.Edge.CsrST)
	assert.Equal(t, "NC", config1.Edge.Csr.Province)
	assert.Equal(t, "Charlotte", data.Router.Edge.CsrL)
	assert.Equal(t, "Charlotte", config1.Edge.Csr.Locality)
	assert.Equal(t, "NetFoundry", data.Router.Edge.CsrO)
	assert.Equal(t, "NetFoundry", config1.Edge.Csr.Organization)
	assert.Equal(t, "Ziti", data.Router.Edge.CsrOU)
	assert.Equal(t, "Ziti", config1.Edge.Csr.OrganizationalUnit)
	assert.Contains(t, config1.Edge.Csr.Sans.Dns, hostname)

	C := "C"
	ST := "ST"
	L := "L"
	O := "O"
	OU := "OU"
	extAddy := "some.external.address"
	_ = os.Setenv(constants.ZitiEdgeRouterCsrCVarName, C)
	_ = os.Setenv(constants.ZitiEdgeRouterCsrSTVarName, ST)
	_ = os.Setenv(constants.ZitiEdgeRouterCsrLVarName, L)
	_ = os.Setenv(constants.ZitiEdgeRouterCsrOVarName, O)
	_ = os.Setenv(constants.ZitiEdgeRouterCsrOUVarName, OU)
	_ = os.Setenv(constants.ZitiRouterCsrSansDnsVarName, extAddy)

	config2, data2 := createRouterConfig([]string{"edge", "--routerName", routerName}, routerOptions, nil)
	// Check that the template values now contains the custom external IP override value
	assert.Equal(t, C, data2.Router.Edge.CsrC)
	assert.Equal(t, C, config2.Edge.Csr.Country)
	assert.Equal(t, ST, data2.Router.Edge.CsrST)
	assert.Equal(t, ST, config2.Edge.Csr.Province)
	assert.Equal(t, L, data2.Router.Edge.CsrL)
	assert.Equal(t, L, config2.Edge.Csr.Locality)
	assert.Equal(t, O, data2.Router.Edge.CsrO)
	assert.Equal(t, O, config2.Edge.Csr.Organization)
	assert.Equal(t, OU, data2.Router.Edge.CsrOU)
	assert.Equal(t, OU, config2.Edge.Csr.OrganizationalUnit)
	assert.Contains(t, config2.Edge.Csr.Sans.Dns, extAddy)
}
