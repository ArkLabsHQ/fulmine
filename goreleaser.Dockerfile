FROM debian:buster-slim

ARG TARGETPLATFORM

WORKDIR /app

COPY . .

RUN set -ex \
  && if [ "${TARGETPLATFORM}" = "linux/amd64" ]; then export TARGETPLATFORM=amd64; fi \
  && if [ "${TARGETPLATFORM}" = "linux/arm64" ]; then export TARGETPLATFORM=arm64; fi \
  && mv "ark-wallet-linux-$TARGETPLATFORM" /usr/local/bin/ark-wallet


# $USER name, and data $DIR to be used in the 'final' image
ARG USER=ArkLabsHQ
ARG DIR=/home/ArkLabsHQ

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates

# NOTE: Default GID == UID == 1000
RUN adduser --disabled-password \
            --home "$DIR/" \
            --gecos "" \
            "$USER"
USER $USER

# Prevents 'VOLUME $DIR/.ark-wallet/' being created as owned by 'root'
RUN mkdir -p "$DIR/.ark-wallet/"

# Expose volume containing all ark-wallet data
VOLUME $DIR/.ark-wallet/

ENTRYPOINT [ "ark-wallet" ]
	