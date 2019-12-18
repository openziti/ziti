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
	"github.com/netfoundry/ziti-edge/controller/zitiql"
	"github.com/stretchr/testify/assert"
	"gopkg.in/Masterminds/squirrel.v1"
	"testing"
	"time"
)

/*
	Integration level testing of the parsing engine for ZitiQl + ToSquirrelListener for datetime
*/

// ------------------------------------ EQ ---------------------------------------

func TestParse_Single_Equal_Datetime_ZForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "a=datetime(2032-09-03T15:36:50Z)", "a", "2032-09-03T15:36:50Z")
}

func TestParse_Single_Equal_Datetime_ZForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeEqual(t, " a =    \n  datetime(  \t   2032-09-03T15:36:50Z      \n )", "a", "2032-09-03T15:36:50Z")
}

// +00:00 for zulu

func TestParse_Single_Equal_Datetime_PositiveZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "a=datetime(2049-07-09T02:25:09+00:00)", "a", "2049-07-09T02:25:09+00:00")
}

func TestParse_Single_Equal_Datetime_PositiveZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "   a          =   datetime(  \t   2049-07-09T02:25:09+00:00      \n )", "a", "2049-07-09T02:25:09+00:00")
}

// -00:00 for zulu

func TestParse_Single_Equal_Datetime_NegativeZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "a=datetime(2016-04-29T23:32:31-00:00)", "a", "2016-04-29T23:32:31-00:00")
}

func TestParse_Single_Equal_Datetime_NegativeZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeEqual(t, " a = datetime(  \t   2016-04-29T23:32:31-00:00      \n )", "a", "2016-04-29T23:32:31-00:00")
}

// +01:00

func TestParse_Single_Equal_Datetime_PositiveOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "a=datetime(1986-12-26T02:21:12+01:00)", "a", "1986-12-26T02:21:12+01:00")
}

func TestParse_Single_Equal_Datetime_PositiveOne_Whitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "\na\n=\ndatetime(  \t     1986-12-26T02:21:12+01:00    \n )", "a", "1986-12-26T02:21:12+01:00")
}

// -01:00

func TestParse_Single_Equal_Datetime_NegativeOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "a=datetime(1986-12-26T02:21:12-01:00)", "a", "1986-12-26T02:21:12-01:00")
}

func TestParse_Single_Equal_Datetime_NegativeOne_Whitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "\ta\t=\tdatetime(  \t     1986-12-26T02:21:12-01:00    \n )", "a", "1986-12-26T02:21:12-01:00")
}

// +23:00

func TestParse_Single_Equal_Datetime_Positive23_NoWhitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "a=datetime(1988-03-15T03:02:12+23:00)", "a", "1988-03-15T03:02:12+23:00")
}

func TestParse_Single_Equal_Datetime_Positive23_Whitespace(t *testing.T) {
	testSingleDatetimeEqual(t, " a = \ndatetime(  \t     1988-03-15T03:02:12+23:00    \n )", "a", "1988-03-15T03:02:12+23:00")
}

// -23:00

func TestParse_Single_Equal_Datetime_Negative23_NoWhitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "a=datetime(1994-11-28T05:52:58-23:00)", "a", "1994-11-28T05:52:58-23:00")
}

func TestParse_Single_Equal_Datetime_Negative23_Whitespace(t *testing.T) {
	testSingleDatetimeEqual(t, "  a \t  = datetime(  \t     1994-11-28T05:52:58-23:00    \n )", "a", "1994-11-28T05:52:58-23:00")
}

// NEQ

func TestParse_Single_NotEqual_Datetime_ZForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a!=datetime(2032-09-03T15:36:50Z)", "a", "2032-09-03T15:36:50Z")
}

func TestParse_Single_NotEqual_Datetime_ZForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "  a  !=   datetime(  \t   2032-09-03T15:36:50Z      \n )", "a", "2032-09-03T15:36:50Z")
}

// +00:00 for zulu

func TestParse_Single_NotEqual_Datetime_PositiveZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a!=datetime(2049-07-09T02:25:09+00:00)", "a", "2049-07-09T02:25:09+00:00")
}

func TestParse_Single_NotEqual_Datetime_PositiveZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "\na \n\t !=datetime(  \t   2049-07-09T02:25:09+00:00      \n )", "a", "2049-07-09T02:25:09+00:00")
}

// -00:00 for zulu

func TestParse_Single_NotEqual_Datetime_NegativeZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a!=datetime(2016-04-29T23:32:31-00:00)", "a", "2016-04-29T23:32:31-00:00")
}

func TestParse_Single_NotEqual_Datetime_NegativeZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "\n \n a \n != \n datetime(  \t   2016-04-29T23:32:31-00:00      \n )", "a", "2016-04-29T23:32:31-00:00")
}

// +01:00

func TestParse_Single_NotEqual_Datetime_PositiveOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a!=datetime(1986-12-26T02:21:12+01:00)", "a", "1986-12-26T02:21:12+01:00")
}

func TestParse_Single_NotEqual_Datetime_PositiveOne_Whitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "\t a  \t !=  \t datetime(  \t     1986-12-26T02:21:12+01:00    \n )", "a", "1986-12-26T02:21:12+01:00")
}

// -01:00

func TestParse_Single_NotEqual_Datetime_NegativeOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a!=datetime(1986-12-26T02:21:12-01:00)", "a", "1986-12-26T02:21:12-01:00")
}

func TestParse_Single_NotEqual_Datetime_NegativeOne_Whitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a !=  \n datetime(  \t     1986-12-26T02:21:12-01:00    \n )", "a", "1986-12-26T02:21:12-01:00")
}

// +23:00

func TestParse_Single_NotEqual_Datetime_Positive23_NoWhitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a!=datetime(1988-03-15T03:02:12+23:00)", "a", "1988-03-15T03:02:12+23:00")
}

func TestParse_Single_NotEqual_Datetime_Positive23_Whitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a \t != datetime(  \t     1988-03-15T03:02:12+23:00    \n )", "a", "1988-03-15T03:02:12+23:00")
}

// -23:00

func TestParse_Single_NotEqual_Datetime_Negative23_NoWhitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a  !=  datetime(1994-11-28T05:52:58-23:00)", "a", "1994-11-28T05:52:58-23:00")
}

func TestParse_Single_NotEqual_Datetime_Negative23_Whitespace(t *testing.T) {
	testSingleDatetimeNotEqual(t, "a!=datetime(  \t     1994-11-28T05:52:58-23:00    \n )", "a", "1994-11-28T05:52:58-23:00")
}

// GT

func TestParse_Single_GreaterThan_Datetime_ZForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a>datetime(2032-09-03T15:36:50Z)", "a", "2032-09-03T15:36:50Z")
}

func TestParse_Single_GreaterThan_Datetime_ZForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "  a  >   datetime(  \t   2032-09-03T15:36:50Z      \n )", "a", "2032-09-03T15:36:50Z")
}

// +00:00 for zulu

func TestParse_Single_GreaterThan_Datetime_PositiveZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a>datetime(2049-07-09T02:25:09+00:00)", "a", "2049-07-09T02:25:09+00:00")
}

func TestParse_Single_GreaterThan_Datetime_PositiveZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "\na \n\t >datetime(  \t   2049-07-09T02:25:09+00:00      \n )", "a", "2049-07-09T02:25:09+00:00")
}

// -00:00 for zulu

func TestParse_Single_GreaterThan_Datetime_NegativeZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a>datetime(2016-04-29T23:32:31-00:00)", "a", "2016-04-29T23:32:31-00:00")
}

func TestParse_Single_GreaterThan_Datetime_NegativeZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "\n \n a \n > \n datetime(  \t   2016-04-29T23:32:31-00:00      \n )", "a", "2016-04-29T23:32:31-00:00")
}

// +01:00

func TestParse_Single_GreaterThan_Datetime_PositiveOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a>datetime(1986-12-26T02:21:12+01:00)", "a", "1986-12-26T02:21:12+01:00")
}

func TestParse_Single_GreaterThan_Datetime_PositiveOne_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "\t a  \t >  \t datetime(  \t     1986-12-26T02:21:12+01:00    \n )", "a", "1986-12-26T02:21:12+01:00")
}

// -01:00

func TestParse_Single_GreaterThan_Datetime_NegativeOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a>datetime(1986-12-26T02:21:12-01:00)", "a", "1986-12-26T02:21:12-01:00")
}

func TestParse_Single_GreaterThan_Datetime_NegativeOne_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a >  \n datetime(  \t     1986-12-26T02:21:12-01:00    \n )", "a", "1986-12-26T02:21:12-01:00")
}

// +23:00

func TestParse_Single_GreaterThan_Datetime_Positive23_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a>datetime(1988-03-15T03:02:12+23:00)", "a", "1988-03-15T03:02:12+23:00")
}

func TestParse_Single_GreaterThan_Datetime_Positive23_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a \t > datetime(  \t     1988-03-15T03:02:12+23:00    \n )", "a", "1988-03-15T03:02:12+23:00")
}

// -23:00

func TestParse_Single_GreaterThan_Datetime_Negative23_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a  >  datetime(1994-11-28T05:52:58-23:00)", "a", "1994-11-28T05:52:58-23:00")
}

func TestParse_Single_GreaterThan_Datetime_Negative23_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThan(t, "a>datetime(  \t     1994-11-28T05:52:58-23:00    \n )", "a", "1994-11-28T05:52:58-23:00")
}

// GTE

func TestParse_Single_GreaterThanEqual_Datetime_ZForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a>=datetime(2032-09-03T15:36:50Z)", "a", "2032-09-03T15:36:50Z")
}

func TestParse_Single_GreaterThanEqual_Datetime_ZForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "  a  >=   datetime(  \t   2032-09-03T15:36:50Z      \n )", "a", "2032-09-03T15:36:50Z")
}

// +00:00 for zulu

func TestParse_Single_GreaterThanEqual_Datetime_PositiveZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a>=datetime(2049-07-09T02:25:09+00:00)", "a", "2049-07-09T02:25:09+00:00")
}

func TestParse_Single_GreaterThanEqual_Datetime_PositiveZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "\na \n\t >=datetime(  \t   2049-07-09T02:25:09+00:00      \n )", "a", "2049-07-09T02:25:09+00:00")
}

// -00:00 for zulu

func TestParse_Single_GreaterThanEqual_Datetime_NegativeZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a>=datetime(2016-04-29T23:32:31-00:00)", "a", "2016-04-29T23:32:31-00:00")
}

func TestParse_Single_GreaterThanEqual_Datetime_NegativeZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "\n \n a \n >= \n datetime(  \t   2016-04-29T23:32:31-00:00      \n )", "a", "2016-04-29T23:32:31-00:00")
}

// +01:00

func TestParse_Single_GreaterThanEqual_Datetime_PositiveOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a>=datetime(1986-12-26T02:21:12+01:00)", "a", "1986-12-26T02:21:12+01:00")
}

func TestParse_Single_GreaterThanEqual_Datetime_PositiveOne_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "\t a  \t >=  \t datetime(  \t     1986-12-26T02:21:12+01:00    \n )", "a", "1986-12-26T02:21:12+01:00")
}

// -01:00

func TestParse_Single_GreaterThanEqual_Datetime_NegativeOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a>=datetime(1986-12-26T02:21:12-01:00)", "a", "1986-12-26T02:21:12-01:00")
}

func TestParse_Single_GreaterThanEqual_Datetime_NegativeOne_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a >=  \n datetime(  \t     1986-12-26T02:21:12-01:00    \n )", "a", "1986-12-26T02:21:12-01:00")
}

// +23:00

func TestParse_Single_GreaterThanEqual_Datetime_Positive23_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a>=datetime(1988-03-15T03:02:12+23:00)", "a", "1988-03-15T03:02:12+23:00")
}

func TestParse_Single_GreaterThanEqual_Datetime_Positive23_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a \t >= datetime(  \t     1988-03-15T03:02:12+23:00    \n )", "a", "1988-03-15T03:02:12+23:00")
}

// -23:00

func TestParse_Single_GreaterThanEqual_Datetime_Negative23_NoWhitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a  >=  datetime(1994-11-28T05:52:58-23:00)", "a", "1994-11-28T05:52:58-23:00")
}

func TestParse_Single_GreaterThanEqual_Datetime_Negative23_Whitespace(t *testing.T) {
	testSingleDatetimeGreaterThanEqual(t, "a>=datetime(  \t     1994-11-28T05:52:58-23:00    \n )", "a", "1994-11-28T05:52:58-23:00")
}

// LT

func TestParse_Single_LessThan_Datetime_ZForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a<datetime(2032-09-03T15:36:50Z)", "a", "2032-09-03T15:36:50Z")
}

func TestParse_Single_LessThan_Datetime_ZForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "  a  <   datetime(  \t   2032-09-03T15:36:50Z      \n )", "a", "2032-09-03T15:36:50Z")
}

// +00:00 for zulu

func TestParse_Single_LessThan_Datetime_PositiveZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a<datetime(2049-07-09T02:25:09+00:00)", "a", "2049-07-09T02:25:09+00:00")
}

func TestParse_Single_LessThan_Datetime_PositiveZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "\na \n\t <datetime(  \t   2049-07-09T02:25:09+00:00      \n )", "a", "2049-07-09T02:25:09+00:00")
}

// -00:00 for zulu

func TestParse_Single_LessThan_Datetime_NegativeZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a<datetime(2016-04-29T23:32:31-00:00)", "a", "2016-04-29T23:32:31-00:00")
}

func TestParse_Single_LessThan_Datetime_NegativeZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "\n \n a \n < \n datetime(  \t   2016-04-29T23:32:31-00:00      \n )", "a", "2016-04-29T23:32:31-00:00")
}

// +01:00

func TestParse_Single_LessThan_Datetime_PositiveOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a<datetime(1986-12-26T02:21:12+01:00)", "a", "1986-12-26T02:21:12+01:00")
}

func TestParse_Single_LessThan_Datetime_PositiveOne_Whitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "\t a  \t <  \t datetime(  \t     1986-12-26T02:21:12+01:00    \n )", "a", "1986-12-26T02:21:12+01:00")
}

// -01:00

func TestParse_Single_LessThan_Datetime_NegativeOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a<datetime(1986-12-26T02:21:12-01:00)", "a", "1986-12-26T02:21:12-01:00")
}

func TestParse_Single_LessThan_Datetime_NegativeOne_Whitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a <  \n datetime(  \t     1986-12-26T02:21:12-01:00    \n )", "a", "1986-12-26T02:21:12-01:00")
}

// +23:00

func TestParse_Single_LessThan_Datetime_Positive23_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a<datetime(1988-03-15T03:02:12+23:00)", "a", "1988-03-15T03:02:12+23:00")
}

func TestParse_Single_LessThan_Datetime_Positive23_Whitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a \t < datetime(  \t     1988-03-15T03:02:12+23:00    \n )", "a", "1988-03-15T03:02:12+23:00")
}

// -23:00

func TestParse_Single_LessThan_Datetime_Negative23_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a  <  datetime(1994-11-28T05:52:58-23:00)", "a", "1994-11-28T05:52:58-23:00")
}

func TestParse_Single_LessThan_Datetime_Negative23_Whitespace(t *testing.T) {
	testSingleDatetimeLessThan(t, "a<datetime(  \t     1994-11-28T05:52:58-23:00    \n )", "a", "1994-11-28T05:52:58-23:00")
}

// LTE

func TestParse_Single_LessThanEqual_Datetime_ZForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a<=datetime(2032-09-03T15:36:50Z)", "a", "2032-09-03T15:36:50Z")
}

func TestParse_Single_LessThanEqual_Datetime_ZForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "  a  <=   datetime(  \t   2032-09-03T15:36:50Z      \n )", "a", "2032-09-03T15:36:50Z")
}

// +00:00 for zulu

func TestParse_Single_LessThanEqual_Datetime_PositiveZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a<=datetime(2049-07-09T02:25:09+00:00)", "a", "2049-07-09T02:25:09+00:00")
}

func TestParse_Single_LessThanEqual_Datetime_PositiveZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "\na \n\t <=datetime(  \t   2049-07-09T02:25:09+00:00      \n )", "a", "2049-07-09T02:25:09+00:00")
}

// -00:00 for zulu

func TestParse_Single_LessThanEqual_Datetime_NegativeZeroForZulu_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a<=datetime(2016-04-29T23:32:31-00:00)", "a", "2016-04-29T23:32:31-00:00")
}

func TestParse_Single_LessThanEqual_Datetime_NegativeZeroForZulu_Whitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "\n \n a \n <= \n datetime(  \t   2016-04-29T23:32:31-00:00      \n )", "a", "2016-04-29T23:32:31-00:00")
}

// +01:00

func TestParse_Single_LessThanEqual_Datetime_PositiveOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a<=datetime(1986-12-26T02:21:12+01:00)", "a", "1986-12-26T02:21:12+01:00")
}

func TestParse_Single_LessThanEqual_Datetime_PositiveOne_Whitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "\t a  \t <=  \t datetime(  \t     1986-12-26T02:21:12+01:00    \n )", "a", "1986-12-26T02:21:12+01:00")
}

// -01:00

func TestParse_Single_LessThanEqual_Datetime_NegativeOne_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a<=datetime(1986-12-26T02:21:12-01:00)", "a", "1986-12-26T02:21:12-01:00")
}

func TestParse_Single_LessThanEqual_Datetime_NegativeOne_Whitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a <=  \n datetime(  \t     1986-12-26T02:21:12-01:00    \n )", "a", "1986-12-26T02:21:12-01:00")
}

// +23:00

func TestParse_Single_LessThanEqual_Datetime_Positive23_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a<=datetime(1988-03-15T03:02:12+23:00)", "a", "1988-03-15T03:02:12+23:00")
}

func TestParse_Single_LessThanEqual_Datetime_Positive23_Whitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a \t <= datetime(  \t     1988-03-15T03:02:12+23:00    \n )", "a", "1988-03-15T03:02:12+23:00")
}

// -23:00

func TestParse_Single_LessThanEqual_Datetime_Negative23_NoWhitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a  <=  datetime(1994-11-28T05:52:58-23:00)", "a", "1994-11-28T05:52:58-23:00")
}

func TestParse_Single_LessThanEqual_Datetime_Negative23_Whitespace(t *testing.T) {
	testSingleDatetimeLessThanEqual(t, "a<=datetime(  \t     1994-11-28T05:52:58-23:00    \n )", "a", "1994-11-28T05:52:58-23:00")
}

// ------------------------------------ IN ARRAY ---------------------------------------
func TestParse_Single_InArray_Datetime_OneZForZulu(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse("a in [datetime(1986-12-26t02:21:12z)]", listener)

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

	value, err := time.Parse(time.RFC3339, "1986-12-26T02:21:12Z")
	assert.Nil(t, err)

	assert.Equal(t, 1, len(args))
	assert.Equal(t, value, args[0])

}

func TestParse_Single_InArray_Datetime_Whitespace(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse("a in [\n\t  datetime(1986-12-26t02:21:12z)   ,datetime(2049-07-09T02:25:09+00:00),   datetime(1986-12-26T02:21:12+01:00) ]", listener)

	assert.Len(t, pe, 0)

	sql, args, err := squirrel.Select("*").From("test").Where(listener.Predicate).ToSql()

	assert.Equal(t, nil, err)
	assert.Equal(t, "SELECT * FROM test WHERE a IN (?,?,?)", sql)

	opSet, ok := listener.IdentifierOps["a"]
	assert.True(t, ok)
	assert.NotNil(t, opSet)

	op, ok := opSet[InOp]
	assert.True(t, ok)
	assert.True(t, op)

	value1, err := time.Parse(time.RFC3339, "1986-12-26T02:21:12Z")
	assert.Nil(t, err)

	value2, err := time.Parse(time.RFC3339, "2049-07-09T02:25:09+00:00")
	assert.Nil(t, err)

	value3, err := time.Parse(time.RFC3339, "1986-12-26T02:21:12+01:00")
	assert.Nil(t, err)

	assert.Equal(t, 3, len(args))
	assert.Equal(t, value1, args[0])
	assert.Equal(t, value2, args[1])
	assert.Equal(t, value3, args[2])
}

// ------------------------------------ BETWEEN  ---------------------------------------

func TestParse_Single_Between_Datetime(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse("a BETWEEN datetime(1986-12-26t02:21:12z) and datetime(2049-07-09T02:25:09+00:00)", listener)

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

	value1, err := time.Parse(time.RFC3339, "1986-12-26T02:21:12Z")
	assert.Nil(t, err)

	value2, err := time.Parse(time.RFC3339, "2049-07-09T02:25:09+00:00")
	assert.Nil(t, err)

	assert.Equal(t, 2, len(args))
	assert.Equal(t, value1, args[0])
	assert.Equal(t, value2, args[1])
}

func TestParse_Single_Between_Number_Whitespace(t *testing.T) {
	listener := NewSquirrelListener()
	pe := zitiql.Parse("a BETWEEN \n\t  datetime(1986-12-26t02:21:12z)  \t \n and \n \t  datetime(2049-07-09T02:25:09+00:00) \n", listener)

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

	value1, err := time.Parse(time.RFC3339, "1986-12-26T02:21:12Z")
	assert.Nil(t, err)

	value2, err := time.Parse(time.RFC3339, "2049-07-09T02:25:09+00:00")
	assert.Nil(t, err)

	assert.Equal(t, 2, len(args))
	assert.Equal(t, value1, args[0])
	assert.Equal(t, value2, args[1])
}

// Helpers

func testSingleDatetimeLessThan(t *testing.T, input, identifier, strdate string) {
	value, err := time.Parse(time.RFC3339, strdate)
	assert.Nil(t, err)

	testSingleDatetimeOp(t, input, identifier, "SELECT * FROM test WHERE a < ?", value, LtOp)
}

func testSingleDatetimeLessThanEqual(t *testing.T, input, identifier, strdate string) {
	value, err := time.Parse(time.RFC3339, strdate)
	assert.Nil(t, err)

	testSingleDatetimeOp(t, input, identifier, "SELECT * FROM test WHERE a <= ?", value, LtEOp)
}

func testSingleDatetimeGreaterThan(t *testing.T, input, identifier, strdate string) {
	value, err := time.Parse(time.RFC3339, strdate)
	assert.Nil(t, err)

	testSingleDatetimeOp(t, input, identifier, "SELECT * FROM test WHERE a > ?", value, GtOp)
}

func testSingleDatetimeGreaterThanEqual(t *testing.T, input, identifier, strdate string) {
	value, err := time.Parse(time.RFC3339, strdate)
	assert.Nil(t, err)

	testSingleDatetimeOp(t, input, identifier, "SELECT * FROM test WHERE a >= ?", value, GtEOp)
}

func testSingleDatetimeEqual(t *testing.T, input, identifier, strdate string) {
	value, err := time.Parse(time.RFC3339, strdate)
	assert.Nil(t, err)

	testSingleDatetimeOp(t, input, identifier, "SELECT * FROM test WHERE a = ?", value, EqOp)
}

func testSingleDatetimeNotEqual(t *testing.T, input, identifier, strdate string) {
	value, err := time.Parse(time.RFC3339, strdate)
	assert.Nil(t, err)

	testSingleDatetimeOp(t, input, identifier, "SELECT * FROM test WHERE a <> ?", value, NeqOp)
}

func testSingleDatetimeOp(t *testing.T, input, identifier, output string, value interface{}, opType OpType) {
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
