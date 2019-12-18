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
)

// EQ

func TestParse_Single_Equal_String_NoEscapes(t *testing.T) {
	testSingleStringEqual(t, `a = "A simple string"`, "a", "A simple string")
}

func TestParse_Single_Equal_String_Escapes(t *testing.T) {
	testSingleStringEqual(t, `a = "A simple\" \\ \n \t \r string"`, "a", "A simple\" \\ \n \t \r string")
}

func TestParse_Single_Equal_String_Whitespace(t *testing.T) {
	testSingleStringEqual(t, "a \n \t = \n \t \"A simple string\"    ", "a", "A simple string")
}

// Not EQ

func TestParse_Single_NotEqual_String_NoEscapes(t *testing.T) {
	testSingleStringNotEqual(t, `a != "A simple string"`, "a", "A simple string")
}

func TestParse_Single_NotEqual_String_Escapes(t *testing.T) {
	testSingleStringNotEqual(t, `a != "A simple\" \\ \n \t \r string"`, "a", "A simple\" \\ \n \t \r string")
}

func TestParse_Single_NotEqual_String_Whitespace(t *testing.T) {
	testSingleStringNotEqual(t, "a \n \t != \n \t \"A simple string\"    ", "a", "A simple string")
}

// Contains

func TestParse_Single_Contains_String_NoEscapes(t *testing.T) {
	testSingleStringContains(t, `a CONTAINS "A simple string"`, "a", "%A simple string%")
}

func TestParse_Single_Contains_String_Escapes(t *testing.T) {
	testSingleStringContains(t, `a CONTAINS "A simple\" \\ \n \t \r string"`, "a", "%A simple\" \\ \n \t \r string%")
}

func TestParse_Single_Contains_String_Whitespace(t *testing.T) {
	testSingleStringContains(t, "a \n \t CONTAINS \n \t \"A simple string\"    ", "a", "%A simple string%")
}

// Not Contains

func TestParse_Single_NotContains_String_NoEscapes(t *testing.T) {
	testSingleStringNotContains(t, `a not CONTAINS "A simple string"`, "a", "%A simple string%")
}

func TestParse_Single_NotContains_String_Escapes(t *testing.T) {
	testSingleStringNotContains(t, `a NoT CONTAINS "A simple\" \\ \n \t \r string"`, "a", "%A simple\" \\ \n \t \r string%")
}

func TestParse_Single_NotContains_String_Whitespace(t *testing.T) {
	testSingleStringNotContains(t, "a \n \t NOT \n CONTAINS \n \t \"A simple string\"    ", "a", "%A simple string%")
}

// Array

func TestParse_Single_InArray_String_OneElement(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a in ["a string 1"]`, listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE a IN (?)", sql)

	opSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, opSet)

	op, ok := opSet[InOp]
	assert.True(t, ok)
	assert.True(t, op)

	value := "a string 1"

	assert.Equal(t, 1, len(args))
	assert.Equal(t, value, args[0])

}

func TestParse_Single_InArray_String_MultiElement(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(`a in ["a string 1 \\ ", " \n some other @!#%#,.<>?}{{[]|$^()!)!)+_ string ", " \t ]Ƈ{⚊ŖÂɅ£ȮɅǝŠŗ7😤ǲ😂♧😴😈😮😆Â😅ā´"]`, listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE a IN (?,?,?)", sql)

	opSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, opSet)

	op, ok := opSet[InOp]
	assert.True(t, ok)
	assert.True(t, op)

	value1 := `a string 1 \ `
	value2 := " \n some other @!#%#,.<>?}{{[]|$^()!)!)+_ string "
	value3 := " \t ]Ƈ{⚊ŖÂɅ£ȮɅǝŠŗ7😤ǲ😂♧😴😈😮😆Â😅ā´"

	assert.Equal(t, 3, len(args))
	assert.Equal(t, value1, args[0])
	assert.Equal(t, value2, args[1])
	assert.Equal(t, value3, args[2])

}

// Helpers

func testSingleStringEqual(t *testing.T, input, identifier, value string) {
	testSingleStringOp(t, input, identifier, "SELECT * FROM test WHERE a = ?", value, EqOp)
}

func testSingleStringNotEqual(t *testing.T, input, identifier, value string) {
	testSingleStringOp(t, input, identifier, "SELECT * FROM test WHERE a <> ?", value, NeqOp)
}

func testSingleStringContains(t *testing.T, input, identifier, value string) {
	testSingleStringOp(t, input, identifier, "SELECT * FROM test WHERE a LIKE ?", value, ContainsOp)
}

func testSingleStringNotContains(t *testing.T, input, identifier, value string) {
	testSingleStringOp(t, input, identifier, "SELECT * FROM test WHERE a NOT LIKE ?", value, ContainsOp)
}

func testSingleStringOp(t *testing.T, input, identifier, output string, value interface{}, opType OpType) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse(input, listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, output, sql)

	opSet, ok := listener.IdentifierOps[identifier]
	assert.True(t, ok)
	assert.NotNil(t, opSet)

	op, ok := opSet[opType]
	assert.True(t, ok)
	assert.True(t, op)

	assert.Equal(t, 1, len(args))

	assert.Nil(t, err)
	assert.Equal(t, value, args[0])
}
