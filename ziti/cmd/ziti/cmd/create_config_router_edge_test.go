package cmd

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

var defaultArgs = []string{"edge", "--routerName", "test-router"}
var testHostname, _ = os.Hostname()

func setEnvByMap[K string, V string](m map[K]V) {
	for k, v := range m {
		os.Setenv(string(k), string(v))
	}
}

func execCreateConfigCommand(args []string, keys map[string]string) {
	// Setup options
	clearOptionsAndTemplateData()
	routerOptions.Output = defaultOutput

	setEnvByMap(keys)
	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigRouter()
	cmd.SetArgs(args)
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})
}

func TestEdgeRouterAdvertised(t *testing.T) {
	routerAdvHostIp := "192.168.10.10"
	routerAdvHostDns := "controller01.zitinetwork.example.org"
	keys := map[string]string{
		"ZITI_CTRL_PORT":        "80",
		"ZITI_EDGE_ROUTER_PORT": "443",
	}
	execCreateConfigCommand(defaultArgs, keys)
	assert.Equal(t, testHostname, data.Router.Edge.AdvertisedHost, nil)

	keys["ZITI_EDGE_ROUTER_RAWNAME"] = routerAdvHostDns
	execCreateConfigCommand(defaultArgs, keys)
	assert.Equal(t, routerAdvHostDns, data.Router.Edge.AdvertisedHost, nil)

	keys["ZITI_EDGE_ROUTER_RAWNAME"] = ""
	keys["ZITI_EDGE_ROUTER_IP_OVERRIDE"] = routerAdvHostIp
	execCreateConfigCommand(defaultArgs, keys)
	assert.Equal(t, routerAdvHostIp, data.Router.Edge.AdvertisedHost, nil)

	keys["ZITI_EDGE_ROUTER_IP_OVERRIDE"] = ""
	keys["EXTERNAL_DNS"] = routerAdvHostDns
	execCreateConfigCommand(defaultArgs, keys)
	assert.Equal(t, routerAdvHostDns, data.Router.Edge.AdvertisedHost, nil)

	keys["ZITI_ROUTER_ADVERTISED_HOST"] = routerAdvHostIp
	keys["ZITI_EDGE_ROUTER_IP_OVERRIDE"] = routerAdvHostIp
	execCreateConfigCommand(defaultArgs, keys)
	assert.Equal(t, routerAdvHostIp, data.Router.Edge.AdvertisedHost, nil)

	keys["ZITI_ROUTER_ADVERTISED_HOST"] = routerAdvHostDns
	keys["EXTERNAL_DNS"] = routerAdvHostDns
	execCreateConfigCommand(defaultArgs, keys)
	assert.Equal(t, routerAdvHostDns, data.Router.Edge.AdvertisedHost, nil)
}
