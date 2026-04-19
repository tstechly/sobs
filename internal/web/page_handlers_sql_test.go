package web

import "testing"

func TestBuildLogsWhereClauseRejectsUnsafeSQLFilter(t *testing.T) {
	where, params, errMsg := buildLogsWhereClause(nil, nil, "", "", "", "", "service='api'; DROP TABLE otel_logs")
	if errMsg == "" {
		t.Fatalf("expected validation error, got where=%q params=%v", where, params)
	}
}

func TestBuildLogsWhereClauseNormalizesAliasesInSQLFilter(t *testing.T) {
	where, _, errMsg := buildLogsWhereClause(nil, nil, "", "", "", "", "service='api' AND level='INFO'")
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if where == "" || where == "WHERE service='api' AND level='INFO'" {
		t.Fatalf("expected normalized filter aliases in where clause, got %q", where)
	}
}
