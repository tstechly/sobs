package integration_test

import (
	"testing"

	"github.com/chdb-io/chdb-go/chdb"
	"github.com/stretchr/testify/require"
)

func TestClickHouseDirectIntegration(t *testing.T) {

	s, err := chdb.NewSession("")
	require.NoError(t, err)
	defer s.Close()

	res, err := s.Query("SELECT version()")
	require.NoError(t, err)
	t.Logf("chDB version: %s", res)
}
