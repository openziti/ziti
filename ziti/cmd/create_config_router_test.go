package cmd

import (
	"fmt"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"os"
	"testing"
)

/* BEGIN Controller config template structure */

type RouterConfig struct {
	V         string     `yaml:"v"`
	Identity  Identity   `yaml:"identity"`
	Ctrl      RouterCtrl `yaml:"ctrl"`
	Link      Link       `yaml:"link"`
	Listeners []Listener `yaml:"listeners"`
	Csr       Csr        `yaml:"csr"`
	Edge      RouterEdge `yaml:"edge"`
	Transport Transport  `yaml:"transport"`
	Forwarder Forwarder  `yaml:"forwarder"`
}

type RouterCtrl struct {
	Endpoint string `yaml:"endpoint"`
}

type Link struct {
	Dialers      []Dialer   `yaml:"dialers"`
	Listeners    []Listener `yaml:"listeners"`
	Timeout      string     `yaml:"timeout"`
	InitialDelay string     `yaml:"initialDelay"`
}

type Dialer struct {
	Binding string `yaml:"binding"`
}

type Listener struct {
	Binding   string          `yaml:"binding"`
	Bind      string          `yaml:"bind"`
	Advertise string          `yaml:"advertise"`
	Address   string          `yaml:"address"`
	Options   ListenerOptions `yaml:"options"`
}

type ListenerOptions struct {
	Advertise         string   `yaml:"advertise"`
	ConnectTimeout    int      `yaml:"connectTimeoutMs"`
	GetSessionTimeout int      `yaml:"getSessionTimeout"`
	Mode              string   `yaml:"mode"`
	Resolver          string   `yaml:"resolver"`
	LanIf             []string `yaml:"lanIf"`
	OutQueueSize      string   `yaml:"outQueueSize"`
}

type RouterEdge struct {
	Csr Csr `yaml:"csr"`
}

type Csr struct {
	Country            string `yaml:"country"`
	Province           string `yaml:"province"`
	Locality           string `yaml:"locality"`
	Organization       string `yaml:"organization"`
	OrganizationalUnit string `yaml:"organizationalUnit"`
	Sans               Sans   `yaml:"sans"`
}

type Sans struct {
	Dns []string `yaml:"dns"`
	Ip  []string `yaml:"ip"`
}

type Transport struct {
	Ws Ws `yaml:"ws"`
}

type Ws struct {
	writeTimeout      int    `yaml:"writeTimeout"`
	readTimeout       int    `yaml:"readTimeout"`
	idleTimeout       int    `yaml:"idleTimeout"`
	pongTimeout       int    `yaml:"pongTimeout"`
	pingInterval      int    `yaml:"pingInterval"`
	handshakeTimeout  int    `yaml:"handshakeTimeout"`
	readBufferSize    int    `yaml:"readBufferSize"`
	writeBufferSize   int    `yaml:"writeBufferSize"`
	enableCompression string `yaml:"enableCompression"`
	server_cert       string `yaml:"server_Cert"`
	key               string `yaml:"key"`
}

type Forwarder struct {
	LatencyProbeInterval  int `yaml:"latencyProbeInterval"`
	XgressDialQueueLength int `yaml:"xgressDialQueueLength"`
	XgressDialWorkerCount int `yaml:"xgressDialWorkerCount"`
	LinkDialQueueLength   int `yaml:"linkDialQueueLength"`
	LinkDialWorkerCount   int `yaml:"linkDialWorkerCount"`
}

/* END Controller config template structure */

func createRouterConfig(args []string) RouterConfig {
	// Create and run the CLI command
	cmd := NewCmdCreateConfigRouter()
	cmd.SetArgs(args)
	// captureOutput is used to consume output, otherwise config prints to stdout along with test results
	output := captureOutput(func() {
		_ = cmd.Execute()
	})

	// Convert output to config struct
	configStruct := RouterConfig{}
	err2 := yaml.Unmarshal([]byte(output), &configStruct)
	if err2 != nil {
		fmt.Println(err2)
	}
	return configStruct
}

func clearOptionsAndTemplateData() {
	routerOptions = CreateConfigRouterOptions{}
	data = &ConfigTemplateValues{}

	// Unset environment variables
	envVars := getZitiEnvironmentVariables()
	for i := 0; i < len(envVars); i++ {
		_ = os.Unsetenv(envVars[i])
	}
	_ = os.Unsetenv(constants.ExternalDNSVarName)
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
	clearOptionsAndTemplateData()
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
	clearOptionsAndTemplateData()
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
