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

import "math/big"

// maskBits is the width, in bit positions, of a capability mask. Negative
// capabilities index down from the top of this range, so -1 is bit maskBits-1.
const maskBits = 64

// Mask is a capability bitmask parameterized by the capability type T (any
// integer-kinded type, e.g. RouterCapability or ControllerCapability). It
// encapsulates a big.Int so every set and check runs through one
// capability-to-bit-position translation:
//
//   - a non-negative capability is its own bit position, numbered upward from
//     bit 1;
//   - a negative capability indexes down from the top of the mask, so -1 is bit
//     maskBits-1, -2 is maskBits-2, and so on.
//
// Because the two groups grow toward each other from opposite ends, values from
// different sources cannot collide without either side seeing the other's
// numbering. The zero value is not usable; construct a Mask with NewMask or
// MaskFromBytes.
type Mask[T ~int] struct {
	bits *big.Int
}

// NewMask returns a Mask with the given capabilities set.
func NewMask[T ~int](capabilities ...T) *Mask[T] {
	m := &Mask[T]{bits: &big.Int{}}
	for _, capability := range capabilities {
		m.Set(capability)
	}
	return m
}

// MaskFromBytes returns a Mask decoded from the big-endian bytes produced by
// Bytes, e.g. a capabilities header read off a channel.
func MaskFromBytes[T ~int](b []byte) *Mask[T] {
	return &Mask[T]{bits: new(big.Int).SetBytes(b)}
}

// Set turns on the given capability and returns the Mask so calls can be chained,
// e.g. NewMask[RouterCapability]().Set(a).Set(b).
func (m *Mask[T]) Set(capability T) *Mask[T] {
	m.bits.SetBit(m.bits, bitPosition(capability), 1)
	return m
}

// IsSet reports whether the given capability is set. A nil Mask reports false.
func (m *Mask[T]) IsSet(capability T) bool {
	if m == nil || m.bits == nil {
		return false
	}
	return m.bits.Bit(bitPosition(capability)) == 1
}

// Bytes returns the big-endian byte encoding of the mask, suitable for a
// capabilities header. It round-trips through MaskFromBytes.
func (m *Mask[T]) Bytes() []byte {
	return m.bits.Bytes()
}

// bitPosition maps a capability value to its bit position. Non-negative values
// are used as-is; negative values count down from the top of the mask, so -1 is
// bit maskBits-1, -2 is maskBits-2, and so on.
func bitPosition[T ~int](capability T) int {
	if capability < 0 {
		return maskBits + int(capability)
	}
	return int(capability)
}
