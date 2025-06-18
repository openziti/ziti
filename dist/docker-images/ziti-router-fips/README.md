
## OpenZiti Router - FIPS

This Docker image is identical to the non-FIPS image except:

- It's based on Ubuntu 22.04 instead of RedHat 9.
- It's built on the `ziti-cli-fips` image instead of `ziti-cli`.
- It's in a separate directory so Dependabot can manage any dependencies added in the future.
