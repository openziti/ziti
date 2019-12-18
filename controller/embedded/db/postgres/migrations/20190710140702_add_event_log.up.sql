create table ziti_edge.event_logs
(
    id               uuid                                           not null
        constraint event_logs_pkey primary key,
    type             varchar(256)                                   not null,
    actor_type        varchar(256)                                   null,
    actor_id          varchar(256)                                   null,
    entity_type       varchar(256)                                   null,
    entity_id         varchar(256)                                   null,
    formatted_message text                                           not null,
    format_string     text                                           not null,
    format_data       text                                           null,
    data             jsonb                                          null,
    created_at       timestamp with time zone default now()         not null,
    updated_at       timestamp with time zone default now()         not null,
    tags             json                     default '{}' :: jsonb not null
);