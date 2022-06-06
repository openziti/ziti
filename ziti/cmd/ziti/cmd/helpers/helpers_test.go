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

func TestGetZitiHomeWhenSetWithWindowsSlashes(t *testing.T) {
	// Setup
	varName := "ZITI_HOME"
	expectedValue := "/path/to/ziti/home"

	// Set the env variable using windows backslash
	_ = os.Setenv(varName, strings.ReplaceAll(expectedValue, "/", "\\"))

	// Check that the value matches
	actualValue, _ := GetZitiHome()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiCtrlAdvertisedAddressWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_ADVERTISED_ADDRESS"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	expectedValue, _ := os.Hostname()

	// Check that the value matches
	actualValue, _ := GetZitiCtrlAdvertisedAddress()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetZitiCtrlAdvertisedAddressWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_ADVERTISED_ADDRESS"
	expectedValue := "localhost"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiCtrlAdvertisedAddress()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiCtrlPortWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_PORT"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	expectedValue := "6262"

	// Check that the value matches
	actualValue, _ := GetZitiCtrlPort()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetZitiCtrlPortWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_PORT"
	expectedValue := "1234"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiCtrlPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiCtrlListenerAddressWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_LISTENER_ADDRESS"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	expectedValue := "0.0.0.0"

	// Check that the value matches
	actualValue, _ := GetZitiCtrlListenerAddress()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetZitiCtrlListenerAddressWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_LISTENER_ADDRESS"
	expectedValue := "localhost"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiCtrlListenerAddress()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiCtrlNameWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_CONTROLLER_NAME"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	expectedValue := "controller"

	// Check that the value matches
	actualValue, _ := GetZitiCtrlName()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetZitiCtrlNameWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_CONTROLLER_NAME"
	expectedValue := "MyController"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiCtrlName()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiEdgeRouterPortWhenUnset(t *testing.T) {
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

func TestGetZitiEdgeRouterPortWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_ROUTER_PORT"
	expectedValue := "4321"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiEdgeRouterPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiEdgeCtrlListenerHostPortWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_EDGE_LISTENER_HOST_PORT"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	expectedValue := "0.0.0.0:1280"

	// Check that the value matches
	actualValue, _ := GetZitiEdgeCtrlListenerHostPort()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetZitiEdgeCtrlListenerHostPortWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_CTRL_EDGE_LISTENER_HOST_PORT"
	expectedValue := "localhost:1234"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiEdgeCtrlListenerHostPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiEdgeCtrlAdvertisedHostPortWhenUnset(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT"

	// Ensure the variable is unset
	_ = os.Unsetenv(varName)
	hostname, _ := os.Hostname()
	expectedValue := hostname + ":1280"

	// Check that the value matches
	actualValue, _ := GetZitiEdgeCtrlAdvertisedHostPort()
	assert.Equal(t, expectedValue, actualValue)

	// The env variable should be populated with the expected value
	envValue := os.Getenv(varName)
	assert.Equal(t, expectedValue, envValue)
}

func TestGetZitiEdgeCtrlAdvertisedHostPortWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT"
	expectedValue := "localhost:4321"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiEdgeCtrlAdvertisedHostPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiEdgeCtrlPortWhenNotSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_CONTROLLER_PORT"
	expectedValue := "1280"

	// Be sure the var is unset
	_ = os.Unsetenv(varName)

	// Check that the value matches
	actualValue, _ := GetZitiEdgeCtrlAdvertisedPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiEdgeCtrlPortWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_CONTROLLER_PORT"
	expectedValue := "1234"

	// Set the env variable
	_ = os.Setenv(varName, expectedValue)

	// Check that the value matches
	actualValue, _ := GetZitiEdgeCtrlAdvertisedPort()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiEdgeIdentityEnrollmentDurationWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION"
	expectedValue := 5 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, fmt.Sprintf("%.0f", expectedValue.Minutes()))

	// Check that the value matches
	actualValue, _ := GetZitiEdgeIdentityEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiEdgeIdentityEnrollmentDurationWhenNotSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION"
	expectedValue := 180 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, fmt.Sprintf("%.0f", expectedValue.Minutes()))

	// Check that the value matches
	actualValue, _ := GetZitiEdgeIdentityEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiEdgeRouterEnrollmentDurationWhenSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_ROUTER_ENROLLMENT_DURATION"
	expectedValue := 5 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, fmt.Sprintf("%.0f", expectedValue.Minutes()))

	// Check that the value matches
	actualValue, _ := GetZitiEdgeRouterEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}

func TestGetZitiEdgeRouterEnrollmentDurationWhenNotSet(t *testing.T) {
	// Setup
	varName := "ZITI_EDGE_ROUTER_ENROLLMENT_DURATION"
	expectedValue := 180 * time.Minute

	// Set the env variable
	_ = os.Setenv(varName, fmt.Sprintf("%.0f", expectedValue.Minutes()))

	// Check that the value matches
	actualValue, _ := GetZitiEdgeRouterEnrollmentDuration()
	assert.Equal(t, expectedValue, actualValue)
}
