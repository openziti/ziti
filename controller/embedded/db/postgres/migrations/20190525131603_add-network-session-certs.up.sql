create table ziti_edge.network_session_certs
(
    id uuid not null
        constraint network_session_certs_pkey
            primary key,
    network_session_id uuid not null
        constraint network_session_cert_network_sessions_id__fk
            references ziti_edge.network_sessions,
    cert text not null,
    fingerprint varchar(512) not null,
    valid_from timestamp with time zone default null,
    valid_to timestamp with time zone default null,
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null,
    tags json default '{}'::jsonb not null
);
