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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/config/routerlink"
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

func Test_Subsystem_CloseUnresponsiveTimeout(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	// before any apply, falls back to the default
	req.Equal(defaultCloseUnresponsiveTimeout, r.Heartbeats().CloseUnresponsiveTimeout)

	// an applied config with the field takes effect (for established links too)
	req.NoError(r.Apply(1, `{"heartbeats":{"closeUnresponsiveTimeout":"45s"}}`))
	req.Equal(45*time.Second, r.Heartbeats().CloseUnresponsiveTimeout)

	// the value comes from the active config, so a later apply that omits it
	// reverts to the default
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))
	req.Equal(defaultCloseUnresponsiveTimeout, r.Heartbeats().CloseUnresponsiveTimeout)
}

func Test_Subsystem_HeartbeatIntervals(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	// before any apply, fall back to the channel defaults
	req.Equal(defaultHeartbeatSendInterval, r.Heartbeats().SendInterval)
	req.Equal(defaultHeartbeatCheckInterval, r.Heartbeats().CheckInterval)

	// applied intervals take effect
	req.NoError(r.Apply(1, `{"heartbeats":{"sendInterval":"3s","checkInterval":"250ms"}}`))
	req.Equal(3*time.Second, r.Heartbeats().SendInterval)
	req.Equal(250*time.Millisecond, r.Heartbeats().CheckInterval)

	// the settings come from the active config, so a later apply that omits
	// them reverts to the defaults
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))
	req.Equal(defaultHeartbeatSendInterval, r.Heartbeats().SendInterval)
	req.Equal(defaultHeartbeatCheckInterval, r.Heartbeats().CheckInterval)
}

func Test_Subsystem_Remove_RevertsHeartbeatsToDefaults(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	req.NoError(r.Apply(1, `{"heartbeats":{"sendInterval":"3s","checkInterval":"250ms","closeUnresponsiveTimeout":"45s"}}`))
	req.Equal(3*time.Second, r.Heartbeats().SendInterval)
	req.Equal(250*time.Millisecond, r.Heartbeats().CheckInterval)
	req.Equal(45*time.Second, r.Heartbeats().CloseUnresponsiveTimeout)

	// removing the config drops its heartbeat settings, reverting to the
	// defaults just as a later apply that omits them would
	req.NoError(r.Remove())
	req.Equal(defaultHeartbeatSendInterval, r.Heartbeats().SendInterval)
	req.Equal(defaultHeartbeatCheckInterval, r.Heartbeats().CheckInterval)
	req.Equal(defaultCloseUnresponsiveTimeout, r.Heartbeats().CloseUnresponsiveTimeout)
}

func Test_Subsystem_QueueSizes(t *testing.T) {
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

func Test_Subsystem_ChangeHandler_RemoveAlwaysSweeps(t *testing.T) {
	// Remove clears the config before notifying, so a handler that read the
	// active config to find the mode would see nothing and skip the sweep,
	// leaving links up with no listeners or dialers to maintain them.
	//
	// The removed config's own mode does not govern: gcMode applies to links the
	// router's configuration made stale, and after a removal there is no
	// configuration. Preserve is no exception.
	req := require.New(t)

	for _, gcMode := range []string{"preserve", "orphaned", "changed", ""} {
		r, _ := newTestRegistry(t)
		data := fmt.Sprintf(`{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"gcMode":%q}`, gcMode)
		req.NoError(r.Apply(1, data))

		rec := newChangeRecorder()
		r.SetConfigurationChangeHandler(rec.handle)

		req.NoError(r.Remove())
		req.True(rec.waitForChange(time.Second), "removal must notify (gcMode %q)", gcMode)

		changes := rec.snapshot()
		req.Len(changes, 1)
		req.Equal(GcModeOrphaned, changes[0].GcMode,
			"a removal sweeps regardless of the removed config's gcMode %q", gcMode)
		req.Nil(r.GetConfig(), "and the config really is gone by then")
	}
}

func Test_Subsystem_ChangeHandler_CarriesAppliedGcMode(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"gcMode":"preserve"}`))

	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	// An apply is swept under the mode it brings, not the one it replaces.
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}],"gcMode":"changed"}`))
	req.True(rec.waitForChange(time.Second))
	req.Equal(GcModeChanged, rec.snapshot()[0].GcMode)
}

func Test_EffectiveGcMode(t *testing.T) {
	req := require.New(t)

	mode := func(gcMode string) *Config { return &Config{GcMode: gcMode} }

	// Apply: the incoming config decides.
	req.Equal(GcModeChanged, effectiveGcMode(mode("changed")))
	req.Equal(GcModeOrphaned, effectiveGcMode(mode("orphaned")))
	req.Equal(GcModePreserve, effectiveGcMode(mode("preserve")))
	req.Equal(GcModePreserve, effectiveGcMode(mode("")))

	// An unparseable mode must not widen what gets closed.
	req.Equal(GcModePreserve, effectiveGcMode(mode("nonsense")))

	// Remove: always sweeps, since there is no longer a configuration whose
	// policy could apply.
	req.Equal(GcModeOrphaned, effectiveGcMode(nil))
}

func Test_Config_Validate_HeartbeatTimings(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		expectErr string
	}{
		{
			name: "no heartbeats section uses defaults",
			data: `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`,
		},
		{
			name: "sane explicit timings",
			data: `{"heartbeats":{"sendInterval":"10s","checkInterval":"1s","closeUnresponsiveTimeout":"60s"}}`,
		},
		{
			// The finding's case: the check interval outruns the timeout, so the
			// pulse sends a heartbeat and condemns the link on the same tick.
			name:      "check interval past the timeout",
			data:      `{"heartbeats":{"checkInterval":"2m","closeUnresponsiveTimeout":"30s"}}`,
			expectErr: "closeUnresponsiveTimeout",
		},
		{
			name:      "timeout equal to send plus check",
			data:      `{"heartbeats":{"sendInterval":"20s","checkInterval":"10s","closeUnresponsiveTimeout":"30s"}}`,
			expectErr: "must be greater than sendInterval + checkInterval",
		},
		{
			// A timeout below the default send interval is broken even though the
			// config never names sendInterval. Caught by the floor first.
			name:      "timeout under the default send interval",
			data:      `{"heartbeats":{"closeUnresponsiveTimeout":"5s"}}`,
			expectErr: "below the 10s minimum",
		},
		{
			// Intervals tight enough that the sum rule would pass: only the
			// absolute floor catches it. A timeout this short closes links that
			// are merely recovering from ordinary packet loss.
			name:      "timeout below the floor despite roomy intervals",
			data:      `{"heartbeats":{"sendInterval":"100ms","checkInterval":"100ms","closeUnresponsiveTimeout":"1s"}}`,
			expectErr: "below the 10s minimum",
		},
		{
			name: "timeout exactly at the floor is accepted",
			data: `{"heartbeats":{"sendInterval":"5s","checkInterval":"1s","closeUnresponsiveTimeout":"10s"}}`,
		},
		{
			// Durations are int64 nanoseconds and the sum of two positive ones can
			// only wrap negative, so the sum rule would compare against a negative
			// and accept every timeout.
			name:      "interval sum overflows",
			data:      `{"heartbeats":{"sendInterval":"2562047h","checkInterval":"1h","closeUnresponsiveTimeout":"60s"}}`,
			expectErr: "exceeds the largest representable duration",
		},
		{
			// Representable, so the overflow guard stands aside and the sum rule
			// judges it.
			name:      "interval sum at the maximum is judged on its merits",
			data:      `{"heartbeats":{"sendInterval":"2562047h47m16.854775806s","checkInterval":"1ns","closeUnresponsiveTimeout":"60s"}}`,
			expectErr: "must be greater than sendInterval + checkInterval",
		},
		{
			name:      "zero check interval",
			data:      `{"heartbeats":{"checkInterval":"0s"}}`,
			expectErr: "checkInterval must be positive",
		},
		{
			name:      "negative send interval",
			data:      `{"heartbeats":{"sendInterval":"-5s"}}`,
			expectErr: "sendInterval must be positive",
		},
		{
			name:      "unparseable duration",
			data:      `{"heartbeats":{"checkInterval":"soon"}}`,
			expectErr: "invalid heartbeats.checkInterval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := require.New(t)
			cfg, err := ParseConfig(tt.data)
			req.NoError(err)

			err = cfg.Validate()
			if tt.expectErr == "" {
				req.NoError(err)
				return
			}
			req.Error(err)
			req.Contains(err.Error(), tt.expectErr)
		})
	}
}

func Test_Subsystem_Apply_RejectsUnsafeHeartbeatTimings(t *testing.T) {
	// Validation has to run on the apply path, not just as a standalone call,
	// and a rejected apply must leave the previous settings in place.
	req := require.New(t)
	r, _ := newTestRegistry(t)

	req.NoError(r.Apply(1, `{"heartbeats":{"sendInterval":"10s","checkInterval":"1s","closeUnresponsiveTimeout":"60s"}}`))
	req.Equal(time.Second, r.Heartbeats().CheckInterval)
	req.Equal(60*time.Second, r.Heartbeats().CloseUnresponsiveTimeout)

	err := r.Apply(1, `{"heartbeats":{"checkInterval":"2m","closeUnresponsiveTimeout":"30s"}}`)
	req.Error(err)
	req.Contains(err.Error(), "closeUnresponsiveTimeout")

	req.Equal(time.Second, r.Heartbeats().CheckInterval, "a rejected apply must not change the live intervals")
	req.Equal(60*time.Second, r.Heartbeats().CloseUnresponsiveTimeout)
}

func Test_HeartbeatDurations_HasThinMargin(t *testing.T) {
	req := require.New(t)

	mk := func(data string) HeartbeatDurations {
		cfg, err := ParseConfig(data)
		req.NoError(err)
		timings, err := cfg.EffectiveHeartbeats()
		req.NoError(err)
		return timings
	}

	// Defaults: 60s over a 11s healthy gap, comfortably more than one cycle.
	req.False(mk(`{}`).HasThinMargin())

	// Valid, but one lost response reaches the timeout.
	req.True(mk(`{"heartbeats":{"sendInterval":"30s","checkInterval":"5s","closeUnresponsiveTimeout":"60s"}}`).HasThinMargin())
}

func Test_Subsystem_Heartbeats_GenerationAdvancesPerApply(t *testing.T) {
	req := require.New(t)
	r, _ := newTestRegistry(t)

	// Before any apply: defaults at generation zero, so a link binding now and
	// a link binding after the first apply are distinguishable.
	initial := r.Heartbeats()
	req.Zero(initial.Generation)
	req.Equal(defaultHeartbeatSendInterval, initial.SendInterval)
	req.Equal(defaultCloseUnresponsiveTimeout, initial.CloseUnresponsiveTimeout)

	req.NoError(r.Apply(1, `{"heartbeats":{"sendInterval":"3s","checkInterval":"250ms","closeUnresponsiveTimeout":"45s"}}`))
	first := r.Heartbeats()
	req.Greater(first.Generation, initial.Generation)
	req.Equal(3*time.Second, first.SendInterval)
	req.Equal(250*time.Millisecond, first.CheckInterval)
	req.Equal(45*time.Second, first.CloseUnresponsiveTimeout)

	// A later apply that omits them reverts to defaults, as a new generation.
	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))
	second := r.Heartbeats()
	req.Greater(second.Generation, first.Generation)
	req.Equal(defaultHeartbeatSendInterval, second.SendInterval)

	// Remove also publishes a generation, so links are re-tuned off the removed
	// values rather than keeping them.
	req.NoError(r.Remove())
	third := r.Heartbeats()
	req.Greater(third.Generation, second.Generation)
	req.Equal(defaultCloseUnresponsiveTimeout, third.CloseUnresponsiveTimeout)
}

func Test_Subsystem_Heartbeats_SnapshotIsCoherent(t *testing.T) {
	// The point of one atomic swap: a reader can never see one generation's
	// interval with another's timeout, however many applies race with it.
	req := require.New(t)
	r, _ := newTestRegistry(t)

	configs := []string{
		`{"heartbeats":{"sendInterval":"1s","checkInterval":"100ms","closeUnresponsiveTimeout":"10s"}}`,
		`{"heartbeats":{"sendInterval":"20s","checkInterval":"5s","closeUnresponsiveTimeout":"5m"}}`,
	}
	valid := map[xlink.HeartbeatSettings]bool{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = r.Apply(1, configs[i%len(configs)])
		}
	}()

	for i := 0; i < 2000; i++ {
		s := r.Heartbeats()
		// Every observed snapshot must be one of the coherent combinations,
		// never a mix.
		coherent := (s.SendInterval == time.Second && s.CheckInterval == 100*time.Millisecond && s.CloseUnresponsiveTimeout == 10*time.Second) ||
			(s.SendInterval == 20*time.Second && s.CheckInterval == 5*time.Second && s.CloseUnresponsiveTimeout == 5*time.Minute) ||
			(s.SendInterval == defaultHeartbeatSendInterval && s.CheckInterval == defaultHeartbeatCheckInterval && s.CloseUnresponsiveTimeout == defaultCloseUnresponsiveTimeout)
		req.True(coherent, "observed a mixed snapshot: %+v", s)
		valid[s] = true
	}
	<-done
	req.NotEmpty(valid)
}

func Test_Subsystem_ListenerSnapshot_PairsGenerationWithItsListeners(t *testing.T) {
	// Sampling the generation and the listeners separately would let a publisher
	// stamp one generation's listeners with another's number, which a receiver
	// discriminating on generation would then accept as the newest state.
	req := require.New(t)
	r, _ := newTestRegistry(t)

	gen0, listeners0 := r.ListenerSnapshot()
	req.Zero(gen0)
	req.Empty(listeners0)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`))
	gen1, listeners1 := r.ListenerSnapshot()
	req.Greater(gen1, gen0)
	req.Len(listeners1, 1)

	req.NoError(r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6263"},{"bind":"tls:0.0.0.0:6264"}]}`))
	gen2, listeners2 := r.ListenerSnapshot()
	req.Greater(gen2, gen1)
	req.Len(listeners2, 2)

	// Under concurrent applies, every snapshot must be a real pairing: the
	// two-listener set never appears stamped with the one-listener generation.
	seen := map[uint64]int{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			if i%2 == 0 {
				_ = r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6262"}]}`)
			} else {
				_ = r.Apply(1, `{"listeners":[{"bind":"tls:0.0.0.0:6263"},{"bind":"tls:0.0.0.0:6264"}]}`)
			}
		}
	}()

	for i := 0; i < 2000; i++ {
		gen, listeners := r.ListenerSnapshot()
		if prev, found := seen[gen]; found {
			req.Equal(prev, len(listeners),
				"generation %d reported %d listeners and then %d", gen, prev, len(listeners))
		}
		seen[gen] = len(listeners)
	}
	<-done
	req.NotEmpty(seen)
}

func Test_LocalYaml_ClampsTimeoutBelowFloor(t *testing.T) {
	// Local link config is applied synchronously and a failure stops the router
	// from starting, so a below-floor value from router YAML is raised to the
	// floor rather than rejected. The managed path rejects it instead, where the
	// operator sees the error on the API call and nothing is already running.
	req := require.New(t)

	hb := heartbeatsFromYamlOptions(channel.HeartbeatOptions{
		SendInterval:             time.Second,
		CheckInterval:            time.Second,
		CloseUnresponsiveTimeout: 5 * time.Second,
	})
	req.NotNil(hb)
	req.Equal(routerlink.MinCloseUnresponsiveTimeout.String(), hb.CloseUnresponsiveTimeout)

	// And the clamped result must survive the validation it was clamped to pass.
	cfg := &Config{Heartbeats: hb}
	req.NoError(cfg.Validate())

	// A value at or above the floor is passed through untouched.
	hb = heartbeatsFromYamlOptions(channel.HeartbeatOptions{
		SendInterval:             10 * time.Second,
		CheckInterval:            time.Second,
		CloseUnresponsiveTimeout: 45 * time.Second,
	})
	req.NotNil(hb)
	req.Equal("45s", hb.CloseUnresponsiveTimeout)
}

func Test_LocalYaml_InvalidTimingFallsBackToDefaults(t *testing.T) {
	// The env loader fills absent fields with non-zero defaults, so every input
	// here is timing an operator wrote. Local link config failing validation
	// stops the router from starting, and the send and check intervals were once
	// ignored for links entirely, so timing that was never in force must not
	// start blocking boot on upgrade: it falls back to the defaults instead.
	cases := []struct {
		name string
		in   channel.HeartbeatOptions
	}{
		{
			// Kept as "0s" so validation sees it, as the controller does, rather than
			// read as an absent field and quietly defaulted.
			name: "explicit zero check interval",
			in:   channel.HeartbeatOptions{SendInterval: 10 * time.Second, CheckInterval: 0, CloseUnresponsiveTimeout: time.Minute},
		},
		{
			name: "every field explicitly zero",
			in:   channel.HeartbeatOptions{},
		},
		{
			// Ignored before these settings reached links, and so harmless then.
			name: "check interval past the timeout",
			in:   channel.HeartbeatOptions{SendInterval: 10 * time.Second, CheckInterval: 2 * time.Minute, CloseUnresponsiveTimeout: 30 * time.Second},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := require.New(t)
			req.Nil(heartbeatsFromYamlOptions(c.in), "invalid local timing must yield the defaults")

			// And the router must still start: the translated config validates.
			js, err := ConfigFromLocalYaml(LocalYamlConfig{
				Listeners:  []map[interface{}]interface{}{{"binding": "transport", "bind": "tls:0.0.0.0:6262"}},
				Heartbeats: c.in,
			})
			req.NoError(err)
			cfg, err := ParseConfig(js)
			req.NoError(err)
			req.NoError(cfg.Validate())
			timings, err := cfg.EffectiveHeartbeats()
			req.NoError(err)
			req.Equal(routerlink.DefaultCloseUnresponsiveTimeout, timings.CloseUnresponsiveTimeout)
			req.Equal(routerlink.DefaultHeartbeatCheckInterval, timings.CheckInterval)
		})
	}
}

func Test_LocalYaml_ValidTimingPassesThrough(t *testing.T) {
	req := require.New(t)
	hb := heartbeatsFromYamlOptions(channel.HeartbeatOptions{
		SendInterval:             5 * time.Second,
		CheckInterval:            500 * time.Millisecond,
		CloseUnresponsiveTimeout: 20 * time.Second,
	})
	req.NotNil(hb)
	req.Equal(&HeartbeatsConfig{SendInterval: "5s", CheckInterval: "500ms", CloseUnresponsiveTimeout: "20s"}, hb)
}

func Test_Subsystem_QueueSizeOnlyChange_AppliesWithoutDispatching(t *testing.T) {
	// Queue sizes are read when a link is created, so a change to them reaches
	// new links only. There is nothing to do for established links and nothing
	// about them has become stale, so the change must not reach the handler:
	// that would run a GC pass and, under a non-preserve gcMode, close healthy
	// links to resize a buffer.
	//
	// This is emergent from which flags hasWork consults, so it is pinned here
	// rather than left for someone adding a flag to rediscover.
	req := require.New(t)
	r, _ := newTestRegistry(t)

	const listener = `{"bind":"tls:0.0.0.0:6262"}`
	req.NoError(r.Apply(1, fmt.Sprintf(`{"listeners":[%s],"gcMode":"changed","payloadSenderQueueSize":128}`, listener)))

	rec := newChangeRecorder()
	r.SetConfigurationChangeHandler(rec.handle)

	req.NoError(r.Apply(1, fmt.Sprintf(`{"listeners":[%s],"gcMode":"changed","payloadSenderQueueSize":512,"ackSenderQueueSize":256}`, listener)))

	req.False(rec.waitForChange(250*time.Millisecond),
		"a queue-size-only change must not dispatch, or a tuning edit would close healthy links")

	// Applied all the same: the next link built picks the new sizes up.
	req.Equal(512, r.PayloadSenderQueueSize())
	req.Equal(256, r.AckSenderQueueSize())
	req.Equal(512, r.GetConfig().PayloadSenderQueueSize)
}

func Test_ConfigFromLocalYaml_LinkSectionWithoutSurfaceIsLocalConfig(t *testing.T) {
	// A link section of any content takes precedence over the controller's
	// router.link config, including one that only tunes heartbeats, so it is applied
	// as local config rather than dropped.
	req := require.New(t)

	js, err := ConfigFromLocalYaml(LocalYamlConfig{
		Configured: true,
		Heartbeats: channel.HeartbeatOptions{
			SendInterval:             5 * time.Second,
			CheckInterval:            time.Second,
			CloseUnresponsiveTimeout: 45 * time.Second,
		},
		PayloadSenderQueueSize: 256,
		AckSenderQueueSize:     64,
	})
	req.NoError(err)
	req.NotEmpty(js, "a link section must count as local config even with no listeners or dialers")

	cfg, err := ParseConfig(js)
	req.NoError(err)
	req.NoError(cfg.Validate())
	req.Empty(cfg.Listeners)
	req.Empty(cfg.Dialers)
	req.Equal("45s", cfg.Heartbeats.CloseUnresponsiveTimeout)
	req.Equal(256, cfg.PayloadSenderQueueSize)
}
