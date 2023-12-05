//go:build perftests

package tests

import (
	"encoding/json"
	"fmt"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/model"
	"go.etcd.io/bbolt"
	"net/url"
	"os"
	"sort"
	"testing"
	"time"
)

func TestServicePerf(t *testing.T) {
	ctx := NewTestContext(t)
	ctx.ApiHost = "127.0.0.1:1280"
	ctx.AdminAuthenticator.Username = "admin"
	ctx.AdminAuthenticator.Password = "admin"
	ctx.RequireAdminManagementApiLogin()

	identities := ctx.AdminManagementSession.requireQuery("identities?filter=" + url.QueryEscape("true limit 50"))
	ids, err := identities.S("data").Children()
	ctx.NoError(err)

	registry := metrics.NewRegistry("test", nil)
	lookupTimer := registry.Timer("serviceLookup")

	wg := concurrenz.NewWaitGroup()

	for _, id := range ids {
		identityId := id.S("id").Data().(string)

		doneC := make(chan struct{})
		wg.AddNotifier(doneC)

		go func() {
			for i := 0; i < 100; i++ {
				start := time.Now()
				_ = ctx.AdminManagementSession.requireQuery("services?asIdentity=" + identityId + "&filter=" + url.QueryEscape("true limit 100"))
				lookupTimer.UpdateSince(start)
			}
			close(doneC)
		}()
	}
	fmt.Println("all queries started")
	wg.WaitForDone(2 * time.Minute)

	msg := registry.Poll()
	lookupTimeSnapshot := msg.Timers["serviceLookup"]
	fmt.Printf("mean: %v\n", time.Duration(lookupTimeSnapshot.Mean).String())
	fmt.Printf("min: %v\n", time.Duration(lookupTimeSnapshot.Min).String())
	fmt.Printf("max: %v\n", time.Duration(lookupTimeSnapshot.Max).String())
	fmt.Printf("p95: %v\n", time.Duration(lookupTimeSnapshot.P95).String())
	fmt.Printf("p99: %v\n", time.Duration(lookupTimeSnapshot.P99).String())
	fmt.Printf("count: %v\n", lookupTimeSnapshot.Count)
}

func Test_ExportIdentityServicePostureChecks(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServerFor("/home/plorenz/work/nf/var/db/ctrl.db", false)

	stores := ctx.EdgeController.AppEnv.GetStores()
	managers := ctx.EdgeController.AppEnv.Managers
	identityManager := managers.Identity

	bindServices := stores.Identity.GetRefCountedLinkCollection(db.FieldIdentityBindServices)
	dialServices := stores.Identity.GetRefCountedLinkCollection(db.FieldIdentityDialServices)

	type identityServicePostureChecks struct {
		Id       string
		Services map[string]map[string]*model.PolicyPostureChecks
	}

	outputFile, err := os.Create("/home/plorenz/tmp/posture-checks.json")
	ctx.Req.NoError(err)

	err = identityManager.GetDb().View(func(tx *bbolt.Tx) error {
		identityIds, _, err := stores.Identity.QueryIds(tx, "limit none")
		if err != nil {
			return err
		}

		for idx, identityId := range identityIds {
			fmt.Printf("%v of %v: %v\n", idx, len(identityIds), identityId)
			output := &identityServicePostureChecks{
				Id:       identityId,
				Services: map[string]map[string]*model.PolicyPostureChecks{},
			}

			serviceIdSet := map[string]struct{}{}
			for cursor := bindServices.IterateLinks(tx, []byte(identityId), true); cursor.IsValid(); cursor.Next() {
				serviceId := string(cursor.Current())
				serviceIdSet[serviceId] = struct{}{}
			}
			for cursor := dialServices.IterateLinks(tx, []byte(identityId), true); cursor.IsValid(); cursor.Next() {
				serviceId := string(cursor.Current())
				serviceIdSet[serviceId] = struct{}{}
			}
			serviceIds := make([]string, 0, len(serviceIdSet))
			for serviceId := range serviceIdSet {
				serviceIds = append(serviceIds, serviceId)
			}
			sort.Strings(serviceIds)

			for _, serviceId := range serviceIds {
				postureChecks := managers.EdgeService.GetPolicyPostureChecks(identityId, serviceId)
				output.Services[serviceId] = postureChecks
			}
			bytes, err := json.Marshal(output)
			ctx.Req.NoError(err)
			_, err = outputFile.Write(bytes)
			ctx.Req.NoError(err)
			_, err = outputFile.WriteString("\n")
			ctx.Req.NoError(err)
		}

		return nil
	})
	ctx.Req.NoError(err)
}
