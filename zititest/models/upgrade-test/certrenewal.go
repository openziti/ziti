/*
	(c) Copyright NetFoundry Inc.

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
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openziti/edge-api/rest_management_api_client/controllers"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitirest"
)

const (
	// ctrlCertMoveTimeout bounds how long the controllers may take to present and sign with their
	// renewed certs. A controller checks its identity every 30 seconds and signs with a renewed cert
	// 5 minutes after its record presents it, so this allows for that plus several missed checks.
	ctrlCertMoveTimeout = 8 * time.Minute
	// defaultCertRenewalSoak covers two 5m access token lifetimes, so every client refreshes onto a
	// token signed by a renewed cert at least once while the soak watches its traffic.
	defaultCertRenewalSoak = 10 * time.Minute
)

// ctrlCertRenewal runs the controller cert renewal phase. Every controller's server cert is reissued
// in place with no restart, the way the packaged renewal timer does it. The phase then checks that
// each controller moves to its renewed cert, soaks traffic while clients refresh their tokens, and
// restarts clients, controllers and routers in turn, so a key set rebuilt from scratch must still
// verify tokens signed with a controller's previous cert.
//
// The phase runs only after the nextVersion hop, which is the build under test; toVersion is a
// release that may not follow a renewed cert. It is skipped when nextVersion is unset or
// ctrlCertRenewal.enabled is false.
func ctrlCertRenewal(run model.Run) error {
	m := run.GetModel()
	log := tui.ValidationLogger()

	if m.GetStringVariableOr("nextVersion", "") == "" {
		log.Infof("nextVersion is not set, skipping the controller cert renewal phase")
		return nil
	}
	if enabled, found := m.GetBoolVariable("ctrlCertRenewal.enabled"); found && !enabled {
		log.Infof("ctrlCertRenewal.enabled is false, skipping the controller cert renewal phase")
		return nil
	}

	return m.Exec(run,
		"renewCtrlCerts",                     // reissue each controller's server cert in place, no restart
		"verifyCtrlCertsRenewed",             // each controller presents and signs with its renewed cert; peers accept its tokens
		"soakAfterCertRenewal",               // clients refresh onto tokens signed by the renewed certs
		"restartOneOfEach",                   // fresh logins from every SDK flavor
		"validateSteadyStateAfterDisruption", // restarted clients back and clean
		"restartClusterControllers",          // every controller rebuilds its key set from the records
		"verifyCtrlCertsRenewed",             // still on the renewed certs after a restart
		"validateSteadyStateAfterDisruption", // terminators + traffic recovered
		"restartClusterRouters",              // every router resyncs its key set from the controllers
		"validateSteadyStateAfterDisruption", // binds made before the renewal re-establish
	)
}

// soakAfterCertRenewal runs gate's post-disruption check with the stability window stretched to
// ctrlCertRenewal.soak.
func soakAfterCertRenewal(gate *steadyStateGate) func(run model.Run) error {
	return func(run model.Run) error {
		window := defaultCertRenewalSoak
		if v := run.GetModel().GetStringVariableOr("ctrlCertRenewal.soak", ""); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("invalid ctrlCertRenewal.soak [%s]: %w", v, err)
			}
			window = d
		}
		return gate.soakAfterDisruption(run, window)
	}
}

// renewCtrlCerts reissues every controller's server cert in place on its host, keeping the key,
// subject, SANs and SPIFFE id, and leaves the controllers running. It fails if a controller's
// cert is unchanged afterwards.
func renewCtrlCerts(run model.Run) error {
	log := tui.ValidationLogger()
	for _, c := range ctrlComponents(run.GetModel()) {
		previous, err := readCtrlServerCert(c)
		if err != nil {
			return err
		}
		if err = c.GetHost().ExecLogOnlyOnError(renewCtrlCertCmd(c)); err != nil {
			return fmt.Errorf("failed to renew the %s server cert: %w", c.Id, err)
		}
		renewed, err := readCtrlServerCert(c)
		if err != nil {
			return err
		}
		if bytes.Equal(previous.Raw, renewed.Raw) {
			return fmt.Errorf("the %s server cert is unchanged after renewal", c.Id)
		}
		log.Infof("renewed the %s server cert, kid %s -> %s", c.Id, certKid(previous), certKid(renewed))
	}
	return nil
}

// renewCtrlCertCmd returns the on-host command that reissues c's server cert with the key, name, SANs
// and SPIFFE id pki.CreateControllerCerts gave it. It runs the locally built ziti, since older
// "ziti pki" builds rewrite the chain file without truncating it.
func renewCtrlCertCmd(c *model.Component) string {
	host := c.GetHost()
	args := []string{
		zitilab.GetZitiBinaryPath(c, ""), "pki", "create", "server",
		"--pki-root", fmt.Sprintf("/home/%s/fablab/pki", host.GetSshUser()),
		"--ca-name", c.Id,
		"--key-file", c.Id + "-server",
		"--server-file", c.Id + "-server",
		"--server-name", c.Id,
		"--ip", host.PublicIp + "," + host.PrivateIp,
		"--spiffe-id", "controller/" + c.Id,
		"--allow-overwrite",
	}
	if ctrlType, ok := c.Type.(*zitilab.ControllerType); ok && len(ctrlType.DNSNames) > 0 {
		args = append(args, "--dns", strings.Join(ctrlType.DNSNames, ","))
	}
	return strings.Join(args, " ")
}

// readCtrlServerCert returns the leaf of the server cert chain c's identity loads, as configured in
// ctrl.yml.tmpl.
func readCtrlServerCert(c *model.Component) (*x509.Certificate, error) {
	path := fmt.Sprintf("/home/%s/fablab/pki/%s/certs/%s-server.chain.pem", c.GetHost().GetSshUser(), c.Id, c.Id)
	out, err := c.GetHost().ExecLogged("cat " + path)
	if err != nil {
		return nil, fmt.Errorf("failed to read the %s server cert: %w", c.Id, err)
	}
	cert, err := firstCert([]byte(out))
	if err != nil {
		return nil, fmt.Errorf("unparsable %s server cert: %w", c.Id, err)
	}
	return cert, nil
}

// firstCert parses the first CERTIFICATE block in data, skipping anything before it.
func firstCert(data []byte) (*x509.Certificate, error) {
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return nil, errors.New("no certificate found")
		}
		if block.Type == "CERTIFICATE" {
			return x509.ParseCertificate(block.Bytes)
		}
	}
}

// certKid returns the JWT kid a controller signs with when cert is its signing cert.
func certKid(cert *x509.Certificate) string {
	return fmt.Sprintf("%x", sha1.Sum(cert.Raw))
}

// verifyCtrlCertsRenewed checks that every controller has moved to the server cert on its host:
// every controller's records present each controller's cert, the access tokens each controller
// issues carry its cert's kid, and every other controller accepts those tokens and exchanges their
// refresh tokens. It waits up to ctrlCertMoveTimeout for that to hold.
func verifyCtrlCertsRenewed(run model.Run) error {
	log := tui.ValidationLogger()
	ctrls := ctrlComponents(run.GetModel())

	certs := map[string]*x509.Certificate{}
	for _, c := range ctrls {
		cert, err := readCtrlServerCert(c)
		if err != nil {
			return err
		}
		certs[c.Id] = cert
	}

	deadline := time.Now().Add(ctrlCertMoveTimeout)
	lastReason := ""
	for {
		err := checkCtrlCertsRenewed(run, ctrls, certs)
		if err == nil {
			log.Infof("all %d controllers present and sign with their renewed certs, and accept each other's tokens", len(ctrls))
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("controllers did not move to their renewed certs within %s: %w", ctrlCertMoveTimeout, err)
		}
		// The signing grace keeps the same reason current for minutes at a time.
		if reason := err.Error(); reason != lastReason {
			log.WithError(err).Info("waiting for the controllers to move to their renewed certs")
			lastReason = reason
		}
		time.Sleep(10 * time.Second)
	}
}

// checkCtrlCertsRenewed makes one pass of the checks verifyCtrlCertsRenewed waits on.
func checkCtrlCertsRenewed(run model.Run, ctrls []*model.Component, certs map[string]*x509.Certificate) error {
	sessions := map[string]edge_apis.ApiSession{}
	for _, c := range ctrls {
		clients, err := chaos.EnsureLoggedIntoCtrlOidc(run, c, time.Minute, 0)
		if err != nil {
			return fmt.Errorf("OIDC login to %s failed: %w", c.Id, err)
		}
		if err = checkCtrlRecords(clients, certs); err != nil {
			return fmt.Errorf("records at %s: %w", c.Id, err)
		}
		kid, err := sessionKid(clients.ApiSession())
		if err != nil {
			return fmt.Errorf("session from %s: %w", c.Id, err)
		}
		if expected := certKid(certs[c.Id]); kid != expected {
			return fmt.Errorf("%s signs tokens with kid %s, its renewed cert is kid %s", c.Id, kid, expected)
		}
		sessions[c.Id] = clients.ApiSession()
	}

	for _, issuer := range ctrls {
		for _, peer := range ctrls {
			if peer == issuer {
				continue
			}
			if err := checkPeerAcceptsSession(peer, sessions[issuer.Id], certs[peer.Id]); err != nil {
				return fmt.Errorf("%s session at %s: %w", issuer.Id, peer.Id, err)
			}
		}
	}
	return nil
}

// checkCtrlRecords reports whether the controller records clients reads present the certs in certs,
// keyed by controller id.
func checkCtrlRecords(clients *zitirest.Clients, certs map[string]*x509.Certificate) error {
	presented := map[string]string{}
	resp, err := listControllers(clients)
	if err != nil {
		return err
	}
	for _, detail := range resp.Payload.Data {
		if detail.ID != nil && detail.CertPem != nil {
			presented[*detail.ID] = *detail.CertPem
		}
	}

	for id, cert := range certs {
		certPem, found := presented[id]
		if !found {
			return fmt.Errorf("no record for %s", id)
		}
		recordCert, err := firstCert([]byte(certPem))
		if err != nil {
			return fmt.Errorf("unparsable cert in the record for %s: %w", id, err)
		}
		if !bytes.Equal(recordCert.Raw, cert.Raw) {
			return fmt.Errorf("the record for %s presents kid %s, its renewed cert is kid %s", id, certKid(recordCert), certKid(cert))
		}
	}
	return nil
}

// checkPeerAcceptsSession reports whether peer serves a management request made with session, which
// another controller issued, and exchanges its refresh token for a token signed with peerCert.
func checkPeerAcceptsSession(peer *model.Component, session edge_apis.ApiSession, peerCert *x509.Certificate) error {
	clients, err := zitirest.NewManagementClients(peer.Host.PublicIp + ":1280")
	if err != nil {
		return err
	}
	clients.UseApiSession(session)
	if _, err = listControllers(clients); err != nil {
		return fmt.Errorf("access token rejected: %w", err)
	}
	if err = clients.RefreshOidcSession(); err != nil {
		return fmt.Errorf("refresh token rejected: %w", err)
	}
	kid, err := sessionKid(clients.ApiSession())
	if err != nil {
		return fmt.Errorf("refreshed session: %w", err)
	}
	if expected := certKid(peerCert); kid != expected {
		return fmt.Errorf("refreshed token carries kid %s, the peer's renewed cert is kid %s", kid, expected)
	}
	return nil
}

func listControllers(clients *zitirest.Clients) (*controllers.ListControllersOK, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return clients.Edge.Controllers.ListControllers(&controllers.ListControllersParams{Context: ctx}, nil)
}

// sessionKid returns the kid from the header of session's access token. session must be an OIDC
// session.
func sessionKid(session edge_apis.ApiSession) (string, error) {
	if session == nil {
		return "", errors.New("not authenticated")
	}
	if session.GetType() != edge_apis.ApiSessionTypeOidc {
		return "", fmt.Errorf("session type is %v, not OIDC", session.GetType())
	}
	header, _, _ := strings.Cut(string(session.GetToken()), ".")
	raw, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		return "", fmt.Errorf("undecodable access token header: %w", err)
	}
	var parsed struct {
		Kid string `json:"kid"`
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("unparsable access token header: %w", err)
	}
	if parsed.Kid == "" {
		return "", errors.New("access token header has no kid")
	}
	return parsed.Kid, nil
}

// restartOneOfEach restarts the -restart instance of every client and host pair, leaving the -stable
// instances up, so every SDK flavor logs in afresh.
func restartOneOfEach(run model.Run) error {
	comps := run.GetModel().SelectComponents(".restart")
	if len(comps) == 0 {
		return fmt.Errorf("no components tagged 'restart' found")
	}
	tui.ValidationLogger().Infof("restarting %d -restart instance(s); -stable siblings stay up", len(comps))
	return chaos.RestartSelected(run, 10, comps...)
}

// restartClusterControllers restarts the controllers one at a time on nextVersion, leader last,
// waiting for the cluster to settle after each.
func restartClusterControllers(run model.Run) error {
	m := run.GetModel()
	return rollingClusterRestart(run, ctrlComponents(m), m.MustStringVariable("nextVersion"))
}

// restartClusterRouters restarts the edge routers one at a time on nextVersion.
func restartClusterRouters(run model.Run) error {
	return restartRoutersOn(run, run.GetModel().MustStringVariable("nextVersion"))
}
