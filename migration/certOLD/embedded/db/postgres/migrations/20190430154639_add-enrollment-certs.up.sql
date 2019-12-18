create table ziti_edge.enrollment_certs
(
    id uuid not null
        constraint enrollment_certs_pkey primary key,
    enrollment_id uuid not null
        constraint enrollment_certs_enrollment_id__fk references ziti_edge.enrollments,
    ca_id uuid
        constraint enrollment_certs_ca_id__fk
            references ziti_edge.cas,
    jwt text not null,
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null,
    tags json default '{}'::jsonb not null
);
