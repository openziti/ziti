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

type Visitor interface {
	VisitNotExprNodeStart(node *NotExprNode)
	VisitNotExprNodeEnd(node *NotExprNode)
	VisitAndExprNodeStart(node *AndExprNode)
	VisitAndExprNodeEnd(node *AndExprNode)
	VisitOrExprNodeStart(node *OrExprNode)
	VisitOrExprNodeEnd(node *OrExprNode)
	VisitBinaryBoolExprNodeStart(node *BinaryBoolExprNode)
	VisitBinaryBoolExprNodeEnd(node *BinaryBoolExprNode)
	VisitBinaryDatetimeExprNodeStart(node *BinaryDatetimeExprNode)
	VisitBinaryDatetimeExprNodeEnd(node *BinaryDatetimeExprNode)
	VisitBinaryFloat64ExprNodeStart(node *BinaryFloat64ExprNode)
	VisitBinaryFloat64ExprNodeEnd(node *BinaryFloat64ExprNode)
	VisitBinaryInt64ExprNodeStart(node *BinaryInt64ExprNode)
	VisitBinaryInt64ExprNodeEnd(node *BinaryInt64ExprNode)
	VisitBinaryStringExprNodeStart(node *BinaryStringExprNode)
	VisitBinaryStringExprNodeEnd(node *BinaryStringExprNode)
	VisitIsNilExprNodeStart(node *IsNilExprNode)
	VisitIsNilExprNodeEnd(node *IsNilExprNode)

	VisitInt64BetweenExprNodeStart(node *Int64BetweenExprNode)
	VisitInt64BetweenExprNodeEnd(node *Int64BetweenExprNode)
	VisitFloat64BetweenExprNodeStart(node *Float64BetweenExprNode)
	VisitFloat64BetweenExprNodeEnd(node *Float64BetweenExprNode)
	VisitDatetimeBetweenExprNodeStart(node *DatetimeBetweenExprNode)
	VisitDatetimeBetweenExprNodeEnd(node *DatetimeBetweenExprNode)

	VisitInDatetimeArrayExprNodeStart(node *InDatetimeArrayExprNode)
	VisitInDatetimeArrayExprNodeEnd(node *InDatetimeArrayExprNode)
	VisitInFloat64ArrayExprNodeStart(node *InFloat64ArrayExprNode)
	VisitInFloat64ArrayExprNodeEnd(node *InFloat64ArrayExprNode)
	VisitInInt64ArrayExprNodeStart(node *InInt64ArrayExprNode)
	VisitInInt64ArrayExprNodeEnd(node *InInt64ArrayExprNode)
	VisitInStringArrayExprNodeStart(node *InStringArrayExprNode)
	VisitInStringArrayExprNodeEnd(node *InStringArrayExprNode)

	// untyped transient nodes
	VisitBooleanLogicExprNodeStart(node *BooleanLogicExprNode)
	VisitBooleanLogicExprNodeEnd(node *BooleanLogicExprNode)
	VisitBinaryExprNodeStart(node *BinaryExprNode)
	VisitBinaryExprNodeEnd(node *BinaryExprNode)
	VisitInArrayExprNodeStart(node *InArrayExprNode)
	VisitInArrayExprNodeEnd(node *InArrayExprNode)
	VisitBetweenExprNodeStart(node *BetweenExprNode)
	VisitBetweenExprNodeEnd(node *BetweenExprNode)
	VisitUntypedSymbolNode(node *UntypedSymbolNode)
	VisitSetFunctionNodeStart(node *SetFunctionNode)
	VisitSetFunctionNodeEnd(node *SetFunctionNode)
	VisitUntypedNotExprStart(node *UntypedNotExprNode)
	VisitUntypedNotExprEnd(node *UntypedNotExprNode)

	// const nodes
	VisitBoolConstNode(node *BoolConstNode)
	VisitDatetimeConstNode(node *DatetimeConstNode)
	VisitFloat64ConstNode(node *Float64ConstNode)
	VisitInt64ConstNode(node *Int64ConstNode)
	VisitStringConstNode(node *StringConstNode)
	VisitNullConstNode(node NullConstNode)

	VisitDatetimeArrayNodeStart(node *DatetimeArrayNode)
	VisitDatetimeArrayNodeEnd(node *DatetimeArrayNode)
	VisitFloat64ArrayNodeStart(node *Float64ArrayNode)
	VisitFloat64ArrayNodeEnd(node *Float64ArrayNode)
	VisitInt64ArrayNodeStart(node *Int64ArrayNode)
	VisitInt64ArrayNodeEnd(node *Int64ArrayNode)
	VisitStringArrayNodeStart(node *StringArrayNode)
	VisitStringArrayNodeEnd(node *StringArrayNode)

	// symbol nodes
	VisitBoolSymbolNode(node *BoolSymbolNode)
	VisitDatetimeSymbolNode(node *DatetimeSymbolNode)
	VisitFloat64SymbolNode(node *Float64SymbolNode)
	VisitInt64SymbolNode(node *Int64SymbolNode)
	VisitStringSymbolNode(node *StringSymbolNode)
	VisitAnyTypeSymbolNode(node *AnyTypeSymbolNode)

	// conversion
	VisitInt64ToFloat64NodeStart(node *Int64ToFloat64Node)
	VisitInt64ToFloat64NodeEnd(node *Int64ToFloat64Node)
	VisitStringFuncNodeStart(node *StringFuncNode)
	VisitStringFuncNodeEnd(node *StringFuncNode)

	// sets
	VisitAllOfSetExprNodeStart(node *AllOfSetExprNode)
	VisitAllOfSetExprNodeEnd(node *AllOfSetExprNode)
	VisitAnyOfSetExprNodeStart(node *AnyOfSetExprNode)
	VisitAnyOfSetExprNodeEnd(node *AnyOfSetExprNode)
	VisitCountSetExprNodeStart(node *CountSetExprNode)
	VisitCountSetExprNodeEnd(node *CountSetExprNode)
	VisitIsEmptySetExprNodeStart(node *IsEmptySetExprNode)
	VisitIsEmptySetExprNodeEnd(node *IsEmptySetExprNode)

	VisitUntypedQueryNodeStart(node *untypedQueryNode)
	VisitUntypedQueryNodeEnd(node *untypedQueryNode)
	VisitQueryNodeStart(node *queryNode)
	VisitQueryNodeEnd(node *queryNode)
	VisitSortByNode(node *SortByNode)
	VisitSortFieldNode(node *SortFieldNode)
	VisitLimitExprNode(node *LimitExprNode)
	VisitSkipExprNode(node *SkipExprNode)

	VisitUntypedSubQueryNodeStart(node *UntypedSubQueryNode)
	VisitUntypedSubQueryNodeEnd(node *UntypedSubQueryNode)
	VisitSubQueryNodeStart(node *subQueryNode)
	VisitSubQueryNodeEnd(node *subQueryNode)

	VisitSymbol(symbol string, nodeType NodeType)
}

var _ Visitor = (*DefaultVisitor)(nil)

type DefaultVisitor struct {
}

func (d DefaultVisitor) VisitStringFuncNodeStart(node *StringFuncNode) {}
func (d DefaultVisitor) VisitStringFuncNodeEnd(node *StringFuncNode)   {}

func (d DefaultVisitor) VisitUntypedSubQueryNodeStart(*UntypedSubQueryNode) {}
func (d DefaultVisitor) VisitUntypedSubQueryNodeEnd(*UntypedSubQueryNode)   {}
func (d DefaultVisitor) VisitSubQueryNodeStart(*subQueryNode)               {}
func (d DefaultVisitor) VisitSubQueryNodeEnd(*subQueryNode)                 {}

func (d DefaultVisitor) VisitUntypedQueryNodeStart(_ *untypedQueryNode) {}
func (d DefaultVisitor) VisitUntypedQueryNodeEnd(_ *untypedQueryNode)   {}
func (d DefaultVisitor) VisitQueryNodeStart(_ *queryNode)               {}
func (d DefaultVisitor) VisitQueryNodeEnd(_ *queryNode)                 {}

func (d DefaultVisitor) VisitSortByNode(_ *SortByNode)       {}
func (d DefaultVisitor) VisitSortFieldNode(_ *SortFieldNode) {}
func (d DefaultVisitor) VisitLimitExprNode(_ *LimitExprNode) {}
func (d DefaultVisitor) VisitSkipExprNode(_ *SkipExprNode)   {}

func (d DefaultVisitor) VisitUntypedNotExprStart(*UntypedNotExprNode) {}
func (d DefaultVisitor) VisitUntypedNotExprEnd(*UntypedNotExprNode)   {}
func (d DefaultVisitor) VisitNotExprNodeStart(_ *NotExprNode)         {}
func (d DefaultVisitor) VisitNotExprNodeEnd(_ *NotExprNode)           {}
func (d DefaultVisitor) VisitAndExprNodeStart(_ *AndExprNode)         {}
func (d DefaultVisitor) VisitAndExprNodeEnd(_ *AndExprNode)           {}
func (d DefaultVisitor) VisitOrExprNodeStart(_ *OrExprNode)           {}
func (d DefaultVisitor) VisitOrExprNodeEnd(_ *OrExprNode)             {}

func (d DefaultVisitor) VisitBinaryBoolExprNodeStart(_ *BinaryBoolExprNode)         {}
func (d DefaultVisitor) VisitBinaryBoolExprNodeEnd(_ *BinaryBoolExprNode)           {}
func (d DefaultVisitor) VisitBinaryDatetimeExprNodeStart(_ *BinaryDatetimeExprNode) {}
func (d DefaultVisitor) VisitBinaryDatetimeExprNodeEnd(_ *BinaryDatetimeExprNode)   {}
func (d DefaultVisitor) VisitBinaryFloat64ExprNodeStart(_ *BinaryFloat64ExprNode)   {}
func (d DefaultVisitor) VisitBinaryFloat64ExprNodeEnd(_ *BinaryFloat64ExprNode)     {}
func (d DefaultVisitor) VisitBinaryInt64ExprNodeStart(_ *BinaryInt64ExprNode)       {}
func (d DefaultVisitor) VisitBinaryInt64ExprNodeEnd(_ *BinaryInt64ExprNode)         {}
func (d DefaultVisitor) VisitBinaryStringExprNodeStart(_ *BinaryStringExprNode)     {}
func (d DefaultVisitor) VisitBinaryStringExprNodeEnd(_ *BinaryStringExprNode)       {}

func (d DefaultVisitor) VisitIsNilExprNodeStart(_ *IsNilExprNode) {}
func (d DefaultVisitor) VisitIsNilExprNodeEnd(_ *IsNilExprNode)   {}

func (d DefaultVisitor) VisitInt64BetweenExprNodeStart(_ *Int64BetweenExprNode)       {}
func (d DefaultVisitor) VisitInt64BetweenExprNodeEnd(_ *Int64BetweenExprNode)         {}
func (d DefaultVisitor) VisitFloat64BetweenExprNodeStart(_ *Float64BetweenExprNode)   {}
func (d DefaultVisitor) VisitFloat64BetweenExprNodeEnd(_ *Float64BetweenExprNode)     {}
func (d DefaultVisitor) VisitDatetimeBetweenExprNodeStart(_ *DatetimeBetweenExprNode) {}
func (d DefaultVisitor) VisitDatetimeBetweenExprNodeEnd(_ *DatetimeBetweenExprNode)   {}

func (d DefaultVisitor) VisitInDatetimeArrayExprNodeStart(_ *InDatetimeArrayExprNode) {}
func (d DefaultVisitor) VisitInDatetimeArrayExprNodeEnd(_ *InDatetimeArrayExprNode)   {}
func (d DefaultVisitor) VisitInFloat64ArrayExprNodeStart(_ *InFloat64ArrayExprNode)   {}
func (d DefaultVisitor) VisitInFloat64ArrayExprNodeEnd(_ *InFloat64ArrayExprNode)     {}
func (d DefaultVisitor) VisitInInt64ArrayExprNodeStart(_ *InInt64ArrayExprNode)       {}
func (d DefaultVisitor) VisitInInt64ArrayExprNodeEnd(_ *InInt64ArrayExprNode)         {}
func (d DefaultVisitor) VisitInStringArrayExprNodeStart(_ *InStringArrayExprNode)     {}
func (d DefaultVisitor) VisitInStringArrayExprNodeEnd(_ *InStringArrayExprNode)       {}

func (d DefaultVisitor) VisitBooleanLogicExprNodeStart(*BooleanLogicExprNode) {}
func (d DefaultVisitor) VisitBooleanLogicExprNodeEnd(*BooleanLogicExprNode)   {}
func (d DefaultVisitor) VisitBinaryExprNodeStart(*BinaryExprNode)             {}
func (d DefaultVisitor) VisitBinaryExprNodeEnd(*BinaryExprNode)               {}
func (d DefaultVisitor) VisitInArrayExprNodeStart(*InArrayExprNode)           {}
func (d DefaultVisitor) VisitInArrayExprNodeEnd(*InArrayExprNode)             {}
func (d DefaultVisitor) VisitBetweenExprNodeStart(*BetweenExprNode)           {}
func (d DefaultVisitor) VisitBetweenExprNodeEnd(*BetweenExprNode)             {}
func (d DefaultVisitor) VisitUntypedSymbolNode(*UntypedSymbolNode)            {}

func (d DefaultVisitor) VisitSetFunctionNodeStart(_ *SetFunctionNode) {}
func (d DefaultVisitor) VisitSetFunctionNodeEnd(_ *SetFunctionNode)   {}

func (d DefaultVisitor) VisitBoolConstNode(_ *BoolConstNode)              {}
func (d DefaultVisitor) VisitDatetimeConstNode(_ *DatetimeConstNode)      {}
func (d DefaultVisitor) VisitFloat64ConstNode(_ *Float64ConstNode)        {}
func (d DefaultVisitor) VisitInt64ConstNode(_ *Int64ConstNode)            {}
func (d DefaultVisitor) VisitStringConstNode(_ *StringConstNode)          {}
func (d DefaultVisitor) VisitNullConstNode(_ NullConstNode)               {}
func (d DefaultVisitor) VisitDatetimeArrayNodeStart(_ *DatetimeArrayNode) {}
func (d DefaultVisitor) VisitDatetimeArrayNodeEnd(_ *DatetimeArrayNode)   {}
func (d DefaultVisitor) VisitFloat64ArrayNodeStart(_ *Float64ArrayNode)   {}
func (d DefaultVisitor) VisitFloat64ArrayNodeEnd(_ *Float64ArrayNode)     {}
func (d DefaultVisitor) VisitInt64ArrayNodeStart(_ *Int64ArrayNode)       {}
func (d DefaultVisitor) VisitInt64ArrayNodeEnd(_ *Int64ArrayNode)         {}
func (d DefaultVisitor) VisitStringArrayNodeStart(_ *StringArrayNode)     {}
func (d DefaultVisitor) VisitStringArrayNodeEnd(_ *StringArrayNode)       {}

func (d DefaultVisitor) VisitBoolSymbolNode(_ *BoolSymbolNode)              {}
func (d DefaultVisitor) VisitDatetimeSymbolNode(_ *DatetimeSymbolNode)      {}
func (d DefaultVisitor) VisitFloat64SymbolNode(_ *Float64SymbolNode)        {}
func (d DefaultVisitor) VisitInt64SymbolNode(_ *Int64SymbolNode)            {}
func (d DefaultVisitor) VisitStringSymbolNode(_ *StringSymbolNode)          {}
func (d DefaultVisitor) VisitAnyTypeSymbolNode(_ *AnyTypeSymbolNode)        {}
func (d DefaultVisitor) VisitInt64ToFloat64NodeStart(_ *Int64ToFloat64Node) {}
func (d DefaultVisitor) VisitInt64ToFloat64NodeEnd(_ *Int64ToFloat64Node)   {}

func (d DefaultVisitor) VisitAllOfSetExprNodeStart(_ *AllOfSetExprNode)     {}
func (d DefaultVisitor) VisitAllOfSetExprNodeEnd(_ *AllOfSetExprNode)       {}
func (d DefaultVisitor) VisitAnyOfSetExprNodeStart(_ *AnyOfSetExprNode)     {}
func (d DefaultVisitor) VisitAnyOfSetExprNodeEnd(_ *AnyOfSetExprNode)       {}
func (d DefaultVisitor) VisitCountSetExprNodeStart(_ *CountSetExprNode)     {}
func (d DefaultVisitor) VisitCountSetExprNodeEnd(_ *CountSetExprNode)       {}
func (d DefaultVisitor) VisitIsEmptySetExprNodeStart(_ *IsEmptySetExprNode) {}
func (d DefaultVisitor) VisitIsEmptySetExprNodeEnd(_ *IsEmptySetExprNode)   {}

func (d DefaultVisitor) VisitSymbol(_ string, _ NodeType) {}
