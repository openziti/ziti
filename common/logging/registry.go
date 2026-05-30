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

package logging

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
)

// Registry tracks the global log level, per-name level overrides, and the
// root handler that named loggers attach to. Levels are stored in
// *slog.LevelVar so any *slog.Logger constructed via For sees subsequent
// level changes on its next Enabled check, without re-creation.
//
// The root handler is held in an atomic.Pointer so SetRoot can swap it
// without re-creating the Registry: existing Loggers keep pointing at this
// Registry instance and pick up the new root on their next Handle call.
// This makes `var log = logging.For(...)` at package-init time safe, since
// the bootstrap Registry created at init can be re-rooted by Install later
// without orphaning any loggers.
//
// Reads (Enabled checks, For lookups in the cache) take RLock; mutations
// take the write lock. Level changes are operator actions and not a hot
// path, so the lock cost on Set/Clear is fine.
type Registry struct {
	mu          sync.RWMutex
	global      *slog.LevelVar
	overrides   map[string]*slog.LevelVar
	loggerCache map[string]*slog.Logger
	root        atomic.Pointer[slog.Handler]
}

// NewRegistry returns a Registry that sends records to root. Panics if root
// is nil.
func NewRegistry(root slog.Handler) *Registry {
	if root == nil {
		panic("logging: NewRegistry requires a non-nil root handler")
	}
	r := &Registry{
		global:      new(slog.LevelVar),
		overrides:   map[string]*slog.LevelVar{},
		loggerCache: map[string]*slog.Logger{},
	}
	r.root.Store(&root)
	return r
}

// SetGlobalLevel sets the level used when no per-name override exists.
// Loggers already constructed via For pick up the new level on their next
// Enabled check. Pure slog-side; lockstep with logrus happens at the
// package-level SetGlobalLevel.
func (r *Registry) SetGlobalLevel(level slog.Level) {
	r.global.Set(level)
}

// GlobalLevel returns the current global level.
func (r *Registry) GlobalLevel() slog.Level {
	return r.global.Level()
}

// SetNamedLevel installs or updates a per-name override. The override is
// held in a *slog.LevelVar created on first use for the name; subsequent
// SetNamedLevel calls update the same LevelVar, so any Logger view of the
// name sees the new level live.
func (r *Registry) SetNamedLevel(name string, level slog.Level) {
	r.mu.Lock()
	lv, ok := r.overrides[name]
	if !ok {
		lv = new(slog.LevelVar)
		r.overrides[name] = lv
	}
	r.mu.Unlock()
	lv.Set(level)
}

// ClearNamedLevel removes the override for name. After clear, the name
// falls back to the live global level; this is deliberately not "set to
// the current global" so a later SetGlobalLevel propagates to the
// previously-overridden name automatically.
func (r *Registry) ClearNamedLevel(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.overrides, name)
}

// For returns the *slog.Logger for a named scope, constructed on first use
// and cached thereafter. Subsequent calls with the same name return the
// same pointer. The logger's handler chain binds "channel": name as the
// first attr, so output records always carry the logger name. Panics on
// empty name (almost always a caller bug).
func (r *Registry) For(name string) *slog.Logger {
	if name == "" {
		panic("logging: For called with empty name")
	}
	r.mu.RLock()
	cached, ok := r.loggerCache[name]
	r.mu.RUnlock()
	if ok {
		return cached
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.loggerCache[name]; ok {
		return cached
	}
	nh := &namedHandler{registry: r, name: name}
	h := nh.WithAttrs([]slog.Attr{slog.String("channel", name)})
	logger := slog.New(h)
	r.loggerCache[name] = logger
	return logger
}

// HandlerFor returns the bare namedHandler for name, without the
// channel-attr wrap For provides. Useful for embedding the level-gating
// handler in a custom chain. Panics on empty name.
func (r *Registry) HandlerFor(name string) slog.Handler {
	if name == "" {
		panic("logging: HandlerFor called with empty name")
	}
	return &namedHandler{registry: r, name: name}
}

// Root returns the registry's current root handler. The package-level
// RootHandler reads from the default Registry via this method; the logrus
// bridge uses it to dispatch records, and SyncEmit uses it to find the
// underlying AsyncHandler when the root is one. Safe under concurrent
// SetRoot.
func (r *Registry) Root() slog.Handler {
	return *r.root.Load()
}

// SetRoot replaces the registry's root handler atomically. Existing Loggers
// (constructed by For at any earlier point) pick up the new root on their
// next Handle call. Configure uses this to install the production handler
// chain after package init has already handed out loggers via For. Panics
// if root is nil so the Registry never has a missing destination.
func (r *Registry) SetRoot(root slog.Handler) {
	if root == nil {
		panic("logging: SetRoot requires a non-nil root handler")
	}
	r.root.Store(&root)
}

// resolveLevel returns the effective level for name: the override's level
// if one is set, otherwise the live global level.
func (r *Registry) resolveLevel(name string) slog.Level {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if lv, ok := r.overrides[name]; ok {
		return lv.Level()
	}
	return r.global.Level()
}

// namedHandler is the chain node that does per-name level gating and
// forwards records to the registry's root handler. Records pass through
// unchanged; the "channel" attr is added by For's WithAttrs wrap, not here.
type namedHandler struct {
	registry *Registry
	name     string
}

func (h *namedHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.registry.resolveLevel(h.name)
}

func (h *namedHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.registry.Root().Handle(ctx, r)
}

func (h *namedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return &boundHandler{parent: h, attrs: slices.Clone(attrs)}
}

func (h *namedHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &groupedHandler{parent: h, name: name}
}

// defaultRegistry is the process-wide Registry. It is created at package
// init with a stderr bootstrap root, so For and the other package-level
// entry points work immediately - including at package-init time, before
// any process startup code has called Configure. Anything that emits
// during the bootstrap window (cobra flag parsing, package init) lands on
// stderr in plain text, so a real startup problem stays visible rather
// than being silently dropped. Configure later swaps the root handler in
// place; existing loggers keep working and pick up the new root
// transparently. The Registry itself is never replaced, so per-name
// overrides set before Configure persist across it.
var defaultRegistry = NewRegistry(newBootstrapHandler())

// newBootstrapHandler returns the slog.Handler used as the default
// Registry's root before Configure runs. Plain text to stderr at the
// lowest level: gating still happens upstream in namedHandler against
// the Registry's global level, so this handler accepts whatever passes
// that gate. Once Install calls Configure, this handler is replaced.
func newBootstrapHandler() slog.Handler {
	return slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: LevelTrace})
}

// DefaultRegistry returns the package-level default Registry. Useful for
// tests that need a stable handle to reset state, and for code that wants
// to bypass the package-level conveniences entirely.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// Configure installs root as the default Registry's root handler. It
// replaces the bootstrap stderr root from init time (or whatever a prior
// Configure call set) without re-creating the Registry, so existing
// loggers and per-name overrides survive. Install calls this from PreRun.
func Configure(root slog.Handler) {
	defaultRegistry.SetRoot(root)
}

// For returns the named logger from the default Registry.
func For(name string) *slog.Logger {
	return defaultRegistry.For(name)
}

// HandlerFor returns the bare named handler from the default Registry.
func HandlerFor(name string) slog.Handler {
	return defaultRegistry.HandlerFor(name)
}

// SetGlobalLevel sets the global level on the default Registry and the
// logrus standard logger in lockstep: logrus's pre-filter drops records
// below the level before they ever reach the bridge, while named slog
// loggers see the same threshold on their next Enabled check. Agent
// log-level callbacks, --verbose-style boot flags, and any other operator
// surface should go through this entry point so the two worlds never drift.
func SetGlobalLevel(level slog.Level) {
	defaultRegistry.SetGlobalLevel(level)
	logrus.SetLevel(slogToLogrus(level))
}

// GlobalLevel returns the global level on the default Registry.
func GlobalLevel() slog.Level {
	return defaultRegistry.GlobalLevel()
}

// SetNamedLevel sets a per-name override on the default Registry.
func SetNamedLevel(name string, level slog.Level) {
	defaultRegistry.SetNamedLevel(name, level)
}

// ClearNamedLevel removes a per-name override on the default Registry.
func ClearNamedLevel(name string) {
	defaultRegistry.ClearNamedLevel(name)
}
