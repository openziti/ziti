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

package xlink_transport

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSplitImpl_IsClosedOnHalfBuiltLink: the listener binds a split link's two channels independently, as
// each arrives, so the link exists with only one of them set. Close has always skipped a nil channel, while
// IsClosed dereferenced both, which made asking whether such a link was closed a panic.
func TestSplitImpl_IsClosedOnHalfBuiltLink(t *testing.T) {
	req := require.New(t)

	req.NotPanics(func() { _ = (&splitImpl{}).IsClosed() })
	req.False((&splitImpl{}).IsClosed(), "a link with neither channel bound is not closed")
}
