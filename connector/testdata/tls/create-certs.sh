#!/bin/bash

set -e

pushd `dirname $0` > /dev/null
SCRIPT_PATH=`pwd -P`
popd > /dev/null

DAYS=720
SUB="/C=DE/ST=NRW/L=Earth/O=Random Company/OU=IT/CN=localhost/emailAddress=hasura@localhost"
ADDEXT="subjectAltName = DNS:localhost,IP:127.0.0.1" 

function generate_client() {
  CLIENT=$1
  openssl genrsa -out ${CLIENT}.key 2048
  openssl ecparam -genkey -name secp384r1 -out ${CLIENT}.key

  openssl req -new -key ${CLIENT}.key -days ${DAYS} -out ${CLIENT}.csr \
    -subj "$SUB" \
    -addext "$ADDEXT"
  openssl x509  -req -in ${CLIENT}.csr \
    -extfile <(printf "$ADDEXT") \
    -CA ca.crt -CAkey ca.key -out ${CLIENT}.crt -days ${DAYS} -sha256 -CAcreateserial
  cat ${CLIENT}.crt ${CLIENT}.key > ${CLIENT}.pem
}

function generate_cert() {
  rm -r ${SCRIPT_PATH}/$1
  mkdir -p ${SCRIPT_PATH}/$1

  pushd ${SCRIPT_PATH}/$1

  # generate a self-signed rootCA file that would be used to sign both the server and client cert.
  # Alternatively, we can use different CA files to sign the server and client, but for our use case, we would use a single CA.
  openssl req -newkey rsa:2048 \
    -new -nodes -x509 \
    -days ${DAYS} \
    -out ca.crt \
    -keyout ca.key \
    -subj "$SUB"

  # create a key for server
  openssl genrsa -out server.key 2048

  #generate the Certificate Signing Request
  openssl req -new -key server.key -days ${DAYS} -out server.csr \
    -addext "$ADDEXT" \
    -subj "$SUB"

  # sign it with Root CA
  # https://stackoverflow.com/questions/64814173/how-do-i-use-sans-with-openssl-instead-of-common-name
  openssl x509  -req -in server.csr \
    -extfile <(printf "$ADDEXT") \
    -CA ca.crt -CAkey ca.key  \
    -days ${DAYS} -sha256 -CAcreateserial \
    -out server.crt

  cat server.crt server.key > server.pem

  generate_client client

  popd
}

generate_cert certs
generate_cert certs_s1
