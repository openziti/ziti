CREATE TABLE ziti_edge.geo_regions (
  id         UUID PRIMARY KEY                       NOT NULL,
  name       varchar(255)                           NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  tags       json                                   NOT NULL DEFAULT '{}' :: jsonb
);


CREATE TABLE ziti_edge.cas
(
  id                             uuid PRIMARY KEY                       NOT NULL,
  name                           varchar(250)                           NOT NULL,
  fingerprint                    varchar(512)                           NOT NULL,
  cert_pem                       text                                   NOT NULL,
  is_verified                    boolean                                NOT NULL,
  verification_token             varchar(512)                           NOT NULL,
  is_auto_ca_enrollment_enabled  boolean                                NOT NULL,
  is_ott_ca_enrollment_enabled   boolean                                NOT NULL,
  is_auth_enabled                boolean                                NOT NULL,
  created_at                     timestamp with time zone DEFAULT now() NOT NULL,
  updated_at                     timestamp with time zone DEFAULT now() NOT NULL,
  tags                           json                                   NOT NULL DEFAULT '{}' :: jsonb
);

CREATE UNIQUE INDEX cas_fingerprint__uindex
  ON ziti_edge.cas (fingerprint);


CREATE TABLE ziti_edge.identity_types
(
  id         UUID PRIMARY KEY                       NOT NULL,
  name       varchar(100)                           NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  tags       json                                   NOT NULL DEFAULT '{}' :: jsonb
);

CREATE UNIQUE INDEX identity_types_id__uindex
  ON ziti_edge.identity_types (id);

CREATE TABLE ziti_edge.identities
(
  id               UUID PRIMARY KEY                       NOT NULL,
  name             varchar(250)                           NOT NULL,
  identity_type_id UUID                                   NOT NULL,
  is_default_admin boolean DEFAULT false                  NOT NULL,
  is_admin         boolean DEFAULT false                  NOT NULL,
  created_at       timestamp with time zone DEFAULT now() NOT NULL,
  updated_at       timestamp with time zone DEFAULT now() NOT NULL,
  tags             json                                   NOT NULL DEFAULT '{}' :: jsonb
);

CREATE UNIQUE INDEX identities_id__uindex
  ON ziti_edge.identities (id);

ALTER TABLE ziti_edge.identities
  ADD CONSTRAINT identities_identity_types_id__fk FOREIGN KEY (identity_type_id) REFERENCES ziti_edge.identity_types (id);



CREATE TABLE ziti_edge.sessions (
  id          UUID PRIMARY KEY                       NOT NULL,
  token       UUID                                   NOT NULL,
  identity_id UUID                                   NOT NULL,
  created_at  timestamp with time zone DEFAULT now() NOT NULL,
  updated_at  timestamp with time zone DEFAULT now() NOT NULL,
  tags        json                                   NOT NULL DEFAULT '{}' :: jsonb
);

ALTER TABLE ziti_edge.sessions
  ADD CONSTRAINT sessions_identity_id__fk FOREIGN KEY (identity_id) REFERENCES ziti_edge.identities (id);

CREATE UNIQUE INDEX sessions_token__uindex
  ON ziti_edge.sessions (token);

CREATE TABLE ziti_edge.clusters (
  id         UUID PRIMARY KEY                       NOT NULL,
  name       varchar(255)                           NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  tags       json                                   NOT NULL DEFAULT '{}' :: jsonb
);

CREATE TABLE ziti_edge.gateways (
  id                    UUID PRIMARY KEY                       NOT NULL,
  cluster_id            uuid                                   NOT NULL,
  name                  varchar(255)                           NOT NULL,
  fingerprint           varchar(1024)                          NULL,
  cert_pem              text                                   NULL,
  is_verified           boolean DEFAULT false                  NOT NULL,
  is_online             boolean DEFAULT false                  NOT NULL,
  geo_region_id         UUID                                   NULL,
  enrollment_token      UUID                                   NULL,
  enrollment_jwt        text                                   NULL,
  enrollment_created_at timestamp with time zone               NULL,
  enrollment_expires_at timestamp with time zone               NULL,
  created_at            timestamp with time zone DEFAULT now() NOT NULL,
  updated_at            timestamp with time zone DEFAULT now() NOT NULL,
  tags                  json                                   NOT NULL DEFAULT '{}' :: jsonb
);

ALTER TABLE ziti_edge.gateways
  ADD CONSTRAINT gateways_cluster_id__fk FOREIGN KEY (cluster_id) REFERENCES ziti_edge.clusters (id);

ALTER TABLE ziti_edge.gateways
  ADD CONSTRAINT gateways_geo_region_id__fk FOREIGN KEY (geo_region_id) REFERENCES ziti_edge.geo_regions (id);

CREATE TABLE ziti_edge.protocols (
  id         varchar(20) PRIMARY KEY                NOT NULL,
  name       varchar(255)                           NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  tags       json                                   NOT NULL DEFAULT '{}' :: jsonb
);

CREATE TABLE ziti_edge.fabric_types (
  id         varchar(20) PRIMARY KEY                NOT NULL,
  name       varchar(255)                           NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  tags       json                                   NOT NULL DEFAULT '{}' :: jsonb
);


CREATE TABLE ziti_edge.fabrics (
  id             UUID PRIMARY KEY                       NOT NULL,
  name           varchar(255)                           NOT NULL,
  fabric_type_id varchar(20)                            NOT NULL,
  config         json                                   NOT NULL DEFAULT '{}' :: jsonb,
  created_at     timestamp with time zone DEFAULT now() NOT NULL,
  updated_at     timestamp with time zone DEFAULT now() NOT NULL,
  tags           json                                   NOT NULL DEFAULT '{}' :: jsonb
);

ALTER TABLE ziti_edge.fabrics
  ADD CONSTRAINT fabrics_fabric_type_id__fk FOREIGN KEY (fabric_type_id) REFERENCES ziti_edge.fabric_types (id);


CREATE TABLE ziti_edge.services (
  id            UUID PRIMARY KEY                       NOT NULL,
  name          varchar(255)                           NOT NULL,
  dns_hostname  varchar(2048)                          NOT NULL,
  dns_port      integer                                NOT NULL,
  fabric_id     UUID                                   NOT NULL,
  fabric_config json                                   NOT NULL DEFAULT '{}' :: jsonb,
  created_at    timestamp with time zone DEFAULT now() NOT NULL,
  updated_at    timestamp with time zone DEFAULT now() NOT NULL,
  tags          json                                   NOT NULL DEFAULT '{}' :: jsonb
);

ALTER TABLE ziti_edge.services
  ADD CONSTRAINT services_fabric_id__fk FOREIGN KEY (fabric_id) REFERENCES ziti_edge.fabrics (id);

-- Table is deprecated as of 2019-06-10
CREATE TABLE ziti_edge.service_protocols (
  service_id  UUID        NOT NULL,
  protocol_id varchar(20) NOT NULL
);

ALTER TABLE ziti_edge.service_protocols
  ADD CONSTRAINT service_protocols__pk PRIMARY KEY (service_id, protocol_id);

ALTER TABLE ziti_edge.service_protocols
  ADD CONSTRAINT service_protocols_service_id__fk FOREIGN KEY (service_id) REFERENCES ziti_edge.services (id);

ALTER TABLE ziti_edge.service_protocols
  ADD CONSTRAINT service_protocols_protocol_id__fk FOREIGN KEY (protocol_id) REFERENCES ziti_edge.protocols (id);


CREATE TABLE ziti_edge.service_clusters (
  service_id UUID NOT NULL,
  cluster_id UUID NOT NULL
);

ALTER TABLE ziti_edge.service_clusters
  ADD CONSTRAINT service_clusters__pk PRIMARY KEY (service_id, cluster_id);

ALTER TABLE ziti_edge.service_clusters
  ADD CONSTRAINT service_clusters_service_id__fk FOREIGN KEY (service_id) REFERENCES ziti_edge.services (id);

ALTER TABLE ziti_edge.service_clusters
  ADD CONSTRAINT service_clusters_cluster_id__fk FOREIGN KEY (cluster_id) REFERENCES ziti_edge.clusters (id);


CREATE TABLE ziti_edge.network_sessions (
  id         UUID PRIMARY KEY                       NOT NULL,
  token      UUID                                   NOT NULL,
  session_id UUID                                   NOT NULL,
  service_id UUID                                   NOT NULL,
  cluster_id UUID                                   NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  tags       json                                   NOT NULL DEFAULT '{}' :: jsonb
);

ALTER TABLE ziti_edge.network_sessions
  ADD CONSTRAINT network_sessions_session_id__fk FOREIGN KEY (session_id) REFERENCES ziti_edge.sessions (id);

ALTER TABLE ziti_edge.network_sessions
  ADD CONSTRAINT network_sessions_service_id__fk FOREIGN KEY (service_id) REFERENCES ziti_edge.services (id);

ALTER TABLE ziti_edge.network_sessions
  ADD CONSTRAINT network_sessions_cluster_id__fk FOREIGN KEY (cluster_id) REFERENCES ziti_edge.clusters (id);

CREATE UNIQUE INDEX network_sessions_token__uindex
  ON ziti_edge.network_sessions (token);

CREATE TABLE ziti_edge.app_wans
(
  id   UUID PRIMARY KEY               NOT NULL,
  name varchar(255)                   NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  tags json                           NOT NULL DEFAULT '{}' :: jsonb NOT NULL
);

CREATE TABLE ziti_edge.app_wan_identities
(
  app_wan_id  UUID NOT NULL,
  identity_id UUID NOT NULL
);

ALTER TABLE ziti_edge.app_wan_identities
  ADD CONSTRAINT app_wan_identities__pk PRIMARY KEY (app_wan_id, identity_id);

ALTER TABLE ziti_edge.app_wan_identities
  ADD CONSTRAINT app_wan_identities_identity_id__fk FOREIGN KEY (identity_id) REFERENCES ziti_edge.identities (id);

ALTER TABLE ziti_edge.app_wan_identities
  ADD CONSTRAINT app_wan_identities_app_wan_id__fk FOREIGN KEY (app_wan_id) REFERENCES ziti_edge.app_wans (id);


CREATE TABLE ziti_edge.app_wan_services
(
  app_wan_id UUID NOT NULL,
  service_id UUID NOT NULL
);

ALTER TABLE ziti_edge.app_wan_services
  ADD CONSTRAINT app_wan_services__pk PRIMARY KEY (app_wan_id, service_id);

ALTER TABLE ziti_edge.app_wan_services
  ADD CONSTRAINT app_wan_services_services_id__fk FOREIGN KEY (service_id) REFERENCES ziti_edge.services (id);

ALTER TABLE ziti_edge.app_wan_services
  ADD CONSTRAINT app_wan_services_app_wan_id__fk FOREIGN KEY (app_wan_id) REFERENCES ziti_edge.app_wans (id);

