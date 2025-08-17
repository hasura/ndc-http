package connector

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hasura/ndc-http/connector/internal"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/ndctest"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils/compression"
	"gotest.tools/v3/assert"
)

func TestRawHTTPRequest(t *testing.T) {
	ndctest.TestConnector(t, NewHTTPConnector(), ndctest.TestConnectorOptions{
		Configuration: "testdata/jsonplaceholder",
		TestDataDir:   "testdata/raw",
	})
}

func TestHTTPConnectorCompression(t *testing.T) {
	t.Setenv("HTTP_STRINGIFY_JSON", "true")

	postsBody := map[string]any{
		"id":     float64(101),
		"title":  "Hello world",
		"userId": float64(10),
		"body":   "A test post",
	}

	rawPostsBody, err := json.Marshal(postsBody)
	assert.NilError(t, err)
	rawMutationArguments, err := json.Marshal(map[string]any{
		"body": postsBody,
	})
	assert.NilError(t, err)

	mux := http.NewServeMux()

	mux.HandleFunc("/posts/gzip", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			compressor := compression.GzipCompressor{}
			assert.Equal(t, "gzip", r.Header.Get(rest.ContentEncodingHeader))
			reqBody, err := compressor.Decompress(r.Body)
			assert.NilError(t, err)

			var expected map[string]any
			err = json.NewDecoder(reqBody).Decode(&expected)
			assert.NilError(t, err)
			assert.DeepEqual(t, postsBody, expected)

			w.Header().Add(rest.ContentTypeHeader, "application/json")
			w.Header().Add(rest.ContentEncodingHeader, string(compression.EncodingGzip))
			w.WriteHeader(http.StatusOK)

			_, err = compressor.Compress(w, bytes.NewReader(rawPostsBody))
			assert.NilError(t, err)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/posts/deflate-failed", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			compressor := compression.GzipCompressor{}
			w.Header().Add(rest.ContentTypeHeader, "application/json")
			w.Header().Add(rest.ContentEncodingHeader, string(compression.EncodingDeflate))
			w.WriteHeader(http.StatusOK)

			_, err = compressor.Compress(w, bytes.NewReader(rawPostsBody))
			assert.NilError(t, err)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/posts/deflate", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			assert.Equal(t, string(compression.EncodingDeflate), r.Header.Get(rest.ContentEncodingHeader))
			compressor := compression.DeflateCompressor{}
			reqBody, err := compressor.Decompress(r.Body)
			assert.NilError(t, err)
			var decodedResp any
			err = json.NewDecoder(reqBody).Decode(&decodedResp)
			assert.NilError(t, err)
			assert.DeepEqual(t, postsBody, decodedResp)

			w.Header().Add(rest.ContentTypeHeader, "application/json")
			w.Header().Add(rest.ContentEncodingHeader, string(compression.EncodingDeflate))
			w.WriteHeader(http.StatusOK)

			_, err = compressor.Compress(w, bytes.NewReader(rawPostsBody))
			assert.NilError(t, err)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	t.Setenv("SERVER_URL", server.URL)
	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/compression",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	t.Run("gzip", func(t *testing.T) {
		rawReqBody, err := json.Marshal(schema.MutationRequest{
			CollectionRelationships: make(schema.MutationRequestCollectionRelationships),
			Operations: []schema.MutationOperation{
				{
					Type:      schema.MutationOperationProcedure,
					Name:      "createPostGzip",
					Arguments: rawMutationArguments,
					Fields: schema.NewNestedObject(map[string]schema.FieldEncoder{
						"id":     schema.NewColumnField("id"),
						"title":  schema.NewColumnField("title"),
						"userId": schema.NewColumnField("userId"),
						"body":   schema.NewColumnField("body"),
					}).Encode(),
				},
			},
		})

		res, err := http.Post(
			testServer.URL+"/mutation",
			"application/json",
			bytes.NewBuffer(rawReqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(postsBody).Encode(),
			},
		})
	})

	t.Run("deflate", func(t *testing.T) {
		rawReqBody, err := json.Marshal(schema.MutationRequest{
			CollectionRelationships: make(schema.MutationRequestCollectionRelationships),
			Operations: []schema.MutationOperation{
				{
					Type:      schema.MutationOperationProcedure,
					Name:      "createPostDeflate",
					Arguments: rawMutationArguments,
					Fields: schema.NewNestedObject(map[string]schema.FieldEncoder{
						"id":     schema.NewColumnField("id"),
						"title":  schema.NewColumnField("title"),
						"userId": schema.NewColumnField("userId"),
						"body":   schema.NewColumnField("body"),
					}).Encode(),
				},
			},
		})

		res, err := http.Post(
			testServer.URL+"/mutation",
			"application/json",
			bytes.NewBuffer(rawReqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(postsBody).Encode(),
			},
		})
	})

	t.Run("deflate_failure", func(t *testing.T) {
		rawReqBody, err := json.Marshal(schema.MutationRequest{
			CollectionRelationships: make(schema.MutationRequestCollectionRelationships),
			Operations: []schema.MutationOperation{
				{
					Type:      schema.MutationOperationProcedure,
					Name:      "createPostDeflateFailed",
					Arguments: rawMutationArguments,
					Fields: schema.NewNestedObject(map[string]schema.FieldEncoder{
						"id":     schema.NewColumnField("id"),
						"title":  schema.NewColumnField("title"),
						"userId": schema.NewColumnField("userId"),
						"body":   schema.NewColumnField("body"),
					}).Encode(),
				},
			},
		})

		res, err := http.Post(
			testServer.URL+"/mutation",
			"application/json",
			bytes.NewBuffer(rawReqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusInternalServerError, schema.ErrorResponse{
			Message: "flate: corrupt input before offset 1",
			Details: make(map[string]any),
		})
	})
}

func TestDisableRawHTTPRequest(t *testing.T) {
	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/compression",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	t.Run("disable_raw_http", func(t *testing.T) {
		rawReqBody, err := json.Marshal(schema.MutationRequest{
			CollectionRelationships: make(schema.MutationRequestCollectionRelationships),
			Operations: []schema.MutationOperation{
				{
					Type: schema.MutationOperationProcedure,
					Name: "sendHttpRequest",
					Arguments: []byte(`{
						"body": {},
						"method": "post",
						"url": "https://jsonplaceholder.typicode.com/posts"
					}`),
					Fields: schema.NewNestedObject(map[string]schema.FieldEncoder{
						"id": schema.NewColumnField("id"),
					}).Encode(),
				},
			},
		})

		res, err := http.Post(
			testServer.URL+"/mutation",
			"application/json",
			bytes.NewBuffer(rawReqBody),
		)
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusInternalServerError, schema.ErrorResponse{
			Message: internal.ProcedureSendHTTPRequest + " mutation is disabled. Set runtime.enableRawRequest=true to enable this operation",
			Details: map[string]any{},
		})
	})
}
