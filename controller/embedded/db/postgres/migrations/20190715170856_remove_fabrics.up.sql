alter table ziti_edge.services
    drop constraint services_fabric_id__fk;
alter table ziti_edge.services
    drop column fabric_id;

alter table ziti_edge.services
    drop column fabric_config;

alter table ziti_edge.services
    add legacy_passthrough bool default false not null;

alter table ziti_edge.services
    add endpoint_address varchar(2048);

alter table ziti_edge.services
    add egress_router varchar(2048) not null default 'unknown';


drop table ziti_edge.fabrics;
drop table ziti_edge.fabric_types;