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

package ast

import (
	"github.com/michaelquigley/pfxlog"
	"reflect"
	"testing"
	"time"

	zitiql "github.com/openziti/storage/zitiql"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var _ Symbols = (*testSymbols)(nil)

type testSymbols struct {
	values  map[string]interface{}
	types   map[string]NodeType
	cursors map[string]*testSymbolsSetCursor
}

func (symbols *testSymbols) GetSetSymbolTypes(string) SymbolTypes {
	return nil
}

func (symbols *testSymbols) OpenSetCursorForQuery(string, Query) SetCursor {
	panic("not implemented")
}

func (symbols *testSymbols) IsSet(name string) (bool, bool) {
	value, found := symbols.values[name]
	if found && value != nil {
		t := reflect.TypeOf(value)
		isSlice := t.Kind() == reflect.Slice
		return isSlice, true
	}
	return false, found
}

func (symbols *testSymbols) OpenSetCursor(name string) SetCursor {
	value, found := symbols.values[name]
	if !found {
		pfxlog.Logger().Errorf("unknown symbol %v, should have been caught in symbol validation pass", name)
		return NewEmptyCursor()
	}

	cursor := &testSymbolsSetCursor{
		symbols: symbols,
		name:    name,
		slice:   reflect.ValueOf(value),
		index:   0,
	}

	symbols.cursors[name] = cursor
	return cursor
}

func (symbols *testSymbols) IsNil(name string) bool {
	value, found := symbols.values[name]
	if !found {
		panic(errors.Errorf("unknown symbol %v", name))
	}
	return value == nil
}

func (symbols *testSymbols) GetSymbolType(name string) (NodeType, bool) {
	value, found := symbols.values[name]
	if found {
		switch value.(type) {
		case int64:
			return NodeTypeInt64, true
		case float64:
			return NodeTypeFloat64, true
		case float32:
			return NodeTypeFloat64, true
		case string:
			return NodeTypeString, true
		case time.Time:
			return NodeTypeDatetime, true
		case bool:
			return NodeTypeBool, true
		}
	}
	nodeType, found := symbols.types[name]
	return nodeType, found
}

func (symbols *testSymbols) getValue(name string) interface{} {
	isSet, found := symbols.IsSet(name)
	if !found {
		panic(errors.Errorf("unknown symbol %v", name))
	}
	if isSet {
		cursor, found := symbols.cursors[name]
		if !found {
			panic(errors.Errorf("attempt to traverse set %v with no open cursor", name))
		}
		if !cursor.IsValid() {
			panic(errors.Errorf("attempt to traverse set %v with invalid cursor", name))
		}
		return cursor.CurrentValue()
	}

	value, found := symbols.values[name]
	if !found {
		panic(errors.Errorf("unknown symbol %v", name))
	}
	return value
}

func (symbols *testSymbols) EvalBool(name string) *bool {
	value := symbols.getValue(name)
	typedVal, ok := value.(bool)
	if ok {
		return &typedVal
	}
	panic(errors.Errorf("symbol %v not of type bool, is %v", name, reflect.TypeOf(value)))
}

func (symbols *testSymbols) EvalString(name string) *string {
	value := symbols.getValue(name)
	typedVal, ok := value.(string)
	if ok {
		return &typedVal
	}
	panic(errors.Errorf("symbol %v not of type string, is %v", name, reflect.TypeOf(value)))
}

func (symbols *testSymbols) EvalInt64(name string) *int64 {
	value := symbols.getValue(name)
	typedVal, ok := value.(int64)
	if ok {
		return &typedVal
	}
	panic(errors.Errorf("symbol %v not of type int64, is %v", name, reflect.TypeOf(value)))
}

func (symbols *testSymbols) EvalFloat64(name string) *float64 {
	value := symbols.getValue(name)
	float64Val, ok := value.(float64)
	if ok {
		return &float64Val
	}
	float32Val, ok := value.(float32)
	float64Val = float64(float32Val)
	if ok {
		return &float64Val
	}
	panic(errors.Errorf("symbol %v not of type float, is %v", name, reflect.TypeOf(value)))
}

func (symbols *testSymbols) EvalDatetime(name string) *time.Time {
	value := symbols.getValue(name)
	typedVal, ok := value.(time.Time)
	if ok {
		return &typedVal
	}
	panic(errors.Errorf("symbol %v not of type string, is %v", name, reflect.TypeOf(value)))
}

type testSymbolsSetCursor struct {
	symbols *testSymbols
	name    string
	slice   reflect.Value
	index   int
}

func (cursor *testSymbolsSetCursor) Next() {
	cursor.index++
}

func (cursor *testSymbolsSetCursor) IsValid() bool {
	return cursor.slice.Len() > cursor.index
}

func (cursor *testSymbolsSetCursor) Close() {
	delete(cursor.symbols.cursors, cursor.name)
}

func (cursor *testSymbolsSetCursor) Current() []byte {
	return cursor.CurrentValue().([]byte)
}

func (cursor *testSymbolsSetCursor) CurrentValue() interface{} {
	return cursor.slice.Index(cursor.index).Interface()
}

var ts = &testSymbols{
	values: map[string]interface{}{
		"a":        int64(1),
		"b":        1.0,
		"c":        2.5,
		"flag":     true,
		"n":        nil,
		"d":        func() time.Time { val, _ := time.Parse(time.RFC3339, "2032-09-03T15:36:50Z"); return val }(),
		"s":        "hello",
		"sn":       "123456789",
		"nci":      int64(123456789),
		"ncf":      123456789.123,
		"link.ids": []int64{123, 456, 789},
		"lunk.ids": []int64{},
	},
	types: map[string]NodeType{
		"n":        NodeTypeString,
		"link.ids": NodeTypeInt64,
		"lunk.ids": NodeTypeInt64,
	},
	cursors: map[string]*testSymbolsSetCursor{},
}

type testDef struct {
	name   string
	expr   string
	result bool
}

func TestBoolAndIsNullFilters(t *testing.T) {
	tests := []testDef{
		{"bool, result true", "flag", true},
		{"not bool, result false", "not flag", false},
		{"bool EQ, result true", "flag = true", true},
		{"bool EQ, result false", "flag = false", false},
		{"bool NEQ, result false", "flag != true", false},
		{"bool NEQ, result true", "flag != false", true},

		{"is nil, result true", "n = null", true},
		{"is nil, result false", "a = null", false},
		{"is not nil, result true", "a != null", true},
		{"is not nil, result false", "n != null", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runFilterTest(t, tt)
		})
	}
}

func TestIntFilters(t *testing.T) {
	tests := []testDef{
		{"int EQ, result true", "a = 1", true},
		{"int EQ, result false", "a = 2", false},
		{"int NEQ, result false", "a != 1", false},
		{"int NEQ, result true", "a != 2", true},
		{"int LT, result true", "a < 2", true},
		{"int LT, result false", "a < 1", false},
		{"int LT, result false", "a < 0", false},
		{"int LTE, result equal true", "a <= 1", true},
		{"int LTE, result less than true", "a <= 5", true},
		{"int LTE, result false", "a <= 0", false},
		{"int GT, result true", "a > 0", true},
		{"int GT, result false", "a > 1", false},
		{"int GT, result false", "a > 2", false},
		{"int GTE, result equal true", "a >= 1", true},
		{"int GTE, result greater than true", "a >= 0", true},
		{"int GTE, result false", "a >= 2", false},

		{"int in, result true", "a in [4, 2, 1, 4, 5]", true},
		{"int in, result false", "a in [4, 2, 0, 4, 5]", false},
		{"int not in, result true", "a not in [4, 2, 0, 4, 5]", true},
		{"int not in, result false", "a not in [4, 2, 1, 4, 5]", false},

		{"int between, result true", "a between 1 and 4", true},
		{"int between, result true", "a between 0 and 4", true},
		{"int between, result true", "a between 1 and 2", true},
		{"int between, result false", "a between 1 and 1", false},
		{"int between, result false", "a between 2 and 4", false},
		{"int between, result false", "a between 0 and 1", false},
		{"int between, result true", "a between 1.0 and 4", true},

		{"int not between, result true", "a not between 1 and 1", true},
		{"int not between, result true", "a not between 2 and 4", true},
		{"int not between, result true", "a not between 0 and 1", true},
		{"int not between, result false", "a not between 1 and 4", false},
		{"int not between, result false", "a not between 0 and 2", false},
		{"int not between, result false", "a not between 1 and 2", false},
		{"int not between, result true", "a not between 1.0 and 1.0", true},

		{"int EQ, with float result true", "a = 1.0", true},
		{"int EQ, with float result false", "a = 1.5", false},
		{"int LT, with float result true", "a < 4.5", true},
		{"int LT, with float result false", "a < 0.5", false},

		{"int in, result true", "a in [4.0, 2, 1, 3.0, 5]", true},
		{"int in, result true", "a in [4.0, 2, 1.0, 4, 5]", true},
		{"int in, result false", "a in [4, 2.0, 0, 4, 5]", false},
		{"int not in, result true", "a not in [4, 2, 0, 4, 5.0]", true},
		{"int not in, result false", "a not in [4, 2, 1.0, 4, 5]", false},
		{"int not in, result false", "a not in [4.3, 2.1, 1.0, 4.2, 5.9]", false},

		{"int64 contains, result true", `nci  contains 234`, true},
		{"int64 contains, result false", `nci  contains 321`, false},
		{"int64 not contains, result false", `nci not contains 234`, false},
		{"int64 not contains, result true", `nci not contains 321`, true},
		{"int64 not contains, result true", `nci not contains 321.123`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runFilterTest(t, tt)
		})
	}
}

func TestFloatFilters(t *testing.T) {
	tests := []testDef{
		{"float EQ, result true", "b = 1", true},
		{"float EQ, result false", "b = 2", false},
		{"float NEQ, result false", "b != 1", false},
		{"float NEQ, result true", "b != 2", true},
		{"float LT, result true", "b < 2", true},
		{"float LT, result false", "b < 1", false},
		{"float LT, result false", "b < 0", false},
		{"float LTE, result equal true", "b <= 1", true},
		{"float LTE, result less than true", "b <= 5", true},
		{"float LTE, result false", "b <= 0", false},
		{"float GT, result true", "b > 0", true},
		{"float GT, result false", "b > 1", false},
		{"float GT, result false", "b > 2", false},
		{"float GTE, result equal true", "b >= 1", true},
		{"float GTE, result greater than true", "b >= 0", true},
		{"float GTE, result false", "b >= 2", false},

		{"float in, result true", "b in [4, 2, 1, 4, 5]", true},
		{"float in, result false", "b in [4, 2, 0, 4, 5]", false},
		{"float not in, result true", "b not in [4, 2, 0, 4, 5]", true},
		{"float not in, result false", "b not in [4, 2, 1, 4, 5]", false},
		{"float in, result true", "b in [4.0, 2, 1, 4, 5]", true},
		{"float in, result false", "b in [4, 2, 0, 4, 5.0]", false},
		{"float not in, result true", "b not in [4, 2.0, 0, 4, 5]", true},
		{"float not in, result false", "b not in [4, 2, 1.0, 4, 5]", false},

		{"float EQ, result true", "c = 2.5", true},
		{"float EQ, result false", "c = 2.6", false},
		{"float NEQ, result false", "c != 2.5", false},
		{"float NEQ, result true", "c != 2.6", true},
		{"float LT, result true", "c < 2.6", true},
		{"float LT, result false", "c < 2.4", false},
		{"float LT, result false", "c < 2.5", false},
		{"float LTE, result equal true", "c <= 2.5", true},
		{"float LTE, result less than true", "c <= 2.6", true},
		{"float LTE, result false", "c <= 1.3", false},
		{"float GT, result true", "c > 0.5", true},
		{"float GT, result false", "c > 4.6", false},
		{"float GT, result false", "c > 4.5", false},
		{"float GTE, result equal true", "c >= 2.5", true},
		{"float GTE, result greater than true", "c >= 2.2", true},
		{"float GTE, result false", "c >= 2.50001", false},

		{"float in, result true", "c in [4.0, 2.0, 2.5, 3.0, 5.0]", true},
		{"float in, result true", "c in [4.0, 2.0, 2.5, 4.2, 5.2]", true},
		{"float in, result false", "c in [4.0, 2.0, 0, 4.6, 5.1]", false},
		{"float not in, result true", "c not in [4.1, 2.4, 0, 4.4, 5.0]", true},
		{"float not in, result false", "c not in [4, 2.5, 1.0, 4, 5]", false},

		{"float between, result true", "c between 2.5 and 3", true},
		{"float between, result true", "c between 0 and 4", true},
		{"float between, result true", "c between 2.5 and 2.6", true},
		{"float between, result false", "c between 2.5 and 2.5", false},
		{"float between, result false", "c between 3.0 and 4", false},
		{"float between, result false", "c between 0 and 2.5", false},

		{"float not between, result true", "c not between 2.5 and 2.5", true},
		{"float not between, result true", "c not between 3.1 and 4", true},
		{"float not between, result true", "c not between 0 and 2.5", true},
		{"float not between, result false", "c not between 2.5 and 4", false},
		{"float not between, result false", "c not between 0 and 4", false},
		{"float not between, result false", "c not between 2.5 and 2.6", false},
		{"float not between, result true", "c not between 0 and 2.5", true},

		{"float64 contains, result true", `ncf  contains 234`, true},
		{"float64 contains, result true", `ncf  contains 9.12`, true},
		{"float64 contains, result false", `ncf  contains 321`, false},
		{"float64 not contains, result false", `ncf not contains 234`, false},
		{"float64 not contains, result false", `ncf not contains 89.12`, false},
		{"float64 not contains, result true", `ncf not contains 321`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runFilterTest(t, tt)
		})
	}
}

func TestStringFilters(t *testing.T) {
	tests := []testDef{
		{"string EQ, result true", `s = "hello"`, true},
		{"string EQ, result false", `s = "hellooop"`, false},
		{"string NEQ, result false", `s != "hello"`, false},
		{"string NEQ, result true", `s != "world"`, true},

		{"string in, result true", `s in ["hello", "goodbye"]`, true},
		{"string in, result false", `s in ["hellooo", "goodbye"]`, false},
		{"string not in, result true", `s not in ["hellooo", "goodbye"]`, true},
		{"string not in, result false", `s not in ["hello", "goodbye"]`, false},

		{"string contains, result true", `s contains "hello"`, true},
		{"string contains, result true", `s contains "ello"`, true},
		{"string contains, result true", `s contains "hell"`, true},
		{"string contains, result true", `s contains "l"`, true},
		{"string contains, result false", `s contains "helloo"`, false},
		{"string contains, result false", `s contains "helli"`, false},
		{"string contains, result false", `s contains "lle"`, false},
		{"string contains, result false", `s contains 0`, false},

		{"string not contains, result false", `s not contains "hello"`, false},
		{"string not contains, result false", `s not contains "ello"`, false},
		{"string not contains, result false", `s not contains "hell"`, false},
		{"string not contains, result false", `s not contains "l"`, false},
		{"string not contains, result true", `s not contains "helloo"`, true},
		{"string not contains, result true", `s not contains "helli"`, true},
		{"string not contains, result true", `s not contains "lle"`, true},
		{"string not contains, result true", `s not contains 0`, true},

		{"string contains, result true", `sn  contains 234`, true},
		{"string contains, result false", `sn  contains 321`, false},
		{"string not contains, result false", `sn not contains 234`, false},
		{"string not contains, result true", `sn not contains 321`, true},
		{"string not contains, result true", `sn not contains 321.123`, true},

		/*
			{"string LT, result true", `s < "jello"`, true},.
			{"string LT, result false", `s < "cello"`, false},
			{"string LT, result false", `s < "hello"`, false},
			{"string LTE, result equal true", `s <= "hello"`, true},
			{"string LTE, result less than true", `s <= "jello"`, true},
			{"string LTE, result false", `s <= "cello"`, false},
			{"string GT, result true", `s > "cello"`, true},
			{"string GT, result false", `s > "hello"`, false},
			{"string GT, result false", `s > "jello"`, false},
			{"string GTE, result equal true", `s >= "hello"`, true},
			{"string GTE, result greater than true", `s >= "cello"`, true},
			{"string GTE, result false", `s >= "jello"`, false},
		*/
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runFilterTest(t, tt)
		})
	}
}

func TestDateFilters(t *testing.T) {
	tests := []testDef{
		{"date EQ, result true", "d = datetime(2032-09-03T15:36:50Z)", true},
		{"date EQ, result false", "d = datetime(1032-09-03T15:36:50Z)", false},
		{"date NEQ, result false", "d != datetime(2032-09-03T15:36:50Z)", false},
		{"date NEQ, result true", "d != datetime(2032-09-03T15:36:51Z)", true},
		{"date LT, result true", "d < datetime(2032-09-03T15:36:55Z)", true},
		{"date LT, result false", "d < datetime(2032-09-03T15:36:50Z)", false},
		{"date LT, result false", "d < datetime(2032-09-03T15:36:49Z)", false},
		{"date LTE, result true E", "d <= datetime(2032-09-03T15:36:50Z)", true},
		{"date LTE, result true LT", "d <= datetime(2032-09-03T15:36:55Z)", true},
		{"date LTE, result false", "d <= datetime(2032-09-03T15:36:49Z)", false},
		{"date GT, result true", "d > datetime(2032-09-03T15:36:45Z)", true},
		{"date GT, result false", "d > datetime(2032-09-03T15:36:50Z)", false},
		{"date GT, result false", "d > datetime(2032-09-03T15:36:51Z)", false},
		{"date GTE, result true E", "d >= datetime(2032-09-03T15:36:50Z)", true},
		{"date GTE, result true GT", "d >= datetime(2032-09-03T15:36:45Z)", true},
		{"date GTE, result false", "d >= datetime(2032-09-03T15:36:51Z)", false},

		{"date in, result true", "d in [datetime(2032-09-03T15:36:50Z), datetime(2032-09-03T15:36:52Z)]", true},
		{"date in, result false", "d in [datetime(2032-09-03T15:36:49Z), datetime(2032-09-03T15:36:52Z)]", false},
		{"date not in, result true", "d not in [datetime(2032-09-03T15:36:49Z), datetime(2032-09-03T15:36:52Z)]", true},
		{"date not in, result false", "d not in [datetime(2032-09-03T15:36:50Z), datetime(2032-09-03T15:36:52Z)]", false},

		{"date between, result true", "d between datetime(2032-09-03T15:36:45Z) and datetime(2032-09-03T15:36:52Z)", true},
		{"date between, result true", "d between datetime(2032-09-03T15:36:50Z) and datetime(2032-09-03T15:36:52Z)", true},
		{"date between, result false", "d between datetime(2032-09-03T15:36:50Z) and datetime(2032-09-03T15:36:50Z)", false},
		{"date between, result false", "d between datetime(2032-09-03T15:36:45Z) and datetime(2032-09-03T15:36:50Z)", false},
		{"date between, result false", "d between datetime(2032-09-03T15:36:45Z) and datetime(2032-09-03T15:36:47Z)", false},
		{"date between, result false", "d between datetime(2032-09-03T15:36:55Z) and datetime(2032-09-03T15:36:58Z)", false},

		{"date not between, result false", "d not between datetime(2032-09-03T15:36:45Z) and datetime(2032-09-03T15:36:52Z)", false},
		{"date not between, result false", "d not between datetime(2032-09-03T15:36:50Z) and datetime(2032-09-03T15:36:52Z)", false},
		{"date not between, result true", "d not between datetime(2032-09-03T15:36:50Z) and datetime(2032-09-03T15:36:50Z)", true},
		{"date not between, result true", "d not between datetime(2032-09-03T15:36:45Z) and datetime(2032-09-03T15:36:50Z)", true},
		{"date not between, result true", "d not between datetime(2032-09-03T15:36:45Z) and datetime(2032-09-03T15:36:47Z)", true},
		{"date not between, result true", "d not between datetime(2032-09-03T15:36:55Z) and datetime(2032-09-03T15:36:58Z)", true},

		{"and, result true", "a = 1 and b = 1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runFilterTest(t, tt)
		})
	}
}

func TestAndsAndOrsFilters(t *testing.T) {
	tests := []testDef{
		{"and 2 clause, result true", "a = 1 and b = 1", true},
		{"and 2 clause, result false", "a = 0 and b = 1", false},
		{"and 2 clause, result false", "a = 1 and b = 0", false},
		{"and 2 clause, result false", "a = 0 and b = 0", false},
		{"and 3 clause, result true", "a = 1 and b = 1 and c = 2.5", true},
		{"and 3 clause, result false", "a = 0 and b = 1 and c = 2.5", false},
		{"and 3 clause, result false", "a = 1 and b = 0 and c = 2.5", false},
		{"and 3 clause, result false", "a = 1 and b = 1 and c = 0", false},
		{"and 3 clause, result false", "a = 0 and b = 0 and c = 2.5", false},
		{"and 3 clause, result false", "a = 0 and b = 1 and c = 0", false},
		{"and 3 clause, result false", "a = 1 and b = 0 and c = 0", false},
		{"and 3 clause, result false", "a = 0 and b = 0 and c = 0", false},

		{"or 2 clause, result true", "a = 1 or b = 1", true},
		{"or 2 clause, result true", "a = 0 or b = 1", true},
		{"or 2 clause, result true", "a = 1 or b = 0", true},
		{"or 2 clause, result false", "a = 0 or b = 0", false},
		{"or 3 clause, result true", "a = 1 or b = 1 or c = 2.5", true},
		{"or 3 clause, result true", "a = 0 or b = 1 or c = 2.5", true},
		{"or 3 clause, result true", "a = 1 or b = 0 or c = 2.5", true},
		{"or 3 clause, result true", "a = 1 or b = 1 or c = 0", true},
		{"or 3 clause, result true", "a = 0 or b = 0 or c = 2.5", true},
		{"or 3 clause, result true", "a = 0 or b = 1 or c = 0", true},
		{"or 3 clause, result true", "a = 1 or b = 0 or c = 0", true},
		{"or 3 clause, result false", "a = 0 or b = 0 or c = 0", false},

		{"and/or 4 clause, result true", "(a = 1 or b = 1) and (c = 2.5 or d = datetime(2032-09-03T15:36:50Z))", true},
		{"and/or 4 clause, result true", "(a = 0 or b = 1) and (c = 2.7 or d = datetime(2032-09-03T15:36:50Z))", true},
		{"and/or 4 clause, result false", "(a = 0 or b = 0) and (c = 2.7 or d = datetime(2032-09-03T15:36:50Z))", false},

		{"and/or 4 clause, result true", "(a = 1 and b = 1) or (c = 2.5 and d = datetime(2032-09-03T15:36:50Z))", true},
		{"and/or 4 clause, result true", "(a = 1 and b = 0) or (c = 2.5 and d = datetime(2032-09-03T15:36:50Z))", true},
		{"and/or 4 clause, result false", "(a = 1 and b = 0) or (c = 2.5 and d = datetime(2031-09-03T15:36:50Z))", false},

		{"and/or 4 clause, result true", "a = 1 or (b = 1 and c = 2.5) or d = datetime(2032-09-03T15:36:50Z)", true},
		{"and/or 4 clause, result true", "a = 0 or (b = 1 and c = 2.7) or d = datetime(2032-09-03T15:36:50Z)", true},
		{"and/or 4 clause, result true", "a = 0 or (b = 0 and c = 2.7) or d = datetime(2032-09-03T15:36:50Z)", true},
		{"and/or 4 clause, result false", "a = 0 or (b = 1 and c = 2.7) or d = datetime(2031-09-03T15:36:50Z)", false},

		{"and/or 4 clause, result true", "a = 1 and (b = 1 or c = 2.5) and d = datetime(2032-09-03T15:36:50Z)", true},
		{"and/or 4 clause, result false", "a = 1 and (b = 0 or c = 2.7) and d = datetime(2032-09-03T15:36:50Z)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runFilterTest(t, tt)
		})
	}
}

func TestSetFunctions(t *testing.T) {
	tests := []testDef{
		{"any of true, first", "anyOf(link.ids) = 123", true},
		{"any of true, second", "anyOf(link.ids) = 456", true},
		{"any of true, third", "anyOf(link.ids) = 789", true},
		{"any of false", "anyOf(link.ids) = 122", false},

		{"all of true", "allOf(link.ids) < 1000", true},
		{"all of false (false for last)", "allOf(link.ids) < 500", false},
		{"all of false (false for all)", "allOf(link.ids) < 100", false},
		{"all of false (only firs true)", "allOf(link.ids) = 123", false},

		{"none of of false", "not anyOf(link.ids) < 1000", false},
		{"none of false (true for last)", "not anyOf(link.ids) < 500", false},
		{"none of true (true for all)", "not anyOf(link.ids) < 100", true},
		{"none of false (true for first)", "not anyOf(link.ids) = 123", false},

		{"any of true in, true", "anyOf(link.ids) in [123, 456]", true},
		{"any of true in, false", "anyOf(link.ids) in [321, 654]", false},
		{"any of true between, true", "anyOf(link.ids) between 123 and 124", true},
		{"any of true between, false", "anyOf(link.ids) between 100 and 110", false},

		{"count linkIds = 3, true", "count(link.ids) = 3", true},
		{"count linkIds > 5, false", "count(link.ids) > 5", false},
		{"count lunkIds = 0, true", "count(lunk.ids) = 0", true},
		{"count lunkIds > 5, false", "count(lunk.ids) > 5", false},

		{"isEmpty linkIds, false", "isEmpty(link.ids)", false},
		{"isEmpty lunkIds, true", "isEmpty(lunk.ids)", true},

		{"not isEmpty linkIds, true", "not isEmpty(link.ids)", true},
		{"not isEmpty lunkIds, false", "not isEmpty(lunk.ids)", false},

		{"and set ops, true", "not isEmpty(link.ids) and count(link.ids) = 3", true},
		// TODO: FIX THIS CASE {"and set ops explicit equals, true", "isEmpty(link.ids) = false and count(link.ids) = 3", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runFilterTest(t, tt)
		})
	}
}

type sortPageTestDef struct {
	name       string
	expr       string
	sortFields []SortField
	skip       *int64
	limit      *int64
	result     bool
}

func sorts(vals ...interface{}) []SortField {
	var result []SortField
	for i := 0; i < len(vals); i += 2 {
		name := vals[i].(string)
		asc := vals[i+1].(bool)
		result = append(result, sortField(name, asc))
	}
	return result
}

func sortField(name string, asc bool) SortField {
	return &SortFieldNode{
		symbol:      &UntypedSymbolNode{symbol: name},
		isAscending: asc,
	}
}

func TestSortingPaging(t *testing.T) {
	ten := int64(10)
	negOne := int64(-1)
	tests := []sortPageTestDef{
		{"defaults", "a=1", nil, nil, nil, true},

		{"one sort", "a=1 sort by a", sorts("a", true), nil, nil, true},
		{"one sort no predicate", "sort by a", sorts("a", true), nil, nil, true},
		{"one sort asc", "a=1 sort by a asc", sorts("a", true), nil, nil, true},
		{"one sort desc", "a=1 sort by a desc", sorts("a", false), nil, nil, true},
		{"two sorts", "a=1 sort by a,b", sorts("a", true, "b", true), nil, nil, true},
		{"two sorts desc, asc", "a=1 sort by a desc, b asc", sorts("a", false, "b", true), nil, nil, true},
		{"two sorts desc, default", "a=1 sort by a desc, b", sorts("a", false, "b", true), nil, nil, true},
		{"two sorts default, desc", "a=1 sort by a, b desc", sorts("a", true, "b", false), nil, nil, true},

		{"skip 10", "a=1 skip 10", nil, &ten, nil, true},
		{"skip 10 no predicate", "skip 10", nil, &ten, nil, true},
		{"limit 10", "a=1 limit 10", nil, nil, &ten, true},
		{"limit 10 no predicate", "limit 10", nil, nil, &ten, true},
		{"limit none", "a=1 limit none", nil, nil, &negOne, true},

		{"sort plus skip", "a=1 SORT BY c desc, d, a desc SKIP 10", sorts("c", false, "d", true, "a", false), &ten, nil, true},
		{"sort plus limit", "a=1 SORT BY c desc, d, a desc LIMIT 10", sorts("c", false, "d", true, "a", false), nil, &ten, true},
		{"use all", "a=1 SORT BY c, d, a desc SKIP 10 LIMIT none", sorts("c", true, "d", true, "a", false), &ten, &negOne, true},
		{"use all, bool expr", "true SORT BY c, d, a desc SKIP 10 LIMIT none", sorts("c", true, "d", true, "a", false), &ten, &negOne, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runSortPageTest(t, tt)
		})
	}
}

func runSortPageTest(t *testing.T, tt sortPageTestDef) {
	listener := NewListener()
	listener.PrintRuleLocation = false
	listener.PrintChildren = false
	listener.PrintStackOps = false

	req := require.New(t)
	parseErrors := zitiql.Parse(tt.expr, listener)
	if len(parseErrors) != 0 {
		req.NoError(parseErrors[0])
	}

	query, err := listener.getQuery(ts)
	if err != nil {
		t.Errorf("error evaluating expr %v, err: %+v", tt.expr, err)
		return
	}

	result := query.EvalBool(ts)

	if tt.result != result {
		t.Errorf("expected filter result %v, got %v", tt.result, result)
	}

	req.Equal(len(tt.sortFields), len(query.GetSortFields()))
	for idx, sortField := range tt.sortFields {
		req.Equal(sortField.Symbol(), query.GetSortFields()[idx].Symbol())
		req.Equal(sortField.IsAscending(), query.GetSortFields()[idx].IsAscending())
	}
	req.Equal(tt.skip, query.GetSkip())
	req.Equal(tt.limit, query.GetLimit())
}

func runFilterTest(t *testing.T, tt testDef) {
	listener := NewListener()
	listener.PrintRuleLocation = false
	listener.PrintChildren = false
	listener.PrintStackOps = false

	req := require.New(t)
	parseErrors := zitiql.Parse(tt.expr, listener)
	if len(parseErrors) != 0 {
		req.NoError(parseErrors[0])
	}

	filter, err := listener.getQuery(ts)
	if err != nil {
		t.Errorf("error evaluating expr %v, err: %+v", tt.expr, err)
		return
	}

	result := filter.EvalBool(ts)

	if tt.result != result {
		t.Errorf("expected filter result %v, got %v", tt.result, result)
	}
}
