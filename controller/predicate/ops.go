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

package predicate

import (
	"fmt"
	"gopkg.in/Masterminds/squirrel.v1"
)

//type Sqlizer interface {
//	Parts() (string, []interface{}, error)
//}

type binaryOp struct {
	Identifier string
	LeftValue  interface{}
	RightValue interface{}
}

type unaryOp struct {
	Identifier string
	value      interface{}
}

func (op unaryOp) Value() interface{} {
	return op.value
}

type Between binaryOp

func newBetween(identifier string, leftValue, rightValue interface{}) Between {
	return Between{
		Identifier: identifier,
		LeftValue:  leftValue,
		RightValue: rightValue,
	}
}

func (b Between) ToSql() (string, []interface{}, error) {
	args := []interface{}{b.LeftValue, b.RightValue}
	sql := fmt.Sprintf("%s BETWEEN %s AND %s", b.Identifier, "?", "?")
	return sql, args, nil
}

type NotBetween binaryOp

func newNotBetween(identifier string, leftValue, rightValue interface{}) NotBetween {
	return NotBetween{
		Identifier: identifier,
		LeftValue:  leftValue,
		RightValue: rightValue,
	}
}

func (b NotBetween) ToSql() (string, []interface{}, error) {
	args := []interface{}{b.LeftValue, b.RightValue}
	sql := fmt.Sprintf("%s NOT BETWEEN %s AND %s", b.Identifier, "?", "?")
	return sql, args, nil
}

type Contains unaryOp

func newContains(identifier, value string) Contains {
	return Contains{
		Identifier: identifier,
		value:      value,
	}
}

func (c Contains) ToSql() (string, []interface{}, error) {
	v := fmt.Sprintf("%%%s%%", c.value)

	args := []interface{}{v}

	sql := fmt.Sprintf("%s LIKE %s", c.Identifier, "?")

	return sql, args, nil
}

type NotContains unaryOp

func newNotContains(identifier, value string) NotContains {
	return NotContains{
		Identifier: identifier,
		value:      value,
	}
}

func (c NotContains) ToSql() (string, []interface{}, error) {
	v := fmt.Sprintf("%%%s%%", c.value)

	args := []interface{}{v}

	sql := fmt.Sprintf("%s NOT LIKE %s", c.Identifier, "?")

	return sql, args, nil
}

type InSubSelect struct {
	Column string
	Select squirrel.SelectBuilder
}

func (ss InSubSelect) ToSql() (string, []interface{}, error) {
	s, b, err := ss.Select.ToSql()
	return fmt.Sprintf("%s IN (%s)", ss.Column, s), b, err
}
