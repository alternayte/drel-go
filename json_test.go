package drel_test

import (
	"database/sql/driver"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSON_ValueMarshalsSlice(t *testing.T) {
	tags := []string{"a", "b"}
	v, err := drel.JSON[[]string]{V: &tags}.Value()
	require.NoError(t, err)
	assert.Equal(t, `["a","b"]`, string(v.([]byte)))
}

func TestJSON_ValueMarshalsMap(t *testing.T) {
	m := map[string]int{"x": 1}
	v, err := drel.JSON[map[string]int]{V: &m}.Value()
	require.NoError(t, err)
	assert.Equal(t, `{"x":1}`, string(v.([]byte)))
}

func TestJSON_ValueNilPointerReturnsNil(t *testing.T) {
	var j drel.JSON[[]string]
	v, err := j.Value()
	require.NoError(t, err)
	assert.Nil(t, v)
}

func TestJSON_ScanFromBytes(t *testing.T) {
	var tags []string
	err := drel.JSON[[]string]{V: &tags}.Scan([]byte(`["a","b"]`))
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, tags)
}

func TestJSON_ScanFromString(t *testing.T) {
	var m map[string]int
	err := drel.JSON[map[string]int]{V: &m}.Scan(`{"x":1}`)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"x": 1}, m)
}

func TestJSON_ScanNilLeavesZeroValue(t *testing.T) {
	tags := []string{"keep"}
	err := drel.JSON[[]string]{V: &tags}.Scan(nil)
	require.NoError(t, err)
	assert.Nil(t, tags)
}

func TestJSON_ImplementsScannerAndValuer(t *testing.T) {
	var tags []string
	var _ driver.Valuer = drel.JSON[[]string]{V: &tags}
	// Scan has a value receiver but mutates *V; assert it satisfies the
	// interface used by database/sql and pgx.
	var s interface{ Scan(any) error } = drel.JSON[[]string]{V: &tags}
	require.NoError(t, s.Scan([]byte(`[]`)))
}

func TestJSONEqual(t *testing.T) {
	type s struct {
		A int
		B []string
	}
	assert.True(t, drel.JSONEqual(s{A: 1, B: []string{"x"}}, s{A: 1, B: []string{"x"}}))
	assert.False(t, drel.JSONEqual(s{A: 1}, s{A: 2}))
}
