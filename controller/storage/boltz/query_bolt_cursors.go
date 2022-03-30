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

import (
	"bytes"
	"github.com/openziti/storage/ast"
	"go.etcd.io/bbolt"
)

type BaseBoltCursor struct {
	cursor *bbolt.Cursor
	key    []byte
}

func (f *BaseBoltCursor) IsValid() bool {
	return f.key != nil
}

func (f *BaseBoltCursor) Current() []byte {
	return f.key
}

func NewBoltCursor(cursor *bbolt.Cursor, forward bool) ast.SeekableSetCursor {
	if forward {
		return NewForwardBoltCursor(cursor)
	}
	return NewReverseBoltCursor(cursor)
}

func NewForwardBoltCursor(cursor *bbolt.Cursor) ast.SeekableSetCursor {
	result := &ForwardBoltCursor{BaseBoltCursor{
		cursor: cursor,
		key:    nil,
	}}
	result.key, _ = result.cursor.First()
	return result
}

type ForwardBoltCursor struct {
	BaseBoltCursor
}

func (f *ForwardBoltCursor) Next() {
	f.key, _ = f.cursor.Next()
}

func (f *ForwardBoltCursor) Seek(val []byte) {
	f.key, _ = f.cursor.Seek(val)
}

func NewReverseBoltCursor(cursor *bbolt.Cursor) ast.SeekableSetCursor {
	result := &ReverseBoltCursor{BaseBoltCursor{
		cursor: cursor,
		key:    nil,
	}}
	result.key, _ = result.cursor.Last()
	return result
}

type ReverseBoltCursor struct {
	BaseBoltCursor
}

func (f *ReverseBoltCursor) Next() {
	f.key, _ = f.cursor.Prev()
}

func (f *ReverseBoltCursor) Seek(val []byte) {
	f.key, _ = f.cursor.Seek(val)
	if !bytes.Equal(val, f.key) {
		f.key, _ = f.cursor.Prev()
	}
}

func NewTypedForwardBoltCursor(cursor *bbolt.Cursor, fieldType FieldType) ast.SeekableSetCursor {
	result := &TypedForwardBoltCursor{
		BaseBoltCursor: BaseBoltCursor{
			cursor: cursor,
			key:    nil,
		},
		fieldType: fieldType,
	}

	key, _ := result.cursor.First()
	_, result.key = GetTypeAndValue(key)

	return result
}

type TypedForwardBoltCursor struct {
	BaseBoltCursor
	fieldType FieldType
}

func (f *TypedForwardBoltCursor) Next() {
	key, _ := f.cursor.Next()
	_, f.key = GetTypeAndValue(key)
}

func (f *TypedForwardBoltCursor) Seek(val []byte) {
	searchVal := PrependFieldType(f.fieldType, val)
	key, _ := f.cursor.Seek(searchVal)
	_, f.key = GetTypeAndValue(key)
}

func NewTypedReverseBoltCursor(cursor *bbolt.Cursor, fieldType FieldType) ast.SeekableSetCursor {
	result := &TypedReverseBoltCursor{
		BaseBoltCursor: BaseBoltCursor{
			cursor: cursor,
			key:    nil,
		},
		fieldType: fieldType,
	}

	key, _ := result.cursor.Last()
	_, result.key = GetTypeAndValue(key)

	return result
}

type TypedReverseBoltCursor struct {
	BaseBoltCursor
	fieldType FieldType
}

func (f *TypedReverseBoltCursor) Next() {
	key, _ := f.cursor.Prev()
	_, f.key = GetTypeAndValue(key)
}

func (f *TypedReverseBoltCursor) Seek(val []byte) {
	searchVal := PrependFieldType(f.fieldType, val)
	f.key, _ = f.cursor.Seek(searchVal)
	if !bytes.Equal(searchVal, f.key) {
		f.Next()
	}
}
