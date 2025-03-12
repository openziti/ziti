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

package database

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/db"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
	"os"
	"sort"
	"strings"
)

var attrCounter = 0

func NewAnonymizeAction() *cobra.Command {
	action := anonymizeDbAction{
		mappings: map[string]map[string]string{
			"attributes": {},
		},
		Entry: pfxlog.Logger().Entry,
	}

	cmd := &cobra.Command{
		Use: "anonymize <path-to-db-file>",
		Short: "This utility attempts to remove personal information from the db. It does the following\n" +
			"1. Renames all identities, services, edge-routers, policies and configs with generic names\n" +
			"2. Removes tags from identities, services, edge-routers, policies and configs \n" +
			"3. Removes app data from identities and edge-routers\n" +
			"4. Makes all role attributes generic \n" +
			"5. Replaces host.v1, host.v2 and intercept.v1 configs with generic versions\n" +
			"6. Deletes all authenticators and enrollments\n" +
			"7. Deletes all api sessions, api session certificates and edge sessions\n\n" +
			"WARNINGS:\n" +
			"* There may be personal information in other database fields, as this doesn't cover every field in every type\n" +
			"* This works in place on the provided database. Only run this utility on a COPY of your database",
		Args: cobra.ExactArgs(1),
		Run:  action.run,
	}

	cmd.Flags().BoolVar(&action.preserveAuthenticators, "preserve-authenticators", false, "do not delete all authenticators")
	cmd.Flags().BoolVar(&action.preserveEnrollments, "preserve-enrollments", false, "do not delete all enrollments")
	cmd.Flags().BoolVar(&action.preserveTags, "preserve-tags", false, "do not clear tags")
	cmd.Flags().BoolVar(&action.preserveAppData, "preserve-app-data", false, "do not clear app data")
	cmd.Flags().StringVarP(&action.mappingOutput, "mapping-output", "m", "", "output mapping file. If not specified, mapping will not be emitted")
	return cmd
}

type anonymizeDbAction struct {
	preserveAuthenticators bool
	preserveEnrollments    bool
	preserveTags           bool
	preserveAppData        bool
	zitiDb                 boltz.Db
	stores                 *db.Stores
	mappings               map[string]map[string]string

	identityDialSvcCounts map[string]int
	identityBindSvcCounts map[string]int
	identityErCounts      map[string]int

	serviceDialIdsCounts map[string]int
	serviceBindIdsCounts map[string]int
	serviceErCounts      map[string]int

	erServiceCounts  map[string]int
	erIdentityCounts map[string]int

	mappingOutput string

	*logrus.Entry
}

func (self *anonymizeDbAction) run(_ *cobra.Command, args []string) {
	dbFile := args[0]

	zitiDb, err := db.Open(dbFile)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err = zitiDb.Close(); err != nil {
			panic(err)
		}
	}()

	stores, err := db.InitStores(zitiDb, command.NoOpRateLimiter{}, nil)
	if err != nil {
		panic(err)
	}

	self.zitiDb = zitiDb
	self.stores = stores

	self.initValidation()

	self.anonymizeIdentities()
	self.anonymizeServices()
	self.anonymizeEdgeRouters()
	self.anonymizeEdgeRouterPolicies()
	self.anonymizeServiceEdgeRouterPolicies()
	self.anonymizeServicePolicies()
	self.validateEntityCounts()

	self.scrubConfigs()

	if !self.preserveAuthenticators {
		self.scrubAuthenticators()
	}

	if !self.preserveEnrollments {
		self.scrubEnrollments()
	}

	self.scrubSessions()

	self.outputMappings()
}

func (self *anonymizeDbAction) outputMappings() {
	if self.mappingOutput == "" {
		return
	}

	var types []string
	for k := range self.mappings {
		types = append(types, k)
	}
	sort.Strings(types)

	output := bytes.Buffer{}

	for _, t := range types {
		m := self.mappings[t]

		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := m[k]
			if _, err := fmt.Fprintf(&output, "%s\t%s\t%s\n", t, k, v); err != nil {
				panic(err)
			}
		}
	}

	if err := os.WriteFile(self.mappingOutput, output.Bytes(), 0644); err != nil {
		panic(err)
	}

	self.Infof("mappings written to %s", self.mappingOutput)
}

func (self *anonymizeDbAction) initValidation() {
	self.identityDialSvcCounts = map[string]int{}
	self.identityBindSvcCounts = map[string]int{}
	self.identityErCounts = map[string]int{}

	self.serviceDialIdsCounts = map[string]int{}
	self.serviceBindIdsCounts = map[string]int{}
	self.serviceErCounts = map[string]int{}

	self.erServiceCounts = map[string]int{}
	self.erIdentityCounts = map[string]int{}

	// load entity relationship verification data
	err := self.zitiDb.View(func(tx *bbolt.Tx) error {
		ids, _, err := self.stores.Identity.QueryIds(tx, "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			entityCounter++
			self.identityDialSvcCounts[id] = getRelatedEntityCount(tx, self.stores.Identity, id, db.FieldIdentityDialServices)
			self.identityBindSvcCounts[id] = getRelatedEntityCount(tx, self.stores.Identity, id, db.FieldIdentityBindServices)
			self.identityErCounts[id] = getRelatedEntityCount(tx, self.stores.Identity, id, db.EntityTypeRouters)
		}

		self.Infof("scanned stats for %d identities", entityCounter)

		ids, _, err = self.stores.EdgeService.QueryIds(tx, "true limit none")
		if err != nil {
			return err
		}

		entityCounter = 0

		for _, id := range ids {
			entityCounter++
			self.serviceDialIdsCounts[id] = getRelatedEntityCount(tx, self.stores.EdgeService, id, db.FieldEdgeServiceDialIdentities)
			self.serviceBindIdsCounts[id] = getRelatedEntityCount(tx, self.stores.EdgeService, id, db.FieldEdgeServiceBindIdentities)
			self.serviceErCounts[id] = getRelatedEntityCount(tx, self.stores.EdgeService, id, db.FieldEdgeRouters)
		}

		self.Infof("scanned stats for %d services", entityCounter)

		ids, _, err = self.stores.EdgeRouter.QueryIds(tx, "true limit none")
		if err != nil {
			return err
		}

		entityCounter = 0

		for _, id := range ids {
			entityCounter++
			self.erServiceCounts[id] = getRelatedEntityCount(tx, self.stores.EdgeRouter, id, db.EntityTypeServices)
			self.erIdentityCounts[id] = getRelatedEntityCount(tx, self.stores.EdgeRouter, id, db.EntityTypeIdentities)
		}

		self.Infof("scanned stats for %d edge-routers", entityCounter)

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (self *anonymizeDbAction) rename(entity boltz.NamedExtEntity, counter int) string {
	m, ok := self.mappings[entity.GetEntityType()]
	if !ok {
		m = make(map[string]string)
		self.mappings[entity.GetEntityType()] = m
	}
	newName := fmt.Sprintf("%s-%04d", self.stores.GetStoreForEntity(entity).GetSingularEntityType(), counter)
	m[entity.GetName()] = newName
	return newName
}

func (self *anonymizeDbAction) anonymizeIdentities() {
	//  Update Identities
	err := self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		ids, _, err := self.stores.Identity.QueryIds(ctx.Tx(), "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			entityCounter++

			entity, err := self.stores.Identity.LoadById(ctx.Tx(), id)
			if err != nil {
				return err
			}
			entity.Name = self.rename(entity, entityCounter)
			entity.RoleAttributes = self.mapAttr(entity.RoleAttributes)
			if !self.preserveTags {
				entity.Tags = map[string]interface{}{}
			}
			if !self.preserveAppData {
				entity.AppData = map[string]interface{}{}
			}
			if err = self.stores.Identity.Update(ctx, entity, nil); err != nil {
				return err
			}
			if entityCounter%100 == 0 {
				self.Infof("processed identity %04d", entityCounter)
			}
		}
		self.Infof("processed %04d identities", entityCounter)
		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (self *anonymizeDbAction) anonymizeServices() {
	// Update Services
	err := self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		ids, _, err := self.stores.EdgeService.QueryIds(ctx.Tx(), "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			entity, err := self.stores.EdgeService.LoadById(ctx.Tx(), id)
			if err != nil {
				return err
			}

			entityCounter++
			entity.Name = self.rename(entity, entityCounter)
			entity.RoleAttributes = self.mapAttr(entity.RoleAttributes)
			if !self.preserveTags {
				entity.Tags = map[string]interface{}{}
			}
			if err = self.stores.EdgeService.Update(ctx, entity, nil); err != nil {
				return err
			}
			if entityCounter%100 == 0 {
				self.Infof("processed service %04d", entityCounter)
			}
		}
		self.Infof("processed %04d services", entityCounter)
		return nil
	})

	if err != nil {
		panic(err)
	}

}

func (self *anonymizeDbAction) anonymizeEdgeRouters() {
	// Update Edge Routers
	err := self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		ids, _, err := self.stores.EdgeRouter.QueryIds(ctx.Tx(), "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			entity, err := self.stores.EdgeRouter.LoadById(ctx.Tx(), id)
			if err != nil {
				return err
			}

			entityCounter++
			entity.Name = self.rename(entity, entityCounter)
			entity.RoleAttributes = self.mapAttr(entity.RoleAttributes)
			if !self.preserveTags {
				entity.Tags = map[string]interface{}{}
			}
			if !self.preserveAppData {
				entity.AppData = map[string]interface{}{}
			}
			if err = self.stores.EdgeRouter.Update(ctx, entity, nil); err != nil {
				return err
			}
			if entityCounter%100 == 0 {
				self.Infof("processed edge-router %04d", entityCounter)
			}
		}

		self.Infof("processed  %04d edge-routers", entityCounter)

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (self *anonymizeDbAction) anonymizeEdgeRouterPolicies() {
	// Update Edge Router Policies
	err := self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		ctx = ctx.GetSystemContext()
		ids, _, err := self.stores.EdgeRouterPolicy.QueryIds(ctx.Tx(), "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			policy, err := self.stores.EdgeRouterPolicy.LoadById(ctx.Tx(), id)
			if err != nil {
				return err
			}

			entityCounter++
			policy.IdentityRoles = self.mapRoles(policy.IdentityRoles)
			policy.EdgeRouterRoles = self.mapRoles(policy.EdgeRouterRoles)
			policy.Name = self.rename(policy, entityCounter)
			if !self.preserveTags {
				policy.Tags = map[string]interface{}{}
			}

			if err = self.stores.EdgeRouterPolicy.Update(ctx, policy, nil); err != nil {
				return err
			}
			if entityCounter%100 == 0 {
				self.Infof("processed edge-router-policy %04d", entityCounter)
			}
		}
		self.Infof("processed %04d edge-router-policies", entityCounter)
		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (self *anonymizeDbAction) anonymizeServiceEdgeRouterPolicies() {
	// Update Service Edge Router Policies
	err := self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		ids, _, err := self.stores.ServiceEdgeRouterPolicy.QueryIds(ctx.Tx(), "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			policy, err := self.stores.ServiceEdgeRouterPolicy.LoadById(ctx.Tx(), id)
			if err != nil {
				return err
			}

			entityCounter++
			policy.ServiceRoles = self.mapRoles(policy.ServiceRoles)
			policy.EdgeRouterRoles = self.mapRoles(policy.EdgeRouterRoles)
			policy.Name = self.rename(policy, entityCounter)
			if !self.preserveTags {
				policy.Tags = map[string]interface{}{}
			}

			if err = self.stores.ServiceEdgeRouterPolicy.Update(ctx, policy, nil); err != nil {
				return err
			}
			if entityCounter%100 == 0 {
				self.Infof("processed service-edge-router-policy %04d", entityCounter)
			}
			self.Infof("processed %04d service-edge-router-policies", entityCounter)
		}
		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (self *anonymizeDbAction) anonymizeServicePolicies() {
	// Update Service Policies
	err := self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		ids, _, err := self.stores.ServicePolicy.QueryIds(ctx.Tx(), "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			policy, err := self.stores.ServicePolicy.LoadById(ctx.Tx(), id)
			if err != nil {
				return err
			}

			entityCounter++
			policy.ServiceRoles = self.mapRoles(policy.ServiceRoles)
			policy.IdentityRoles = self.mapRoles(policy.IdentityRoles)
			policy.Name = self.rename(policy, entityCounter)
			if !self.preserveTags {
				policy.Tags = map[string]interface{}{}
			}

			if err = self.stores.ServicePolicy.Update(ctx, policy, nil); err != nil {
				return err
			}
			if entityCounter%100 == 0 {
				self.Infof("processed service-policy %04d", entityCounter)
			}
		}

		self.Infof("processed %04d service-policies", entityCounter)

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (self *anonymizeDbAction) validateEntityCounts() {
	// Validate identity references
	err := self.zitiDb.View(func(tx *bbolt.Tx) error {
		ids, _, err := self.stores.Identity.QueryIds(tx, "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0
		for _, id := range ids {
			entityCounter++
			self.validateRefCount(tx, self.stores.Identity, id, db.FieldIdentityDialServices, self.identityDialSvcCounts)
			self.validateRefCount(tx, self.stores.Identity, id, db.FieldIdentityBindServices, self.identityBindSvcCounts)
			self.validateRefCount(tx, self.stores.Identity, id, db.EntityTypeRouters, self.identityErCounts)
		}
		self.Infof("validated %04d identities", entityCounter)
		return nil
	})

	if err != nil {
		panic(err)
	}

	// Validate edge router references
	err = self.zitiDb.View(func(tx *bbolt.Tx) error {
		ids, _, err := self.stores.EdgeRouter.QueryIds(tx, "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0
		for _, id := range ids {
			entityCounter++
			self.validateRefCount(tx, self.stores.EdgeRouter, id, db.EntityTypeServices, self.erServiceCounts)
			self.validateRefCount(tx, self.stores.EdgeRouter, id, db.EntityTypeIdentities, self.erIdentityCounts)
		}
		self.Infof("validated %04d edge-routers", entityCounter)
		return nil
	})

	if err != nil {
		panic(err)
	}

	// Validate service references
	err = self.zitiDb.View(func(tx *bbolt.Tx) error {
		ids, _, err := self.stores.EdgeService.QueryIds(tx, "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0
		for _, id := range ids {
			entityCounter++
			self.validateRefCount(tx, self.stores.EdgeService, id, db.FieldEdgeServiceDialIdentities, self.serviceDialIdsCounts)
			self.validateRefCount(tx, self.stores.EdgeService, id, db.FieldEdgeServiceBindIdentities, self.serviceBindIdsCounts)
			self.validateRefCount(tx, self.stores.EdgeService, id, db.FieldEdgeRouters, self.serviceErCounts)
		}
		self.Infof("validated %04d services", entityCounter)
		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (self *anonymizeDbAction) scrubConfigs() {
	hostV2 := map[string]interface{}{
		"terminators": []interface{}{
			map[string]interface{}{
				"address":  "localhost",
				"port":     8888,
				"protocol": "tcp",
			},
		},
	}

	hostV1 := map[string]interface{}{
		"address":  "localhost",
		"port":     8888,
		"protocol": "tcp",
	}

	interceptV1 := map[string]interface{}{
		"addresses": []interface{}{"echo.ziti"},
		"portRanges": []interface{}{
			map[string]interface{}{
				"low":  1234,
				"high": 1234,
			},
		},
		"protocols": []interface{}{
			"tcp",
			"udp",
		},
	}

	err := self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		ids, _, err := self.stores.Config.QueryIds(ctx.Tx(), "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			entity, err := self.stores.Config.LoadById(ctx.Tx(), id)
			if err != nil {
				return err
			}

			entityCounter++
			entity.Name = self.rename(entity, entityCounter)
			if entity.Type == "NH5p4FpGR" { // host.v1
				entity.Data = hostV1
			} else if entity.Type == "host.v2" {
				entity.Data = hostV2
			} else if entity.Type == "g7cIWbcGg" { // intercept.v1
				entity.Data = interceptV1
			} else {
				return fmt.Errorf("unexpected config type: %s", entity.Type)
			}

			if !self.preserveTags {
				entity.Tags = map[string]interface{}{}
			}

			if err = self.stores.Config.Update(ctx, entity, nil); err != nil {
				return err
			}
			if entityCounter%100 == 0 {
				self.Infof("updated config %04d", entityCounter)
			}
		}

		self.Infof("updated %04d configs", entityCounter)

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (self *anonymizeDbAction) scrubAuthenticators() {
	err := self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		ids, _, err := self.stores.Authenticator.QueryIds(ctx.Tx(), "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			if err = self.stores.Authenticator.DeleteById(ctx, id); err != nil {
				return err
			}

			entityCounter++
			if entityCounter%100 == 0 {
				self.Infof("deleted authenticator %04d", entityCounter)
			}
		}

		self.Infof("deleted %04d authenticators", entityCounter)

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (self *anonymizeDbAction) scrubEnrollments() {
	err := self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		ids, _, err := self.stores.Enrollment.QueryIds(ctx.Tx(), "true limit none")
		if err != nil {
			return err
		}

		entityCounter := 0

		for _, id := range ids {
			if err = self.stores.Enrollment.DeleteById(ctx, id); err != nil {
				return err
			}

			entityCounter++
			if entityCounter%100 == 0 {
				self.Infof("deleted enrollment %04d", entityCounter)
			}
		}

		self.Infof("deleted %04d enrollments", entityCounter)

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func getRelatedEntityCount(tx *bbolt.Tx, store boltz.Store, id string, name string) int {
	count := 0
	cursor := store.GetRelatedEntitiesCursor(tx, id, name, true)
	for cursor.IsValid() {
		count++
		cursor.Next()
	}
	return count
}

func (self *anonymizeDbAction) mapRoles(roles []string) []string {
	attrMap := self.mappings["attributes"]
	var result []string
	for _, attr := range roles {
		if attr == "#all" || strings.HasPrefix(attr, "@") {
			result = append(result, attr)
		} else {
			key := strings.TrimPrefix(attr, "#")
			if newVal, found := attrMap[key]; found {
				result = append(result, "#"+newVal)
			} else {
				attrCounter++
				newVal = fmt.Sprintf("attr%04d", attrCounter)
				attrMap[attr] = newVal
				result = append(result, "#"+newVal)
			}
		}
	}
	return result
}

func (self *anonymizeDbAction) mapAttr(attrs []string) []string {
	attrMap := self.mappings["attributes"]

	var result []string

	for _, attr := range attrs {
		if newVal, ok := attrMap[attr]; ok {
			result = append(result, newVal)
		} else {
			attrCounter++
			newVal = fmt.Sprintf("attr%04d", attrCounter)
			attrMap[attr] = newVal
			result = append(result, newVal)
		}
	}
	return result
}

func (self *anonymizeDbAction) validateRefCount(tx *bbolt.Tx, store boltz.Store, id string, field string, m map[string]int) {
	count := getRelatedEntityCount(tx, store, id, field)

	if _, ok := m[id]; !ok {
		self.Infof("%s %s, old %s: NOT FOUND, new: %v", store.GetEntityType(), id, field, count)
		os.Exit(1)
	}

	if m[id] != count {
		self.Infof("%s %s, old %s: %v, current: %v", store.GetEntityType(), id, field, m[id], count)
		os.Exit(1)
	}
}

func (self *anonymizeDbAction) scrubSessions() {
	apiSessionBucketExists := false
	apiSessionCertsBucketExists := false
	sessionBucketExists := false

	apiSessionIndexBucketExists := false
	apiSessionCertificatesIndexBucketExists := false
	sessionIndexBucketExists := false

	logger := pfxlog.Logger()

	err := self.zitiDb.View(func(tx *bbolt.Tx) error {
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

	err = self.zitiDb.Update(nil, func(ctx boltz.MutateContext) error {
		root := ctx.Tx().Bucket([]byte(RootBucketName))
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
