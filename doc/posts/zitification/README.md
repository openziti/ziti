# Zitification

"Zitification" is the act of taking an application and incorporating a Ziti SDK into that application. Once an
application has a Ziti SDK incorporated into it, that application can now access network resources securely from
anywhere in the world provided that the computer has internet access: NO VPN NEEDED, NO ADDITIONAL SOFTWARE NEEDED.

Simply integrating the Ziti SDK into your application and enrolling the application itself into a Ziti Network provides
you with _tremendous_ additional security. By using the Ziti SDK to access remote resources your application will
become _IMMUNE_ to classic [ransomware attacks](https://netfoundry.io/ztna-ransomware/) of land, expand/multiply,
destroy. When your application uses a Ziti Network configure with a truly zero-trust mindset it will be impervious to
the "expand/multiple" phases of a classic [ransomware attacks](https://netfoundry.io/ztna-ransomware/). As recent
events have shown, it's probably not a case of if you application will be attacked, but when.

In these posts we're going to explore how common applications can be "zitified". The first application we are going
to focus on will be `ssh` and it's corollary `scp`.  At first you might think, "why even bother" zitifying (of all
things) `ssh` and `scp`? These applications are vital to system administration and we have been using `ssh` and
`scp` "safely-enough" on the internet for years.  Hopefully your interest is now sufficiently-piqued to explore the
first post: [zitifying ssh]( ./ssh/zitifying-ssh.md).
