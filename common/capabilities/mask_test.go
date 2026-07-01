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

package capabilities

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskWidth(t *testing.T) {
	require.Equal(t, 64, maskBits)
}

// TestMaskPositiveCapabilities checks that non-negative capabilities use their
// value as the bit position.
func TestMaskPositiveCapabilities(t *testing.T) {
	req := require.New(t)
	mask := NewMask[int](1, 4)
	req.True(mask.IsSet(1))
	req.True(mask.IsSet(4))
	req.False(mask.IsSet(0))
	req.False(mask.IsSet(2))
}

// TestMaskNegativeCapabilities checks that negative capabilities index down from
// the top of the mask: -1 is bit 63, -2 is bit 62.
func TestMaskNegativeCapabilities(t *testing.T) {
	req := require.New(t)
	mask := NewMask[int](-1, -2)
	req.True(mask.IsSet(-1))
	req.True(mask.IsSet(-2))
	// -1 and -2 alias bits 63 and 62; the same positions read positively.
	req.True(mask.IsSet(63))
	req.True(mask.IsSet(62))
	req.False(mask.IsSet(-3))
}

// TestMaskNoCollision verifies that upward-numbered (shared) and downward-numbered
// (control-plane-only) capabilities never collide.
func TestMaskNoCollision(t *testing.T) {
	req := require.New(t)
	shared := NewMask[int](1, 2)
	req.False(shared.IsSet(-1))
	req.False(shared.IsSet(-2))

	controlOnly := NewMask[int](-1)
	req.False(controlOnly.IsSet(1))
	req.False(controlOnly.IsSet(2))
}

// TestMaskByteRoundTrip verifies Bytes and MaskFromBytes round-trip a mask that
// carries both a shared and a control-plane-only capability.
func TestMaskByteRoundTrip(t *testing.T) {
	req := require.New(t)
	original := NewMask(RouterMultiChannel, RouterConnectV2, -1)

	decoded := MaskFromBytes[RouterCapability](original.Bytes())
	req.True(decoded.IsSet(RouterMultiChannel))
	req.True(decoded.IsSet(RouterConnectV2))
	req.True(decoded.IsSet(-1))
	req.False(decoded.IsSet(3))
}

// TestMaskNilIsSet verifies a nil Mask reports nothing set rather than panicking.
func TestMaskNilIsSet(t *testing.T) {
	var mask *Mask[RouterCapability]
	require.False(t, mask.IsSet(RouterMultiChannel))
}
