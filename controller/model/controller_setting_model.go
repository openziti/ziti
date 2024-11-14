package model

import (
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
)

type TrustDomainSettings struct {
	TrustDomain            string
	AdditionalTrustDomains []string
}

type ControllerSetting struct {
	models.BaseEntity
	Oidc        *OidcSettings
	IsMigration bool
}

type ControllerSettingEffective struct {
	models.BaseEntity
	Effective *ControllerSetting
	Instance  *ControllerSetting
}

func (s *ControllerSetting) fillFrom(env Env, tx *bbolt.Tx, boltEntity *db.ControllerSetting) error {
	s.Oidc = &OidcSettings{
		OidcSettingDef: boltEntity.Oidc,
	}

	s.IsMigration = boltEntity.IsMigration

	return nil
}

func (s *ControllerSetting) toBoltEntity() (*db.ControllerSetting, error) {
	return &db.ControllerSetting{
		BaseExtEntity: *boltz.NewExtEntity(s.Id, s.Tags),
		Oidc:          s.Oidc.OidcSettingDef,
		IsMigration:   s.IsMigration,
	}, nil
}

func (s *ControllerSetting) toBoltEntityForCreate(_ *bbolt.Tx, _ Env) (*db.ControllerSetting, error) {
	return s.toBoltEntity()
}

func (s *ControllerSetting) toBoltEntityForUpdate(_ *bbolt.Tx, _ Env, _ boltz.FieldChecker) (*db.ControllerSetting, error) {
	return s.toBoltEntity()
}

type OidcSettings struct {
	*db.OidcSettingDef
}
