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
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/controller"
	fabricdb "github.com/openziti/ziti/controller/db"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

func NewDeleteSessionsFromDbCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete-sessions-from-db <path/to/db>",
		Short: "Delete all API Sessions and Edge Sessions, controller must be shutdown",
		Args:  cobra.ExactArgs(1),
		Run:   deleteSessionsFromDb,
	}
}

func NewDeleteSessionsFromConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete-sessions <config>",
		Short: "Delete all API Sessions and Edge Sessions, controller must be shutdown",
		Args:  cobra.ExactArgs(1),
		Run:   deleteSessionsFromConfig,
	}
}

const (
	ApiSessionBucketName             = "apiSessions"
	ApiSessionCertificatesBucketName = "apiSessionCertificates"
	SessionBucketName                = "sessions"
	RootBucketName                   = "ziti"

	IndexBucketName = "indexes"
)

func deleteSessionsFromConfig(_ *cobra.Command, args []string) {
	if config, err := controller.LoadConfig(args[0]); err == nil {
		deleteSessions(config.Db)
	} else {
		panic(err)
	}
}

func deleteSessionsFromDb(_ *cobra.Command, args []string) {
	db, err := fabricdb.Open(args[0])
	if err != nil {
		panic(err)
	}
	deleteSessions(db)
}

func deleteSessions(db boltz.Db) {
	logrus.WithField("version", version.GetVersion()).
		WithField("go-version", version.GetGoVersion()).
		WithField("os", version.GetOS()).
		WithField("arch", version.GetArchitecture()).
		WithField("build-date", version.GetBuildDate()).
		WithField("revision", version.GetRevision()).
		Info("removing API Sessions and Edge Sessions from ziti-controller")

	apiSessionBucketExists := false
	apiSessionCertsBucketExists := false
	sessionBucketExists := false

	apiSessionIndexBucketExists := false
	apiSessionCertificatesIndexBucketExists := false
	sessionIndexBucketExists := false

	logger := pfxlog.Logger()

	defer func() {
		_ = db.Close()
	}()

	err := db.View(func(tx *bbolt.Tx) error {
		root := tx.Bucket([]byte(RootBucketName))

		if root == nil {
			return errors.New("root 'ziti' bucket not found")
		}

		apiSessionBucket := root.Bucket([]byte(ApiSessionBucketName))

		if apiSessionBucket == nil {
			logger.Info("api session bucket does not exist, skipping, count is: 0")
		} else {
			apiSessionBucketExists = true
			count := 0
			_ = apiSessionBucket.ForEach(func(_, _ []byte) error {
				count++
				return nil
			})
			logger.Infof("existing api sessions: %v", count)
		}

		apiSessionCertificatesBucket := root.Bucket([]byte(ApiSessionCertificatesBucketName))

		if apiSessionCertificatesBucket == nil {
			logger.Info("api session certificates bucket does not exist, skipping, count is: 0")
		} else {
			apiSessionCertsBucketExists = true
			count := 0
			_ = apiSessionCertificatesBucket.ForEach(func(_, _ []byte) error {
				count++
				return nil
			})
			logger.Infof("existing api sessions certificates: %v", count)
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
				apiSessionIndexBucketExists = true
			}

			apiSessionIndexBucket := indexBucket.Bucket([]byte(ApiSessionCertificatesBucketName))

			if apiSessionIndexBucket == nil {
				logger.Print("api sessions certificates index bucket does not exist, skipping")
			} else {
				apiSessionCertificatesIndexBucketExists = true
			}

			sessionTokenBucket := indexBucket.Bucket([]byte(SessionBucketName))

			if sessionTokenBucket == nil {
				logger.Print("edge sessions index bucket does not exist, skipping")
			} else {
				sessionIndexBucketExists = true
			}
		}

		return nil
	})

	if err != nil {
		pfxlog.Logger().Errorf("could not read database stats: %v", err)
	}

	err = db.Update(nil, func(ctx boltz.MutateContext) error {
		root := ctx.Tx().Bucket([]byte("ziti"))
		if root == nil {
			return errors.New("root 'ziti' bucket not found")
		}

		if apiSessionBucketExists {
			if err := root.DeleteBucket([]byte(ApiSessionBucketName)); err != nil {
				logger.Infof("could not delete api sessions: %v", err)
			} else {
				logger.Infof("done removing api sessions")
			}
		}

		if apiSessionCertsBucketExists {
			if err := root.DeleteBucket([]byte(ApiSessionCertificatesBucketName)); err != nil {
				logger.Infof("could not delete api sessions certificates: %v", err)
			} else {
				logger.Infof("done removing api sessions certificates")
			}
		}

		if sessionBucketExists {
			if err := root.DeleteBucket([]byte(SessionBucketName)); err != nil {
				logger.Infof("could not delete sessions: %v", err)
			} else {
				logger.Infof("done removing edge sessions")
			}
		}

		indexBucket := root.Bucket([]byte(IndexBucketName))

		if apiSessionIndexBucketExists {
			if err := indexBucket.DeleteBucket([]byte(ApiSessionBucketName)); err != nil {
				logger.Infof("could not delete api session indexes: %v", err)
			} else {
				logger.Infof("done removing api session indexes")
			}
		}

		if apiSessionCertificatesIndexBucketExists {
			if err := indexBucket.DeleteBucket([]byte(ApiSessionCertificatesBucketName)); err != nil {
				logger.Infof("could not delete api session certificates indexes: %v", err)
			} else {
				logger.Infof("done removing api session certificates indexes")
			}
		}

		if sessionIndexBucketExists {
			if err := indexBucket.DeleteBucket([]byte(SessionBucketName)); err != nil {
				logger.Infof("could not delete edge session indexes: %v", err)
			} else {
				logger.Infof("done removing edge session indexes")
			}
		}

		return nil
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Error("error removing sessions")
	}
}
