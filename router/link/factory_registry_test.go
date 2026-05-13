/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package link

import (
	"sync"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/router/xlink"

	"github.com/stretchr/testify/require"
)

// --- Test doubles for xlink.Factory / Listener / Dialer ---------------------

type fakeFactory struct {
	mu              sync.Mutex
	createdListeners []*fakeListener
	createdDialers   []*fakeDialer
	dialerConfigs    []transport.Configuration
	listenerErr      error
	dialerErr        error
}

func (f *fakeFactory) CreateListener(id *identity.TokenId, cfg transport.Configuration) (xlink.Listener, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listenerErr != nil {
		return nil, f.listenerErr
	}
	bind, _ := cfg["bind"].(string)
	binding, _ := cfg["binding"].(string)
	l := &fakeListener{bind: bind, binding: binding}
	f.createdListeners = append(f.createdListeners, l)
	return l, nil
}

func (f *fakeFactory) CreateDialer(id *identity.TokenId, cfg transport.Configuration) (xlink.Dialer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dialerErr != nil {
		return nil, f.dialerErr
	}
	configCopy := transport.Configuration{}
	for k, v := range cfg {
		configCopy[k] = v
	}
	f.dialerConfigs = append(f.dialerConfigs, configCopy)
	binding, _ := cfg["binding"].(string)
	d := &fakeDialer{binding: binding}
	f.createdDialers = append(f.createdDialers, d)
	return d, nil
}

type fakeListener struct {
	bind    string
	binding string
	started bool
	closed  bool
	listenErr error
}

func (l *fakeListener) Listen() error             { l.started = true; return l.listenErr }
func (l *fakeListener) GetAdvertisement() string  { return l.bind }
func (l *fakeListener) GetLinkProtocol() string   { return l.binding }
func (l *fakeListener) GetLinkCostTags() []string { return nil }
func (l *fakeListener) GetGroups() []string       { return nil }
func (l *fakeListener) GetLocalBinding() string   { return "" }
func (l *fakeListener) Close() error              { l.closed = true; return nil }

type fakeDialer struct {
	binding string
	adopted string
}

func (d *fakeDialer) Dial(xlink.Dial) (xlink.Xlink, error) { return nil, nil }
func (d *fakeDialer) GetGroups() []string                  { return nil }
func (d *fakeDialer) GetBinding() string                   { return d.binding }
func (d *fakeDialer) GetHealthyBackoffConfig() xlink.BackoffConfig {
	return nil
}
func (d *fakeDialer) GetUnhealthyBackoffConfig() xlink.BackoffConfig { return nil }
func (d *fakeDialer) AdoptBinding(l xlink.Listener) {
	d.adopted = l.GetLocalBinding()
	d.binding = "adopted"
}

func mustTokenId(t *testing.T) *identity.TokenId {
	t.Helper()
	return &identity.TokenId{Token: "test-router"}
}

func newTestRegistry(t *testing.T) (*FactoryRegistry, *fakeFactory) {
	t.Helper()
	r := NewFactoryRegistry(mustTokenId(t))
	f := &fakeFactory{}
	require.NoError(t, r.Register("transport", f))
	return r, f
}

// --- Tests ------------------------------------------------------------------

func Test_FactoryRegistry_Register_RejectsDifferentFactoryForSameBinding(t *testing.T) {
	req := require.New(t)
	r := NewFactoryRegistry(mustTokenId(t))
	f1 := &fakeFactory{}
	req.NoError(r.Register("transport", f1))
	// Same factory re-registered: no-op.
	req.NoError(r.Register("transport", f1))
	// Different factory for same binding: error.
	f2 := &fakeFactory{}
	req.Error(r.Register("transport", f2))
}

func Test_FactoryRegistry_Apply_BuildsListenersAndDialers(t *testing.T) {
	req := require.New(t)
	r, f := newTestRegistry(t)

	data := `{
		"listeners": [{"bind": "tls:0.0.0.0:6262"}],
		"dialers":   [{}]
	}`
	req.NoError(r.Apply(1, data))

	listeners := r.Listeners()
	dialers := r.Dialers()
	req.Len(listeners, 1)
	req.Len(dialers, 1)
	req.True(f.createdListeners[0].started, "Listener.Listen() should have been called")
	req.Equal("tls:0.0.0.0:6262", f.createdListeners[0].bind)
}

func Test_FactoryRegistry_Apply_MapsDialerBindInterfaceToTransportBind(t *testing.T) {
	req := require.New(t)
	r, f := newTestRegistry(t)

	data := `{"dialers":[{"bindInterface":"eth0"}]}`
	req.NoError(r.Apply(1, data))

	req.Len(f.dialerConfigs, 1)
	req.Equal("eth0", f.dialerConfigs[0]["bind"])
	req.NotContains(f.dialerConfigs[0], "bindInterface")
}

func Test_FactoryRegistry_Apply_AdoptsBindingWhenSingleListenerAndDialer(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	data := `{
		"listeners": [{"bind": "tls:0.0.0.0:6262"}],
		"dialers":   [{}]
	}`
	req.NoError(r.Apply(1, data))

	// fakeDialer.AdoptBinding sets binding to "adopted"
	req.Equal("adopted", r.Dialers()[0].GetBinding())
}

func Test_FactoryRegistry_Apply_ClosesOldListenersOnRebuild(t *testing.T) {
	req := require.New(t)
	r, f := newTestRegistry(t)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))
	oldListener := f.createdListeners[0]
	req.False(oldListener.closed)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6263"}]}`))
	req.True(oldListener.closed, "previous listener should be closed on rebuild")
	// New listener built and started.
	req.Len(f.createdListeners, 2)
	req.True(f.createdListeners[1].started)
}

func Test_FactoryRegistry_Apply_UnknownBinding(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	err := r.Apply(1, `{"listeners":[{"binding":"made-up","bind":"x"}]}`)
	req.Error(err)
	req.Empty(r.Listeners())
}

func Test_FactoryRegistry_Apply_MalformedJson(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	err := r.Apply(1, `{not json`)
	req.Error(err)
}

func Test_FactoryRegistry_Apply_UnsupportedVersion(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	err := r.Apply(2, `{}`)
	req.Error(err)
}

func Test_FactoryRegistry_Apply_ListenerCreateError_LeavesStateUnchanged(t *testing.T) {
	req := require.New(t)
	r, f := newTestRegistry(t)

	// Successful first apply.
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))
	req.Len(r.Listeners(), 1)
	firstListener := f.createdListeners[0]

	// Force factory to fail on next CreateListener.
	f.mu.Lock()
	f.listenerErr = fakeErr("kaboom")
	f.mu.Unlock()

	err := r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:9999"}]}`)
	req.Error(err)
	// On error from build(), state is left unchanged — old listener remains.
	req.Len(r.Listeners(), 1)
	req.False(firstListener.closed, "old listener should not be closed when new build fails")
}

func Test_FactoryRegistry_Remove_TearsDown(t *testing.T) {
	req := require.New(t)
	r, f := newTestRegistry(t)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"dialers":[{}]}`))
	req.NoError(r.Remove())

	req.Empty(r.Listeners())
	req.Empty(r.Dialers())
	req.Nil(r.GetConfig())
	req.True(f.createdListeners[0].closed)
}

// --- Concurrent access -------------------------------------------------------

func Test_FactoryRegistry_AccessorsReturnSnapshots(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))

	snapshot := r.Listeners()
	// Reapply with empty listeners should NOT mutate the previously-returned slice.
	req.NoError(r.Apply(1, `{}`))
	req.Len(snapshot, 1, "previously-returned slice should be a stable snapshot")
	req.Empty(r.Listeners())
}

// --- helpers -----------------------------------------------------------------

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func Test_ConfigFromLocalYaml_EmptyReturnsEmptyString(t *testing.T) {
	req := require.New(t)
	js, err := ConfigFromLocalYaml(LocalYamlConfig{})
	req.NoError(err)
	req.Equal("", js)
}

func Test_ConfigFromLocalYaml_Roundtrip(t *testing.T) {
	req := require.New(t)
	yaml := LocalYamlConfig{
		Listeners: []map[interface{}]interface{}{
			{
				"binding":       "transport",
				"bind":          "tls:0.0.0.0:6262",
				"advertise":     "tls:router1:6262",
				"bindInterface": "eth0",
				"groups":        []interface{}{"default", "mesh"},
				"options": map[interface{}]interface{}{
					"outQueueSize":   16,
					"connectTimeout": "30s",
				},
			},
		},
		Dialers: []map[interface{}]interface{}{
			{
				"binding":  "transport",
				"groups":   "default", // single-string form
			},
		},
		Heartbeats: channel.HeartbeatOptions{
			SendInterval:             5 * time.Second,
			CheckInterval:            10 * time.Second,
			CloseUnresponsiveTimeout: 30 * time.Second,
		},
		PayloadSenderQueueSize: 256,
		AckSenderQueueSize:     128,
	}

	js, err := ConfigFromLocalYaml(yaml)
	req.NoError(err)
	req.NotEmpty(js)

	cfg, err := ParseConfig(js)
	req.NoError(err)

	req.Len(cfg.Listeners, 1)
	l := cfg.Listeners[0]
	req.Equal("transport", l.Binding)
	req.Equal("tls:0.0.0.0:6262", l.Bind)
	req.Equal("tls:router1:6262", l.Advertise)
	req.Equal("eth0", l.BindInterface)
	req.Equal(Groups{"default", "mesh"}, l.Groups)
	req.NotNil(l.Options)
	req.Equal(16, l.Options.OutQueueSize)
	req.Equal("30s", l.Options.ConnectTimeout)

	req.Len(cfg.Dialers, 1)
	d := cfg.Dialers[0]
	req.Equal("transport", d.Binding)
	req.Equal(Groups{"default"}, d.Groups, "single-string groups should normalize to []")

	req.NotNil(cfg.Heartbeats)
	req.Equal("5s", cfg.Heartbeats.SendInterval)
	req.Equal(10*time.Second, mustParseDur(req, cfg.Heartbeats.CheckInterval))

	req.Equal(256, cfg.PayloadSenderQueueSize)
	req.Equal(128, cfg.AckSenderQueueSize)
}

func Test_ConfigFromLocalYaml_DialerLegacyBindMapsToBindInterface(t *testing.T) {
	req := require.New(t)
	yaml := LocalYamlConfig{
		Dialers: []map[interface{}]interface{}{
			{
				"bind": "eth0",
			},
		},
	}

	js, err := ConfigFromLocalYaml(yaml)
	req.NoError(err)

	cfg, err := ParseConfig(js)
	req.NoError(err)
	req.Len(cfg.Dialers, 1)
	req.Equal("eth0", cfg.Dialers[0].BindInterface)
}

func mustParseDur(req *require.Assertions, s string) time.Duration {
	d, err := time.ParseDuration(s)
	req.NoError(err)
	return d
}

func Test_Groups_UnmarshalSingleString(t *testing.T) {
	req := require.New(t)
	var g Groups
	req.NoError(g.UnmarshalJSON([]byte(`"only-one"`)))
	req.Equal(Groups{"only-one"}, g)
}

func Test_Groups_UnmarshalArray(t *testing.T) {
	req := require.New(t)
	var g Groups
	req.NoError(g.UnmarshalJSON([]byte(`["a","b","c"]`)))
	req.Equal(Groups{"a", "b", "c"}, g)
}

func Test_Groups_MarshalArray(t *testing.T) {
	req := require.New(t)
	g := Groups{"a", "b"}
	js, err := g.MarshalJSON()
	req.NoError(err)
	req.Equal(`["a","b"]`, string(js))
}
