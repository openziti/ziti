This page discusses the changes that you need to be aware of when migrating your Ziti deployment from version 0.8.x to version 0.9.x

# Theme 
 * Ziti 0.9.0 adds a generic service configuration facility, useful for configuring service centric edge configuration data
 * Ziti 0.9.0 make several tweaks to policy syntax and semantics

# Service Configuration
Configurations are named JSON style objects that can be associated with services. Configurations have a type. 
A service can have 0 or 1 configurations of each configuration type associated with it.  

## Configuration types
There is a new endpoint for managing config types.

    * Endpoint: `/config-types`
    * Supported operations
        * Detail: GET `/config-types/<config-type-id>`
        * List: GET `/config-types/`
        * Create: POST `/config-types`
        * Update All Fields: PUT `/config-types/<config-type-id>`
        * Update Selective Fields: PATCH `/config-types/<config-type-id>`
        * Delete: DELETE `/config-types/<config-type-id>`
        * List associated configs GET `/config-types/<config-id>/configs`
     * Properties
         * Config types support the standard properties (id, createdAt, updatedAt, tags)
         * name - type: string, constraints: unique

The ziti CLI supports the following operations on config types: 

    * create config-type
    * list config-types
    * list config-type configs
    * delete config-type

## Configurations
There is a new endpoint for managing configurations       

    * Endpoint: `/configs`
    * Supported operations
        * Detail: GET `/configs/<config-id>`
        * List: GET `/configs/`
        * Create: POST `/configs/`
        * Update All Fields: PUT `/configs/<config-id>`
        * Update Selective Fields: PATCH `/configs/<config-id>`
        * Delete: DELETE `/config-types/<config-id>`
     * Properties
         * Configs support the standard properties (id, createdAt, updatedAt, tags)
         * name - type: string, constraints: unique
         * type - type: string. May be a config type id or config type name
         * data - type: JSON object
             * Support values are strings, numbers, booleans and nested objects/maps

The ziti CLI supports the following operations on configs: 

    * create config
    * update config
    * list configs
    * delete config
    
```shell script
$ ziti edge controller create config ssh ziti-tunneler-client.v1 '{ "hostname" : "ssh.mycompany.com", "port" : 22 }'
83a1e815-04bc-4c91-8d88-1de8c943545f

$ ziti edge controller list configs
id:   83a1e815-04bc-4c91-8d88-1de8c943545f
name: ssh
type: f2dd2df0-9c04-4b84-a91e-71437ac229f1
data: {
          "hostname": "ssh.mycompany.com",
          "port": 22
      }

$ ziti edge controller update config ssh -d '{ "hostname" : "ssh.mycompany.com", "port" : 2022 }'
Found configs with id 83a1e815-04bc-4c91-8d88-1de8c943545f for name ssh

$ ziti edge controller list configs
id:   83a1e815-04bc-4c91-8d88-1de8c943545f
name: ssh
type: f2dd2df0-9c04-4b84-a91e-71437ac229f1
data: {
          "hostname": "ssh.mycompany.com",
          "port": 2022
      }

$ ziti edge controller delete config ssh
Found configs with id 83a1e815-04bc-4c91-8d88-1de8c943545f for name ssh

$ ziti edge controller list configs
$ 
```
             
## Service Configuration
The DNS block, which included hostname and port, has been removed from service definitions. When creating or updating
services, you can submit a `configs` array, which may include config ids or names (or a mix of the two). Configs are
not required. 

**NOTE**: Only one config of a given type may be associated with a service.

Configurations associated with a service may be listed as entities using:

    * List associated configs GET `/services/<config-id>/configs`

### Retrieving service configuration
When authenticating, a user may now indicate which config types should be included when listing services. 
The authentication POST may include a body. If the body has a content-type of application/json, it will 
be parsed as a map. The controller will looking for a key at the top level of the map called `configTypes`,
which should be an array of config type ids or names (or mix of the two). 

Example authentication POST body:
```json
{
    "configTypes" : ["ziti-tunneler-client.v1", "ziti-tunneler-client.v2"]
}
``` 
When retrieving services, the config data for for those configuration types that were requested will be embedded in 
the service definition. For example, if the user has requested (by name) the config types "ziti-tunneler-client.v1" and
"ziti-tunneler-server.v1" and the `ssh` service has configurations of both of those kinds associated, a listing which
includes that service might look as follows:

```json
{
    "meta": {
        "filterableFields": [
            "id",
            "createdAt",
            "updatedAt",
            "name",
            "dnsHostname",
            "dnsPort"
        ],
        "pagination": {
            "limit": 10,
            "offset": 0,
            "totalCount": 1
        }
    },
    "data": [
        {
            "id": "2e79d56a-e37a-4f32-9769-f934976843d9",
            "createdAt": "2020-01-23T20:08:58.634275277Z",
            "updatedAt": "2020-01-23T20:08:58.634275277Z",
            "_links": {
                "edge-routers": {
                    "href": "./services/2e79d56a-e37a-4f32-9769-f934976843d9/edge-routers"
                },
                "self": {
                    "href": "./services/2e79d56a-e37a-4f32-9769-f934976843d9"
                },
                "service-policies": {
                    "href": "./services/2e79d56a-e37a-4f32-9769-f934976843d9/identities"
                }
            },
            "tags": {},
            "name": "ssh",
            "endpointAddress": "tcp:localhost:22",
            "egressRouter": "cf5d76cb-3fff-4dce-8376-60b2bfb505a6",
            "edgeRouterRoles": null,
            "roleAttributes": null,
            "permissions": [
                "Dial"
            ],
            "config": {
                "ziti-tunneler-client.v1": {
                    "hostname": "ssh.mycompany.com",
                    "port": 22
                },
                "ziti-tunneler-server.v1": {
                    "protocol" : "tcp",
                    "hostname": "ssh.mycompany.com",
                    "port": 22
                }
            }
        }
    ]
}
```
 
# Policy Changes
## Syntax Changes
   1. Roles are now prefixed with `#` instead of `@`
   1. Ids previously did not require a prefix. They now require an `@` prefix
   1. Entities could previously only be referenced by id. They can now also be referenced by name.
   1. Like ids, names must be prefixed with `@`. Entity references will first be check to see if they are a name. If no name is found then they are treated as ids.

## Entity Reference by Name
Previously, entities could be referenced in policies by id. They can now also be referenced by name, using the same
syntax. So a service named "ssh" can be referenced as `@ssh`. If the entity is renamed, the policy will be updated 
with the updated name. 

If a reference matches both a name and an ID, the ID will always take precedence.

## `Any Of` Semantics
Previously polices operated using 'all of' semantics. In other words, to match a policy, an entity had to have ALL OF
the role attributes specified by the policy or be listed explicitly by id. 

Edge Router and Service policies now have a new field `semantics`, which may have values of `AnyOf` or `AllOf`. If no
value is provided, it will default to the original behavior of `AllOf`. If `AnyOf` is provided then an entity will match
if it matches any of the roles listed, or if it is listed explicitly by id or name. 

**NOTE** 
Because service edgeRouterRoles are not broken out into a separate policy entity, they do not support `AnyOf` semantics. 

## `#All` limitations
Because having #all grouped with other roles or entity references doesn't make any sense, `#all` policies must now be
created with no other roles or entity references. 

# Session Types
Previously when creating a session a flag named `hosting` was provided to indicate if this was a Dial or Bind session.
Now a field named `type` should be provided instead with `Dial` and `Bind` being accepted values. If no value is provided
it will default to `Dial`.

Ex:
```json
    {
        "serviceId" : "a5a0f6af-c833-4961-be0a-c7fb093bb11e",
        "type" : "Dial"
    }
```

Similarly, when sessions were listed, they had a `hosting` flag, which has been replaced by a `type` flag. 

**NOTE**: Finally when sessions are transmitted between the controller and edge router, the format has also switched from using
a hosting flag to a type field. This means that controllers and edge routers will **not inter-operate** across the the 0.9
version boundary.
     