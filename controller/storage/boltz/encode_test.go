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

package boltz

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

func TestEncodeDecodeStringSlice(t *testing.T) {
	req := require.New(t)
	for i := 0; i < 100; i++ {
		size := rand.Intn(10) + 1

		var ids []string
		for j := 0; j < size; j++ {
			ids = append(ids, uuid.New().String())
		}

		encoded, err := EncodeStringSlice(ids)
		req.NoError(err)

		decodedIds, err := DecodeStringSlice(encoded)
		req.NoError(err)
		req.Equal(ids, decodedIds)
	}
}
