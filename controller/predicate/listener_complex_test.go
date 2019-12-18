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
	"gopkg.in/Masterminds/squirrel.v1"
	"testing"
	"time"
)

func TestParse_Multi_NoGroups(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a = 1 and b = 2 or c = 3 and d = 4`, listener)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE ((a = ? AND b = ?) OR (c = ? AND d = ?))", sql)

	aOpSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, aOpSet)

	aOp, ok := aOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, aOp)

	bOpSet, ok := listener.IdentifierOps["b"]
	assert.True(t, ok)
	assert.NotNil(t, bOpSet)

	bOp, ok := bOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, bOp)

	cOpSet, ok := listener.IdentifierOps["c"]
	assert.True(t, ok)
	assert.NotNil(t, cOpSet)

	cOp, ok := cOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, cOp)

	dOpSet, ok := listener.IdentifierOps["d"]
	assert.True(t, ok)
	assert.NotNil(t, dOpSet)

	dOp, ok := dOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, dOp)

	assert.Equal(t, 4, len(args))

	assert.Equal(t, 1, args[0])
	assert.Equal(t, 2, args[1])
	assert.Equal(t, 3, args[2])
	assert.Equal(t, 4, args[3])

	assert.Len(t, pe, 0)

}

func TestParse_Multi_Groups(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`(a = 1 and b = 2) or (c = 3 and d = 4)`, listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE ((a = ? AND b = ?) OR (c = ? AND d = ?))", sql)

	aOpSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, aOpSet)

	aOp, ok := aOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, aOp)

	bOpSet, ok := listener.IdentifierOps["b"]
	assert.True(t, ok)
	assert.NotNil(t, bOpSet)

	bOp, ok := bOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, bOp)

	cOpSet, ok := listener.IdentifierOps["c"]
	assert.True(t, ok)
	assert.NotNil(t, cOpSet)

	cOp, ok := cOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, cOp)

	dOpSet, ok := listener.IdentifierOps["d"]
	assert.True(t, ok)
	assert.NotNil(t, dOpSet)

	dOp, ok := dOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, dOp)

	assert.Equal(t, 4, len(args))

	assert.Equal(t, 1, args[0])
	assert.Equal(t, 2, args[1])
	assert.Equal(t, 3, args[2])
	assert.Equal(t, 4, args[3])

}

func TestParse_Multi_PartialGroups(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`(a = 1 and b = 2) or c = 3 and d = 4`, listener)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE ((a = ? AND b = ?) OR (c = ? AND d = ?))", sql)

	aOpSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, aOpSet)

	aOp, ok := aOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, aOp)

	bOpSet, ok := listener.IdentifierOps["b"]
	assert.True(t, ok)
	assert.NotNil(t, bOpSet)

	bOp, ok := bOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, bOp)

	cOpSet, ok := listener.IdentifierOps["c"]
	assert.True(t, ok)
	assert.NotNil(t, cOpSet)

	cOp, ok := cOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, cOp)

	dOpSet, ok := listener.IdentifierOps["d"]
	assert.True(t, ok)
	assert.NotNil(t, dOpSet)

	dOp, ok := dOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, dOp)

	assert.Equal(t, 4, len(args))

	assert.Equal(t, 1, args[0])
	assert.Equal(t, 2, args[1])
	assert.Equal(t, 3, args[2])
	assert.Equal(t, 4, args[3])

	assert.Len(t, pe, 0)

}

func TestParse_Multi_ConsecutiveAnds(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`(a = 1 and b != false and c = true)`, listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE (a = ? AND b <> ? AND c = ?)", sql)

	aOpSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, aOpSet)

	aOp, ok := aOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, aOp)

	bOpSet, ok := listener.IdentifierOps["b"]
	assert.True(t, ok)
	assert.NotNil(t, bOpSet)

	bOp, ok := bOpSet[NeqOp]
	assert.True(t, ok)
	assert.True(t, bOp)

	cOpSet, ok := listener.IdentifierOps["c"]
	assert.True(t, ok)
	assert.NotNil(t, cOpSet)

	cOp, ok := cOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, cOp)

	argsOk := assert.Equal(t, 3, len(args))

	if argsOk {
		assert.Equal(t, 1, args[0])     //a
		assert.Equal(t, false, args[1]) //b
		assert.Equal(t, true, args[2])  //c
	}
}

func TestParse_Multi_MultiLevel(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`(a = 1 and b != 6.0221409e+23 oR (c > 3.14259265359 OR d >= -4 AND (e < -5.1567489615 or f <= -1.25e-14))) or g COnTAINs "a string" aNd h BeTWeeN datetime(1986-12-26T02:21:12Z) aNd datetime(1988-03-15T03:02:12-23:00)`, listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE (((a = ? AND b <> ?) OR (c > ? OR (d >= ? AND (e < ? OR f <= ?)))) OR (g LIKE ? AND h BETWEEN ? AND ?))", sql)

	aOpSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, aOpSet)

	aOp, ok := aOpSet[EqOp]
	assert.True(t, ok)
	assert.True(t, aOp)

	bOpSet, ok := listener.IdentifierOps["b"]
	assert.True(t, ok)
	assert.NotNil(t, bOpSet)

	bOp, ok := bOpSet[NeqOp]
	assert.True(t, ok)
	assert.True(t, bOp)

	cOpSet, ok := listener.IdentifierOps["c"]
	assert.True(t, ok)
	assert.NotNil(t, cOpSet)

	cOp, ok := cOpSet[GtOp]
	assert.True(t, ok)
	assert.True(t, cOp)

	dOpSet, ok := listener.IdentifierOps["d"]
	assert.True(t, ok)
	assert.NotNil(t, dOpSet)

	dOp, ok := dOpSet[GtEOp]
	assert.True(t, ok)
	assert.True(t, dOp)

	//
	eOpSet, ok := listener.IdentifierOps["e"]
	assert.True(t, ok)
	assert.NotNil(t, eOpSet)

	eOp, ok := eOpSet[LtOp]
	assert.True(t, ok)
	assert.True(t, eOp)

	fOpSet, ok := listener.IdentifierOps["f"]
	assert.True(t, ok)
	assert.NotNil(t, fOpSet)

	fOp, ok := fOpSet[LtEOp]
	assert.True(t, ok)
	assert.True(t, fOp)

	gOpSet, ok := listener.IdentifierOps["g"]
	assert.True(t, ok)
	assert.NotNil(t, gOpSet)

	gOp, ok := gOpSet[ContainsOp]
	assert.True(t, ok)
	assert.True(t, gOp)

	hOpSet, ok := listener.IdentifierOps["h"]
	assert.True(t, ok)
	assert.NotNil(t, hOpSet)

	hOp, ok := hOpSet[BetweenOp]
	assert.True(t, ok)
	assert.True(t, hOp)

	argsOk := assert.Equal(t, 9, len(args))

	if argsOk {
		assert.Equal(t, 1, args[0])             //a
		assert.Equal(t, 6.0221409e+23, args[1]) //b
		assert.Equal(t, 3.14259265359, args[2]) //c
		assert.Equal(t, -4, args[3])            //d
		assert.Equal(t, -5.1567489615, args[4]) //e
		assert.Equal(t, -1.25e-14, args[5])     //f
		assert.Equal(t, "%a string%", args[6])  //g

		h1, _ := time.Parse(time.RFC3339, "1986-12-26T02:21:12Z")
		h2, _ := time.Parse(time.RFC3339, "1988-03-15T03:02:12-23:00")

		assert.Equal(t, h1, args[7]) //h1
		assert.Equal(t, h2, args[8]) //h2
	}
}

func TestParse_Multi_MultipleOnSameIdentifier(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a = 1 and a != 2`, listener)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE (a = ? AND a <> ?)", sql)

	aOpSet1, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, aOpSet1)

	aOp1, ok := aOpSet1[EqOp]
	assert.True(t, ok)
	assert.True(t, aOp1)

	aOp2, ok := aOpSet1[NeqOp]
	assert.True(t, ok)
	assert.True(t, aOp2)

	assert.Equal(t, 2, len(args))

	assert.Equal(t, 1, args[0])
	assert.Equal(t, 2, args[1])

	assert.Len(t, pe, 0)

}
