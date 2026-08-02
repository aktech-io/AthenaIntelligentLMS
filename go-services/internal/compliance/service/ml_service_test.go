package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	commonerrors "github.com/athena-lms/go-services/internal/common/errors"
)

// fakeMLflow is a canned MLflow tracking server covering the four REST
// calls the proxy makes. int64 fields are deliberately emitted as strings
// on some fixtures (MLflow's proto-JSON encoding varies by version).
func fakeMLflow(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/2.0/mlflow/experiments/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("experiments/search: unexpected method %s", r.Method)
		}
		w.Write([]byte(`{"experiments":[
			{"experiment_id":"7","name":"something-else","lifecycle_stage":"active","last_update_time":1720000000000},
			{"experiment_id":"1","name":"liveness-training","lifecycle_stage":"active","last_update_time":"1721000000000"},
			{"experiment_id":"2","name":"liveness-redteam","lifecycle_stage":"active","last_update_time":1722000000000}
		]}`))
	})

	mux.HandleFunc("/api/2.0/mlflow/experiments/get-by-name", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("experiment_name") {
		case "liveness-training":
			w.Write([]byte(`{"experiment":{"experiment_id":"1","name":"liveness-training","lifecycle_stage":"active"}}`))
		case "liveness-redteam":
			// Not created yet — training has never published.
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error_code":"RESOURCE_DOES_NOT_EXIST","message":"no such experiment"}`))
		default:
			t.Errorf("get-by-name: unexpected experiment %q", r.URL.Query().Get("experiment_name"))
			w.WriteHeader(http.StatusNotFound)
		}
	})

	mux.HandleFunc("/api/2.0/mlflow/runs/search", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ExperimentIDs []string `json:"experiment_ids"`
			MaxResults    int      `json:"max_results"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, []string{"1"}, body.ExperimentIDs)
		assert.Equal(t, 5, body.MaxResults)
		w.Write([]byte(`{"runs":[{
			"info":{"run_id":"abc123def456","run_name":"pad-v3","status":"FINISHED",
				"start_time":"1721000000000","end_time":1721003600000},
			"data":{
				"params":[{"key":"lr","value":"0.001"},{"key":"epochs","value":"40"}],
				"metrics":[
					{"key":"val_apcer_max","value":0.031,"timestamp":1721003500000,"step":39},
					{"key":"val_bpcer","value":"0.012","timestamp":"1721003500000","step":"39"},
					{"key":"exploded","value":"NaN","timestamp":1721003500000,"step":39}
				],
				"tags":[
					{"key":"mlflow.user","value":"trainer"},
					{"key":"mlflow.runName","value":"pad-v3"},
					{"key":"l1_gate","value":"pass"}
				]
			}
		}]}`))
	})

	mux.HandleFunc("/api/2.0/mlflow/metrics/get-history", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "abc123def456", r.URL.Query().Get("run_id"))
		assert.Equal(t, "val_apcer_max", r.URL.Query().Get("metric_key"))
		w.Write([]byte(`{"metrics":[
			{"key":"val_apcer_max","value":0.4,"timestamp":1721000100000,"step":0},
			{"key":"val_apcer_max","value":"0.1","timestamp":"1721001000000","step":"10"},
			{"key":"val_apcer_max","value":"Infinity","timestamp":1721001100000,"step":11},
			{"key":"val_apcer_max","value":0.031,"timestamp":1721003500000,"step":39}
		]}`))
	})

	return httptest.NewServer(mux)
}

func newTestML(mlflowURL, engineURL string) *MLService {
	return NewML(mlflowURL, engineURL, zap.NewNop())
}

func TestListMLExperiments_AllowlistAndOrder(t *testing.T) {
	srv := fakeMLflow(t)
	defer srv.Close()

	exps, err := newTestML(srv.URL, "").ListMLExperiments(context.Background())
	require.NoError(t, err)
	require.Len(t, exps, 2) // "something-else" filtered out
	assert.Equal(t, "liveness-training", exps[0].Name)
	assert.Equal(t, int64(1721000000000), exps[0].LastUpdateTime) // int64-as-string tolerated
	assert.Equal(t, "liveness-redteam", exps[1].Name)
	assert.Equal(t, "2", exps[1].ExperimentID)
}

func TestSearchMLRuns_Normalizes(t *testing.T) {
	srv := fakeMLflow(t)
	defer srv.Close()

	runs, err := newTestML(srv.URL, "").SearchMLRuns(context.Background(), MLExperimentTraining, 5)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	run := runs[0]
	assert.Equal(t, "abc123def456", run.RunID)
	assert.Equal(t, "pad-v3", run.Name)
	assert.Equal(t, "FINISHED", run.Status)
	assert.Equal(t, int64(1721000000000), run.StartTime)
	assert.Equal(t, int64(1721003600000), run.EndTime)
	assert.Equal(t, map[string]string{"lr": "0.001", "epochs": "40"}, run.Params)
	// mlflow.* tags dropped, user tags kept
	assert.Equal(t, map[string]string{"l1_gate": "pass"}, run.Tags)
	// NaN metric dropped (unmarshalable), string-encoded numbers tolerated
	assert.Equal(t, map[string]float64{"val_apcer_max": 0.031, "val_bpcer": 0.012}, run.LatestMetrics)
}

func TestSearchMLRuns_UnknownExperimentRejected(t *testing.T) {
	srv := fakeMLflow(t)
	defer srv.Close()

	_, err := newTestML(srv.URL, "").SearchMLRuns(context.Background(), "secret-experiment", 5)
	var be *commonerrors.BusinessError
	require.True(t, errors.As(err, &be))
	assert.Equal(t, http.StatusBadRequest, be.StatusCode)
}

func TestSearchMLRuns_ExperimentNotCreatedYet_EmptyNotError(t *testing.T) {
	srv := fakeMLflow(t)
	defer srv.Close()

	runs, err := newTestML(srv.URL, "").SearchMLRuns(context.Background(), MLExperimentRedTeam, 5)
	require.NoError(t, err)
	assert.Empty(t, runs)
}

func TestMLMetricHistory_SkipsNonFinite(t *testing.T) {
	srv := fakeMLflow(t)
	defer srv.Close()

	hist, err := newTestML(srv.URL, "").MLMetricHistory(context.Background(), "abc123def456", "val_apcer_max")
	require.NoError(t, err)
	assert.Equal(t, "abc123def456", hist.RunID)
	assert.Equal(t, "val_apcer_max", hist.Metric)
	require.Len(t, hist.Points, 3) // Infinity point dropped
	assert.Equal(t, int64(10), hist.Points[1].Step)
	assert.InDelta(t, 0.1, hist.Points[1].Value, 1e-9)
}

func TestML_MLflowUnreachable_Returns503(t *testing.T) {
	// Port from a server that is already closed — connection refused.
	srv := httptest.NewServer(http.NotFoundHandler())
	deadURL := srv.URL
	srv.Close()

	svc := newTestML(deadURL, "")
	_, err := svc.ListMLExperiments(context.Background())
	var be *commonerrors.BusinessError
	require.True(t, errors.As(err, &be), "want BusinessError, got %v", err)
	assert.Equal(t, http.StatusServiceUnavailable, be.StatusCode)

	_, err = svc.SearchMLRuns(context.Background(), MLExperimentTraining, 5)
	require.True(t, errors.As(err, &be))
	assert.Equal(t, http.StatusServiceUnavailable, be.StatusCode)
}

func TestML_MLflowErrorStatus_Returns502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newTestML(srv.URL, "").ListMLExperiments(context.Background())
	var be *commonerrors.BusinessError
	require.True(t, errors.As(err, &be))
	assert.Equal(t, http.StatusBadGateway, be.StatusCode)
}

func TestDeployedModel_OK(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.Write([]byte(`{"status":"ok","service":"ekyc-ml-service",
			"faceEngine":"sface","livenessEngine":"minifasnet_v2",
			"livenessModelChecksum":"sha256:deadbeef"}`))
	}))
	defer engine.Close()

	dm := newTestML("http://unused", engine.URL).DeployedModel(context.Background())
	assert.True(t, dm.Reachable)
	assert.Equal(t, "ok", dm.Status)
	assert.Equal(t, "minifasnet_v2", dm.LivenessEngine)
	assert.Equal(t, "minifasnet_v2", dm.ModelName) // falls back to engine name
	assert.Equal(t, "sha256:deadbeef", dm.ModelChecksum)
	assert.Equal(t, "sface", dm.FaceEngine)
	assert.NotNil(t, dm.Engine)
}

func TestDeployedModel_DegradedStillReachable(t *testing.T) {
	// Readiness gating answers 503 WITH a JSON payload — that is reachable.
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"degraded","livenessEngine":"fallback","degraded":["liveness: fallback"]}`))
	}))
	defer engine.Close()

	dm := newTestML("http://unused", engine.URL).DeployedModel(context.Background())
	assert.True(t, dm.Reachable)
	assert.Equal(t, "degraded", dm.Status)
	assert.Equal(t, "fallback", dm.LivenessEngine)
}

func TestDeployedModel_Unreachable(t *testing.T) {
	engine := httptest.NewServer(http.NotFoundHandler())
	deadURL := engine.URL
	engine.Close()

	dm := newTestML("http://unused", deadURL).DeployedModel(context.Background())
	assert.False(t, dm.Reachable)
	assert.NotEmpty(t, dm.Message)
}

func TestDeployedModel_NotConfigured(t *testing.T) {
	dm := newTestML("http://unused", "").DeployedModel(context.Background())
	assert.False(t, dm.Reachable)
	assert.Contains(t, dm.Message, "EKYC_ML_SERVICE_URL")
}
