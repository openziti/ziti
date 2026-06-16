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

package main

import (
	"context"
	"fmt"

	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/xt"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
	"go.etcd.io/bbolt"
)

// runVerify runs the structural, file-level checks for the service-collapse migration against a bolt
// db. It is the one-time gate's structural pass (the API/snapshot comparison is done separately): it
// opens the db (migrating it if it predates the collapse), then asserts CheckIntegrity is clean, no
// edge index bucket exists, and re-running the migration is a no-op. The controller MUST be
// stopped first (bolt takes an exclusive lock); run against a COPY of the db.
func runVerify(dbPath string) error {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())

	boltDb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	defer boltDb.Close()

	// The migration is expected to report no anomaly events: the indexes are already in place
	// (nothing for its trailing integrity check to fix) and no edge-owned fields should exist at
	// the service level (no pre-existing destinations to replace). Capture any events: an
	// integrity fix means the pre-migration data (or the checker) was bad, and a replaced
	// destination means the db held unexpected, unreachable service-level data.
	var collapseEvents []string
	db.ServiceCollapseEventListener = func(event string) {
		collapseEvents = append(collapseEvents, event)
	}
	defer func() { db.ServiceCollapseEventListener = nil }()

	// InitStores + RunMigrations apply the collapse if the db predates it; on an already-migrated db
	// they are a no-op.
	stores, err := db.InitStores(boltDb, command.NoOpRateLimiter{}, nil)
	if err != nil {
		return fmt.Errorf("failed to init stores: %w", err)
	}
	if err := db.RunMigrations(boltDb, stores, nil); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	closeNotify := make(chan struct{})
	defer close(closeNotify)
	if err := stores.EventualEventer.Start(closeNotify); err != nil {
		return fmt.Errorf("failed to start eventual eventer: %w", err)
	}

	var problems []string

	// 0. The migration must not have reported any anomaly events.
	for _, event := range collapseEvents {
		problems = append(problems, fmt.Sprintf("migration reported: %v", event))
	}

	// 1. CheckIntegrity(fix=false) must report no drift (M7).
	if n, ierr := countIntegrityErrors(boltDb, stores); ierr != nil {
		problems = append(problems, fmt.Sprintf("CheckIntegrity failed to run: %v", ierr))
	} else if n > 0 {
		problems = append(problems, fmt.Sprintf("CheckIntegrity reported %d issue(s)", n))
	}

	// 2. No edge index bucket may exist (M6) -- it never did pre-collapse (the edge child store
	// inherited its parent's entity type, so its index lives at the unified service path), so
	// finding one means something unexpected happened. Also tally the edge/fabric partition.
	var fabricCount, edgeCount int
	err = boltDb.View(func(tx *bbolt.Tx) error {
		if boltz.Path(tx, db.RootBucket, boltz.IndexesBucket, db.EntityTypeServices, db.EdgeBucket) != nil {
			problems = append(problems, "unexpected edge index bucket ziti/indexes/services/edge is present")
		}
		var cErr error
		edgeCount, fabricCount, cErr = countServicePartition(tx, stores)
		return cErr
	})
	if err != nil {
		return fmt.Errorf("post-migration inspection failed: %w", err)
	}

	// 3. M8: re-running the migration must be a no-op -- no edge bucket reappears and integrity
	// stays clean (catches a missing/broken version gate that would re-collapse v46 data).
	if err := db.RunMigrations(boltDb, stores, nil); err != nil {
		return fmt.Errorf("idempotency re-run failed: %w", err)
	}
	err = boltDb.View(func(tx *bbolt.Tx) error {
		if boltz.Path(tx, db.RootBucket, boltz.IndexesBucket, db.EntityTypeServices, db.EdgeBucket) != nil {
			problems = append(problems, "edge index bucket reappeared after idempotency re-run")
		}
		// The service partition must be byte-for-byte stable across the re-run: a broken version gate
		// could re-collapse already-migrated rows (reclassifying edge services as fabric-only) while
		// leaving CheckIntegrity clean, so compare the edge/fabric tally to the post-migration tally.
		reEdge, reFabric, cErr := countServicePartition(tx, stores)
		if cErr != nil {
			return cErr
		}
		if reEdge != edgeCount || reFabric != fabricCount {
			problems = append(problems, fmt.Sprintf("service partition changed across idempotency re-run: was %d edge / %d fabric-only, now %d edge / %d fabric-only", edgeCount, fabricCount, reEdge, reFabric))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("idempotency inspection failed: %w", err)
	}
	if n, ierr := countIntegrityErrors(boltDb, stores); ierr != nil {
		problems = append(problems, fmt.Sprintf("CheckIntegrity failed to run after idempotency re-run: %v", ierr))
	} else if n > 0 {
		problems = append(problems, fmt.Sprintf("CheckIntegrity reported %d issue(s) after idempotency re-run", n))
	}

	fmt.Printf("services: %d edge, %d fabric-only\n", edgeCount, fabricCount)
	if len(problems) > 0 {
		fmt.Println("VERIFY FAILED:")
		for _, p := range problems {
			fmt.Println("  -", p)
		}
		return fmt.Errorf("%d verification problem(s)", len(problems))
	}
	fmt.Println("VERIFY OK: CheckIntegrity clean, no in-migration fixes, no edge index bucket, migration idempotent")
	return nil
}

// countServicePartition tallies how many services are edge vs fabric-only in the unified store. It
// is used to assert the partition is unchanged across the idempotency re-run.
func countServicePartition(tx *bbolt.Tx, stores *db.Stores) (edge int, fabric int, err error) {
	ids, _, qErr := stores.Service.QueryIds(tx, "true limit none")
	if qErr != nil {
		return 0, 0, qErr
	}
	for _, id := range ids {
		svc, _, fErr := stores.Service.FindById(tx, id)
		if fErr != nil {
			return 0, 0, fErr
		}
		if svc.IsFabricOnly {
			fabric++
		} else {
			edge++
		}
	}
	return edge, fabric, nil
}

// countIntegrityErrors returns the number of integrity issues reported via the callback AND the
// error returned by CheckIntegrity itself. The latter must be surfaced: if CheckIntegrity aborts
// with a hard error (rather than reporting drift through the callback), discarding it would let
// verify report success on a broken db.
func countIntegrityErrors(boltDb boltz.Db, stores *db.Stores) (int, error) {
	count := 0
	err := stores.CheckIntegrity(boltDb, context.Background(), false, func(e error, _ bool) {
		count++
		fmt.Printf("  integrity: %v\n", e)
	})
	return count, err
}
