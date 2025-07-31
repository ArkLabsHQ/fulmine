# Taken from a Boltz Repository https://github.com/BoltzExchange/boltz-backend/blob/master/docker/c-lightning/Dockerfile
ARG VERSION=latest

FROM ubuntu:22.04 AS build

RUN apt-get update && apt-get -y upgrade
RUN apt-get -y install \
    gcc \
    git \
    curl \
    libpq-dev \
    libsqlite3-dev \
    protobuf-compiler

RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

RUN git clone https://github.com/BoltzExchange/hold.git

WORKDIR /hold
RUN git checkout 10c572a8abea39c73bf3323fbb5fa4739ca52279
RUN cargo build --release

FROM boltz/c-lightning:${VERSION} AS builder

RUN apt-get update && apt-get install -y git pkg-config wget

WORKDIR /clnurl

# TODO: download arm artifacts
RUN wget https://github.com/michael1011/clnurl/releases/download/v0.1.0/clnurl.tar.gz
RUN tar -xvf clnurl.tar.gz

FROM boltz/c-lightning:${VERSION}

COPY --from=build /hold/target/release/hold /root/hold
COPY --from=builder /clnurl/clnurl /root/clnurl

ENTRYPOINT ["lightningd"]