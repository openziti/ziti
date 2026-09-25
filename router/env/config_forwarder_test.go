package env

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_LoadForwarderOptions_EndpointFaultRetentionDefaults(t *testing.T) {
	options, err := LoadForwarderOptions(map[interface{}]interface{}{})
	require.NoError(t, err)
	require.Equal(t, DefaultEndpointFaultRetention, options.EndpointFaultRetention)
}

func Test_LoadForwarderOptions_EndpointFaultRetentionParsesDuration(t *testing.T) {
	options, err := LoadForwarderOptions(map[interface{}]interface{}{
		"endpointFaultRetention": "90s",
	})
	require.NoError(t, err)
	require.Equal(t, 90*time.Second, options.EndpointFaultRetention)
}

func Test_LoadForwarderOptions_EndpointFaultRetentionRejectsNonDuration(t *testing.T) {
	_, err := LoadForwarderOptions(map[interface{}]interface{}{
		"endpointFaultRetention": "not-a-duration",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "endpointFaultRetention")
}

// Test_LoadForwarderOptions_EndpointFaultRetentionRejectsNumber guards the convention: durations
// are configured as strings, and a bare number must be rejected rather than silently read as some
// unit.
func Test_LoadForwarderOptions_EndpointFaultRetentionRejectsNumber(t *testing.T) {
	_, err := LoadForwarderOptions(map[interface{}]interface{}{
		"endpointFaultRetention": 300000,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duration")
}
