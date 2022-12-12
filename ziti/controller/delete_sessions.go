/*
	Copyright NetFoundry Inc.

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

package controller

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller"
	"github.com/openziti/ziti/common/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

func NewDeleteSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete-sessions <config>",
		Short: "Delete all API Sessions and Edge Sessions, controller must be shutdown",
		Args:  cobra.ExactArgs(1),
		Run:   deleteSessions,
	}
}

const (
	ApiSessionBucketName = "apiSessions"
	SessionBucketName    = "sessions"
	RootBucketName       = "ziti"

	IndexBucketName = "indexes"
)

func deleteSessions(_ *cobra.Command, args []string) {
	logrus.WithField("version", version.GetVersion()).
		WithField("go-version", version.GetGoVersion()).
		WithField("os", version.GetOS()).
		WithField("arch", version.GetArchitecture()).
		WithField("build-date", version.GetBuildDate()).
		WithField("revision", version.GetRevision()).
		Info("removing API Sessions and Edge Sessions from ziti-controller")

	if config, err := controller.LoadConfig(args[0]); err == nil {

		apiSessionBucketExists := false
		sessionBucketExists := false

		apiSessionTokenBucketExists := false
		sessionTokenBucketExists := false

		logger := pfxlog.Logger()

		defer func() {
			_ = config.Db.Close()
		}()

		err = config.Db.View(func(tx *bbolt.Tx) error {
			root := tx.Bucket([]byte(RootBucketName))

			if root == nil {
				return errors.New("root 'ziti' bucket not found")
			}

			apiSessionBucket := root.Bucket([]byte(ApiSessionBucketName))

			if apiSessionBucket == nil {
				logger.Info("api Session bucket does not exist, skipping, count is: 0")
			} else {
				apiSessionBucketExists = true
				count := 0
				_ = apiSessionBucket.ForEach(func(_, _ []byte) error {
					count++
					return nil
				})
				logger.Infof("existing api Sessions: %v", count)
			}

			sessionBucket := root.Bucket([]byte(SessionBucketName))

			if sessionBucket == nil {
				logger.Print("edge sessions bucket does not exist, skipping, count is: 0")
			} else {
				sessionBucketExists = true
				count := 0
				_ = sessionBucket.ForEach(func(_, _ []byte) error {
					count++
					return nil
				})

				logger.Infof("existing edge Sessions: %v", count)
			}

			indexBucket := root.Bucket([]byte(IndexBucketName))

			if indexBucket == nil {
				logger.Info("ziti index bucket does not exist, skipping indexes")
			} else {
				apiSessionTokenBucket := indexBucket.Bucket([]byte(ApiSessionBucketName))

				if apiSessionTokenBucket == nil {
					logger.Print("api sessions index bucket does not exist, skipping")
				} else {
					apiSessionTokenBucketExists = true
				}

				sessionTokenBucket := indexBucket.Bucket([]byte(SessionBucketName))

				if sessionTokenBucket == nil {
					logger.Print("edge sessions index bucket does not exist, skipping")
				} else {
					sessionTokenBucketExists = true
				}
			}

			return nil
		})

		if err != nil {
			pfxlog.Logger().Errorf("could not read databse stats: %v", err)
		}

		_ = config.Db.Update(func(tx *bbolt.Tx) error {

			root := tx.Bucket([]byte("ziti"))

			if root == nil {
				return errors.New("root 'ziti' bucket not found")
			}

			if apiSessionBucketExists {
				if err := root.DeleteBucket([]byte(ApiSessionBucketName)); err != nil {
					logger.Infof("could not delete apiSessions: %v", err)
				} else {
					logger.Infof("done removing api Sessions")
				}
			}

			if sessionBucketExists {
				if err := root.DeleteBucket([]byte(SessionBucketName)); err != nil {
					logger.Infof("could not delete sessions: %v", err)
				} else {
					logger.Infof("done removing Edge Sessions")
				}
			}

			indexBucket := root.Bucket([]byte(IndexBucketName))

			if apiSessionTokenBucketExists {
				if err := indexBucket.DeleteBucket([]byte(ApiSessionBucketName)); err != nil {
					logger.Infof("could not delete api session indexes: %v", err)
				} else {
					logger.Infof("done removing api session indexes")
				}
			}

			if sessionTokenBucketExists {
				if err := indexBucket.DeleteBucket([]byte(SessionBucketName)); err != nil {
					logger.Infof("could not delete edge session indexes: %v", err)
				} else {
					logger.Infof("done removing edge session indexes")
				}
			}

			return nil
		})

	} else {
		panic(err)
	}
}
