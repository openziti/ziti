CREATE TABLE ziti_edge.authenticator_certs
(
    id          UUID PRIMARY KEY                       NOT NULL,
    identity_id UUID                                   NOT NULL,
    fingerprint VARCHAR(100)                           NOT NULL,
    created_at  timestamp with time zone DEFAULT now() NOT NULL,
    updated_at  timestamp with time zone DEFAULT now() NOT NULL,
    tags        json                                   NOT NULL DEFAULT '{}' :: jsonb
);

ALTER TABLE ziti_edge.authenticator_certs
    ADD CONSTRAINT authenticator_certs_identities_id__fk FOREIGN KEY (identity_id) REFERENCES ziti_edge.identities (id);

CREATE UNIQUE INDEX authenticator_certs_fingerprint__uindex
    ON ziti_edge.authenticator_certs (fingerprint);

CREATE UNIQUE INDEX authenticator_certs_identity_id__uindex
    ON ziti_edge.authenticator_certs (identity_id);


CREATE TABLE ziti_edge.identity_enrollment_otts
(
    id          uuid PRIMARY KEY                       NOT NULL,
    identity_id uuid                                   NOT NULL,
    token       varchar(100)                           NOT NULL,
    ca_id       uuid                                   NULL,
    jwt         text                                   NOT NULL,
    expires_at  timestamp with time zone               NOT NULL,
    created_at  timestamp with time zone DEFAULT now() NOT NULL,
    updated_at  timestamp with time zone DEFAULT now() NOT NULL,
    tags        json                                   NOT NULL DEFAULT '{}' :: jsonb
);

ALTER TABLE ziti_edge.identity_enrollment_otts
    ADD CONSTRAINT identity_enrolment_otts_identity_id__fk FOREIGN KEY (identity_id) REFERENCES ziti_edge.identities (id);

ALTER TABLE ziti_edge.identity_enrollment_otts
    ADD CONSTRAINT identity_enrolment_otts_ca_id__fk FOREIGN KEY (ca_id) REFERENCES ziti_edge.cas (id);
