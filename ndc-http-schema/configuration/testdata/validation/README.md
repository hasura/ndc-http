# HTTP connector

## Environment Variables

The connector plugin can't automatically add environment variables. Therefore, you need to manually add the required environment variables.

```bash
ddn connector env add --connector testdata/validation/connector/http/connector.yaml --ENV_NAME=value
```

### testdata/validation/connector/http/schema.yaml

| Name | Type | Default |
| ---- | ---- | ------- |
| DEFAULT_PET_NAME |  |  |
| OAUTH2_CLIENT_ID | string |  |
| OAUTH2_CLIENT_SECRET | string |  |
| PET_STORE_API_KEY | string |  |
| PET_STORE_BEARER_TOKEN | string |  |
| PET_STORE_CA_FILE | string |  |
| PET_STORE_CA_PEM | string |  |
| PET_STORE_CERT_FILE | string |  |
| PET_STORE_CERT_PEM | string |  |
| PET_STORE_INSECURE_SKIP_VERIFY | boolean | false |
| PET_STORE_KEY_FILE | string |  |
| PET_STORE_KEY_PEM | string |  |
| PET_STORE_TEST_HEADER | string |  |
| PET_STORE_URL | string |  |


### testdata/validation/connector/http/schema2.yaml

| Name | Type | Default |
| ---- | ---- | ------- |
| CAT_PET_HEADER | string |  |
| CAT_STORE_CA_FILE | string |  |
| CAT_STORE_CA_PEM | string |  |
| CAT_STORE_CERT_FILE | string |  |
| CAT_STORE_CERT_PEM | string |  |
| CAT_STORE_INSECURE_SKIP_VERIFY | boolean | false |
| CAT_STORE_KEY_FILE | string |  |
| CAT_STORE_KEY_PEM | string |  |
| CAT_STORE_URL | string |  |


## Forwarding Headers

The following headers should be forwarded from the engine:

- Cookie
- X-Pet-Status

Check if you have already enabled header forwarding settings in the `config.yaml` file:

```yaml
forwardHeaders:
  enabled: true
  argumentField: headers
```

And check if you already configured argument presets in `testdata/validation/metadata/app.yaml`:

```yaml
kind: DataConnectorLink
version: v1
definition:
  argumentPresets:
    - argument: headers
      value:
        httpHeaders:
          forward:
            - Cookie
            - X-Pet-Status
            
```
## Advanced Configurations

Read more at [ndc-http/docs](https://github.com/hasura/ndc-http/blob/main/docs).