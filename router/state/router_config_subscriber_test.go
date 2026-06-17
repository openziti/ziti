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

package state

import (
	"sort"
	"testing"

	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/router/managedconfig"
	"github.com/stretchr/testify/require"
)

// --- allow-list subscriber -----------------------------------------------

type fakeAllow struct {
	allowed map[string]bool
}

func (f *fakeAllow) IsAllowed(configType string) bool { return f.allowed[configType] }

type fakeRegistryHandler struct {
	base     string
	versions []int
	applies  []applyRec
	removes  int
}

type applyRec struct {
	version int
	data    string
}

func (f *fakeRegistryHandler) BaseType() string                  { return f.base }
func (f *fakeRegistryHandler) SupportedVersions() []int          { return f.versions }
func (f *fakeRegistryHandler) Apply(v int, data string) error    { f.applies = append(f.applies, applyRec{v, data}); return nil }
func (f *fakeRegistryHandler) Remove() error                     { f.removes++; return nil }

func newSealedRegistry(t *testing.T, h *fakeRegistryHandler) *managedconfig.Registry {
	t.Helper()
	r := managedconfig.NewRegistry(nil)
	require.NoError(t, r.Register(h))
	r.Seal()
	return r
}

func Test_RouterConfigSubscriber_AppliedAllowedReachesRegistry(t *testing.T) {
	req := require.New(t)
	h := &fakeRegistryHandler{base: "router.link", versions: []int{1}}
	r := newSealedRegistry(t, h)
	allow := &fakeAllow{allowed: map[string]bool{"router.link.v1": true}}

	sub := newRouterConfigSubscriberFromParts(allow, r)
	sub.OnRouterConfigApplied("router.link.v1", `{"k":"v"}`)
	r.WaitForIdle()

	req.Len(h.applies, 1)
	req.Equal(1, h.applies[0].version)
	req.Equal(`{"k":"v"}`, h.applies[0].data)
}

func Test_RouterConfigSubscriber_AppliedDisallowedDropped(t *testing.T) {
	req := require.New(t)
	h := &fakeRegistryHandler{base: "router.link", versions: []int{1}}
	r := newSealedRegistry(t, h)
	allow := &fakeAllow{allowed: map[string]bool{}} // empty: nothing allowed

	sub := newRouterConfigSubscriberFromParts(allow, r)
	sub.OnRouterConfigApplied("router.link.v1", `{}`)
	r.WaitForIdle()

	req.Empty(h.applies)
}

func Test_RouterConfigSubscriber_RemovedAllowedReachesRegistry(t *testing.T) {
	req := require.New(t)
	h := &fakeRegistryHandler{base: "router.link", versions: []int{1}}
	r := newSealedRegistry(t, h)
	allow := &fakeAllow{allowed: map[string]bool{"router.link.v1": true}}

	sub := newRouterConfigSubscriberFromParts(allow, r)
	sub.OnRouterConfigApplied("router.link.v1", `{"k":"v"}`)
	r.WaitForIdle()
	sub.OnRouterConfigRemoved("router.link.v1")
	r.WaitForIdle()

	req.Equal(1, h.removes)
}

func Test_RouterConfigSubscriber_RemovedDisallowedDropped(t *testing.T) {
	req := require.New(t)
	h := &fakeRegistryHandler{base: "router.link", versions: []int{1}}
	r := newSealedRegistry(t, h)
	allow := &fakeAllow{allowed: map[string]bool{}}

	sub := newRouterConfigSubscriberFromParts(allow, r)
	sub.OnRouterConfigRemoved("router.link.v1")
	r.WaitForIdle()

	req.Equal(0, h.removes)
}

// --- diff helper tests ---------------------------------------------------

// recordingSub captures subscriber notifications for assertions. Order is
// preserved for applies; removes are sorted before comparison since map
// iteration is non-deterministic.
type recordingSub struct {
	applied []string
	removed []string
}

func (r *recordingSub) OnRouterConfigApplied(configType string, data string) {
	r.applied = append(r.applied, configType+"="+data)
}

func (r *recordingSub) OnRouterConfigRemoved(configType string) {
	r.removed = append(r.removed, configType)
}

func newRdmWithConfigs(routerId string, configs map[string]string, types map[string]string) *common.RouterDataModel {
	rdm := common.NewBareRouterDataModel(routerId)
	for typeId, typeName := range types {
		target := common.ConfigTypeTargetRouter
		// If typeName starts with "service.", treat as service target.
		if len(typeName) >= 8 && typeName[:8] == "service." {
			target = "service"
		}
		rdm.ConfigTypes.Set(typeId, &common.ConfigType{Id: typeId, Name: typeName, Target: target})
	}
	for cfgId, typeId := range configs {
		rdm.Configs.Set(cfgId, &common.Config{Id: cfgId, Name: cfgId, TypeId: typeId, DataJson: cfgId + "-data"})
	}
	return rdm
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func Test_DispatchRouterConfigDiff_BothNilNoOp(t *testing.T) {
	rec := &recordingSub{}
	dispatchRouterConfigDiff(nil, nil, rec)
	require.Empty(t, rec.applied)
	require.Empty(t, rec.removed)
}

func Test_DispatchRouterConfigDiff_NilSubscriberNoPanic(t *testing.T) {
	rdm := newRdmWithConfigs("r1",
		map[string]string{"cfg-A": "rt"},
		map[string]string{"rt": "router.link.v1"},
	)
	dispatchRouterConfigDiff(nil, rdm, nil)
	// no panic = pass
}

func Test_DispatchRouterConfigDiff_OnlyNewState_AllApplied(t *testing.T) {
	req := require.New(t)
	rec := &recordingSub{}
	newRdm := newRdmWithConfigs("r1",
		map[string]string{"cfg-A": "rt", "cfg-B": "rt2"},
		map[string]string{"rt": "router.link.v1", "rt2": "router.xgress.v1"},
	)
	dispatchRouterConfigDiff(nil, newRdm, rec)

	req.Empty(rec.removed)
	req.ElementsMatch([]string{"router.link.v1=cfg-A-data", "router.xgress.v1=cfg-B-data"}, rec.applied)
}

func Test_DispatchRouterConfigDiff_OnlyOldState_AllRemoved(t *testing.T) {
	req := require.New(t)
	rec := &recordingSub{}
	oldRdm := newRdmWithConfigs("r1",
		map[string]string{"cfg-A": "rt", "cfg-B": "rt2"},
		map[string]string{"rt": "router.link.v1", "rt2": "router.xgress.v1"},
	)
	dispatchRouterConfigDiff(oldRdm, nil, rec)

	req.Empty(rec.applied)
	req.ElementsMatch([]string{"router.link.v1", "router.xgress.v1"}, sortedCopy(rec.removed))
}

func Test_DispatchRouterConfigDiff_MixedAddRemoveKeep(t *testing.T) {
	req := require.New(t)
	rec := &recordingSub{}

	oldRdm := newRdmWithConfigs("r1",
		map[string]string{"cfg-keep": "rt-link", "cfg-gone": "rt-gone"},
		map[string]string{"rt-link": "router.link.v1", "rt-gone": "router.gone.v1"},
	)
	newRdm := newRdmWithConfigs("r1",
		map[string]string{"cfg-keep": "rt-link", "cfg-new": "rt-new"},
		map[string]string{"rt-link": "router.link.v1", "rt-new": "router.new.v1"},
	)

	dispatchRouterConfigDiff(oldRdm, newRdm, rec)

	req.ElementsMatch([]string{"router.gone.v1"}, rec.removed)
	// cfg-keep is unchanged (same data) so it should NOT be re-applied; only
	// the new config dispatches.
	req.ElementsMatch([]string{"router.new.v1=cfg-new-data"}, rec.applied)
}

func Test_DispatchRouterConfigDiff_UnchangedConfigsSkipped(t *testing.T) {
	req := require.New(t)
	rec := &recordingSub{}

	// Both RDMs have the same configs with the same data. Diff must be empty.
	oldRdm := newRdmWithConfigs("r1",
		map[string]string{"cfg-A": "rt-1", "cfg-B": "rt-2"},
		map[string]string{"rt-1": "router.a.v1", "rt-2": "router.b.v1"},
	)
	newRdm := newRdmWithConfigs("r1",
		map[string]string{"cfg-A": "rt-1", "cfg-B": "rt-2"},
		map[string]string{"rt-1": "router.a.v1", "rt-2": "router.b.v1"},
	)

	dispatchRouterConfigDiff(oldRdm, newRdm, rec)

	req.Empty(rec.removed)
	req.Empty(rec.applied)
}

func Test_DispatchRouterConfigDiff_ChangedDataReapplied(t *testing.T) {
	req := require.New(t)
	rec := &recordingSub{}

	oldRdm := common.NewBareRouterDataModel("r1")
	oldRdm.ConfigTypes.Set("rt", &common.ConfigType{Id: "rt", Name: "router.link.v1", Target: common.ConfigTypeTargetRouter})
	oldRdm.Configs.Set("cfg", &common.Config{Id: "cfg", Name: "cfg", TypeId: "rt", DataJson: `{"old":true}`})

	newRdm := common.NewBareRouterDataModel("r1")
	newRdm.ConfigTypes.Set("rt", &common.ConfigType{Id: "rt", Name: "router.link.v1", Target: common.ConfigTypeTargetRouter})
	newRdm.Configs.Set("cfg", &common.Config{Id: "cfg", Name: "cfg", TypeId: "rt", DataJson: `{"new":true}`})

	dispatchRouterConfigDiff(oldRdm, newRdm, rec)

	req.Empty(rec.removed)
	req.Equal([]string{`router.link.v1={"new":true}`}, rec.applied)
}

func Test_DispatchRouterConfigDiff_SkipsNonRouterTarget(t *testing.T) {
	req := require.New(t)
	rec := &recordingSub{}
	newRdm := newRdmWithConfigs("r1",
		map[string]string{"cfg-svc": "svc-type", "cfg-router": "rt"},
		map[string]string{"svc-type": "service.intercept.v1", "rt": "router.link.v1"},
	)
	dispatchRouterConfigDiff(nil, newRdm, rec)

	req.ElementsMatch([]string{"router.link.v1=cfg-router-data"}, rec.applied)
	req.Empty(rec.removed)
}

func Test_CollectRouterConfigTypes_NilRdm(t *testing.T) {
	req := require.New(t)
	out := collectRouterConfigTypes(nil)
	req.NotNil(out)
	req.Empty(out)
}
