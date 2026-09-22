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

package model

import (
	"fmt"
	"testing"

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/stretchr/testify/require"
)

// deferReportForTest stands in for a report arriving from a router, without needing a real one.
func deferReportForTest(mgr *LinkManager, linkId, destId, srcId string) {
	mgr.deferReport(&ctrl_pb.RouterLinks_RouterLink{
		Id:           linkId,
		DestRouterId: destId,
		LinkProtocol: "tls",
		Iteration:    1,
	}, &Router{BaseEntity: models.BaseEntity{Id: srcId}})
}

// Test_DeferReport_ReplacesRatherThanDuplicating: a router re-reports the same link on every notification
// cycle while a controller is behind, and each arrival is the same question, not a new one.
func Test_DeferReport_ReplacesRatherThanDuplicating(t *testing.T) {
	req := require.New(t)
	mgr := &LinkManager{}

	deferReportForTest(mgr, "l1", "unknown-dest", "src")
	deferReportForTest(mgr, "l1", "unknown-dest", "src")
	deferReportForTest(mgr, "l2", "unknown-dest", "src")

	req.Equal(2, mgr.DeferredReportCount())
}

// Test_DeferReport_IsBounded: a controller that never catches up is partitioned and cannot route, and every
// router announces its whole set again on reconnect, so the held set must not grow without limit.
func Test_DeferReport_IsBounded(t *testing.T) {
	req := require.New(t)
	mgr := &LinkManager{}

	for i := 0; i < maxDeferredReports+50; i++ {
		deferReportForTest(mgr, fmt.Sprintf("link-%d", i), "unknown-dest", "src")
	}

	req.Equal(maxDeferredReports, mgr.DeferredReportCount())
}

// Test_RetryDeferredReports_WaitsForCatchUp: retrying while still behind would reach the same wrong answer,
// so nothing is decided until the store is current.
func Test_RetryDeferredReports_WaitsForCatchUp(t *testing.T) {
	req := require.New(t)
	mgr := &LinkManager{}

	deferReportForTest(mgr, "l1", "unknown-dest", "src")
	mgr.RetryDeferredReports()

	req.Equal(1, mgr.DeferredReportCount(), "a held report must survive a retry that runs with no env")
}
