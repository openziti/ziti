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

package managedconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/inspect"
)

// ConfigSource identifies where a Config event originated. The registry
// tracks data per source and resolves precedence at reconcile time: local
// wins entirely at the base level (if the operator set anything locally for
// a given base, the controller's versions are ignored for that base).
type ConfigSource int

const (
	// SourceController means the data came from the controller via the RDM.
	SourceController ConfigSource = iota
	// SourceLocal means the data came from the router's local config file.
	SourceLocal
)

// String returns the lowercase name of the source, suitable for diagnostics.
func (s ConfigSource) String() string {
	switch s {
	case SourceController:
		return "controller"
	case SourceLocal:
		return "local"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// AlertCallback is invoked when the registry encounters a non-recoverable
// outcome (Apply fails AND rollback fails, or Remove fails). Phase 3c will
// wire this to a controller-alerting transport; the default implementation
// logs.
type AlertCallback func(baseType, detail string)

// ErrHandlerAlreadyRegistered is returned by Register when a handler is
// already registered for the same BaseType.
var ErrHandlerAlreadyRegistered = errors.New("a handler is already registered for this base type")

// ErrNoHandlerRegistered is returned by Apply/Remove when no handler has been
// registered for the requested config type's base.
var ErrNoHandlerRegistered = errors.New("no handler registered for config type's base")

// ParseConfigType parses a versioned config type name like "router.link.v2"
// into ("router.link", 2). Returns an error if the name doesn't end in
// ".v<positive integer>".
func ParseConfigType(name string) (baseType string, version int, err error) {
	idx := strings.LastIndex(name, ".v")
	if idx < 1 {
		return "", 0, fmt.Errorf("config type %q is not of the form <baseType>.v<N>", name)
	}
	baseType = name[:idx]
	versionStr := name[idx+2:]
	if versionStr == "" {
		return "", 0, fmt.Errorf("config type %q has empty version", name)
	}
	version, err = strconv.Atoi(versionStr)
	if err != nil {
		return "", 0, fmt.Errorf("config type %q has non-integer version %q", name, versionStr)
	}
	if version <= 0 {
		return "", 0, fmt.Errorf("config type %q has non-positive version %d", name, version)
	}
	return baseType, version, nil
}

// appliedState records what a handler currently has active. version == 0 means
// nothing is applied; source is meaningful only when version > 0.
type appliedState struct {
	source  ConfigSource
	version int
	data    string
}

// localEntry is the local config currently in effect for a handler base.
// Stored as a pointer on handlerEntry so nil unambiguously means "no local
// config." version is the JSON schema version the YAML translator emitted
// (typically the newest the build supports); data is the raw JSON.
type localEntry struct {
	version int
	data    string
}

// handlerEntry binds a registered handler with the per-handler lock that
// serializes reconciles for that handler family, the set of currently-known
// data versions, and the currently-applied state. All fields except `lock`
// are guarded by Registry.mu.
//
// Controller data can carry multiple versions simultaneously (e.g. v1 and v2
// both flowing during a rollout). Local data is always at most one
// (version, data) pair — the local YAML file expresses one effective config
// per subsystem, not a multi-version set — so we store it as a single
// pointer rather than a map.
type handlerEntry struct {
	handler            ConfigHandler
	controllerVersions map[int]string // version -> data
	local              *localEntry    // nil means no local config set
	applied            appliedState
	lock               sync.Mutex
}

// Registry holds router subsystems that accept controller-managed
// configuration and routes Config events to them. It implements multi-version
// selection (highest int wins among versions a handler supports), source-aware
// precedence (local config wins over controller), and the rollback contract
// from doc/design/ctrl-managed-router-config.md.
//
// Lifecycle:
//
//  1. Construct with NewRegistry.
//  2. Subsystems Register their handlers.
//  3. Caller invokes Seal. After Seal, Register panics. Apply / Remove
//     before Seal also panic.
//  4. Config events arrive via ApplyController / ApplyLocal (and the
//     matching Remove* methods).
//  5. Close drains in-flight reconciles and stops the registry.
//
// Apply / Remove return as soon as the registry has updated its shared state;
// the actual handler.Apply / handler.Remove call runs on a freshly-spawned
// goroutine so slow handlers don't back up the caller. Per-handler reconciles
// serialize via the handlerEntry mutex; different handlers reconcile in
// parallel.
type Registry struct {
	mu       sync.Mutex
	handlers map[string]*handlerEntry // baseType -> entry
	alert    AlertCallback

	sealed atomic.Bool
	wg     sync.WaitGroup
	closed atomic.Bool
}

// NewRegistry creates a new Registry. If alert is nil, a logging alerter is
// used.
func NewRegistry(alert AlertCallback) *Registry {
	if alert == nil {
		alert = defaultAlert
	}
	return &Registry{
		handlers: map[string]*handlerEntry{},
		alert:    alert,
	}
}

func defaultAlert(baseType, detail string) {
	pfxlog.Logger().WithField("baseType", baseType).Warn(detail)
}

// Register associates the handler with its BaseType. Returns
// ErrHandlerAlreadyRegistered if a different handler is already registered
// for that base. Panics if called after Seal.
func (self *Registry) Register(handler ConfigHandler) error {
	if self.sealed.Load() {
		panic(fmt.Sprintf("managedconfig.Registry.Register called after Seal for base %q", handler.BaseType()))
	}

	base := handler.BaseType()

	self.mu.Lock()
	defer self.mu.Unlock()

	if existing, ok := self.handlers[base]; ok && existing.handler != handler {
		return fmt.Errorf("%w: %s", ErrHandlerAlreadyRegistered, base)
	}
	if _, ok := self.handlers[base]; !ok {
		self.handlers[base] = &handlerEntry{
			handler:            handler,
			controllerVersions: map[int]string{},
		}
	}
	return nil
}

// Seal marks the registration phase complete. After Seal, calls to Register
// panic. Apply / Remove may only be called after Seal.
func (self *Registry) Seal() {
	self.sealed.Store(true)
}

// Handler returns the handler registered for the BaseType extracted from
// configType, or nil if none is registered.
func (self *Registry) Handler(configType string) ConfigHandler {
	base, _, err := ParseConfigType(configType)
	if err != nil {
		return nil
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	if entry := self.handlers[base]; entry != nil {
		return entry.handler
	}
	return nil
}

// ApplyController records the controller's most recent data for configType
// and spawns a goroutine to reconcile the owning handler. Returns parse
// errors synchronously, ErrNoHandlerRegistered when no handler owns the
// base, or nil. Panics if called pre-Seal.
func (self *Registry) ApplyController(configType string, data string) error {
	if !self.sealed.Load() {
		panic("managedconfig.Registry.ApplyController called before Seal")
	}
	base, version, err := ParseConfigType(configType)
	if err != nil {
		return err
	}

	self.mu.Lock()
	entry, ok := self.handlers[base]
	if !ok {
		self.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNoHandlerRegistered, base)
	}
	entry.controllerVersions[version] = data
	self.mu.Unlock()

	self.spawnReconcile(entry)
	return nil
}

// RemoveController drops a specific (base, version) entry from the
// controller-source set. Other controller versions for the same base
// remain. If local data is set for the base, the handler is unaffected
// (local was already winning). Otherwise the handler reconciles to whatever
// is left.
func (self *Registry) RemoveController(configType string) error {
	if !self.sealed.Load() {
		panic("managedconfig.Registry.RemoveController called before Seal")
	}
	base, version, err := ParseConfigType(configType)
	if err != nil {
		return err
	}

	self.mu.Lock()
	entry, ok := self.handlers[base]
	if !ok {
		self.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNoHandlerRegistered, base)
	}
	delete(entry.controllerVersions, version)
	self.mu.Unlock()

	self.spawnReconcile(entry)
	return nil
}

// ApplyLocal records the local config file's data for configType and spawns
// a goroutine to reconcile the owning handler. Local config is always a
// single (version, data) pair per base — repeated calls replace any prior
// local entry. Local takes precedence over controller versions for the
// same base, so as long as a local entry exists, the controller's data is
// ignored. Panics if called pre-Seal.
func (self *Registry) ApplyLocal(configType string, data string) error {
	if !self.sealed.Load() {
		panic("managedconfig.Registry.ApplyLocal called before Seal")
	}
	base, version, err := ParseConfigType(configType)
	if err != nil {
		return err
	}

	self.mu.Lock()
	entry, ok := self.handlers[base]
	if !ok {
		self.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNoHandlerRegistered, base)
	}
	entry.local = &localEntry{version: version, data: data}
	self.mu.Unlock()

	self.spawnReconcile(entry)
	return nil
}

// RemoveLocal clears the local config for the given base. Takes a base type
// rather than a configType because the version is meaningless for local
// removal — there's at most one local entry per base, regardless of which
// version it was set at. When local is cleared, the controller's highest
// supported version becomes effective. Panics if called pre-Seal.
func (self *Registry) RemoveLocal(baseType string) error {
	if !self.sealed.Load() {
		panic("managedconfig.Registry.RemoveLocal called before Seal")
	}
	if baseType == "" {
		return errors.New("RemoveLocal requires a non-empty base type")
	}

	self.mu.Lock()
	entry, ok := self.handlers[baseType]
	if !ok {
		self.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNoHandlerRegistered, baseType)
	}
	entry.local = nil
	self.mu.Unlock()

	self.spawnReconcile(entry)
	return nil
}

// Close marks the registry shut down and blocks until every in-flight
// reconcile goroutine has exited. After Close, future Apply / Remove calls
// still update state but do not spawn new reconciles.
func (self *Registry) Close() {
	self.closed.Store(true)
	self.wg.Wait()
}

// WaitForIdle blocks until every currently-spawned reconcile goroutine has
// exited. Useful in tests to assert handler effects synchronously.
func (self *Registry) WaitForIdle() {
	self.wg.Wait()
}

func (self *Registry) spawnReconcile(entry *handlerEntry) {
	if self.closed.Load() {
		return
	}
	self.wg.Add(1)
	go self.reconcileAsync(entry)
}

func (self *Registry) reconcileAsync(entry *handlerEntry) {
	defer self.wg.Done()

	entry.lock.Lock()
	defer entry.lock.Unlock()

	if self.closed.Load() {
		return
	}

	handler := entry.handler

	self.mu.Lock()
	prev := entry.applied
	nextSource, nextVersion, nextData, hasNext := self.findEffectiveLocked(entry)
	self.mu.Unlock()

	base := handler.BaseType()

	switch {
	case prev.version == 0 && !hasNext:
		// nothing applied, nothing available

	case prev.version == 0 && hasNext:
		if err := handler.Apply(nextVersion, nextData); err != nil {
			self.alert(base, fmt.Sprintf("v%d (%s) initial apply failed: %v", nextVersion, nextSource, err))
			if rmErr := handler.Remove(); rmErr != nil {
				self.alert(base, fmt.Sprintf("v%d (%s) initial apply failed and Remove also failed: %v", nextVersion, nextSource, rmErr))
			}
			self.setApplied(entry, appliedState{})
			return
		}
		self.setApplied(entry, appliedState{source: nextSource, version: nextVersion, data: nextData})

	case prev.version != 0 && !hasNext:
		if err := handler.Remove(); err != nil {
			self.alert(base, fmt.Sprintf("v%d (%s) Remove failed: %v; subsystem state unchanged", prev.version, prev.source, err))
			return
		}
		self.setApplied(entry, appliedState{})

	case prev.version != 0 && hasNext:
		if prev.source == nextSource && prev.version == nextVersion && prev.data == nextData {
			return
		}
		if err := handler.Apply(nextVersion, nextData); err != nil {
			self.alert(base, fmt.Sprintf("v%d (%s) apply failed (%v); rolling back to v%d (%s)", nextVersion, nextSource, err, prev.version, prev.source))
			if rbErr := handler.Apply(prev.version, prev.data); rbErr != nil {
				self.alert(base, fmt.Sprintf("rollback to v%d (%s) also failed (%v); calling Remove", prev.version, prev.source, rbErr))
				if rmErr := handler.Remove(); rmErr != nil {
					self.alert(base, fmt.Sprintf("Remove also failed: %v; subsystem state unknown", rmErr))
				}
				self.setApplied(entry, appliedState{})
				return
			}
			// rollback succeeded; applied stays at prev
			return
		}
		self.setApplied(entry, appliedState{source: nextSource, version: nextVersion, data: nextData})
	}
}

// findEffectiveLocked computes the effective config for the handler.
//
// Strict local-wins at the base level: if local is set, the controller's
// data is entirely ignored. If local's version is one the handler supports,
// that's the effective config. If local's version is NOT supported (e.g.
// after an upgrade that drops the version), nothing applies — the operator
// must fix their YAML. We deliberately don't fall back to the controller's
// data in that case because the operator's intent ("use my local config")
// should not be silently overridden.
//
// If local isn't set, the effective config is the highest controller
// version the handler supports. Caller holds Registry.mu.
func (self *Registry) findEffectiveLocked(entry *handlerEntry) (source ConfigSource, version int, data string, found bool) {
	if entry.local != nil {
		for _, v := range entry.handler.SupportedVersions() {
			if v == entry.local.version {
				return SourceLocal, entry.local.version, entry.local.data, true
			}
		}
		pfxlog.Logger().WithField("baseType", entry.handler.BaseType()).WithField("version", entry.local.version).WithField("supported", entry.handler.SupportedVersions()).Error("local config at unsupported version; nothing applied (likely a programming error in the YAML translator)")
		return 0, 0, "", false
	}

	bestVersion := 0
	var bestData string
	for _, v := range entry.handler.SupportedVersions() {
		if d, ok := entry.controllerVersions[v]; ok {
			if v > bestVersion {
				bestVersion = v
				bestData = d
			}
		}
	}
	if bestVersion > 0 {
		return SourceController, bestVersion, bestData, true
	}
	return 0, 0, "", false
}

func (self *Registry) setApplied(entry *handlerEntry, state appliedState) {
	self.mu.Lock()
	entry.applied = state
	self.mu.Unlock()
}

// AppliedVersion returns the version of configType currently applied for the
// handler whose BaseType matches the parsed configType, or 0 if nothing is
// applied. Source-agnostic; use Applied for the full state.
func (self *Registry) AppliedVersion(configType string) int {
	_, version, _ := self.Applied(configType)
	return version
}

// Inspect returns a snapshot of the registry's state, intended for diagnostics
// via `ziti fabric inspect router-config-registry`. Handlers are returned in
// BaseType order for deterministic output.
func (self *Registry) Inspect() inspect.RouterConfigRegistryState {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := inspect.RouterConfigRegistryState{
		Sealed:   self.sealed.Load(),
		Closed:   self.closed.Load(),
		Handlers: make([]inspect.RouterConfigHandlerDetail, 0, len(self.handlers)),
	}

	bases := make([]string, 0, len(self.handlers))
	for base := range self.handlers {
		bases = append(bases, base)
	}
	sort.Strings(bases)

	for _, base := range bases {
		result.Handlers = append(result.Handlers, self.handlers[base].inspect())
	}
	return result
}

// inspect returns a snapshot of this handler's registry state. Caller must
// hold Registry.mu.
func (self *handlerEntry) inspect() inspect.RouterConfigHandlerDetail {
	detail := inspect.RouterConfigHandlerDetail{
		BaseType:          self.handler.BaseType(),
		SupportedVersions: self.handler.SupportedVersions(),
	}
	versions := make([]int, 0, len(self.controllerVersions))
	for v := range self.controllerVersions {
		versions = append(versions, v)
	}
	sort.Ints(versions)
	for _, v := range versions {
		detail.ControllerConfigs = append(detail.ControllerConfigs, inspect.RouterConfigVersionDetail{
			Version: v,
			Data:    parseInspectData(self.controllerVersions[v]),
		})
	}
	if self.local != nil {
		local := self.local.inspect()
		detail.LocalConfig = &local
	}
	detail.Applied = self.applied.inspect()
	return detail
}

// inspect returns a version detail for this local entry.
func (self *localEntry) inspect() inspect.RouterConfigVersionDetail {
	return inspect.RouterConfigVersionDetail{
		Version: self.version,
		Data:    parseInspectData(self.data),
	}
}

// parseInspectData decodes a stored config payload into the parsed structure
// the inspect output should display. If the payload isn't valid JSON, the raw
// string is returned so the diagnostic still shows what the registry holds.
func parseInspectData(data string) any {
	var parsed any
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return data
	}
	return parsed
}

// inspect returns the applied detail, or nil if nothing is applied.
func (self appliedState) inspect() *inspect.RouterConfigAppliedDetail {
	if self.version == 0 {
		return nil
	}
	return &inspect.RouterConfigAppliedDetail{
		Source:  self.source.String(),
		Version: self.version,
	}
}

// Applied returns the source and version currently applied for the handler
// owning configType. found is false if nothing is applied or no handler is
// registered for the base.
func (self *Registry) Applied(configType string) (source ConfigSource, version int, found bool) {
	base, _, err := ParseConfigType(configType)
	if err != nil {
		return 0, 0, false
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	entry := self.handlers[base]
	if entry == nil || entry.applied.version == 0 {
		return 0, 0, false
	}
	return entry.applied.source, entry.applied.version, true
}
