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

package db

import (
	"time"

	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

// IdentityRevocationConstraint creates an identity-scoped revocation in the same
// transaction as the identity change that warrants it: deleting an identity, or
// disabling one. Self-contained OIDC JWTs are not stored, so without a revocation
// the router would keep serving a deleted/disabled identity's live sessions until
// their tokens expired. Running as a store pre-commit constraint makes the
// revocation atomic with the change and impossible to skip from any code path.
//
// The revocation type and lifetime are injected (the db layer has no view of the
// OIDC config or the rest_model revocation taxonomy), so the constraint stays
// free of those dependencies and the wiring layer supplies them.
type IdentityRevocationConstraint struct {
	revocationStore RevocationStore
	revocationType  string
	revocationTtl   func() time.Duration
}

// NewIdentityRevocationConstraint builds an IdentityRevocationConstraint that
// writes revocations of the given type into revocationStore, expiring
// revocationTtl() (the longest a token could remain valid) past the mutation.
func NewIdentityRevocationConstraint(revocationStore RevocationStore, revocationType string, revocationTtl func() time.Duration) *IdentityRevocationConstraint {
	return &IdentityRevocationConstraint{
		revocationStore: revocationStore,
		revocationType:  revocationType,
		revocationTtl:   revocationTtl,
	}
}

// ProcessPreCommit creates the revocation, within the change's transaction, when
// an identity is deleted or transitions into the disabled state.
func (self *IdentityRevocationConstraint) ProcessPreCommit(state *boltz.EntityChangeState[*Identity]) error {
	switch state.ChangeType {
	case boltz.EntityDeleted:
		if state.InitialState != nil {
			// A deleted identity can never re-authenticate, so the cutoff is the
			// moment of deletion. Take it from the replicated change context so
			// every raft node writes the same value.
			return self.revoke(state, state.InitialState.Id, self.mutationTime(state))
		}
	case boltz.EntityUpdated:
		// Only the transition into the disabled state warrants a revocation;
		// other updates, including changes made while already disabled, leave any
		// existing revocation untouched.
		if state.InitialState != nil && state.FinalState != nil &&
			state.InitialState.DisabledAt == nil && state.FinalState.DisabledAt != nil {
			// DisabledAt is stamped once on the originating controller and carried
			// in the update command, so it's identical on every node and is exactly
			// the cutoff we want: sessions issued before the disable are revoked,
			// while one re-authenticated after a re-enable survives.
			return self.revoke(state, state.FinalState.Id, *state.FinalState.DisabledAt)
		}
	}
	return nil
}

// mutationTime returns the cluster-consistent time of the current mutation,
// read from the replicated change context so it's identical on every raft node.
// It falls back to time.Now() only when no change context is present, a path
// with no raft peer to diverge from.
func (self *IdentityRevocationConstraint) mutationTime(state *boltz.EntityChangeState[*Identity]) time.Time {
	if changeCtx := change.FromContext(state.GetCtx().Context()); changeCtx != nil && !changeCtx.Timestamp.IsZero() {
		return changeCtx.Timestamp
	}
	return time.Now()
}

// ProcessPostCommit is a no-op; the revocation is persisted in ProcessPreCommit
// and the revocation store's own handler pushes it to the routers on commit.
func (self *IdentityRevocationConstraint) ProcessPostCommit(*boltz.EntityChangeState[*Identity]) {}

// revoke creates, replacing any prior entry, an identity-scoped revocation within
// the change's transaction. The cutoff is at, so a session re-authenticated
// after a re-enable survives the still-lingering revocation. at must be a
// cluster-consistent value (not a per-node time.Now()) so the revocation is
// identical across controllers and routers.
func (self *IdentityRevocationConstraint) revoke(state *boltz.EntityChangeState[*Identity], identityId string, at time.Time) error {
	revocation := &Revocation{
		BaseExtEntity: *boltz.NewExtEntity(identityId, nil),
		ExpiresAt:     at.Add(self.revocationTtl()),
		Type:          self.revocationType,
		IssuedBefore:  at,
	}

	ctx := state.GetCtx()
	if self.revocationStore.IsEntityPresent(ctx.Tx(), identityId) {
		if err := self.revocationStore.DeleteById(ctx, identityId); err != nil {
			return err
		}
	}
	return self.revocationStore.Create(ctx, revocation)
}
