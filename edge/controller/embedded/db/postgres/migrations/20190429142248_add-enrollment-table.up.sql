create table ziti_edge.enrollments
(
    id          uuid                                         not null
        constraint enrollments_pkey
            primary key,
    identity_id uuid                                         not null
        constraint enrollments_identity_id__fk
            references ziti_edge.identities,
    token       varchar(100)                                 not null,
    method      varchar(256)                                 not null,
    expires_at  timestamp with time zone                     not null,
    created_at  timestamp with time zone default now()       not null,
    updated_at  timestamp with time zone default now()       not null,
    tags        json                     default '{}'::jsonb not null
);
