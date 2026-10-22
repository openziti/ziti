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

package diagnostics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStackDumpWatch_SkipsADumpAfterTheHandlerReturned: a watchdog callback can start after the handler has
// returned and the timer was stopped. Dumping then records something else and spends the dump budget.
func TestStackDumpWatch_SkipsADumpAfterTheHandlerReturned(t *testing.T) {
	dumps := 0
	watch := &stackDumpWatch{dump: func() { dumps++ }}

	watch.disarm()
	watch.fire()

	require.Zero(t, dumps)
}

// TestStackDumpWatch_DumpsWhileTheHandlerRuns: the case the watchdog exists for, and a dump already under way
// is not undone by the handler returning during it.
func TestStackDumpWatch_DumpsWhileTheHandlerRuns(t *testing.T) {
	dumps := 0
	watch := &stackDumpWatch{dump: func() { dumps++ }}

	watch.fire()
	watch.disarm()
	watch.fire()

	require.Equal(t, 1, dumps)
}
