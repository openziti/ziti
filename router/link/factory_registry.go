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
	"reflect"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/router/xlink"
)

// defaultCloseUnresponsiveTimeout is the fallback used when no link config has
// supplied heartbeats.closeUnresponsiveTimeout. It matches the router env
// default (DefaultLinkUnresponsiveTimeout) so behavior is unchanged when the
// field is absent.
const defaultCloseUnresponsiveTimeout = time.Minute

// Heartbeat interval fallbacks, matching the channel package defaults, used
// when no link config supplies heartbeats.sendInterval / checkInterval.
const (
	defaultHeartbeatSendInterval  = 10 * time.Second
	defaultHeartbeatCheckInterval = time.Second
)

// Link sender queue size fallbacks, matching the router env defaults
// (DefaultLinkPayloadSenderQueueSize / DefaultLinkAckSenderQueueSize), used
// when no link config supplies them.
const (
	defaultPayloadSenderQueueSize = 128
	defaultAckSenderQueueSize     = 64
)

// FactoryRegistry owns the link subsystem's configurable surface: the set of
// xlink.Factory implementations registered for each binding name, the
// currently-applied router.link.v1 config, and the listener / dialer
// instances built from it.
//
// Implements managedconfig.ConfigHandler. Registered with the router's
// configRegistry before Seal; thereafter Apply / Remove drive the link
// subsystem entirely.
//
// Concurrency model:
//   - factories is set once (during pre-Seal registration) and read-only
//     thereafter; protected by mu for the registration phase only.
//   - config, listeners, and dialers are read/written under mu. Apply /
//     Remove serialize via the managedconfig.Registry's per-handler lock,
//     so they don't race with each other; accessor methods (Listeners,
//     Dialers, GetConfig) take RLock and return slice copies for safe
//     iteration by callers outside the handler.
type FactoryRegistry struct {
	routerId *identity.TokenId

	mu        sync.RWMutex
	factories map[string]xlink.Factory
	config    *Config
	// appliedData is the raw JSON of the currently-applied config, kept so
	// Apply can detect a redundant re-apply (notably rollback, which replays
	// the previous data verbatim) by comparing the incoming data against it.
	appliedData string
	listeners   []xlink.Listener
	dialers     []xlink.Dialer

	// closeUnresponsiveTimeout, sendInterval, and checkInterval hold the
	// effective heartbeat settings from the applied config, read by each link's
	// heartbeat wiring off the per-link hot path. Updated on Apply; the zero
	// value (returned when unset) means "use the default".
	closeUnresponsiveTimeout concurrenz.AtomicValue[time.Duration]
	sendInterval             concurrenz.AtomicValue[time.Duration]
	checkInterval            concurrenz.AtomicValue[time.Duration]

	changeHandler ConfigurationChangeHandler
}

// CloseUnresponsiveTimeout returns the effective link heartbeat
// close-unresponsive timeout. Lock-free so the per-link heartbeat callback can
// read it on every check. Falls back to defaultCloseUnresponsiveTimeout until a
// config with the field has been applied.
func (self *FactoryRegistry) CloseUnresponsiveTimeout() time.Duration {
	if v := self.closeUnresponsiveTimeout.Load(); v > 0 {
		return v
	}
	return defaultCloseUnresponsiveTimeout
}

// SendInterval returns the effective heartbeat send interval, falling back to
// the default until a config supplies it.
func (self *FactoryRegistry) SendInterval() time.Duration {
	if v := self.sendInterval.Load(); v > 0 {
		return v
	}
	return defaultHeartbeatSendInterval
}

// CheckInterval returns the effective heartbeat check interval, falling back to
// the default until a config supplies it.
func (self *FactoryRegistry) CheckInterval() time.Duration {
	if v := self.checkInterval.Load(); v > 0 {
		return v
	}
	return defaultHeartbeatCheckInterval
}

// PayloadSenderQueueSize returns the effective link payload sender queue size
// from the applied config, falling back to the default when unset. Read when a
// link channel is built, so a change takes effect on links established after
// it; existing links keep the size they were built with.
func (self *FactoryRegistry) PayloadSenderQueueSize() int {
	self.mu.RLock()
	defer self.mu.RUnlock()
	if self.config != nil && self.config.PayloadSenderQueueSize > 0 {
		return self.config.PayloadSenderQueueSize
	}
	return defaultPayloadSenderQueueSize
}

// AckSenderQueueSize returns the effective link ack sender queue size from the
// applied config, falling back to the default when unset. Like
// PayloadSenderQueueSize, it applies to links established after a change.
func (self *FactoryRegistry) AckSenderQueueSize() int {
	self.mu.RLock()
	defer self.mu.RUnlock()
	if self.config != nil && self.config.AckSenderQueueSize > 0 {
		return self.config.AckSenderQueueSize
	}
	return defaultAckSenderQueueSize
}

// applyHeartbeatDuration parses a heartbeat duration string from config and
// stores it (in nanos) for live reads. An empty value is left untouched so the
// accessor's default applies; a malformed value (which the schema should
// prevent) is logged and the previous value kept.
func (self *FactoryRegistry) applyHeartbeatDuration(value string, target *concurrenz.AtomicValue[time.Duration], name string) {
	if value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			target.Store(d)
			return
		} else {
			pfxlog.Logger().WithError(err).WithField("value", value).
				Errorf("invalid heartbeats.%s; using default", name)
		}
	}
	// The applied config is authoritative: an absent (or invalid) value reverts
	// to the default rather than retaining a previously-applied one.
	target.Store(0)
}

// ConfigurationChange describes which parts of the link configuration
// changed during an Apply / Remove. Any flag may be true; consumers
// inspect them to decide what work to do.
type ConfigurationChange struct {
	// ListenersChanged is true when the listener set differs from the
	// previous state (membership and/or any listener field that affects
	// what peers should know — bind address, advertise, groups, etc.).
	ListenersChanged bool
	// DialersChanged is true when the dialer set differs from the
	// previous state, including dialer groups and effective local
	// binding. The effective local binding includes the single
	// listener/single dialer default adoption rule.
	DialersChanged bool
	// GcModeChanged is true when the gcMode field transitioned. The
	// handler should consult the current config for the new mode.
	GcModeChanged bool
	// HeartbeatsChanged is true when any heartbeat field (send/check interval
	// or close-unresponsive timeout) differs from the previous state. The
	// handler retunes established links' heartbeat intervals in response.
	HeartbeatsChanged bool
}

// ConfigurationChangeHandler is invoked asynchronously after a successful
// Apply or Remove when the listener and/or dialer set actually changed.
// Consumers inspect the change flags to decide what to do (e.g., publish
// new listeners to the controller, re-evaluate dial opportunities).
type ConfigurationChangeHandler func(change ConfigurationChange)

// SetConfigurationChangeHandler installs a callback that fires after each successful
// Apply or Remove with non-trivial state changes. Pass nil to unset.
// Safe to call concurrent with Apply / Remove; the handler is invoked
// off the apply path so handler work doesn't block reconcile.
func (self *FactoryRegistry) SetConfigurationChangeHandler(h ConfigurationChangeHandler) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.changeHandler = h
}

// NewFactoryRegistry constructs an empty registry. Caller registers
// factories before the managedconfig registry is sealed.
func NewFactoryRegistry(routerId *identity.TokenId) *FactoryRegistry {
	return &FactoryRegistry{
		routerId:  routerId,
		factories: map[string]xlink.Factory{},
	}
}

// Register binds an xlink.Factory to a binding name. Called during router
// init for the built-in transport factory and any plugin factories.
// Returns an error if a different factory is already registered for that
// binding; same-factory re-register is a no-op.
func (self *FactoryRegistry) Register(binding string, factory xlink.Factory) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if existing, ok := self.factories[binding]; ok && existing != factory {
		return fmt.Errorf("factory already registered for binding %q", binding)
	}
	self.factories[binding] = factory
	return nil
}

// Factory looks up the registered factory for a binding name.
func (self *FactoryRegistry) Factory(binding string) (xlink.Factory, bool) {
	self.mu.RLock()
	defer self.mu.RUnlock()
	f, ok := self.factories[binding]
	return f, ok
}

// BaseType implements managedconfig.ConfigHandler.
func (self *FactoryRegistry) BaseType() string { return ConfigBaseType }

// SupportedVersions implements managedconfig.ConfigHandler.
func (self *FactoryRegistry) SupportedVersions() []int { return []int{1} }

// Apply implements managedconfig.ConfigHandler. Parses the JSON payload,
// closes every current listener (safe mid-run: accepted Xlinks survive as
// independent channels), replaces the dialer slice with fresh instances, and
// starts new listeners from the new config.
//
// On error, the registry's transition matrix may roll back by calling
// Apply with the previously-applied data; that's a separate Apply call so
// no special "are we rolling back?" handling is needed here.
func (self *FactoryRegistry) Apply(version int, data string) error {
	if version != 1 {
		return fmt.Errorf("unsupported %s version %d", ConfigBaseType, version)
	}

	cfg, err := ParseConfig(data)
	if err != nil {
		return fmt.Errorf("parse %s.v%d: %w", ConfigBaseType, version, err)
	}

	newListeners, newDialers, err := self.build(cfg)
	if err != nil {
		return err
	}

	self.mu.Lock()
	prevConfig := self.config
	// Re-applying the current config is a no-op: leave the running listeners in
	// place rather than tearing them down and rebuilding them. This matters for
	// rollback, where a failed update is undone by re-applying the current
	// config verbatim; without this, the rollback would briefly close and reopen
	// an otherwise-undisturbed listener. Comparing the raw config data covers
	// every field the config carries, so a change to any of them still applies.
	if prevConfig != nil && data == self.appliedData {
		self.mu.Unlock()
		return nil
	}
	oldListeners := self.listeners
	self.listeners = newListeners
	self.dialers = newDialers
	self.config = cfg
	self.appliedData = data
	handler := self.changeHandler
	self.mu.Unlock()

	// Apply the heartbeat settings so they take effect on established links,
	// not just new ones: the callbacks read closeUnresponsiveTimeout live, and
	// onLinkSubsystemChanged pushes interval changes via UpdateHeartbeatIntervals.
	// The applied config is authoritative, so settings absent from it revert to
	// their defaults (handled in applyHeartbeatDuration).
	var hb HeartbeatsConfig
	if cfg.Heartbeats != nil {
		hb = *cfg.Heartbeats
	}
	self.applyHeartbeatDuration(hb.CloseUnresponsiveTimeout, &self.closeUnresponsiveTimeout, "closeUnresponsiveTimeout")
	self.applyHeartbeatDuration(hb.SendInterval, &self.sendInterval, "sendInterval")
	self.applyHeartbeatDuration(hb.CheckInterval, &self.checkInterval, "checkInterval")

	closeListeners(oldListeners)
	setDefaultDialerBinding(newListeners, newDialers)
	if err := startListeners(newListeners); err != nil {
		return err
	}

	notifyChange(handler, prevConfig, cfg)
	return nil
}

// Remove implements managedconfig.ConfigHandler. Tears down listeners and
// clears dialers; the router has no link surface until a subsequent
// Apply. Established Xlinks are not touched.
func (self *FactoryRegistry) Remove() error {
	self.mu.Lock()
	prevConfig := self.config
	oldListeners := self.listeners
	self.listeners = nil
	self.dialers = nil
	self.config = nil
	self.appliedData = ""
	handler := self.changeHandler
	self.mu.Unlock()

	// A removed config is no longer authoritative, so its heartbeat settings
	// revert to the defaults, mirroring how Apply treats a config that omits
	// them. Without this, the removed intervals would linger on the lock-free
	// accessors and the change handler would retune established links to the
	// stale values instead of the defaults.
	self.closeUnresponsiveTimeout.Store(0)
	self.sendInterval.Store(0)
	self.checkInterval.Store(0)

	closeListeners(oldListeners)
	notifyChange(handler, prevConfig, nil)
	return nil
}

// notifyChange invokes handler asynchronously when prev and next differ.
// nil handler is a no-op. Compares listeners and dialers slices by
// deep-equality; identical state is a no-op (avoids spurious peer
// notifications and dialer rescans when an Apply was just re-applying
// the same config).
func notifyChange(handler ConfigurationChangeHandler, prev, next *Config) {
	if handler == nil {
		return
	}
	change := ConfigurationChange{
		ListenersChanged:  !listenerSlicesEqual(getListeners(prev), getListeners(next)),
		DialersChanged:    !dialerSlicesEqual(getEffectiveDialers(prev), getEffectiveDialers(next)),
		GcModeChanged:     getGcMode(prev) != getGcMode(next),
		HeartbeatsChanged: getHeartbeats(prev) != getHeartbeats(next),
	}
	if !change.ListenersChanged && !change.DialersChanged && !change.GcModeChanged && !change.HeartbeatsChanged {
		return
	}
	go handler(change)
}

// getHeartbeats returns a comparable value of the config's heartbeat settings
// (zero value when unset), so notifyChange can detect heartbeat changes.
func getHeartbeats(c *Config) HeartbeatsConfig {
	if c == nil || c.Heartbeats == nil {
		return HeartbeatsConfig{}
	}
	return *c.Heartbeats
}

func getGcMode(c *Config) string {
	if c == nil {
		return ""
	}
	return c.GcMode
}

func getListeners(c *Config) []ListenerConfig {
	if c == nil {
		return nil
	}
	return c.Listeners
}

func getEffectiveDialers(c *Config) []DialerConfig {
	if c == nil {
		return nil
	}
	out := make([]DialerConfig, len(c.Dialers))
	copy(out, c.Dialers)
	if len(c.Listeners) == 1 && len(out) == 1 && out[0].BindInterface == "" {
		out[0].BindInterface = c.Listeners[0].BindInterface
	}
	return out
}

// listenerSlicesEqual compares listener configs by every field that
// affects what the controller (and through it, peer routers) should
// see — binding, bind, advertise, bindInterface, groups, options.
func listenerSlicesEqual(a, b []ListenerConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// dialerSlicesEqual compares effective dialer configs by every field
// that affects local dial decisions — binding, groups, bindInterface,
// options, backoffs.
func dialerSlicesEqual(a, b []DialerConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// Listeners returns a snapshot of the current listener slice. Safe to
// iterate by callers concurrent with Apply / Remove.
func (self *FactoryRegistry) Listeners() []xlink.Listener {
	self.mu.RLock()
	defer self.mu.RUnlock()
	out := make([]xlink.Listener, len(self.listeners))
	copy(out, self.listeners)
	return out
}

// Dialers returns a snapshot of the current dialer slice.
func (self *FactoryRegistry) Dialers() []xlink.Dialer {
	self.mu.RLock()
	defer self.mu.RUnlock()
	out := make([]xlink.Dialer, len(self.dialers))
	copy(out, self.dialers)
	return out
}

// GetConfig returns the currently-applied config or nil if nothing is applied.
func (self *FactoryRegistry) GetConfig() *Config {
	self.mu.RLock()
	defer self.mu.RUnlock()
	return self.config
}

// build constructs new listeners and dialers from cfg without taking the
// lock; the caller swaps them in. If any construction fails, all
// successfully-built listeners are closed and an error is returned —
// caller's state is left unchanged.
func (self *FactoryRegistry) build(cfg *Config) ([]xlink.Listener, []xlink.Dialer, error) {
	self.mu.RLock()
	factories := make(map[string]xlink.Factory, len(self.factories))
	for k, v := range self.factories {
		factories[k] = v
	}
	self.mu.RUnlock()

	var listeners []xlink.Listener
	var dialers []xlink.Dialer

	for i, l := range cfg.Listeners {
		binding := defaultBinding(l.Binding)
		factory, ok := factories[binding]
		if !ok {
			closeListeners(listeners)
			return nil, nil, fmt.Errorf("listener[%d]: no factory registered for binding %q", i, binding)
		}
		tcfg := listenerConfigToMap(&l)
		tcfg[transport.KeyProtocol] = "ziti-link"
		listener, err := factory.CreateListener(self.routerId, tcfg)
		if err != nil {
			closeListeners(listeners)
			return nil, nil, fmt.Errorf("listener[%d]: create: %w", i, err)
		}
		listeners = append(listeners, listener)
	}

	for i, d := range cfg.Dialers {
		binding := defaultBinding(d.Binding)
		factory, ok := factories[binding]
		if !ok {
			closeListeners(listeners)
			return nil, nil, fmt.Errorf("dialer[%d]: no factory registered for binding %q", i, binding)
		}
		dialer, err := factory.CreateDialer(self.routerId, dialerConfigToMap(&d))
		if err != nil {
			closeListeners(listeners)
			return nil, nil, fmt.Errorf("dialer[%d]: create: %w", i, err)
		}
		dialers = append(dialers, dialer)
	}

	return listeners, dialers, nil
}

// startListeners calls Listen on each. On failure, listeners successfully
// started so far are closed and an error is returned.
func startListeners(listeners []xlink.Listener) error {
	for i, l := range listeners {
		if err := l.Listen(); err != nil {
			closeListeners(listeners[:i+1])
			return fmt.Errorf("listener[%d]: listen: %w", i, err)
		}
		pfxlog.Logger().WithField("advertise", l.GetAdvertisement()).
			WithField("binding", l.GetLinkProtocol()).
			Info("started Xlink listener")
	}
	return nil
}

// closeListeners closes each in order, logging but not aborting on error.
// Used both during shutdown and during error-path cleanup in build /
// startListeners.
func closeListeners(listeners []xlink.Listener) {
	for _, l := range listeners {
		if err := l.Close(); err != nil {
			pfxlog.Logger().WithError(err).Warn("error closing xlink listener")
		}
	}
}

// setDefaultDialerBinding mirrors router.setDefaultDialerBindings — if
// there's exactly one listener and one dialer and the dialer has no
// explicit binding, adopt the listener's. Pre-existing behavior preserved
// for backward compatibility with single-listener single-dialer configs.
func setDefaultDialerBinding(listeners []xlink.Listener, dialers []xlink.Dialer) {
	if len(listeners) == 1 && len(dialers) == 1 && dialers[0].GetBinding() == "" {
		dialers[0].AdoptBinding(listeners[0])
	}
}

func defaultBinding(b string) string {
	if b == "" {
		return "transport"
	}
	return b
}

// listenerConfigToMap converts a typed ListenerConfig back into the
// map[interface{}]interface{} shape that xlink.Factory.CreateListener
// (and the underlying loadListenerConfig) expects. This preserves the
// existing factory contract used by third-party plugins.
func listenerConfigToMap(l *ListenerConfig) map[interface{}]interface{} {
	out := map[interface{}]interface{}{}
	if l.Binding != "" {
		out["binding"] = l.Binding
	}
	if l.Bind != "" {
		out["bind"] = l.Bind
	}
	if l.Advertise != "" {
		out["advertise"] = l.Advertise
	}
	if l.BindInterface != "" {
		out["bindInterface"] = l.BindInterface
	}
	if len(l.Groups) > 0 {
		out["groups"] = stringSliceToIfaceSlice(l.Groups)
	}
	if l.Options != nil {
		out["options"] = channelOptionsToMap(l.Options)
	}
	return out
}

func dialerConfigToMap(d *DialerConfig) map[interface{}]interface{} {
	out := map[interface{}]interface{}{}
	if d.Binding != "" {
		out["binding"] = d.Binding
	}
	if d.MaxDefaultConnections > 0 {
		out["maxDefaultConnections"] = d.MaxDefaultConnections
	}
	if d.MaxAckConnections > 0 {
		out["maxAckConnections"] = d.MaxAckConnections
	}
	if d.StartupDelay != "" {
		out["startupDelay"] = d.StartupDelay
	}
	if d.BindInterface != "" {
		out["bind"] = d.BindInterface
	}
	if len(d.Groups) > 0 {
		out["groups"] = stringSliceToIfaceSlice(d.Groups)
	}
	if d.HealthyDialBackoff != nil {
		out["healthyDialBackoff"] = backoffToMap(d.HealthyDialBackoff)
	}
	if d.UnhealthyDialBackoff != nil {
		out["unhealthyDialBackoff"] = backoffToMap(d.UnhealthyDialBackoff)
	}
	if d.Options != nil {
		out["options"] = channelOptionsToMap(d.Options)
	}
	return out
}

func channelOptionsToMap(o *ChannelOptions) map[interface{}]interface{} {
	out := map[interface{}]interface{}{}
	if o.OutQueueSize > 0 {
		out["outQueueSize"] = o.OutQueueSize
	}
	if o.MaxQueuedConnects > 0 {
		out["maxQueuedConnects"] = o.MaxQueuedConnects
	}
	if o.MaxOutstandingConnects > 0 {
		out["maxOutstandingConnects"] = o.MaxOutstandingConnects
	}
	if o.ConnectTimeout != "" {
		out["connectTimeout"] = o.ConnectTimeout
	}
	if o.WriteTimeout != "" {
		out["writeTimeout"] = o.WriteTimeout
	}
	return out
}

func backoffToMap(b *BackoffConfig) map[interface{}]interface{} {
	out := map[interface{}]interface{}{}
	if b.RetryBackoffFactor > 0 {
		out["retryBackoffFactor"] = b.RetryBackoffFactor
	}
	if b.MinRetryInterval != "" {
		out["minRetryInterval"] = b.MinRetryInterval
	}
	if b.MaxRetryInterval != "" {
		out["maxRetryInterval"] = b.MaxRetryInterval
	}
	return out
}

func stringSliceToIfaceSlice(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
