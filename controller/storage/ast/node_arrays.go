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
	"strings"
)

func NewStringArrayNode(values []string) *StringArrayNode {
	result := &StringArrayNode{}
	for _, val := range values {
		result.values = append(result.values, &StringConstNode{value: val})
	}
	return result
}

// StringArrayNode encapsulates a string array
type StringArrayNode struct {
	values []StringNode
}

func (node *StringArrayNode) String() string {
	builder := &strings.Builder{}
	builder.WriteString("[")
	if len(node.values) > 0 {
		builder.WriteString(node.values[0].String())
		for _, child := range node.values[1:] {
			builder.WriteString(", ")
			builder.WriteString(child.String())
		}
	}
	builder.WriteString("]")
	return builder.String()
}

func (*StringArrayNode) GetType() NodeType {
	return NodeTypeOther
}

func (node *StringArrayNode) Accept(visitor Visitor) {
	visitor.VisitStringArrayNodeStart(node)
	for _, child := range node.values {
		child.Accept(visitor)
	}
	visitor.VisitStringArrayNodeEnd(node)
}

func (node *StringArrayNode) AsStringArray() *StringArrayNode {
	return node
}

func (node *StringArrayNode) IsConst() bool {
	return true
}

// Float64ArrayNode encapsulates a float64 array
type Float64ArrayNode struct {
	values []Float64Node
}

func (node *Float64ArrayNode) String() string {
	builder := &strings.Builder{}
	builder.WriteString("[")
	if len(node.values) > 0 {
		builder.WriteString(node.values[0].String())
		for _, child := range node.values[1:] {
			builder.WriteString(", ")
			builder.WriteString(child.String())
		}
	}
	builder.WriteString("]")
	return builder.String()
}

func (node *Float64ArrayNode) GetType() NodeType {
	return NodeTypeOther
}

func (node *Float64ArrayNode) Accept(visitor Visitor) {
	visitor.VisitFloat64ArrayNodeStart(node)
	for _, child := range node.values {
		child.Accept(visitor)
	}
	visitor.VisitFloat64ArrayNodeEnd(node)
}

func (node *Float64ArrayNode) AsStringArray() *StringArrayNode {
	result := &StringArrayNode{}
	for _, child := range node.values {
		result.values = append(result.values, child)
	}
	return result
}

func (node *Float64ArrayNode) IsConst() bool {
	return true
}

// Int64ArrayNode encapsulates an int64 array
type Int64ArrayNode struct {
	values []Int64Node
}

func (node *Int64ArrayNode) String() string {
	builder := &strings.Builder{}
	builder.WriteString("[")
	if len(node.values) > 0 {
		builder.WriteString(node.values[0].String())
		for _, child := range node.values[1:] {
			builder.WriteString(", ")
			builder.WriteString(child.String())
		}
	}
	builder.WriteString("]")
	return builder.String()
}

func (node *Int64ArrayNode) GetType() NodeType {
	return NodeTypeOther
}

func (node *Int64ArrayNode) ToFloat64ArrayNode() *Float64ArrayNode {
	result := &Float64ArrayNode{}
	for _, intNode := range node.values {
		result.values = append(result.values, intNode.ToFloat64())
	}
	return result
}

func (node *Int64ArrayNode) Accept(visitor Visitor) {
	visitor.VisitInt64ArrayNodeStart(node)
	for _, child := range node.values {
		child.Accept(visitor)
	}
	visitor.VisitInt64ArrayNodeEnd(node)
}

func (node *Int64ArrayNode) AsStringArray() *StringArrayNode {
	result := &StringArrayNode{}
	for _, child := range node.values {
		result.values = append(result.values, child)
	}
	return result
}

func (node *Int64ArrayNode) IsConst() bool {
	return true
}

// DatetimeArrayNode encapsulates a datetime array
type DatetimeArrayNode struct {
	values []DatetimeNode
}

func (node *DatetimeArrayNode) String() string {
	builder := &strings.Builder{}
	builder.WriteString("[")
	if len(node.values) > 0 {
		builder.WriteString(node.values[0].String())
		for _, child := range node.values[1:] {
			builder.WriteString(", ")
			builder.WriteString(child.String())
		}
	}
	builder.WriteString("]")
	return builder.String()
}

func (node *DatetimeArrayNode) GetType() NodeType {
	return NodeTypeOther
}

func (node *DatetimeArrayNode) Accept(visitor Visitor) {
	visitor.VisitDatetimeArrayNodeStart(node)
	for _, child := range node.values {
		child.Accept(visitor)
	}
	visitor.VisitDatetimeArrayNodeEnd(node)
}

func (node *DatetimeArrayNode) IsConst() bool {
	return true
}

type InStringArrayExprNode struct {
	left  StringNode
	right *StringArrayNode
}

func (node *InStringArrayExprNode) String() string {
	return fmt.Sprintf("%v in %v", node.left, node.right)
}

func (*InStringArrayExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *InStringArrayExprNode) EvalBool(s Symbols) bool {
	left := node.left.EvalString(s)

	for _, rightNode := range node.right.values {
		right := rightNode.EvalString(s)
		if left != nil && right != nil {
			if *left == *right {
				return true
			}
		}
	}
	return false
}

func (node *InStringArrayExprNode) Accept(visitor Visitor) {
	visitor.VisitInStringArrayExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitInStringArrayExprNodeEnd(node)
}

func (node *InStringArrayExprNode) IsConst() bool {
	return false
}

type InInt64ArrayExprNode struct {
	left  Int64Node
	right *Int64ArrayNode
}

func (node *InInt64ArrayExprNode) String() string {
	return fmt.Sprintf("%v in %v", node.left, node.right)
}

func (node *InInt64ArrayExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *InInt64ArrayExprNode) EvalBool(s Symbols) bool {
	left := node.left.EvalInt64(s)

	for _, rightNode := range node.right.values {
		right := rightNode.EvalInt64(s)
		if left != nil && right != nil {
			if *left == *right {
				return true
			}
		}
	}
	return false
}

func (node *InInt64ArrayExprNode) Accept(visitor Visitor) {
	visitor.VisitInInt64ArrayExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitInInt64ArrayExprNodeEnd(node)
}

func (node *InInt64ArrayExprNode) IsConst() bool {
	return false
}

type InFloat64ArrayExprNode struct {
	left  Float64Node
	right *Float64ArrayNode
}

func (node *InFloat64ArrayExprNode) String() string {
	return fmt.Sprintf("%v in %v", node.left, node.right)
}

func (node *InFloat64ArrayExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *InFloat64ArrayExprNode) EvalBool(s Symbols) bool {
	left := node.left.EvalFloat64(s)

	for _, rightNode := range node.right.values {
		right := rightNode.EvalFloat64(s)
		if left != nil && right != nil {
			if *left == *right {
				return true
			}
		}
	}
	return false
}

func (node *InFloat64ArrayExprNode) Accept(visitor Visitor) {
	visitor.VisitInFloat64ArrayExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitInFloat64ArrayExprNodeEnd(node)
}

func (node *InFloat64ArrayExprNode) IsConst() bool {
	return false
}

type InDatetimeArrayExprNode struct {
	left  DatetimeNode
	right *DatetimeArrayNode
}

func (node *InDatetimeArrayExprNode) String() string {
	return fmt.Sprintf("%v in %v", node.left, node.right)
}

func (*InDatetimeArrayExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *InDatetimeArrayExprNode) EvalBool(s Symbols) bool {
	left := node.left.EvalDatetime(s)

	for _, rightNode := range node.right.values {
		right := rightNode.EvalDatetime(s)
		if left != nil && right != nil {
			if left.Equal(*right) {
				return true
			}
		}
	}
	return false
}

func (node *InDatetimeArrayExprNode) Accept(visitor Visitor) {
	visitor.VisitInDatetimeArrayExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitInDatetimeArrayExprNodeEnd(node)
}

func (node *InDatetimeArrayExprNode) IsConst() bool {
	return false
}
