create table ziti_edge.identity_enrollment_otts
(
    id uuid not null
        constraint identity_enrollment_otts_pkey
            primary key,
    identity_id uuid not null
        constraint identity_enrolment_otts_identity_id__fk
            references ziti_edge.identities,
    token varchar(100) not null,
    ca_id uuid
        constraint identity_enrolment_otts_ca_id__fk
            references ziti_edge.cas,
    jwt text not null,
    expires_at timestamp with time zone not null,
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null,
    tags json default '{}'::jsonb not null
);