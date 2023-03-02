package router

import (
	"github.com/openziti/fabric/router/env"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
	"time"

	"github.com/openziti/channel/v2"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tls"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func Test_initializeCtrlEndpoints_ErrorsWithoutDataDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)

	defer os.RemoveAll(tmpDir)

	r := Router{
		config: &Config{},
	}
	_, err = r.getInitialCtrlEndpoints()
	assert.Error(t, err)
	assert.ErrorContains(t, err, "ctrl DataDir not configured")
}

func Test_initializeCtrlEndpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)

	defer os.RemoveAll(tmpDir)

	transport.AddAddressParser(tls.AddressParser{})
	addr, err := transport.ParseAddress("tls:localhost:6565")
	if err != nil {
		t.Fatal(err)
	}
	r := Router{
		config: &Config{
			Ctrl: struct {
				InitialEndpoints      []*UpdatableAddress
				LocalBinding          string
				DefaultRequestTimeout time.Duration
				Options               *channel.Options
				DataDir               string
				Heartbeats            env.HeartbeatOptions
			}{
				DataDir:          tmpDir,
				InitialEndpoints: []*UpdatableAddress{NewUpdatableAddress(addr)},
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

	defer os.RemoveAll(tmpDir)

	transport.AddAddressParser(tls.AddressParser{})
	addr, err := transport.ParseAddress("tls:localhost:6565")
	req.NoError(err)

	addr2, err := transport.ParseAddress("tls:localhost:6767")
	req.NoError(err)

	ctrlDialer := func(address transport.Address, bindHandler channel.BindHandler) error {
		return nil
	}

	r := Router{
		config: &Config{
			Ctrl: struct {
				InitialEndpoints      []*UpdatableAddress
				LocalBinding          string
				DefaultRequestTimeout time.Duration
				Options               *channel.Options
				DataDir               string
				Heartbeats            env.HeartbeatOptions
			}{
				DataDir:          tmpDir,
				InitialEndpoints: []*UpdatableAddress{NewUpdatableAddress(addr), NewUpdatableAddress(addr2)},
			},
		},
		ctrls: env.NewNetworkControllers(time.Minute, ctrlDialer, env.NewDefaultHeartbeatOptions()),
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
