#!/bin/bash
set -eo pipefail

trap 'docker compose down -v --remove-orphans' EXIT

http_wait() {
  printf "$1:\t "

  for i in {1..120};
  do
    local code="$(curl -s -o /dev/null -m 2 -w '%{http_code}' $1)"
    if [ "$code" != "200" ]; then
      printf "."
      sleep 1
    else
      printf "\r\033[K$1:\t ${GREEN}OK${NC}\n"
      return 0
    fi
  done
  printf "\n${RED}ERROR${NC}: cannot connect to $1.\n"

  exit 1
}

# start unit tests
rm -rf ./coverage
mkdir -p ./coverage/exhttp
mkdir -p ./coverage/connector
mkdir -p ./coverage/schema

go test -v -race -timeout 3m -cover ./exhttp/... -args -test.gocoverdir=$PWD/coverage/exhttp ./exhttp/...
# go test -v -race -timeout 3m -cover ./ndc-http-schema/... -args -test.gocoverdir=$PWD/coverage/schema ./...

docker compose up -d hydra hydra-migrate
http_wait http://localhost:4444/health/ready

go test -v -race -timeout 3m -coverpkg=./... -cover ./... -args -test.gocoverdir=$PWD/coverage/connector ./...
docker compose down -v
go tool covdata textfmt -i=./coverage/connector,./coverage/exhttp -o ./coverage/profile.tmp

cat ./coverage/profile.tmp | grep -v "main.go" > ./coverage/profile.tmp2
cat ./coverage/profile.tmp2 | grep -v "version.go" > ./coverage/profile

# start ndc-test
NDC_TEST_VERSION=v0.1.6
CONFIG_PATH="./connector-definition"
if [ -n "$1" ]; then
  CONFIG_PATH="$1"
fi

mkdir -p ./tmp

if [ ! -f ./tmp/ndc-test ]; then
  if [ "$(uname -m)" == "arm64" ]; then
    curl -L https://github.com/hasura/ndc-spec/releases/download/$NDC_TEST_VERSION/ndc-test-aarch64-apple-darwin -o ./tmp/ndc-test
  elif [ $(uname) == "Darwin" ]; then
    curl -L https://github.com/hasura/ndc-spec/releases/download/$NDC_TEST_VERSION/ndc-test-x86_64-apple-darwin -o ./tmp/ndc-test
  else
    curl -L https://github.com/hasura/ndc-spec/releases/download/$NDC_TEST_VERSION/ndc-test-x86_64-unknown-linux-gnu -o ./tmp/ndc-test
  fi
  chmod +x ./tmp/ndc-test
fi

CONFIG_PATH=$CONFIG_PATH docker compose up -d --build ndc-http

http_wait http://localhost:8080/health

./tmp/ndc-test test --endpoint http://localhost:8080