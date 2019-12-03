alter table ziti_edge.authenticator_updbs
    add authenticator_id uuid;

alter table ziti_edge.authenticator_updbs
    add constraint authenticator_id__fk
        foreign key (authenticator_id) references ziti_edge.authenticators;


-- add a default UUID v4 generator for id so it created per row
alter table ziti_edge.authenticators
    alter column id set default uuid_in(
            overlay(overlay(md5(random()::text || ':' || clock_timestamp()::text) placing '4' from 13)
                    placing to_hex(floor(random() * (11 - 8 + 1) + 8)::int)::text from 17)::cstring);

-- copy existing auth certs -> authenticators
insert into ziti_edge.authenticators(identity_id, method)
(SELECT
     identity_id,
     'updb' as type
 from ziti_edge.authenticator_updbs);


--remove default id generator
alter table ziti_edge.authenticators
    alter column id drop default;

-- add authenticators.id as authenticator_id (assumes 1:1 for cert auth and identity which should be true)
update ziti_edge.authenticator_updbs au set authenticator_id = (select a.id from ziti_edge.authenticators a where a.identity_id = au.identity_id) where 1=1;

-- make authenticator id not null
alter table ziti_edge.authenticator_updbs alter column authenticator_id set not null;

-- remove identity id column, now available on ziti_edge.authenticators parent
alter table ziti_edge.authenticator_updbs drop column identity_id;

