package env

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/fabric/controller/xtv"
	nfpem "github.com/openziti/foundation/util/pem"
	"github.com/openziti/sdk-golang/ziti/signing"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"strings"
	"time"
)

func NewEdgeTerminatorValidator(ae *AppEnv) xtv.Validator {
	return &EdgeTerminatorValidator{
		ae: ae,
	}
}

type EdgeTerminatorValidator struct {
	ae *AppEnv
}

func (v *EdgeTerminatorValidator) Validate(tx *bbolt.Tx, terminator xtv.Terminator, create bool) error {
	session, err := v.getTerminatorSession(tx, terminator, "")
	if err != nil {
		return err
	}

	if terminator.GetIdentity() == "" {
		return nil
	}

	identityTerminators, err := v.ae.BoltStores.Terminator.GetTerminatorsInIdentityGroup(tx, terminator, create)
	for _, otherTerminator := range identityTerminators {
		otherSession, err := v.getTerminatorSession(tx, otherTerminator, "sibling ")
		if err != nil {
			return err
		}
		if otherSession != nil {
			if otherSession.ApiSession.IdentityId != session.ApiSession.IdentityId {
				return errors.Errorf("sibling terminator %v with shared identity %v belongs to different identity", terminator.GetId(), terminator.GetIdentity())
			}
		}
	}

	verifier, err := signing.GetVerifier(terminator.GetIdentitySecret())
	if err != nil {
		return err
	}

	now := time.Now()
	certs, err := v.ae.BoltStores.Session.LoadCerts(tx, session.Id)
	if err != nil {
		return err
	}
	for _, cert := range certs {
		if cert.ValidFrom.Before(now) && cert.ValidTo.After(now) {
			for _, x509 := range nfpem.PemToX509(cert.Cert) {
				if verifier.Verify(x509.PublicKey) {
					pfxlog.Logger().Debugf("verified terminator %v with identity %v", terminator.GetId(), terminator.GetIdentity())
					return nil
				}
			}
		}
	}

	return errors.Errorf("unable to verify identity secret for identity %v", terminator.GetIdentity())
}

func (v *EdgeTerminatorValidator) getTerminatorSession(tx *bbolt.Tx, terminator xtv.Terminator, context string) (*persistence.Session, error) {
	if terminator.GetBinding() != edge_common.Binding {
		return nil, errors.Errorf("%vterminator %v with identity %v is not edge terminator. Can't share identity", context, terminator.GetId(), terminator.GetIdentity())
	}

	addressParts := strings.Split(terminator.GetAddress(), ":")
	if len(addressParts) != 2 {
		return nil, errors.Errorf("%vterminator %v with identity %v is not edge terminator. Can't share identity", context, terminator.GetId(), terminator.GetIdentity())
	}

	if addressParts[0] != "hosted" {
		return nil, errors.Errorf("%vterminator %v with identity %v is not edge terminator. Can't share identity", context, terminator.GetId(), terminator.GetIdentity())
	}

	sessionToken := addressParts[1]
	session, err := v.ae.BoltStores.Session.LoadOneByToken(tx, sessionToken)
	if err != nil {
		pfxlog.Logger().Warnf("sibling terminator %v with shared identity %v has invalid session token %v", terminator.GetId(), terminator.GetIdentity(), sessionToken)
		return nil, nil
	}

	if session.ApiSession == nil {
		apiSession, err := v.ae.BoltStores.ApiSession.LoadOneById(tx, session.ApiSessionId)
		if err != nil {
			return nil, err
		}
		session.ApiSession = apiSession
	}

	return session, nil
}
