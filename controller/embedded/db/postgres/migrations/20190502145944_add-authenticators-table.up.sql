create table ziti_edge.authenticators
(
    id          uuid                                           not null
        constraint authenticators_pkey primary key,
    identity_id uuid                                           not null
        constraint authenticators_identity_id__fk references ziti_edge.identities,
    method      varchar(256)                                   not null,
    created_at  timestamp with time zone default now()         not null,
    updated_at  timestamp with time zone default now()         not null,
    tags        json                     default '{}' :: jsonb not null
);

