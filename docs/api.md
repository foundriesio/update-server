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
