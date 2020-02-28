This page discusses the changes that you need to be aware of when migrating your Ziti deployment from version 0.10.x to version 0.11.x

# Theme
Ziti 0.11 includes the following: 
 
 * Ziti connections from Ziti SDK client to services hosted by SDK are encrypted end-to-end (no API changes)
 

# End-to-end Encryption

Client and Hosting SDK instances setup end-to-end channels using secure key exchange and [AEAD](https://en.wikipedia.org/wiki/Authenticated_encryption) streams.
Read more about on https://netfoundry.github.io/ziti-doc (_coming soon_)