package setupwizard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSteps_Defaults(t *testing.T) {
	h := NewHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/setup-wizard/steps", nil)
	rr := httptest.NewRecorder()

	h.Steps(rr, r)

	require.Equal(t, http.StatusOK, rr.Code)
	var got Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.True(t, got.OK)
	require.Equal(t, "1", got.Version)
	require.Equal(t, "dev", got.Env)
	require.Equal(t, "python", got.Language)
	require.Equal(t, "docker", got.Deployment)
	require.Len(t, got.Steps, 5)
	require.Len(t, got.Checklist, 3)
	require.Equal(t, "sdk_install", got.Steps[0].ID)
	require.Equal(t, "collector_run", got.Steps[2].ID)
	require.Equal(t, "sobs_verify", got.Steps[4].ID)
}

func TestSteps_ProdAddsAnomalyStep(t *testing.T) {
	h := NewHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/setup-wizard/steps?env=prod&language=go&deployment=cloud", nil)
	rr := httptest.NewRecorder()

	h.Steps(rr, r)

	require.Equal(t, http.StatusOK, rr.Code)
	var got Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.True(t, got.OK)
	require.Equal(t, "prod", got.Env)
	require.Equal(t, "go", got.Language)
	require.Equal(t, "cloud", got.Deployment)
	require.Len(t, got.Steps, 5)
	require.Equal(t, "sobs_anomaly", got.Steps[len(got.Steps)-1].ID)
	require.Len(t, got.Checklist, 4)
	require.Equal(t, "anomaly", got.Checklist[len(got.Checklist)-1].ID)
}

func TestSteps_InvalidParams(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "invalid env",
			url:  "/api/setup-wizard/steps?env=stage",
			want: "Invalid env 'stage'. Must be one of: ['dev', 'prod']",
		},
		{
			name: "invalid language",
			url:  "/api/setup-wizard/steps?language=rust",
			want: "Invalid language 'rust'. Must be one of: ['dotnet', 'go', 'java', 'node', 'php', 'python', 'ruby']",
		},
		{
			name: "invalid deployment",
			url:  "/api/setup-wizard/steps?deployment=vm",
			want: "Invalid deployment 'vm'. Must be one of: ['baremetal', 'cloud', 'docker', 'kubernetes']",
		},
	}

	h := NewHandler()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()

			h.Steps(rr, r)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			var got Response
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			require.False(t, got.OK)
			require.Equal(t, tc.want, got.Error)
		})
	}
}
