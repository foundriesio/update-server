# About

This directory contains tools useful for local development

## perf-test

Self-contained Locust-based mTLS performance test that seeds and drives
thousands of fake devices against the update server. See
`perf-test/README.md`.

## dev-shell

This script builds a container with all the required dependencies for
developing on this code base and will drop you in a container with the
project source code mounted.

## docker-compose.yml

This compose project will launch an update server that devices can
communicate with. In order to use this you must first:
```
 $ go run github.com/foundriesio/update-server/cmd/server create-csr \
     --datadir .compose-server-data \
     --dnsname <HOSTNAME> --factory <FACTORY>
 $ go run github.com/foundriesio/update-server/cmd/server \
    --datadir .compose-server-data sign-csr \
    --cakey <PATH TO FACTORY PKI>/factory_ca.key \
    --cacert <PATH TO FACTORY PKI>/factory_ca.pem
 $ fioctl keys ca show --just-device-cas > .compose-server-data/certs/cas.pem

 $ go run github.com/foundriesio/update-server/cmd/server \
    --datadir .compose-server-data auth-init

 $ go run github.com/foundriesio/update-server/cmd/server \
    --datadir .compose-server-data tuf-init
```

## gen-certs.sh / fake-device.py

`gen-certs.sh` creates minimal fake data to stand up an update server and
have fake-devices connect to it.

`fake-device.py` is a simple script to issue HTTP requests against the server.

Example:
```
 $ ./contrib/gen-certs /tmp/server
 $  go run github.com/foundriesio/update-server/cmd/server serve --datadir /tmp/server

 # From another terminal:
 ./contrib/fake-device.py -d /tmp/server/fake-devices/device-1 /device
 < HTTP 200
 {"Uuid":"04581446-DB43-43B1-BAC3-DBA7D1328AAC","PubKey":"-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcD
QgAE9M4irNPAcO+3N+UZdfqH6M86IhGg\nC1X2xPHpE1q1JkPnJUYnOtoLPrCVERAQqN/2gzeJG3nl7fqKHrbzNRixgA==\n-----END PUBLIC KEY-----\n","UpdateName":"","Deleted":false,"LastSeen":1754345526,"IsProd":false}
```
