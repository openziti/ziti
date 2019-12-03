create table ziti_edge.service_protocols
(
    service_id uuid not null
        constraint service_protocols_service_id__fk
            references ziti_edge.services,
    protocol_id varchar(20) not null
        constraint service_protocols_protocol_id__fk
            references ziti_edge.protocols,
    constraint service_protocols__pk
        primary key (service_id, protocol_id)
);

alter table ziti_edge.service_protocols owner to postgres;

