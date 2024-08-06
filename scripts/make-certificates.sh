#!/bin/bash
set -ex

certs_dir="certs"

mkdir -p "$certs_dir"

pushd "$certs_dir"

rm -f *.pem

# 1. Generate CA's private key and self-signed certificate
openssl req -x509 -newkey rsa:4096 -days 365 -nodes -keyout ca-key.pem -out ca-cert.pem -subj "/CN=issuer"

# 2. Generate server's certs
openssl req -x509 -newkey rsa:4096 -nodes -keyout server-key.pem -out server-cert.pem -subj "/CN=server" -addext "subjectAltName=DNS:localhost" -CA ca-cert.pem -CAkey ca-key.pem -days 60

# 3. Generate client 1's certs
openssl req -x509 -newkey rsa:4096 -nodes -keyout client-1-key.pem -out client-1-cert.pem -subj "/CN=client-1" -CA ca-cert.pem -CAkey ca-key.pem -days 60

# 4. Generate client 2's certs
openssl req -x509 -newkey rsa:4096 -nodes -keyout client-2-key.pem -out client-2-cert.pem -subj "/CN=client-2" -CA ca-cert.pem -CAkey ca-key.pem -days 60

popd
