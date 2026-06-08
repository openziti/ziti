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

package cert

import (
	"math/big"
	"testing"
)

func TestDefaultSerialGenerator(t *testing.T) {
	gen := DefaultSerialGenerator{}

	// 2^159 - 1 is the inclusive maximum (20-octet, positive ceiling per RFC 5280).
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 159), big.NewInt(1))
	one := big.NewInt(1)

	for range 10000 {
		serial, err := gen.Generate()
		if err != nil {
			t.Fatalf("unexpected error generating serial: %v", err)
		}

		// must be strictly positive
		if serial.Sign() <= 0 {
			t.Fatalf("serial must be > 0, got %s", serial)
		}

		// must not exceed the positive 20-octet ceiling
		if serial.Cmp(max) > 0 {
			t.Fatalf("serial %s exceeds 2^159 - 1", serial)
		}

		// at least 1, at most 1: confirm strictly positive lower bound holds
		if serial.Cmp(one) < 0 {
			t.Fatalf("serial %s below 1", serial)
		}

		// the DER encoding (signed two's-complement, minimal) must be <= 20 octets
		if octets := len(serial.Bytes()); octets > 20 {
			// big.Int.Bytes() is unsigned big-endian magnitude; for a value <= 2^159-1
			// the top magnitude bit is 0, so no sign-pad octet is added by DER and the
			// magnitude length equals the DER content length.
			t.Fatalf("serial %s encodes to %d octets, exceeds 20", serial, octets)
		}
	}
}
