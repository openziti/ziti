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
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
)

// GetCapabilities reads the capabilities bitmask from underlay headers, checking
// the current header ID first, then falling back to the legacy ID for pre-2.0
// compatibility. T selects the capability namespace to interpret the mask as
// (e.g. RouterCapability or ControllerCapability).
func GetCapabilities[T ~int](headers map[int32][]byte) *Mask[T] {
	if val, found := headers[int32(ctrl_pb.ControlHeaders_CapabilitiesHeader)]; found {
		return MaskFromBytes[T](val)
	}
	if val, found := headers[ctrl_pb.LegacyCapabilitiesHeader]; found {
		return MaskFromBytes[T](val)
	}
	return NewMask[T]()
}

func IsCapable[T ~int](headers map[int32][]byte, capability T) bool {
	return GetCapabilities[T](headers).IsSet(capability)
}
