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

package router

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// recordingAlerter implements env.Alerter; captures every ReportError call.
type recordingAlerter struct {
	calls []alertCall
}

type alertCall struct {
	message         string
	details         []string
	relatedEntities map[string]string
}

func (r *recordingAlerter) ReportError(message string, details []string, relatedEntities map[string]string) {
	r.calls = append(r.calls, alertCall{message: message, details: details, relatedEntities: relatedEntities})
}

func Test_ManagedConfigAlertCallback_ForwardsToReporter(t *testing.T) {
	req := require.New(t)
	rec := &recordingAlerter{}
	cb := newManagedConfigAlertCallback(rec)

	cb("router.link", "v1 (controller) initial apply failed: boom")

	req.Len(rec.calls, 1)
	req.Contains(rec.calls[0].message, "router.link")
	req.Contains(rec.calls[0].message, "v1 (controller) initial apply failed: boom")
	req.Equal(map[string]string{"configBaseType": "router.link"}, rec.calls[0].relatedEntities)
	req.Nil(rec.calls[0].details)
}

func Test_ManagedConfigAlertCallback_MultipleCallsAreIndependent(t *testing.T) {
	req := require.New(t)
	rec := &recordingAlerter{}
	cb := newManagedConfigAlertCallback(rec)

	cb("router.link", "first")
	cb("router.xgress", "second")

	req.Len(rec.calls, 2)
	req.Equal("router.link", rec.calls[0].relatedEntities["configBaseType"])
	req.Equal("router.xgress", rec.calls[1].relatedEntities["configBaseType"])
}
