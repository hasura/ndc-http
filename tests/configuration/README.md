# HTTP connector

## Environment Variables

The connector plugin can't automatically add environment variables. Therefore, you need to manually add the required environment variables.

```bash
ddn connector env add --connector ./tests/configuration/connector.yaml --ENV_NAME=value
```

### https://raw.githubusercontent.com/hasura/ndc-http/refs/heads/main/connector/testdata/jsonplaceholder/swagger.json

| Name | Type | Default |
| ---- | ---- | ------- |
| SERVER_URL | string | https://jsonplaceholder.typicode.com |


## Advanced Configurations

Read more at [ndc-http/docs](https://github.com/hasura/ndc-http/blob/main/docs).
