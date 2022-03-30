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
	"fmt"
	"strconv"
	"time"
)

var _ BoolNode = (*BoolConstNode)(nil)
var _ DatetimeNode = (*DatetimeConstNode)(nil)
var _ Int64Node = (*Int64ConstNode)(nil)
var _ Float64Node = (*Float64ConstNode)(nil)
var _ StringNode = (*StringConstNode)(nil)

func NewBoolConstNode(value bool) BoolNode {
	return &BoolConstNode{value: value}
}

// BoolConstNode wraps a bool constant expression
type BoolConstNode struct {
	value bool
}

func (node *BoolConstNode) Accept(visitor Visitor) {
	visitor.VisitBoolConstNode(node)
}

func (node *BoolConstNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *BoolConstNode) EvalBool(_ Symbols) bool {
	return node.value
}

func (node *BoolConstNode) String() string {
	return fmt.Sprintf("%v", node.value)
}

func (node *BoolConstNode) IsConst() bool {
	return true
}

// DatetimeConstNode wraps a datetime constant expression
type DatetimeConstNode struct {
	value time.Time
}

func (node *DatetimeConstNode) Accept(visitor Visitor) {
	visitor.VisitDatetimeConstNode(node)
}

func (node *DatetimeConstNode) GetType() NodeType {
	return NodeTypeDatetime
}

func (node *DatetimeConstNode) EvalDatetime(_ Symbols) *time.Time {
	return &node.value
}

func (node *DatetimeConstNode) String() string {
	return fmt.Sprintf("%v", node.value)
}

func (node *DatetimeConstNode) IsConst() bool {
	return true
}

// Float64ConstNode wraps a float64 constant expression
type Float64ConstNode struct {
	value float64
}

func (node *Float64ConstNode) Accept(visitor Visitor) {
	visitor.VisitFloat64ConstNode(node)
}

func (node *Float64ConstNode) GetType() NodeType {
	return NodeTypeFloat64
}

func (node *Float64ConstNode) EvalFloat64(_ Symbols) *float64 {
	return &node.value
}

func (node *Float64ConstNode) EvalString(_ Symbols) *string {
	result := strconv.FormatFloat(node.value, 'f', -1, 64)
	return &result
}

func (node *Float64ConstNode) String() string {
	return fmt.Sprintf("%v", node.value)
}

func (node *Float64ConstNode) IsConst() bool {
	return true
}

// Int64ConstNode wraps an int64 constant expression
type Int64ConstNode struct {
	value int64
}

func (node *Int64ConstNode) Accept(visitor Visitor) {
	visitor.VisitInt64ConstNode(node)
}

func (node *Int64ConstNode) GetType() NodeType {
	return NodeTypeInt64
}

func (node *Int64ConstNode) EvalInt64(_ Symbols) *int64 {
	return &node.value
}

func (node *Int64ConstNode) EvalString(_ Symbols) *string {
	result := strconv.FormatInt(node.value, 10)
	return &result
}

func (node *Int64ConstNode) ToFloat64() Float64Node {
	return &Float64ConstNode{value: float64(node.value)}
}

func (node *Int64ConstNode) String() string {
	return strconv.FormatInt(node.value, 10)
}

func (node *Int64ConstNode) IsConst() bool {
	return true
}

// StringConstNode wraps a string constant expression
type StringConstNode struct {
	value string
}

func (node *StringConstNode) Accept(visitor Visitor) {
	visitor.VisitStringConstNode(node)
}

func (node *StringConstNode) GetType() NodeType {
	return NodeTypeString
}

func (node *StringConstNode) EvalString(_ Symbols) *string {
	return &node.value
}

func (node *StringConstNode) String() string {
	return fmt.Sprintf(`"%v"`, node.value)
}

func (node *StringConstNode) IsConst() bool {
	return true
}

var _ Node = (*NullConstNode)(nil)

// NullNode wraps a null constant expression
type NullConstNode struct{}

func (node NullConstNode) Accept(visitor Visitor) {
	visitor.VisitNullConstNode(node)
}

func (NullConstNode) GetType() NodeType {
	return NodeTypeOther
}

func (NullConstNode) String() string {
	return "null"
}

func (node NullConstNode) IsConst() bool {
	return true
}
