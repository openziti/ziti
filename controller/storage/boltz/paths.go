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
	"fmt"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"strings"
)

func Path(tx *bbolt.Tx, path ...string) *TypedBucket {
	if len(path) == 0 {
		return nil
	}
	bucket := tx.Bucket([]byte(path[0]))
	if bucket == nil {
		return nil
	}
	typeBucket := newRootTypedBucket(bucket)
	return typeBucket.GetPath(path[1:]...)
}

func GetOrCreatePath(tx *bbolt.Tx, path ...string) *TypedBucket {
	if len(path) == 0 {
		return ErrBucket(errors.New("No path provided"))
	}
	name := []byte(path[0])
	bucket := tx.Bucket(name)
	if bucket == nil {
		var err error
		bucket, err = tx.CreateBucket(name)
		if err != nil {
			return ErrBucket(err)
		}
	}
	typeBucket := newRootTypedBucket(bucket)
	return typeBucket.GetOrCreatePath(path[1:]...)
}

type BoltVisitor interface {
	VisitBucket(path string, key []byte, bucket *bbolt.Bucket) bool
	VisitKeyValue(path string, key, value []byte) bool
}

type boltTraverseEntry struct {
	Traversable
	path string
}

type loggingTraverseVisitor struct {
}

func (visitor loggingTraverseVisitor) VisitBucket(path string, key []byte, _ *bbolt.Bucket) bool {
	fmt.Printf("%v/%v\n", path, string(key))
	return true
}

func (visitor loggingTraverseVisitor) VisitKeyValue(path string, key, value []byte) bool {
	fieldType, fieldValue := GetTypeAndValue(value)
	strVal := FieldToString(fieldType, fieldValue)
	if strVal == nil {
		fmt.Printf("%v/%v = nil\n", path, string(key))
	} else {
		fmt.Printf("%v/%v = %v\n", path, string(key), *strVal)
	}
	return true
}

func DumpBoltDb(tx *bbolt.Tx) {
	Traverse(tx, "", loggingTraverseVisitor{})
}

func DumpBucket(tx *bbolt.Tx, path ...string) {
	bucket := Path(tx, path...)
	if bucket != nil {
		Traverse(bucket.Bucket, "/"+strings.Join(path, "/"), loggingTraverseVisitor{})
	}
}

type Traversable interface {
	Cursor() *bbolt.Cursor
	Bucket(key []byte) *bbolt.Bucket
}

func Traverse(traversable Traversable, basePath string, visitor BoltVisitor) {
	var queue []*boltTraverseEntry
	queue = append(queue, &boltTraverseEntry{
		Traversable: traversable,
		path:        basePath,
	})
	for len(queue) > 0 {
		entry := queue[0]
		cursor := entry.Cursor()
		queue = queue[1:]
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			if value == nil {
				childBucket := entry.Bucket(key)
				if childBucket == nil {
					if !visitor.VisitKeyValue(entry.path, key, nil) {
						return
					}
				} else {
					if !visitor.VisitBucket(entry.path, key, childBucket) {
						return
					}
					queue = append(queue, &boltTraverseEntry{
						Traversable: childBucket,
						path:        entry.path + "/" + string(key),
					})
				}
			} else if !visitor.VisitKeyValue(entry.path, key, value) {
				return
			}
		}
	}
}

func ValidateDeleted(tx *bbolt.Tx, id string, ignorePaths ...string) error {
	visitor := &deletedIdScanner{
		key:         []byte(id),
		fieldAndKey: PrependFieldType(TypeString, []byte(id)),
		ignorePaths: ignorePaths,
	}
	Traverse(tx, "", visitor)
	return visitor.err
}

type deletedIdScanner struct {
	key         []byte
	fieldAndKey []byte
	err         error
	ignorePaths []string
}

func (visitor *deletedIdScanner) VisitBucket(path string, key []byte, _ *bbolt.Bucket) bool {
	if bytes.Equal(visitor.key, key) {
		visitor.err = errors.Errorf("found id %v as key under path %v", string(visitor.key), path)
		return false
	}
	if bytes.Equal(visitor.fieldAndKey, key) {
		visitor.err = errors.Errorf("found field encoded key %v as key under path %v", string(visitor.key), path)
		return false
	}
	return true
}

func (visitor *deletedIdScanner) VisitKeyValue(path string, key, value []byte) bool {
	for _, ignorePath := range visitor.ignorePaths {
		if strings.HasPrefix(path, ignorePath) {
			return true
		}
	}

	if !visitor.VisitBucket(path, key, nil) {
		return false
	}
	if bytes.Equal(visitor.key, value) {
		visitor.err = errors.Errorf("found id %v as value under path %v/%v", string(visitor.key), path, string(key))
		return false
	}
	if bytes.Equal(visitor.fieldAndKey, value) {
		visitor.err = errors.Errorf("found field encoded key %v as key under path %v/%v", string(visitor.key), path, string(key))
		return false
	}
	return true
}
