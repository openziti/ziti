// Code generated from ZitiQl.g4 by ANTLR 4.9.1. DO NOT EDIT.

package zitiql // ZitiQl
import "github.com/antlr/antlr4/runtime/Go/antlr"

// ZitiQlListener is a complete listener for a parse tree produced by ZitiQlParser.
type ZitiQlListener interface {
	antlr.ParseTreeListener

	// EnterStringArray is called when entering the stringArray production.
	EnterStringArray(c *StringArrayContext)

	// EnterNumberArray is called when entering the numberArray production.
	EnterNumberArray(c *NumberArrayContext)

	// EnterDatetimeArray is called when entering the datetimeArray production.
	EnterDatetimeArray(c *DatetimeArrayContext)

	// EnterEnd is called when entering the End production.
	EnterEnd(c *EndContext)

	// EnterQueryStmt is called when entering the QueryStmt production.
	EnterQueryStmt(c *QueryStmtContext)

	// EnterSkipExpr is called when entering the SkipExpr production.
	EnterSkipExpr(c *SkipExprContext)

	// EnterLimitExpr is called when entering the LimitExpr production.
	EnterLimitExpr(c *LimitExprContext)

	// EnterSortByExpr is called when entering the SortByExpr production.
	EnterSortByExpr(c *SortByExprContext)

	// EnterSortFieldExpr is called when entering the SortFieldExpr production.
	EnterSortFieldExpr(c *SortFieldExprContext)

	// EnterAndExpr is called when entering the AndExpr production.
	EnterAndExpr(c *AndExprContext)

	// EnterGroup is called when entering the Group production.
	EnterGroup(c *GroupContext)

	// EnterBoolConst is called when entering the BoolConst production.
	EnterBoolConst(c *BoolConstContext)

	// EnterIsEmptyFunction is called when entering the IsEmptyFunction production.
	EnterIsEmptyFunction(c *IsEmptyFunctionContext)

	// EnterNotExpr is called when entering the NotExpr production.
	EnterNotExpr(c *NotExprContext)

	// EnterOperationOp is called when entering the OperationOp production.
	EnterOperationOp(c *OperationOpContext)

	// EnterOrExpr is called when entering the OrExpr production.
	EnterOrExpr(c *OrExprContext)

	// EnterBoolSymbol is called when entering the BoolSymbol production.
	EnterBoolSymbol(c *BoolSymbolContext)

	// EnterInStringArrayOp is called when entering the InStringArrayOp production.
	EnterInStringArrayOp(c *InStringArrayOpContext)

	// EnterInNumberArrayOp is called when entering the InNumberArrayOp production.
	EnterInNumberArrayOp(c *InNumberArrayOpContext)

	// EnterInDatetimeArrayOp is called when entering the InDatetimeArrayOp production.
	EnterInDatetimeArrayOp(c *InDatetimeArrayOpContext)

	// EnterBetweenNumberOp is called when entering the BetweenNumberOp production.
	EnterBetweenNumberOp(c *BetweenNumberOpContext)

	// EnterBetweenDateOp is called when entering the BetweenDateOp production.
	EnterBetweenDateOp(c *BetweenDateOpContext)

	// EnterBinaryLessThanStringOp is called when entering the BinaryLessThanStringOp production.
	EnterBinaryLessThanStringOp(c *BinaryLessThanStringOpContext)

	// EnterBinaryLessThanNumberOp is called when entering the BinaryLessThanNumberOp production.
	EnterBinaryLessThanNumberOp(c *BinaryLessThanNumberOpContext)

	// EnterBinaryLessThanDatetimeOp is called when entering the BinaryLessThanDatetimeOp production.
	EnterBinaryLessThanDatetimeOp(c *BinaryLessThanDatetimeOpContext)

	// EnterBinaryGreaterThanStringOp is called when entering the BinaryGreaterThanStringOp production.
	EnterBinaryGreaterThanStringOp(c *BinaryGreaterThanStringOpContext)

	// EnterBinaryGreaterThanNumberOp is called when entering the BinaryGreaterThanNumberOp production.
	EnterBinaryGreaterThanNumberOp(c *BinaryGreaterThanNumberOpContext)

	// EnterBinaryGreaterThanDatetimeOp is called when entering the BinaryGreaterThanDatetimeOp production.
	EnterBinaryGreaterThanDatetimeOp(c *BinaryGreaterThanDatetimeOpContext)

	// EnterBinaryEqualToStringOp is called when entering the BinaryEqualToStringOp production.
	EnterBinaryEqualToStringOp(c *BinaryEqualToStringOpContext)

	// EnterBinaryEqualToNumberOp is called when entering the BinaryEqualToNumberOp production.
	EnterBinaryEqualToNumberOp(c *BinaryEqualToNumberOpContext)

	// EnterBinaryEqualToDatetimeOp is called when entering the BinaryEqualToDatetimeOp production.
	EnterBinaryEqualToDatetimeOp(c *BinaryEqualToDatetimeOpContext)

	// EnterBinaryEqualToBoolOp is called when entering the BinaryEqualToBoolOp production.
	EnterBinaryEqualToBoolOp(c *BinaryEqualToBoolOpContext)

	// EnterBinaryEqualToNullOp is called when entering the BinaryEqualToNullOp production.
	EnterBinaryEqualToNullOp(c *BinaryEqualToNullOpContext)

	// EnterBinaryContainsOp is called when entering the BinaryContainsOp production.
	EnterBinaryContainsOp(c *BinaryContainsOpContext)

	// EnterBinaryLhs is called when entering the binaryLhs production.
	EnterBinaryLhs(c *BinaryLhsContext)

	// EnterSetFunctionExpr is called when entering the SetFunctionExpr production.
	EnterSetFunctionExpr(c *SetFunctionExprContext)

	// EnterSetExpr is called when entering the setExpr production.
	EnterSetExpr(c *SetExprContext)

	// EnterSubQuery is called when entering the SubQuery production.
	EnterSubQuery(c *SubQueryContext)

	// ExitStringArray is called when exiting the stringArray production.
	ExitStringArray(c *StringArrayContext)

	// ExitNumberArray is called when exiting the numberArray production.
	ExitNumberArray(c *NumberArrayContext)

	// ExitDatetimeArray is called when exiting the datetimeArray production.
	ExitDatetimeArray(c *DatetimeArrayContext)

	// ExitEnd is called when exiting the End production.
	ExitEnd(c *EndContext)

	// ExitQueryStmt is called when exiting the QueryStmt production.
	ExitQueryStmt(c *QueryStmtContext)

	// ExitSkipExpr is called when exiting the SkipExpr production.
	ExitSkipExpr(c *SkipExprContext)

	// ExitLimitExpr is called when exiting the LimitExpr production.
	ExitLimitExpr(c *LimitExprContext)

	// ExitSortByExpr is called when exiting the SortByExpr production.
	ExitSortByExpr(c *SortByExprContext)

	// ExitSortFieldExpr is called when exiting the SortFieldExpr production.
	ExitSortFieldExpr(c *SortFieldExprContext)

	// ExitAndExpr is called when exiting the AndExpr production.
	ExitAndExpr(c *AndExprContext)

	// ExitGroup is called when exiting the Group production.
	ExitGroup(c *GroupContext)

	// ExitBoolConst is called when exiting the BoolConst production.
	ExitBoolConst(c *BoolConstContext)

	// ExitIsEmptyFunction is called when exiting the IsEmptyFunction production.
	ExitIsEmptyFunction(c *IsEmptyFunctionContext)

	// ExitNotExpr is called when exiting the NotExpr production.
	ExitNotExpr(c *NotExprContext)

	// ExitOperationOp is called when exiting the OperationOp production.
	ExitOperationOp(c *OperationOpContext)

	// ExitOrExpr is called when exiting the OrExpr production.
	ExitOrExpr(c *OrExprContext)

	// ExitBoolSymbol is called when exiting the BoolSymbol production.
	ExitBoolSymbol(c *BoolSymbolContext)

	// ExitInStringArrayOp is called when exiting the InStringArrayOp production.
	ExitInStringArrayOp(c *InStringArrayOpContext)

	// ExitInNumberArrayOp is called when exiting the InNumberArrayOp production.
	ExitInNumberArrayOp(c *InNumberArrayOpContext)

	// ExitInDatetimeArrayOp is called when exiting the InDatetimeArrayOp production.
	ExitInDatetimeArrayOp(c *InDatetimeArrayOpContext)

	// ExitBetweenNumberOp is called when exiting the BetweenNumberOp production.
	ExitBetweenNumberOp(c *BetweenNumberOpContext)

	// ExitBetweenDateOp is called when exiting the BetweenDateOp production.
	ExitBetweenDateOp(c *BetweenDateOpContext)

	// ExitBinaryLessThanStringOp is called when exiting the BinaryLessThanStringOp production.
	ExitBinaryLessThanStringOp(c *BinaryLessThanStringOpContext)

	// ExitBinaryLessThanNumberOp is called when exiting the BinaryLessThanNumberOp production.
	ExitBinaryLessThanNumberOp(c *BinaryLessThanNumberOpContext)

	// ExitBinaryLessThanDatetimeOp is called when exiting the BinaryLessThanDatetimeOp production.
	ExitBinaryLessThanDatetimeOp(c *BinaryLessThanDatetimeOpContext)

	// ExitBinaryGreaterThanStringOp is called when exiting the BinaryGreaterThanStringOp production.
	ExitBinaryGreaterThanStringOp(c *BinaryGreaterThanStringOpContext)

	// ExitBinaryGreaterThanNumberOp is called when exiting the BinaryGreaterThanNumberOp production.
	ExitBinaryGreaterThanNumberOp(c *BinaryGreaterThanNumberOpContext)

	// ExitBinaryGreaterThanDatetimeOp is called when exiting the BinaryGreaterThanDatetimeOp production.
	ExitBinaryGreaterThanDatetimeOp(c *BinaryGreaterThanDatetimeOpContext)

	// ExitBinaryEqualToStringOp is called when exiting the BinaryEqualToStringOp production.
	ExitBinaryEqualToStringOp(c *BinaryEqualToStringOpContext)

	// ExitBinaryEqualToNumberOp is called when exiting the BinaryEqualToNumberOp production.
	ExitBinaryEqualToNumberOp(c *BinaryEqualToNumberOpContext)

	// ExitBinaryEqualToDatetimeOp is called when exiting the BinaryEqualToDatetimeOp production.
	ExitBinaryEqualToDatetimeOp(c *BinaryEqualToDatetimeOpContext)

	// ExitBinaryEqualToBoolOp is called when exiting the BinaryEqualToBoolOp production.
	ExitBinaryEqualToBoolOp(c *BinaryEqualToBoolOpContext)

	// ExitBinaryEqualToNullOp is called when exiting the BinaryEqualToNullOp production.
	ExitBinaryEqualToNullOp(c *BinaryEqualToNullOpContext)

	// ExitBinaryContainsOp is called when exiting the BinaryContainsOp production.
	ExitBinaryContainsOp(c *BinaryContainsOpContext)

	// ExitBinaryLhs is called when exiting the binaryLhs production.
	ExitBinaryLhs(c *BinaryLhsContext)

	// ExitSetFunctionExpr is called when exiting the SetFunctionExpr production.
	ExitSetFunctionExpr(c *SetFunctionExprContext)

	// ExitSetExpr is called when exiting the setExpr production.
	ExitSetExpr(c *SetExprContext)

	// ExitSubQuery is called when exiting the SubQuery production.
	ExitSubQuery(c *SubQueryContext)
}
