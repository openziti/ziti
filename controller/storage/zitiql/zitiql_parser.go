// Code generated from ZitiQl.g4 by ANTLR 4.13.1. DO NOT EDIT.

package zitiql // ZitiQl
import (
	"fmt"
	"strconv"
	"sync"

	"github.com/antlr4-go/antlr/v4"
)

// Suppress unused import errors
var _ = fmt.Printf
var _ = strconv.Itoa
var _ = sync.Once{}

type ZitiQlParser struct {
	*antlr.BaseParser
}

var ZitiQlParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	LiteralNames           []string
	SymbolicNames          []string
	RuleNames              []string
	PredictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func zitiqlParserInit() {
	staticData := &ZitiQlParserStaticData
	staticData.LiteralNames = []string{
		"", "','", "", "'('", "')'", "'['", "']'",
	}
	staticData.SymbolicNames = []string{
		"", "", "WS", "LPAREN", "RPAREN", "LBRACKET", "RBRACKET", "AND", "OR",
		"LT", "GT", "EQ", "CONTAINS", "ICONTAINS", "IN", "BETWEEN", "BOOL",
		"DATETIME", "ALL_OF", "ANY_OF", "COUNT", "ISEMPTY", "STRING", "NUMBER",
		"NULL", "NOT", "ASC", "DESC", "SORT", "BY", "SKIP_ROWS", "LIMIT_ROWS",
		"NONE", "WHERE", "FROM", "IDENTIFIER", "RFC3339_DATE_TIME",
	}
	staticData.RuleNames = []string{
		"stringArray", "numberArray", "datetimeArray", "start", "query", "skip",
		"limit", "sortBy", "sortField", "boolExpr", "operation", "binaryLhs",
		"setFunction", "setExpr", "subQueryExpr",
	}
	staticData.PredictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 36, 728, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 1, 0, 1, 0,
		5, 0, 33, 8, 0, 10, 0, 12, 0, 36, 9, 0, 1, 0, 1, 0, 5, 0, 40, 8, 0, 10,
		0, 12, 0, 43, 9, 0, 1, 0, 1, 0, 5, 0, 47, 8, 0, 10, 0, 12, 0, 50, 9, 0,
		1, 0, 5, 0, 53, 8, 0, 10, 0, 12, 0, 56, 9, 0, 1, 0, 5, 0, 59, 8, 0, 10,
		0, 12, 0, 62, 9, 0, 1, 0, 1, 0, 1, 1, 1, 1, 5, 1, 68, 8, 1, 10, 1, 12,
		1, 71, 9, 1, 1, 1, 1, 1, 5, 1, 75, 8, 1, 10, 1, 12, 1, 78, 9, 1, 1, 1,
		1, 1, 5, 1, 82, 8, 1, 10, 1, 12, 1, 85, 9, 1, 1, 1, 5, 1, 88, 8, 1, 10,
		1, 12, 1, 91, 9, 1, 1, 1, 5, 1, 94, 8, 1, 10, 1, 12, 1, 97, 9, 1, 1, 1,
		1, 1, 1, 2, 1, 2, 5, 2, 103, 8, 2, 10, 2, 12, 2, 106, 9, 2, 1, 2, 1, 2,
		5, 2, 110, 8, 2, 10, 2, 12, 2, 113, 9, 2, 1, 2, 1, 2, 5, 2, 117, 8, 2,
		10, 2, 12, 2, 120, 9, 2, 1, 2, 5, 2, 123, 8, 2, 10, 2, 12, 2, 126, 9, 2,
		1, 2, 5, 2, 129, 8, 2, 10, 2, 12, 2, 132, 9, 2, 1, 2, 1, 2, 1, 3, 5, 3,
		137, 8, 3, 10, 3, 12, 3, 140, 9, 3, 1, 3, 1, 3, 5, 3, 144, 8, 3, 10, 3,
		12, 3, 147, 9, 3, 1, 3, 1, 3, 1, 4, 1, 4, 4, 4, 153, 8, 4, 11, 4, 12, 4,
		154, 1, 4, 3, 4, 158, 8, 4, 1, 4, 4, 4, 161, 8, 4, 11, 4, 12, 4, 162, 1,
		4, 3, 4, 166, 8, 4, 1, 4, 4, 4, 169, 8, 4, 11, 4, 12, 4, 170, 1, 4, 3,
		4, 174, 8, 4, 1, 4, 1, 4, 4, 4, 178, 8, 4, 11, 4, 12, 4, 179, 1, 4, 3,
		4, 183, 8, 4, 1, 4, 4, 4, 186, 8, 4, 11, 4, 12, 4, 187, 1, 4, 3, 4, 191,
		8, 4, 1, 4, 1, 4, 4, 4, 195, 8, 4, 11, 4, 12, 4, 196, 1, 4, 3, 4, 200,
		8, 4, 1, 4, 3, 4, 203, 8, 4, 1, 5, 1, 5, 4, 5, 207, 8, 5, 11, 5, 12, 5,
		208, 1, 5, 1, 5, 1, 6, 1, 6, 4, 6, 215, 8, 6, 11, 6, 12, 6, 216, 1, 6,
		1, 6, 1, 7, 1, 7, 4, 7, 223, 8, 7, 11, 7, 12, 7, 224, 1, 7, 1, 7, 4, 7,
		229, 8, 7, 11, 7, 12, 7, 230, 1, 7, 1, 7, 5, 7, 235, 8, 7, 10, 7, 12, 7,
		238, 9, 7, 1, 7, 1, 7, 5, 7, 242, 8, 7, 10, 7, 12, 7, 245, 9, 7, 1, 7,
		5, 7, 248, 8, 7, 10, 7, 12, 7, 251, 9, 7, 1, 8, 1, 8, 4, 8, 255, 8, 8,
		11, 8, 12, 8, 256, 1, 8, 3, 8, 260, 8, 8, 1, 9, 1, 9, 1, 9, 1, 9, 5, 9,
		266, 8, 9, 10, 9, 12, 9, 269, 9, 9, 1, 9, 1, 9, 5, 9, 273, 8, 9, 10, 9,
		12, 9, 276, 9, 9, 1, 9, 1, 9, 1, 9, 1, 9, 1, 9, 1, 9, 5, 9, 284, 8, 9,
		10, 9, 12, 9, 287, 9, 9, 1, 9, 1, 9, 5, 9, 291, 8, 9, 10, 9, 12, 9, 294,
		9, 9, 1, 9, 1, 9, 1, 9, 1, 9, 1, 9, 4, 9, 301, 8, 9, 11, 9, 12, 9, 302,
		1, 9, 3, 9, 306, 8, 9, 1, 9, 1, 9, 4, 9, 310, 8, 9, 11, 9, 12, 9, 311,
		1, 9, 1, 9, 4, 9, 316, 8, 9, 11, 9, 12, 9, 317, 1, 9, 4, 9, 321, 8, 9,
		11, 9, 12, 9, 322, 1, 9, 1, 9, 4, 9, 327, 8, 9, 11, 9, 12, 9, 328, 1, 9,
		1, 9, 4, 9, 333, 8, 9, 11, 9, 12, 9, 334, 1, 9, 4, 9, 338, 8, 9, 11, 9,
		12, 9, 339, 5, 9, 342, 8, 9, 10, 9, 12, 9, 345, 9, 9, 1, 10, 1, 10, 4,
		10, 349, 8, 10, 11, 10, 12, 10, 350, 1, 10, 1, 10, 4, 10, 355, 8, 10, 11,
		10, 12, 10, 356, 1, 10, 1, 10, 1, 10, 1, 10, 4, 10, 363, 8, 10, 11, 10,
		12, 10, 364, 1, 10, 1, 10, 4, 10, 369, 8, 10, 11, 10, 12, 10, 370, 1, 10,
		1, 10, 1, 10, 1, 10, 4, 10, 377, 8, 10, 11, 10, 12, 10, 378, 1, 10, 1,
		10, 4, 10, 383, 8, 10, 11, 10, 12, 10, 384, 1, 10, 1, 10, 1, 10, 1, 10,
		4, 10, 391, 8, 10, 11, 10, 12, 10, 392, 1, 10, 1, 10, 4, 10, 397, 8, 10,
		11, 10, 12, 10, 398, 1, 10, 1, 10, 4, 10, 403, 8, 10, 11, 10, 12, 10, 404,
		1, 10, 1, 10, 4, 10, 409, 8, 10, 11, 10, 12, 10, 410, 1, 10, 1, 10, 1,
		10, 1, 10, 4, 10, 417, 8, 10, 11, 10, 12, 10, 418, 1, 10, 1, 10, 4, 10,
		423, 8, 10, 11, 10, 12, 10, 424, 1, 10, 1, 10, 4, 10, 429, 8, 10, 11, 10,
		12, 10, 430, 1, 10, 1, 10, 4, 10, 435, 8, 10, 11, 10, 12, 10, 436, 1, 10,
		1, 10, 1, 10, 1, 10, 5, 10, 443, 8, 10, 10, 10, 12, 10, 446, 9, 10, 1,
		10, 1, 10, 5, 10, 450, 8, 10, 10, 10, 12, 10, 453, 9, 10, 1, 10, 1, 10,
		1, 10, 1, 10, 5, 10, 459, 8, 10, 10, 10, 12, 10, 462, 9, 10, 1, 10, 1,
		10, 5, 10, 466, 8, 10, 10, 10, 12, 10, 469, 9, 10, 1, 10, 1, 10, 1, 10,
		1, 10, 5, 10, 475, 8, 10, 10, 10, 12, 10, 478, 9, 10, 1, 10, 1, 10, 5,
		10, 482, 8, 10, 10, 10, 12, 10, 485, 9, 10, 1, 10, 1, 10, 1, 10, 1, 10,
		5, 10, 491, 8, 10, 10, 10, 12, 10, 494, 9, 10, 1, 10, 1, 10, 5, 10, 498,
		8, 10, 10, 10, 12, 10, 501, 9, 10, 1, 10, 1, 10, 1, 10, 1, 10, 5, 10, 507,
		8, 10, 10, 10, 12, 10, 510, 9, 10, 1, 10, 1, 10, 5, 10, 514, 8, 10, 10,
		10, 12, 10, 517, 9, 10, 1, 10, 1, 10, 1, 10, 1, 10, 5, 10, 523, 8, 10,
		10, 10, 12, 10, 526, 9, 10, 1, 10, 1, 10, 5, 10, 530, 8, 10, 10, 10, 12,
		10, 533, 9, 10, 1, 10, 1, 10, 1, 10, 1, 10, 5, 10, 539, 8, 10, 10, 10,
		12, 10, 542, 9, 10, 1, 10, 1, 10, 5, 10, 546, 8, 10, 10, 10, 12, 10, 549,
		9, 10, 1, 10, 1, 10, 1, 10, 1, 10, 5, 10, 555, 8, 10, 10, 10, 12, 10, 558,
		9, 10, 1, 10, 1, 10, 5, 10, 562, 8, 10, 10, 10, 12, 10, 565, 9, 10, 1,
		10, 1, 10, 1, 10, 1, 10, 5, 10, 571, 8, 10, 10, 10, 12, 10, 574, 9, 10,
		1, 10, 1, 10, 5, 10, 578, 8, 10, 10, 10, 12, 10, 581, 9, 10, 1, 10, 1,
		10, 1, 10, 1, 10, 5, 10, 587, 8, 10, 10, 10, 12, 10, 590, 9, 10, 1, 10,
		1, 10, 5, 10, 594, 8, 10, 10, 10, 12, 10, 597, 9, 10, 1, 10, 1, 10, 1,
		10, 1, 10, 5, 10, 603, 8, 10, 10, 10, 12, 10, 606, 9, 10, 1, 10, 1, 10,
		5, 10, 610, 8, 10, 10, 10, 12, 10, 613, 9, 10, 1, 10, 1, 10, 1, 10, 1,
		10, 5, 10, 619, 8, 10, 10, 10, 12, 10, 622, 9, 10, 1, 10, 1, 10, 4, 10,
		626, 8, 10, 11, 10, 12, 10, 627, 1, 10, 1, 10, 1, 10, 1, 10, 5, 10, 634,
		8, 10, 10, 10, 12, 10, 637, 9, 10, 1, 10, 1, 10, 4, 10, 641, 8, 10, 11,
		10, 12, 10, 642, 1, 10, 1, 10, 3, 10, 647, 8, 10, 1, 11, 1, 11, 3, 11,
		651, 8, 11, 1, 12, 1, 12, 1, 12, 5, 12, 656, 8, 12, 10, 12, 12, 12, 659,
		9, 12, 1, 12, 1, 12, 5, 12, 663, 8, 12, 10, 12, 12, 12, 666, 9, 12, 1,
		12, 1, 12, 1, 12, 1, 12, 5, 12, 672, 8, 12, 10, 12, 12, 12, 675, 9, 12,
		1, 12, 1, 12, 5, 12, 679, 8, 12, 10, 12, 12, 12, 682, 9, 12, 1, 12, 1,
		12, 1, 12, 1, 12, 5, 12, 688, 8, 12, 10, 12, 12, 12, 691, 9, 12, 1, 12,
		1, 12, 5, 12, 695, 8, 12, 10, 12, 12, 12, 698, 9, 12, 1, 12, 1, 12, 3,
		12, 702, 8, 12, 1, 13, 1, 13, 3, 13, 706, 8, 13, 1, 14, 1, 14, 4, 14, 710,
		8, 14, 11, 14, 12, 14, 711, 1, 14, 1, 14, 4, 14, 716, 8, 14, 11, 14, 12,
		14, 717, 1, 14, 1, 14, 4, 14, 722, 8, 14, 11, 14, 12, 14, 723, 1, 14, 1,
		14, 1, 14, 0, 1, 18, 15, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24,
		26, 28, 0, 3, 2, 0, 23, 23, 32, 32, 1, 0, 26, 27, 1, 0, 22, 23, 841, 0,
		30, 1, 0, 0, 0, 2, 65, 1, 0, 0, 0, 4, 100, 1, 0, 0, 0, 6, 138, 1, 0, 0,
		0, 8, 202, 1, 0, 0, 0, 10, 204, 1, 0, 0, 0, 12, 212, 1, 0, 0, 0, 14, 220,
		1, 0, 0, 0, 16, 252, 1, 0, 0, 0, 18, 305, 1, 0, 0, 0, 20, 646, 1, 0, 0,
		0, 22, 650, 1, 0, 0, 0, 24, 701, 1, 0, 0, 0, 26, 705, 1, 0, 0, 0, 28, 707,
		1, 0, 0, 0, 30, 34, 5, 5, 0, 0, 31, 33, 5, 2, 0, 0, 32, 31, 1, 0, 0, 0,
		33, 36, 1, 0, 0, 0, 34, 32, 1, 0, 0, 0, 34, 35, 1, 0, 0, 0, 35, 37, 1,
		0, 0, 0, 36, 34, 1, 0, 0, 0, 37, 54, 5, 22, 0, 0, 38, 40, 5, 2, 0, 0, 39,
		38, 1, 0, 0, 0, 40, 43, 1, 0, 0, 0, 41, 39, 1, 0, 0, 0, 41, 42, 1, 0, 0,
		0, 42, 44, 1, 0, 0, 0, 43, 41, 1, 0, 0, 0, 44, 48, 5, 1, 0, 0, 45, 47,
		5, 2, 0, 0, 46, 45, 1, 0, 0, 0, 47, 50, 1, 0, 0, 0, 48, 46, 1, 0, 0, 0,
		48, 49, 1, 0, 0, 0, 49, 51, 1, 0, 0, 0, 50, 48, 1, 0, 0, 0, 51, 53, 5,
		22, 0, 0, 52, 41, 1, 0, 0, 0, 53, 56, 1, 0, 0, 0, 54, 52, 1, 0, 0, 0, 54,
		55, 1, 0, 0, 0, 55, 60, 1, 0, 0, 0, 56, 54, 1, 0, 0, 0, 57, 59, 5, 2, 0,
		0, 58, 57, 1, 0, 0, 0, 59, 62, 1, 0, 0, 0, 60, 58, 1, 0, 0, 0, 60, 61,
		1, 0, 0, 0, 61, 63, 1, 0, 0, 0, 62, 60, 1, 0, 0, 0, 63, 64, 5, 6, 0, 0,
		64, 1, 1, 0, 0, 0, 65, 69, 5, 5, 0, 0, 66, 68, 5, 2, 0, 0, 67, 66, 1, 0,
		0, 0, 68, 71, 1, 0, 0, 0, 69, 67, 1, 0, 0, 0, 69, 70, 1, 0, 0, 0, 70, 72,
		1, 0, 0, 0, 71, 69, 1, 0, 0, 0, 72, 89, 5, 23, 0, 0, 73, 75, 5, 2, 0, 0,
		74, 73, 1, 0, 0, 0, 75, 78, 1, 0, 0, 0, 76, 74, 1, 0, 0, 0, 76, 77, 1,
		0, 0, 0, 77, 79, 1, 0, 0, 0, 78, 76, 1, 0, 0, 0, 79, 83, 5, 1, 0, 0, 80,
		82, 5, 2, 0, 0, 81, 80, 1, 0, 0, 0, 82, 85, 1, 0, 0, 0, 83, 81, 1, 0, 0,
		0, 83, 84, 1, 0, 0, 0, 84, 86, 1, 0, 0, 0, 85, 83, 1, 0, 0, 0, 86, 88,
		5, 23, 0, 0, 87, 76, 1, 0, 0, 0, 88, 91, 1, 0, 0, 0, 89, 87, 1, 0, 0, 0,
		89, 90, 1, 0, 0, 0, 90, 95, 1, 0, 0, 0, 91, 89, 1, 0, 0, 0, 92, 94, 5,
		2, 0, 0, 93, 92, 1, 0, 0, 0, 94, 97, 1, 0, 0, 0, 95, 93, 1, 0, 0, 0, 95,
		96, 1, 0, 0, 0, 96, 98, 1, 0, 0, 0, 97, 95, 1, 0, 0, 0, 98, 99, 5, 6, 0,
		0, 99, 3, 1, 0, 0, 0, 100, 104, 5, 5, 0, 0, 101, 103, 5, 2, 0, 0, 102,
		101, 1, 0, 0, 0, 103, 106, 1, 0, 0, 0, 104, 102, 1, 0, 0, 0, 104, 105,
		1, 0, 0, 0, 105, 107, 1, 0, 0, 0, 106, 104, 1, 0, 0, 0, 107, 124, 5, 17,
		0, 0, 108, 110, 5, 2, 0, 0, 109, 108, 1, 0, 0, 0, 110, 113, 1, 0, 0, 0,
		111, 109, 1, 0, 0, 0, 111, 112, 1, 0, 0, 0, 112, 114, 1, 0, 0, 0, 113,
		111, 1, 0, 0, 0, 114, 118, 5, 1, 0, 0, 115, 117, 5, 2, 0, 0, 116, 115,
		1, 0, 0, 0, 117, 120, 1, 0, 0, 0, 118, 116, 1, 0, 0, 0, 118, 119, 1, 0,
		0, 0, 119, 121, 1, 0, 0, 0, 120, 118, 1, 0, 0, 0, 121, 123, 5, 17, 0, 0,
		122, 111, 1, 0, 0, 0, 123, 126, 1, 0, 0, 0, 124, 122, 1, 0, 0, 0, 124,
		125, 1, 0, 0, 0, 125, 130, 1, 0, 0, 0, 126, 124, 1, 0, 0, 0, 127, 129,
		5, 2, 0, 0, 128, 127, 1, 0, 0, 0, 129, 132, 1, 0, 0, 0, 130, 128, 1, 0,
		0, 0, 130, 131, 1, 0, 0, 0, 131, 133, 1, 0, 0, 0, 132, 130, 1, 0, 0, 0,
		133, 134, 5, 6, 0, 0, 134, 5, 1, 0, 0, 0, 135, 137, 5, 2, 0, 0, 136, 135,
		1, 0, 0, 0, 137, 140, 1, 0, 0, 0, 138, 136, 1, 0, 0, 0, 138, 139, 1, 0,
		0, 0, 139, 141, 1, 0, 0, 0, 140, 138, 1, 0, 0, 0, 141, 145, 3, 8, 4, 0,
		142, 144, 5, 2, 0, 0, 143, 142, 1, 0, 0, 0, 144, 147, 1, 0, 0, 0, 145,
		143, 1, 0, 0, 0, 145, 146, 1, 0, 0, 0, 146, 148, 1, 0, 0, 0, 147, 145,
		1, 0, 0, 0, 148, 149, 5, 0, 0, 1, 149, 7, 1, 0, 0, 0, 150, 157, 3, 18,
		9, 0, 151, 153, 5, 2, 0, 0, 152, 151, 1, 0, 0, 0, 153, 154, 1, 0, 0, 0,
		154, 152, 1, 0, 0, 0, 154, 155, 1, 0, 0, 0, 155, 156, 1, 0, 0, 0, 156,
		158, 3, 14, 7, 0, 157, 152, 1, 0, 0, 0, 157, 158, 1, 0, 0, 0, 158, 165,
		1, 0, 0, 0, 159, 161, 5, 2, 0, 0, 160, 159, 1, 0, 0, 0, 161, 162, 1, 0,
		0, 0, 162, 160, 1, 0, 0, 0, 162, 163, 1, 0, 0, 0, 163, 164, 1, 0, 0, 0,
		164, 166, 3, 10, 5, 0, 165, 160, 1, 0, 0, 0, 165, 166, 1, 0, 0, 0, 166,
		173, 1, 0, 0, 0, 167, 169, 5, 2, 0, 0, 168, 167, 1, 0, 0, 0, 169, 170,
		1, 0, 0, 0, 170, 168, 1, 0, 0, 0, 170, 171, 1, 0, 0, 0, 171, 172, 1, 0,
		0, 0, 172, 174, 3, 12, 6, 0, 173, 168, 1, 0, 0, 0, 173, 174, 1, 0, 0, 0,
		174, 203, 1, 0, 0, 0, 175, 182, 3, 14, 7, 0, 176, 178, 5, 2, 0, 0, 177,
		176, 1, 0, 0, 0, 178, 179, 1, 0, 0, 0, 179, 177, 1, 0, 0, 0, 179, 180,
		1, 0, 0, 0, 180, 181, 1, 0, 0, 0, 181, 183, 3, 10, 5, 0, 182, 177, 1, 0,
		0, 0, 182, 183, 1, 0, 0, 0, 183, 190, 1, 0, 0, 0, 184, 186, 5, 2, 0, 0,
		185, 184, 1, 0, 0, 0, 186, 187, 1, 0, 0, 0, 187, 185, 1, 0, 0, 0, 187,
		188, 1, 0, 0, 0, 188, 189, 1, 0, 0, 0, 189, 191, 3, 12, 6, 0, 190, 185,
		1, 0, 0, 0, 190, 191, 1, 0, 0, 0, 191, 203, 1, 0, 0, 0, 192, 199, 3, 10,
		5, 0, 193, 195, 5, 2, 0, 0, 194, 193, 1, 0, 0, 0, 195, 196, 1, 0, 0, 0,
		196, 194, 1, 0, 0, 0, 196, 197, 1, 0, 0, 0, 197, 198, 1, 0, 0, 0, 198,
		200, 3, 12, 6, 0, 199, 194, 1, 0, 0, 0, 199, 200, 1, 0, 0, 0, 200, 203,
		1, 0, 0, 0, 201, 203, 3, 12, 6, 0, 202, 150, 1, 0, 0, 0, 202, 175, 1, 0,
		0, 0, 202, 192, 1, 0, 0, 0, 202, 201, 1, 0, 0, 0, 203, 9, 1, 0, 0, 0, 204,
		206, 5, 30, 0, 0, 205, 207, 5, 2, 0, 0, 206, 205, 1, 0, 0, 0, 207, 208,
		1, 0, 0, 0, 208, 206, 1, 0, 0, 0, 208, 209, 1, 0, 0, 0, 209, 210, 1, 0,
		0, 0, 210, 211, 5, 23, 0, 0, 211, 11, 1, 0, 0, 0, 212, 214, 5, 31, 0, 0,
		213, 215, 5, 2, 0, 0, 214, 213, 1, 0, 0, 0, 215, 216, 1, 0, 0, 0, 216,
		214, 1, 0, 0, 0, 216, 217, 1, 0, 0, 0, 217, 218, 1, 0, 0, 0, 218, 219,
		7, 0, 0, 0, 219, 13, 1, 0, 0, 0, 220, 222, 5, 28, 0, 0, 221, 223, 5, 2,
		0, 0, 222, 221, 1, 0, 0, 0, 223, 224, 1, 0, 0, 0, 224, 222, 1, 0, 0, 0,
		224, 225, 1, 0, 0, 0, 225, 226, 1, 0, 0, 0, 226, 228, 5, 29, 0, 0, 227,
		229, 5, 2, 0, 0, 228, 227, 1, 0, 0, 0, 229, 230, 1, 0, 0, 0, 230, 228,
		1, 0, 0, 0, 230, 231, 1, 0, 0, 0, 231, 232, 1, 0, 0, 0, 232, 249, 3, 16,
		8, 0, 233, 235, 5, 2, 0, 0, 234, 233, 1, 0, 0, 0, 235, 238, 1, 0, 0, 0,
		236, 234, 1, 0, 0, 0, 236, 237, 1, 0, 0, 0, 237, 239, 1, 0, 0, 0, 238,
		236, 1, 0, 0, 0, 239, 243, 5, 1, 0, 0, 240, 242, 5, 2, 0, 0, 241, 240,
		1, 0, 0, 0, 242, 245, 1, 0, 0, 0, 243, 241, 1, 0, 0, 0, 243, 244, 1, 0,
		0, 0, 244, 246, 1, 0, 0, 0, 245, 243, 1, 0, 0, 0, 246, 248, 3, 16, 8, 0,
		247, 236, 1, 0, 0, 0, 248, 251, 1, 0, 0, 0, 249, 247, 1, 0, 0, 0, 249,
		250, 1, 0, 0, 0, 250, 15, 1, 0, 0, 0, 251, 249, 1, 0, 0, 0, 252, 259, 5,
		35, 0, 0, 253, 255, 5, 2, 0, 0, 254, 253, 1, 0, 0, 0, 255, 256, 1, 0, 0,
		0, 256, 254, 1, 0, 0, 0, 256, 257, 1, 0, 0, 0, 257, 258, 1, 0, 0, 0, 258,
		260, 7, 1, 0, 0, 259, 254, 1, 0, 0, 0, 259, 260, 1, 0, 0, 0, 260, 17, 1,
		0, 0, 0, 261, 262, 6, 9, -1, 0, 262, 306, 3, 20, 10, 0, 263, 267, 5, 3,
		0, 0, 264, 266, 5, 2, 0, 0, 265, 264, 1, 0, 0, 0, 266, 269, 1, 0, 0, 0,
		267, 265, 1, 0, 0, 0, 267, 268, 1, 0, 0, 0, 268, 270, 1, 0, 0, 0, 269,
		267, 1, 0, 0, 0, 270, 274, 3, 18, 9, 0, 271, 273, 5, 2, 0, 0, 272, 271,
		1, 0, 0, 0, 273, 276, 1, 0, 0, 0, 274, 272, 1, 0, 0, 0, 274, 275, 1, 0,
		0, 0, 275, 277, 1, 0, 0, 0, 276, 274, 1, 0, 0, 0, 277, 278, 5, 4, 0, 0,
		278, 306, 1, 0, 0, 0, 279, 306, 5, 16, 0, 0, 280, 281, 5, 21, 0, 0, 281,
		285, 5, 3, 0, 0, 282, 284, 5, 2, 0, 0, 283, 282, 1, 0, 0, 0, 284, 287,
		1, 0, 0, 0, 285, 283, 1, 0, 0, 0, 285, 286, 1, 0, 0, 0, 286, 288, 1, 0,
		0, 0, 287, 285, 1, 0, 0, 0, 288, 292, 3, 26, 13, 0, 289, 291, 5, 2, 0,
		0, 290, 289, 1, 0, 0, 0, 291, 294, 1, 0, 0, 0, 292, 290, 1, 0, 0, 0, 292,
		293, 1, 0, 0, 0, 293, 295, 1, 0, 0, 0, 294, 292, 1, 0, 0, 0, 295, 296,
		5, 4, 0, 0, 296, 306, 1, 0, 0, 0, 297, 306, 5, 35, 0, 0, 298, 300, 5, 25,
		0, 0, 299, 301, 5, 2, 0, 0, 300, 299, 1, 0, 0, 0, 301, 302, 1, 0, 0, 0,
		302, 300, 1, 0, 0, 0, 302, 303, 1, 0, 0, 0, 303, 304, 1, 0, 0, 0, 304,
		306, 3, 18, 9, 1, 305, 261, 1, 0, 0, 0, 305, 263, 1, 0, 0, 0, 305, 279,
		1, 0, 0, 0, 305, 280, 1, 0, 0, 0, 305, 297, 1, 0, 0, 0, 305, 298, 1, 0,
		0, 0, 306, 343, 1, 0, 0, 0, 307, 320, 10, 6, 0, 0, 308, 310, 5, 2, 0, 0,
		309, 308, 1, 0, 0, 0, 310, 311, 1, 0, 0, 0, 311, 309, 1, 0, 0, 0, 311,
		312, 1, 0, 0, 0, 312, 313, 1, 0, 0, 0, 313, 315, 5, 7, 0, 0, 314, 316,
		5, 2, 0, 0, 315, 314, 1, 0, 0, 0, 316, 317, 1, 0, 0, 0, 317, 315, 1, 0,
		0, 0, 317, 318, 1, 0, 0, 0, 318, 319, 1, 0, 0, 0, 319, 321, 3, 18, 9, 0,
		320, 309, 1, 0, 0, 0, 321, 322, 1, 0, 0, 0, 322, 320, 1, 0, 0, 0, 322,
		323, 1, 0, 0, 0, 323, 342, 1, 0, 0, 0, 324, 337, 10, 5, 0, 0, 325, 327,
		5, 2, 0, 0, 326, 325, 1, 0, 0, 0, 327, 328, 1, 0, 0, 0, 328, 326, 1, 0,
		0, 0, 328, 329, 1, 0, 0, 0, 329, 330, 1, 0, 0, 0, 330, 332, 5, 8, 0, 0,
		331, 333, 5, 2, 0, 0, 332, 331, 1, 0, 0, 0, 333, 334, 1, 0, 0, 0, 334,
		332, 1, 0, 0, 0, 334, 335, 1, 0, 0, 0, 335, 336, 1, 0, 0, 0, 336, 338,
		3, 18, 9, 0, 337, 326, 1, 0, 0, 0, 338, 339, 1, 0, 0, 0, 339, 337, 1, 0,
		0, 0, 339, 340, 1, 0, 0, 0, 340, 342, 1, 0, 0, 0, 341, 307, 1, 0, 0, 0,
		341, 324, 1, 0, 0, 0, 342, 345, 1, 0, 0, 0, 343, 341, 1, 0, 0, 0, 343,
		344, 1, 0, 0, 0, 344, 19, 1, 0, 0, 0, 345, 343, 1, 0, 0, 0, 346, 348, 3,
		22, 11, 0, 347, 349, 5, 2, 0, 0, 348, 347, 1, 0, 0, 0, 349, 350, 1, 0,
		0, 0, 350, 348, 1, 0, 0, 0, 350, 351, 1, 0, 0, 0, 351, 352, 1, 0, 0, 0,
		352, 354, 5, 14, 0, 0, 353, 355, 5, 2, 0, 0, 354, 353, 1, 0, 0, 0, 355,
		356, 1, 0, 0, 0, 356, 354, 1, 0, 0, 0, 356, 357, 1, 0, 0, 0, 357, 358,
		1, 0, 0, 0, 358, 359, 3, 0, 0, 0, 359, 647, 1, 0, 0, 0, 360, 362, 3, 22,
		11, 0, 361, 363, 5, 2, 0, 0, 362, 361, 1, 0, 0, 0, 363, 364, 1, 0, 0, 0,
		364, 362, 1, 0, 0, 0, 364, 365, 1, 0, 0, 0, 365, 366, 1, 0, 0, 0, 366,
		368, 5, 14, 0, 0, 367, 369, 5, 2, 0, 0, 368, 367, 1, 0, 0, 0, 369, 370,
		1, 0, 0, 0, 370, 368, 1, 0, 0, 0, 370, 371, 1, 0, 0, 0, 371, 372, 1, 0,
		0, 0, 372, 373, 3, 2, 1, 0, 373, 647, 1, 0, 0, 0, 374, 376, 3, 22, 11,
		0, 375, 377, 5, 2, 0, 0, 376, 375, 1, 0, 0, 0, 377, 378, 1, 0, 0, 0, 378,
		376, 1, 0, 0, 0, 378, 379, 1, 0, 0, 0, 379, 380, 1, 0, 0, 0, 380, 382,
		5, 14, 0, 0, 381, 383, 5, 2, 0, 0, 382, 381, 1, 0, 0, 0, 383, 384, 1, 0,
		0, 0, 384, 382, 1, 0, 0, 0, 384, 385, 1, 0, 0, 0, 385, 386, 1, 0, 0, 0,
		386, 387, 3, 4, 2, 0, 387, 647, 1, 0, 0, 0, 388, 390, 3, 22, 11, 0, 389,
		391, 5, 2, 0, 0, 390, 389, 1, 0, 0, 0, 391, 392, 1, 0, 0, 0, 392, 390,
		1, 0, 0, 0, 392, 393, 1, 0, 0, 0, 393, 394, 1, 0, 0, 0, 394, 396, 5, 15,
		0, 0, 395, 397, 5, 2, 0, 0, 396, 395, 1, 0, 0, 0, 397, 398, 1, 0, 0, 0,
		398, 396, 1, 0, 0, 0, 398, 399, 1, 0, 0, 0, 399, 400, 1, 0, 0, 0, 400,
		402, 5, 23, 0, 0, 401, 403, 5, 2, 0, 0, 402, 401, 1, 0, 0, 0, 403, 404,
		1, 0, 0, 0, 404, 402, 1, 0, 0, 0, 404, 405, 1, 0, 0, 0, 405, 406, 1, 0,
		0, 0, 406, 408, 5, 7, 0, 0, 407, 409, 5, 2, 0, 0, 408, 407, 1, 0, 0, 0,
		409, 410, 1, 0, 0, 0, 410, 408, 1, 0, 0, 0, 410, 411, 1, 0, 0, 0, 411,
		412, 1, 0, 0, 0, 412, 413, 5, 23, 0, 0, 413, 647, 1, 0, 0, 0, 414, 416,
		3, 22, 11, 0, 415, 417, 5, 2, 0, 0, 416, 415, 1, 0, 0, 0, 417, 418, 1,
		0, 0, 0, 418, 416, 1, 0, 0, 0, 418, 419, 1, 0, 0, 0, 419, 420, 1, 0, 0,
		0, 420, 422, 5, 15, 0, 0, 421, 423, 5, 2, 0, 0, 422, 421, 1, 0, 0, 0, 423,
		424, 1, 0, 0, 0, 424, 422, 1, 0, 0, 0, 424, 425, 1, 0, 0, 0, 425, 426,
		1, 0, 0, 0, 426, 428, 5, 17, 0, 0, 427, 429, 5, 2, 0, 0, 428, 427, 1, 0,
		0, 0, 429, 430, 1, 0, 0, 0, 430, 428, 1, 0, 0, 0, 430, 431, 1, 0, 0, 0,
		431, 432, 1, 0, 0, 0, 432, 434, 5, 7, 0, 0, 433, 435, 5, 2, 0, 0, 434,
		433, 1, 0, 0, 0, 435, 436, 1, 0, 0, 0, 436, 434, 1, 0, 0, 0, 436, 437,
		1, 0, 0, 0, 437, 438, 1, 0, 0, 0, 438, 439, 5, 17, 0, 0, 439, 647, 1, 0,
		0, 0, 440, 444, 3, 22, 11, 0, 441, 443, 5, 2, 0, 0, 442, 441, 1, 0, 0,
		0, 443, 446, 1, 0, 0, 0, 444, 442, 1, 0, 0, 0, 444, 445, 1, 0, 0, 0, 445,
		447, 1, 0, 0, 0, 446, 444, 1, 0, 0, 0, 447, 451, 5, 9, 0, 0, 448, 450,
		5, 2, 0, 0, 449, 448, 1, 0, 0, 0, 450, 453, 1, 0, 0, 0, 451, 449, 1, 0,
		0, 0, 451, 452, 1, 0, 0, 0, 452, 454, 1, 0, 0, 0, 453, 451, 1, 0, 0, 0,
		454, 455, 5, 22, 0, 0, 455, 647, 1, 0, 0, 0, 456, 460, 3, 22, 11, 0, 457,
		459, 5, 2, 0, 0, 458, 457, 1, 0, 0, 0, 459, 462, 1, 0, 0, 0, 460, 458,
		1, 0, 0, 0, 460, 461, 1, 0, 0, 0, 461, 463, 1, 0, 0, 0, 462, 460, 1, 0,
		0, 0, 463, 467, 5, 9, 0, 0, 464, 466, 5, 2, 0, 0, 465, 464, 1, 0, 0, 0,
		466, 469, 1, 0, 0, 0, 467, 465, 1, 0, 0, 0, 467, 468, 1, 0, 0, 0, 468,
		470, 1, 0, 0, 0, 469, 467, 1, 0, 0, 0, 470, 471, 5, 23, 0, 0, 471, 647,
		1, 0, 0, 0, 472, 476, 3, 22, 11, 0, 473, 475, 5, 2, 0, 0, 474, 473, 1,
		0, 0, 0, 475, 478, 1, 0, 0, 0, 476, 474, 1, 0, 0, 0, 476, 477, 1, 0, 0,
		0, 477, 479, 1, 0, 0, 0, 478, 476, 1, 0, 0, 0, 479, 483, 5, 9, 0, 0, 480,
		482, 5, 2, 0, 0, 481, 480, 1, 0, 0, 0, 482, 485, 1, 0, 0, 0, 483, 481,
		1, 0, 0, 0, 483, 484, 1, 0, 0, 0, 484, 486, 1, 0, 0, 0, 485, 483, 1, 0,
		0, 0, 486, 487, 5, 17, 0, 0, 487, 647, 1, 0, 0, 0, 488, 492, 3, 22, 11,
		0, 489, 491, 5, 2, 0, 0, 490, 489, 1, 0, 0, 0, 491, 494, 1, 0, 0, 0, 492,
		490, 1, 0, 0, 0, 492, 493, 1, 0, 0, 0, 493, 495, 1, 0, 0, 0, 494, 492,
		1, 0, 0, 0, 495, 499, 5, 10, 0, 0, 496, 498, 5, 2, 0, 0, 497, 496, 1, 0,
		0, 0, 498, 501, 1, 0, 0, 0, 499, 497, 1, 0, 0, 0, 499, 500, 1, 0, 0, 0,
		500, 502, 1, 0, 0, 0, 501, 499, 1, 0, 0, 0, 502, 503, 5, 22, 0, 0, 503,
		647, 1, 0, 0, 0, 504, 508, 3, 22, 11, 0, 505, 507, 5, 2, 0, 0, 506, 505,
		1, 0, 0, 0, 507, 510, 1, 0, 0, 0, 508, 506, 1, 0, 0, 0, 508, 509, 1, 0,
		0, 0, 509, 511, 1, 0, 0, 0, 510, 508, 1, 0, 0, 0, 511, 515, 5, 10, 0, 0,
		512, 514, 5, 2, 0, 0, 513, 512, 1, 0, 0, 0, 514, 517, 1, 0, 0, 0, 515,
		513, 1, 0, 0, 0, 515, 516, 1, 0, 0, 0, 516, 518, 1, 0, 0, 0, 517, 515,
		1, 0, 0, 0, 518, 519, 5, 23, 0, 0, 519, 647, 1, 0, 0, 0, 520, 524, 3, 22,
		11, 0, 521, 523, 5, 2, 0, 0, 522, 521, 1, 0, 0, 0, 523, 526, 1, 0, 0, 0,
		524, 522, 1, 0, 0, 0, 524, 525, 1, 0, 0, 0, 525, 527, 1, 0, 0, 0, 526,
		524, 1, 0, 0, 0, 527, 531, 5, 10, 0, 0, 528, 530, 5, 2, 0, 0, 529, 528,
		1, 0, 0, 0, 530, 533, 1, 0, 0, 0, 531, 529, 1, 0, 0, 0, 531, 532, 1, 0,
		0, 0, 532, 534, 1, 0, 0, 0, 533, 531, 1, 0, 0, 0, 534, 535, 5, 17, 0, 0,
		535, 647, 1, 0, 0, 0, 536, 540, 3, 22, 11, 0, 537, 539, 5, 2, 0, 0, 538,
		537, 1, 0, 0, 0, 539, 542, 1, 0, 0, 0, 540, 538, 1, 0, 0, 0, 540, 541,
		1, 0, 0, 0, 541, 543, 1, 0, 0, 0, 542, 540, 1, 0, 0, 0, 543, 547, 5, 11,
		0, 0, 544, 546, 5, 2, 0, 0, 545, 544, 1, 0, 0, 0, 546, 549, 1, 0, 0, 0,
		547, 545, 1, 0, 0, 0, 547, 548, 1, 0, 0, 0, 548, 550, 1, 0, 0, 0, 549,
		547, 1, 0, 0, 0, 550, 551, 5, 22, 0, 0, 551, 647, 1, 0, 0, 0, 552, 556,
		3, 22, 11, 0, 553, 555, 5, 2, 0, 0, 554, 553, 1, 0, 0, 0, 555, 558, 1,
		0, 0, 0, 556, 554, 1, 0, 0, 0, 556, 557, 1, 0, 0, 0, 557, 559, 1, 0, 0,
		0, 558, 556, 1, 0, 0, 0, 559, 563, 5, 11, 0, 0, 560, 562, 5, 2, 0, 0, 561,
		560, 1, 0, 0, 0, 562, 565, 1, 0, 0, 0, 563, 561, 1, 0, 0, 0, 563, 564,
		1, 0, 0, 0, 564, 566, 1, 0, 0, 0, 565, 563, 1, 0, 0, 0, 566, 567, 5, 23,
		0, 0, 567, 647, 1, 0, 0, 0, 568, 572, 3, 22, 11, 0, 569, 571, 5, 2, 0,
		0, 570, 569, 1, 0, 0, 0, 571, 574, 1, 0, 0, 0, 572, 570, 1, 0, 0, 0, 572,
		573, 1, 0, 0, 0, 573, 575, 1, 0, 0, 0, 574, 572, 1, 0, 0, 0, 575, 579,
		5, 11, 0, 0, 576, 578, 5, 2, 0, 0, 577, 576, 1, 0, 0, 0, 578, 581, 1, 0,
		0, 0, 579, 577, 1, 0, 0, 0, 579, 580, 1, 0, 0, 0, 580, 582, 1, 0, 0, 0,
		581, 579, 1, 0, 0, 0, 582, 583, 5, 17, 0, 0, 583, 647, 1, 0, 0, 0, 584,
		588, 3, 22, 11, 0, 585, 587, 5, 2, 0, 0, 586, 585, 1, 0, 0, 0, 587, 590,
		1, 0, 0, 0, 588, 586, 1, 0, 0, 0, 588, 589, 1, 0, 0, 0, 589, 591, 1, 0,
		0, 0, 590, 588, 1, 0, 0, 0, 591, 595, 5, 11, 0, 0, 592, 594, 5, 2, 0, 0,
		593, 592, 1, 0, 0, 0, 594, 597, 1, 0, 0, 0, 595, 593, 1, 0, 0, 0, 595,
		596, 1, 0, 0, 0, 596, 598, 1, 0, 0, 0, 597, 595, 1, 0, 0, 0, 598, 599,
		5, 16, 0, 0, 599, 647, 1, 0, 0, 0, 600, 604, 3, 22, 11, 0, 601, 603, 5,
		2, 0, 0, 602, 601, 1, 0, 0, 0, 603, 606, 1, 0, 0, 0, 604, 602, 1, 0, 0,
		0, 604, 605, 1, 0, 0, 0, 605, 607, 1, 0, 0, 0, 606, 604, 1, 0, 0, 0, 607,
		611, 5, 11, 0, 0, 608, 610, 5, 2, 0, 0, 609, 608, 1, 0, 0, 0, 610, 613,
		1, 0, 0, 0, 611, 609, 1, 0, 0, 0, 611, 612, 1, 0, 0, 0, 612, 614, 1, 0,
		0, 0, 613, 611, 1, 0, 0, 0, 614, 615, 5, 24, 0, 0, 615, 647, 1, 0, 0, 0,
		616, 620, 3, 22, 11, 0, 617, 619, 5, 2, 0, 0, 618, 617, 1, 0, 0, 0, 619,
		622, 1, 0, 0, 0, 620, 618, 1, 0, 0, 0, 620, 621, 1, 0, 0, 0, 621, 623,
		1, 0, 0, 0, 622, 620, 1, 0, 0, 0, 623, 625, 5, 12, 0, 0, 624, 626, 5, 2,
		0, 0, 625, 624, 1, 0, 0, 0, 626, 627, 1, 0, 0, 0, 627, 625, 1, 0, 0, 0,
		627, 628, 1, 0, 0, 0, 628, 629, 1, 0, 0, 0, 629, 630, 7, 2, 0, 0, 630,
		647, 1, 0, 0, 0, 631, 635, 3, 22, 11, 0, 632, 634, 5, 2, 0, 0, 633, 632,
		1, 0, 0, 0, 634, 637, 1, 0, 0, 0, 635, 633, 1, 0, 0, 0, 635, 636, 1, 0,
		0, 0, 636, 638, 1, 0, 0, 0, 637, 635, 1, 0, 0, 0, 638, 640, 5, 13, 0, 0,
		639, 641, 5, 2, 0, 0, 640, 639, 1, 0, 0, 0, 641, 642, 1, 0, 0, 0, 642,
		640, 1, 0, 0, 0, 642, 643, 1, 0, 0, 0, 643, 644, 1, 0, 0, 0, 644, 645,
		5, 22, 0, 0, 645, 647, 1, 0, 0, 0, 646, 346, 1, 0, 0, 0, 646, 360, 1, 0,
		0, 0, 646, 374, 1, 0, 0, 0, 646, 388, 1, 0, 0, 0, 646, 414, 1, 0, 0, 0,
		646, 440, 1, 0, 0, 0, 646, 456, 1, 0, 0, 0, 646, 472, 1, 0, 0, 0, 646,
		488, 1, 0, 0, 0, 646, 504, 1, 0, 0, 0, 646, 520, 1, 0, 0, 0, 646, 536,
		1, 0, 0, 0, 646, 552, 1, 0, 0, 0, 646, 568, 1, 0, 0, 0, 646, 584, 1, 0,
		0, 0, 646, 600, 1, 0, 0, 0, 646, 616, 1, 0, 0, 0, 646, 631, 1, 0, 0, 0,
		647, 21, 1, 0, 0, 0, 648, 651, 5, 35, 0, 0, 649, 651, 3, 24, 12, 0, 650,
		648, 1, 0, 0, 0, 650, 649, 1, 0, 0, 0, 651, 23, 1, 0, 0, 0, 652, 653, 5,
		18, 0, 0, 653, 657, 5, 3, 0, 0, 654, 656, 5, 2, 0, 0, 655, 654, 1, 0, 0,
		0, 656, 659, 1, 0, 0, 0, 657, 655, 1, 0, 0, 0, 657, 658, 1, 0, 0, 0, 658,
		660, 1, 0, 0, 0, 659, 657, 1, 0, 0, 0, 660, 664, 5, 35, 0, 0, 661, 663,
		5, 2, 0, 0, 662, 661, 1, 0, 0, 0, 663, 666, 1, 0, 0, 0, 664, 662, 1, 0,
		0, 0, 664, 665, 1, 0, 0, 0, 665, 667, 1, 0, 0, 0, 666, 664, 1, 0, 0, 0,
		667, 702, 5, 4, 0, 0, 668, 669, 5, 19, 0, 0, 669, 673, 5, 3, 0, 0, 670,
		672, 5, 2, 0, 0, 671, 670, 1, 0, 0, 0, 672, 675, 1, 0, 0, 0, 673, 671,
		1, 0, 0, 0, 673, 674, 1, 0, 0, 0, 674, 676, 1, 0, 0, 0, 675, 673, 1, 0,
		0, 0, 676, 680, 5, 35, 0, 0, 677, 679, 5, 2, 0, 0, 678, 677, 1, 0, 0, 0,
		679, 682, 1, 0, 0, 0, 680, 678, 1, 0, 0, 0, 680, 681, 1, 0, 0, 0, 681,
		683, 1, 0, 0, 0, 682, 680, 1, 0, 0, 0, 683, 702, 5, 4, 0, 0, 684, 685,
		5, 20, 0, 0, 685, 689, 5, 3, 0, 0, 686, 688, 5, 2, 0, 0, 687, 686, 1, 0,
		0, 0, 688, 691, 1, 0, 0, 0, 689, 687, 1, 0, 0, 0, 689, 690, 1, 0, 0, 0,
		690, 692, 1, 0, 0, 0, 691, 689, 1, 0, 0, 0, 692, 696, 3, 26, 13, 0, 693,
		695, 5, 2, 0, 0, 694, 693, 1, 0, 0, 0, 695, 698, 1, 0, 0, 0, 696, 694,
		1, 0, 0, 0, 696, 697, 1, 0, 0, 0, 697, 699, 1, 0, 0, 0, 698, 696, 1, 0,
		0, 0, 699, 700, 5, 4, 0, 0, 700, 702, 1, 0, 0, 0, 701, 652, 1, 0, 0, 0,
		701, 668, 1, 0, 0, 0, 701, 684, 1, 0, 0, 0, 702, 25, 1, 0, 0, 0, 703, 706,
		5, 35, 0, 0, 704, 706, 3, 28, 14, 0, 705, 703, 1, 0, 0, 0, 705, 704, 1,
		0, 0, 0, 706, 27, 1, 0, 0, 0, 707, 709, 5, 34, 0, 0, 708, 710, 5, 2, 0,
		0, 709, 708, 1, 0, 0, 0, 710, 711, 1, 0, 0, 0, 711, 709, 1, 0, 0, 0, 711,
		712, 1, 0, 0, 0, 712, 713, 1, 0, 0, 0, 713, 715, 5, 35, 0, 0, 714, 716,
		5, 2, 0, 0, 715, 714, 1, 0, 0, 0, 716, 717, 1, 0, 0, 0, 717, 715, 1, 0,
		0, 0, 717, 718, 1, 0, 0, 0, 718, 719, 1, 0, 0, 0, 719, 721, 5, 33, 0, 0,
		720, 722, 5, 2, 0, 0, 721, 720, 1, 0, 0, 0, 722, 723, 1, 0, 0, 0, 723,
		721, 1, 0, 0, 0, 723, 724, 1, 0, 0, 0, 724, 725, 1, 0, 0, 0, 725, 726,
		3, 8, 4, 0, 726, 29, 1, 0, 0, 0, 106, 34, 41, 48, 54, 60, 69, 76, 83, 89,
		95, 104, 111, 118, 124, 130, 138, 145, 154, 157, 162, 165, 170, 173, 179,
		182, 187, 190, 196, 199, 202, 208, 216, 224, 230, 236, 243, 249, 256, 259,
		267, 274, 285, 292, 302, 305, 311, 317, 322, 328, 334, 339, 341, 343, 350,
		356, 364, 370, 378, 384, 392, 398, 404, 410, 418, 424, 430, 436, 444, 451,
		460, 467, 476, 483, 492, 499, 508, 515, 524, 531, 540, 547, 556, 563, 572,
		579, 588, 595, 604, 611, 620, 627, 635, 642, 646, 650, 657, 664, 673, 680,
		689, 696, 701, 705, 711, 717, 723,
	}
	deserializer := antlr.NewATNDeserializer(nil)
	staticData.atn = deserializer.Deserialize(staticData.serializedATN)
	atn := staticData.atn
	staticData.decisionToDFA = make([]*antlr.DFA, len(atn.DecisionToState))
	decisionToDFA := staticData.decisionToDFA
	for index, state := range atn.DecisionToState {
		decisionToDFA[index] = antlr.NewDFA(state, index)
	}
}

// ZitiQlParserInit initializes any static state used to implement ZitiQlParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewZitiQlParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func ZitiQlParserInit() {
	staticData := &ZitiQlParserStaticData
	staticData.once.Do(zitiqlParserInit)
}

// NewZitiQlParser produces a new parser instance for the optional input antlr.TokenStream.
func NewZitiQlParser(input antlr.TokenStream) *ZitiQlParser {
	ZitiQlParserInit()
	this := new(ZitiQlParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &ZitiQlParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.PredictionContextCache)
	this.RuleNames = staticData.RuleNames
	this.LiteralNames = staticData.LiteralNames
	this.SymbolicNames = staticData.SymbolicNames
	this.GrammarFileName = "ZitiQl.g4"

	return this
}

// ZitiQlParser tokens.
const (
	ZitiQlParserEOF               = antlr.TokenEOF
	ZitiQlParserT__0              = 1
	ZitiQlParserWS                = 2
	ZitiQlParserLPAREN            = 3
	ZitiQlParserRPAREN            = 4
	ZitiQlParserLBRACKET          = 5
	ZitiQlParserRBRACKET          = 6
	ZitiQlParserAND               = 7
	ZitiQlParserOR                = 8
	ZitiQlParserLT                = 9
	ZitiQlParserGT                = 10
	ZitiQlParserEQ                = 11
	ZitiQlParserCONTAINS          = 12
	ZitiQlParserICONTAINS         = 13
	ZitiQlParserIN                = 14
	ZitiQlParserBETWEEN           = 15
	ZitiQlParserBOOL              = 16
	ZitiQlParserDATETIME          = 17
	ZitiQlParserALL_OF            = 18
	ZitiQlParserANY_OF            = 19
	ZitiQlParserCOUNT             = 20
	ZitiQlParserISEMPTY           = 21
	ZitiQlParserSTRING            = 22
	ZitiQlParserNUMBER            = 23
	ZitiQlParserNULL              = 24
	ZitiQlParserNOT               = 25
	ZitiQlParserASC               = 26
	ZitiQlParserDESC              = 27
	ZitiQlParserSORT              = 28
	ZitiQlParserBY                = 29
	ZitiQlParserSKIP_ROWS         = 30
	ZitiQlParserLIMIT_ROWS        = 31
	ZitiQlParserNONE              = 32
	ZitiQlParserWHERE             = 33
	ZitiQlParserFROM              = 34
	ZitiQlParserIDENTIFIER        = 35
	ZitiQlParserRFC3339_DATE_TIME = 36
)

// ZitiQlParser rules.
const (
	ZitiQlParserRULE_stringArray   = 0
	ZitiQlParserRULE_numberArray   = 1
	ZitiQlParserRULE_datetimeArray = 2
	ZitiQlParserRULE_start         = 3
	ZitiQlParserRULE_query         = 4
	ZitiQlParserRULE_skip          = 5
	ZitiQlParserRULE_limit         = 6
	ZitiQlParserRULE_sortBy        = 7
	ZitiQlParserRULE_sortField     = 8
	ZitiQlParserRULE_boolExpr      = 9
	ZitiQlParserRULE_operation     = 10
	ZitiQlParserRULE_binaryLhs     = 11
	ZitiQlParserRULE_setFunction   = 12
	ZitiQlParserRULE_setExpr       = 13
	ZitiQlParserRULE_subQueryExpr  = 14
)

// IStringArrayContext is an interface to support dynamic dispatch.
type IStringArrayContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	LBRACKET() antlr.TerminalNode
	AllSTRING() []antlr.TerminalNode
	STRING(i int) antlr.TerminalNode
	RBRACKET() antlr.TerminalNode
	AllWS() []antlr.TerminalNode
	WS(i int) antlr.TerminalNode

	// IsStringArrayContext differentiates from other interfaces.
	IsStringArrayContext()
}

type StringArrayContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStringArrayContext() *StringArrayContext {
	var p = new(StringArrayContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_stringArray
	return p
}

func InitEmptyStringArrayContext(p *StringArrayContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_stringArray
}

func (*StringArrayContext) IsStringArrayContext() {}

func NewStringArrayContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StringArrayContext {
	var p = new(StringArrayContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_stringArray

	return p
}

func (s *StringArrayContext) GetParser() antlr.Parser { return s.parser }

func (s *StringArrayContext) LBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLBRACKET, 0)
}

func (s *StringArrayContext) AllSTRING() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserSTRING)
}

func (s *StringArrayContext) STRING(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSTRING, i)
}

func (s *StringArrayContext) RBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRBRACKET, 0)
}

func (s *StringArrayContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *StringArrayContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *StringArrayContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringArrayContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StringArrayContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterStringArray(s)
	}
}

func (s *StringArrayContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitStringArray(s)
	}
}

func (p *ZitiQlParser) StringArray() (localctx IStringArrayContext) {
	localctx = NewStringArrayContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, ZitiQlParserRULE_stringArray)
	var _la int

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(30)
		p.Match(ZitiQlParserLBRACKET)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(34)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(31)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(36)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(37)
		p.Match(ZitiQlParserSTRING)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(54)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 3, p.GetParserRuleContext())
	if p.HasError() {
		goto errorExit
	}
	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(41)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(38)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(43)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(44)
				p.Match(ZitiQlParserT__0)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}
			p.SetState(48)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(45)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(50)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(51)
				p.Match(ZitiQlParserSTRING)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

		}
		p.SetState(56)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 3, p.GetParserRuleContext())
		if p.HasError() {
			goto errorExit
		}
	}
	p.SetState(60)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(57)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(62)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(63)
		p.Match(ZitiQlParserRBRACKET)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// INumberArrayContext is an interface to support dynamic dispatch.
type INumberArrayContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	LBRACKET() antlr.TerminalNode
	AllNUMBER() []antlr.TerminalNode
	NUMBER(i int) antlr.TerminalNode
	RBRACKET() antlr.TerminalNode
	AllWS() []antlr.TerminalNode
	WS(i int) antlr.TerminalNode

	// IsNumberArrayContext differentiates from other interfaces.
	IsNumberArrayContext()
}

type NumberArrayContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNumberArrayContext() *NumberArrayContext {
	var p = new(NumberArrayContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_numberArray
	return p
}

func InitEmptyNumberArrayContext(p *NumberArrayContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_numberArray
}

func (*NumberArrayContext) IsNumberArrayContext() {}

func NewNumberArrayContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NumberArrayContext {
	var p = new(NumberArrayContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_numberArray

	return p
}

func (s *NumberArrayContext) GetParser() antlr.Parser { return s.parser }

func (s *NumberArrayContext) LBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLBRACKET, 0)
}

func (s *NumberArrayContext) AllNUMBER() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserNUMBER)
}

func (s *NumberArrayContext) NUMBER(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, i)
}

func (s *NumberArrayContext) RBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRBRACKET, 0)
}

func (s *NumberArrayContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *NumberArrayContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *NumberArrayContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NumberArrayContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NumberArrayContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterNumberArray(s)
	}
}

func (s *NumberArrayContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitNumberArray(s)
	}
}

func (p *ZitiQlParser) NumberArray() (localctx INumberArrayContext) {
	localctx = NewNumberArrayContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, ZitiQlParserRULE_numberArray)
	var _la int

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(65)
		p.Match(ZitiQlParserLBRACKET)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(69)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(66)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(71)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(72)
		p.Match(ZitiQlParserNUMBER)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(89)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 8, p.GetParserRuleContext())
	if p.HasError() {
		goto errorExit
	}
	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(76)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(73)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(78)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(79)
				p.Match(ZitiQlParserT__0)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}
			p.SetState(83)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(80)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(85)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(86)
				p.Match(ZitiQlParserNUMBER)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

		}
		p.SetState(91)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 8, p.GetParserRuleContext())
		if p.HasError() {
			goto errorExit
		}
	}
	p.SetState(95)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(92)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(97)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(98)
		p.Match(ZitiQlParserRBRACKET)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IDatetimeArrayContext is an interface to support dynamic dispatch.
type IDatetimeArrayContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	LBRACKET() antlr.TerminalNode
	AllDATETIME() []antlr.TerminalNode
	DATETIME(i int) antlr.TerminalNode
	RBRACKET() antlr.TerminalNode
	AllWS() []antlr.TerminalNode
	WS(i int) antlr.TerminalNode

	// IsDatetimeArrayContext differentiates from other interfaces.
	IsDatetimeArrayContext()
}

type DatetimeArrayContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDatetimeArrayContext() *DatetimeArrayContext {
	var p = new(DatetimeArrayContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_datetimeArray
	return p
}

func InitEmptyDatetimeArrayContext(p *DatetimeArrayContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_datetimeArray
}

func (*DatetimeArrayContext) IsDatetimeArrayContext() {}

func NewDatetimeArrayContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DatetimeArrayContext {
	var p = new(DatetimeArrayContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_datetimeArray

	return p
}

func (s *DatetimeArrayContext) GetParser() antlr.Parser { return s.parser }

func (s *DatetimeArrayContext) LBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLBRACKET, 0)
}

func (s *DatetimeArrayContext) AllDATETIME() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserDATETIME)
}

func (s *DatetimeArrayContext) DATETIME(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, i)
}

func (s *DatetimeArrayContext) RBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRBRACKET, 0)
}

func (s *DatetimeArrayContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *DatetimeArrayContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *DatetimeArrayContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DatetimeArrayContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DatetimeArrayContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterDatetimeArray(s)
	}
}

func (s *DatetimeArrayContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitDatetimeArray(s)
	}
}

func (p *ZitiQlParser) DatetimeArray() (localctx IDatetimeArrayContext) {
	localctx = NewDatetimeArrayContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, ZitiQlParserRULE_datetimeArray)
	var _la int

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(100)
		p.Match(ZitiQlParserLBRACKET)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(104)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(101)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(106)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(107)
		p.Match(ZitiQlParserDATETIME)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(124)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 13, p.GetParserRuleContext())
	if p.HasError() {
		goto errorExit
	}
	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(111)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(108)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(113)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(114)
				p.Match(ZitiQlParserT__0)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}
			p.SetState(118)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(115)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(120)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(121)
				p.Match(ZitiQlParserDATETIME)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

		}
		p.SetState(126)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 13, p.GetParserRuleContext())
		if p.HasError() {
			goto errorExit
		}
	}
	p.SetState(130)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(127)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(132)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(133)
		p.Match(ZitiQlParserRBRACKET)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IStartContext is an interface to support dynamic dispatch.
type IStartContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsStartContext differentiates from other interfaces.
	IsStartContext()
}

type StartContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStartContext() *StartContext {
	var p = new(StartContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_start
	return p
}

func InitEmptyStartContext(p *StartContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_start
}

func (*StartContext) IsStartContext() {}

func NewStartContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StartContext {
	var p = new(StartContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_start

	return p
}

func (s *StartContext) GetParser() antlr.Parser { return s.parser }

func (s *StartContext) CopyAll(ctx *StartContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *StartContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StartContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type EndContext struct {
	StartContext
}

func NewEndContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *EndContext {
	var p = new(EndContext)

	InitEmptyStartContext(&p.StartContext)
	p.parser = parser
	p.CopyAll(ctx.(*StartContext))

	return p
}

func (s *EndContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *EndContext) Query() IQueryContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IQueryContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IQueryContext)
}

func (s *EndContext) EOF() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEOF, 0)
}

func (s *EndContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *EndContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *EndContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterEnd(s)
	}
}

func (s *EndContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitEnd(s)
	}
}

func (p *ZitiQlParser) Start_() (localctx IStartContext) {
	localctx = NewStartContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, ZitiQlParserRULE_start)
	var _la int

	localctx = NewEndContext(p, localctx)
	p.EnterOuterAlt(localctx, 1)
	p.SetState(138)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(135)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(140)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(141)
		p.Query()
	}
	p.SetState(145)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(142)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(147)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(148)
		p.Match(ZitiQlParserEOF)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IQueryContext is an interface to support dynamic dispatch.
type IQueryContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsQueryContext differentiates from other interfaces.
	IsQueryContext()
}

type QueryContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyQueryContext() *QueryContext {
	var p = new(QueryContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_query
	return p
}

func InitEmptyQueryContext(p *QueryContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_query
}

func (*QueryContext) IsQueryContext() {}

func NewQueryContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *QueryContext {
	var p = new(QueryContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_query

	return p
}

func (s *QueryContext) GetParser() antlr.Parser { return s.parser }

func (s *QueryContext) CopyAll(ctx *QueryContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *QueryContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *QueryContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type QueryStmtContext struct {
	QueryContext
}

func NewQueryStmtContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *QueryStmtContext {
	var p = new(QueryStmtContext)

	InitEmptyQueryContext(&p.QueryContext)
	p.parser = parser
	p.CopyAll(ctx.(*QueryContext))

	return p
}

func (s *QueryStmtContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *QueryStmtContext) BoolExpr() IBoolExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBoolExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBoolExprContext)
}

func (s *QueryStmtContext) SortBy() ISortByContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISortByContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISortByContext)
}

func (s *QueryStmtContext) Skip() ISkipContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISkipContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISkipContext)
}

func (s *QueryStmtContext) Limit() ILimitContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILimitContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILimitContext)
}

func (s *QueryStmtContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *QueryStmtContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *QueryStmtContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterQueryStmt(s)
	}
}

func (s *QueryStmtContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitQueryStmt(s)
	}
}

func (p *ZitiQlParser) Query() (localctx IQueryContext) {
	localctx = NewQueryContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, ZitiQlParserRULE_query)
	var _la int

	p.SetState(202)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case ZitiQlParserLPAREN, ZitiQlParserBOOL, ZitiQlParserALL_OF, ZitiQlParserANY_OF, ZitiQlParserCOUNT, ZitiQlParserISEMPTY, ZitiQlParserNOT, ZitiQlParserIDENTIFIER:
		localctx = NewQueryStmtContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(150)
			p.boolExpr(0)
		}
		p.SetState(157)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 18, p.GetParserRuleContext()) == 1 {
			p.SetState(152)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for ok := true; ok; ok = _la == ZitiQlParserWS {
				{
					p.SetState(151)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(154)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(156)
				p.SortBy()
			}

		} else if p.HasError() { // JIM
			goto errorExit
		}
		p.SetState(165)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 20, p.GetParserRuleContext()) == 1 {
			p.SetState(160)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for ok := true; ok; ok = _la == ZitiQlParserWS {
				{
					p.SetState(159)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(162)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(164)
				p.Skip()
			}

		} else if p.HasError() { // JIM
			goto errorExit
		}
		p.SetState(173)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 22, p.GetParserRuleContext()) == 1 {
			p.SetState(168)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for ok := true; ok; ok = _la == ZitiQlParserWS {
				{
					p.SetState(167)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(170)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(172)
				p.Limit()
			}

		} else if p.HasError() { // JIM
			goto errorExit
		}

	case ZitiQlParserSORT:
		localctx = NewQueryStmtContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(175)
			p.SortBy()
		}
		p.SetState(182)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 24, p.GetParserRuleContext()) == 1 {
			p.SetState(177)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for ok := true; ok; ok = _la == ZitiQlParserWS {
				{
					p.SetState(176)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(179)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(181)
				p.Skip()
			}

		} else if p.HasError() { // JIM
			goto errorExit
		}
		p.SetState(190)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 26, p.GetParserRuleContext()) == 1 {
			p.SetState(185)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for ok := true; ok; ok = _la == ZitiQlParserWS {
				{
					p.SetState(184)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(187)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(189)
				p.Limit()
			}

		} else if p.HasError() { // JIM
			goto errorExit
		}

	case ZitiQlParserSKIP_ROWS:
		localctx = NewQueryStmtContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(192)
			p.Skip()
		}
		p.SetState(199)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 28, p.GetParserRuleContext()) == 1 {
			p.SetState(194)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for ok := true; ok; ok = _la == ZitiQlParserWS {
				{
					p.SetState(193)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(196)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(198)
				p.Limit()
			}

		} else if p.HasError() { // JIM
			goto errorExit
		}

	case ZitiQlParserLIMIT_ROWS:
		localctx = NewQueryStmtContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(201)
			p.Limit()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISkipContext is an interface to support dynamic dispatch.
type ISkipContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsSkipContext differentiates from other interfaces.
	IsSkipContext()
}

type SkipContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySkipContext() *SkipContext {
	var p = new(SkipContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_skip
	return p
}

func InitEmptySkipContext(p *SkipContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_skip
}

func (*SkipContext) IsSkipContext() {}

func NewSkipContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SkipContext {
	var p = new(SkipContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_skip

	return p
}

func (s *SkipContext) GetParser() antlr.Parser { return s.parser }

func (s *SkipContext) CopyAll(ctx *SkipContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *SkipContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SkipContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type SkipExprContext struct {
	SkipContext
}

func NewSkipExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *SkipExprContext {
	var p = new(SkipExprContext)

	InitEmptySkipContext(&p.SkipContext)
	p.parser = parser
	p.CopyAll(ctx.(*SkipContext))

	return p
}

func (s *SkipExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SkipExprContext) SKIP_ROWS() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSKIP_ROWS, 0)
}

func (s *SkipExprContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *SkipExprContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *SkipExprContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *SkipExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterSkipExpr(s)
	}
}

func (s *SkipExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitSkipExpr(s)
	}
}

func (p *ZitiQlParser) Skip() (localctx ISkipContext) {
	localctx = NewSkipContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, ZitiQlParserRULE_skip)
	var _la int

	localctx = NewSkipExprContext(p, localctx)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(204)
		p.Match(ZitiQlParserSKIP_ROWS)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(206)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == ZitiQlParserWS {
		{
			p.SetState(205)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(208)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(210)
		p.Match(ZitiQlParserNUMBER)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ILimitContext is an interface to support dynamic dispatch.
type ILimitContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsLimitContext differentiates from other interfaces.
	IsLimitContext()
}

type LimitContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyLimitContext() *LimitContext {
	var p = new(LimitContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_limit
	return p
}

func InitEmptyLimitContext(p *LimitContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_limit
}

func (*LimitContext) IsLimitContext() {}

func NewLimitContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *LimitContext {
	var p = new(LimitContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_limit

	return p
}

func (s *LimitContext) GetParser() antlr.Parser { return s.parser }

func (s *LimitContext) CopyAll(ctx *LimitContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *LimitContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LimitContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type LimitExprContext struct {
	LimitContext
}

func NewLimitExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *LimitExprContext {
	var p = new(LimitExprContext)

	InitEmptyLimitContext(&p.LimitContext)
	p.parser = parser
	p.CopyAll(ctx.(*LimitContext))

	return p
}

func (s *LimitExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LimitExprContext) LIMIT_ROWS() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLIMIT_ROWS, 0)
}

func (s *LimitExprContext) NONE() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNONE, 0)
}

func (s *LimitExprContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *LimitExprContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *LimitExprContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *LimitExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterLimitExpr(s)
	}
}

func (s *LimitExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitLimitExpr(s)
	}
}

func (p *ZitiQlParser) Limit() (localctx ILimitContext) {
	localctx = NewLimitContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, ZitiQlParserRULE_limit)
	var _la int

	localctx = NewLimitExprContext(p, localctx)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(212)
		p.Match(ZitiQlParserLIMIT_ROWS)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(214)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == ZitiQlParserWS {
		{
			p.SetState(213)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(216)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(218)
		_la = p.GetTokenStream().LA(1)

		if !(_la == ZitiQlParserNUMBER || _la == ZitiQlParserNONE) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISortByContext is an interface to support dynamic dispatch.
type ISortByContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsSortByContext differentiates from other interfaces.
	IsSortByContext()
}

type SortByContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySortByContext() *SortByContext {
	var p = new(SortByContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_sortBy
	return p
}

func InitEmptySortByContext(p *SortByContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_sortBy
}

func (*SortByContext) IsSortByContext() {}

func NewSortByContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SortByContext {
	var p = new(SortByContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_sortBy

	return p
}

func (s *SortByContext) GetParser() antlr.Parser { return s.parser }

func (s *SortByContext) CopyAll(ctx *SortByContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *SortByContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SortByContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type SortByExprContext struct {
	SortByContext
}

func NewSortByExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *SortByExprContext {
	var p = new(SortByExprContext)

	InitEmptySortByContext(&p.SortByContext)
	p.parser = parser
	p.CopyAll(ctx.(*SortByContext))

	return p
}

func (s *SortByExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SortByExprContext) SORT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSORT, 0)
}

func (s *SortByExprContext) BY() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserBY, 0)
}

func (s *SortByExprContext) AllSortField() []ISortFieldContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISortFieldContext); ok {
			len++
		}
	}

	tst := make([]ISortFieldContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISortFieldContext); ok {
			tst[i] = t.(ISortFieldContext)
			i++
		}
	}

	return tst
}

func (s *SortByExprContext) SortField(i int) ISortFieldContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISortFieldContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISortFieldContext)
}

func (s *SortByExprContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *SortByExprContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *SortByExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterSortByExpr(s)
	}
}

func (s *SortByExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitSortByExpr(s)
	}
}

func (p *ZitiQlParser) SortBy() (localctx ISortByContext) {
	localctx = NewSortByContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, ZitiQlParserRULE_sortBy)
	var _la int

	var _alt int

	localctx = NewSortByExprContext(p, localctx)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(220)
		p.Match(ZitiQlParserSORT)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(222)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == ZitiQlParserWS {
		{
			p.SetState(221)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(224)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(226)
		p.Match(ZitiQlParserBY)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(228)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == ZitiQlParserWS {
		{
			p.SetState(227)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(230)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(232)
		p.SortField()
	}
	p.SetState(249)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 36, p.GetParserRuleContext())
	if p.HasError() {
		goto errorExit
	}
	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(236)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(233)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(238)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(239)
				p.Match(ZitiQlParserT__0)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}
			p.SetState(243)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(240)
					p.Match(ZitiQlParserWS)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}

				p.SetState(245)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(246)
				p.SortField()
			}

		}
		p.SetState(251)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 36, p.GetParserRuleContext())
		if p.HasError() {
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISortFieldContext is an interface to support dynamic dispatch.
type ISortFieldContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsSortFieldContext differentiates from other interfaces.
	IsSortFieldContext()
}

type SortFieldContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySortFieldContext() *SortFieldContext {
	var p = new(SortFieldContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_sortField
	return p
}

func InitEmptySortFieldContext(p *SortFieldContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_sortField
}

func (*SortFieldContext) IsSortFieldContext() {}

func NewSortFieldContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SortFieldContext {
	var p = new(SortFieldContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_sortField

	return p
}

func (s *SortFieldContext) GetParser() antlr.Parser { return s.parser }

func (s *SortFieldContext) CopyAll(ctx *SortFieldContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *SortFieldContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SortFieldContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type SortFieldExprContext struct {
	SortFieldContext
}

func NewSortFieldExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *SortFieldExprContext {
	var p = new(SortFieldExprContext)

	InitEmptySortFieldContext(&p.SortFieldContext)
	p.parser = parser
	p.CopyAll(ctx.(*SortFieldContext))

	return p
}

func (s *SortFieldExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SortFieldExprContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *SortFieldExprContext) ASC() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserASC, 0)
}

func (s *SortFieldExprContext) DESC() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDESC, 0)
}

func (s *SortFieldExprContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *SortFieldExprContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *SortFieldExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterSortFieldExpr(s)
	}
}

func (s *SortFieldExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitSortFieldExpr(s)
	}
}

func (p *ZitiQlParser) SortField() (localctx ISortFieldContext) {
	localctx = NewSortFieldContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, ZitiQlParserRULE_sortField)
	var _la int

	localctx = NewSortFieldExprContext(p, localctx)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(252)
		p.Match(ZitiQlParserIDENTIFIER)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(259)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 38, p.GetParserRuleContext()) == 1 {
		p.SetState(254)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(253)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(256)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(258)
			_la = p.GetTokenStream().LA(1)

			if !(_la == ZitiQlParserASC || _la == ZitiQlParserDESC) {
				p.GetErrorHandler().RecoverInline(p)
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IBoolExprContext is an interface to support dynamic dispatch.
type IBoolExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsBoolExprContext differentiates from other interfaces.
	IsBoolExprContext()
}

type BoolExprContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBoolExprContext() *BoolExprContext {
	var p = new(BoolExprContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_boolExpr
	return p
}

func InitEmptyBoolExprContext(p *BoolExprContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_boolExpr
}

func (*BoolExprContext) IsBoolExprContext() {}

func NewBoolExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BoolExprContext {
	var p = new(BoolExprContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_boolExpr

	return p
}

func (s *BoolExprContext) GetParser() antlr.Parser { return s.parser }

func (s *BoolExprContext) CopyAll(ctx *BoolExprContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *BoolExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BoolExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type AndExprContext struct {
	BoolExprContext
}

func NewAndExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *AndExprContext {
	var p = new(AndExprContext)

	InitEmptyBoolExprContext(&p.BoolExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*BoolExprContext))

	return p
}

func (s *AndExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AndExprContext) AllBoolExpr() []IBoolExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IBoolExprContext); ok {
			len++
		}
	}

	tst := make([]IBoolExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IBoolExprContext); ok {
			tst[i] = t.(IBoolExprContext)
			i++
		}
	}

	return tst
}

func (s *AndExprContext) BoolExpr(i int) IBoolExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBoolExprContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBoolExprContext)
}

func (s *AndExprContext) AllAND() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserAND)
}

func (s *AndExprContext) AND(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserAND, i)
}

func (s *AndExprContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *AndExprContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *AndExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterAndExpr(s)
	}
}

func (s *AndExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitAndExpr(s)
	}
}

type GroupContext struct {
	BoolExprContext
}

func NewGroupContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *GroupContext {
	var p = new(GroupContext)

	InitEmptyBoolExprContext(&p.BoolExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*BoolExprContext))

	return p
}

func (s *GroupContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *GroupContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLPAREN, 0)
}

func (s *GroupContext) BoolExpr() IBoolExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBoolExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBoolExprContext)
}

func (s *GroupContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRPAREN, 0)
}

func (s *GroupContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *GroupContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *GroupContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterGroup(s)
	}
}

func (s *GroupContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitGroup(s)
	}
}

type BoolConstContext struct {
	BoolExprContext
}

func NewBoolConstContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BoolConstContext {
	var p = new(BoolConstContext)

	InitEmptyBoolExprContext(&p.BoolExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*BoolExprContext))

	return p
}

func (s *BoolConstContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BoolConstContext) BOOL() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserBOOL, 0)
}

func (s *BoolConstContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBoolConst(s)
	}
}

func (s *BoolConstContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBoolConst(s)
	}
}

type IsEmptyFunctionContext struct {
	BoolExprContext
}

func NewIsEmptyFunctionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *IsEmptyFunctionContext {
	var p = new(IsEmptyFunctionContext)

	InitEmptyBoolExprContext(&p.BoolExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*BoolExprContext))

	return p
}

func (s *IsEmptyFunctionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *IsEmptyFunctionContext) ISEMPTY() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserISEMPTY, 0)
}

func (s *IsEmptyFunctionContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLPAREN, 0)
}

func (s *IsEmptyFunctionContext) SetExpr() ISetExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISetExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISetExprContext)
}

func (s *IsEmptyFunctionContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRPAREN, 0)
}

func (s *IsEmptyFunctionContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *IsEmptyFunctionContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *IsEmptyFunctionContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterIsEmptyFunction(s)
	}
}

func (s *IsEmptyFunctionContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitIsEmptyFunction(s)
	}
}

type NotExprContext struct {
	BoolExprContext
}

func NewNotExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NotExprContext {
	var p = new(NotExprContext)

	InitEmptyBoolExprContext(&p.BoolExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*BoolExprContext))

	return p
}

func (s *NotExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NotExprContext) NOT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNOT, 0)
}

func (s *NotExprContext) BoolExpr() IBoolExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBoolExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBoolExprContext)
}

func (s *NotExprContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *NotExprContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *NotExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterNotExpr(s)
	}
}

func (s *NotExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitNotExpr(s)
	}
}

type OperationOpContext struct {
	BoolExprContext
}

func NewOperationOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *OperationOpContext {
	var p = new(OperationOpContext)

	InitEmptyBoolExprContext(&p.BoolExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*BoolExprContext))

	return p
}

func (s *OperationOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperationOpContext) Operation() IOperationContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperationContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperationContext)
}

func (s *OperationOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterOperationOp(s)
	}
}

func (s *OperationOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitOperationOp(s)
	}
}

type OrExprContext struct {
	BoolExprContext
}

func NewOrExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *OrExprContext {
	var p = new(OrExprContext)

	InitEmptyBoolExprContext(&p.BoolExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*BoolExprContext))

	return p
}

func (s *OrExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OrExprContext) AllBoolExpr() []IBoolExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IBoolExprContext); ok {
			len++
		}
	}

	tst := make([]IBoolExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IBoolExprContext); ok {
			tst[i] = t.(IBoolExprContext)
			i++
		}
	}

	return tst
}

func (s *OrExprContext) BoolExpr(i int) IBoolExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBoolExprContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBoolExprContext)
}

func (s *OrExprContext) AllOR() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserOR)
}

func (s *OrExprContext) OR(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserOR, i)
}

func (s *OrExprContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *OrExprContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *OrExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterOrExpr(s)
	}
}

func (s *OrExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitOrExpr(s)
	}
}

type BoolSymbolContext struct {
	BoolExprContext
}

func NewBoolSymbolContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BoolSymbolContext {
	var p = new(BoolSymbolContext)

	InitEmptyBoolExprContext(&p.BoolExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*BoolExprContext))

	return p
}

func (s *BoolSymbolContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BoolSymbolContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BoolSymbolContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBoolSymbol(s)
	}
}

func (s *BoolSymbolContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBoolSymbol(s)
	}
}

func (p *ZitiQlParser) BoolExpr() (localctx IBoolExprContext) {
	return p.boolExpr(0)
}

func (p *ZitiQlParser) boolExpr(_p int) (localctx IBoolExprContext) {
	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()

	_parentState := p.GetState()
	localctx = NewBoolExprContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx IBoolExprContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 18
	p.EnterRecursionRule(localctx, 18, ZitiQlParserRULE_boolExpr, _p)
	var _la int

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(305)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 44, p.GetParserRuleContext()) {
	case 1:
		localctx = NewOperationOpContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(262)
			p.Operation()
		}

	case 2:
		localctx = NewGroupContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(263)
			p.Match(ZitiQlParserLPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(267)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(264)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(269)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(270)
			p.boolExpr(0)
		}
		p.SetState(274)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(271)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(276)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(277)
			p.Match(ZitiQlParserRPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 3:
		localctx = NewBoolConstContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(279)
			p.Match(ZitiQlParserBOOL)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 4:
		localctx = NewIsEmptyFunctionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(280)
			p.Match(ZitiQlParserISEMPTY)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(281)
			p.Match(ZitiQlParserLPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(285)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(282)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(287)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(288)
			p.SetExpr()
		}
		p.SetState(292)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(289)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(294)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(295)
			p.Match(ZitiQlParserRPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 5:
		localctx = NewBoolSymbolContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(297)
			p.Match(ZitiQlParserIDENTIFIER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 6:
		localctx = NewNotExprContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(298)
			p.Match(ZitiQlParserNOT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(300)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(299)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(302)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(304)
			p.boolExpr(1)
		}

	case antlr.ATNInvalidAltNumber:
		goto errorExit
	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(343)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 52, p.GetParserRuleContext())
	if p.HasError() {
		goto errorExit
	}
	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(341)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}

			switch p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 51, p.GetParserRuleContext()) {
			case 1:
				localctx = NewAndExprContext(p, NewBoolExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, ZitiQlParserRULE_boolExpr)
				p.SetState(307)

				if !(p.Precpred(p.GetParserRuleContext(), 6)) {
					p.SetError(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 6)", ""))
					goto errorExit
				}
				p.SetState(320)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_alt = 1
				for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
					switch _alt {
					case 1:
						p.SetState(309)
						p.GetErrorHandler().Sync(p)
						if p.HasError() {
							goto errorExit
						}
						_la = p.GetTokenStream().LA(1)

						for ok := true; ok; ok = _la == ZitiQlParserWS {
							{
								p.SetState(308)
								p.Match(ZitiQlParserWS)
								if p.HasError() {
									// Recognition error - abort rule
									goto errorExit
								}
							}

							p.SetState(311)
							p.GetErrorHandler().Sync(p)
							if p.HasError() {
								goto errorExit
							}
							_la = p.GetTokenStream().LA(1)
						}
						{
							p.SetState(313)
							p.Match(ZitiQlParserAND)
							if p.HasError() {
								// Recognition error - abort rule
								goto errorExit
							}
						}
						p.SetState(315)
						p.GetErrorHandler().Sync(p)
						if p.HasError() {
							goto errorExit
						}
						_la = p.GetTokenStream().LA(1)

						for ok := true; ok; ok = _la == ZitiQlParserWS {
							{
								p.SetState(314)
								p.Match(ZitiQlParserWS)
								if p.HasError() {
									// Recognition error - abort rule
									goto errorExit
								}
							}

							p.SetState(317)
							p.GetErrorHandler().Sync(p)
							if p.HasError() {
								goto errorExit
							}
							_la = p.GetTokenStream().LA(1)
						}
						{
							p.SetState(319)
							p.boolExpr(0)
						}

					default:
						p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
						goto errorExit
					}

					p.SetState(322)
					p.GetErrorHandler().Sync(p)
					_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 47, p.GetParserRuleContext())
					if p.HasError() {
						goto errorExit
					}
				}

			case 2:
				localctx = NewOrExprContext(p, NewBoolExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, ZitiQlParserRULE_boolExpr)
				p.SetState(324)

				if !(p.Precpred(p.GetParserRuleContext(), 5)) {
					p.SetError(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 5)", ""))
					goto errorExit
				}
				p.SetState(337)
				p.GetErrorHandler().Sync(p)
				if p.HasError() {
					goto errorExit
				}
				_alt = 1
				for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
					switch _alt {
					case 1:
						p.SetState(326)
						p.GetErrorHandler().Sync(p)
						if p.HasError() {
							goto errorExit
						}
						_la = p.GetTokenStream().LA(1)

						for ok := true; ok; ok = _la == ZitiQlParserWS {
							{
								p.SetState(325)
								p.Match(ZitiQlParserWS)
								if p.HasError() {
									// Recognition error - abort rule
									goto errorExit
								}
							}

							p.SetState(328)
							p.GetErrorHandler().Sync(p)
							if p.HasError() {
								goto errorExit
							}
							_la = p.GetTokenStream().LA(1)
						}
						{
							p.SetState(330)
							p.Match(ZitiQlParserOR)
							if p.HasError() {
								// Recognition error - abort rule
								goto errorExit
							}
						}
						p.SetState(332)
						p.GetErrorHandler().Sync(p)
						if p.HasError() {
							goto errorExit
						}
						_la = p.GetTokenStream().LA(1)

						for ok := true; ok; ok = _la == ZitiQlParserWS {
							{
								p.SetState(331)
								p.Match(ZitiQlParserWS)
								if p.HasError() {
									// Recognition error - abort rule
									goto errorExit
								}
							}

							p.SetState(334)
							p.GetErrorHandler().Sync(p)
							if p.HasError() {
								goto errorExit
							}
							_la = p.GetTokenStream().LA(1)
						}
						{
							p.SetState(336)
							p.boolExpr(0)
						}

					default:
						p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
						goto errorExit
					}

					p.SetState(339)
					p.GetErrorHandler().Sync(p)
					_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 50, p.GetParserRuleContext())
					if p.HasError() {
						goto errorExit
					}
				}

			case antlr.ATNInvalidAltNumber:
				goto errorExit
			}

		}
		p.SetState(345)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 52, p.GetParserRuleContext())
		if p.HasError() {
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.UnrollRecursionContexts(_parentctx)
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IOperationContext is an interface to support dynamic dispatch.
type IOperationContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsOperationContext differentiates from other interfaces.
	IsOperationContext()
}

type OperationContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperationContext() *OperationContext {
	var p = new(OperationContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_operation
	return p
}

func InitEmptyOperationContext(p *OperationContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_operation
}

func (*OperationContext) IsOperationContext() {}

func NewOperationContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperationContext {
	var p = new(OperationContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_operation

	return p
}

func (s *OperationContext) GetParser() antlr.Parser { return s.parser }

func (s *OperationContext) CopyAll(ctx *OperationContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *OperationContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperationContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type BinaryEqualToNullOpContext struct {
	OperationContext
}

func NewBinaryEqualToNullOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToNullOpContext {
	var p = new(BinaryEqualToNullOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToNullOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToNullOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryEqualToNullOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToNullOpContext) NULL() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNULL, 0)
}

func (s *BinaryEqualToNullOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToNullOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToNullOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToNullOp(s)
	}
}

func (s *BinaryEqualToNullOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToNullOp(s)
	}
}

type BinaryLessThanNumberOpContext struct {
	OperationContext
}

func NewBinaryLessThanNumberOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryLessThanNumberOpContext {
	var p = new(BinaryLessThanNumberOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryLessThanNumberOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryLessThanNumberOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryLessThanNumberOpContext) LT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLT, 0)
}

func (s *BinaryLessThanNumberOpContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *BinaryLessThanNumberOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryLessThanNumberOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryLessThanNumberOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryLessThanNumberOp(s)
	}
}

func (s *BinaryLessThanNumberOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryLessThanNumberOp(s)
	}
}

type BinaryGreaterThanDatetimeOpContext struct {
	OperationContext
}

func NewBinaryGreaterThanDatetimeOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryGreaterThanDatetimeOpContext {
	var p = new(BinaryGreaterThanDatetimeOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryGreaterThanDatetimeOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryGreaterThanDatetimeOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryGreaterThanDatetimeOpContext) GT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserGT, 0)
}

func (s *BinaryGreaterThanDatetimeOpContext) DATETIME() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, 0)
}

func (s *BinaryGreaterThanDatetimeOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryGreaterThanDatetimeOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryGreaterThanDatetimeOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryGreaterThanDatetimeOp(s)
	}
}

func (s *BinaryGreaterThanDatetimeOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryGreaterThanDatetimeOp(s)
	}
}

type InNumberArrayOpContext struct {
	OperationContext
}

func NewInNumberArrayOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *InNumberArrayOpContext {
	var p = new(InNumberArrayOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *InNumberArrayOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InNumberArrayOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *InNumberArrayOpContext) IN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIN, 0)
}

func (s *InNumberArrayOpContext) NumberArray() INumberArrayContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INumberArrayContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INumberArrayContext)
}

func (s *InNumberArrayOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *InNumberArrayOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *InNumberArrayOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterInNumberArrayOp(s)
	}
}

func (s *InNumberArrayOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitInNumberArrayOp(s)
	}
}

type InStringArrayOpContext struct {
	OperationContext
}

func NewInStringArrayOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *InStringArrayOpContext {
	var p = new(InStringArrayOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *InStringArrayOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InStringArrayOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *InStringArrayOpContext) IN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIN, 0)
}

func (s *InStringArrayOpContext) StringArray() IStringArrayContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringArrayContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringArrayContext)
}

func (s *InStringArrayOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *InStringArrayOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *InStringArrayOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterInStringArrayOp(s)
	}
}

func (s *InStringArrayOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitInStringArrayOp(s)
	}
}

type BinaryLessThanDatetimeOpContext struct {
	OperationContext
}

func NewBinaryLessThanDatetimeOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryLessThanDatetimeOpContext {
	var p = new(BinaryLessThanDatetimeOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryLessThanDatetimeOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryLessThanDatetimeOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryLessThanDatetimeOpContext) LT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLT, 0)
}

func (s *BinaryLessThanDatetimeOpContext) DATETIME() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, 0)
}

func (s *BinaryLessThanDatetimeOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryLessThanDatetimeOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryLessThanDatetimeOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryLessThanDatetimeOp(s)
	}
}

func (s *BinaryLessThanDatetimeOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryLessThanDatetimeOp(s)
	}
}

type BinaryGreaterThanNumberOpContext struct {
	OperationContext
}

func NewBinaryGreaterThanNumberOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryGreaterThanNumberOpContext {
	var p = new(BinaryGreaterThanNumberOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryGreaterThanNumberOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryGreaterThanNumberOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryGreaterThanNumberOpContext) GT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserGT, 0)
}

func (s *BinaryGreaterThanNumberOpContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *BinaryGreaterThanNumberOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryGreaterThanNumberOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryGreaterThanNumberOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryGreaterThanNumberOp(s)
	}
}

func (s *BinaryGreaterThanNumberOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryGreaterThanNumberOp(s)
	}
}

type InDatetimeArrayOpContext struct {
	OperationContext
}

func NewInDatetimeArrayOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *InDatetimeArrayOpContext {
	var p = new(InDatetimeArrayOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *InDatetimeArrayOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InDatetimeArrayOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *InDatetimeArrayOpContext) IN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIN, 0)
}

func (s *InDatetimeArrayOpContext) DatetimeArray() IDatetimeArrayContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDatetimeArrayContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDatetimeArrayContext)
}

func (s *InDatetimeArrayOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *InDatetimeArrayOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *InDatetimeArrayOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterInDatetimeArrayOp(s)
	}
}

func (s *InDatetimeArrayOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitInDatetimeArrayOp(s)
	}
}

type BetweenDateOpContext struct {
	OperationContext
}

func NewBetweenDateOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BetweenDateOpContext {
	var p = new(BetweenDateOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BetweenDateOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BetweenDateOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BetweenDateOpContext) BETWEEN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserBETWEEN, 0)
}

func (s *BetweenDateOpContext) AllDATETIME() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserDATETIME)
}

func (s *BetweenDateOpContext) DATETIME(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, i)
}

func (s *BetweenDateOpContext) AND() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserAND, 0)
}

func (s *BetweenDateOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BetweenDateOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BetweenDateOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBetweenDateOp(s)
	}
}

func (s *BetweenDateOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBetweenDateOp(s)
	}
}

type BinaryGreaterThanStringOpContext struct {
	OperationContext
}

func NewBinaryGreaterThanStringOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryGreaterThanStringOpContext {
	var p = new(BinaryGreaterThanStringOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryGreaterThanStringOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryGreaterThanStringOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryGreaterThanStringOpContext) GT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserGT, 0)
}

func (s *BinaryGreaterThanStringOpContext) STRING() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSTRING, 0)
}

func (s *BinaryGreaterThanStringOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryGreaterThanStringOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryGreaterThanStringOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryGreaterThanStringOp(s)
	}
}

func (s *BinaryGreaterThanStringOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryGreaterThanStringOp(s)
	}
}

type BinaryEqualToNumberOpContext struct {
	OperationContext
}

func NewBinaryEqualToNumberOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToNumberOpContext {
	var p = new(BinaryEqualToNumberOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToNumberOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToNumberOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryEqualToNumberOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToNumberOpContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *BinaryEqualToNumberOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToNumberOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToNumberOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToNumberOp(s)
	}
}

func (s *BinaryEqualToNumberOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToNumberOp(s)
	}
}

type BinaryEqualToBoolOpContext struct {
	OperationContext
}

func NewBinaryEqualToBoolOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToBoolOpContext {
	var p = new(BinaryEqualToBoolOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToBoolOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToBoolOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryEqualToBoolOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToBoolOpContext) BOOL() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserBOOL, 0)
}

func (s *BinaryEqualToBoolOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToBoolOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToBoolOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToBoolOp(s)
	}
}

func (s *BinaryEqualToBoolOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToBoolOp(s)
	}
}

type BinaryEqualToStringOpContext struct {
	OperationContext
}

func NewBinaryEqualToStringOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToStringOpContext {
	var p = new(BinaryEqualToStringOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToStringOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToStringOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryEqualToStringOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToStringOpContext) STRING() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSTRING, 0)
}

func (s *BinaryEqualToStringOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToStringOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToStringOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToStringOp(s)
	}
}

func (s *BinaryEqualToStringOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToStringOp(s)
	}
}

type BetweenNumberOpContext struct {
	OperationContext
}

func NewBetweenNumberOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BetweenNumberOpContext {
	var p = new(BetweenNumberOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BetweenNumberOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BetweenNumberOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BetweenNumberOpContext) BETWEEN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserBETWEEN, 0)
}

func (s *BetweenNumberOpContext) AllNUMBER() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserNUMBER)
}

func (s *BetweenNumberOpContext) NUMBER(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, i)
}

func (s *BetweenNumberOpContext) AND() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserAND, 0)
}

func (s *BetweenNumberOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BetweenNumberOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BetweenNumberOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBetweenNumberOp(s)
	}
}

func (s *BetweenNumberOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBetweenNumberOp(s)
	}
}

type BinaryContainsOpContext struct {
	OperationContext
}

func NewBinaryContainsOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryContainsOpContext {
	var p = new(BinaryContainsOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryContainsOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryContainsOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryContainsOpContext) CONTAINS() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserCONTAINS, 0)
}

func (s *BinaryContainsOpContext) STRING() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSTRING, 0)
}

func (s *BinaryContainsOpContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *BinaryContainsOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryContainsOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryContainsOpContext) ICONTAINS() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserICONTAINS, 0)
}

func (s *BinaryContainsOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryContainsOp(s)
	}
}

func (s *BinaryContainsOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryContainsOp(s)
	}
}

type BinaryLessThanStringOpContext struct {
	OperationContext
}

func NewBinaryLessThanStringOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryLessThanStringOpContext {
	var p = new(BinaryLessThanStringOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryLessThanStringOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryLessThanStringOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryLessThanStringOpContext) LT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLT, 0)
}

func (s *BinaryLessThanStringOpContext) STRING() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSTRING, 0)
}

func (s *BinaryLessThanStringOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryLessThanStringOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryLessThanStringOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryLessThanStringOp(s)
	}
}

func (s *BinaryLessThanStringOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryLessThanStringOp(s)
	}
}

type BinaryEqualToDatetimeOpContext struct {
	OperationContext
}

func NewBinaryEqualToDatetimeOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToDatetimeOpContext {
	var p = new(BinaryEqualToDatetimeOpContext)

	InitEmptyOperationContext(&p.OperationContext)
	p.parser = parser
	p.CopyAll(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToDatetimeOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToDatetimeOpContext) BinaryLhs() IBinaryLhsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBinaryLhsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBinaryLhsContext)
}

func (s *BinaryEqualToDatetimeOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToDatetimeOpContext) DATETIME() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, 0)
}

func (s *BinaryEqualToDatetimeOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToDatetimeOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToDatetimeOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToDatetimeOp(s)
	}
}

func (s *BinaryEqualToDatetimeOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToDatetimeOp(s)
	}
}

func (p *ZitiQlParser) Operation() (localctx IOperationContext) {
	localctx = NewOperationContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, ZitiQlParserRULE_operation)
	var _la int

	p.SetState(646)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 93, p.GetParserRuleContext()) {
	case 1:
		localctx = NewInStringArrayOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(346)
			p.BinaryLhs()
		}
		p.SetState(348)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(347)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(350)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(352)
			p.Match(ZitiQlParserIN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(354)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(353)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(356)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(358)
			p.StringArray()
		}

	case 2:
		localctx = NewInNumberArrayOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(360)
			p.BinaryLhs()
		}
		p.SetState(362)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(361)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(364)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(366)
			p.Match(ZitiQlParserIN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(368)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(367)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(370)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(372)
			p.NumberArray()
		}

	case 3:
		localctx = NewInDatetimeArrayOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(374)
			p.BinaryLhs()
		}
		p.SetState(376)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(375)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(378)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(380)
			p.Match(ZitiQlParserIN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(382)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(381)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(384)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(386)
			p.DatetimeArray()
		}

	case 4:
		localctx = NewBetweenNumberOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(388)
			p.BinaryLhs()
		}
		p.SetState(390)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(389)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(392)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(394)
			p.Match(ZitiQlParserBETWEEN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(396)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(395)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(398)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(400)
			p.Match(ZitiQlParserNUMBER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(402)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(401)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(404)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(406)
			p.Match(ZitiQlParserAND)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(408)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(407)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(410)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(412)
			p.Match(ZitiQlParserNUMBER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 5:
		localctx = NewBetweenDateOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(414)
			p.BinaryLhs()
		}
		p.SetState(416)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(415)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(418)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(420)
			p.Match(ZitiQlParserBETWEEN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(422)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(421)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(424)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(426)
			p.Match(ZitiQlParserDATETIME)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(428)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(427)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(430)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(432)
			p.Match(ZitiQlParserAND)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(434)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(433)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(436)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(438)
			p.Match(ZitiQlParserDATETIME)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 6:
		localctx = NewBinaryLessThanStringOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(440)
			p.BinaryLhs()
		}
		p.SetState(444)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(441)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(446)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(447)
			p.Match(ZitiQlParserLT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(451)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(448)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(453)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(454)
			p.Match(ZitiQlParserSTRING)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 7:
		localctx = NewBinaryLessThanNumberOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(456)
			p.BinaryLhs()
		}
		p.SetState(460)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(457)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(462)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(463)
			p.Match(ZitiQlParserLT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(467)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(464)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(469)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(470)
			p.Match(ZitiQlParserNUMBER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 8:
		localctx = NewBinaryLessThanDatetimeOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(472)
			p.BinaryLhs()
		}
		p.SetState(476)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(473)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(478)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(479)
			p.Match(ZitiQlParserLT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(483)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(480)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(485)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(486)
			p.Match(ZitiQlParserDATETIME)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 9:
		localctx = NewBinaryGreaterThanStringOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(488)
			p.BinaryLhs()
		}
		p.SetState(492)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(489)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(494)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(495)
			p.Match(ZitiQlParserGT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(499)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(496)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(501)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(502)
			p.Match(ZitiQlParserSTRING)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 10:
		localctx = NewBinaryGreaterThanNumberOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(504)
			p.BinaryLhs()
		}
		p.SetState(508)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(505)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(510)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(511)
			p.Match(ZitiQlParserGT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(515)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(512)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(517)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(518)
			p.Match(ZitiQlParserNUMBER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 11:
		localctx = NewBinaryGreaterThanDatetimeOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(520)
			p.BinaryLhs()
		}
		p.SetState(524)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(521)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(526)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(527)
			p.Match(ZitiQlParserGT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(531)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(528)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(533)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(534)
			p.Match(ZitiQlParserDATETIME)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 12:
		localctx = NewBinaryEqualToStringOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(536)
			p.BinaryLhs()
		}
		p.SetState(540)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(537)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(542)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(543)
			p.Match(ZitiQlParserEQ)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(547)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(544)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(549)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(550)
			p.Match(ZitiQlParserSTRING)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 13:
		localctx = NewBinaryEqualToNumberOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 13)
		{
			p.SetState(552)
			p.BinaryLhs()
		}
		p.SetState(556)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(553)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(558)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(559)
			p.Match(ZitiQlParserEQ)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(563)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(560)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(565)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(566)
			p.Match(ZitiQlParserNUMBER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 14:
		localctx = NewBinaryEqualToDatetimeOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 14)
		{
			p.SetState(568)
			p.BinaryLhs()
		}
		p.SetState(572)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(569)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(574)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(575)
			p.Match(ZitiQlParserEQ)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(579)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(576)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(581)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(582)
			p.Match(ZitiQlParserDATETIME)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 15:
		localctx = NewBinaryEqualToBoolOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 15)
		{
			p.SetState(584)
			p.BinaryLhs()
		}
		p.SetState(588)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(585)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(590)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(591)
			p.Match(ZitiQlParserEQ)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(595)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(592)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(597)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(598)
			p.Match(ZitiQlParserBOOL)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 16:
		localctx = NewBinaryEqualToNullOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 16)
		{
			p.SetState(600)
			p.BinaryLhs()
		}
		p.SetState(604)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(601)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(606)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(607)
			p.Match(ZitiQlParserEQ)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(611)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(608)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(613)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(614)
			p.Match(ZitiQlParserNULL)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 17:
		localctx = NewBinaryContainsOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 17)
		{
			p.SetState(616)
			p.BinaryLhs()
		}
		p.SetState(620)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(617)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(622)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(623)
			p.Match(ZitiQlParserCONTAINS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(625)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(624)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(627)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(629)
			_la = p.GetTokenStream().LA(1)

			if !(_la == ZitiQlParserSTRING || _la == ZitiQlParserNUMBER) {
				p.GetErrorHandler().RecoverInline(p)
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}

	case 18:
		localctx = NewBinaryContainsOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 18)
		{
			p.SetState(631)
			p.BinaryLhs()
		}
		p.SetState(635)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(632)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(637)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(638)
			p.Match(ZitiQlParserICONTAINS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(640)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(639)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(642)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(644)
			p.Match(ZitiQlParserSTRING)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case antlr.ATNInvalidAltNumber:
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IBinaryLhsContext is an interface to support dynamic dispatch.
type IBinaryLhsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	IDENTIFIER() antlr.TerminalNode
	SetFunction() ISetFunctionContext

	// IsBinaryLhsContext differentiates from other interfaces.
	IsBinaryLhsContext()
}

type BinaryLhsContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBinaryLhsContext() *BinaryLhsContext {
	var p = new(BinaryLhsContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_binaryLhs
	return p
}

func InitEmptyBinaryLhsContext(p *BinaryLhsContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_binaryLhs
}

func (*BinaryLhsContext) IsBinaryLhsContext() {}

func NewBinaryLhsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BinaryLhsContext {
	var p = new(BinaryLhsContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_binaryLhs

	return p
}

func (s *BinaryLhsContext) GetParser() antlr.Parser { return s.parser }

func (s *BinaryLhsContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryLhsContext) SetFunction() ISetFunctionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISetFunctionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISetFunctionContext)
}

func (s *BinaryLhsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryLhsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *BinaryLhsContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryLhs(s)
	}
}

func (s *BinaryLhsContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryLhs(s)
	}
}

func (p *ZitiQlParser) BinaryLhs() (localctx IBinaryLhsContext) {
	localctx = NewBinaryLhsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, ZitiQlParserRULE_binaryLhs)
	p.SetState(650)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case ZitiQlParserIDENTIFIER:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(648)
			p.Match(ZitiQlParserIDENTIFIER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case ZitiQlParserALL_OF, ZitiQlParserANY_OF, ZitiQlParserCOUNT:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(649)
			p.SetFunction()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISetFunctionContext is an interface to support dynamic dispatch.
type ISetFunctionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsSetFunctionContext differentiates from other interfaces.
	IsSetFunctionContext()
}

type SetFunctionContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySetFunctionContext() *SetFunctionContext {
	var p = new(SetFunctionContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_setFunction
	return p
}

func InitEmptySetFunctionContext(p *SetFunctionContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_setFunction
}

func (*SetFunctionContext) IsSetFunctionContext() {}

func NewSetFunctionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SetFunctionContext {
	var p = new(SetFunctionContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_setFunction

	return p
}

func (s *SetFunctionContext) GetParser() antlr.Parser { return s.parser }

func (s *SetFunctionContext) CopyAll(ctx *SetFunctionContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *SetFunctionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SetFunctionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type SetFunctionExprContext struct {
	SetFunctionContext
}

func NewSetFunctionExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *SetFunctionExprContext {
	var p = new(SetFunctionExprContext)

	InitEmptySetFunctionContext(&p.SetFunctionContext)
	p.parser = parser
	p.CopyAll(ctx.(*SetFunctionContext))

	return p
}

func (s *SetFunctionExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SetFunctionExprContext) ALL_OF() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserALL_OF, 0)
}

func (s *SetFunctionExprContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLPAREN, 0)
}

func (s *SetFunctionExprContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *SetFunctionExprContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRPAREN, 0)
}

func (s *SetFunctionExprContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *SetFunctionExprContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *SetFunctionExprContext) ANY_OF() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserANY_OF, 0)
}

func (s *SetFunctionExprContext) COUNT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserCOUNT, 0)
}

func (s *SetFunctionExprContext) SetExpr() ISetExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISetExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISetExprContext)
}

func (s *SetFunctionExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterSetFunctionExpr(s)
	}
}

func (s *SetFunctionExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitSetFunctionExpr(s)
	}
}

func (p *ZitiQlParser) SetFunction() (localctx ISetFunctionContext) {
	localctx = NewSetFunctionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, ZitiQlParserRULE_setFunction)
	var _la int

	p.SetState(701)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case ZitiQlParserALL_OF:
		localctx = NewSetFunctionExprContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(652)
			p.Match(ZitiQlParserALL_OF)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(653)
			p.Match(ZitiQlParserLPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(657)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(654)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(659)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(660)
			p.Match(ZitiQlParserIDENTIFIER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(664)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(661)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(666)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(667)
			p.Match(ZitiQlParserRPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case ZitiQlParserANY_OF:
		localctx = NewSetFunctionExprContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(668)
			p.Match(ZitiQlParserANY_OF)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(669)
			p.Match(ZitiQlParserLPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(673)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(670)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(675)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(676)
			p.Match(ZitiQlParserIDENTIFIER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(680)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(677)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(682)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(683)
			p.Match(ZitiQlParserRPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case ZitiQlParserCOUNT:
		localctx = NewSetFunctionExprContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(684)
			p.Match(ZitiQlParserCOUNT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(685)
			p.Match(ZitiQlParserLPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(689)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(686)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(691)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(692)
			p.SetExpr()
		}
		p.SetState(696)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(693)
				p.Match(ZitiQlParserWS)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

			p.SetState(698)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(699)
			p.Match(ZitiQlParserRPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISetExprContext is an interface to support dynamic dispatch.
type ISetExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	IDENTIFIER() antlr.TerminalNode
	SubQueryExpr() ISubQueryExprContext

	// IsSetExprContext differentiates from other interfaces.
	IsSetExprContext()
}

type SetExprContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySetExprContext() *SetExprContext {
	var p = new(SetExprContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_setExpr
	return p
}

func InitEmptySetExprContext(p *SetExprContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_setExpr
}

func (*SetExprContext) IsSetExprContext() {}

func NewSetExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SetExprContext {
	var p = new(SetExprContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_setExpr

	return p
}

func (s *SetExprContext) GetParser() antlr.Parser { return s.parser }

func (s *SetExprContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *SetExprContext) SubQueryExpr() ISubQueryExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISubQueryExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISubQueryExprContext)
}

func (s *SetExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SetExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SetExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterSetExpr(s)
	}
}

func (s *SetExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitSetExpr(s)
	}
}

func (p *ZitiQlParser) SetExpr() (localctx ISetExprContext) {
	localctx = NewSetExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, ZitiQlParserRULE_setExpr)
	p.SetState(705)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case ZitiQlParserIDENTIFIER:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(703)
			p.Match(ZitiQlParserIDENTIFIER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case ZitiQlParserFROM:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(704)
			p.SubQueryExpr()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISubQueryExprContext is an interface to support dynamic dispatch.
type ISubQueryExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsSubQueryExprContext differentiates from other interfaces.
	IsSubQueryExprContext()
}

type SubQueryExprContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySubQueryExprContext() *SubQueryExprContext {
	var p = new(SubQueryExprContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_subQueryExpr
	return p
}

func InitEmptySubQueryExprContext(p *SubQueryExprContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ZitiQlParserRULE_subQueryExpr
}

func (*SubQueryExprContext) IsSubQueryExprContext() {}

func NewSubQueryExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SubQueryExprContext {
	var p = new(SubQueryExprContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_subQueryExpr

	return p
}

func (s *SubQueryExprContext) GetParser() antlr.Parser { return s.parser }

func (s *SubQueryExprContext) CopyAll(ctx *SubQueryExprContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *SubQueryExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SubQueryExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type SubQueryContext struct {
	SubQueryExprContext
}

func NewSubQueryContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *SubQueryContext {
	var p = new(SubQueryContext)

	InitEmptySubQueryExprContext(&p.SubQueryExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*SubQueryExprContext))

	return p
}

func (s *SubQueryContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SubQueryContext) FROM() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserFROM, 0)
}

func (s *SubQueryContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *SubQueryContext) WHERE() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWHERE, 0)
}

func (s *SubQueryContext) Query() IQueryContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IQueryContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IQueryContext)
}

func (s *SubQueryContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *SubQueryContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *SubQueryContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterSubQuery(s)
	}
}

func (s *SubQueryContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitSubQuery(s)
	}
}

func (p *ZitiQlParser) SubQueryExpr() (localctx ISubQueryExprContext) {
	localctx = NewSubQueryExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, ZitiQlParserRULE_subQueryExpr)
	var _la int

	localctx = NewSubQueryContext(p, localctx)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(707)
		p.Match(ZitiQlParserFROM)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(709)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == ZitiQlParserWS {
		{
			p.SetState(708)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(711)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(713)
		p.Match(ZitiQlParserIDENTIFIER)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(715)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == ZitiQlParserWS {
		{
			p.SetState(714)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(717)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(719)
		p.Match(ZitiQlParserWHERE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(721)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == ZitiQlParserWS {
		{
			p.SetState(720)
			p.Match(ZitiQlParserWS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(723)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(725)
		p.Query()
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

func (p *ZitiQlParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 9:
		var t *BoolExprContext = nil
		if localctx != nil {
			t = localctx.(*BoolExprContext)
		}
		return p.BoolExpr_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *ZitiQlParser) BoolExpr_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	switch predIndex {
	case 0:
		return p.Precpred(p.GetParserRuleContext(), 6)

	case 1:
		return p.Precpred(p.GetParserRuleContext(), 5)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
