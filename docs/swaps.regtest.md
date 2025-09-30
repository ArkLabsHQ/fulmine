# Swaps

The purpose of this guide is to make you able to test Ark/LN submarine and reverse submarine swaps and walks you through setting a full stack on regtest that includes:

- Ark stack
  - Bitcoind
  - Arkd
- Boltz stack
  - Bitcoind
  - LND
  - Fulmine
  - Boltz backend
- User stack
  - Bitcoind
  - LND
  - Fulmine

NOTE: *For sake of simplicity, all stacks use the same Bitcoind instance.*

## Requirements

* [Docker](https://docs.docker.com/engine/install/)
* [Nigiri](https://nigiri.vulpem.com/)
* [jq](https://formulae.brew.sh/formula/jq)


## Setup regtest environment

Start regtest enviroment with Bitcoin and LND - this LND instance will be used by the end user:

```sh
nigiri start --ln
```

Fund LND wallet:

```sh
# Faucet 1 BTC
nigiri faucet lnd
```

Start LND used by boltz:

```sh
docker compose -f boltz.docker-compose.yml up -d boltz-lnd
# Create an alias for lncli
alias lncli="docker exec -it boltz-lnd lncli --network=regtest"
```

Start CLN used by boltz:

```sh
docker compose -f boltz.docker-compose.yml up -d boltz-cln
# Create an alias for lncli
alias clncli="docker exec -it boltz-cln lightning-cli --network=regtest"
```


Fund LND wallet:

```sh
lncli newaddress p2wkh
# Faucet 1 BTC
nigiri faucet <address>
```

Fund CLN wallet:

```sh 
clncli --network=regtest newaddr bech32
# Faucet 1 BTC
nigiri faucet <address>
```

Connect the LND instances:

```sh
lncli connect `nigiri lnd getinfo | jq -r .identity_pubkey`@lnd:9735
# Check the list of peers contains exactly one peer on both sides
lncli listpeers | jq .peers | jq length
nigiri lnd listpeers | jq .peers | jq length
```

Connect the CLN instances:

```sh
clncli connect `nigiri cln getinfo | jq -r .id` cln 9935
# Check the list of peers contains exactly one peer on both sides
clncli listpeers | jq .peers | jq length
nigiri cln listpeers | jq .peers | jq length
```


Open and fund channel between the LND instances:

```sh
# 100k sats channel Boltz <> User
lncli openchannel --node_key=`nigiri lnd getinfo | jq -r .identity_pubkey` --local_amt=100000
# Make the channel mature by mining 10 blocks
nigiri rpc --generate 10
# Generate an invoice for 50k sats
nigiri lnd addinvoice --amt 50000
# Manually copy the payment request and send 50k sats to the other side to balance the channel
lncli payinvoice <invoice>
# Type 'yes' when asked
```

Open and fund channel between the CLN instances:

```sh
# 100k sats channel Boltz <> User
clncli fundchannel id=`nigiri cln getinfo | jq -r .id` amount=100000
# Make the channel mature by mining 10 blocks
nigiri rpc --generate 10
# Send 50k sats to the other side to balance the channel
nigiri cln invoice 50000000 "" ""
# Type 'yes' when asked
clncli pay <invoice>
```

## Setup arkd

Start and provision Arkd:

```sh
docker compose -f test.docker-compose.yml up -d arkd
# Create an alias for arkd
alias arkd="docker exec arkd arkd"
# Wait till arkd is built then initialize the wallet
arkd wallet create --password password
# Unlock the service
arkd wallet unlock --password password
# Get an address to deposit funds.
# If it returns error, just wait few seconds and retry.
arkd wallet address
# Faucet 1 BTC (better if you repeat few times)
nigiri faucet <address>
```

NOTE: *The docker services defined in `test.docker-compose.yml` make use of temporary volumes, therefore any restart will become a fresh new start:* **DON'T DO THAT**.

The regtest setup is finished.

## Setup Boltz

### Setup Fulmine used by Boltz

Start Fulmine used by Boltz:

```sh
docker compose -f boltz.docker-compose.yml up -d boltz-fulmine
```

On your browser, go to http://localhost:7003 and initialize and unlock Fulmine - the Arkd url will be filled by default with the right value.

Go to the receive page, copy the bitcoin address (the second one) and send it some funds:

```sh
# Faucet 100k sats
nigiri faucet <address> 0.001
```

On your browser, go back to homepage of Fulmine, click on the pending tx and settle - click on the action menu, three dots on the top-right.

Lastly, connect Fulmine with the LND instance. For this you need an lndconnect URL that you can generate with:

```sh
docker exec -i boltz-lnd bash -c \
  'echo -n "lndconnect://boltz-lnd:10009?cert=$(grep -v CERTIFICATE /root/.lnd/tls.cert \
     | tr -d = | tr "/+" "_-")&macaroon=$(base64 /root/.lnd/data/chain/bitcoin/regtest/admin.macaroon \
     | tr -d = | tr "/+" "_-")"' | tr -d '\n'
```

Copy the generated URL to the clipboard. On Fulmine's tab of your browser, go to Settings > Lightning, paste the URL and click the Connect button.

### Start Boltz backend

Start Boltz backend with:

```sh
docker compose -f boltz.docker-compose.yml up -d boltz-postgres boltz
```

## Setup Fulmine used by end user

Start Fulmine used by end user:

```sh
docker compose -f test.docker-compose.yml up -d fulmine
```

Like done for the other Fulmine instance, open a new tab on the browser, go to http://localhost:7001 and initialize and unlock the service.

Go then to the receive page, copy the boarding address - the second one - and send it some funds:

```sh
nigiri faucet <address> 0.001
```

Last step, generate the lndconnect URL and connect Fulmine with the LND instance (the nigiri's one) from Settings:

```sh
docker exec -i lnd bash -c \
  'echo -n "lndconnect://lnd:10009?cert=$(grep -v CERTIFICATE /data/.lnd/tls.cert \
     | tr -d = | tr "/+" "_-")&macaroon=$(base64 /data/.lnd/data/chain/bitcoin/regtest/admin.macaroon \
     | tr -d = | tr "/+" "_-")"' | tr -d '\n'
```

You're good to go to test submarine and reverse submarine swaps on Ark!

**NOTE:** Use amounts above 1000 sats when testing swaps.

### Troubleshooting

* If you're on mac M-family, you have to build the boltz-backend docker image locally:
```sh
# Clone boltz-backend locally
git clone git@github.com:BoltzExchange/boltz-backend.git && cd boltz-backend
# Build the image, the VERSION=ark makes sure that the ark branch of the repo is built. 
docker build --build-arg NODE_VERSION=lts-bookworm-slim --build-arg VERSION=ark -t boltz/boltz:ark -f docker/boltz/Dockerfile .
```

