package ast

import (
	"bytes"
	"github.com/biogo/store/llrb"
	"go.etcd.io/bbolt"
)

var EmptyCursor emptyCursor

type emptyCursor struct{}

func (e emptyCursor) Next() {}

func (e emptyCursor) IsValid() bool {
	return false
}

func (e emptyCursor) Current() []byte {
	return nil
}

func (e emptyCursor) Seek([]byte) {}

func NewEmptyCursor() SetCursor {
	return emptyCursor{}
}

func OpenEmptyCursor(_ *bbolt.Tx, _ bool) SetCursor {
	return emptyCursor{}
}

type filteredCursor struct {
	wrapped SetCursor
	filter  func(val []byte) bool
}

func NewFilteredCursor(cursor SetCursor, filter func(val []byte) bool) SetCursor {
	if cursor == nil || !cursor.IsValid() {
		return emptyCursor{}
	}

	result := &filteredCursor{
		wrapped: cursor,
		filter:  filter,
	}

	if !filter(cursor.Current()) {
		result.Next()
	}

	return result
}

func (cursor *filteredCursor) Next() {
	for cursor.wrapped.IsValid() {
		cursor.wrapped.Next()
		if cursor.wrapped.IsValid() {
			if cursor.filter(cursor.wrapped.Current()) {
				return
			}
		}
	}
}

func (cursor *filteredCursor) IsValid() bool {
	return cursor.wrapped.IsValid()
}

func (cursor *filteredCursor) Current() []byte {
	return cursor.wrapped.Current()
}

type byteArrayWrapper interface {
	toBytes() []byte
}

type byteArrayComparable []byte

func (b byteArrayComparable) toBytes() []byte {
	return b
}

func (b byteArrayComparable) Compare(comparable llrb.Comparable) int {
	other, ok := comparable.(byteArrayComparable)
	if !ok {
		return -1
	}
	return bytes.Compare(b, other)
}

type reverseByteArrayComparable []byte

func (b reverseByteArrayComparable) toBytes() []byte {
	return b
}

func (b reverseByteArrayComparable) Compare(comparable llrb.Comparable) int {
	other, ok := comparable.(reverseByteArrayComparable)
	if !ok {
		return -1
	}
	return -bytes.Compare(b, other)
}

func NewTreeSet(forward bool) *TreeSet {
	return &TreeSet{
		tree:    &llrb.Tree{},
		forward: forward,
	}
}

type TreeSet struct {
	tree    *llrb.Tree
	forward bool
}

func (set *TreeSet) Add(val []byte) {
	if set.forward {
		set.tree.Insert(byteArrayComparable(val))
	} else {
		set.tree.Insert(reverseByteArrayComparable(val))
	}
}

func (set *TreeSet) Size() int {
	return set.tree.Len()
}

func (set *TreeSet) ToCursor() SetCursor {
	return NewTreeCursor(set.tree)
}

func NewTreeCursor(tree *llrb.Tree) SetCursor {
	result := &treeCursor{}
	result.next(tree.Root)
	return result
}

type treeCursor struct {
	stack   []*llrb.Node
	current *llrb.Node
}

func (cursor *treeCursor) next(node *llrb.Node) {
	if node.Left != nil {
		cursor.stack = append(cursor.stack, node)
		cursor.next(node.Left)
	} else {
		cursor.current = node
	}
}

func (cursor *treeCursor) Next() {
	if cursor.current.Right != nil {
		cursor.next(cursor.current.Right)
		return
	}

	if len(cursor.stack) > 0 {
		cursor.current = cursor.stack[len(cursor.stack)-1]
		cursor.stack = cursor.stack[0 : len(cursor.stack)-1]
	} else {
		cursor.current = nil
	}
}

func (cursor *treeCursor) IsValid() bool {
	return cursor.current != nil
}

func (cursor *treeCursor) Current() []byte {
	return cursor.current.Elem.(byteArrayWrapper).toBytes()
}

type sliceSetCursor struct {
	values [][]byte
}

func (cursor *sliceSetCursor) Next() {
	if len(cursor.values) > 0 {
		cursor.values = cursor.values[1:]
	}
}

func (cursor *sliceSetCursor) IsValid() bool {
	return len(cursor.values) > 0
}

func (cursor *sliceSetCursor) Current() []byte {
	return cursor.values[0]
}

func NewUnionSetCursor(fst SetCursor, snd SetCursor, forward bool) SetCursor {
	result := &unionSetCursor{
		fst:     fst,
		snd:     snd,
		forward: forward,
	}
	result.Next()
	return result
}

type unionSetCursor struct {
	current []byte
	fst     SetCursor
	snd     SetCursor
	forward bool
}

func (cursor *unionSetCursor) Next() {
	if !cursor.fst.IsValid() {
		if cursor.snd.IsValid() {
			cursor.current = cursor.snd.Current()
			cursor.snd.Next()
		} else {
			cursor.current = nil // end of cursor
		}
		return
	}

	if !cursor.snd.IsValid() {
		cursor.current = cursor.fst.Current()
		cursor.fst.Next()
		return
	}

	cmp := bytes.Compare(cursor.fst.Current(), cursor.snd.Current())
	if cmp == 0 { // if they're the same, advance both and only use one value
		cursor.current = cursor.fst.Current()
		cursor.fst.Next()
		cursor.snd.Next()
	} else if (cmp < 0 && cursor.forward) || (cmp > 0 && !cursor.forward) {
		cursor.current = cursor.fst.Current()
		cursor.fst.Next()
	} else {
		cursor.current = cursor.snd.Current()
		cursor.snd.Next()
	}
}

func (cursor *unionSetCursor) IsValid() bool {
	return cursor.current != nil
}

func (cursor *unionSetCursor) Current() []byte {
	return cursor.current
}
