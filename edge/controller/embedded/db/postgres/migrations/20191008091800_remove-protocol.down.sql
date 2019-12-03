CREATE TABLE ziti_edge.protocols (
    id         varchar(20) PRIMARY KEY                NOT NULL,
    name       varchar(255)                           NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    tags       json                                   NOT NULL DEFAULT '{}' :: jsonb
);

INSERT INTO "ziti_edge"."protocols" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('tls', 'tls', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."protocols" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('quic', 'quic', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."protocols" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('tcp', 'tcp', DEFAULT, DEFAULT, DEFAULT);