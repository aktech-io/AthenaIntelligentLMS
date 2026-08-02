package model

// ML observability DTOs for the portal's Model Training page — lean
// projections of MLflow entities (the proxy never exposes raw MLflow
// responses) plus the eKYC engine's deployed-model identity.

// MLExperiment is a lean projection of an MLflow experiment.
type MLExperiment struct {
	ExperimentID   string `json:"experimentId"`
	Name           string `json:"name"`
	LifecycleStage string `json:"lifecycleStage,omitempty"`
	// LastUpdateTime is epoch milliseconds (MLflow native).
	LastUpdateTime int64 `json:"lastUpdateTime,omitempty"`
}

// MLRun is a lean projection of an MLflow run: identity, lifecycle, params
// and the LATEST value of each metric (curves come from MLMetricHistory).
type MLRun struct {
	RunID  string `json:"runId"`
	Name   string `json:"name"`
	Status string `json:"status"` // RUNNING | FINISHED | FAILED | KILLED | SCHEDULED
	// StartTime/EndTime are epoch milliseconds; EndTime is 0 while running.
	StartTime     int64              `json:"startTime,omitempty"`
	EndTime       int64              `json:"endTime,omitempty"`
	Params        map[string]string  `json:"params"`
	LatestMetrics map[string]float64 `json:"latestMetrics"`
	// Tags excludes MLflow-internal "mlflow.*" tags; gate verdicts
	// (l1_gate/l2_gate = pass|fail) ride here.
	Tags map[string]string `json:"tags"`
}

// MLMetricPoint is one observation on a metric curve.
type MLMetricPoint struct {
	Step      int64   `json:"step"`
	Timestamp int64   `json:"timestamp"` // epoch milliseconds
	Value     float64 `json:"value"`
}

// MLMetricHistory is the full curve for one metric of one run.
type MLMetricHistory struct {
	RunID  string          `json:"runId"`
	Metric string          `json:"metric"`
	Points []MLMetricPoint `json:"points"`
}

// MLDeployedModel reports the liveness model identity the eKYC engine is
// currently serving, lifted from its GET /health payload. Reachable=false
// (with Message) is a first-class state so the portal can render an explicit
// "engine unreachable" card instead of a generic error.
type MLDeployedModel struct {
	Reachable      bool   `json:"reachable"`
	Status         string `json:"status,omitempty"` // ok | degraded
	LivenessEngine string `json:"livenessEngine,omitempty"`
	FaceEngine     string `json:"faceEngine,omitempty"`
	ModelName      string `json:"modelName,omitempty"`
	ModelChecksum  string `json:"modelChecksum,omitempty"`
	Message        string `json:"message,omitempty"`
	// Engine is the raw /health payload for forward-compatibility (new
	// identity fields appear here before the proxy learns to lift them).
	Engine map[string]any `json:"engine,omitempty"`
}
