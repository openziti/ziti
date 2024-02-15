# use a more recent debian image so it can run the ziti binary built with the Docker host's glibc
FROM debian:bookworm-slim

ARG ARTIFACTS_DIR=./build

COPY ${ARTIFACTS_DIR}/ziti /usr/local/bin/

CMD ["ziti"]
