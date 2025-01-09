package exporter

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_InputArgs(t *testing.T) {

	exporter := Exporter{}

	t.Run("service export", func(t *testing.T) {

		assert.True(t, exporter.IsCertificateAuthorityExportRequired([]string{"certificate-authority"}), "should be exported")
		assert.True(t, exporter.IsCertificateAuthorityExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsCertificateAuthorityExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"edge-router-policy"}), "should not be exported")
		assert.False(t, exporter.IsCertificateAuthorityExportRequired([]string{"service-edge-router-policy"}), "should not be exported")

	})

	t.Run("service export", func(t *testing.T) {

		assert.True(t, exporter.IsServiceExportRequired([]string{"service"}), "should be exported")
		assert.True(t, exporter.IsServiceExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsServiceExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsServiceExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsServiceExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsServiceExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsServiceExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsServiceExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsServiceExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsServiceExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsServiceExportRequired([]string{"edge-router-policy"}), "should not be exported")
		assert.False(t, exporter.IsServiceExportRequired([]string{"service-edge-router-policy"}), "should not be exported")

	})

	t.Run("config export", func(t *testing.T) {

		assert.True(t, exporter.IsConfigExportRequired([]string{"config"}), "should be exported")
		assert.True(t, exporter.IsConfigExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsConfigExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsConfigExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsConfigExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsConfigExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsConfigExportRequired([]string{"certificate-authority"}), "should not be exported")
		assert.False(t, exporter.IsConfigExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsConfigExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsConfigExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsConfigExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsConfigExportRequired([]string{"edge-router-policy"}), "should not be exported")
		assert.False(t, exporter.IsConfigExportRequired([]string{"service-edge-router-policy"}), "should not be exported")

	})

	t.Run("config-type export", func(t *testing.T) {

		assert.True(t, exporter.IsConfigTypeExportRequired([]string{"config-type"}), "should be exported")
		assert.True(t, exporter.IsConfigTypeExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsConfigTypeExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"certificate-authority"}), "should not be exported")
		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"edge-router-policy"}), "should not be exported")
		assert.False(t, exporter.IsConfigTypeExportRequired([]string{"service-edge-router-policy"}), "should not be exported")

	})

	t.Run("identity export", func(t *testing.T) {

		assert.True(t, exporter.IsIdentityExportRequired([]string{"identity"}), "should be exported")
		assert.True(t, exporter.IsIdentityExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsIdentityExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsIdentityExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"external-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"certificate-authority"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"edge-router-policy"}), "should not be exported")
		assert.False(t, exporter.IsIdentityExportRequired([]string{"service-edge-router-policy"}), "should not be exported")

	})

	t.Run("auth-policy export", func(t *testing.T) {

		assert.True(t, exporter.IsAuthPolicyExportRequired([]string{"auth-policy"}), "should be exported")
		assert.True(t, exporter.IsAuthPolicyExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsAuthPolicyExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"external-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"certificate-authority"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"edge-router-policy"}), "should not be exported")
		assert.False(t, exporter.IsAuthPolicyExportRequired([]string{"service-edge-router-policy"}), "should not be exported")

	})

	t.Run("ext-jwt-signer export", func(t *testing.T) {

		assert.True(t, exporter.IsExtJwtSignerExportRequired([]string{"ext-jwt-signer"}), "should be exported")
		assert.True(t, exporter.IsExtJwtSignerExportRequired([]string{"external-jwt-signer"}), "should be exported")
		assert.True(t, exporter.IsExtJwtSignerExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsExtJwtSignerExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"certificate-authority"}), "should not be exported")
		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"edge-router-policy"}), "should not be exported")
		assert.False(t, exporter.IsExtJwtSignerExportRequired([]string{"service-edge-router-policy"}), "should not be exported")

	})

	t.Run("posture-check export", func(t *testing.T) {

		assert.True(t, exporter.IsPostureCheckExportRequired([]string{"posture-check"}), "should be exported")
		assert.True(t, exporter.IsPostureCheckExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsPostureCheckExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"external-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"service-edge-router-policy"}), "should not be exported")
		assert.False(t, exporter.IsPostureCheckExportRequired([]string{"edge-router-policy"}), "should not be exported")

	})

	t.Run("service-policy export", func(t *testing.T) {

		assert.True(t, exporter.IsServicePolicyExportRequired([]string{"service-policy"}), "should be exported")
		assert.True(t, exporter.IsServicePolicyExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsServicePolicyExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"external-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"service-edge-router-policy"}), "should not be exported")
		assert.False(t, exporter.IsServicePolicyExportRequired([]string{"edge-router-policy"}), "should not be exported")

	})

	t.Run("service-edge-router-policy export", func(t *testing.T) {

		assert.True(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"service-edge-router-policy"}), "should be exported")
		assert.True(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"external-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsServiceEdgeRouterPolicyExportRequired([]string{"edge-router-policy"}), "should not be exported")

	})

	t.Run("service-edge-router-policy export", func(t *testing.T) {

		assert.True(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"edge-router-policy"}), "should be exported")
		assert.True(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"all"}), "should be exported")
		assert.True(t, exporter.IsEdgeRouterPolicyExportRequired([]string{}), "should be exported")

		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"service"}), "should not be exported")
		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"config"}), "should not be exported")
		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"config-type"}), "should not be exported")
		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"identity"}), "should not be exported")
		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"auth-policy"}), "should not be exported")
		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"ext-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"external-jwt-signer"}), "should not be exported")
		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"posture-check"}), "should not be exported")
		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"service-policy"}), "should not be exported")
		assert.False(t, exporter.IsEdgeRouterPolicyExportRequired([]string{"service-edge-router-policy"}), "should not be exported")

	})

}
