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

package table

import (
	"math"
	"strings"
)

const (
	ALIGN_LEFT   = 0
	ALIGN_CENTER = 1
	ALIGN_RIGHT  = 2
)

func Pad(s, pad string, width int, align int) string {
	switch align {
	case ALIGN_CENTER:
		return PadCenter(s, pad, width)
	case ALIGN_RIGHT:
		return PadLeft(s, pad, width)
	default:
		return PadRight(s, pad, width)
	}
}

func PadRight(s, pad string, width int) string {
	gap := width - len(s)
	if gap > 0 {
		return s + strings.Repeat(pad, gap)
	}
	return s
}

func PadLeft(s, pad string, width int) string {
	gap := width - len(s)
	if gap > 0 {
		return strings.Repeat(pad, gap) + s
	}
	return s
}

func PadCenter(s, pad string, width int) string {
	gap := width - len(s)
	if gap > 0 {
		gapLeft := int(math.Ceil(float64(gap) / 2))
		gapRight := gap - gapLeft
		return strings.Repeat(pad, gapLeft) + s + strings.Repeat(pad, gapRight)
	}
	return s
}
