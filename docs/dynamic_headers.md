# Dynamic Headers Forwarding

## Forward Headers from DDN Engine

Headers will be forwarded forth and back between the DDN engine and data connectors:
- `Client -> DDN Engine -> Data Connector`: is usually used for forwarding Cookie or OAuth2 access tokens from clients.
- `Data Connector -> DDN Engine -> Client`: forward headers from the external HTTP execution back to the client.

### Forward Request Headers

- Enable `forwardHeaders` in the `config.yaml` file of the connector directory, and define the name of the `headers` argument field.

```yaml
# ...
forwardHeaders:
  enabled: true
  argumentField: headers
```

- Introspect the connector to update the relevant schema. 

```sh
ddn connector introspect \<connector-name\>
```

- Finally, add the headers argument, which you defined above, to the `argumentPresets` in `DataConnectorLink` metadata with allowed HTTP headers. For instance:  

```yaml
kind: DataConnectorLink
version: v1
definition:
  name: my_api
  # ...
  argumentPresets:
    - argument: headers
      value:
        httpHeaders:
          forward:
            - Cookie
          additional: {}
```

### Forward Response Headers

- Enable `forwardHeaders` in the `config.yaml` file of the connector directory, and configure `responseHeaders`.       
  - `headersField` and `resultField` field wrappers will be added in the connector schema. 
  - `forwardHeaders` array is allowed headers that will be forwarded back.

```yaml
# ...
forwardHeaders:
  enabled: true
  responseHeaders: 
    headersField: headers
    resultField: response
    forwardHeaders:
      - X-Test-Header
      - X-Test-ResponseHeader
```

- Introspect the connector to update the relevant schema. 

```sh
ddn connector introspect \<connector-name\>
```

- Finally, add the exact `responseHeaders` object which you defined above to `DataConnectorLink` metadata. For instance:  

```yaml
kind: DataConnectorLink
version: v1
definition:
  name: my_api
  # ...
  responseHeaders:
    headersField: headers
    resultField: response
    forwardHeaders:
      - X-Test-Header
      - X-Test-ResponseHeader
```

## Forward Headers from Pre-NDC Request Plugin
 
You can use a [Pre-NDC Request Plugin](https://hasura.io/docs/3.0/plugins/introduction#pre-ndc-request-plugin) to modify the request, and add dynamic headers in runtime via `request_arguments.headers` field, which is a string map. Those headers will be merged into the HTTP request headers before being sent to external services.

> See the full example at [Pre-NDC Request Plugin Request](https://hasura.io/docs/3.0/plugins/introduction#example-configuration)
 
```json
{
  // ...
  "ndcRequest": {
    // ...
    "request_arguments": {
        "headers": {
            "Authorization": "Bearer <token>",
            "X-Custom-Header": "foo"
        }
    }
  }
}
```
