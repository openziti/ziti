package router

import (
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tls"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// mockNetworkControllers implements env.NetworkControllers for testing endpoint saving
// without actually making network connections
type mockNetworkControllers struct {
	env.MockNetworkControllers
	endpoints map[string]struct{}
}

func newMockNetworkControllers() *mockNetworkControllers {
	return &mockNetworkControllers{
		endpoints: make(map[string]struct{}),
	}
}

func (m *mockNetworkControllers) UpdateControllerEndpoints(endpoints []string) bool {
	changed := false
	newEndpoints := make(map[string]struct{})
	for _, ep := range endpoints {
		newEndpoints[ep] = struct{}{}
		if _, exists := m.endpoints[ep]; !exists {
			changed = true
		}
	}
	for ep := range m.endpoints {
		if _, exists := newEndpoints[ep]; !exists {
			changed = true
		}
	}
	m.endpoints = newEndpoints
	return changed
}

func Test_initializeCtrlEndpoints_ErrorsWithoutDataDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)

	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	r := Router{
		config: &env.Config{},
	}
	_, err = r.getInitialCtrlEndpoints()
	assert.Error(t, err)
	assert.ErrorContains(t, err, "ctrl endpointsFile not configured")
}

func Test_initializeCtrlEndpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)

	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	transport.AddAddressParser(tls.AddressParser{})
	addr, err := transport.ParseAddress("tls:localhost:6565")
	if err != nil {
		t.Fatal(err)
	}
	r := Router{
		config: &env.Config{
			Ctrl: struct {
				InitialEndpoints      []*env.UpdatableAddress
				LocalBinding          string
				DefaultRequestTimeout time.Duration
				Options               *channel.Options
				EndpointsFile         string
				Heartbeats            env.HeartbeatOptions
				StartupTimeout        time.Duration
				RateLimit             command.AdaptiveRateLimiterConfig
				Listeners             []*env.CtrlListenerConfig
			}{
				EndpointsFile:    filepath.Join(tmpDir, "endpoints"),
				InitialEndpoints: []*env.UpdatableAddress{env.NewUpdatableAddress(addr)},
			},
		},
	}
	expected := []string{addr.String()}
	endpoints, err := r.getInitialCtrlEndpoints()
	assert.NoError(t, err)
	assert.Equal(t, expected, endpoints)
	assert.NoFileExists(t, path.Join(tmpDir, "endpoints"))
}

func Test_updateCtrlEndpoints(t *testing.T) {
	req := require.New(t)
	tmpDir, err := os.MkdirTemp("", "")
	req.NoError(err)

	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	transport.AddAddressParser(tls.AddressParser{})
	addr, err := transport.ParseAddress("tls:localhost:6565")
	req.NoError(err)

	addr2, err := transport.ParseAddress("tls:localhost:6767")
	req.NoError(err)

	cfg := &env.Config{
		Ctrl: struct {
			InitialEndpoints      []*env.UpdatableAddress
			LocalBinding          string
			DefaultRequestTimeout time.Duration
			Options               *channel.Options
			EndpointsFile         string
			Heartbeats            env.HeartbeatOptions
			StartupTimeout        time.Duration
			RateLimit             command.AdaptiveRateLimiterConfig
			Listeners             []*env.CtrlListenerConfig
		}{
			EndpointsFile:         filepath.Join(tmpDir, "endpoints"),
			InitialEndpoints:      []*env.UpdatableAddress{env.NewUpdatableAddress(addr), env.NewUpdatableAddress(addr2)},
			DefaultRequestTimeout: time.Minute,
			Options:               channel.DefaultOptions(),
		},
	}

	r := Router{
		config: cfg,
		ctrls:  newMockNetworkControllers(),
	}

	endpoints, err := r.getInitialCtrlEndpoints()
	req.NoError(err)
	r.UpdateCtrlEndpoints(endpoints)

	r.UpdateCtrlEndpoints([]string{"tls:localhost:6565"})
	req.FileExists(path.Join(tmpDir, "endpoints"))

	b, err := os.ReadFile(path.Join(tmpDir, "endpoints"))
	req.NoError(err)
	req.NotEmpty(b)

	endpointCfg := &endpointConfig{}

	err = yaml.Unmarshal(b, &endpointCfg)
	req.NoError(err)

	req.Equal(1, len(endpointCfg.Endpoints))
	req.Equal(addr.String(), endpointCfg.Endpoints[0])
}
