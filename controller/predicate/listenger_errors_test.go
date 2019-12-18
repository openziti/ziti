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

package predicate

import (
	"github.com/netfoundry/ziti-edge/controller/zitiql"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParse_Error_MultipleAND(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a=1 AND AND b=2`, listener)

	assert.Len(t, pe, 1)
}

func TestParse_Error_MultipleOR(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a=1 or OR b=2`, listener)

	assert.Len(t, pe, 1)
}

func TestParse_Error_MissingOperator(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a 11`, listener)

	assert.Len(t, pe, 2)
}

func TestParse_Error_InvalidIdentifier(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`1a=1`, listener)

	assert.Len(t, pe, 1)
}

func TestParse_Error_InvalidStringForContains(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a contains true`, listener)

	assert.Len(t, pe, 1)
}

func TestParse_Error_InvalidTypeForBetween(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a between "1" and "2"`, listener)

	assert.Len(t, pe, 5)
}

func TestParse_Error_MisspelledBetween(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a betweeeeeeeeeeeeeeeeen 1 and 2`, listener)

	assert.Len(t, pe, 5)
}

func TestParse_Error_MisspelledContains(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a contaaaaaaaains 1 and 2`, listener)

	assert.Len(t, pe, 5)
}

func TestParse_Error_DanglingOperator(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a = 1 and b = `, listener)

	assert.Len(t, pe, 2)
}

func TestParse_Error_DanglingGroup(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`(a = 1 and b = 2`, listener)

	assert.Len(t, pe, 1)
}
