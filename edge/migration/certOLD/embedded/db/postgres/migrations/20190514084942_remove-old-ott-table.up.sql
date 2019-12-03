insert into ziti_edge.enrollments(id, identity_id, token, method, expires_at, created_at, updated_at, tags)
select id,
       identity_id,
       token,
       'ott',
       expires_at,
       created_at,
       updated_at,
       tags
from ziti_edge.identity_enrollment_otts;


-- add a default UUID v4 generator for id so it created per row
alter table ziti_edge.enrollment_certs
    alter column id set default uuid_in(
            overlay(overlay(md5(random()::text || ':' || clock_timestamp()::text) placing '4' from 13)
                    placing to_hex(floor(random() * (11 - 8 + 1) + 8)::int)::text from 17)::cstring);

insert into ziti_edge.enrollment_certs(enrollment_id, ca_id, jwt)
select id, ca_id, jwt
from ziti_edge.identity_enrollment_otts;


--remove default id generator
alter table ziti_edge.enrollment_certs
    alter column id drop default;


drop table ziti_edge.identity_enrollment_otts;