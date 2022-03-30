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

package boltz

type rowComparatorImpl struct {
	symbols []symbolComparator
}

func (rc *rowComparatorImpl) Compare(row1, row2 RowCursor) int {
	result := 0
	for _, symbol := range rc.symbols {
		result = symbol.Compare(row1, row2)
		if result != 0 {
			return result
		}
	}
	return result
}

type symbolComparator interface {
	Compare(row1, row2 RowCursor) int
}

type stringSymbolComparator struct {
	symbol  EntitySymbol
	forward bool
}

func (c *stringSymbolComparator) Compare(row1, row2 RowCursor) int {
	s1 := FieldToString(c.symbol.Eval(row1.Tx(), row1.CurrentRow()))
	s2 := FieldToString(c.symbol.Eval(row2.Tx(), row2.CurrentRow()))

	result := 0
	if s1 == nil {
		if s2 != nil {
			result = -1
		}
	} else if s2 == nil {
		result = 1
	} else if *s1 < *s2 {
		result = -1
	} else if *s1 > *s2 {
		result = 1
	}

	if c.forward {
		return result
	}
	return -result
}

type int64SymbolComparator struct {
	symbol  EntitySymbol
	forward bool
}

func (c *int64SymbolComparator) Compare(row1, row2 RowCursor) int {
	s1 := FieldToInt64(c.symbol.Eval(row1.Tx(), row1.CurrentRow()))
	s2 := FieldToInt64(c.symbol.Eval(row2.Tx(), row2.CurrentRow()))

	result := 0
	if s1 == nil {
		if s2 != nil {
			result = -1
		}
	} else if s2 == nil {
		result = 1
	} else if *s1 < *s2 {
		result = -1
	} else if *s1 > *s2 {
		result = 1
	}

	if c.forward {
		return result
	}
	return -result
}

type float64SymbolComparator struct {
	symbol  EntitySymbol
	forward bool
}

func (c *float64SymbolComparator) Compare(row1, row2 RowCursor) int {
	s1 := FieldToFloat64(c.symbol.Eval(row1.Tx(), row1.CurrentRow()))
	s2 := FieldToFloat64(c.symbol.Eval(row2.Tx(), row2.CurrentRow()))

	result := 0
	if s1 == nil {
		if s2 != nil {
			result = -1
		}
	} else if s2 == nil {
		result = 1
	} else if *s1 < *s2 {
		result = -1
	} else if *s1 > *s2 {
		result = 1
	}

	if c.forward {
		return result
	}
	return -result
}

type datetimeSymbolComparator struct {
	symbol  EntitySymbol
	forward bool
}

func (c *datetimeSymbolComparator) Compare(row1, row2 RowCursor) int {
	field1, val1 := c.symbol.Eval(row1.Tx(), row1.CurrentRow())
	field2, val2 := c.symbol.Eval(row2.Tx(), row2.CurrentRow())
	s1 := FieldToDatetime(field1, val1, c.symbol.GetName())
	s2 := FieldToDatetime(field2, val2, c.symbol.GetName())

	result := 0
	if s1 == nil {
		if s2 != nil {
			result = -1
		}
	} else if s2 == nil {
		result = 1
	} else if s1.Before(*s2) {
		result = -1
	} else if s1.After(*s2) {
		result = 1
	}

	if c.forward {
		return result
	}
	return -result
}

type boolSymbolComparator struct {
	symbol  EntitySymbol
	forward bool
}

func (c *boolSymbolComparator) Compare(row1, row2 RowCursor) int {
	s1 := FieldToBool(c.symbol.Eval(row1.Tx(), row1.CurrentRow()))
	s2 := FieldToBool(c.symbol.Eval(row2.Tx(), row2.CurrentRow()))

	result := 0
	if s1 == nil {
		if s2 != nil {
			result = -1
		}
	} else if s2 == nil {
		result = 1
	} else if !*s1 && *s2 {
		result = -1
	} else if *s1 && !*s2 {
		result = 1
	}

	if c.forward {
		return result
	}
	return -result
}
