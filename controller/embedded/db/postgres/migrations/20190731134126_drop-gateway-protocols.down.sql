CREATE TABLE ziti_edge.gateway_protocols
(
    id          UUID PRIMARY KEY                       NOT NULL,
    gateway_id  UUID                                   NOT NULL,
    protocol_id varchar(20)                            NOT NULL,
    ingress_url varchar(8192)                          NOT NULL,
    created_at  timestamp with time zone DEFAULT now() NOT NULL,
    updated_at  timestamp with time zone DEFAULT now() NOT NULL,
    tags        json                                   NOT NULL DEFAULT '{}' :: jsonb
);

ALTER TABLE ziti_edge.gateway_protocols
    ADD CONSTRAINT gateway_protocols_gateway_id__fk FOREIGN KEY (gateway_id) REFERENCES ziti_edge.gateways (id);

ALTER TABLE ziti_edge.gateway_protocols
    ADD CONSTRAINT gateway_protocols_protocol_id__fk FOREIGN KEY (protocol_id) REFERENCES ziti_edge.protocols (id);

alter table ziti_edge.gateways add column hostname  varchar(512)