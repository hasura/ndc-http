package exhttp

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestPort(t *testing.T) {
	port, err := ParsePort("", "https")
	assert.NilError(t, err)
	assert.Equal(t, 443, port)

	port, err = ParsePort("", "http")
	assert.NilError(t, err)
	assert.Equal(t, 80, port)

	port, err = ParsePort("10000", "http")
	assert.NilError(t, err)
	assert.Equal(t, 10000, port)

	_, err = ParsePort("abc", "http")
	assert.ErrorContains(t, err, `strconv.Atoi: parsing "abc": invalid syntax`)
}

func TestParseHTTPUrl(t *testing.T) {
	expected := "http://localhost:8080/v1/api"
	result, err := ParseHttpURL(expected)
	assert.NilError(t, err)
	assert.Equal(t, expected, result.String())

	_, err = ParseHttpURL("!@#$%")
	assert.ErrorContains(t, err, "invalid URL escape")

	_, err = ParseHttpURL("gs://path/to/file")
	assert.ErrorContains(t, err, "invalid http(s) scheme")
}
