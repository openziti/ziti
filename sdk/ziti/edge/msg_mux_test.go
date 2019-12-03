/*
	Copyright 2019 Netfoundry, Inc.

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

package edge

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func Test_newMsgMux(t *testing.T) {
	mux := NewMsgMux()
	assert := require.New(t)
	assert.True(mux.running.Get())
	assert.False(mux.closed.Get())
	mux.Close()
	assert.NoError(mux.closed.WaitForState(true, time.Millisecond * 100, time.Millisecond * 5))
	assert.NoError(mux.running.WaitForState(false, time.Millisecond * 150, time.Millisecond * 5))
}
