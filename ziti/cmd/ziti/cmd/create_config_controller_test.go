package cmd

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestControllerOutputPathDoesNotExist(t *testing.T) {
	expectedErrorMsg := "stat /IDoNotExist: no such file or directory"

	// Create the options with both flags set to true
	options := &CreateConfigControllerOptions{}
	options.Output = "/IDoNotExist/MyController.yaml"

	err := options.run(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}

func TestExecuteCreateConfigControllerHasNonBlankTemplateValues(t *testing.T) {
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
