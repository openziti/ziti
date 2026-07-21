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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type applyCall struct {
	version int
	data    string
}

// fakeHandler is a scriptable ConfigHandler for tests. ApplyErrs and
// RemoveErrs are consumed in order; nil means succeed. After exhausting the
// scripted errors, all subsequent calls succeed. Methods are safe to call
// from any goroutine.
type fakeHandler struct {
	mu         sync.Mutex
	base       string
	versions   []int
	applies    []applyCall
	removes    int
	applyErrs  []error
	removeErrs []error
}

func (f *fakeHandler) BaseType() string         { return f.base }
func (f *fakeHandler) SupportedVersions() []int { return f.versions }

func (f *fakeHandler) Apply(version int, data string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applies = append(f.applies, applyCall{version, data})
	if len(f.applyErrs) == 0 {
		return nil
	}
	err := f.applyErrs[0]
	f.applyErrs = f.applyErrs[1:]
	return err
}

func (f *fakeHandler) Remove() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes++
	if len(f.removeErrs) == 0 {
		return nil
	}
	err := f.removeErrs[0]
	f.removeErrs = f.removeErrs[1:]
	return err
}

func (f *fakeHandler) snapshot() (applies []applyCall, removes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]applyCall, len(f.applies))
	copy(out, f.applies)
	return out, f.removes
}

type alertEntry struct {
	baseType string
	detail   string
}

type alertRecorder struct {
	mu      sync.Mutex
	entries []alertEntry
}

func (a *alertRecorder) record(baseType, detail string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, alertEntry{baseType, detail})
}

func (a *alertRecorder) all() []alertEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]alertEntry, len(a.entries))
	copy(out, a.entries)
	return out
}

func newRecordingRegistry() (*Registry, *alertRecorder) {
	rec := &alertRecorder{}
	r := NewRegistry(rec.record)
	return r, rec
}

// newSealedRegistry returns a recording registry with the given handlers
// already registered and Seal() called. The standard test fixture for any
// test that needs to invoke Apply / Remove.
func newSealedRegistry(t *testing.T, handlers ...ConfigHandler) (*Registry, *alertRecorder) {
	t.Helper()
	r, rec := newRecordingRegistry()
	for _, h := range handlers {
		require.NoError(t, r.Register(h))
	}
	r.Seal()
	return r, rec
}

// --- ParseConfigType ---------------------------------------------------------

func Test_ParseConfigType_Valid(t *testing.T) {
	req := require.New(t)
	cases := []struct {
		in           string
		wantBaseType string
		wantVersion  int
	}{
		{"router.link.v1", "router.link", 1},
		{"router.link.v2", "router.link", 2},
		{"router.link.v42", "router.link", 42},
		{"router.xgress.proxy.v1", "router.xgress.proxy", 1},
	}
	for _, c := range cases {
		base, ver, err := ParseConfigType(c.in)
		req.NoError(err, c.in)
		req.Equal(c.wantBaseType, base, c.in)
		req.Equal(c.wantVersion, ver, c.in)
	}
}

func Test_ParseConfigType_Invalid(t *testing.T) {
	req := require.New(t)
	cases := []string{
		"router.link",
		"router.link.v",
		"router.link.va",
		"router.link.v1a",
		"router.link.v0",
		"router.link.v-1",
		".v1",
		"v1",
	}
	for _, c := range cases {
		_, _, err := ParseConfigType(c)
		req.Error(err, c)
	}
}

// --- Register & lookup -------------------------------------------------------

func Test_Register_Single(t *testing.T) {
	req := require.New(t)
	r, _ := newRecordingRegistry()

	h := &fakeHandler{base: "router.link", versions: []int{1}}
	req.NoError(r.Register(h))
	req.Same(h, r.Handler("router.link.v1"))
	req.Nil(r.Handler("other.family.v1"))
}

func Test_Register_Handler_OwnsBase(t *testing.T) {
	req := require.New(t)
	r, _ := newRecordingRegistry()

	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	req.NoError(r.Register(h))
	req.Same(h, r.Handler("router.link.v1"))
	req.Same(h, r.Handler("router.link.v2"))
	req.Same(h, r.Handler("router.link.v99"))
}

func Test_Register_Duplicate(t *testing.T) {
	req := require.New(t)
	r, _ := newRecordingRegistry()

	h1 := &fakeHandler{base: "router.link", versions: []int{1}}
	req.NoError(r.Register(h1))

	h2 := &fakeHandler{base: "router.link", versions: []int{2}}
	err := r.Register(h2)
	req.Error(err)
	req.ErrorIs(err, ErrHandlerAlreadyRegistered)
}

// --- Seal lifecycle ----------------------------------------------------------

func Test_Seal_PanicsOnLateRegister(t *testing.T) {
	req := require.New(t)
	r, _ := newRecordingRegistry()
	r.Seal()

	defer func() {
		req.NotNil(recover(), "Register after Seal should panic")
	}()
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	_ = r.Register(h)
}

func Test_Seal_ApplyStillWorks(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `{}`))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 1)
}

func Test_Seal_PanicsOnApplyBeforeSeal(t *testing.T) {
	req := require.New(t)
	r, _ := newRecordingRegistry()
	defer func() {
		req.NotNil(recover(), "Apply before Seal should panic")
	}()
	_ = r.ApplyController("router.link.v1", `{}`)
}

func Test_Seal_PanicsOnRemoveBeforeSeal(t *testing.T) {
	req := require.New(t)
	r, _ := newRecordingRegistry()
	defer func() {
		req.NotNil(recover(), "Remove before Seal should panic")
	}()
	_ = r.RemoveController("router.link.v1")
}

// --- Apply / Remove parse errors --------------------------------------------

func Test_Apply_InvalidConfigType_ReturnsError(t *testing.T) {
	req := require.New(t)
	r, _ := newSealedRegistry(t)
	err := r.ApplyController("router.link", `{}`)
	req.Error(err)
}

func Test_Remove_InvalidConfigType_ReturnsError(t *testing.T) {
	req := require.New(t)
	r, _ := newSealedRegistry(t)
	err := r.RemoveController("router.link.va")
	req.Error(err)
}

// --- Apply / Remove without a handler ---------------------------------------

func Test_Apply_NoHandler_ReturnsError(t *testing.T) {
	req := require.New(t)
	r, _ := newSealedRegistry(t)
	err := r.ApplyController("router.unknown.v1", `{}`)
	req.Error(err)
	req.ErrorIs(err, ErrNoHandlerRegistered)
}

func Test_Remove_NoHandler_ReturnsError(t *testing.T) {
	req := require.New(t)
	r, _ := newSealedRegistry(t)
	err := r.RemoveController("router.unknown.v1")
	req.Error(err)
	req.ErrorIs(err, ErrNoHandlerRegistered)
}

// --- Single-version flow -----------------------------------------------------

func Test_Apply_FirstTime(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `{"k":"v"}`))
	r.WaitForIdle()

	applies, removes := h.snapshot()
	req.Len(applies, 1)
	req.Equal(1, applies[0].version)
	req.Equal(0, removes)
	req.Equal(1, r.AppliedVersion("router.link.v1"))
}

func Test_Apply_Update(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `{"k":"a"}`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v1", `{"k":"b"}`))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 2)
	req.Equal(`{"k":"a"}`, string(applies[0].data))
	req.Equal(`{"k":"b"}`, string(applies[1].data))
}

func Test_Apply_NoOpWhenIdentical(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `{"k":"v"}`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v1", `{"k":"v"}`))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 1)
}

func Test_Apply_FirstFailure_TriggersRemove(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}, applyErrs: []error{errors.New("bad")}}
	r, alerts := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `{}`))
	r.WaitForIdle()

	applies, removes := h.snapshot()
	req.Len(applies, 1)
	req.Equal(1, removes)
	req.Equal(0, r.AppliedVersion("router.link.v1"))
	req.NotEmpty(alerts.all())
}

func Test_ApplyLocalSync_SurfacesApplyError(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}, applyErrs: []error{errors.New("bad bind address")}}
	r, alerts := newSealedRegistry(t, h)

	err := r.ApplyLocalSync("router.link.v1", `{}`)
	req.Error(err, "ApplyLocalSync must surface the handler Apply failure to the caller")
	req.ErrorContains(err, "bad bind address")

	// failed apply still triggers the rollback contract (Remove) and an alert
	applies, removes := h.snapshot()
	req.Len(applies, 1)
	req.Equal(1, removes)
	req.Equal(0, r.AppliedVersion("router.link.v1"))
	req.NotEmpty(alerts.all())
}

func Test_ApplyLocalSync_Succeeds(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyLocalSync("router.link.v1", `{"k":"a"}`))
	req.Equal(1, r.AppliedVersion("router.link.v1"))
}

func Test_Apply_UpdateFailure_RollbackSucceeds(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{
		base:      "router.link",
		versions:  []int{1},
		applyErrs: []error{nil, errors.New("bad-update"), nil},
	}
	r, alerts := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `{"k":"a"}`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v1", `{"k":"b"}`))
	r.WaitForIdle()

	applies, removes := h.snapshot()
	req.Len(applies, 3)
	req.Equal(`{"k":"a"}`, string(applies[0].data))
	req.Equal(`{"k":"b"}`, string(applies[1].data))
	req.Equal(`{"k":"a"}`, string(applies[2].data))
	req.Equal(0, removes)
	req.Equal(1, r.AppliedVersion("router.link.v1"))
	req.NotEmpty(alerts.all())
}

func Test_Apply_UpdateFailure_RollbackFails(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{
		base:      "router.link",
		versions:  []int{1},
		applyErrs: []error{nil, errors.New("bad-update"), errors.New("bad-rollback")},
	}
	r, alerts := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `{"k":"a"}`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v1", `{"k":"b"}`))
	r.WaitForIdle()

	applies, removes := h.snapshot()
	req.Len(applies, 3)
	req.Equal(1, removes)
	req.Equal(0, r.AppliedVersion("router.link.v1"))
	req.GreaterOrEqual(len(alerts.all()), 2)
}

// --- Multi-version flow ------------------------------------------------------

func Test_MultiVersion_HighestWins(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `v1data`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v2", `v2data`))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 2)
	req.Equal(1, applies[0].version)
	req.Equal(2, applies[1].version)
	req.Equal(2, r.AppliedVersion("router.link.v1"))
}

func Test_MultiVersion_HandlerSupportsSubset(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v2", `v2`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v1", `v1`))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 1)
	req.Equal(1, applies[0].version)
}

func Test_MultiVersion_FallbackOnRemove(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `v1data`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v2", `v2data`))
	r.WaitForIdle()
	req.NoError(r.RemoveController("router.link.v2"))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 3)
	req.Equal(1, applies[2].version)
	req.Equal(`v1data`, string(applies[2].data))
	req.Equal(1, r.AppliedVersion("router.link.v1"))
}

func Test_MultiVersion_FallbackOnApplyFailure(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{
		base:      "router.link",
		versions:  []int{1, 2},
		applyErrs: []error{nil, errors.New("v2 broken"), nil},
	}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `v1data`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v2", `v2data`))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 3)
	req.Equal(1, applies[0].version)
	req.Equal(2, applies[1].version)
	req.Equal(1, applies[2].version)
	req.Equal(1, r.AppliedVersion("router.link.v1"))
}

func Test_MultiVersion_RemoveLastAvailable(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `v1`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v2", `v2`))
	r.WaitForIdle()
	req.NoError(r.RemoveController("router.link.v2"))
	r.WaitForIdle()
	req.NoError(r.RemoveController("router.link.v1"))
	r.WaitForIdle()

	_, removes := h.snapshot()
	req.Equal(1, removes)
	req.Equal(0, r.AppliedVersion("router.link.v1"))
}

func Test_MultiVersion_OutOfOrderArrival(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `v1`))
	r.WaitForIdle()
	req.Equal(1, r.AppliedVersion("router.link.v1"))
	req.NoError(r.ApplyController("router.link.v2", `v2`))
	r.WaitForIdle()
	req.Equal(2, r.AppliedVersion("router.link.v1"))

	applies, _ := h.snapshot()
	req.Len(applies, 2)
}

// --- Remove flow -------------------------------------------------------------

func Test_Remove_HandlerError(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{
		base:       "router.link",
		versions:   []int{1},
		removeErrs: []error{errors.New("remove broken")},
	}
	r, alerts := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `{}`))
	r.WaitForIdle()
	req.NoError(r.RemoveController("router.link.v1"))
	r.WaitForIdle()

	_, removes := h.snapshot()
	req.Equal(1, removes)
	req.Equal(1, r.AppliedVersion("router.link.v1"),
		"applied should stay at previous when Remove fails")
	req.NotEmpty(alerts.all())
}

// --- Source precedence ------------------------------------------------------

func Test_Source_LocalApplies(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyLocal("router.link.v1", `local-data`))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 1)
	req.Equal(`local-data`, string(applies[0].data))

	src, ver, ok := r.Applied("router.link.v1")
	req.True(ok)
	req.Equal(SourceLocal, src)
	req.Equal(1, ver)
}

func Test_Source_LocalBeatsController(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	// Controller arrives first with v2, then local arrives with v1.
	// Local-wins is at the base level, so local v1 should beat controller v2.
	req.NoError(r.ApplyController("router.link.v2", `ctrl-v2`))
	r.WaitForIdle()
	req.Equal(2, r.AppliedVersion("router.link.v1"))

	req.NoError(r.ApplyLocal("router.link.v1", `local-v1`))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 2)
	req.Equal(2, applies[0].version) // ctrl v2 came first
	req.Equal(1, applies[1].version) // then local v1 took over

	src, ver, _ := r.Applied("router.link.v1")
	req.Equal(SourceLocal, src)
	req.Equal(1, ver)
}

func Test_Source_ControllerIgnoredWhileLocalSet(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	// Local v1 first, then controller tries v2. Controller should be ignored.
	req.NoError(r.ApplyLocal("router.link.v1", `local-v1`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v2", `ctrl-v2`))
	r.WaitForIdle()

	applies, _ := h.snapshot()
	req.Len(applies, 1, "controller event should not produce a handler call when local is set")
	req.Equal(1, applies[0].version)

	src, _, _ := r.Applied("router.link.v1")
	req.Equal(SourceLocal, src)
}

func Test_Source_RemoveLocalFallsBackToController(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	// Both sources have data; local wins.
	req.NoError(r.ApplyController("router.link.v2", `ctrl-v2`))
	r.WaitForIdle()
	req.NoError(r.ApplyLocal("router.link.v1", `local-v1`))
	r.WaitForIdle()
	src, _, _ := r.Applied("router.link.v1")
	req.Equal(SourceLocal, src)

	// Drop local; controller becomes effective.
	req.NoError(r.RemoveLocal("router.link"))
	r.WaitForIdle()

	src, ver, _ := r.Applied("router.link.v1")
	req.Equal(SourceController, src)
	req.Equal(2, ver)

	applies, _ := h.snapshot()
	// initial ctrl v2 + local v1 + fallback to ctrl v2 = 3 handler calls
	req.Len(applies, 3)
	req.Equal(2, applies[2].version)
}

func Test_Source_RemoveControllerWhileLocalSet_NoChange(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v2", `ctrl-v2`))
	r.WaitForIdle()
	req.NoError(r.ApplyLocal("router.link.v1", `local-v1`))
	r.WaitForIdle()
	appliesBefore, _ := h.snapshot()

	req.NoError(r.RemoveController("router.link.v2"))
	r.WaitForIdle()

	appliesAfter, _ := h.snapshot()
	req.Len(appliesAfter, len(appliesBefore),
		"controller removal should not trigger handler when local was already winning")

	src, ver, _ := r.Applied("router.link.v1")
	req.Equal(SourceLocal, src)
	req.Equal(1, ver)
}

func Test_Source_LocalMultiVersion(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	// Both local versions present; highest wins within the source.
	req.NoError(r.ApplyLocal("router.link.v1", `local-v1`))
	r.WaitForIdle()
	req.NoError(r.ApplyLocal("router.link.v2", `local-v2`))
	r.WaitForIdle()

	src, ver, _ := r.Applied("router.link.v1")
	req.Equal(SourceLocal, src)
	req.Equal(2, ver)
}

func Test_Source_LocalUnsupportedVersion_NothingApplies(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	// Controller has v2 that the handler supports.
	req.NoError(r.ApplyController("router.link.v2", `ctrl-v2`))
	r.WaitForIdle()
	req.Equal(2, r.AppliedVersion("router.link.v1"))

	// Operator sets local at v3 — handler doesn't support v3. Local-wins is
	// strict: controller's v2 must NOT silently take over. Subsystem should
	// reconcile to "nothing applied," surfacing the problem.
	req.NoError(r.ApplyLocal("router.link.v3", `local-v3`))
	r.WaitForIdle()

	_, _, found := r.Applied("router.link.v1")
	req.False(found, "local-but-unsupported should not silently fall back to controller")
}

func Test_ConfigSource_String(t *testing.T) {
	req := require.New(t)
	req.Equal("controller", SourceController.String())
	req.Equal("local", SourceLocal.String())
}

// --- Inspect ----------------------------------------------------------------

func Test_Inspect_Empty(t *testing.T) {
	req := require.New(t)
	r, _ := newSealedRegistry(t)
	snap := r.Inspect()
	req.True(snap.Sealed)
	req.False(snap.Closed)
	req.Empty(snap.Handlers)
}

func Test_Inspect_HandlersSortedByBase(t *testing.T) {
	req := require.New(t)
	h1 := &fakeHandler{base: "router.xgress.proxy", versions: []int{1}}
	h2 := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	h3 := &fakeHandler{base: "router.forwarder", versions: []int{1}}
	r, _ := newSealedRegistry(t, h1, h2, h3)

	snap := r.Inspect()
	req.Len(snap.Handlers, 3)
	req.Equal("router.forwarder", snap.Handlers[0].BaseType)
	req.Equal("router.link", snap.Handlers[1].BaseType)
	req.Equal("router.xgress.proxy", snap.Handlers[2].BaseType)
}

func Test_Inspect_ReportsControllerAndLocalAndApplied(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1, 2}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyController("router.link.v1", `{"src":"ctrl","v":1}`))
	r.WaitForIdle()
	req.NoError(r.ApplyController("router.link.v2", `{"src":"ctrl","v":2}`))
	r.WaitForIdle()
	req.NoError(r.ApplyLocal("router.link.v1", `{"src":"local","v":1}`))
	r.WaitForIdle()

	snap := r.Inspect()
	req.Len(snap.Handlers, 1)
	hi := snap.Handlers[0]
	req.Equal("router.link", hi.BaseType)
	req.Equal([]int{1, 2}, hi.SupportedVersions)
	req.Len(hi.ControllerConfigs, 2)
	req.Equal(1, hi.ControllerConfigs[0].Version)
	req.Equal(map[string]any{"src": "ctrl", "v": float64(1)}, hi.ControllerConfigs[0].Data)
	req.Equal(2, hi.ControllerConfigs[1].Version)
	req.Equal(map[string]any{"src": "ctrl", "v": float64(2)}, hi.ControllerConfigs[1].Data)
	req.NotNil(hi.LocalConfig)
	req.Equal(1, hi.LocalConfig.Version)
	req.Equal(map[string]any{"src": "local", "v": float64(1)}, hi.LocalConfig.Data)
	req.NotNil(hi.Applied)
	req.Equal("local", hi.Applied.Source)
	req.Equal(1, hi.Applied.Version)
}

func Test_Inspect_LocalConfigJsonKeyIsCamelCase(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)

	req.NoError(r.ApplyLocal("router.link.v1", `{"src":"local","v":1}`))
	r.WaitForIdle()

	buf, err := json.Marshal(r.Inspect().Handlers[0])
	req.NoError(err)
	req.Contains(string(buf), `"localConfig"`)
	req.NotContains(string(buf), `"localconfig"`)
}

func Test_Inspect_JSON(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)
	req.NoError(r.ApplyController("router.link.v1", `{"hello":"world"}`))
	r.WaitForIdle()

	b, err := json.Marshal(r.Inspect())
	req.NoError(err)
	// Spot-check the marshaled output for the expected keys; not a full
	// schema validation, just enough to confirm the struct tags work and that
	// the config payload is inlined as parsed JSON rather than a quoted string.
	out := string(b)
	req.Contains(out, `"sealed":true`)
	req.Contains(out, `"baseType":"router.link"`)
	req.Contains(out, `"applied":{"source":"controller","version":1}`)
	req.Contains(out, `"data":{"hello":"world"}`)
}

// --- Concurrency -------------------------------------------------------------

func Test_DifferentHandlersReconcileInParallel(t *testing.T) {
	req := require.New(t)
	slowGate := make(chan struct{})
	slow := &slowHandler{base: "router.slow", versions: []int{1}, gate: slowGate}
	fast := &fakeHandler{base: "router.fast", versions: []int{1}}
	r, _ := newSealedRegistry(t, slow, fast)

	req.NoError(r.ApplyController("router.slow.v1", `a`))
	req.NoError(r.ApplyController("router.fast.v1", `b`))

	pollUntil(t, func() bool {
		applies, _ := fast.snapshot()
		return len(applies) == 1
	})

	close(slowGate)
	r.WaitForIdle()

	slowApplies, _ := slow.snapshot()
	req.Len(slowApplies, 1)
}

type slowHandler struct {
	mu       sync.Mutex
	base     string
	versions []int
	applies  []applyCall
	gate     chan struct{}
}

func (s *slowHandler) BaseType() string         { return s.base }
func (s *slowHandler) SupportedVersions() []int { return s.versions }
func (s *slowHandler) Apply(version int, data string) error {
	<-s.gate
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applies = append(s.applies, applyCall{version, data})
	return nil
}
func (s *slowHandler) Remove() error { return nil }
func (s *slowHandler) snapshot() (applies []applyCall, removes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]applyCall, len(s.applies))
	copy(out, s.applies)
	return out, 0
}

func pollUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within 5s")
}

// --- Default alert -----------------------------------------------------------

func Test_Default_AlertLogs(t *testing.T) {
	r := NewRegistry(nil)
	h := &fakeHandler{base: "x", versions: []int{1}, applyErrs: []error{errors.New("nope")}}
	require.NoError(t, r.Register(h))
	r.Seal()
	require.NoError(t, r.ApplyController("x.v1", `{}`))
	r.WaitForIdle()
}

// --- Close -------------------------------------------------------------------

func Test_Close_DrainsInFlight(t *testing.T) {
	req := require.New(t)
	gate := make(chan struct{})
	slow := &slowHandler{base: "router.slow", versions: []int{1}, gate: gate}
	r, _ := newSealedRegistry(t, slow)

	req.NoError(r.ApplyController("router.slow.v1", `a`))

	closeDone := make(chan struct{})
	go func() {
		r.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		t.Fatal("Close returned before in-flight reconcile completed")
	default:
	}

	close(gate)
	<-closeDone
}

func Test_Close_PreventsNewSpawns(t *testing.T) {
	req := require.New(t)
	h := &fakeHandler{base: "router.link", versions: []int{1}}
	r, _ := newSealedRegistry(t, h)
	r.Close()

	// After Close, Apply still records data but does not spawn a reconcile.
	req.NoError(r.ApplyController("router.link.v1", `{}`))
	applies, _ := h.snapshot()
	req.Empty(applies, "no apply should be observed after Close")
}
