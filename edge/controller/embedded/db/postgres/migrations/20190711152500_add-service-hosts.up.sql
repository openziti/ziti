CREATE TABLE ziti_edge.service_hosts (
    service_id  UUID NOT NULL,
    identity_id UUID NOT NULL
);

ALTER TABLE ziti_edge.service_hosts
    ADD CONSTRAINT service_hosts__pk PRIMARY KEY (service_id, identity_id);

ALTER TABLE ziti_edge.service_hosts
    ADD CONSTRAINT service_hosts_service_id__fk FOREIGN KEY (service_id) REFERENCES ziti_edge.services (id);

ALTER TABLE ziti_edge.service_hosts
    ADD CONSTRAINT service_hosts_protocol_id__fk FOREIGN KEY (identity_id) REFERENCES ziti_edge.identities (id);

ALTER TABLE ziti_edge.network_sessions ADD COLUMN IF NOT EXISTS hosting boolean DEFAULT false;
