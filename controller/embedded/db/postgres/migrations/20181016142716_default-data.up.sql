INSERT INTO "ziti_edge"."identity_types" ("id", "name", "created_at", "updated_at")
VALUES ('577104f2-1e3a-4947-a927-7383baefbc9a', 'User', DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."identity_types" ("id", "name", "created_at", "updated_at")
VALUES ('5b53fb49-51b1-4a87-a4e4-edda9716a970', 'Device', DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."identity_types" ("id", "name", "created_at", "updated_at")
VALUES ('c4d66f9d-fe18-4143-85d3-74329c54282b', 'Service', DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."protocols" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('tls', 'tls', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."protocols" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('quic', 'quic', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."protocols" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('tcp', 'tcp', DEFAULT, DEFAULT, DEFAULT);



INSERT INTO "ziti_edge"."fabric_types" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('zt-passthrough', 'Passthrough', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."fabric_types" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('zt-native', 'Ziti Native', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."fabrics" ("id", "name", "fabric_type_id", "config", "created_at", "updated_at", "tags")
VALUES ('ffb41b14-d539-4db0-8651-ec1d62408023', 'Passthrough', 'zt-passthrough', DEFAULT, DEFAULT, DEFAULT, DEFAULT);




INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('a0e2c29f-9922-4435-a8a7-5dbf7bd92377', 'Canada Central', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('ac469973-105c-4de1-9f31-fffc077487fb', 'US West', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('2360479d-cc08-4224-bd56-43baa672af30', 'Japan', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('edd0680f-3ab4-49e6-9db5-c68258ba480d', 'Australia', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('521a3db9-8140-4854-a782-61cb5d3fe043', 'South America', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('8342acad-4e49-4098-84de-829feb55d350', 'Korea', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('b5562b15-ffeb-4910-bf14-a03067f9ca2e', 'US Midwest', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('6efe28a5-744e-464d-b147-4072efb769f0', 'Canada West', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('10c6f648-92b7-49e2-be96-f62357ea572f', 'Europe West', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('72946251-1fc7-4b3b-8568-c59d4723e704', 'Africa', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('70438d63-97b3-48b2-aeb5-b066a9526456', 'Europe East', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('e339d699-7f51-4e9c-a2ca-81720e07f196', 'US South', DEFAULT, DEFAULT, DEFAULT);
INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('00586703-6748-4c78-890d-efa216f21ef3', 'Canada East', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('63f7200b-7794-4a68-92aa-a36ed338ecba', 'Asia', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('f91ecca3-6f82-4c7b-8191-ea0036ce7b5a', 'US East', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('41929e78-6674-4708-89d2-9b934ea96822', 'Middle East', DEFAULT, DEFAULT, DEFAULT);

INSERT INTO "ziti_edge"."geo_regions" ("id", "name", "created_at", "updated_at", "tags")
VALUES ('5d0042bb-6fd5-4959-90a7-6bca70e23f76', 'US Central', DEFAULT, DEFAULT, DEFAULT);