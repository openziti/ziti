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
	"github.com/openziti/foundation/v2/util"
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

func (l *fakeListener) Listen() error            { l.started = true; return l.listenErr }
func (l *fakeListener) GetAdvertisement() string { return l.bind }
func (l *fakeListener) GetLinkProtocol() string  { return l.binding }
func (l *fakeListener) GetGroups() []string      { return nil }
func (l *fakeListener) GetLocalBinding() string  { return l.localBinding }
func (l *fakeListener) Close() error             { l.closed = true; return nil }

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

func newTestRegistry(t *testing.T) (*Subsystem, *fakeFactory) {
	t.Helper()
	r := NewSubsystem(mustTokenId(t))
	f := &fakeFactory{}
	require.NoError(t, r.Register("transport", f))
	return r, f
}

// --- Tests ------------------------------------------------------------------

func Test_Subsystem_Register_RejectsDifferentFactoryForSameBinding(t *testing.T) {
	req := require.New(t)
	r := NewSubsystem(mustTokenId(t))
	f1 := &fakeFactory{}
	req.NoError(r.Register("transport", f1))
	// Same factory re-registered: no-op.
	req.NoError(r.Register("transport", f1))
	// Different factory for same binding: error.
	f2 := &fakeFactory{}
	req.Error(r.Register("transport", f2))
}

func Test_Subsystem_Apply_BuildsListenersAndDialers(t *testing.T) {
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

func Test_Subsystem_Apply_MapsDialerBindInterfaceToTransportBind(t *testing.T) {
	req := require.New(t)
	r, f := newTestRegistry(t)

	data := `{"dialers":[{"bindInterface":"eth0"}]}`
	req.NoError(r.Apply(1, data))

	req.Len(f.dialerConfigs, 1)
	req.Equal("eth0", f.dialerConfigs[0]["bind"])
	req.NotContains(f.dialerConfigs[0], "bindInterface")
}

func Test_Subsystem_Apply_AdoptsBindingWhenSingleListenerAndDialer(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	data := `{
		"listeners": [{"bind": "tls:0.0.0.0:6262", "bindInterface":"eth0"}],
		"dialers":   [{}]
	}`
	req.NoError(r.Apply(1, data))

	req.Equal("eth0", r.Dialers()[0].GetBinding())
}

func Test_Subsystem_Apply_ClosesOldListenersOnRebuild(t *testing.T) {
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

func Test_Subsystem_Apply_UnknownBinding(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	err := r.Apply(1, `{"listeners":[{"binding":"made-up","bind":"x"}]}`)
	req.Error(err)
	req.Empty(r.Listeners())
}

func Test_Subsystem_Apply_MalformedJson(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	err := r.Apply(1, `{not json`)
	req.Error(err)
}

func Test_Subsystem_Apply_UnsupportedVersion(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)
	err := r.Apply(2, `{}`)
	req.Error(err)
}

func Test_Subsystem_Apply_ListenerCreateError_LeavesStateUnchanged(t *testing.T) {
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

func Test_Subsystem_Remove_TearsDown(t *testing.T) {
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

func Test_Subsystem_ChangeHandler_FiresOnFirstApply(t *testing.T) {
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

func Test_Subsystem_ChangeHandler_NoFireOnIdenticalApply(t *testing.T) {
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

func Test_Subsystem_ChangeHandler_ListenersOnlyChange(t *testing.T) {
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

func Test_Subsystem_ChangeHandler_GcModeOnlyChange(t *testing.T) {
	req := require.New(t)
	r, f := newTestRegistry(t)

	// Prime with a listener and the default gc mode.
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"gcMode":"preserve"}`))
	req.Len(f.createdListeners, 1)
	original := f.createdListeners[0]

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

	// Rebuilding here would rebind the listen socket and drop half-established
	// links for nothing.
	req.Len(f.createdListeners, 1, "gcMode change must not build a replacement listener")
	req.False(original.closed, "gcMode change must not close the running listener")
	req.Equal([]xlink.Listener{original}, r.Listeners(), "the same listener instance must still be live")
}

func Test_Subsystem_Apply_ListenerChangeStillRebuilds(t *testing.T) {
	// Guards the other direction: the narrowed rebuild condition must not make
	// real listener changes inert.
	req := require.New(t)
	r, f := newTestRegistry(t)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"gcMode":"orphaned"}`))
	req.Len(f.createdListeners, 1)
	original := f.createdListeners[0]

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6263"}],"gcMode":"orphaned"}`))
	req.Len(f.createdListeners, 2, "a changed bind address must build a replacement listener")
	req.True(original.closed, "the superseded listener must be closed")
	req.True(f.createdListeners[1].started, "the replacement listener must be started")
}

func Test_Subsystem_ChangeHandler_ListenerBindInterfaceChangeAffectsDefaultDialer(t *testing.T) {
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

func Test_Subsystem_ChangeHandler_ExplicitDialerBindInterfaceIgnoresListenerDefault(t *testing.T) {
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

func Test_Subsystem_ChangeHandler_DialersOnlyChange(t *testing.T) {
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

func Test_Subsystem_ChangeHandler_FiresOnRemove(t *testing.T) {
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

func Test_Subsystem_ChangeHandler_RemoveOnEmptyIsNoop(t *testing.T) {
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

func Test_Subsystem_AccessorsReturnSnapshots(t *testing.T) {
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

func Test_channelOptionsToMap_LoadsIntoChannelOptions(t *testing.T) {
	req := require.New(t)

	out := channelOptionsToMap(&ChannelOptions{
		OutQueueSize:           util.Ptr(16),
		MaxQueuedConnects:      util.Ptr(4),
		MaxOutstandingConnects: 8,
		ConnectTimeout:         "30s",
		WriteTimeout:           "5s",
	})

	// The emitted map must be consumable by the real channel loader in its
	// canonical form (int outQueueSize, connectTimeout as a duration string).
	opts := channel.DefaultOptions()
	req.NoError(opts.Load(out))

	req.Equal(16, opts.OutQueueSize)
	req.Equal(4, opts.MaxQueuedConnects)
	req.Equal(8, opts.MaxOutstandingConnects)
	req.Equal(30*time.Second, opts.ConnectTimeout)
	req.Equal(5*time.Second, opts.WriteTimeout)
}

func Test_DialerConfig_ExplicitZeroMaxAckConnectionsSurvives(t *testing.T) {
	req := require.New(t)

	// An explicit maxAckConnections:0 (no dedicated ack underlay) must survive
	// translation rather than being dropped and defaulted back to 1.
	js, err := ConfigFromLocalYaml(LocalYamlConfig{
		Dialers: []map[interface{}]interface{}{
			{"binding": "transport", "maxAckConnections": 0},
		},
	})
	req.NoError(err)

	cfg, err := ParseConfig(js)
	req.NoError(err)
	req.Len(cfg.Dialers, 1)
	req.NotNil(cfg.Dialers[0].MaxAckConnections, "explicit maxAckConnections:0 must be preserved, not dropped")
	req.Equal(0, *cfg.Dialers[0].MaxAckConnections)

	// The transport map carries the explicit 0 as an int.
	m := dialerConfigToMap(&cfg.Dialers[0])
	req.Contains(m, "maxAckConnections")
	req.Equal(0, m["maxAckConnections"])

	// A dialer that omits the field leaves it nil and the key absent, so the
	// transport applies its own default.
	omitted := dialerConfigToMap(&DialerConfig{Binding: "transport"})
	req.NotContains(omitted, "maxAckConnections")
}

func Test_DialerConfig_ExplicitSplitFalseSurvives(t *testing.T) {
	req := require.New(t)

	// An explicit split:false in local YAML must survive translation rather than
	// being dropped and reverting to the transport's split=true default.
	js, err := ConfigFromLocalYaml(LocalYamlConfig{
		Dialers: []map[interface{}]interface{}{
			{"binding": "transport", "split": false},
		},
	})
	req.NoError(err)

	cfg, err := ParseConfig(js)
	req.NoError(err)
	req.Len(cfg.Dialers, 1)
	req.NotNil(cfg.Dialers[0].Split, "explicit split:false must be preserved, not dropped")
	req.False(*cfg.Dialers[0].Split)

	// The transport map carries the explicit false as a bool.
	m := dialerConfigToMap(&cfg.Dialers[0])
	req.Contains(m, "split")
	req.Equal(false, m["split"])

	// A dialer that omits split leaves it nil and the key absent, so the
	// transport applies its own default.
	omitted := dialerConfigToMap(&DialerConfig{Binding: "transport"})
	req.NotContains(omitted, "split")
}

func Test_Config_Validate_RejectsBadConnectTimeout(t *testing.T) {
	req := require.New(t)

	// Anything below the channel minimum is rejected, including sub-millisecond
	// values.
	subMs := &Config{Dialers: []DialerConfig{{Options: &ChannelOptions{ConnectTimeout: "500us"}}}}
	req.Error(subMs.Validate())

	// Anything below the channel minimum is rejected.
	belowMin := &Config{Listeners: []ListenerConfig{{Options: &ChannelOptions{ConnectTimeout: "5ms"}}}}
	req.Error(belowMin.Validate())

	// Anything above the channel maximum is rejected.
	aboveMax := &Config{Dialers: []DialerConfig{{Options: &ChannelOptions{ConnectTimeout: "90s"}}}}
	req.Error(aboveMax.Validate())

	// Malformed durations fail rather than being silently dropped.
	malformed := &Config{Dialers: []DialerConfig{{Options: &ChannelOptions{ConnectTimeout: "nope"}}}}
	req.Error(malformed.Validate())

	// A valid value and an absent value both pass.
	req.NoError((&Config{Dialers: []DialerConfig{{Options: &ChannelOptions{ConnectTimeout: "5s"}}}}).Validate())
	req.NoError((&Config{Dialers: []DialerConfig{{Binding: "transport"}}}).Validate())
}

func Test_ConfigFromLocalYaml_RejectsFractionalInts(t *testing.T) {
	req := require.New(t)

	// A fractional integer field is rejected rather than truncated.
	_, err := ConfigFromLocalYaml(LocalYamlConfig{
		Dialers: []map[interface{}]interface{}{{"maxAckConnections": 0.5}},
	})
	req.Error(err)

	_, err = ConfigFromLocalYaml(LocalYamlConfig{
		Listeners: []map[interface{}]interface{}{
			{"bind": "tls:0.0.0.0:6262", "options": map[interface{}]interface{}{"outQueueSize": 1.9}},
		},
	})
	req.Error(err)

	// An integral float (as YAML/JSON may produce) is accepted.
	_, err = ConfigFromLocalYaml(LocalYamlConfig{
		Dialers: []map[interface{}]interface{}{{"maxAckConnections": float64(4)}},
	})
	req.NoError(err)
}

func Test_ConfigFromLocalYaml_RejectsNonPositiveValues(t *testing.T) {
	req := require.New(t)

	// Values that the config-to-map translation drops must be reported rather
	// than silently replaced by the subsystem default.
	for _, tc := range []struct {
		name string
		cfg  LocalYamlConfig
	}{
		{"maxDefaultConnections zero", LocalYamlConfig{
			Dialers: []map[interface{}]interface{}{{"maxDefaultConnections": 0}},
		}},
		{"maxDefaultConnections negative", LocalYamlConfig{
			Dialers: []map[interface{}]interface{}{{"maxDefaultConnections": -1}},
		}},
		{"outQueueSize negative", LocalYamlConfig{
			Listeners: []map[interface{}]interface{}{
				{"bind": "tls:0.0.0.0:6262", "options": map[interface{}]interface{}{"outQueueSize": -1}},
			},
		}},
		{"maxQueuedConnects negative", LocalYamlConfig{
			Listeners: []map[interface{}]interface{}{
				{"bind": "tls:0.0.0.0:6262", "options": map[interface{}]interface{}{"maxQueuedConnects": -1}},
			},
		}},
		{"maxOutstandingConnects zero", LocalYamlConfig{
			Listeners: []map[interface{}]interface{}{
				{"bind": "tls:0.0.0.0:6262", "options": map[interface{}]interface{}{"maxOutstandingConnects": 0}},
			},
		}},
		{"maxOutstandingConnects negative", LocalYamlConfig{
			Listeners: []map[interface{}]interface{}{
				{"bind": "tls:0.0.0.0:6262", "options": map[interface{}]interface{}{"maxOutstandingConnects": -5}},
			},
		}},
		{"retryBackoffFactor zero", LocalYamlConfig{
			Dialers: []map[interface{}]interface{}{
				{"binding": "transport", "healthyDialBackoff": map[interface{}]interface{}{"retryBackoffFactor": 0}},
			},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ConfigFromLocalYaml(tc.cfg)
			require.Error(t, err)
		})
	}

	// maxAckConnections is a pointer field and 0 is a valid setting for it, so
	// it must still round-trip rather than be caught by the positive check.
	js, err := ConfigFromLocalYaml(LocalYamlConfig{
		Dialers: []map[interface{}]interface{}{{"binding": "transport", "maxAckConnections": 0}},
	})
	req.NoError(err)
	cfg, err := ParseConfig(js)
	req.NoError(err)
	req.NotNil(cfg.Dialers[0].MaxAckConnections)
	req.Equal(0, *cfg.Dialers[0].MaxAckConnections)

	// Absent keys stay absent; the positive check only fires on explicit values.
	_, err = ConfigFromLocalYaml(LocalYamlConfig{
		Dialers: []map[interface{}]interface{}{{"binding": "transport"}},
	})
	req.NoError(err)
}

func Test_ConfigFromLocalYaml_PreservesZeroQueueSizes(t *testing.T) {
	req := require.New(t)

	// Zero requests an unbuffered channel and is accepted by the channel
	// loader, so it must survive translation rather than being dropped and
	// replaced by the default.
	js, err := ConfigFromLocalYaml(LocalYamlConfig{
		Listeners: []map[interface{}]interface{}{
			{"bind": "tls:0.0.0.0:6262", "options": map[interface{}]interface{}{
				"outQueueSize":      0,
				"maxQueuedConnects": 0,
			}},
		},
	})
	req.NoError(err)

	cfg, err := ParseConfig(js)
	req.NoError(err)
	req.NotNil(cfg.Listeners[0].Options)
	req.NotNil(cfg.Listeners[0].Options.OutQueueSize)
	req.Equal(0, *cfg.Listeners[0].Options.OutQueueSize)
	req.NotNil(cfg.Listeners[0].Options.MaxQueuedConnects)
	req.Equal(0, *cfg.Listeners[0].Options.MaxQueuedConnects)

	// ...and the emitted map must carry the zero through to the real loader,
	// leaving it unbuffered rather than at the default.
	out := channelOptionsToMap(cfg.Listeners[0].Options)
	req.Equal(0, out["outQueueSize"])
	opts := channel.DefaultOptions()
	req.NoError(opts.Load(out))
	req.Equal(0, opts.OutQueueSize)
	req.Equal(0, opts.MaxQueuedConnects)

	// An absent options block leaves both unset so defaults still apply.
	js, err = ConfigFromLocalYaml(LocalYamlConfig{
		Listeners: []map[interface{}]interface{}{{"bind": "tls:0.0.0.0:6262"}},
	})
	req.NoError(err)
	cfg, err = ParseConfig(js)
	req.NoError(err)
	req.Nil(cfg.Listeners[0].Options)
}

func Test_ConfigFromLocalYaml_LegacyConnectTimeoutMs(t *testing.T) {
	req := require.New(t)

	// A local config using the legacy connectTimeoutMs key must keep its timeout
	// rather than being silently dropped to the channel default.
	js, err := ConfigFromLocalYaml(LocalYamlConfig{
		Listeners: []map[interface{}]interface{}{
			{"bind": "tls:0.0.0.0:6262", "options": map[interface{}]interface{}{"connectTimeoutMs": 30000}},
		},
	})
	req.NoError(err)
	cfg, err := ParseConfig(js)
	req.NoError(err)
	req.NotNil(cfg.Listeners[0].Options)
	req.Equal("30s", cfg.Listeners[0].Options.ConnectTimeout)

	// connectTimeout (duration string) wins when both keys are present.
	js, err = ConfigFromLocalYaml(LocalYamlConfig{
		Dialers: []map[interface{}]interface{}{
			{"binding": "transport", "options": map[interface{}]interface{}{
				"connectTimeout":   "10s",
				"connectTimeoutMs": 30000,
			}},
		},
	})
	req.NoError(err)
	cfg, err = ParseConfig(js)
	req.NoError(err)
	req.Equal("10s", cfg.Dialers[0].Options.ConnectTimeout)
}

func Test_getEffectiveDialers_ImplicitDialerFollowsListenerBind(t *testing.T) {
	req := require.New(t)

	// One listener (no bindInterface) plus one implicit dialer: the effective
	// dialer binding must track the listener's bind address, so a bind change to a
	// different interface registers as a dialer change.
	mk := func(bind string) *Config {
		return &Config{
			Listeners: []ListenerConfig{{Bind: bind}},
			Dialers:   []DialerConfig{{Binding: "transport"}},
		}
	}
	a := getEffectiveDialers(mk("tls:10.0.0.1:6262"))
	b := getEffectiveDialers(mk("tls:10.0.1.1:6262"))
	req.False(dialerSlicesEqual(a, b), "implicit dialer must change when the listener bind changes")

	// An explicit dialer bindInterface is not overridden by the listener bind.
	explicit := &Config{
		Listeners: []ListenerConfig{{Bind: "tls:10.0.0.1:6262"}},
		Dialers:   []DialerConfig{{Binding: "transport", BindInterface: "eth9"}},
	}
	req.Equal("eth9", getEffectiveDialers(explicit)[0].BindInterface)
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
	req.NotNil(l.Options.OutQueueSize)
	req.Equal(16, *l.Options.OutQueueSize)
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
