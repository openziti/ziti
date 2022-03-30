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
	"errors"
	"fmt"
)

func NewNotFoundError(entityType, field, id string) error {
	return &RecordNotFoundError{
		EntityType: entityType,
		Field:      field,
		Id:         id,
	}
}

type RecordNotFoundError struct {
	EntityType string
	Field      string
	Id         string
}

func (err *RecordNotFoundError) Error() string {
	return fmt.Sprintf("%v with %v %v not found", err.EntityType, err.Field, err.Id)
}

var testErrorNotFound = &RecordNotFoundError{}

func IsErrNotFoundErr(err error) bool {
	return errors.As(err, &testErrorNotFound)
}

func NewReferenceByIdsError(localType, localId, remoteType string, remoteIds []string, remoteField string) error {
	return &ReferenceExistsError{
		LocalType:   localType,
		LocalId:     localId,
		RemoteType:  remoteType,
		RemoteIds:   remoteIds,
		RemoteField: remoteField,
	}
}

func NewReferenceByIdError(localType, localId, remoteType, remoteId, remoteField string) error {
	return NewReferenceByIdsError(localType, localId, remoteType, []string{remoteId}, remoteField)
}

var testErrorReferenceExists = &ReferenceExistsError{}

// ReferenceExistsError is an error returned when an operation cannot be completed due to a referential constraint.
// Typically, when deleting an entity (called local) that is referenced by another entity (called the remote)
type ReferenceExistsError struct {
	LocalType   string
	RemoteType  string
	RemoteField string
	LocalId     string
	RemoteIds   []string
}

func IsReferenceExistsError(err error) bool {
	return errors.As(err, &testErrorReferenceExists)
}

func (err *ReferenceExistsError) Error() string {
	return fmt.Sprintf("cannot delete %v with id %v is referenced by %v with id(s) %v, field %v", err.LocalType, err.LocalId, err.RemoteType, err.RemoteIds, err.RemoteField)
}

var testUniqueIndexDuplicateError = &UniqueIndexDuplicateError{}

// UniqueIndexDuplicateError is an error that is returned when a unique index is violated due to duplicate values
type UniqueIndexDuplicateError struct {
	Field      string
	Value      string
	EntityType string
}

func IsUniqueIndexDuplicateError(err error) bool {
	return errors.As(err, &testUniqueIndexDuplicateError)
}

func (err *UniqueIndexDuplicateError) Error() string {
	return fmt.Sprintf("duplicate value '%v' in unique index on %v store", err.Value, err.EntityType)
}
