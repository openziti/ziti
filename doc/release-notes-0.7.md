 

This page discusses the changes that you need to be aware of when migrating your Ziti deployment from version 0.6.x to version 0.7.x

# Theme 
 * Ziti 0.7.0 replaces clusters with role attribute based policies
 * Ziti 0.7.0 takes steps towards consistent terminology for sessions

# Edge Router Policy
In 0.6.0 access to edge routers was controlled by clusters and services.

  * Every edge router was assigned to a cluster
  * Services belonged to 1 or more clusters
  * Dial/bind request would results would include a list of edge routers which were 
      * in clusters linked to the dialed/bound service and 
      * were online when the request was made 
      
Release 0.7.0 replaces this model with something new. It has the following goals:

  * Allow grouping edge routers and other entities dynamically using role attributes rather than hard-coded lists
  * Allow restricting access to edge router by identity in addition to by service

It includes the following new concepts:

  * Role attributes
     * Role attributes are just a set of strings associated to a model entity
     * The semantics of the role attributes are determined by the system administrator 
     * Ex: an edge router might have the role attributes `["us-east", "new-york", "omnicorp"]` 
     * These tags might indicate that this edge router is located on the east coast of the USA, specifically in New York and should be dedicated to use by a customer named OmniCorp
     * Currently role attributes are supported on edge routers and identities
  * Roles 
     * Roles specify a set of entities
     * Roles may include role attributes as well as entity ids
     * A role will match all entities which either:
         * Have **_all_** role attributes in the role OR
         * Have an ID which is listed explicitly
     * Role attributes are prefixed with `@`. Role elements not prefixed with `@` are assumed to be ids
     * There is a special role attribute `@all` which will match all entities
     * A role may have only role attributes or only ids or may have both
     
## Role Example
  * Edge router with id 1 has role attributes `["us-east", "New York City"]`    
  * Edge router with id 2 has role attributes `["us-east", "Albany"]`    
  * Edge router with id 3 has role attributes `["us-west", "Los Angeles"]`
  * An edge router role of `["@us-east", "@New York City", "3"]` would evaluate as follows
     * Edge router 1 would match because it has all listed role attributes
     * Edge router 2 would not match, because it doesn't have all listed role attributes
     * Edge router 3 would match because its ID is listed explicitly
     
## Model Changes
### Role Attributes
Edge routers and identities now have roleAttributes fields. Edge routers no longer have an associated cluster.

### Edge Router Policies
0.7.0 introduces a new model construct, the Edge Router Policy. This entity allows restricting which edge routers identities are allowed to use. An edge router policy has three attributes:

  * Name
  * Identity Roles
  * Edge Router Roles
  
An identity can be a member of multiple policies and will have access to the union of all edge routers linked to from those policies.

There is a new `/edge-router-policies` endpoint which can be used for creating/updating/deleting/querying edge router policies. Edge router policies PUT/POST/PATCH all take the following properties:

  * name
  * edgeRouterRoles
  * identityRoles
  * tags

### Service Edge Router Roles
Services now have a new edgeRouterRoles field. If set, this specifies which edge routers may be used for a service. This replaces the old cluster functionality. 

### Edge Router Access 
When a service is dialed or bound, which edge routers will be returned?

  * If the service edgeRouterRoles are NOT set, then it will be the set of edge routers to which the dialing/binding identity has access 
  * If the service edgeRouterRoles ARE set, then it will be the intersection of the edge routers to which the service has access and the set of edge routers to which the identity has access

### Cluster Removal and Migration
The `/clusters` endpoint has been removed. The bbolt schema version has been bumped to 2. If starting a fresh controller no action will be taken. However, if coming from an existing 0.6 or earlier bbolt database, the following will be done:

  1. An edge router policy will be created with `@all` for both identityRoles and edgeRouterRoles, allowing access to all edge routers from all identities. This will allow the current identities to continue using the system. Otherwise, no identities would be able to connect to any edge routers.
  2. Each edge router will get a role attribute of `cluster-<cluster name>` for the cluster it belonged to
  3. If a service belongs to 1 or more clusters it will get a role attribute corresponding to the first cluster. Any edge routers assigned to additional clusters will be added to edge router roles field by ID. 
      1. Noe: If we were to add additional role clusters for the other clusts we'd get the intersection, not the union and would end up with access to 0 edge routers  

# Session changes
Terminology related to sessions is being made consistent between the edge and fabric.

There are two types of sessions:

  1. Sessions between edge clients the edge controller, which allowed clients to manage controller state as well as dial and bind services
      1. These were referred to as sessions in the edge and have no fabric equivalent
  1. Sessions which establish routing and allow data flow to/from/within the edge and fabric
      1. These were referred to as network sessions in the edge and sessions in the fabric
      
Going forward, what was called a session in the edge will now be referred to as an API session. What was called a network session will be now just be called session in both the edge and fabric.

As a first step, in 0.7.0 API sessions will be available at both the `/sessions` and `/api-sessions` endpoints. Use of the `/sessions` endpoint is deprecated. In later releases the `/sessions` endpoint will be used for sessions instead of API sessions. 
