package router

import (
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
	assert.FileExists(t, path.Join(tmpDir, "endpoints"))

	b, err := os.ReadFile(path.Join(tmpDir, "endpoints"))
	assert.NoError(t, err)
	assert.NotEmpty(t, b)

	//TODO: Figure out why we can't just unmarshal directly on struct
	var holder = struct {
		inner ctrlEndpoints
	}{
		inner: newCtrlEndpoints(),
	}

	out := newCtrlEndpoints()
	err = yaml.Unmarshal(b, &holder.inner)
	assert.NoError(t, err)

	for k, v := range out.Items() {
		assert.True(t, expected.Has(k), "Expected to have addr %s", k)
		expectedAddr, _ := expected.Get(k)
		assert.Equal(t, expectedAddr, v)
	}
}
