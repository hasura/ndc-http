WARNING:

  * Authorization header must be forwarded for the following authentication schemes: [cookie openIdConnect]
    See https://github.com/hasura/ndc-http/blob/main/docs/authentication.md#headers-forwarding for more information.

  testdata/validation/connector/http/schema.yaml
  
    * Make sure that the X-Pet-Status header is added to the header forwarding list.

Environment Variables:
  Make sure that the following environment variable mappings were added to your subgraph configuration (with subgraph prefixes such as APP_MYAPI_):

    - CAT_PET_HEADER
    - CAT_STORE_CA_PEM
    - CAT_STORE_CERT_PEM
    - CAT_STORE_KEY_PEM
    - CAT_STORE_URL
    - DEFAULT_PET_NAME
    - OAUTH2_CLIENT_ID
    - OAUTH2_CLIENT_SECRET
    - PET_STORE_API_KEY
    - PET_STORE_BEARER_TOKEN
    - PET_STORE_CA_PEM
    - PET_STORE_CERT_PEM
    - PET_STORE_KEY_PEM
    - PET_STORE_TEST_HEADER
    - PET_STORE_URL

  Use the DDN CLI to add environment variables if you haven't added them yet:

    ddn connector env add \
      --env CAT_PET_HEADER=<value> \
      --env CAT_STORE_CA_PEM=<value> \
      --env CAT_STORE_CERT_PEM=<value> \
      --env CAT_STORE_KEY_PEM=<value> \
      --env CAT_STORE_URL=<value> \
      --env DEFAULT_PET_NAME=<value> \
      --env OAUTH2_CLIENT_ID=<value> \
      --env OAUTH2_CLIENT_SECRET=<value> \
      --env PET_STORE_API_KEY=<value> \
      --env PET_STORE_BEARER_TOKEN=<value> \
      --env PET_STORE_CA_PEM=<value> \
      --env PET_STORE_CERT_PEM=<value> \
      --env PET_STORE_KEY_PEM=<value> \
      --env PET_STORE_TEST_HEADER=<value> \
      --env PET_STORE_URL=<value> \
      --connector testdata/validation/connector/http/connector.yaml