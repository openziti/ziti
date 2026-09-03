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

package db

import (
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
)

const (
	dirtyProcessHash        = "3C:DA:EF:ED:01:38:A1:D0:1D:F9:AC:5C:8A:57:F0:2B:29:C2:4A:31:20:16:14:C5:1A:59:2E:EE:D2:E5:F7:F3"
	cleanProcessHash        = "3cdaefed0138a1d01df9ac5c8a57f02b29c24a31201614c51a592eeed2e5f7f3"
	dirtyProcessFingerprint = "F1B2A6E9A37DFC918BD495E79B03DBBE6CB7477E3C6A0C29FF476C2B9A43AD0F\n"
	cleanProcessFingerprint = "f1b2a6e9a37dfc918bd495e79b03dbbe6cb7477e3c6a0c29ff476c2b9a43ad0f"

	testProcessOsType = "Windows"
	testProcessPath   = "C:\\example\\path\\1.exe"
)

// newProcessPostureCheck builds a PROCESS posture check carrying the given configured hashes and
// signer fingerprint.
func newProcessPostureCheck(hashes []string, fingerprint string) *PostureCheck {
	return &PostureCheck{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		TypeId:        PostureCheckTypeProcess,
		SubType: &PostureCheckProcess{
			OperatingSystem: testProcessOsType,
			Path:            testProcessPath,
			Hashes:          hashes,
			Fingerprint:     fingerprint,
		},
	}
}

// newProcessMultiPostureCheck builds a PROCESS_MULTI posture check with a single process carrying
// the given configured hashes and signer fingerprints.
func newProcessMultiPostureCheck(hashes, fingerprints []string) *PostureCheck {
	return &PostureCheck{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		TypeId:        PostureCheckTypeProcessMulti,
		SubType: &PostureCheckProcessMulti{
			Semantic: SemanticAllOf,
			Processes: []*ProcessMulti{
				{
					OsType:             testProcessOsType,
					Path:               testProcessPath,
					Hashes:             hashes,
					SignerFingerprints: fingerprints,
				},
			},
		},
	}
}

// Test_PostureCheckProcessStore_NormalizesConfiguredValues locks in that a PROCESS check's
// configured hashes and signer fingerprint are stored as lowercase unseparated hex, whatever case
// and separator style they were configured in.
func Test_PostureCheckProcessStore_NormalizesConfiguredValues(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	check := newProcessPostureCheck([]string{dirtyProcessHash}, dirtyProcessFingerprint)

	boltztest.RequireCreate(ctx, check)
	boltztest.RequireReload(ctx, check)

	stored := check.SubType.(*PostureCheckProcess)
	ctx.Equal([]string{cleanProcessHash}, stored.Hashes)
	ctx.Equal(cleanProcessFingerprint, stored.Fingerprint)
}

// Test_PostureCheckProcessMultiStore_NormalizesConfiguredValues locks in the same for a
// PROCESS_MULTI check, whose configured values are stored per process.
func Test_PostureCheckProcessMultiStore_NormalizesConfiguredValues(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	check := newProcessMultiPostureCheck([]string{dirtyProcessHash}, []string{dirtyProcessFingerprint})

	boltztest.RequireCreate(ctx, check)
	boltztest.RequireReload(ctx, check)

	stored := check.SubType.(*PostureCheckProcessMulti)
	ctx.Require().Len(stored.Processes, 1)
	ctx.Equal([]string{cleanProcessHash}, stored.Processes[0].Hashes)
	ctx.Equal([]string{cleanProcessFingerprint}, stored.Processes[0].SignerFingerprints)
}

// Test_PostureCheckProcessStore_NormalizesStoredValuesOnLoad locks in that a PROCESS check
// persisted before normalization reads back normalized, without waiting for the check to be saved
// again.
func Test_PostureCheckProcessStore_NormalizesStoredValuesOnLoad(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	check := newProcessPostureCheck([]string{cleanProcessHash}, cleanProcessFingerprint)
	boltztest.RequireCreate(ctx, check)

	err := ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
		bucket := ctx.stores.PostureCheck.GetEntityBucket(mc.Tx(), []byte(check.Id))
		ctx.Require().NotNil(bucket)

		typeBucket := bucket.GetOrCreateBucket(PostureCheckTypeProcess)
		typeBucket.SetStringList(FieldPostureCheckProcessHashes, []string{dirtyProcessHash}, nil)
		typeBucket.SetString(FieldPostureCheckProcessFingerprint, dirtyProcessFingerprint, nil)

		return typeBucket.GetError()
	})
	ctx.Require().NoError(err)

	boltztest.RequireReload(ctx, check)

	stored := check.SubType.(*PostureCheckProcess)
	ctx.Equal([]string{cleanProcessHash}, stored.Hashes)
	ctx.Equal(cleanProcessFingerprint, stored.Fingerprint)
}

// Test_PostureCheckProcessMultiStore_NormalizesStoredValuesOnLoad locks in the same for a
// PROCESS_MULTI check.
func Test_PostureCheckProcessMultiStore_NormalizesStoredValuesOnLoad(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	check := newProcessMultiPostureCheck([]string{cleanProcessHash}, []string{cleanProcessFingerprint})
	boltztest.RequireCreate(ctx, check)

	err := ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
		bucket := ctx.stores.PostureCheck.GetEntityBucket(mc.Tx(), []byte(check.Id))
		ctx.Require().NotNil(bucket)

		typeBucket := bucket.GetOrCreateBucket(PostureCheckTypeProcessMulti)
		processesBucket := typeBucket.GetOrCreateBucket(FieldPostureCheckProcessMultiProcesses)
		procBucket := processesBucket.GetOrCreateBucket(testProcessOsType + "-" + testProcessPath)
		procBucket.SetStringList(FieldPostureCheckProcessMultiHashes, []string{dirtyProcessHash}, nil)
		procBucket.SetStringList(FieldPostureCheckProcessMultiSignerFingerprints, []string{dirtyProcessFingerprint}, nil)

		return procBucket.GetError()
	})
	ctx.Require().NoError(err)

	boltztest.RequireReload(ctx, check)

	stored := check.SubType.(*PostureCheckProcessMulti)
	ctx.Require().Len(stored.Processes, 1)
	ctx.Equal([]string{cleanProcessHash}, stored.Processes[0].Hashes)
	ctx.Equal([]string{cleanProcessFingerprint}, stored.Processes[0].SignerFingerprints)
}
