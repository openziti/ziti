package cmd

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"os"
	"testing"
	"time"
)

/* BEGIN Controller config template structure */

type ControllerConfig struct {
	V            string       `yaml:"v"`
	Db           string       `yaml:"db"`
	Identity     Identity     `yaml:"identity"`
	Ctrl         Ctrl         `yaml:"ctrl"`
	Mgmt         Mgmt         `yaml:"mgmt"`
	HealthChecks HealthChecks `yaml:"healthChecks"`
	Edge         Edge         `yaml:"edge"`
	Web          []Web        `yaml:"web"`
}

type Identity struct {
	Cert        string `yaml:"cert"`
	Server_cert string `yaml:"server_cert"`
	Key         string `yaml:"key"`
	Ca          string `yaml:"ca"`
}

type Ctrl struct {
	Listener string `yaml:"listener"`
}

type Mgmt struct {
	Listener string `yaml:"listener"`
}

type HealthChecks struct {
	BoltCheck BoltCheck `yaml:"boltCheck"`
}

type BoltCheck struct {
	Interval     string `yaml:"interval"`
	Timeout      string `yaml:"timeout"`
	InitialDelay string `yaml:"initialDelay"`
}

type Edge struct {
	Api        Api        `yaml:"api"`
	Enrollment Enrollment `yaml:"enrollment"`
}

type Api struct {
	SessionTimeout string `yaml:"sessionTimeout"`
	Address        string `yaml:"address"`
}

type Enrollment struct {
	SigningCert  SigningCert  `yaml:"signingCert"`
	EdgeIdentity EdgeIdentity `yaml:"edgeIdentity"`
	EdgeRouter   EdgeRouter   `yaml:"edgeRouter"`
}

type SigningCert struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type EdgeIdentity struct {
	Duration string `yaml:"duration"`
}

type EdgeRouter struct {
	Duration string `yaml:"duration"`
}

type Web struct {
	Name       string       `yaml:"name"`
	BindPoints []BindPoints `yaml:"bindPoints"`
	Identity   Identity     `yaml:"identity"`
	Options    Options      `yaml:"options"`
	Apis       []Apis       `yaml:"apis"`
}

type BindPoints struct {
	BpInterface string `yaml:"interface"`
	Address     string `yaml:"address"`
}

type Options struct {
	IdleTimeout   string `yaml:"idleTimeout"`
	ReadTimeout   string `yaml:"readTimeout"`
	WriteTimeout  string `yaml:"writeTimeout"`
	MinTLSVersion string `yaml:"minTLSVersion"`
	MaxTLSVersion string `yaml:"maxTLSVersion"`
}

type Apis struct {
	Binding string     `yaml:"binding"`
	Options ApiOptions `yaml:"options"`
}

type ApiOptions struct {
	// Unsure of this format right now
}

/* END Controller config template structure */

func TestControllerOutputPathDoesNotExist(t *testing.T) {
	expectedErrorMsg := "stat /IDoNotExist: no such file or directory"

	// Create the options with non-existent path
	options := &CreateConfigControllerOptions{}
	options.Output = "/IDoNotExist/MyController.yaml"

	err := options.run(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestCreateConfigControllerTemplateValues(t *testing.T) {
	expectedNonEmptyStringFields := []string{".Controller.Name", ".ZitiHome", ".Controller.IdentityCert", ".Controller.IdentityServerCert", ".Controller.IdentityKey", ".Controller.IdentityCA", ".Controller.ListenerAddress", ".Controller.Port", ".Controller.Edge.AdvertisedHostPort", ".Controller.Edge.ZitiSigningCert", ".Controller.Edge.ZitiSigningKey", ".Controller.Edge.ListenerHostPort", ".Controller.Edge.IdentityCA", ".Controller.Edge.IdentityKey", ".Controller.Edge.IdentityServerCert", ".Controller.Edge.IdentityCert", ".Controller.WebListener.MinTLSVersion", ".Controller.WebListener.MaxTLSVersion"}
	expectedNonEmptyStringValues := []*string{&data.Controller.Name, &data.ZitiHome, &data.Controller.IdentityCert, &data.Controller.IdentityServerCert, &data.Controller.IdentityKey, &data.Controller.IdentityCA, &data.Controller.ListenerAddress, &data.Controller.Port, &data.Controller.Edge.AdvertisedHostPort, &data.Controller.Edge.ZitiSigningCert, &data.Controller.Edge.ZitiSigningKey, &data.Controller.Edge.ListenerHostPort, &data.Controller.Edge.IdentityCA, &data.Controller.Edge.IdentityKey, &data.Controller.Edge.IdentityServerCert, &data.Controller.Edge.IdentityCert, &data.Controller.WebListener.MinTLSVersion, &data.Controller.WebListener.MaxTLSVersion}
	expectedNonEmptyTimeFields := []string{".Controller.HealthCheck.Interval", ".Controller.HealthCheck.Timeout", ".Controller.HealthCheck.InitialDelay", ".Controller.Edge.APISessionTimeout", ".Controller.EdgeIdentityDuration", ".Controller.EdgeRouterDuration", ".Controller.WebListener.IdleTimeout", ".Controller.WebListener.ReadTimeout", ".Controller.WebListener.WriteTimeout"}
	expectedNonEmptyTimeValues := []*time.Duration{&data.Controller.HealthCheck.Interval, &data.Controller.HealthCheck.Timeout, &data.Controller.HealthCheck.InitialDelay, &data.Controller.Edge.APISessionTimeout, &data.Controller.EdgeIdentityDuration, &data.Controller.EdgeRouterDuration, &data.Controller.WebListener.IdleTimeout, &data.Controller.WebListener.ReadTimeout, &data.Controller.WebListener.WriteTimeout}

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	// Check that the expected string template values are not blank
	for field, value := range expectedNonEmptyStringValues {
		assert.NotEqualf(t, "", *value, expectedNonEmptyStringFields[field]+" should be a non-blank value")
	}

	// Check that the expected time.Duration template values are not zero
	for field, value := range expectedNonEmptyTimeValues {
		assert.NotZero(t, *value, expectedNonEmptyTimeFields[field]+" should be a non-zero value")
	}
}

// Edge Ctrl Listener address and port should use default values if env vars are not set
func TestDefaultListenerAddress(t *testing.T) {
	expectedListenerAddress := "0.0.0.0:1280"

	// Make sure the related env vars are unset
	_ = os.Unsetenv("ZITI_EDGE_CONTROLLER_PORT")
	_ = os.Unsetenv("ZITI_CTRL_EDGE_LISTENER_HOST_PORT")

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedListenerAddress, data.Controller.Edge.ListenerHostPort)
}

// Edge Ctrl Listener port should use ZITI_EDGE_CONTROLLER_PORT if it is set
func TestListenerAddressWhenEdgeCtrlPortAndListenerHostPortNotSet(t *testing.T) {
	myPort := "1234"
	expectedListenerAddress := "0.0.0.0:" + myPort

	// Make sure the related env vars are unset
	_ = os.Unsetenv("ZITI_CTRL_EDGE_LISTENER_HOST_PORT")

	// Set the edge controller port
	_ = os.Setenv("ZITI_EDGE_CONTROLLER_PORT", myPort)

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedListenerAddress, data.Controller.Edge.ListenerHostPort)
}

// Edge Ctrl Listener address and port should always use ZITI_EDGE_CTRL_LISTENER_HOST_PORT value if it is set
func TestListenerAddressWhenEdgeCtrlPortAndListenerHostPortSet(t *testing.T) {
	myPort := "1234"
	expectedListenerAddress := "0.0.0.0:4321" // Expecting a different port even when edge ctrl port is set

	// Set a custom value for the host and port
	_ = os.Setenv("ZITI_CTRL_EDGE_LISTENER_HOST_PORT", expectedListenerAddress)

	// Set the edge controller port (this should not show up in the end resulting listener address)
	_ = os.Setenv("ZITI_EDGE_CONTROLLER_PORT", myPort)

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedListenerAddress, data.Controller.Edge.ListenerHostPort)
}

// Edge Ctrl Advertised Port should update the edge ctrl port to the default when ZITI_EDGE_CONTROLLER_PORT is not set
func TestDefaultEdgeCtrlAdvertisedPort(t *testing.T) {
	expectedPort := "1280" // Expecting the default port of 1280

	// Set a custom value for the host and port
	_ = os.Unsetenv("ZITI_EDGE_CONTROLLER_PORT")

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedPort, data.Controller.Edge.AdvertisedPort)
}

// Edge Ctrl Advertised Port should update the edge ctrl port to the custom value when ZITI_EDGE_CONTROLLER_PORT is set
func TestEdgeCtrlAdvertisedPortValueWhenSet(t *testing.T) {
	expectedPort := "1234" // Setting a custom port which is not the default value

	// Set a custom value for the host and port
	_ = os.Setenv("ZITI_EDGE_CONTROLLER_PORT", expectedPort)

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedPort, data.Controller.Edge.AdvertisedPort)
}

func TestDefaultEdgeIdentityEnrollmentDuration(t *testing.T) {
	// Expect the default (3 hours)
	expectedDuration := time.Duration(180) * time.Minute
	expectedConfigValue := "180m"

	// Unset the env var so the default is used
	_ = os.Unsetenv("ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION")

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeIdentityDuration)

	// Expect that the config value is represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedConfigValue, configStruct.Edge.Enrollment.EdgeIdentity.Duration)
}

func TestEdgeIdentityEnrollmentDurationWhenEnvVarSet(t *testing.T) {
	expectedDuration := 5 * time.Minute // Setting a custom duration which is not the default value
	expectedConfigValue := "5m"

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION", fmt.Sprintf("%.0f", expectedDuration.Minutes()))

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeIdentityDuration)

	// Expect that the config value is represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedConfigValue, configStruct.Edge.Enrollment.EdgeIdentity.Duration)
}

func TestEdgeIdentityEnrollmentDurationWhenEnvVarSetToBlank(t *testing.T) {
	// Expect the default (3 hours)
	expectedDuration := time.Duration(180) * time.Minute
	expectedConfigValue := "180m"

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION", "")

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeIdentityDuration)

	// Expect that the config value is represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedConfigValue, configStruct.Edge.Enrollment.EdgeIdentity.Duration)
}

func TestEdgeIdentityEnrollmentDurationCLITakesPriority(t *testing.T) {
	envVarValue := 5 * time.Minute // Setting a custom duration which is not the default value
	cliValue := "10m"              // Setting a CLI custom duration which is also not the default value
	expectedConfigValue := "10m"

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION", fmt.Sprintf("%.0f", envVarValue.Minutes()))

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	cmd.SetArgs([]string{"--identityEnrollmentDuration", cliValue})
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	// Expect that the CLI value was used over the environment variable
	expectedValue, _ := time.ParseDuration(cliValue)
	assert.Equal(t, expectedValue, data.Controller.EdgeIdentityDuration)

	// Expect that the config value is represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedConfigValue, configStruct.Edge.Enrollment.EdgeIdentity.Duration)
}

func TestDefaultEdgeRouterEnrollmentDuration(t *testing.T) {
	expectedDuration := time.Duration(180) * time.Minute
	expectedConfigValue := "180m"

	// Unset the env var so the default is used
	_ = os.Unsetenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION")

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeRouterDuration)

	// Expect that the config value is represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedConfigValue, configStruct.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterEnrollmentDurationWhenEnvVarSet(t *testing.T) {
	expectedDuration := 5 * time.Minute // Setting a custom duration which is not the default value
	expectedConfigValue := "5m"

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION", fmt.Sprintf("%.0f", expectedDuration.Minutes()))

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeRouterDuration)

	// Expect that the config value is represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedConfigValue, configStruct.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterEnrollmentDurationWhenEnvVarSetToBlank(t *testing.T) {
	// Expect the default (3 hours)
	expectedDuration := time.Duration(180) * time.Minute
	expectedConfigValue := "180m"

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION", "")

	// Create and run the CLI command
	cmd := NewCmdCreateConfigController()
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeRouterDuration)

	// Expect that the config value is represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedConfigValue, configStruct.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterEnrollmentDurationCLITakesPriority(t *testing.T) {
	envVarValue := 5 * time.Minute // Setting a custom duration which is not the default value
	cliValue := "10m"              // Setting a CLI custom duration which is also not the default value
	expectedConfigValue := "10m"   // Config value representation should be in minutes

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION", fmt.Sprintf("%.0f", envVarValue.Minutes()))

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	cmd.SetArgs([]string{"--routerEnrollmentDuration", cliValue})
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	// Expect that the CLI value was used over the environment variable
	expectedValue, _ := time.ParseDuration(cliValue)
	assert.Equal(t, expectedValue, data.Controller.EdgeRouterDuration)

	// Expect that the config value is represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedConfigValue, configStruct.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterEnrollmentDurationCLIConvertsToMin(t *testing.T) {
	cliValue := "1h"             // Setting a CLI custom duration which is also not the default value
	expectedConfigValue := "60m" // Config value representation should be in minutes

	// Make sure the env var is not set
	_ = os.Unsetenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION")

	// Create and run the CLI command
	cmd := NewCmdCreateConfigController()
	cmd.SetArgs([]string{"--routerEnrollmentDuration", cliValue})
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	// Expect that the CLI value was used over the environment variable
	expectedValue, _ := time.ParseDuration(cliValue)
	assert.Equal(t, expectedValue, data.Controller.EdgeRouterDuration)

	// Expect that the config value is represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedConfigValue, configStruct.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterAndIdentityEnrollmentDurationTogetherCLI(t *testing.T) {
	cliIdentityDurationValue := "1h"
	cliRouterDurationValue := "30m"
	expectedIdentityConfigValue := "60m"
	expectedRouterConfigValue := "30m"

	// Make sure the env vars are not set
	_ = os.Unsetenv("ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION")
	_ = os.Unsetenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION")

	// Create and run the CLI command
	cmd := NewCmdCreateConfigController()
	cmd.SetArgs([]string{"--routerEnrollmentDuration", cliRouterDurationValue, "--identityEnrollmentDuration", cliIdentityDurationValue})
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	// Expect that the config values are represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedIdentityConfigValue, configStruct.Edge.Enrollment.EdgeIdentity.Duration)
	assert.Equal(t, expectedRouterConfigValue, configStruct.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterAndIdentityEnrollmentDurationTogetherEnvVar(t *testing.T) {
	envVarIdentityDurationValue := "120"
	envVarRouterDurationValue := "60"
	expectedIdentityConfigValue := "120m"
	expectedRouterConfigValue := "60m"

	// Set the env vars
	_ = os.Setenv("ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION", envVarIdentityDurationValue)
	_ = os.Setenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION", envVarRouterDurationValue)

	// Create and run the CLI command
	cmd := NewCmdCreateConfigController()
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	// Expect that the config values are represented correctly
	configStruct := configToStruct(configOutput)
	assert.Equal(t, expectedIdentityConfigValue, configStruct.Edge.Enrollment.EdgeIdentity.Duration)
	assert.Equal(t, expectedRouterConfigValue, configStruct.Edge.Enrollment.EdgeRouter.Duration)
}

func configToStruct(config string) ControllerConfig {
	configStruct := ControllerConfig{}
	err2 := yaml.Unmarshal([]byte(config), &configStruct)
	if err2 != nil {
		fmt.Println(err2)
	}
	return configStruct
}
