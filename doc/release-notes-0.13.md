# 0.13 is not yet released! These are pre=release notes for features coming in 0.13. Some of these features may be released in 0.12 point releases

This page discusses the changes that you need to be aware of when migrating your Ziti deployment from version 0.12.x to version 0.13.x

# Theme
Ziti 0.13 includes the following: 
 
  * Changes to make working with policies easier, including
      * New APIs to list existing role attributes used by edge routers, identities and services
      
# Making Policies More User Friendly 
## Listing Role Attributes in Use

There are three new endpoints for listing role attributes in use. 

    * Endpoint: /edge-router-role-attributes
    * Endpoint: /identity-role-attributes
    * Endpoint: /service-role-attributes
    
All three support the same operations:

    * Supported operations
        * List: GET
            * Supports filtering
            * role attributes can be filtered/sorted using the symbol `id`
            * Ex:`?filter=id contains "north" limit 5`
            
The CLI supports these new operations as well.

    ziti edge controller list edge-router-role-policices
    ziti edge controller list identity-role-policices
    ziti edge controller list service-role-policices
    
Example output:

    $ ec list service-role-attributes "true sort by id desc limit 5" -j
    {
        "meta": {
            "filterableFields": [
                "id"
            ],
            "pagination": {
                "limit": 5,
                "offset": 0,
                "totalCount": 10
            }
        },
        "data": [
            "two",
            "three",
            "support",
            "sales",
            "one"
        ]
    }