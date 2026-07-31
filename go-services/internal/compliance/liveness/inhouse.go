// Provider "inhouse" — the Nemo-owned PAD scorer: MiniFASNetV2 behind the
// ekyc-ml-service Python sidecar (POST /v1/face/liveness, 1–5 multipart
// "frame" parts). Extracted from the eKYC inhouse provider (audit action 3)
// so bridge SDKs and VeriFayda can register alongside it in the same
// registry.
//
// Fail-closed contract: any engine error returns an error; the caller's
// shadow/enforce logic decides whether that error is tolerated (shadow) or
// fatal (enforce). This provider never fabricates a score.
package liveness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// maxFrames caps frames sent to the PAD engine (its API limit).
const maxFrames = 5

// InhouseConfig wires the in-house PAD scorer.
type InhouseConfig struct {
	EngineURL string // ekyc-ml-service base URL (EKYC_ML_SERVICE_URL)
}

// Inhouse implements Provider against the ekyc-ml-service engine.
type Inhouse struct {
	cfg    InhouseConfig
	client *http.Client // plain client for the internal ML sidecar
}

// NewInhouse builds the scorer. EngineURL may be empty; Score fails closed
// on first use so a misconfigured deployment never fabricates a verdict.
func NewInhouse(cfg InhouseConfig) *Inhouse {
	return &Inhouse{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// NewInhouseFromEnv builds the scorer from EKYC_ML_SERVICE_URL (the same
// sidecar the eKYC inhouse provider talks to).
func NewInhouseFromEnv() *Inhouse {
	return NewInhouse(InhouseConfig{
		EngineURL: strings.TrimRight(os.Getenv("EKYC_ML_SERVICE_URL"), "/"),
	})
}

func (p *Inhouse) Name() string { return "inhouse" }

// livenessResponse mirrors ekyc-ml-service POST /v1/face/liveness.
type livenessResponse struct {
	LiveScore float64 `json:"liveScore"`
	Label     string  `json:"label"`
	Model     string  `json:"model"`
}

// Score sends 1..maxFrames frames to the passive-PAD engine as repeated
// multipart parts all named "frame" and maps the verdict onto Result. Extra
// frames beyond the engine's API limit are dropped, newest last.
func (p *Inhouse) Score(ctx context.Context, frames [][]byte) (Result, error) {
	if p.cfg.EngineURL == "" {
		return Result{}, fmt.Errorf("inhouse liveness: EKYC_ML_SERVICE_URL not configured")
	}
	if len(frames) == 0 {
		return Result{}, fmt.Errorf("inhouse liveness: no frames to score")
	}
	if len(frames) > maxFrames {
		frames = frames[:maxFrames]
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for i, frame := range frames {
		fw, err := mw.CreateFormFile("frame", fmt.Sprintf("frame%d.jpg", i))
		if err != nil {
			return Result{}, fmt.Errorf("inhouse liveness: build form: %w", err)
		}
		if _, err := fw.Write(frame); err != nil {
			return Result{}, fmt.Errorf("inhouse liveness: build form: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return Result{}, fmt.Errorf("inhouse liveness: build form: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.EngineURL+"/v1/face/liveness", &buf)
	if err != nil {
		return Result{}, fmt.Errorf("inhouse liveness: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("inhouse liveness: engine /v1/face/liveness: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("inhouse liveness: engine /v1/face/liveness: status %d: %s",
			resp.StatusCode, truncate(string(body), 200))
	}
	var out livenessResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return Result{}, fmt.Errorf("inhouse liveness: engine /v1/face/liveness: bad response: %w", err)
	}
	return Result{
		LiveScore: out.LiveScore,
		Label:     out.Label,
		Provider:  p.Name(),
		AuditRef:  "inhouse-liveness-" + uuid.NewString(),
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
