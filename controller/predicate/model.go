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
	"github.com/michaelquigley/pfxlog"
	"gopkg.in/Masterminds/squirrel.v1"
)

type ConjType string

const (
	AndConj ConjType = "AND"
	OrConj  ConjType = "OR"
)

type ResolverType string

const (
	ConjResolver  ResolverType = "CONJ"
	GroupResolver ResolverType = "GROUP"
	OpResolver    ResolverType = "OP"
)

type OpType string

const (
	EqOp          OpType = "="
	NeqOp         OpType = "!="
	BetweenOp     OpType = "BETWEEN"
	NotBetweenOp  OpType = "NOT BETWEEN"
	InOp          OpType = "IN"
	NotInOp       OpType = "NOT IN"
	ContainsOp    OpType = "CONTAINS"
	NotContainsOp OpType = "NOT CONTAINS"
	LtOp          OpType = "<"
	LtEOp         OpType = "<="
	GtOp          OpType = ">"
	GtEOp         OpType = ">="
)

type ValueType string

const (
	StringType        ValueType = "STRING"
	NumberType        ValueType = "NUMBER"
	DatetimeType      ValueType = "DATETIME"
	BoolType          ValueType = "BOOL"
	NullType          ValueType = "NULL"
	StringArrayType   ValueType = "STRING_ARRAY"
	NumberArrayType   ValueType = "NUMBER_ARRAY"
	DatetimeArrayType ValueType = "DATETIME_ARRAY"
)

type OpSet map[OpType]bool

func (os *OpSet) Merge(os2 *OpSet) {
	for k, v := range *os2 {
		(*os)[k] = v
	}
}

type IdentifierOps map[string]OpSet

func (io *IdentifierOps) Merge(io2 *IdentifierOps) {
	for k, v := range *io2 {
		if g, found := (*io)[k]; found {
			g.Merge(&v)
		} else {
			(*io)[k] = v
		}
	}
}

type IdentifierTranslations map[string]string

type resolver interface {
	Resolve() (squirrel.Sqlizer, IdentifierOps)
	ResolverType() ResolverType
}

type conj interface {
	Resolve() (squirrel.Sqlizer, IdentifierOps)
	ResolverType() ResolverType
	ConjType() ConjType
	Append(r resolver)
}

type group struct {
	Parent       *group
	conjugations []conj
	operations   []resolver
	resolverType ResolverType
}

func newGroup(parent *group) *group {
	return &group{
		Parent:       parent,
		conjugations: []conj{},
		operations:   []resolver{},
		resolverType: GroupResolver,
	}
}

func (g *group) Resolve() (squirrel.Sqlizer, IdentifierOps) {
	if len(g.conjugations) == 0 {

		if len(g.operations) == 0 {
			return nil, nil
		}

		switch len(g.operations) {
		case 1:
			return g.operations[0].Resolve()
		default:
			pfxlog.Logger().Panic("Invalid number of operations")
		}
	}

	var root conj

	ops := g.operations

	for i, curConj := range g.conjugations {
		if i == 0 {
			root = curConj
		}

		// One or in a group causes the entire root clause to swap to an OR
		if root.ConjType() != OrConj && curConj.ConjType() == OrConj {
			curConj.Append(root)
			root = curConj
		}

		var prevConj conj

		if i > 0 {
			prevConj = g.conjugations[i-1]
		}

		if len(ops) == 0 {
			break
		}

		var r resolver
		switch {
		case prevConj == nil && curConj.ConjType() == AndConj:
			//First ANDs consume 2 ops
			r, ops = ops[0], ops[1:]
			curConj.Append(r)

			if len(ops) == 0 {
				break
			}

			r, ops = ops[0], ops[1:]
			curConj.Append(r)

		case prevConj == nil && curConj.ConjType() == OrConj:
			//First ORs consume 1 op
			r, ops = ops[0], ops[1:]
			curConj.Append(r)

			//..unless there are no other conjs e.g. (a=1 or b=2)
			onLastConj := i >= len(g.conjugations)-1
			if onLastConj || g.conjugations[i+1].ConjType() == OrConj {
				r, ops = ops[0], ops[1:]
				root.Append(r)
			}

		case prevConj.ConjType() == AndConj && curConj.ConjType() == AndConj:
			// serial ANDs after the first consume 1 op (e.g a=1 AND b=3 AND c=3)
			r, ops = ops[0], ops[1:]
			prevConj.Append(r)

		case prevConj.ConjType() == OrConj && curConj.ConjType() == AndConj:
			// swapping from OR to AND means this is the new first AND of potentially serial ANDs, consume 2 ops
			r, ops = ops[0], ops[1:]
			curConj.Append(r)

			if len(ops) == 0 {
				break
			}

			r, ops = ops[0], ops[1:]
			curConj.Append(r)

			root.Append(curConj)

		case curConj.ConjType() == OrConj:
			// ors only consume tokens if they are the last conj, always append ors to root OR (1 or makes root an OR)
			onLastConj := i >= len(g.conjugations)-1
			if len(ops) > 0 && (onLastConj || g.conjugations[i+1].ConjType() == OrConj) {
				r, ops = ops[0], ops[1:]
				root.Append(r)
			}

			//only append if we are also not root
			if root != curConj {
				root.Append(curConj)
			}
		default:

			panic("unhandled case")
		}

	}

	if len(ops) != 0 {
		panic("ops left behind")
	}

	return root.Resolve()
}

func (g *group) ResolverType() ResolverType {
	return g.resolverType
}

type op struct {
	op           squirrel.Sqlizer
	resolverType ResolverType
	identifier   string
	value        interface{}
	opType       OpType
	valueType    ValueType
}

func newOp(o squirrel.Sqlizer, identifier string, opType OpType, value interface{}, valueType ValueType) resolver {
	return &op{
		op:           o,
		resolverType: OpResolver,
		identifier:   identifier,
		opType:       opType,
		value:        value,
		valueType:    valueType,
	}
}

func (o *op) Resolve() (squirrel.Sqlizer, IdentifierOps) {
	i := IdentifierOps{
		o.identifier: OpSet{
			o.opType: true,
		},
	}

	return o.op, i
}

func (o *op) ResolverType() ResolverType {
	return o.resolverType
}

type and struct {
	resolverType ResolverType
	conjType     ConjType
	resolvers    []resolver
}

func newAnd() *and {
	return &and{
		resolverType: ConjResolver,
		conjType:     AndConj,
		resolvers:    []resolver{},
	}
}

func (a *and) Resolve() (squirrel.Sqlizer, IdentifierOps) {
	sos := []squirrel.Sqlizer{}
	io := IdentifierOps{}
	for _, r := range a.resolvers {
		so, io2 := r.Resolve()
		sos = append(sos, so)
		io.Merge(&io2)
	}
	return squirrel.And(sos), io
}

func (a *and) ResolverType() ResolverType {
	return a.resolverType
}

func (a *and) ConjType() ConjType {
	return a.conjType
}

func (a *and) Append(op resolver) {
	a.resolvers = append(a.resolvers, op)
}

type or struct {
	resolverType ResolverType
	conjType     ConjType
	resolvers    []resolver
}

func newOr() *or {
	return &or{
		resolverType: ConjResolver,
		conjType:     OrConj,
		resolvers:    []resolver{},
	}
}

func (o *or) Resolve() (squirrel.Sqlizer, IdentifierOps) {
	sos := []squirrel.Sqlizer{}
	io := IdentifierOps{}
	for _, r := range o.resolvers {
		so, io2 := r.Resolve()
		sos = append(sos, so)
		io.Merge(&io2)
	}
	return squirrel.Or(sos), io
}

func (o *or) ResolverType() ResolverType {
	return ConjResolver
}

func (o *or) ConjType() ConjType {
	return o.conjType
}

func (o *or) Append(op resolver) {
	o.resolvers = append(o.resolvers, op)
}
