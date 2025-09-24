package connector

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hasura/ndc-http/connector/internal"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/v2/connector"
	"github.com/hasura/ndc-sdk-go/v2/schema"
	"gotest.tools/v3/assert"
)

func TestHTTPConnectorAuthentication(t *testing.T) {
	apiKey := "random_api_key"
	bearerToken := "random_bearer_token"
	// slog.SetLogLoggerLevel(slog.LevelDebug)
	state := createMockServer(t, apiKey, bearerToken)
	defer state.Server.Close()

	t.Setenv("PET_STORE_URL", state.Server.URL)
	t.Setenv("PET_STORE_API_KEY", apiKey)
	t.Setenv("PET_STORE_BEARER_TOKEN", bearerToken)
	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/auth",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	findPetsBody := []byte(`{
		"collection": "findPets",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {},
		"collection_relationships": {}
	}`)

	findPetsBodyWithRequestArguments := []byte(`{
		"collection": "findPets",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {},
		"collection_relationships": {},
		"request_arguments": {
			"headers": {
				"Api_key": "unauthorized",
				"foo": "bar"
			}
		}
	}`)

	t.Run("auth_default_explain", func(t *testing.T) {
		res, err := http.Post(
			fmt.Sprintf("%s/query/explain", testServer.URL),
			"application/json",
			bytes.NewBuffer(findPetsBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{
				"url":     state.Server.URL + "/pet",
				"headers": `{"Accept":["application/json"],"Api_key":["ran*******(14)"],"Content-Type":["application/json"]}`,
			},
		})
	})

	t.Run("query_with_request_arguments_explain", func(t *testing.T) {
		res, err := http.Post(
			fmt.Sprintf("%s/query/explain", testServer.URL),
			"application/json",
			bytes.NewBuffer(findPetsBodyWithRequestArguments),
		)

		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{
				"url":     state.Server.URL + "/pet",
				"headers": `{"Accept":["application/json"],"Api_key":["una*******(12)"],"Content-Type":["application/json"],"Foo":["bar"]}`,
			},
		})
	})

	t.Run("auth_default", func(t *testing.T) {
		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(findPetsBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{
					{
						"__value": map[string]any{
							"headers": map[string]any{
								"Content-Type": string("application/json"),
							},
							"response": []any{
								map[string]any{
									"id":           float64(1),
									"custom_field": `[{"foo":"bar"}]`,
								},
							},
						},
					},
				},
			},
		})
	})

	t.Run("query_with_request_arguments", func(t *testing.T) {
		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(findPetsBodyWithRequestArguments),
		)
		assert.NilError(t, err)

		assertHTTPResponse(t, res, http.StatusUnprocessableEntity, schema.ErrorResponse{
			Message: "422 Unprocessable Entity",
			Details: map[string]any{
				"error": "unauthorized",
			},
		})
	})

	addPetBody := []byte(`{
		"operations": [
			{
				"type": "procedure",
				"name": "addPet",
				"arguments": {
					"body": {
						"name": "pet"
					}
				},
				"fields": {
					"type": "object",
					"fields": {
						"headers": {
							"column": "headers",
							"type": "column"
						},
						"response": {
							"column": "response",
							"type": "column"
						}
					}
				}
			}
		],
		"collection_relationships": {}
	}`)

	addPetBodyWithRequestArguments := []byte(`{
		"operations": [
			{
				"type": "procedure",
				"name": "addPet",
				"arguments": {
					"body": {
						"name": "pet"
					}
				},
				"fields": {
					"type": "object",
					"fields": {
						"headers": {
							"column": "headers",
							"type": "column"
						},
						"response": {
							"column": "response",
							"type": "column"
						}
					}
				}
			}
		],
		"collection_relationships": {},
		"request_arguments": {
			"headers": {
				"Api_key": "unauthorized",
				"foo": "bar"
			}
		}
	}`)

	t.Run("mutation_auth_api_key_explain", func(t *testing.T) {
		res, err := http.Post(
			fmt.Sprintf("%s/mutation/explain", testServer.URL),
			"application/json",
			bytes.NewBuffer(addPetBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{
				"url":     state.Server.URL + "/pet",
				"headers": `{"Accept":["application/json"],"Api_key":["ran*******(14)"],"Content-Type":["application/json"]}`,
				"body":    "{\"name\":\"pet\"}",
			},
		})
	})

	t.Run("mutation_with_request_arguments_explain", func(t *testing.T) {
		res, err := http.Post(
			fmt.Sprintf("%s/mutation/explain", testServer.URL),
			"application/json",
			bytes.NewBuffer(addPetBodyWithRequestArguments),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{
				"url":     state.Server.URL + "/pet",
				"headers": `{"Accept":["application/json"],"Api_key":["una*******(12)"],"Content-Type":["application/json"],"Foo":["bar"]}`,
				"body":    "{\"name\":\"pet\"}",
			},
		})
	})

	t.Run("mutation_auth_api_key", func(t *testing.T) {
		res, err := http.Post(
			fmt.Sprintf("%s/mutation", testServer.URL),
			"application/json",
			bytes.NewBuffer(addPetBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{
					"headers": map[string]any{
						"Content-Type": string("application/json"),
					},
					"response": map[string]any{
						"custom_field": string(`[{"foo":"bar"}]`),
						"id":           float64(1),
					},
				}).Encode(),
			},
		})
	})

	t.Run("mutation_with_request_arguments", func(t *testing.T) {
		res, err := http.Post(
			fmt.Sprintf("%s/mutation", testServer.URL),
			"application/json",
			bytes.NewBuffer(addPetBodyWithRequestArguments),
		)

		assert.NilError(t, err)

		assertHTTPResponse(t, res, http.StatusUnprocessableEntity, schema.ErrorResponse{
			Message: "422 Unprocessable Entity",
			Details: map[string]any{
				"error": "unauthorized",
			},
		})
	})

	authBearerBody := []byte(`{
		"collection": "findPetsByStatus",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {
			"headers": {
				"type": "literal",
				"value": {
					"X-Custom-Header": "This is a test"
				}
			},
			"status": {
				"type": "literal",
				"value": "available"
			}
		},
		"collection_relationships": {}
	}`)

	t.Run("auth_bearer_explain", func(t *testing.T) {
		res, err := http.Post(
			fmt.Sprintf("%s/query/explain", testServer.URL),
			"application/json",
			bytes.NewBuffer(authBearerBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{
				"url":     state.Server.URL + "/pet/findByStatus?status=available",
				"headers": `{"Accept":["application/json"],"Authorization":["Bearer ran*******(19)"],"Content-Type":["application/json"],"X-Custom-Header":["This is a test"]}`,
			},
		})
	})

	t.Run("auth_bearer", func(t *testing.T) {
		for range 2 {
			res, err := http.Post(
				fmt.Sprintf("%s/query", testServer.URL),
				"application/json",
				bytes.NewBuffer(authBearerBody),
			)
			assert.NilError(t, err)
			assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
				{
					Rows: []map[string]any{
						{
							"__value": map[string]any{
								"headers": map[string]any{
									"Content-Type": string("application/json"),
								},
								"response": []any{map[string]any{}},
							},
						},
					},
				},
			})
		}
	})

	t.Run("auth_cookie", func(t *testing.T) {
		requestBody := []byte(`{
		"collection": "findPetsCookie",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {
			"headers": { 
				"type": "literal", 
				"value": {
					"Cookie": "auth=auth_token"
				} 
			}
		},
		"collection_relationships": {}
	}`)

		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(requestBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{
					{
						"__value": map[string]any{
							"headers": map[string]any{
								"Content-Type": string("application/json"),
							},
							"response": []any{map[string]any{}},
						},
					},
				},
			},
		})
	})

	t.Run("auth_oidc", func(t *testing.T) {
		addPetOidcBody := []byte(`{
			"operations": [
				{
					"type": "procedure",
					"name": "addPetOidc",
					"arguments": {
						"headers": {
							"Authorization": "Bearer random_token"
						},
						"body": {
							"name": "pet"
						}
					},
					"fields": {
						"type": "object",
						"fields": {
							"headers": {
								"column": "headers",
								"type": "column"
							},
							"response": {
								"column": "response",
								"type": "column"
							}
						}
					}
				}
			],
			"collection_relationships": {}
		}`)
		res, err := http.Post(
			fmt.Sprintf("%s/mutation", testServer.URL),
			"application/json",
			bytes.NewBuffer(addPetOidcBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{
					"headers": map[string]any{
						"Content-Type": string("application/json"),
					},
					"response": map[string]any{},
				}).Encode(),
			},
		})
	})

	t.Run("retry", func(t *testing.T) {
		getReqBody := func(retryAfter bool) []byte {
			return []byte(fmt.Sprintf(`{
				"collection": "petRetry",
				"query": {
					"fields": {
						"__value": {
							"type": "column",
							"column": "__value"
						}
					}
				},
				"arguments": {
					"retry_after": {
						"type": "literal",
						"value": %t
					}
				},
				"collection_relationships": {}
			}`, retryAfter))
		}

		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(getReqBody(false)),
		)
		assert.NilError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, res.StatusCode)
		assert.Equal(t, state.RetryCount, int32(2))

		atomic.StoreInt32(&state.RetryCount, 0)
		start := time.Now()
		res, err = http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(getReqBody(true)),
		)
		assert.NilError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, res.StatusCode)
		assert.Equal(t, state.RetryCount, int32(2))

		delay := time.Since(start)
		log.Println("delay", delay)
		assert.Assert(t, delay >= time.Second && delay <= 2*time.Second)
	})

	t.Run("query-form-non-explode", func(t *testing.T) {
		rawBody := []byte(`{
			"collection": "findPetsByTags",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {
				"tags": {
					"type": "literal",
					"value": ["foo", "bar"]
				}
			},
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(rawBody),
		)
		assert.NilError(t, err)
		res.Body.Close()

		assert.Equal(t, res.StatusCode, http.StatusOK)
	})

	t.Run("encoding-ndjson", func(t *testing.T) {
		reqBody := []byte(`{
			"operations": [
				{
					"type": "procedure",
					"name": "createModel",
					"arguments": {
						"body": {
							"model": "gpt3.5"
						}
					},
					"fields": {
						"fields": {
							"headers": {
								"column": "headers",
								"type": "column"
							},
							"response": {
								"column": "response",
								"type": "column",
								"fields": {
									"type": "array",
									"fields": {
										"fields": {
											"completed": {
												"column": "completed",
												"type": "column"
											},
											"status": {
												"column": "status",
												"type": "column"
											}
										},
										"type": "object"
									}
								}
							}
						},
						"type": "object"
					}
				}
			],
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/mutation", testServer.URL),
			"application/json",
			bytes.NewBuffer(reqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{
					"headers": map[string]any{"Content-Type": string("application/x-ndjson")},
					"response": []any{
						map[string]any{"completed": float64(1), "status": string("OK")},
						map[string]any{"completed": float64(0), "status": string("FAILED")},
					},
				}).Encode(),
			},
		})
	})

	t.Run("encoding-xml", func(t *testing.T) {
		reqBody := []byte(`{
			"operations": [
				{
					"type": "procedure",
					"name": "putPetXml",
					"arguments": {
						"body": {
							"id":   10,
							"name": "doggie",
							"category": {
								"id":   1,
								"name": "Dogs"
							},
							"photoUrls": ["string"],
							"tags": [
								{
									"id":   0,
									"name": "string"
								}
							],
							"status": "available"
						}
					},
					"fields": {
						"fields": {
							"headers": {
								"column": "headers",
								"type": "column"
							},
							"response": {
								"column": "response",
								"type": "column"
							}
						},
						"type": "object"
					}
				}
			],
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/mutation", testServer.URL),
			"application/json",
			bytes.NewBuffer(reqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{
					"headers":  map[string]any{"Content-Type": string("application/xml")},
					"response": "Dogs",
				}).Encode(),
			},
		})
	})

	t.Run("stringify_json", func(t *testing.T) {
		reqBody := []byte(`{
		"operations": [
			{
				"type": "procedure",
				"name": "postPetStringifyJson",
				"arguments": {
					"body": {
						"name": "dog",
						"custom_field": [{
							"user_id": 1,
							"foo": "baz",
							"active": true,
							"object": {
								"hello": "world"
							}
						}]
					}
				},
				"fields": {
					"type": "object",
					"fields": {
						"headers": {
							"column": "headers",
							"type": "column"
						},
						"response": {
							"column": "response",
							"type": "column"
						}
					}
				}
			}
		],
		"collection_relationships": {}
	}`)

		res, err := http.Post(
			fmt.Sprintf("%s/mutation", testServer.URL),
			"application/json",
			bytes.NewBuffer(reqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{
					"headers": map[string]any{
						"Content-Type": string("application/json"),
					},
					"response": map[string]any{
						"custom_field": string(`[{"foo":"bar"}]`),
						"id":           float64(1),
					},
				}).Encode(),
			},
		})
	})

	t.Run("http_raw_stringify_json", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{
		"operations": [
			{
				"type": "procedure",
				"name": "sendHttpRequest",
				"arguments": {
					"url": "%s/pet/stringify-json",
					"method": "post",
					"body": {
						"name": "dog",
						"custom_field": [{
							"user_id": 1,
							"foo": "baz",
							"active": true,
							"object": {
								"hello": "world"
							}
						}]
					}
				},
				"fields": {
					"type": "object",
					"fields": {
						"headers": {
							"column": "headers",
							"type": "column"
						},
						"response": {
							"column": "response",
							"type": "column"
						}
					}
				}
			}
		],
		"collection_relationships": {}
	}`, state.Server.URL))

		res, err := http.Post(
			fmt.Sprintf("%s/mutation", testServer.URL),
			"application/json",
			bytes.NewBuffer(reqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{
					"headers": map[string]any{
						"Content-Type": string("application/json"),
					},
					"response": `{"id":"1","custom_field":[{"foo":"bar"}]}`,
				}).Encode(),
			},
		})
	})
}

func TestHTTPConnector_distribution(t *testing.T) {
	apiKey := "random_api_key"
	bearerToken := "random_bearer_token"

	type distributedResultData struct {
		Name string `json:"name"`
	}

	expectedResults := []internal.DistributedResult[[]distributedResultData]{
		{
			Server: "cat",
			Data: []distributedResultData{
				{Name: "cat"},
			},
		},
		{
			Server: "dog",
			Data: []distributedResultData{
				{Name: "dog"},
			},
		},
	}

	t.Setenv("PET_STORE_API_KEY", apiKey)
	t.Setenv("PET_STORE_BEARER_TOKEN", bearerToken)

	t.Run("distributed_sequence", func(t *testing.T) {
		mock := mockDistributedServer{}
		server := mock.createServer(t)
		defer server.Close()

		t.Setenv("PET_STORE_DOG_URL", fmt.Sprintf("%s/dog", server.URL))
		t.Setenv("PET_STORE_CAT_URL", fmt.Sprintf("%s/cat", server.URL))

		rc := NewHTTPConnector()
		connServer, err := connector.NewServer(rc, &connector.ServerOptions{
			Configuration: "testdata/patch",
		}, connector.WithoutRecovery())
		assert.NilError(t, err)

		testServer := connServer.BuildTestServer()
		defer testServer.Close()

		assert.Equal(t, uint(30), rc.metadata[0].Runtime.Timeout)
		assert.Equal(t, uint(2), rc.metadata[0].Runtime.Retry.Times)
		assert.Equal(t, uint(1000), rc.metadata[0].Runtime.Retry.Delay)
		assert.Equal(t, uint(1000), rc.metadata[0].Runtime.Retry.Delay)
		assert.DeepEqual(t, []int{429, 500}, rc.metadata[0].Runtime.Retry.HTTPStatus)

		reqBody := []byte(`{
			"collection": "findPetsDistributed",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {},
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(reqBody),
		)
		assert.NilError(t, err)

		defer res.Body.Close()

		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal("failed to read response body")
		}

		if res.StatusCode != 200 {
			t.Fatalf("expected status %d, got %d. Body: %s", 200, res.StatusCode, string(bodyBytes))
		}

		var body []struct {
			Rows []struct {
				Value struct {
					Headers  map[string]string `json:"headers"`
					Response struct {
						Errors  []internal.DistributedError                           `json:"errors"`
						Results []internal.DistributedResult[[]distributedResultData] `json:"results"`
					} `json:"response"`
				} `json:"__value"`
			} `json:"rows"`
		}
		if err = json.Unmarshal(bodyBytes, &body); err != nil {
			t.Errorf("failed to decode json body, got error: %s; body: %s", err, string(bodyBytes))
		}

		assert.Equal(t, 1, len(body))
		row := body[0].Rows[0]
		assert.Equal(t, 0, len(row.Value.Response.Errors))
		assert.Equal(t, 2, len(row.Value.Response.Results))

		slices.SortFunc(
			row.Value.Response.Results,
			func(a internal.DistributedResult[[]distributedResultData], b internal.DistributedResult[[]distributedResultData]) int {
				return strings.Compare(a.Server, b.Server)
			},
		)

		assert.DeepEqual(t, expectedResults, row.Value.Response.Results)

		assert.Equal(t, int32(1), mock.catCount)
		assert.Equal(t, int32(1), mock.dogCount)
	})

	t.Run("distributed_parallel", func(t *testing.T) {
		mock := mockDistributedServer{}
		server := mock.createServer(t)
		defer server.Close()

		t.Setenv("PET_STORE_DOG_URL", fmt.Sprintf("%s/dog", server.URL))
		t.Setenv("PET_STORE_CAT_URL", fmt.Sprintf("%s/cat", server.URL))
		rc := NewHTTPConnector()
		connServer, err := connector.NewServer(rc, &connector.ServerOptions{
			Configuration: "testdata/patch",
		}, connector.WithoutRecovery())
		assert.NilError(t, err)

		testServer := connServer.BuildTestServer()
		defer testServer.Close()

		reqBody := []byte(`{
			"operations": [
				{
					"type": "procedure",
					"name": "addPetDistributed",
					"arguments": {
						"body": {
							"name": "pet"
						},
						"httpOptions": {
							"parallel": true
						}
					}
				}
			],
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/mutation", testServer.URL),
			"application/json",
			bytes.NewBuffer(reqBody),
		)
		assert.NilError(t, err)

		defer res.Body.Close()

		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal("failed to read response body")
		}

		if res.StatusCode != 200 {
			t.Fatalf("expected status %d, got %d. Body: %s", 200, res.StatusCode, string(bodyBytes))
		}

		var body struct {
			OperationResults []struct {
				Result struct {
					Headers  map[string]string `json:"headers"`
					Response struct {
						Errors  []internal.DistributedError                           `json:"errors"`
						Results []internal.DistributedResult[[]distributedResultData] `json:"results"`
					} `json:"response"`
				} `json:"result"`
			} `json:"operation_results"`
		}
		if err = json.Unmarshal(bodyBytes, &body); err != nil {
			t.Errorf("failed to decode json body, got error: %s; body: %s", err, string(bodyBytes))
		}

		row := body.OperationResults[0].Result
		assert.Equal(t, 0, len(row.Response.Errors))
		assert.Equal(t, 2, len(row.Response.Results))

		slices.SortFunc(
			row.Response.Results,
			func(a internal.DistributedResult[[]distributedResultData], b internal.DistributedResult[[]distributedResultData]) int {
				return strings.Compare(a.Server, b.Server)
			},
		)

		assert.DeepEqual(t, expectedResults, row.Response.Results)
		assert.Equal(t, int32(1), mock.catCount)
		assert.Equal(t, int32(1), mock.dogCount)
	})

	t.Run("specify_server", func(t *testing.T) {
		mock := mockDistributedServer{}
		server := mock.createServer(t)
		defer server.Close()

		t.Setenv("PET_STORE_DOG_URL", fmt.Sprintf("%s/dog", server.URL))
		t.Setenv("PET_STORE_CAT_URL", fmt.Sprintf("%s/cat", server.URL))

		rc := NewHTTPConnector()
		connServer, err := connector.NewServer(rc, &connector.ServerOptions{
			Configuration: "testdata/patch",
		}, connector.WithoutRecovery())
		assert.NilError(t, err)

		testServer := connServer.BuildTestServer()
		defer testServer.Close()

		reqBody := []byte(`{
			"collection": "findPetsDistributed",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {
				"httpOptions": {
					"type": "literal",
					"value": {
						"servers": ["cat"]
					}
				}
			},
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(reqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{
					{"__value": map[string]any{
						"headers": map[string]any{
							"Content-Length": "17",
							"Content-Type":   "application/json",
						},
						"response": map[string]any{
							"errors": []any{},
							"results": []any{
								map[string]any{
									"data": []any{
										map[string]any{"name": "cat"},
									},
									"server": string("cat"),
								},
							},
						},
					}},
				},
			},
		})
		assert.Equal(t, int32(1), mock.catCount)
		assert.Equal(t, int32(0), mock.dogCount)
	})
}

func TestHTTPConnector_multiSchemas(t *testing.T) {
	mock := mockMultiSchemaServer{}
	server := mock.createServer()
	defer server.Close()

	t.Setenv("CAT_STORE_URL", fmt.Sprintf("%s/cat", server.URL))
	t.Setenv("DOG_STORE_URL", fmt.Sprintf("%s/dog", server.URL))

	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/multi-schemas",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	// slog.SetLogLoggerLevel(slog.LevelDebug)

	reqBody := []byte(`{
			"collection": "findCats",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {},
			"collection_relationships": {}
		}`)

	res, err := http.Post(
		fmt.Sprintf("%s/query", testServer.URL),
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	assert.NilError(t, err)
	assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
		{
			Rows: []map[string]any{
				{"__value": []any{
					map[string]any{"name": "cat"},
				}},
			},
		},
	})
	assert.Equal(t, int32(1), mock.catCount)
	assert.Equal(t, int32(0), mock.dogCount)

	reqBody = []byte(`{
		"collection": "findDogs",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {},
		"collection_relationships": {}
	}`)

	res, err = http.Post(
		fmt.Sprintf("%s/query", testServer.URL),
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	assert.NilError(t, err)

	assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
		{
			Rows: []map[string]any{
				{"__value": []any{
					map[string]any{
						"name": "dog",
					},
				}},
			},
		},
	})

	assert.Equal(t, int32(1), mock.catCount)
	assert.Equal(t, int32(1), mock.dogCount)
}

type mockServerState struct {
	Server     *httptest.Server
	RetryCount int32
}

func createMockServer(t *testing.T, apiKey string, bearerToken string) *mockServerState {
	t.Helper()

	state := mockServerState{}
	mux := http.NewServeMux()

	writeResponse := func(w http.ResponseWriter, body string) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}

	mux.HandleFunc("/pet", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodPost:
			switch r.Header.Get("api_key") {
			case apiKey:
				petItemStr := `{
					"id": "1",
					"custom_field": [{
						"foo": "bar"
					}]
				}`

				if r.Method == http.MethodGet {
					responseBody := fmt.Sprintf(`[%s]`, petItemStr)
					writeResponse(w, responseBody)

					return
				}

				var postBody any
				err := json.NewDecoder(r.Body).Decode(&postBody)
				assert.NilError(t, err)
				writeResponse(w, petItemStr)
			case "unauthorized":
				w.WriteHeader(http.StatusUnprocessableEntity)
				w.Write([]byte("unauthorized"))
			default:
				t.Errorf("invalid api key, expected %s, got %s", apiKey, r.Header.Get("api_key"))
				t.FailNow()
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/pet/findByStatus", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", bearerToken) {
				t.Fatalf(
					"invalid bearer token, expected %s, got %s",
					bearerToken,
					r.Header.Get("Authorization"),
				)
				return
			}
			if r.Header.Get("X-Custom-Header") != "This is a test" {
				t.Fatalf(
					"invalid X-Custom-Header, expected `This is a test`, got %s",
					r.Header.Get("X-Custom-Header"),
				)
				return
			}

			if r.URL.Query().Encode() != "status=available" {
				t.Fatalf("expected query param: status=available, got: %s", r.URL.Query().Encode())
				return
			}
			writeResponse(w, "[{}]")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/pet/findByTags", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("tags") != "foo,bar" {
				t.Fatalf("expected query param: tags=foo,bar, got: %s", r.URL.Query().Encode())
				return
			}

			writeResponse(w, "[{}]")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/pet/retry", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&state.RetryCount, 1)
		if state.RetryCount > 3 {
			panic("retry count must not be larger than 2")
		}

		if r.URL.Query().Get("retry_after") == "true" {
			switch state.RetryCount {
			case 1:
				w.Header().Set("Retry-After", "1")
			case 2:
				w.Header().Set("Retry-After", time.Now().Add(time.Hour).Format(time.RFC1123))
			}
		}

		w.WriteHeader(http.StatusTooManyRequests)
	})

	mux.HandleFunc("/model", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			user, password, ok := r.BasicAuth()
			if !ok || user != "user" || password != "password" {
				t.Errorf("invalid basic auth, expected user:password, got %s:%s", user, password)
				t.FailNow()
				return
			}

			w.Header().Add("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"completed": 1, "status": "OK"}
{"completed": 0, "status": "FAILED"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/pet/xml", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.Header().Add("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)

			_, _ = w.Write(
				[]byte(
					"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<pet><category><id>1</id><name>Dogs</name></category><id>10</id><name>doggie</name><photoUrls><photoUrl>string</photoUrl></photoUrls><status>available</status><tags><tag><id>0</id><name>string</name></tag></tags></pet>",
				),
			)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/pet/oauth", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if authToken == "" {
				t.Errorf("empty Authorization token")
				t.FailNow()

				return
			}

			tokenBody := "token=" + authToken
			tokenResp, err := http.DefaultClient.Post(
				"http://localhost:4445/admin/oauth2/introspect",
				rest.ContentTypeFormURLEncoded,
				bytes.NewBufferString(tokenBody),
			)
			assert.NilError(t, err)
			assert.Equal(t, http.StatusOK, tokenResp.StatusCode)

			var result struct {
				Active   bool   `json:"active"`
				CLientID string `json:"client_id"`
			}

			assert.NilError(t, json.NewDecoder(tokenResp.Body).Decode(&result))
			assert.Equal(t, "test-client", result.CLientID)
			assert.Equal(t, true, result.Active)

			writeResponse(w, "[{}]")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/pet/cookie", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authCookie, err := r.Cookie("auth")
			assert.NilError(t, err)
			assert.Equal(t, "auth_token", authCookie.Value)
			writeResponse(w, "[{}]")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/pet/oidc", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer random_token" {
				t.Errorf(
					"invalid bearer token, expected: `Authorization: Bearer random_token`, got %s",
					r.Header.Get("Authorization"),
				)
				t.FailNow()
				return
			}
			writeResponse(w, "{}")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodPost:
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/pet/stringify-json", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			respBody := `{"id":"1","custom_field":[{"foo":"bar"}]}`

			expectedBody := map[string]any{
				"name": "dog",
				"custom_field": []any{
					map[string]any{
						"user_id": float64(1),
						"foo":     "baz",
						"active":  true,
						"object": map[string]any{
							"hello": "world",
						},
					},
				},
			}

			var postBody map[string]any
			err := json.NewDecoder(r.Body).Decode(&postBody)
			assert.NilError(t, err)
			assert.DeepEqual(t, expectedBody, postBody)

			writeResponse(w, respBody)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	server := httptest.NewServer(mux)
	state.Server = server

	return &state
}

type mockDistributedServer struct {
	dogCount int32
	catCount int32
}

func (mds *mockDistributedServer) createServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	writeResponse := func(w http.ResponseWriter, data []byte) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}

	createHandler := func(name string, apiKey string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("api_key") != apiKey {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write(
					[]byte(
						fmt.Sprintf(
							`{"message": "invalid api key, expected %s, got %s"}`,
							apiKey,
							r.Header.Get("api_key"),
						),
					),
				)
				return
			}
			switch r.Method {
			case http.MethodGet:
				writeResponse(w, []byte(fmt.Sprintf(`[{"name": "%s"}]`, name)))
			case http.MethodPost:
				rawBody, err := io.ReadAll(r.Body)
				assert.NilError(t, err)

				var body struct {
					Name string `json:"name"`
				}
				// log.Printf("request body: %s", string(rawBody))
				err = json.Unmarshal(rawBody, &body)
				assert.NilError(t, err)
				assert.Equal(t, "pet", body.Name)
				writeResponse(w, []byte(fmt.Sprintf(`[{"name": "%s"}]`, name)))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
		}
	}
	mux.HandleFunc("/cat/pet", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mds.catCount, 1)
		time.Sleep(100 * time.Millisecond)
		createHandler("cat", "cat-secret")(w, r)
	})
	mux.HandleFunc("/dog/pet", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mds.dogCount, 1)
		createHandler("dog", "dog-secret")(w, r)
	})

	return httptest.NewServer(mux)
}

type mockMultiSchemaServer struct {
	dogCount int32
	catCount int32
}

func (mds *mockMultiSchemaServer) createServer() *httptest.Server {
	mux := http.NewServeMux()

	writeResponse := func(w http.ResponseWriter, data []byte) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
	createHandler := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				if r.Header.Get("pet") != name {
					slog.Error(
						fmt.Sprintf(
							"expected r.Header.Get(\"pet\") == %s, got %s",
							name,
							r.Header.Get("pet"),
						),
					)
					w.WriteHeader(http.StatusBadRequest)

					return
				}
				writeResponse(w, []byte(fmt.Sprintf(`[{"name": "%s"}]`, name)))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
		}
	}
	mux.HandleFunc("/cat/cat", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mds.catCount, 1)
		createHandler("cat")(w, r)
	})
	mux.HandleFunc("/dog/dog", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mds.dogCount, 1)
		createHandler("dog")(w, r)
	})

	return httptest.NewServer(mux)
}

func TestConnectorOAuth(t *testing.T) {
	apiKey := "random_api_key"
	bearerToken := "random_bearer_token"
	oauth2ClientID := "test-client"
	oauth2ClientSecret := "randomsecret"
	createClientBody := []byte(fmt.Sprintf(`{
		"client_id": "%s",
		"client_name": "Test client",
		"client_secret": "%s",
		"audience": ["http://hasura.io"],
		"grant_types": ["client_credentials"],
		"response_types": ["code"],
		"scope": "openid read:pets write:pets",
		"token_endpoint_auth_method": "client_secret_post"
	}`, oauth2ClientID, oauth2ClientSecret))

	oauthResp, err := http.DefaultClient.Post(
		"http://localhost:4445/admin/clients",
		"application/json",
		bytes.NewBuffer(createClientBody),
	)
	assert.NilError(t, err)
	defer oauthResp.Body.Close()

	if oauthResp.StatusCode != http.StatusCreated && oauthResp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(oauthResp.Body)
		t.Fatal(string(body))
	}

	state := createMockServer(t, apiKey, bearerToken)
	defer state.Server.Close()

	t.Setenv("PET_STORE_URL", state.Server.URL)
	t.Setenv("PET_STORE_API_KEY", apiKey)
	t.Setenv("PET_STORE_BEARER_TOKEN", bearerToken)
	t.Setenv("OAUTH2_CLIENT_ID", oauth2ClientID)
	t.Setenv("OAUTH2_CLIENT_SECRET", oauth2ClientSecret)
	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/auth",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	findPetsBody := []byte(`{
		"collection": "findPetsOAuth",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {},
		"collection_relationships": {}
	}`)

	res, err := http.Post(
		fmt.Sprintf("%s/query", testServer.URL),
		"application/json",
		bytes.NewBuffer(findPetsBody),
	)
	assert.NilError(t, err)
	assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
		{
			Rows: []map[string]any{
				{
					"__value": map[string]any{
						"headers": map[string]any{
							"Content-Type": string("application/json"),
						},
						"response": []any{map[string]any{}},
					},
				},
			},
		},
	})

	failureBody := []byte(`{
		"collection": "findPetsOAuth",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {
			"httpOptions": {
				"type": "literal",
				"value": {
					"servers": ["1"]
				}
			}
		},
		"collection_relationships": {}
	}`)

	res, err = http.Post(
		fmt.Sprintf("%s/query", testServer.URL),
		"application/json",
		bytes.NewBuffer(failureBody),
	)
	assert.NilError(t, err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, res.StatusCode)

	respBody, err := io.ReadAll(res.Body)
	assert.NilError(t, err)
	assert.Assert(
		t,
		strings.Contains(string(respBody), "oauth2: cannot fetch token: 400 Bad Request"),
	)
}

type mockTLSServer struct {
	counter int
	lock    sync.Mutex
}

func (mtls *mockTLSServer) IncreaseCount() {
	mtls.lock.Lock()
	defer mtls.lock.Unlock()

	mtls.counter++
}

func (mtls *mockTLSServer) Count() int {
	mtls.lock.Lock()
	defer mtls.lock.Unlock()

	return mtls.counter
}

func (mts *mockTLSServer) createMockTLSServer(
	t *testing.T,
	dir string,
	insecure bool,
) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	writeResponse := func(w http.ResponseWriter, body string) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
	mux.HandleFunc("/pet", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			mts.IncreaseCount()
			writeResponse(w, "[]")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	var tlsConfig *tls.Config

	if !insecure {
		// load CA certificate file and add it to list of client CAs
		caCertFile, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
		if err != nil {
			log.Fatalf("error reading CA certificate: %v", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCertFile)

		// Create the TLS Config with the CA pool and enable Client certificate validation
		cert, err := tls.LoadX509KeyPair(
			filepath.Join(dir, "server.crt"),
			filepath.Join(dir, "server.key"),
		)
		assert.NilError(t, err)

		tlsConfig = &tls.Config{
			ClientCAs:    caCertPool,
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}
	}

	server := httptest.NewUnstartedServer(mux)
	server.TLS = tlsConfig
	server.StartTLS()

	return server
}

func TestConnectorTLS(t *testing.T) {
	mockServer := &mockTLSServer{}
	server := mockServer.createMockTLSServer(t, "testdata/tls/certs", false)
	defer server.Close()

	mockServer1 := &mockTLSServer{}
	server1 := mockServer1.createMockTLSServer(t, "testdata/tls/certs_s1", false)
	defer server1.Close()

	t.Setenv("PET_STORE_URL", server.URL)
	t.Setenv("PET_STORE_CA_FILE", filepath.Join("testdata/tls/certs", "ca.crt"))
	t.Setenv("PET_STORE_CERT_FILE", filepath.Join("testdata/tls/certs", "client.crt"))
	t.Setenv("PET_STORE_KEY_FILE", filepath.Join("testdata/tls/certs", "client.key"))

	t.Setenv("PET_STORE_S1_URL", server1.URL)
	caPem, err := os.ReadFile(filepath.Join("testdata/tls/certs_s1", "ca.crt"))
	assert.NilError(t, err)
	caData := base64.StdEncoding.EncodeToString(caPem)
	t.Setenv("PET_STORE_S1_CA_PEM", caData)

	certPem, err := os.ReadFile(filepath.Join("testdata/tls/certs_s1", "client.crt"))
	assert.NilError(t, err)
	certData := base64.StdEncoding.EncodeToString(certPem)
	t.Setenv("PET_STORE_S1_CERT_PEM", certData)

	keyPem, err := os.ReadFile(filepath.Join("testdata/tls/certs_s1", "client.key"))
	assert.NilError(t, err)
	keyData := base64.StdEncoding.EncodeToString(keyPem)
	t.Setenv("PET_STORE_S1_KEY_PEM", keyData)

	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/tls",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	func() {
		findPetsBody := []byte(`{
			"collection": "findPets",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {},
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(findPetsBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{
					{
						"__value": []any{},
					},
				},
			},
		})
	}()

	func() {
		findPetsBody := []byte(`{
			"collection": "findPets",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {
				"httpOptions": {
					"type": "literal",
					"value": {
						"servers": ["1"]
					}
				}
			},
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(findPetsBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{
					{
						"__value": []any{},
					},
				},
			},
		})
	}()

	time.Sleep(time.Second)
	assert.Equal(t, 1, mockServer.Count())
	assert.Equal(t, 1, mockServer1.Count())
}

func TestConnectorTLSInsecure(t *testing.T) {
	mockServer := &mockTLSServer{}
	server := mockServer.createMockTLSServer(t, "testdata/tls/certs", true)
	defer server.Close()

	t.Setenv("PET_STORE_URL", server.URL)
	t.Setenv("PET_STORE_S1_URL", server.URL)
	t.Setenv("PET_STORE_INSECURE_SKIP_VERIFY", "true")

	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/tls",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	func() {
		findPetsBody := []byte(`{
			"collection": "findPets",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {},
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(findPetsBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{
					{
						"__value": []any{},
					},
				},
			},
		})
	}()

	assert.Equal(t, 1, mockServer.Count())
}

func TestConnectorArgumentPresets(t *testing.T) {
	mux := http.NewServeMux()
	writeResponse := func(w http.ResponseWriter, data []byte) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}

	mux.HandleFunc("/pet/findByStatus", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			assert.Equal(t, "active", r.URL.Query().Get("status"))
			writeResponse(w, []byte(`[{"id": 1, "name": "test"}]`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/pet", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var body map[string]any
			assert.NilError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.DeepEqual(t, map[string]any{
				"id":   float64(1),
				"name": "Dog",
				"categories": []any{
					map[string]any{
						"id":   float64(1),
						"name": "mammal",
						"addresses": []any{
							map[string]any{
								"id":   float64(1),
								"name": string("Street 0"),
							},
							map[string]any{
								"id":   float64(2),
								"name": "Street 1",
							},
						},
					},
				},
			}, body)

			writeResponse(w, []byte(`[{"id": 1, "name": "Dog"}]`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	t.Setenv("PET_STORE_URL", httpServer.URL)
	t.Setenv("PET_NAME", "Dog")
	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/presets",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	t.Run("/schema", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s/schema", testServer.URL))
		assert.NilError(t, err)
		schemaBytes, err := os.ReadFile("testdata/presets/schema.json")
		var expected map[string]any
		assert.NilError(t, json.Unmarshal(schemaBytes, &expected))
		assertHTTPResponse(t, res, http.StatusOK, expected)
	})

	t.Run("/pet/findByStatus", func(t *testing.T) {
		reqBody := []byte(`{
		"collection": "findPetsByStatus",
		"arguments": {
			"headers": {
				"type": "literal",
				"value": {
					"X-Pet-Status": "active"
				}
			}
		},
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value",
					"fields": {
						"type": "array",
						"fields": {
							"type": "object",
							"fields": {
								"id": { "type": "column", "column": "id", "fields": null },
								"name": { "type": "column", "column": "name", "fields": null }
							}
						}
					}
				}
			}
		},
		"arguments": {},
		"collection_relationships": {}
	}`)

		res, err := http.Post(
			fmt.Sprintf("%s/query", testServer.URL),
			"application/json",
			bytes.NewBuffer(reqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{
					{"__value": []any{
						map[string]any{
							"id":   float64(1),
							"name": "test",
						},
					}},
				},
			},
		})
	})

	t.Run("POST /pet", func(t *testing.T) {
		reqBody := []byte(`{
			"operations": [
				{
					"type": "procedure",
					"name": "addPet",
					"arguments": {
						"body": {
							"categories": [{
								"name": "mammal",
								"addresses": [
									{
										"id": 1
									},
									{
										"id": 2
									}
								]
							}]
						}
					}
				}
			],
			"collection_relationships": {}
		}`)

		res, err := http.Post(
			fmt.Sprintf("%s/mutation", testServer.URL),
			"application/json",
			bytes.NewBuffer(reqBody),
		)
		assert.NilError(t, err)

		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult([]any{
					map[string]any{
						"id":   float64(1),
						"name": "Dog",
					},
				}).Encode(),
			},
		})
	})
}
