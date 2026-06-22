# Using the REST API

## Authentication

The REST API is protected by API tokens. To create a token, you must go to the
settings page in the UI and create a token.

The token must be provided using the `Authorization: Bearer <token>` header.
For example, you could list devices using curl with:

```
 $ curl -H "Authorization: Bearer <your token>" http://localhost:8000/devices
```

## API Documentation

Each release of this project includes Swagger documentation for both the
REST API, `api_swagger.yaml`, and the device gateway API,
`gateway_swagger.yaml`. They are available [here](https://github.com/foundriesio/update-server/releases/latest)

They can also be generated from source by running `make swagger`.

## Denied devices

Deleting a device (`DELETE /devices/{uuid}`) adds it to the denied list: the
device record is retained in the database so the backend can reject further
mTLS connections from that UUID/pubkey pair. Two endpoints manage this list:

 * `GET /denied-devices` returns a JSON array of UUIDs currently on the denied
   list. Requires the `devices:read` scope.
 * `DELETE /denied-devices/{uuid}` removes a device from the denied list,
   allowing it to connect again. Returns 404 if the device is not on the
   denied list. Requires the `devices:delete` scope.

***NOTE***: Removing a device from the denied list recovers its server record
only. The device's stored data (configs, update events, and apps states) is
destroyed when the device is deleted and is **not** recovered.

***NOTE***: Order of operations matters when a device's key changes. The server
record retains the public key the device had when it was added to the denied
list, and the gateway does not currently support key rotation. So a device that
regenerates its keypair while denied (for example after a factory reset) cannot
be brought back via `DELETE /denied-devices/{uuid}`: once removed from the
denied list it presents a new key that no longer matches the stored one, and the
gateway rejects its connections. Because the record still exists, the gateway
will also not auto-enrol it afresh. Such a device must instead be fully deleted
and re-enrolled from scratch under a new connection.
