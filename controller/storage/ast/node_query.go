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
	"github.com/michaelquigley/pfxlog"
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

type untypedQueryNode struct {
	predicate Node
	sortBy    *SortByNode
	skip      *SkipExprNode
	limit     *LimitExprNode
}

func (node *untypedQueryNode) String() string {
	return node.predicate.String() + " " + node.sortBy.String() + " " + node.limit.String()
}

func (node *untypedQueryNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *untypedQueryNode) Accept(visitor Visitor) {
	visitor.VisitUntypedQueryNodeStart(node)
	node.predicate.Accept(visitor)
	node.sortBy.Accept(visitor)
	node.skip.Accept(visitor)
	node.limit.Accept(visitor)
	visitor.VisitUntypedQueryNodeEnd(node)
}

func (node *untypedQueryNode) EvalBool(Symbols) bool {
	pfxlog.Logger().Errorf("cannot evaluate transitory untyped query node %v", node)
	return false
}

func (node *untypedQueryNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	if err := transformTypes(s, &node.predicate); err != nil {
		return node, err
	}
	if _, err := node.sortBy.TypeTransform(s); err != nil {
		return node, err
	}

	boolNode, ok := node.predicate.(BoolNode)
	if !ok {
		return node, errors.Errorf("query expr predicate must be a boolean expr. contains %v", reflect.TypeOf(node.predicate))
	}

	return &queryNode{
		Predicate: boolNode,
		SortBy:    node.sortBy,
		Skip:      node.skip,
		Limit:     node.limit,
	}, nil
}

func (node *untypedQueryNode) IsConst() bool {
	return node.predicate.IsConst()
}

type queryNode struct {
	Predicate BoolNode
	SortBy    *SortByNode
	Skip      *SkipExprNode
	Limit     *LimitExprNode
}

func (node *queryNode) SetSkip(skip int64) {
	node.Skip = &SkipExprNode{Int64ConstNode{value: skip}}
}

func (node *queryNode) SetLimit(limit int64) {
	node.Limit = &LimitExprNode{Int64ConstNode{value: limit}}
}

func (node *queryNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	if err := transformBools(s, &node.Predicate); err != nil {
		return node, err
	}
	if _, err := node.SortBy.TypeTransform(s); err != nil {
		return node, err
	}
	return node, nil
}

func (node *queryNode) EvalBool(s Symbols) bool {
	return node.Predicate.EvalBool(s)
}

func (node *queryNode) GetPredicate() BoolNode {
	return node.Predicate
}

func (node *queryNode) SetPredicate(predicate BoolNode) {
	node.Predicate = predicate
}

func (node *queryNode) GetSortFields() []SortField {
	return node.SortBy.getSortFields()
}

func (node *queryNode) AdoptSortFields(query Query) error {
	qn, ok := query.(*queryNode)
	if !ok {
		return errors.Errorf("unhanded query type: %v. expecting queryNode", reflect.TypeOf(query))
	}
	node.SortBy = qn.SortBy
	return nil
}

func (node *queryNode) GetSkip() *int64 {
	if node.Skip == nil {
		return nil
	}
	return &node.Skip.value
}

func (node *queryNode) GetLimit() *int64 {
	if node.Limit == nil {
		return nil
	}
	return &node.Limit.value
}

func (node *queryNode) String() string {
	builder := strings.Builder{}
	builder.WriteString(node.Predicate.String())
	if node.SortBy != nil && len(node.SortBy.SortFields) > 0 {
		builder.WriteString(" ")
		builder.WriteString(node.SortBy.String())
	}
	if node.Limit != nil {
		builder.WriteString(" ")
		builder.WriteString(node.Limit.String())
	}
	return builder.String()
}

func (node *queryNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *queryNode) Accept(visitor Visitor) {
	visitor.VisitQueryNodeStart(node)
	node.Predicate.Accept(visitor)
	node.SortBy.Accept(visitor)
	node.Skip.Accept(visitor)
	node.Limit.Accept(visitor)
	visitor.VisitQueryNodeEnd(node)
}

func (node *queryNode) IsConst() bool {
	return node.Predicate.IsConst()
}

type SortByNode struct {
	SortFields []*SortFieldNode
}

func (node *SortByNode) getSortFields() []SortField {
	if node == nil {
		return nil
	}
	var result []SortField
	for _, sortField := range node.SortFields {
		result = append(result, sortField)
	}
	return result
}

func (node *SortByNode) TypeTransform(s SymbolTypes) (Node, error) {
	if node != nil {
		for _, sortField := range node.SortFields {
			if _, err := sortField.TypeTransform(s); err != nil {
				return node, err
			}
		}
	}
	return node, nil
}

func (node *SortByNode) String() string {
	builder := strings.Builder{}
	builder.WriteString("sort by ")
	if node == nil || len(node.SortFields) == 0 {
		builder.WriteString("<default>")
	} else {
		builder.WriteString(node.SortFields[0].String())
		for _, sortField := range node.SortFields[1:] {
			builder.WriteString(", ")
			builder.WriteString(sortField.String())
		}
	}
	return builder.String()
}

func (node *SortByNode) GetType() NodeType {
	return NodeTypeOther
}

func (node *SortByNode) Accept(visitor Visitor) {
	if node != nil {
		for _, sortField := range node.SortFields {
			sortField.Accept(visitor)
		}
		visitor.VisitSortByNode(node)
	}
}

func (node *SortByNode) IsConst() bool {
	return false
}

func NewSortFieldNode(symbol string, isAscending bool) SortField {
	return &SortFieldNode{
		symbol: &UntypedSymbolNode{
			symbol: symbol,
		},
		isAscending: isAscending,
	}
}

type SortFieldNode struct {
	symbol      SymbolNode
	isAscending bool
}

func (node *SortFieldNode) Symbol() string {
	return node.symbol.Symbol()
}

func (node *SortFieldNode) IsAscending() bool {
	return node.isAscending
}

func (node *SortFieldNode) TypeTransform(s SymbolTypes) (Node, error) {
	var symbolNode Node = node.symbol
	err := transformTypes(s, &symbolNode)
	node.symbol = symbolNode.(SymbolNode)
	return node, err
}

func (node *SortFieldNode) String() string {
	if node.isAscending {
		return fmt.Sprintf("%v ASC", node.symbol)
	}
	return fmt.Sprintf("%v DESC", node.symbol)
}

func (node *SortFieldNode) GetType() NodeType {
	return NodeTypeOther
}

func (node *SortFieldNode) Accept(visitor Visitor) {
	node.symbol.Accept(visitor)
	visitor.VisitSortFieldNode(node)
}

func (node *SortFieldNode) IsConst() bool {
	return false
}

type SortDirection bool

const (
	SortAscending  SortDirection = true
	SortDescending SortDirection = false
)

type LimitExprNode struct {
	Int64ConstNode
}

func (node *LimitExprNode) String() string {
	if node == nil || node.value == -1 {
		return "limit none"
	}
	return fmt.Sprintf("limit %v", node.value)
}

func (node *LimitExprNode) Accept(visitor Visitor) {
	if node != nil {
		visitor.VisitLimitExprNode(node)
	}
}

type SkipExprNode struct {
	Int64ConstNode
}

func (node *SkipExprNode) String() string {
	if node == nil {
		return "skip 0"
	}
	return fmt.Sprintf("skip %v", node.value)
}

func (node *SkipExprNode) Accept(visitor Visitor) {
	if node != nil {
		visitor.VisitSkipExprNode(node)
	}
}

type UntypedSubQueryNode struct {
	symbol SymbolNode
	query  Node
}

func (node *UntypedSubQueryNode) Symbol() string {
	return node.symbol.Symbol()
}

func (node *UntypedSubQueryNode) Accept(visitor Visitor) {
	visitor.VisitUntypedSubQueryNodeStart(node)
	node.symbol.Accept(visitor)
	node.query.Accept(visitor)
	visitor.VisitUntypedSubQueryNodeEnd(node)
}

func (node *UntypedSubQueryNode) TypeTransform(s SymbolTypes) (Node, error) {
	var symbolAsNode Node = node.symbol
	if err := transformTypes(s, &symbolAsNode); err != nil {
		return node, err
	}

	subQuerySymbolTypes := s.GetSetSymbolTypes(node.Symbol())
	if subQuerySymbolTypes == nil {
		return node, errors.Errorf("symbol for sub-query is not an entity type %v", node.Symbol())
	}

	if err := transformTypes(subQuerySymbolTypes, &node.query); err != nil {
		return node, err
	}

	symbolNode, ok := symbolAsNode.(SymbolNode)
	if !ok {
		return node, errors.Errorf("from symbol must be an expr. contains %v", reflect.TypeOf(symbolAsNode))
	}

	queryNode, ok := node.query.(Query)
	if !ok {
		return node, errors.Errorf("from query must be a query instance. contains %v", reflect.TypeOf(node.query))
	}

	return &subQueryNode{
		symbol: symbolNode,
		query:  queryNode,
	}, nil
}

func (node *UntypedSubQueryNode) String() string {
	return fmt.Sprintf("from(%v where %v)", node.symbol.String(), node.query.String())
}

func (node *UntypedSubQueryNode) GetType() NodeType {
	return NodeTypeOther
}

func (node *UntypedSubQueryNode) IsConst() bool {
	return node.query.IsConst()
}

type subQueryNode struct {
	symbol SymbolNode
	query  Query
}

func (node *subQueryNode) Symbol() string {
	return node.symbol.Symbol()
}

func (node *subQueryNode) Accept(visitor Visitor) {
	visitor.VisitSubQueryNodeStart(node)
	node.symbol.Accept(visitor)
	node.query.Accept(visitor)
	visitor.VisitSubQueryNodeEnd(node)
}

func (node *subQueryNode) String() string {
	return fmt.Sprintf("from(%v where %v)", node.symbol.String(), node.query.String())
}

func (node *subQueryNode) GetType() NodeType {
	return NodeTypeOther
}

func (node *subQueryNode) IsConst() bool {
	return node.query.IsConst()
}
