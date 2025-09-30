#!/bin/bash

if [[ "$OSTYPE" == "linux-musl"* ]];
then
  switch=""
  volumeDir="/data/.lnd"
elif [[ "$OSTYPE" == "darwin"* ]];
then
  volumeDir=~/Library/Application\ Support/Nigiri/volumes/lightningd/regtest
else
  exit "Unknown operating system: $OSTYPE"
fi

# generate data parts

volumeDir="/root/.lightning/regtest"
rootCert=$(grep -v 'CERTIFICATE' "${volumeDir}/ca.pem" | tr -d '=' | tr '/+' '_-' | tr -d '\n')
privateKey=$(grep -v 'PRIVATE KEY' "${volumeDir}/client-key.pem" | tr -d '=' | tr '/+' '_-' | tr -d '\n')
certChain=$(grep -v 'CERTIFICATE' "${volumeDir}/client.pem" | tr -d '=' | tr '/+' '_-' | tr -d '\n')

# generate URI parameters
rootCertParameter="?rootCert=${rootCert}"
privateKeyParameter="&privateKey=${privateKey}"
certChainParameter="&certChain=${certChain}"

clnconnect="clnconnect://boltz-cln:9736${rootCertParameter}${privateKeyParameter}${certChainParameter}"

echo $clnconnect