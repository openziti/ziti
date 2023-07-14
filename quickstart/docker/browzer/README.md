sudo docker run -it --rm --name certbot \
-v "/etc/letsencrypt:/etc/letsencrypt" \
-v "/var/lib/letsencrypt:/var/lib/letsencrypt" certbot/certbot certonly \
-d '*.zititv.demo.openziti.org' \
--manual \
--preferred-challenges dns \
--email clint@openziti.org \
--agree-tos


permission denied

sudo groupdel ziti
sudo userdel ziti

sudo groupadd ziti
sudo chmod -R g+r /etc/letsencrypt/
sudo chmod -R g+x /etc/letsencrypt/
sudo chgrp -R ziti /etc/letsencrypt/
sudo usermod -aG ziti $USER
sudo useradd ziti -g ziti --uid 2171
ll /etc/letsencrypt/




