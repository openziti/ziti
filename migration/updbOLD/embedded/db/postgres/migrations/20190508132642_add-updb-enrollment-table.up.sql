create table ziti_edge.enrollment_updbs
(
    id            uuid                                         not null
        constraint enrollment_updb_pkey primary key,
    enrollment_id uuid                                         not null
        constraint enrollment_updb_enrollment_id__fk references ziti_edge.enrollments,
    username      varchar(1024)                                not null,
    url           text,
    created_at    timestamp with time zone default now()       not null,
    updated_at    timestamp with time zone default now()       not null,
    tags          json                     default '{}'::jsonb not null
);


