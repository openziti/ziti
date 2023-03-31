//go:build perftests

package tests

import (
	"fmt"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/metrics"
	"net/url"
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
