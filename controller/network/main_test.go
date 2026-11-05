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

package network

import (
	"os"
	"testing"

	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
)

// TestMain configures logging once for the whole package, keeping the path-finding perf tests quiet.
// GlobalInit writes package-level logger state, so calling it from inside a test races any goroutine a
// previous test left running that logs while shutting down: a TestContext's api-session heartbeat
// collector, for instance, does a final flush after its close notification, and that flush logs. Doing
// this before any test starts leaves no concurrent reader to race with.
func TestMain(m *testing.M) {
	pfxlog.GlobalInit(logrus.WarnLevel, pfxlog.DefaultOptions())
	os.Exit(m.Run())
}
