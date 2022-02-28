package cmd

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestControllerOutputPathDoesNotExist(t *testing.T) {
	expectedErrorMsg := "stat /IDoNotExist: no such file or directory"

	// Create the options with both flags set to true
	options := &CreateConfigControllerOptions{}
	options.Output = "/IDoNotExist/MyController.yaml"

	err := options.run(&ConfigTemplateValues{})

	assert.EqualError(t, err, expectedErrorMsg, "Error does not match, expected %s but got %s", expectedErrorMsg, err)
}
