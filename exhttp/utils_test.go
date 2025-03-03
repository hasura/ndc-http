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
	assert.ErrorContains(t, err, `strconv.ParseInt: parsing "abc": invalid syntax`)
}
