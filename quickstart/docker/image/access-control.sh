#!/bin/bash

echo "*****************************************************"
#### Add service policies

# Allow all identities to use any edge router with the "public" attribute
ziti edge create edge-router-policy all-endpoints-public-routers --edge-router-roles "#public" --identity-roles "#all"

# Allow all edge-routers to access all services
ziti edge create service-edge-router-policy all-routers-all-services --edge-router-roles "#all" --service-roles "#all"
