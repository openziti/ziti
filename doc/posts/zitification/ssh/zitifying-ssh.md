# Zitifying SSH

As we learned in the [opening post](../README.md) - "zitifying" an application means to embed a Ziti SDK into an
application and leverage the power of the Ziti Network to provide secure, truly zero-trust access to your application
wherever in the world that application goes. In this post we are going to see how we have zitified ssh and why. Future
posts will expand on this even further by showing how it is that NetFoundry uses `zssh` to support our customers.

## Why SSH

As I sit here typing these words I can tell you're skeptical. I can tell you're wondering why in the world we would even
attempt to mess with `ssh` at all. After all, `ssh` has been a foundation of the administration of not only home
networks but also corporate networks and the Internet itself. Surely if millions (billions?) of computers can interact
every day safely and securly using `ssh` there is "no need" for us to be spending time zitifying `ssh`
right? (Spoiler alert: wrong)

I'm sure you've guessed that this is not the case whatsoever. Attackers don't leave `ssh` alone just because it's not
worth it. Put a machine on the open internet, expose `ssh` on port 22 and watch for yourself all the attempts to
access `ssh` using known default/weak/bad passwords flood in. Attacks are not always limited to coming from the outside
either! A compromised machine on your network very well could behave in the same way when trying to expand/multiply. The
problems don't just stop here either. DoS attacks, other zero-day type bugs and more are all waiting for any service
sitting on the open internet.

A zitified `ssh` client is superior since openssh/sshd can be configured to listen only on localhost, preventing any
connections whatsoever from any network client - including the local network. Cool right? The only way to
`ssh` to a machine using a Ziti Network will be to have an identity authorized for that Ziti Network. Let's see how we
did that.

## Network Setup

## The Client

## Providing SSH Access Through Ziti

## Hardening SSH

/etc/ssh/sshd_config






