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

type objectComparator[T any] interface {
	compare(a, b T) int
}

type compoundObjectComparator[T any] struct {
	comparators []objectComparator[T]
}

//lint:ignore U1000 Ignore unused function as this is a false positive in staticcheck
func (self *compoundObjectComparator[T]) compare(a, b T) int {
	result := 0
	for _, symbol := range self.comparators {
		result = symbol.compare(a, b)
		if result != 0 {
			return result
		}
	}
	return result
}

type objectBoolSymbolComparator[T any] struct {
	symbol  *ObjectBoolSymbol[T]
	forward bool
}

//lint:ignore U1000 Ignore unused function as this is a false positive in staticcheck
func (c *objectBoolSymbolComparator[T]) compare(a, b T) int {
	s1 := c.symbol.EvalBool(a)
	s2 := c.symbol.EvalBool(b)

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

type objectStringSymbolComparator[T any] struct {
	symbol  *ObjectStringSymbol[T]
	forward bool
}

//lint:ignore U1000 Ignore unused function as this is a false positive in staticcheck
func (c *objectStringSymbolComparator[T]) compare(a, b T) int {
	s1 := c.symbol.EvalString(a)
	s2 := c.symbol.EvalString(b)

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

type objectInt64SymbolComparator[T any] struct {
	symbol  *ObjectInt64Symbol[T]
	forward bool
}

//lint:ignore U1000 Ignore unused function as this is a false positive in staticcheck
func (c *objectInt64SymbolComparator[T]) compare(a, b T) int {
	s1 := c.symbol.EvalInt64(a)
	s2 := c.symbol.EvalInt64(b)

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

type objectFloat64SymbolComparator[T any] struct {
	symbol  *ObjectFloat64Symbol[T]
	forward bool
}

//lint:ignore U1000 Ignore unused function as this is a false positive in staticcheck
func (c *objectFloat64SymbolComparator[T]) compare(a, b T) int {
	s1 := c.symbol.EvalFloat64(a)
	s2 := c.symbol.EvalFloat64(b)

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

type objectDatetimeSymbolComparator[T any] struct {
	symbol  *ObjectDatetimeSymbol[T]
	forward bool
}

//lint:ignore U1000 Ignore unused function as this is a false positive in staticcheck
func (c *objectDatetimeSymbolComparator[T]) compare(a, b T) int {
	s1 := c.symbol.EvalDatetime(a)
	s2 := c.symbol.EvalDatetime(b)

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
