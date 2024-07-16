FROM alpine:3.12

ARG TARGETPLATFORM

WORKDIR /app

COPY . .

RUN set -ex \
  && if [ "${TARGETPLATFORM}" = "linux/amd64" ]; then export TARGETPLATFORM=amd64; fi \
  && if [ "${TARGETPLATFORM}" = "linux/arm64" ]; then export TARGETPLATFORM=arm64; fi \
  && mv "ark-node-linux-$TARGETPLATFORM" /usr/local/bin/ark-node

ENV PATH="/app:${PATH}"
ENV ARK_NODE_DATADIR=/app/data

# Expose volume containing all 'arkd' data
VOLUME /app/data

ENTRYPOINT [ "ark-node" ]
	