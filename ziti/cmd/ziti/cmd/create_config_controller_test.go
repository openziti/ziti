package cmd

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

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
	expectedDuration := time.Duration(180) * time.Minute

	// Unset the env var so the default is used
	_ = os.Unsetenv("ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION")

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeIdentityDuration)
}

func TestEdgeIdentityEnrollmentDurationWhenSet(t *testing.T) {
	expectedDuration := 5 * time.Minute // Setting a custom duration which is not the default value

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION", fmt.Sprintf("%.0f", expectedDuration.Minutes()))

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeIdentityDuration)
}

func TestEdgeIdentityEnrollmentDurationCLITakesPriority(t *testing.T) {
	envVarValue := 5 * time.Minute // Setting a custom duration which is not the default value
	cliValue := "10m"              // Setting a CLI custom duration which is also not the default value

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION", fmt.Sprintf("%.0f", envVarValue.Minutes()))

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	cmd.SetArgs([]string{"--identityEnrollmentDuration", cliValue})
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	// Expect that the CLI value was used over the environment variable
	expectedValue, _ := time.ParseDuration(cliValue)
	assert.Equal(t, expectedValue, data.Controller.EdgeIdentityDuration)
}

func TestDefaultEdgeRouterEnrollmentDuration(t *testing.T) {
	expectedDuration := time.Duration(180) * time.Minute

	// Unset the env var so the default is used
	_ = os.Unsetenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION")

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeRouterDuration)
}

func TestEdgeRouterEnrollmentDurationWhenSet(t *testing.T) {
	expectedDuration := 5 * time.Minute // Setting a custom duration which is not the default value

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION", fmt.Sprintf("%.0f", expectedDuration.Minutes()))

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	assert.Equal(t, expectedDuration, data.Controller.EdgeRouterDuration)
}

func TestEdgeRouterEnrollmentDurationCLITakesPriority(t *testing.T) {
	envVarValue := 5 * time.Minute // Setting a custom duration which is not the default value
	cliValue := "10m"              // Setting a CLI custom duration which is also not the default value

	// Set a custom value for the enrollment duration
	_ = os.Setenv("ZITI_EDGE_ROUTER_ENROLLMENT_DURATION", fmt.Sprintf("%.0f", envVarValue.Minutes()))

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	cmd := NewCmdCreateConfigController()
	cmd.SetArgs([]string{"--routerEnrollmentDuration", cliValue})
	_ = captureOutput(func() {
		_ = cmd.Execute()
	})

	// Expect that the CLI value was used over the environment variable
	expectedValue, _ := time.ParseDuration(cliValue)
	assert.Equal(t, expectedValue, data.Controller.EdgeRouterDuration)
}
