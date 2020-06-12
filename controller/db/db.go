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

package db

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"go.etcd.io/bbolt"
	"os"
	"time"
)

type Db struct {
	db *bbolt.DB
}

func Open(path string) (*Db, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to open controller database [%s] (%s)", path, err)
	}
	if err := db.Update(createRoots); err != nil {
		return nil, err
	}
	return &Db{db: db}, nil
}

func (db *Db) Close() error {
	return db.db.Close()
}

func (db *Db) Update(fn func(tx *bbolt.Tx) error) error {
	return db.db.Update(fn)
}

func (db *Db) View(fn func(tx *bbolt.Tx) error) error {
	return db.db.View(fn)
}

func (db *Db) RootBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	ziti := tx.Bucket([]byte("ziti"))
	if ziti == nil {
		return nil, errors.New("db missing 'ziti' root")
	}
	return ziti, nil
}

func (db *Db) Snapshot(tx *bbolt.Tx) error {
	path := db.db.Path()
	path += "-" + time.Now().Format("20060102-150405")

	_, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		pfxlog.Logger().Infof("bolt db backup already made: %v", path)
		return nil
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err = file.Close(); err != nil {
			pfxlog.Logger().Errorf("failed to close backup database file %v (%v)", path, err)
		}
	}()

	_, err = tx.WriteTo(file)
	if err != nil {
		pfxlog.Logger().Infof("created bolt db backup: %v", path)
	}
	return err
}

func createRoots(tx *bbolt.Tx) error {
	if ziti, err := tx.CreateBucketIfNotExists([]byte("ziti")); err == nil {
		if _, err := ziti.CreateBucketIfNotExists([]byte("services")); err != nil {
			return err
		}
		if _, err := ziti.CreateBucketIfNotExists([]byte("routers")); err != nil {
			return err
		}
	}
	return nil
}
