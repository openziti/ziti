package helpers

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHomeDirHasNoWindowsSlashes(t *testing.T) {
	value := HomeDir()
	assert.Zero(t, strings.Count(value, "\\"))
}

func TestWorkingDirHasNoWindowsSlashes(t *testing.T) {
	value, _ := WorkingDir()
	assert.Zero(t, strings.Count(value, "\\"))
}

func TestGetZitiHomeWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_HOME"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	wd, _ := os.Getwd()
	expectedValue := NormalizePath(wd)

	// Check that the value matches
	actualValue, _ := GetZitiHome()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetZitiHomeWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_HOME"
	expectedValue := "/path/to/ziti/home"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiHome()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetHomeWhenSetWithWindowsSlashes(t *testing.T) {
	// Setup
	varName := "ZITI_HOME"
	expectedValue := "/path/to/ziti/home"

	// Set the env variable using windows backslash
	_ = os.Setenv(varName, strings.ReplaceAll(expectedValue, "/", "\\"))

	// Check that the value matches
	actualValue, _ := GetZitiHome()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetCtrlEdgeAdvertisedAddressWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_EDGE_ADVERTISED_ADDRESS"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	expectedValue, _ := os.Hostname()

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeAdvertisedAddress()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetCtrlEdgeAdvertisedAddressWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_EDGE_ADVERTISED_ADDRESS"
	expectedValue := "localhost"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeAdvertisedAddress()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetCtrlPortWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_LISTENER_PORT"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	expectedValue := "6262"

	// Check that the value matches
	actualValue, _ := GetCtrlListenerPort()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetCtrlPortWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_LISTENER_PORT"
	expectedValue := "1234"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetCtrlListenerPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetCtrlListenerAddressWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_LISTENER_ADDRESS"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	expectedValue := "0.0.0.0"

	// Check that the value matches
	actualValue, _ := GetCtrlListenerAddress()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetCtrlListenerAddressWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_LISTENER_ADDRESS"
	expectedValue := "localhost"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetCtrlListenerAddress()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetEdgeRouterPortWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_ROUTER_PORT"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	expectedValue := "3022"

	// Check that the value matches
	actualValue, _ := GetZitiEdgeRouterPort()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetEdgeRouterPortWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_ROUTER_PORT"
	expectedValue := "4321"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiEdgeRouterPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetCtrlEdgeAdvertisedPortWhenNotSet(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_EDGE_ADVERTISED_PORT"
	expectedValue := "1280"

	// Be sure the var is unset
	_ = os.Unsetenv(varName)

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeAdvertisedPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetCtrlEdgeAdvertisedPortWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_EDGE_ADVERTISED_PORT"
	expectedValue := "1234"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeAdvertisedPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetEdgeIdentityEnrollmentDurationWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION"
	expectedValue := 5 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, fmt.Sprintf("%.0f", expectedValue.Minutes()))

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeIdentityEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}

/*  Ensure that the default value is returned even if the environment variable is set but is blank. */
func TestGetEdgeIdentityEnrollmentDurationWhenSetToBlank(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION"
	// Expect the default, hard coding the value to act as an alert if default is changed in edge project
	expectedValue := 180 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, "")

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeIdentityEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetEdgeIdentityEnrollmentDurationWhenNotSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION"
	// Expect the default, hard coding the value to act as an alert if default is changed in edge project
	expectedValue := 180 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, fmt.Sprintf("%.0f", expectedValue.Minutes()))

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeIdentityEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetEdgeRouterEnrollmentDurationWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_ROUTER_ENROLLMENT_DURATION"
	expectedValue := 5 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, fmt.Sprintf("%.0f", expectedValue.Minutes()))

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeRouterEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}

/*  Ensure that the default value is returned even if the environment variable is set but is blank. */
func TestGetEdgeRouterEnrollmentDurationWhenSetToBlank(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_ROUTER_ENROLLMENT_DURATION"
	// Expect the default, hard coding the value to act as an alert if default is changed in edge project
	expectedValue := 180 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, "")

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeRouterEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetEdgeRouterEnrollmentDurationWhenNotSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_ROUTER_ENROLLMENT_DURATION"
	// Expect the default, hard coding the value to act as an alert if default is changed in edge project
	expectedValue := 180 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, fmt.Sprintf("%.0f", expectedValue.Minutes()))

	// Check that the value matches
	actualValue, _ := GetCtrlEdgeRouterEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}
