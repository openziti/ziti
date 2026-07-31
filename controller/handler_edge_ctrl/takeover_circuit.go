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

package handler_edge_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/pkg/errors"
)

// NewTakeoverCircuitHandler creates a handler for TakeoverCircuit requests, which a router sends to
// reattach a reroutable circuit's ingress to itself on behalf of an SDK presenting a reroute token.
func NewTakeoverCircuitHandler(appEnv *env.AppEnv, ch channel.Channel) channel.ContentTypeReceiver {
	handler := &takeoverCircuitHandler{
		baseRequestHandler: baseRequestHandler{ch: ch, appEnv: appEnv},
	}
	return &channel.AsyncFunctionReceiveAdapter{
		Type:    int32(edge_ctrl_pb.ContentType_TakeoverCircuitRequestType),
		Handler: handler.handleReceive,
	}
}

type takeoverCircuitHandler struct {
	baseRequestHandler
}

func (self *takeoverCircuitHandler) handleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	req, err := ctrl_msg.DecodeTakeoverCircuitRequest(msg)
	if err != nil {
		log.WithError(err).Error("could not decode TakeoverCircuitRequest")
		return
	}

	resp := self.processTakeover(req)
	respMsg := resp.ToMessage()
	respMsg.ReplyTo(msg)
	if err := ch.Send(respMsg); err != nil {
		log.WithError(err).Error("failed to send takeover circuit response")
	}
}

func (self *takeoverCircuitHandler) processTakeover(req *ctrl_msg.TakeoverCircuitRequest) *ctrl_msg.TakeoverCircuitResponse {
	log := pfxlog.ContextLogger(self.ch.Label())

	fail := func(code edge_client_pb.TakeoverResult, err error) *ctrl_msg.TakeoverCircuitResponse {
		log.WithError(err).WithField("code", code.String()).Info("takeover circuit rejected")
		return &ctrl_msg.TakeoverCircuitResponse{ResultCode: int32(code), ErrorMsg: err.Error()}
	}

	// Verify the token signature and its purpose/version (ParseRerouteToken enforces those).
	claims, err := edge.ParseRerouteToken(req.Token, self.appEnv.JwtSignerKeyFunc)
	if err != nil {
		return fail(edge_client_pb.TakeoverResult_TakeoverTokenRejected, err)
	}

	// The identity the router authenticated on the edge channel must match the token's bound
	// identity. The controller does not see the edge channel, so it trusts the router's attestation
	// here, re-checking it against the signed claim defense-in-depth.
	if claims.IdentityId != req.AuthenticatedIdentityId {
		return fail(edge_client_pb.TakeoverResult_TakeoverTokenRejected,
			errors.Errorf("token identity %s does not match authenticated identity %s", claims.IdentityId, req.AuthenticatedIdentityId))
	}

	// This controller must own the circuit (Phase A single-owner model).
	if claims.OwnerControllerId != self.appEnv.GetId() {
		return fail(edge_client_pb.TakeoverResult_TakeoverTokenRejected,
			errors.Errorf("circuit is owned by controller %s, not this controller", claims.OwnerControllerId))
	}

	circuit, found := self.getNetwork().GetCircuit(claims.CircuitId)
	if !found {
		return fail(edge_client_pb.TakeoverResult_TakeoverNotFound, errors.Errorf("circuit %s not found", claims.CircuitId))
	}

	// The new ingress is the router that sent this request (its channel id is its router id).
	newIngress, err := self.getNetwork().GetRouter(self.ch.Id())
	if err != nil {
		return fail(edge_client_pb.TakeoverResult_TakeoverTokenRejected, err)
	}

	// Authorization backstop against current RDM state. The new ingress router already ran the
	// authoritative dial-policy-plus-posture check locally before sending this request; this is the
	// controller's lag-free re-check that dial authorization still holds at revival time.
	if err := self.checkTakeoverAccess(claims.IdentityId, claims.ServiceId, newIngress.Id); err != nil {
		return fail(edge_client_pb.TakeoverResult_TakeoverTokenRejected, err)
	}

	newIteration, err := self.getNetwork().TakeoverCircuit(circuit, claims, newIngress)
	if err != nil {
		return fail(takeoverResultForError(err), err)
	}

	// Mint a fresh token at the advanced iteration so the SDK can reroute again later. If minting
	// fails the takeover has still committed and traffic flows; the SDK just can't reroute again
	// until it re-dials, so we surface success with an empty token rather than failing the takeover.
	freshToken, err := mintRerouteToken(self.appEnv, circuit.Id, claims.IdentityId, claims.ServiceId, circuit.Path.IngressId, newIteration)
	if err != nil {
		log.WithError(err).WithField("circuitId", circuit.Id).Error("takeover committed but failed to mint fresh reroute token")
	}

	return &ctrl_msg.TakeoverCircuitResponse{
		ResultCode:   int32(edge_client_pb.TakeoverResult_TakeoverSuccess),
		XgressId:     circuit.Path.IngressId,
		RerouteToken: freshToken,
		PeerData:     circuit.PeerData,
	}
}

// checkTakeoverAccess re-runs the controller-side dial authorization (service dial policy + edge
// router access) for the new ingress, matching what CreateCircuitV3 checks at dial time.
func (self *takeoverCircuitHandler) checkTakeoverAccess(identityId, serviceId, routerId string) error {
	dialable, err := self.appEnv.Managers.EdgeService.IsDialableByIdentity(serviceId, identityId)
	if err != nil {
		return err
	}
	if !dialable {
		return errors.Errorf("identity %s does not have dial access to service %s", identityId, serviceId)
	}

	allowed, err := self.appEnv.Managers.EdgeRouter.IsAccessToEdgeRouterAllowed(identityId, serviceId, routerId)
	if err != nil {
		return err
	}
	if !allowed {
		return errors.Errorf("identity %s is not allowed to use edge router %s for service %s", identityId, routerId, serviceId)
	}
	return nil
}

// takeoverResultForError maps a splice error to the SDK-facing result disposition: guard contention
// and route-install failures are retryable, while token/state mismatches are fatal.
func takeoverResultForError(err error) edge_client_pb.TakeoverResult {
	switch {
	case errors.Is(err, network.ErrCircuitMutationInProgress):
		return edge_client_pb.TakeoverResult_TakeoverBusy
	case errors.Is(err, network.ErrCircuitNotReroutable):
		return edge_client_pb.TakeoverResult_TakeoverNotReroutable
	case errors.Is(err, network.ErrStaleRerouteToken), errors.Is(err, network.ErrWrongRerouteSide):
		return edge_client_pb.TakeoverResult_TakeoverTokenRejected
	default:
		return edge_client_pb.TakeoverResult_TakeoverRouteInstallFailed
	}
}

// mintRerouteToken produces a controller-signed ingress-side reroute token for a circuit at the
// given iteration, signed with the controller's root JWT signer so routers and controllers can
// verify it via the cluster's published keys. ingressId is the circuit's ingress xgress address,
// which a takeover router pre-registers its forwarder at.
func mintRerouteToken(appEnv *env.AppEnv, circuitId, identityId, serviceId, xgressId string, iteration uint64) (string, error) {
	claims := &edge.RerouteClaims{
		CircuitId:         circuitId,
		IdentityId:        identityId,
		ServiceId:         serviceId,
		XgressId:          xgressId,
		Iteration:         iteration,
		OwnerControllerId: appEnv.GetId(),
		Purpose:           edge.RerouteTokenPurpose,
		Version:           edge.RerouteTokenVersion,
		Side:              edge.TokenSideIngress,
	}
	return appEnv.GetRootTlsJwtSigner().Generate(claims)
}
