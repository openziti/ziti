 

This page discusses the changes that you need to be aware of when migrating your Ziti deployment from version 0.7.x to version 0.8.x

# Theme 
 * Ziti 0.8.0 replaces appwans with role attribute based service policies
 * Ziti 0.8.0 consolidates dial and bind permissions into service policies

# Service Policy
In 0.7.0 and prior access to services was controlled by appwans.

  * Appwans had lists of identities and services
  * Identities and services could be associated with 0-n appwans
  * Services had explicit lists of identities that could bind the service
  * In order to dial a service, the identity had to be an admin or be in at least one appwan with that service
  * In order to bind a serivice, the identity had to be able to dial the service and be in the list of identities allowed to bind the service
      
Release 0.8.0 replaces this model with something new. It has the following goals:

  * Allow grouping identities and services dynamically using role attributes rather than hard-coded lists
  * Consolidate dial and bind permissions into the same model

The following concepts were introduced in 0.7 for edge router policies. They are now used for service policies as well.

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
  * Service with id 1 has role attributes `["sales", "New York City"]`    
  * Service with id 2 has role attributes `["sales", "Albany"]`    
  * Service with id 3 has role attributes `["support", "Los Angeles"]`
  * A service role of `["@sales", "@New York City", "3"]` would evaluate as follows
     * Service 1 would match because it has all listed role attributes
     * Service 2 would not match, because it doesn't have all listed role attributes
     * Service 3 would match because its ID is listed explicitly
     
## Model Changes
### Session Names
  1. api sessions had two endpoints in 0.7, `/api-sessions` and `/sessions` which was deprecated. `/sessions` is now no longer valid for api sessions
  2. sessions used the `/network-sessions` endpoint. In this release, `/network-sessions` has been deprecated and `/sessions` should be used instead. 
  3. `/current-session` is now `/current-api-session`
  
### Session Format
  1. When creating a session, the returned JSON has the same base format as when listing sessions, so it now includes the service and api-session information. The only difference is that the session token is also returned from session create, but not when listing sessions.
  1. The gateways attribute of session has been renamed to edgeRouters.

### Role Attributes
Services now have a roleAttributes field. Identities already had a roleAttributes field, for used with edge router policies.

### Service Policies
0.8.0 introduces a new model construct, the Service Policy. This entity allows restricting which services identities are allowed to dial or bind. A service policy has four attributes:

  * Name
  * Policy Type ("Bind" or "Dial")
  * Identity Roles
  * Service Roles
  
An identity can be a member of multiple policies and will have access to the union of all services linked to from those policies.

There is a new `/service-policies` endpoint which can be used for creating/updating/deleting/querying service policies. Service policies PUT/POST/PATCH all take the following properties:

  * name
  * type 
      * valid values are "Bind" and "Dial"
  * identityRoles
  * serviceRoles
  * tags

There are also new association endpoints allowing the listing of services and identities associated with service policies and vice-versa.

  * /service-policies/<id>/services
  * /service-policies/<id>/identities
  * /identities/<id>/service-policies
  * /services/<id>/service-policies

### Service Access 
  * An admin may dial or bind any service
  * A non-admin identity may dial any service it has access to via service policies of type "Dial"
  * A non-admin identity may bind any service it has access to via service policies of type "Bind"
  
When listing services, the controller used to provide a hostable flag with each service to indicate if the service could be bound in addition to being dialed. Now, the service will have a permissions block which will indicate if the service may be dialed, bound or both.

Ex:
```json
        {
            "meta": {},
            "data": {
                "id": "1012d4d7-3ab3-4722-8fa3-ae9f4da3c8ba",
                "createdAt": "2020-01-04T02:34:00.788444359Z",
                "updatedAt": "2020-01-04T02:34:00.788444359Z",
                "_links": {
                    "edge-routers": {
                        "href": "./services/1012d4d7-3ab3-4722-8fa3-ae9f4da3c8ba/edge-routers"
                    },
                    "self": {
                        "href": "./services/1012d4d7-3ab3-4722-8fa3-ae9f4da3c8ba"
                    },
                    "service-policies": {
                        "href": "./services/1012d4d7-3ab3-4722-8fa3-ae9f4da3c8ba/identities"
                    }
                },
                "tags": {},
                "name": "cac9593c-0494-4800-9f70-c258ff28a702",
                "dns": {
                    "hostname": "0bf71754-ed5b-4b2d-9adf-a542f1284275",
                    "port": 0
                },
                "endpointAddress": "4662d564-3fc3-4f10-b8cd-ee0e3629ad24",
                "egressRouter": "aedab92f-2ddf-445a-9194-73d428322a34",
                "edgeRouterRoles": null,
                "roleAttributes": [
                    "2c68789a-fe71-4d25-a483-43e54ee4fd98"
                ],
                "permissions": [
                    "Bind"
                ]
            }
        }
```

### Appwan Removal and Migration
The `/app-wans` endpoint has been removed. The bbolt schema version has been bumped to 3. If starting a fresh controller no action will be taken. However, if coming from an existing 0.7 or earlier bbolt database, the following will be done:

  1. For each existing appwan, a service policy with type "Dial" will be created 
  1. The new service policy will have the same name as the appwan it replaces 
  1. The new service policy will have the same identities and services as the appwan it replaces
  1. Identities and services will be specified explicitly by ID rather as opposed to by creating new role attributes 

NOTE: Service hosting identities will not be migrated into equivalent Bind service policies, as binds are not yet used in any production scenarios. 

# Go SDK changes
Several types have been renamed to conform to standard nomenclature

  * Session is now ApiSession
  * NetworkSession is now Session
     * The SessionId field is now ApiSessionId
     * The Gateways field is now EdgeRouters
  * Gateway is now EdgeRouter
  * On the Service type the Hostable flag has been removed and replaced with a Permissions string array
      * It may be nil, empty or contain either or both of "Dial" and "Bind" 
  * On the Context type
      * GetNetworkSession is now GetSession
      * GetNetworkHostSession is now GetBindSession
      
# ziti command line changes
  1. The `ziti edge controller create/delete gateway` commands have been removed. Use `ziti edge controller create/delete edge-router` instead.
  2. There are new `ziti edge controller create/delete service-policy` commands
  
# Ziti Proxy changes
ziti-proxy has been incorporated into the ziti-tunnel command. Where previously one would have run 

```
ZITI_SDK_CONFIG=./config.json ziti-proxy run <proxied services>
``` 

now one should use 

```
ziti-tunnel proxy -i ./config.json <proxied services>
``` 
