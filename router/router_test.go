package router

import (
	"github.com/openziti/fabric/router/env"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"sync"
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
	err = r.initializeCtrlEndpoints()
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
		ctrlEndpoints: newCtrlEndpoints(),
	}
	expected := newCtrlEndpoints()
	expected.Set(addr.String(), NewUpdatableAddress(addr))

	assert.NoError(t, r.initializeCtrlEndpoints())
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
		ctrls:         env.NewNetworkControllers(time.Minute, env.NewDefaultHeartbeatOptions()),
		ctrlEndpoints: newCtrlEndpoints(),
		controllersToConnect: struct {
			controllers map[*UpdatableAddress]bool
			mtx         sync.Mutex
		}{controllers: map[*UpdatableAddress]bool{}, mtx: sync.Mutex{}},
	}
	expected := newCtrlEndpoints()
	expected.Set(addr.String(), NewUpdatableAddress(addr))

	req.NoError(r.initializeCtrlEndpoints())

	err = r.UpdateCtrlEndpoints([]string{"tls:localhost:6565"})
	req.NoError(err)
	req.FileExists(path.Join(tmpDir, "endpoints"))

	b, err := os.ReadFile(path.Join(tmpDir, "endpoints"))
	req.NoError(err)
	req.NotEmpty(b)

	//TODO: Figure out why we can't just unmarshal directly on struct
	var holder = struct {
		inner ctrlEndpoints
	}{
		inner: newCtrlEndpoints(),
	}

	out := newCtrlEndpoints()
	err = yaml.Unmarshal(b, &holder.inner)
	req.NoError(err)

	for k, v := range out.Items() {
		req.True(expected.Has(k), "Expected to have addr %s", k)
		expectedAddr, _ := expected.Get(k)
		req.Equal(expectedAddr, v)
	}
}
