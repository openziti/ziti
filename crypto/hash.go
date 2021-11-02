/*
	Copyright NetFoundry, Inc.

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

package crypto

import (
	"crypto/rand"
	"encoding/binary"
	"github.com/michaelquigley/pfxlog"
	"golang.org/x/crypto/argon2"
)

type HashResult struct {
	Hash []byte
	Salt []byte
}

func salt() []byte {
	buf := make([]byte, binary.MaxVarintLen64)
	_, err := rand.Read(buf)

	if err != nil {
		pfxlog.Logger().Panic(err)
	}

	return buf
}

func Hash(password string) *HashResult {
	s := salt()
	return ReHash(password, s)
}

func ReHash(password string, s []byte) *HashResult {
	h := argon2.IDKey([]byte(password), s, 1, 3*1024, 4, 32)

	return &HashResult{
		Hash: h,
		Salt: s,
	}
}
