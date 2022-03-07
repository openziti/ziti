package cmd

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func getZitiEnvironmentVariables() []string {
	return []string{
		"ZITI_HOME",
		"ZITI_CONTROLLER_NAME",
		"ZITI_CTRL_PORT",
		"ZITI_EDGE_ROUTER_RAWNAME",
		"ZITI_EDGE_ROUTER_PORT",
		"ZITI_EDGE_CTRL_IDENTITY_CERT",
		"ZITI_EDGE_CTRL_IDENTITY_SERVER_CERT",
		"ZITI_EDGE_CTRL_IDENTITY_KEY",
		"ZITI_EDGE_CTRL_IDENTITY_CA",
		"ZITI_CTRL_IDENTITY_CERT",
		"ZITI_CTRL_IDENTITY_SERVER_CERT",
		"ZITI_CTRL_IDENTITY_KEY",
		"ZITI_CTRL_IDENTITY_CA",
		"ZITI_SIGNING_CERT",
		"ZITI_SIGNING_KEY",
		"ZITI_ROUTER_IDENTITY_CERT",
		"ZITI_ROUTER_IDENTITY_SERVER_CERT",
		"ZITI_ROUTER_IDENTITY_KEY",
		"ZITI_ROUTER_IDENTITY_CA",
		"ZITI_CTRL_LISTENER_ADDRESS",
		"ZITI_CTRL_ADVERTISED_ADDRESS",
		"ZITI_CTRL_EDGE_LISTENER_HOST_PORT",
		"ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT",
	}
}

// Test that all ZITI_* variables are included in the values for output
func TestThatNoNewUnknownTemplateEnvVariablesExist(t *testing.T) {
	// Get the list of ZITI_* environment variables
	allEnvVars := getZitiEnvironmentVariables()

	// Create a config environment command which will populate the env variable metadata
	NewCmdCreateConfigEnvironment()

	values := reflect.ValueOf(environmentOptions.EnvVariableMetaData)
	elements := make([]interface{}, values.NumField())

	for i := 0; i < values.NumField(); i++ {
		elements[i] = values.Field(i).Interface()
	}

	// Check that every known environment variable is represented in the interface of values
	var unknownValues []string
	for _, value := range elements {
		strValue := fmt.Sprintf("%v", value)
		// If the value starts with ZITI_, make sure it's in the list of known environment variables
		if strings.HasPrefix(strValue, "ZITI_") {
			if !contains(allEnvVars, strValue) {
				unknownValues = append(unknownValues, strValue)
			}
		}
	}

	assert.Zero(t, len(unknownValues))
	for _, value := range unknownValues {
		fmt.Printf("The variable %s was found in template values but is not a known variable. If necessary, update ZITI env variables list in test with the new variable\n", value)
	}
}

// Test that all known ZITI env variables are found in the output
func TestThatAllKnownEnvVariablesAreAccountedForInTemplate(t *testing.T) {
	// Get the list of ZITI_* environment variables
	allEnvVars := getZitiEnvironmentVariables()

	// Create a config environment command which will populate the env variable metadata
	NewCmdCreateConfigEnvironment()

	values := reflect.ValueOf(environmentOptions.EnvVariableMetaData)
	elements := make([]interface{}, values.NumField())

	for i := 0; i < values.NumField(); i++ {
		elements[i] = values.Field(i).Interface()
	}

	// Check that the interface of values doesn't have any remaining variables unaccounted for here
	var unfoundVariables []string
	for _, variable := range allEnvVars {
		found := false
		for _, value := range elements {
			strValue := fmt.Sprintf("%v", value)
			if variable == strValue {
				found = true
				break
			}
		}
		if !found {
			unfoundVariables = append(unfoundVariables, variable)
		}
	}

	assert.Zero(t, len(unfoundVariables))
	for _, value := range unfoundVariables {
		fmt.Printf("The variable %s was not found in template values. Update ZITI env variables list in test by removing this variable if necessary\n", value)
	}
}

// Test that all ZITI_* variables are included in the values for output
func TestThatNoNewUnknownOutputEnvVariablesExist(t *testing.T) {
	// Get the list of ZITI_* environment variables
	allEnvVars := getZitiEnvironmentVariables()

	// Create a config environment command which will populate the env variable metadata
	NewCmdCreateConfigEnvironment()

	// Run the environment options command and capture stdout
	cmd := NewCmdCreateConfigEnvironment()
	cmd.SetArgs([]string{"-o", "stdout"})
	output := captureOutput(func() {
		_ = cmd.Execute()
	})

	// Split the output on newlines
	lines := strings.Split(output, "\n")
	// Check that every known environment variable is represented in the env file output
	prefix := "ZITI_"
	var unknownValues []string
	for _, line := range lines {
		// Only look at lines with a ZITI_* env var
		if !strings.Contains(line, prefix) {
			continue
		}
		// Strip out the variable name and see if it's a known value
		start := strings.Index(line, prefix)
		end := strings.Index(line, "=")
		if end < 0 {
			// If there's no assignment, assume a variable name was referenced in a comment and ignore
			continue
		}
		envVar := strings.TrimSpace(line[start:end])
		if !contains(allEnvVars, envVar) {
			unknownValues = append(unknownValues, envVar)
		}
	}

	assert.Zero(t, len(unknownValues))
	for _, value := range unknownValues {
		fmt.Printf("The variable %s was found in env file output but is not a known variable. If necessary, update ZITI env variables list in test with the new variable\n", value)
	}
}

// Test that all known ZITI_* variables are included in the env file
func TestThatAllKnownEnvVariablesAreAccountedForInOutput(t *testing.T) {
	// Get the list of ZITI_* environment variables
	allEnvVars := getZitiEnvironmentVariables()

	// Create a config environment command which will populate the env variable metadata
	NewCmdCreateConfigEnvironment()

	// Run the environment options command and capture stdout
	cmd := NewCmdCreateConfigEnvironment()
	cmd.SetArgs([]string{"-o", "stdout"})
	output := captureOutput(func() {
		_ = cmd.Execute()
	})

	// Split the output on newlines
	lines := strings.Split(output, "\n")
	// Check that every known environment variable is represented in the env file output
	var unfoundVariables []string
	for _, variable := range allEnvVars {
		found := false
		for _, line := range lines {
			// Check if the line contains an assignment for this env variable
			assignment := variable + "="
			if strings.Contains(line, assignment) {
				found = true
				break
			}
		}
		if !found {
			unfoundVariables = append(unfoundVariables, variable)
		}
	}

	assert.Zero(t, len(unfoundVariables))
	for _, value := range unfoundVariables {
		fmt.Printf("The variable %s was not found in env file output. Update ZITI env variables list in test by removing this variable if necessary\n", value)
	}
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func captureOutput(function func()) string {
	var buffer bytes.Buffer
	oldStdOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	function()

	_ = w.Close()
	os.Stdout = oldStdOut
	_, _ = io.Copy(&buffer, r)
	return buffer.String()
}
