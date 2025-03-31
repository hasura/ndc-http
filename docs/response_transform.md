# Transform Response

You can transform the default body of your HTTP API response by configuring a response transform template. The HTTP connector helps transform the original schema to fit your response template as well as runtime results.

## Configuration

The configuration is in the `settings` of each schema file. You need to add patches with the `responseTransforms` settings:

```yaml
# config.yaml
files:
  - file: https://raw.githubusercontent.com/hasura/ndc-http/main/connector/testdata/jsonplaceholder/swagger.json
    spec: oas2
    patchAfter:
      - path: response-transform.yaml
        strategy: merge
```

```yaml
# response-transform.yaml
settings:
  responseTransforms:
    - targets: [query1]
      body: $.data
```

The `responseTransforms` accepts a list of transformation pipelines. Each element is an object with the following properties:

- `targets`: list of operations to be applied. If the target field is empty the connector will try to evaluate all operations.
- `body`: the body template will be transformed. You can use the JSON path to pick values from the original response.

The transformation pipelines are executed in sequence. Therefore you can compose many transformations into the same operation.

```yaml
settings:
  responseTransforms:
    - targets: [query1]
      body: $.category
    - targets: ["query1"]
      body: $.name
```
