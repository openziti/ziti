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

package migration

import (
	"fmt"
	"github.com/jinzhu/gorm"
)

type QueryResult struct {
	Value          interface{}
	Error          error
	RowsAffected   int64
	RecordNotFound bool
}

func NewQueryResult(db *gorm.DB) *QueryResult {
	qr := &QueryResult{}

	if db != nil {
		qr.Error = db.Error
		qr.Value = db.Value
		qr.RowsAffected = db.RowsAffected
	}

	if qr.Error != nil && gorm.IsRecordNotFoundError(qr.Error) {
		qr.Error = &RecordNotFoundError{}
	}

	return qr
}

func NewQueryResultFromError(err error) *QueryResult {
	return &QueryResult{
		Error: err,
	}
}

func NewQueryResultF(err string, args ...interface{}) *QueryResult {
	return NewQueryResultFromError(fmt.Errorf(err, args...))
}
