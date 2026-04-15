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

package objectz

import (
	"github.com/openziti/storage/ast"
	"time"
)

type ObjectSymbol[T any] interface {
	GetType() ast.NodeType
	Eval(entity T) any
}

// ObjectStringSymbol can evaluate string symbols on a given entity type
type ObjectStringSymbol[T any] struct {
	f func(entity T) *string
}

func (self *ObjectStringSymbol[T]) GetType() ast.NodeType {
	return ast.NodeTypeString
}

func (self *ObjectStringSymbol[T]) Eval(entity T) any {
	return self.f(entity)
}

func (self *ObjectStringSymbol[T]) EvalString(entity T) *string {
	return self.f(entity)
}

// ObjectBoolSymbol can evaluate boolean symbols on a given entity type
type ObjectBoolSymbol[T any] struct {
	f func(entity T) *bool
}

func (self *ObjectBoolSymbol[T]) GetType() ast.NodeType {
	return ast.NodeTypeBool
}

func (self *ObjectBoolSymbol[T]) EvalBool(entity T) *bool {
	return self.f(entity)
}

func (self *ObjectBoolSymbol[T]) Eval(entity T) any {
	return self.f(entity)
}

// ObjectInt64Symbol can evaluate int64 symbols on a given entity type
type ObjectInt64Symbol[T any] struct {
	f func(entity T) *int64
}

func (self *ObjectInt64Symbol[T]) GetType() ast.NodeType {
	return ast.NodeTypeInt64
}

func (self *ObjectInt64Symbol[T]) EvalInt64(entity T) *int64 {
	return self.f(entity)
}

func (self *ObjectInt64Symbol[T]) Eval(entity T) any {
	return self.f(entity)
}

// ObjectFloat64Symbol can evaluate float64 symbols on a given entity type
type ObjectFloat64Symbol[T any] struct {
	f func(entity T) *float64
}

func (self *ObjectFloat64Symbol[T]) GetType() ast.NodeType {
	return ast.NodeTypeFloat64
}

func (self *ObjectFloat64Symbol[T]) EvalFloat64(entity T) *float64 {
	return self.f(entity)
}

func (self *ObjectFloat64Symbol[T]) Eval(entity T) any {
	return self.f(entity)
}

// ObjectDatetimeSymbol can evaluate date-time symbols on a given entity type
type ObjectDatetimeSymbol[T any] struct {
	f func(entity T) *time.Time
}

func (self *ObjectDatetimeSymbol[T]) GetType() ast.NodeType {
	return ast.NodeTypeDatetime
}

func (self *ObjectDatetimeSymbol[T]) EvalDatetime(entity T) *time.Time {
	return self.f(entity)
}

func (self *ObjectDatetimeSymbol[T]) Eval(entity T) any {
	return self.f(entity)
}
