// MLService is a thin, read-only proxy in front of the MLflow tracking
// server for the portal's Model Training page. MLflow has NO auth of its own
// and is never exposed through ingress — the ONLY way portal users reach it
// is through these JWT + compliance.decide-gated endpoints, which also
// normalize MLflow's verbose wire format into lean DTOs and allowlist the
// two liveness experiments (docs/nemo: liveness-training / liveness-redteam).
//
// Fail-graceful contract: a dead or slow MLflow returns a clear 503 quickly
// (bounded client timeout) — it must never hang the portal. The eKYC engine
// health probe (deployed-model card) reports unreachability as data
// (Reachable=false), not as an error, so the portal renders an amber state.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	commonerrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/compliance/model"
)

// Experiment names owned by the liveness training/red-team pipelines. The
// proxy is deliberately an allowlist: whatever else lands on the tracking
// server, the portal only ever sees these two.
const (
	MLExperimentTraining = "liveness-training"
	MLExperimentRedTeam  = "liveness-redteam"
)

const (
	mlDefaultRunLimit = 25
	mlMaxRunLimit     = 100
	// mlMaxHistoryPoints bounds a metric-history response (per-epoch curves
	// are small; this only guards against a pathological logger).
	mlMaxHistoryPoints = 5000
)

// errMLflowNotFound marks an MLflow 404 (RESOURCE_DOES_NOT_EXIST) so callers
// can choose between "empty result" (experiment not created yet) and a real
// 404 to the client (unknown run/metric).
var errMLflowNotFound = errors.New("mlflow: resource does not exist")

// MLService proxies MLflow and the eKYC engine health endpoint.
type MLService struct {
	mlflowURL string
	engineURL string
	http      *http.Client
	logger    *zap.Logger
}

// NewML creates the ML observability service. mlflowURL comes from
// MLFLOW_BASE_URL (default http://mlflow:5000 in compose,
// http://nemo-mlflow:5000 in k8s); engineURL is the eKYC engine base URL
// (EKYC_ML_SERVICE_URL), reused for the deployed-model health probe.
func NewML(mlflowURL, engineURL string, logger *zap.Logger) *MLService {
	return &MLService{
		mlflowURL: strings.TrimRight(mlflowURL, "/"),
		engineURL: strings.TrimRight(engineURL, "/"),
		// Bounded timeout so an unreachable/slow upstream degrades to a
		// quick 503 instead of hanging portal requests.
		http:   &http.Client{Timeout: 8 * time.Second},
		logger: logger,
	}
}

// ─── MLflow wire format ──────────────────────────────────────────────────────
//
// MLflow's REST layer is proto-JSON: int64 fields MAY arrive as strings
// depending on server version, and doubles may arrive as "NaN"/"Infinity"
// strings. flexInt64/flexFloat64 tolerate both encodings.

type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("mlflow: bad int64 %q: %w", s, err)
	}
	*f = flexInt64(n)
	return nil
}

type flexFloat64 float64

func (f *flexFloat64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = flexFloat64(math.NaN())
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("mlflow: bad float %q: %w", s, err)
	}
	*f = flexFloat64(v)
	return nil
}

type mlflowExperiment struct {
	ExperimentID   string    `json:"experiment_id"`
	Name           string    `json:"name"`
	LifecycleStage string    `json:"lifecycle_stage"`
	LastUpdateTime flexInt64 `json:"last_update_time"`
}

type mlflowKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type mlflowMetric struct {
	Key       string      `json:"key"`
	Value     flexFloat64 `json:"value"`
	Timestamp flexInt64   `json:"timestamp"`
	Step      flexInt64   `json:"step"`
}

type mlflowRun struct {
	Info struct {
		RunID     string    `json:"run_id"`
		RunName   string    `json:"run_name"`
		Status    string    `json:"status"`
		StartTime flexInt64 `json:"start_time"`
		EndTime   flexInt64 `json:"end_time"`
	} `json:"info"`
	Data struct {
		Metrics []mlflowMetric `json:"metrics"`
		Params  []mlflowKV     `json:"params"`
		Tags    []mlflowKV     `json:"tags"`
	} `json:"data"`
}

// mlflowDo performs one MLflow REST call. Transport failures (connection
// refused, timeout) map to a 503 BusinessError with an operator-actionable
// message; unexpected statuses map to 502; 404 returns errMLflowNotFound.
func (s *MLService) mlflowDo(ctx context.Context, method, path string, body, out any) error {
	if s.mlflowURL == "" {
		return &commonerrors.BusinessError{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "MLflow is not configured (MLFLOW_BASE_URL is empty)",
		}
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("mlflow: marshal request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.mlflowURL+path, rdr)
	if err != nil {
		return fmt.Errorf("mlflow: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.http.Do(req)
	if err != nil {
		s.logger.Warn("MLflow unreachable", zap.String("path", path), zap.Error(err))
		return &commonerrors.BusinessError{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "MLflow tracking server is unreachable — training history is temporarily unavailable",
		}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	switch {
	case resp.StatusCode == http.StatusOK:
		// fall through to decode
	case resp.StatusCode == http.StatusNotFound:
		return errMLflowNotFound
	default:
		s.logger.Warn("MLflow error response", zap.String("path", path),
			zap.Int("status", resp.StatusCode))
		return &commonerrors.BusinessError{
			StatusCode: http.StatusBadGateway,
			Message:    fmt.Sprintf("MLflow returned status %d for %s", resp.StatusCode, path),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &commonerrors.BusinessError{
			StatusCode: http.StatusBadGateway,
			Message:    "MLflow returned an unparseable response",
		}
	}
	return nil
}

// ListMLExperiments returns the liveness experiments that exist on the
// tracking server (subset of the allowlist, in canonical order). A server
// with neither experiment yet returns an empty list, not an error.
func (s *MLService) ListMLExperiments(ctx context.Context) ([]model.MLExperiment, error) {
	var resp struct {
		Experiments []mlflowExperiment `json:"experiments"`
	}
	err := s.mlflowDo(ctx, http.MethodPost, "/api/2.0/mlflow/experiments/search",
		map[string]any{"max_results": 1000}, &resp)
	if err != nil && !errors.Is(err, errMLflowNotFound) {
		return nil, err
	}
	out := make([]model.MLExperiment, 0, 2)
	for _, name := range []string{MLExperimentTraining, MLExperimentRedTeam} {
		for _, e := range resp.Experiments {
			if e.Name == name {
				out = append(out, model.MLExperiment{
					ExperimentID:   e.ExperimentID,
					Name:           e.Name,
					LifecycleStage: e.LifecycleStage,
					LastUpdateTime: int64(e.LastUpdateTime),
				})
				break
			}
		}
	}
	return out, nil
}

// SearchMLRuns lists runs of one allowlisted experiment, newest first,
// normalized to lean DTOs. An experiment that does not exist yet (training
// has never published) yields an empty list — the portal renders its
// "no runs yet" empty state, not an error.
func (s *MLService) SearchMLRuns(ctx context.Context, experiment string, limit int) ([]model.MLRun, error) {
	if experiment != MLExperimentTraining && experiment != MLExperimentRedTeam {
		return nil, commonerrors.BadRequest(fmt.Sprintf(
			"experiment must be %q or %q", MLExperimentTraining, MLExperimentRedTeam))
	}
	if limit <= 0 || limit > mlMaxRunLimit {
		limit = mlDefaultRunLimit
	}

	var expResp struct {
		Experiment mlflowExperiment `json:"experiment"`
	}
	err := s.mlflowDo(ctx, http.MethodGet,
		"/api/2.0/mlflow/experiments/get-by-name?experiment_name="+url.QueryEscape(experiment),
		nil, &expResp)
	if errors.Is(err, errMLflowNotFound) {
		return []model.MLRun{}, nil
	}
	if err != nil {
		return nil, err
	}

	var runsResp struct {
		Runs []mlflowRun `json:"runs"`
	}
	err = s.mlflowDo(ctx, http.MethodPost, "/api/2.0/mlflow/runs/search", map[string]any{
		"experiment_ids": []string{expResp.Experiment.ExperimentID},
		"max_results":    limit,
		"order_by":       []string{"attributes.start_time DESC"},
	}, &runsResp)
	if errors.Is(err, errMLflowNotFound) {
		return []model.MLRun{}, nil
	}
	if err != nil {
		return nil, err
	}

	out := make([]model.MLRun, 0, len(runsResp.Runs))
	for _, r := range runsResp.Runs {
		out = append(out, normalizeMLRun(r))
	}
	return out, nil
}

// normalizeMLRun flattens an MLflow run into the lean portal DTO.
func normalizeMLRun(r mlflowRun) model.MLRun {
	run := model.MLRun{
		RunID:         r.Info.RunID,
		Name:          r.Info.RunName,
		Status:        r.Info.Status,
		StartTime:     int64(r.Info.StartTime),
		EndTime:       int64(r.Info.EndTime),
		Params:        map[string]string{},
		LatestMetrics: map[string]float64{},
		Tags:          map[string]string{},
	}
	for _, p := range r.Data.Params {
		run.Params[p.Key] = p.Value
	}
	for _, t := range r.Data.Tags {
		if t.Key == "mlflow.runName" && run.Name == "" {
			run.Name = t.Value
		}
		// MLflow-internal tags are noise for the portal; user tags
		// (l1_gate/l2_gate, dataset ids, ...) pass through.
		if strings.HasPrefix(t.Key, "mlflow.") {
			continue
		}
		run.Tags[t.Key] = t.Value
	}
	// runs/search returns the latest value per metric key. Non-finite
	// values are dropped: encoding/json cannot marshal NaN/Inf.
	for _, m := range r.Data.Metrics {
		if v := float64(m.Value); !math.IsNaN(v) && !math.IsInf(v, 0) {
			run.LatestMetrics[m.Key] = v
		}
	}
	if run.Name == "" && len(run.RunID) >= 8 {
		run.Name = run.RunID[:8]
	}
	return run
}

// MLMetricHistory returns the full curve of one metric for one run (the
// expandable chart in the portal). Unknown run/metric maps to 404.
func (s *MLService) MLMetricHistory(ctx context.Context, runID, metric string) (model.MLMetricHistory, error) {
	out := model.MLMetricHistory{RunID: runID, Metric: metric, Points: []model.MLMetricPoint{}}
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(metric) == "" {
		return out, commonerrors.BadRequest("runId and metric are required")
	}
	q := url.Values{
		"run_id":      {runID},
		"metric_key":  {metric},
		"max_results": {strconv.Itoa(mlMaxHistoryPoints)},
	}
	var resp struct {
		Metrics []mlflowMetric `json:"metrics"`
	}
	err := s.mlflowDo(ctx, http.MethodGet,
		"/api/2.0/mlflow/metrics/get-history?"+q.Encode(), nil, &resp)
	if errors.Is(err, errMLflowNotFound) {
		return out, commonerrors.NotFound(fmt.Sprintf("run %s not found in MLflow", runID))
	}
	if err != nil {
		return out, err
	}
	for _, m := range resp.Metrics {
		if v := float64(m.Value); !math.IsNaN(v) && !math.IsInf(v, 0) {
			out.Points = append(out.Points, model.MLMetricPoint{
				Step:      int64(m.Step),
				Timestamp: int64(m.Timestamp),
				Value:     v,
			})
		}
	}
	return out, nil
}

// DeployedModel probes the eKYC engine's /health and lifts the liveness
// model identity. The engine answers 503 with a JSON payload when degraded
// (readiness gating) — that is still "reachable"; only transport failures
// and non-JSON answers produce Reachable=false. Never returns an error:
// unreachability is data for the portal's amber card.
func (s *MLService) DeployedModel(ctx context.Context) model.MLDeployedModel {
	if s.engineURL == "" {
		return model.MLDeployedModel{
			Reachable: false,
			Message:   "eKYC engine is not configured (EKYC_ML_SERVICE_URL is empty)",
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.engineURL+"/health", nil)
	if err != nil {
		return model.MLDeployedModel{Reachable: false, Message: "bad engine URL: " + err.Error()}
	}
	resp, err := s.http.Do(req)
	if err != nil {
		s.logger.Warn("eKYC engine health probe failed", zap.Error(err))
		return model.MLDeployedModel{
			Reachable: false,
			Message:   "eKYC engine unreachable — deployed model identity unavailable",
		}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil ||
		(resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable) {
		return model.MLDeployedModel{
			Reachable: false,
			Message:   fmt.Sprintf("eKYC engine answered status %d with an unexpected body", resp.StatusCode),
		}
	}

	dm := model.MLDeployedModel{
		Reachable:      true,
		Status:         strAt(payload, "status"),
		LivenessEngine: strAt(payload, "livenessEngine"),
		FaceEngine:     strAt(payload, "faceEngine"),
		Engine:         payload,
	}
	// Identity fields: tolerate the naming the engine ships now and the
	// richer identity the training pipeline adds (docs/nemo).
	dm.ModelName = firstStrAt(payload,
		"livenessModel", "livenessModelName", "modelName", "model")
	if dm.ModelName == "" {
		dm.ModelName = dm.LivenessEngine
	}
	dm.ModelChecksum = firstStrAt(payload,
		"livenessModelChecksum", "livenessModelSha256", "modelChecksum", "modelSha256")
	return dm
}

func strAt(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func firstStrAt(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := strAt(m, k); s != "" {
			return s
		}
	}
	return ""
}
