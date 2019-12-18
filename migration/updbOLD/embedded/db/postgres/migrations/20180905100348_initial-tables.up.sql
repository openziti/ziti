CREATE TABLE ziti_edge.authenticator_updbs
(
    id          UUID PRIMARY KEY                       NOT NULL,
    identity_id UUID                                   NOT NULL,
    username    VARCHAR(100)                           NOT NULL,
    salt        VARCHAR(100)                           NOT NULL,
    password    VARCHAR(100)                           NOT NULL,
    created_at  timestamp with time zone DEFAULT now() NOT NULL,
    updated_at  timestamp with time zone DEFAULT now() NOT NULL,
    tags        json                                   NOT NULL DEFAULT '{}' :: jsonb
);

ALTER TABLE ziti_edge.authenticator_updbs
    ADD CONSTRAINT authenticator_updbs_identity_id__fk FOREIGN KEY (identity_id) REFERENCES ziti_edge.identities (id);

CREATE UNIQUE INDEX authenticator_updbs_username__uindex
    ON ziti_edge.authenticator_updbs (username);

CREATE UNIQUE INDEX authenticator_updbs_salt__uindex
    ON ziti_edge.authenticator_updbs (salt);

CREATE UNIQUE INDEX authenticator_updbs_identity_id__uindex
    ON ziti_edge.authenticator_updbs (identity_id);
