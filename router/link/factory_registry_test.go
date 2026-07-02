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

	"github.com/openziti/channel/v5"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/router/xlink"

	"github.com/stretchr/testify/require"
)

// --- Test doubles for xlink.Factory / Listener / Dialer ---------------------

type fakeFactory struct {
	mu               sync.Mutex
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
	localBinding, _ := cfg["bindInterface"].(string)
	l := &fakeListener{bind: bind, binding: binding, localBinding: localBinding}
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
	binding, _ := cfg["bind"].(string)
	d := &fakeDialer{binding: binding}
	f.createdDialers = append(f.createdDialers, d)
	return d, nil
}

type fakeListener struct {
	bind         string
	binding      string
	localBinding string
	started      bool
	closed       bool
	listenErr    error
}

func (l *fakeListener) Listen() error             { l.started = true; return l.listenErr }
func (l *fakeListener) GetAdvertisement() string  { return l.bind }
func (l *fakeListener) GetLinkProtocol() string   { return l.binding }
func (l *fakeListener) GetLinkCostTags() []string { return nil }
func (l *fakeListener) GetGroups() []string       { return nil }
func (l *fakeListener) GetLocalBinding() string   { return l.localBinding }
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
	d.binding = d.adopted
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

func Test_FactoryRegistry_CloseUnresponsiveTimeout(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	// before any apply, falls back to the default
	req.Equal(defaultCloseUnresponsiveTimeout, r.CloseUnresponsiveTimeout())

	// an applied config with the field takes effect (for established links too)
	req.NoError(r.Apply(1, `{"heartbeats":{"closeUnresponsiveTimeout":"45s"}}`))
	req.Equal(45*time.Second, r.CloseUnresponsiveTimeout())

	// the value comes from the active config, so a later apply that omits it
	// reverts to the default
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))
	req.Equal(defaultCloseUnresponsiveTimeout, r.CloseUnresponsiveTimeout())
}

func Test_FactoryRegistry_HeartbeatIntervals(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	// before any apply, fall back to the channel defaults
	req.Equal(defaultHeartbeatSendInterval, r.SendInterval())
	req.Equal(defaultHeartbeatCheckInterval, r.CheckInterval())

	// applied intervals take effect
	req.NoError(r.Apply(1, `{"heartbeats":{"sendInterval":"3s","checkInterval":"250ms"}}`))
	req.Equal(3*time.Second, r.SendInterval())
	req.Equal(250*time.Millisecond, r.CheckInterval())

	// the settings come from the active config, so a later apply that omits
	// them reverts to the defaults
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))
	req.Equal(defaultHeartbeatSendInterval, r.SendInterval())
	req.Equal(defaultHeartbeatCheckInterval, r.CheckInterval())
}

func Test_FactoryRegistry_Remove_RevertsHeartbeatsToDefaults(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	req.NoError(r.Apply(1, `{"heartbeats":{"sendInterval":"3s","checkInterval":"250ms","closeUnresponsiveTimeout":"45s"}}`))
	req.Equal(3*time.Second, r.SendInterval())
	req.Equal(250*time.Millisecond, r.CheckInterval())
	req.Equal(45*time.Second, r.CloseUnresponsiveTimeout())

	// removing the config drops its heartbeat settings, reverting to the
	// defaults just as a later apply that omits them would
	req.NoError(r.Remove())
	req.Equal(defaultHeartbeatSendInterval, r.SendInterval())
	req.Equal(defaultHeartbeatCheckInterval, r.CheckInterval())
	req.Equal(defaultCloseUnresponsiveTimeout, r.CloseUnresponsiveTimeout())
}

func Test_FactoryRegistry_QueueSizes(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	// before any apply, fall back to the defaults
	req.Equal(defaultPayloadSenderQueueSize, r.PayloadSenderQueueSize())
	req.Equal(defaultAckSenderQueueSize, r.AckSenderQueueSize())

	// applied sizes take effect (for links established afterward)
	req.NoError(r.Apply(1, `{"payloadSenderQueueSize":256,"ackSenderQueueSize":96}`))
	req.Equal(256, r.PayloadSenderQueueSize())
	req.Equal(96, r.AckSenderQueueSize())

	// the sizes come from the active config, so a later apply that omits them
	// reverts to the defaults
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))
	req.Equal(defaultPayloadSenderQueueSize, r.PayloadSenderQueueSize())
	req.Equal(defaultAckSenderQueueSize, r.AckSenderQueueSize())
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
		"listeners": [{"bind": "tls:0.0.0.0:6262", "bindInterface":"eth0"}],
		"dialers":   [{}]
	}`
	req.NoError(r.Apply(1, data))

	req.Equal("eth0", r.Dialers()[0].GetBinding())
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

// changeRecorder captures ConfigurationChange events for assertions.
type changeRecorder struct {
	mu      sync.Mutex
	changes []ConfigurationChange
	done    chan struct{}
}

func newChangeRecorder() *changeRecorder {
	return &changeRecorder{done: make(chan struct{}, 4)}
}

func (r *changeRecorder) handle(c ConfigurationChange) {
	r.mu.Lock()
	r.changes = append(r.changes, c)
	r.mu.Unlock()
	select {
	case r.done <- struct{}{}:
	default:
	}
}

func (r *changeRecorder) snapshot() []ConfigurationChange {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ConfigurationChange, len(r.changes))
	copy(out, r.changes)
	return out
}

// waitForChange blocks until at least one change event fires or the
// timeout expires. Returns true if an event arrived.
func (r *changeRecorder) waitForChange(timeout time.Duration) bool {
	select {
	case <-r.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func Test_FactoryRegistry_ChangeHandler_FiresOnFirstApply(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"dialers":[{}]}`))
	req.True(rec.waitForChange(time.Second), "handler should fire on Apply")

	changes := rec.snapshot()
	req.Len(changes, 1)
	req.True(changes[0].ListenersChanged, "listeners went 0→1")
	req.True(changes[0].DialersChanged, "dialers went 0→1")
}

func Test_FactoryRegistry_ChangeHandler_NoFireOnIdenticalApply(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	// Prime with an initial apply (no handler).
	data := `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`
	req.NoError(r.Apply(1, data))

	// Install handler and re-apply the same data. No change → no fire.
	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)
	req.NoError(r.Apply(1, data))

	// Allow async settling.
	req.False(rec.waitForChange(150*time.Millisecond),
		"handler must not fire when listeners and dialers are unchanged")
}

func Test_FactoryRegistry_ChangeHandler_ListenersOnlyChange(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"dialers":[{}]}`))

	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	// Same dialer, different listener bind.
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6263"}],"dialers":[{}]}`))
	req.True(rec.waitForChange(time.Second))

	changes := rec.snapshot()
	req.Len(changes, 1)
	req.True(changes[0].ListenersChanged)
	req.False(changes[0].DialersChanged, "dialer slice unchanged")
}

func Test_FactoryRegistry_ChangeHandler_GcModeOnlyChange(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	// Prime with a listener and the default gc mode.
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"gcMode":"preserve"}`))

	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	// Only gcMode changes; listeners and dialers are identical. The apply must
	// still take effect (the config data differs, so it is not a no-op).
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"gcMode":"orphaned"}`))
	req.True(rec.waitForChange(time.Second), "handler should fire when only gcMode changes")

	changes := rec.snapshot()
	req.Len(changes, 1)
	req.False(changes[0].ListenersChanged, "listeners unchanged")
	req.False(changes[0].DialersChanged, "dialers unchanged")
	req.True(changes[0].GcModeChanged, "gcMode changed preserve→orphaned")
	req.Equal("orphaned", r.GetConfig().GcMode)
}

func Test_FactoryRegistry_ChangeHandler_ListenerBindInterfaceChangeAffectsDefaultDialer(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262","bindInterface":"eth0"}],"dialers":[{}]}`))

	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262","bindInterface":"eth1"}],"dialers":[{}]}`))
	req.True(rec.waitForChange(time.Second))

	changes := rec.snapshot()
	req.Len(changes, 1)
	req.True(changes[0].ListenersChanged)
	req.True(changes[0].DialersChanged, "default-adopted dialer binding changed")
	req.Equal("eth1", r.Dialers()[0].GetBinding())
}

func Test_FactoryRegistry_ChangeHandler_ExplicitDialerBindInterfaceIgnoresListenerDefault(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262","bindInterface":"eth0"}],"dialers":[{"bindInterface":"wan0"}]}`))

	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262","bindInterface":"eth1"}],"dialers":[{"bindInterface":"wan0"}]}`))
	req.True(rec.waitForChange(time.Second))

	changes := rec.snapshot()
	req.Len(changes, 1)
	req.True(changes[0].ListenersChanged)
	req.False(changes[0].DialersChanged, "explicit dialer binding should not follow listener binding")
	req.Equal("wan0", r.Dialers()[0].GetBinding())
}

func Test_FactoryRegistry_ChangeHandler_DialersOnlyChange(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"dialers":[{"groups":["a"]}]}`))

	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	// Same listener, different dialer groups.
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"dialers":[{"groups":["a","b"]}]}`))
	req.True(rec.waitForChange(time.Second))

	changes := rec.snapshot()
	req.Len(changes, 1)
	req.False(changes[0].ListenersChanged, "listener slice unchanged")
	req.True(changes[0].DialersChanged)
}

func Test_FactoryRegistry_ChangeHandler_FiresOnRemove(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"dialers":[{}]}`))

	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	req.NoError(r.Remove())
	req.True(rec.waitForChange(time.Second))

	changes := rec.snapshot()
	req.Len(changes, 1)
	req.True(changes[0].ListenersChanged, "listeners went N→0")
	req.True(changes[0].DialersChanged, "dialers went N→0")
}

func Test_FactoryRegistry_ChangeHandler_RemoveOnEmptyIsNoop(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	// Remove with nothing applied. Both sides go from nil to nil — no
	// change to publish.
	req.NoError(r.Remove())
	req.False(rec.waitForChange(150 * time.Millisecond))
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
				"binding": "transport",
				"groups":  "default", // single-string form
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

func Test_ConfigFromLocalYaml_MalformedOptionsFailsFast(t *testing.T) {
	req := require.New(t)
	yaml := LocalYamlConfig{
		Listeners: []map[interface{}]interface{}{
			{
				"binding": "transport",
				"bind":    "tls:0.0.0.0:6262",
				"options": "not-a-map", // present but wrong type
			},
		},
	}
	_, err := ConfigFromLocalYaml(yaml)
	req.Error(err, "malformed options must fail rather than be silently dropped")
	req.ErrorContains(err, "options")
}

func Test_ConfigFromLocalYaml_MalformedBackoffFailsFast(t *testing.T) {
	req := require.New(t)
	yaml := LocalYamlConfig{
		Dialers: []map[interface{}]interface{}{
			{
				"binding":            "transport",
				"healthyDialBackoff": []interface{}{"not", "a", "map"}, // present but wrong type
			},
		},
	}
	_, err := ConfigFromLocalYaml(yaml)
	req.Error(err, "malformed backoff must fail rather than be silently dropped")
	req.ErrorContains(err, "healthyDialBackoff")
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
