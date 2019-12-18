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
	"fmt"
	"github.com/netfoundry/ziti-edge/controller/zitiql"
	"github.com/stretchr/testify/assert"
	"gopkg.in/Masterminds/squirrel.v1"
	"log"
	"testing"
)

/*
	Integration level testing of the parsing engine for ZitiQl + ToSquirrelListener for numbers
*/

func BenchmarkParseSimple(b *testing.B) {
	listener := NewSquirrelListener()
	zitiql.Parse("a=1", listener)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("SQL: %s\n - %v", sql, args)
}

// ------------------------------------ EQ ---------------------------------------

func TestParse_Single_Equal_Number_PositiveSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=1", "a", 1)
}

func TestParse_Single_Equal_Number_PositiveSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, " \n \ta\t=\t1\t \n ", "a", 1)
}

func TestParse_Single_Equal_Number_NegativeSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=-456", "a", -456)
}

func TestParse_Single_Equal_Number_NegativeSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, " \n \ta\t=\t-156\t \n ", "a", -156)
}

func TestParse_Single_Equal_Number_PositiveMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=18975565", "a", 18975565)
}

func TestParse_Single_Equal_Number_PositiveMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, "\t a \n \t  =   \t \t 18975565 \t ", "a", 18975565)
}

func TestParse_Single_Equal_Number_NegativeMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=-18975565", "a", -18975565)
}

func TestParse_Single_Equal_Number_NegativeMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, "\t a \n \t  =   \t \t -18975565 \t ", "a", -18975565)
}

func TestParse_Single_Equal_Number_PositiveFloat_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=4.4564859489", "a", 4.4564859489)
}

func TestParse_Single_Equal_Number_PositiveFloat_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, "\t a \n \t  =   \t \t 4.4564859489 \t ", "a", 4.4564859489)
}

func TestParse_Single_Equal_Number_NegativeFloat_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=-4.4564859489", "a", -4.4564859489)
}

func TestParse_Single_Equal_Number_NegativeFloat_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, "\t a \n \t  =   \t \t -4.4564859489 \t ", "a", -4.4564859489)
}

func TestParse_Single_Equal_Number_NegativeFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=-95.65e10", "a", -95.65e10)
}

func TestParse_Single_Equal_Number_NegativeFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, "\t a \n \t  =   \t \t -95.65e10 \t ", "a", -95.65e10)
}

func TestParse_Single_Equal_Number_NegativeFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=-6.45e-5", "a", -6.45e-5)
}

func TestParse_Single_Equal_Number_NegativeFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, "\t a \n \t  =   \t \t -6.45e-5 \t ", "a", -6.45e-5)
}

func TestParse_Single_Equal_Number_PositiveFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=95.65e10", "a", 95.65e10)
}

func TestParse_Single_Equal_Number_PositiveFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, "\t a \n \t  =   \t \t 95.65e10 \t ", "a", 95.65e10)
}

func TestParse_Single_Equal_Number_PositiveFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberEqual(t, "a=6.45e-5", "a", 6.45e-5)
}

func TestParse_Single_Equal_Number_PositiveFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberEqual(t, "\t a \n \t  =   \t \t 6.45e-5 \t ", "a", 6.45e-5)
}

// ------------------------------------ NEQ ---------------------------------------

func TestParse_Single_NotEqual_Number_PositiveSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=1", "a", 1)
}

func TestParse_Single_NotEqual_Number_PositiveSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, " \n \ta\t!=\t1\t \n ", "a", 1)
}

func TestParse_Single_NotEqual_Number_NegativeSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=-456", "a", -456)
}

func TestParse_Single_NotEqual_Number_NegativeSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, " \n \ta\t!=\t-156\t \n ", "a", -156)
}

func TestParse_Single_NotEqual_Number_PositiveMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=18975565", "a", 18975565)
}

func TestParse_Single_NotEqual_Number_PositiveMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "\t a \n \t  !=   \t \t 18975565 \t ", "a", 18975565)
}

func TestParse_Single_NotEqual_Number_NegativeMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=-18975565", "a", -18975565)
}

func TestParse_Single_NotEqual_Number_NegativeMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "\t a \n \t  !=   \t \t -18975565 \t ", "a", -18975565)
}

func TestParse_Single_NotEqual_Number_PositiveFloat_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=4.4564859489", "a", 4.4564859489)
}

func TestParse_Single_NotEqual_Number_PositiveFloat_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "\t a \n \t  !=   \t \t 4.4564859489 \t ", "a", 4.4564859489)
}

func TestParse_Single_NotEqual_Number_NegativeFloat_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=-4.4564859489", "a", -4.4564859489)
}

func TestParse_Single_NotEqual_Number_NegativeFloat_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "\t a \n \t  !=   \t \t -4.4564859489 \t ", "a", -4.4564859489)
}

func TestParse_Single_NotEqual_Number_NegativeFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=-95.65e10", "a", -95.65e10)
}

func TestParse_Single_NotEqual_Number_NegativeFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "\t a \n \t  !=   \t \t -95.65e10 \t ", "a", -95.65e10)
}

func TestParse_Single_NotEqual_Number_NegativeFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=-6.45e-5", "a", -6.45e-5)
}

func TestParse_Single_NotEqual_Number_NegativeFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "\t a \n \t  !=   \t \t -6.45e-5 \t ", "a", -6.45e-5)
}

func TestParse_Single_NotEqual_Number_PositiveFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=95.65e10", "a", 95.65e10)
}

func TestParse_Single_NotEqual_Number_PositiveFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "\t a \n \t  !=   \t \t 95.65e10 \t ", "a", 95.65e10)
}

func TestParse_Single_NotEqual_Number_PositiveFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "a!=6.45e-5", "a", 6.45e-5)
}

func TestParse_Single_NotEqual_Number_PositiveFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberNotEqual(t, "\t a \n \t  !=   \t \t 6.45e-5 \t ", "a", 6.45e-5)
}

// ------------------------------------ GT ---------------------------------------

func TestParse_Single_GreaterThan_Number_PositiveSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>1", "a", 1)
}

func TestParse_Single_GreaterThan_Number_PositiveSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, " \n \ta\t>\t1\t \n ", "a", 1)
}

func TestParse_Single_GreaterThan_Number_NegativeSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>-456", "a", -456)
}

func TestParse_Single_GreaterThan_Number_NegativeSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, " \n \ta\t>\t-156\t \n ", "a", -156)
}

func TestParse_Single_GreaterThan_Number_PositiveMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>18975565", "a", 18975565)
}

func TestParse_Single_GreaterThan_Number_PositiveMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "\t a \n \t  >   \t \t 18975565 \t ", "a", 18975565)
}

func TestParse_Single_GreaterThan_Number_NegativeMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>-18975565", "a", -18975565)
}

func TestParse_Single_GreaterThan_Number_NegativeMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "\t a \n \t  >   \t \t -18975565 \t ", "a", -18975565)
}

func TestParse_Single_GreaterThan_Number_PositiveFloat_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>4.4564859489", "a", 4.4564859489)
}

func TestParse_Single_GreaterThan_Number_PositiveFloat_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "\t a \n \t  >   \t \t 4.4564859489 \t ", "a", 4.4564859489)
}

func TestParse_Single_GreaterThan_Number_NegativeFloat_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>-4.4564859489", "a", -4.4564859489)
}

func TestParse_Single_GreaterThan_Number_NegativeFloat_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "\t a \n \t  >   \t \t -4.4564859489 \t ", "a", -4.4564859489)
}

func TestParse_Single_GreaterThan_Number_NegativeFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>-95.65e10", "a", -95.65e10)
}

func TestParse_Single_GreaterThan_Number_NegativeFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "\t a \n \t  >   \t \t -95.65e10 \t ", "a", -95.65e10)
}

func TestParse_Single_GreaterThan_Number_NegativeFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>-6.45e-5", "a", -6.45e-5)
}

func TestParse_Single_GreaterThan_Number_NegativeFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "\t a \n \t  >   \t \t -6.45e-5 \t ", "a", -6.45e-5)
}

func TestParse_Single_GreaterThan_Number_PositiveFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>95.65e10", "a", 95.65e10)
}

func TestParse_Single_GreaterThan_Number_PositiveFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "\t a \n \t  >   \t \t 95.65e10 \t ", "a", 95.65e10)
}

func TestParse_Single_GreaterThan_Number_PositiveFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "a>6.45e-5", "a", 6.45e-5)
}

func TestParse_Single_GreaterThan_Number_PositiveFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberGreaterThan(t, "\t a \n \t  >   \t \t 6.45e-5 \t ", "a", 6.45e-5)
}

// ------------------------------------ GTE ---------------------------------------

func TestParse_Single_GreaterThanEqual_Number_PositiveSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=1", "a", 1)
}

func TestParse_Single_GreaterThanEqual_Number_PositiveSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, " \n \ta\t>=\t1\t \n ", "a", 1)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=-456", "a", -456)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, " \n \ta\t>=\t-156\t \n ", "a", -156)
}

func TestParse_Single_GreaterThanEqual_Number_PositiveMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=18975565", "a", 18975565)
}

func TestParse_Single_GreaterThanEqual_Number_PositiveMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "\t a \n \t  >=   \t \t 18975565 \t ", "a", 18975565)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=-18975565", "a", -18975565)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "\t a \n \t  >=   \t \t -18975565 \t ", "a", -18975565)
}

func TestParse_Single_GreaterThanEqual_Number_PositiveFloat_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=4.4564859489", "a", 4.4564859489)
}

func TestParse_Single_GreaterThanEqual_Number_PositiveFloat_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "\t a \n \t  >=   \t \t 4.4564859489 \t ", "a", 4.4564859489)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeFloat_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=-4.4564859489", "a", -4.4564859489)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeFloat_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "\t a \n \t  >=   \t \t -4.4564859489 \t ", "a", -4.4564859489)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=-95.65e10", "a", -95.65e10)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "\t a \n \t  >=   \t \t -95.65e10 \t ", "a", -95.65e10)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=-6.45e-5", "a", -6.45e-5)
}

func TestParse_Single_GreaterThanEqual_Number_NegativeFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "\t a \n \t  >=   \t \t -6.45e-5 \t ", "a", -6.45e-5)
}

func TestParse_Single_GreaterThanEqual_Number_PositiveFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=95.65e10", "a", 95.65e10)
}

func TestParse_Single_GreaterThanEqual_Number_PositiveFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "\t a \n \t  >=   \t \t 95.65e10 \t ", "a", 95.65e10)
}

func TestParse_Single_GreaterThanEqual_Number_PositiveFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "a>=6.45e-5", "a", 6.45e-5)
}

func TestParse_Single_GreaterThanEqual_Number_PositiveFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberGreaterThanEqual(t, "\t a \n \t  >=   \t \t 6.45e-5 \t ", "a", 6.45e-5)
}

// ------------------------------------ LT ---------------------------------------

func TestParse_Single_LessThan_Number_PositiveSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<1", "a", 1)
}

func TestParse_Single_LessThan_Number_PositiveSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, " \n \ta\t<\t1\t \n ", "a", 1)
}

func TestParse_Single_LessThan_Number_NegativeSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<-456", "a", -456)
}

func TestParse_Single_LessThan_Number_NegativeSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, " \n \ta\t<\t-156\t \n ", "a", -156)
}

func TestParse_Single_LessThan_Number_PositiveMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<18975565", "a", 18975565)
}

func TestParse_Single_LessThan_Number_PositiveMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, "\t a \n \t  <   \t \t 18975565 \t ", "a", 18975565)
}

func TestParse_Single_LessThan_Number_NegativeMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<-18975565", "a", -18975565)
}

func TestParse_Single_LessThan_Number_NegativeMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, "\t a \n \t  <   \t \t -18975565 \t ", "a", -18975565)
}

func TestParse_Single_LessThan_Number_PositiveFloat_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<4.4564859489", "a", 4.4564859489)
}

func TestParse_Single_LessThan_Number_PositiveFloat_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, "\t a \n \t  <   \t \t 4.4564859489 \t ", "a", 4.4564859489)
}

func TestParse_Single_LessThan_Number_NegativeFloat_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<-4.4564859489", "a", -4.4564859489)
}

func TestParse_Single_LessThan_Number_NegativeFloat_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, "\t a \n \t  <   \t \t -4.4564859489 \t ", "a", -4.4564859489)
}

func TestParse_Single_LessThan_Number_NegativeFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<-95.65e10", "a", -95.65e10)
}

func TestParse_Single_LessThan_Number_NegativeFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, "\t a \n \t  <   \t \t -95.65e10 \t ", "a", -95.65e10)
}

func TestParse_Single_LessThan_Number_NegativeFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<-6.45e-5", "a", -6.45e-5)
}

func TestParse_Single_LessThan_Number_NegativeFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, "\t a \n \t  <   \t \t -6.45e-5 \t ", "a", -6.45e-5)
}

func TestParse_Single_LessThan_Number_PositiveFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<95.65e10", "a", 95.65e10)
}

func TestParse_Single_LessThan_Number_PositiveFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, "\t a \n \t  <   \t \t 95.65e10 \t ", "a", 95.65e10)
}

func TestParse_Single_LessThan_Number_PositiveFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberLessThan(t, "a<6.45e-5", "a", 6.45e-5)
}

func TestParse_Single_LessThan_Number_PositiveFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberLessThan(t, "\t a \n \t  <   \t \t 6.45e-5 \t ", "a", 6.45e-5)
}

// ------------------------------------ LTE ---------------------------------------

func TestParse_Single_LessThanEqual_Number_PositiveSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=1", "a", 1)
}

func TestParse_Single_LessThanEqual_Number_PositiveSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, " \n \ta\t<=\t1\t \n ", "a", 1)
}

func TestParse_Single_LessThanEqual_Number_NegativeSingleDigit_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=-456", "a", -456)
}

func TestParse_Single_LessThanEqual_Number_NegativeSingleDigit_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, " \n \ta\t<=\t-156\t \n ", "a", -156)
}

func TestParse_Single_LessThanEqual_Number_PositiveMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=18975565", "a", 18975565)
}

func TestParse_Single_LessThanEqual_Number_PositiveMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "\t a \n \t  <=   \t \t 18975565 \t ", "a", 18975565)
}

func TestParse_Single_LessThanEqual_Number_NegativeMultiDigit_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=-18975565", "a", -18975565)
}

func TestParse_Single_LessThanEqual_Number_NegativeMultiDigit_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "\t a \n \t  <=   \t \t -18975565 \t ", "a", -18975565)
}

func TestParse_Single_LessThanEqual_Number_PositiveFloat_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=4.4564859489", "a", 4.4564859489)
}

func TestParse_Single_LessThanEqual_Number_PositiveFloat_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "\t a \n \t  <=   \t \t 4.4564859489 \t ", "a", 4.4564859489)
}

func TestParse_Single_LessThanEqual_Number_NegativeFloat_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=-4.4564859489", "a", -4.4564859489)
}

func TestParse_Single_LessThanEqual_Number_NegativeFloat_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "\t a \n \t  <=   \t \t -4.4564859489 \t ", "a", -4.4564859489)
}

func TestParse_Single_LessThanEqual_Number_NegativeFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=-95.65e10", "a", -95.65e10)
}

func TestParse_Single_LessThanEqual_Number_NegativeFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "\t a \n \t  <=   \t \t -95.65e10 \t ", "a", -95.65e10)
}

func TestParse_Single_LessThanEqual_Number_NegativeFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=-6.45e-5", "a", -6.45e-5)
}

func TestParse_Single_LessThanEqual_Number_NegativeFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "\t a \n \t  <=   \t \t -6.45e-5 \t ", "a", -6.45e-5)
}

func TestParse_Single_LessThanEqual_Number_PositiveFloat_PositiveExponent_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=95.65e10", "a", 95.65e10)
}

func TestParse_Single_LessThanEqual_Number_PositiveFloat_PositiveExponent_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "\t a \n \t  <=   \t \t 95.65e10 \t ", "a", 95.65e10)
}

func TestParse_Single_LessThanEqual_Number_PositiveFloat_NegativeExponent_NoWhitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "a<=6.45e-5", "a", 6.45e-5)
}

func TestParse_Single_LessThanEqual_Number_PositiveFloat_NegativeExponent_Whitespace(t *testing.T) {
	testSingleNumberLessThanEqual(t, "\t a \n \t  <=   \t \t 6.45e-5 \t ", "a", 6.45e-5)
}

// ------------------------------------ IN ARRAY ---------------------------------------
func TestParse_Single_InArray_Number_OneInteger(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse("a in [456465]", listener)

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

	assert.Equal(t, 1, len(args))
	assert.Equal(t, 456465, args[0])
}

func TestParse_Single_InArray_Number_MultipleIntegers(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse("a in [ 564,  -5064 , 4.5454, -0.0045, 99e-12, 2e4, -45e3, -99e-10   \n \t]", listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE a IN (?,?,?,?,?,?,?,?)", sql)

	opSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, opSet)

	op, ok := opSet[InOp]
	assert.True(t, ok)
	assert.True(t, op)

	assert.Equal(t, 8, len(args))
	assert.Equal(t, 564, args[0])
	assert.Equal(t, -5064, args[1])
	assert.Equal(t, 4.5454, args[2])
	assert.Equal(t, -0.0045, args[3])
	assert.Equal(t, 99e-12, args[4])
	assert.Equal(t, 2e4, args[5])
	assert.Equal(t, -45e3, args[6])
	assert.Equal(t, -99e-10, args[7])
}

// ------------------------------------ BETWEEN  ---------------------------------------

func TestParse_Single_Between_Number_Integers(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse("a BETWEEN 123 and 456", listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE a BETWEEN ? AND ?", sql)

	opSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, opSet)

	op, ok := opSet[BetweenOp]
	assert.True(t, ok)
	assert.True(t, op)

	assert.Equal(t, 2, len(args))
	assert.Equal(t, 123, args[0])
	assert.Equal(t, 456, args[1])
}

func TestParse_Single_Between_Number_Mixed(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse("a BETWEEN -456 and 4e10", listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE a BETWEEN ? AND ?", sql)

	opSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, opSet)

	op, ok := opSet[BetweenOp]
	assert.True(t, ok)
	assert.True(t, op)

	assert.Equal(t, 2, len(args))
	assert.Equal(t, -456, args[0])
	assert.Equal(t, 4e10, args[1])
}

// Helpers

func testSingleNumberLessThan(t *testing.T, input, identifier string, value interface{}) {
	testSingleNumberOp(t, input, identifier, "SELECT * FROM test WHERE a < ?", value, LtOp)
}

func testSingleNumberLessThanEqual(t *testing.T, input, identifier string, value interface{}) {
	testSingleNumberOp(t, input, identifier, "SELECT * FROM test WHERE a <= ?", value, LtEOp)
}

func testSingleNumberGreaterThan(t *testing.T, input, identifier string, value interface{}) {
	testSingleNumberOp(t, input, identifier, "SELECT * FROM test WHERE a > ?", value, GtOp)
}

func testSingleNumberGreaterThanEqual(t *testing.T, input, identifier string, value interface{}) {
	testSingleNumberOp(t, input, identifier, "SELECT * FROM test WHERE a >= ?", value, GtEOp)
}

func testSingleNumberEqual(t *testing.T, input, identifier string, value interface{}) {
	testSingleNumberOp(t, input, identifier, "SELECT * FROM test WHERE a = ?", value, EqOp)
}

func testSingleNumberNotEqual(t *testing.T, input, identifier string, value interface{}) {
	testSingleNumberOp(t, input, identifier, "SELECT * FROM test WHERE a <> ?", value, NeqOp)
}

func testSingleNumberOp(t *testing.T, input, identifier, output string, value interface{}, opType OpType) {
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
