WARNING:

  * Authorization header must be forwarded for the following authentication schemes: [cookie openIdConnect]
    See https://github.com/hasura/ndc-http/blob/main/docs/authentication.md#headers-forwarding for more information.

  testdata/validation/connector/http/schema.yaml
  
    * Make sure that the X-Pet-Status header is added to the header forwarding list.

Environment Variables:
  Make sure that the following environment variables were added to your subgraph configuration:

  ``` testdata/validation/connector/http/docker.yaml
  services:
    app_myapi:
      environment:
        CAT_PET_HEADER: $APP_MYAPI_CAT_PET_HEADER
        CAT_STORE_CA_PEM: $APP_MYAPI_CAT_STORE_CA_PEM
        CAT_STORE_CERT_PEM: $APP_MYAPI_CAT_STORE_CERT_PEM
        CAT_STORE_KEY_PEM: $APP_MYAPI_CAT_STORE_KEY_PEM
        CAT_STORE_URL: $APP_MYAPI_CAT_STORE_URL
        DEFAULT_PET_NAME: $APP_MYAPI_DEFAULT_PET_NAME
        OAUTH2_CLIENT_ID: $APP_MYAPI_OAUTH2_CLIENT_ID
        OAUTH2_CLIENT_SECRET: $APP_MYAPI_OAUTH2_CLIENT_SECRET
        PET_STORE_API_KEY: $APP_MYAPI_PET_STORE_API_KEY
        PET_STORE_BEARER_TOKEN: $APP_MYAPI_PET_STORE_BEARER_TOKEN
        PET_STORE_CA_PEM: $APP_MYAPI_PET_STORE_CA_PEM
        PET_STORE_CERT_PEM: $APP_MYAPI_PET_STORE_CERT_PEM
        PET_STORE_KEY_PEM: $APP_MYAPI_PET_STORE_KEY_PEM
        PET_STORE_TEST_HEADER: $APP_MYAPI_PET_STORE_TEST_HEADER
        PET_STORE_URL: $APP_MYAPI_PET_STORE_URL
        # ...

  ```

  ``` testdata/validation/connector/http/connector.yaml
  envMapping:
    CAT_PET_HEADER:
      fromEnv: APP_MYAPI_CAT_PET_HEADER
    CAT_STORE_CA_PEM:
      fromEnv: APP_MYAPI_CAT_STORE_CA_PEM
    CAT_STORE_CERT_PEM:
      fromEnv: APP_MYAPI_CAT_STORE_CERT_PEM
    CAT_STORE_KEY_PEM:
      fromEnv: APP_MYAPI_CAT_STORE_KEY_PEM
    CAT_STORE_URL:
      fromEnv: APP_MYAPI_CAT_STORE_URL
    DEFAULT_PET_NAME:
      fromEnv: APP_MYAPI_DEFAULT_PET_NAME
    OAUTH2_CLIENT_ID:
      fromEnv: APP_MYAPI_OAUTH2_CLIENT_ID
    OAUTH2_CLIENT_SECRET:
      fromEnv: APP_MYAPI_OAUTH2_CLIENT_SECRET
    PET_STORE_API_KEY:
      fromEnv: APP_MYAPI_PET_STORE_API_KEY
    PET_STORE_BEARER_TOKEN:
      fromEnv: APP_MYAPI_PET_STORE_BEARER_TOKEN
    PET_STORE_CA_PEM:
      fromEnv: APP_MYAPI_PET_STORE_CA_PEM
    PET_STORE_CERT_PEM:
      fromEnv: APP_MYAPI_PET_STORE_CERT_PEM
    PET_STORE_KEY_PEM:
      fromEnv: APP_MYAPI_PET_STORE_KEY_PEM
    PET_STORE_TEST_HEADER:
      fromEnv: APP_MYAPI_PET_STORE_TEST_HEADER
    PET_STORE_URL:
      fromEnv: APP_MYAPI_PET_STORE_URL
    # ...

  ```

  ``` .env
  APP_MYAPI_CAT_PET_HEADER=
  APP_MYAPI_CAT_STORE_CA_PEM=
  APP_MYAPI_CAT_STORE_CERT_PEM=
  APP_MYAPI_CAT_STORE_KEY_PEM=
  APP_MYAPI_CAT_STORE_URL=
  APP_MYAPI_DEFAULT_PET_NAME=
  APP_MYAPI_OAUTH2_CLIENT_ID=
  APP_MYAPI_OAUTH2_CLIENT_SECRET=
  APP_MYAPI_PET_STORE_API_KEY=
  APP_MYAPI_PET_STORE_BEARER_TOKEN=
  APP_MYAPI_PET_STORE_CA_PEM=
  APP_MYAPI_PET_STORE_CERT_PEM=
  APP_MYAPI_PET_STORE_KEY_PEM=
  APP_MYAPI_PET_STORE_TEST_HEADER=
  APP_MYAPI_PET_STORE_URL=
  # ...
  
  ```
