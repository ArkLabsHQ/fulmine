#!/bin/bash

if [[ "$OSTYPE" == "linux-musl"* ]]; then
  switch=""
  volumeDir="/data/.lnd"
  elif [[ "$OSTYPE" == "darwin"* ]]; then
  switch="-i"
  volumeDir=~/Library/Application\ Support/Nigiri/volumes/lnd
else
  exit "Unknown operating system: $OSTYPE"
fi

# generate data parts
macaroon=$(base64 ${switch} "${volumeDir}/data/chain/bitcoin/regtest/admin.macaroon" | tr -d '=' | tr '/+' '_-' | tr -d '\n')
cert=$(grep -v 'CERTIFICATE' "${volumeDir}/tls.cert" | tr -d '=' | tr '/+' '_-' | tr -d '\n')

# generate URI parameters
macaroonParameter="?macaroon=${macaroon}"
certParameter="&cert=${cert}"

lndconnect="lndconnect://localhost:10009${macaroonParameter}${certParameter}"

echo $lndconnect