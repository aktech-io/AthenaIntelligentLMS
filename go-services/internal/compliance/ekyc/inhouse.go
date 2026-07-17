// Provider "inhouse" — the Nemo-owned eKYC engine (founder decision: in-house
// first, commercial vendors remain a pluggable per-client option via the same
// registry). It orchestrates the ekyc-ml-service Python sidecar:
//
//	document/selfie media refs → media-service (bytes, X-Service-Key auth)
//	  → POST /v1/document/extract  (OCR + MRZ fields, per-field confidence)
//	  → POST /v1/face/match        (document portrait vs selfie score)
//	  → POST /v1/screen            (sanctions/PEP name screening)
//
// Fail-closed contract: ANY engine or media error returns an error, which the
// onboarding tier policy turns into an officer referral. This provider never
// fabricates a pass — a Result is only returned when every stage answered.
package ekyc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/common/auth"
)

const (
	// defaultMinFieldConfidence gates DocumentVerified: OCR fields below this
	// confidence do not count as "extracted".
	defaultMinFieldConfidence = 0.60
	// defaultNameMatchThreshold gates declared-vs-extracted name agreement.
	defaultNameMatchThreshold = 0.75
)

// InhouseConfig wires the in-house provider. All fields are required except
// the thresholds, which default sensibly.
type InhouseConfig struct {
	EngineURL          string  // ekyc-ml-service base URL (EKYC_ML_SERVICE_URL)
	MediaURL           string  // media-service base URL (MEDIA_SERVICE_URL)
	ServiceKey         string  // X-Service-Key for media-service (LMS_INTERNAL_SERVICE_KEY)
	MinFieldConfidence float64 // 0 → defaultMinFieldConfidence
	NameMatchThreshold float64 // 0 → defaultNameMatchThreshold
}

// Inhouse implements Provider against the ekyc-ml-service engine.
type Inhouse struct {
	cfg    InhouseConfig
	engine *http.Client // plain client for the internal ML sidecar
	media  *http.Client // service-key-stamped client for media-service
}

// NewInhouse builds the provider. URLs may be empty; Verify fails closed on
// first use so a misconfigured deployment refers applicants to a human
// instead of passing them.
func NewInhouse(cfg InhouseConfig) *Inhouse {
	if cfg.MinFieldConfidence <= 0 {
		cfg.MinFieldConfidence = defaultMinFieldConfidence
	}
	if cfg.NameMatchThreshold <= 0 {
		cfg.NameMatchThreshold = defaultNameMatchThreshold
	}
	return &Inhouse{
		cfg:    cfg,
		engine: &http.Client{Timeout: 60 * time.Second},
		media: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  cfg.ServiceKey,
				ServiceName: "compliance-service",
			},
		},
	}
}

// NewInhouseFromEnv builds the provider from EKYC_ML_SERVICE_URL,
// MEDIA_SERVICE_URL and LMS_INTERNAL_SERVICE_KEY.
func NewInhouseFromEnv() *Inhouse {
	return NewInhouse(InhouseConfig{
		EngineURL:  strings.TrimRight(os.Getenv("EKYC_ML_SERVICE_URL"), "/"),
		MediaURL:   strings.TrimRight(os.Getenv("MEDIA_SERVICE_URL"), "/"),
		ServiceKey: os.Getenv("LMS_INTERNAL_SERVICE_KEY"),
	})
}

func (p *Inhouse) Name() string { return "inhouse" }

// extractResponse mirrors ekyc-ml-service POST /v1/document/extract.
type extractResponse struct {
	Engine string                    `json:"engine"`
	Fields map[string]extractedField `json:"fields"`
	MRZ    *struct {
		Valid bool `json:"valid"`
	} `json:"mrz"`
}

type extractedField struct {
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
}

// faceMatchResponse mirrors POST /v1/face/match.
type faceMatchResponse struct {
	Engine          string  `json:"engine"`
	Score           float64 `json:"score"`
	DocumentFace    bool    `json:"documentFaceFound"`
	SelfieFaceFound bool    `json:"selfieFaceFound"`
}

// screenResponse mirrors POST /v1/screen.
type screenResponse struct {
	SanctionsHit bool `json:"sanctionsHit"`
	PEPHit       bool `json:"pepHit"`
}

// Verify runs the three engine stages and maps them onto Result.
//
//	DocumentVerified — fullName+documentNumber extracted at/above the
//	  confidence floor AND, when the request declares fullName/nationalId,
//	  the extracted values agree with the declared ones (fuzzy for names,
//	  normalized-exact for ids).
//	LivenessPassed   — v1 heuristic: a face was detected in the selfie. Real
//	  liveness (challenge-response) is an app-side capture feature, deferred.
//	FaceMatchScore   — engine score in [0,1], threshold applied by tiering.
//	SanctionsHit/PEPHit — screening verdict over the configured lists.
func (p *Inhouse) Verify(ctx context.Context, req Request) (Result, error) {
	if p.cfg.EngineURL == "" {
		return Result{}, fmt.Errorf("inhouse ekyc: EKYC_ML_SERVICE_URL not configured")
	}
	if p.cfg.MediaURL == "" {
		return Result{}, fmt.Errorf("inhouse ekyc: MEDIA_SERVICE_URL not configured")
	}

	res := Result{ProviderRef: "inhouse-" + uuid.NewString()}

	// Screening runs on the declared name regardless of media evidence.
	scr, err := p.screen(ctx, req.FullName)
	if err != nil {
		return Result{}, err
	}
	res.SanctionsHit = scr.SanctionsHit
	res.PEPHit = scr.PEPHit

	// Missing media refs are a fact ("no evidence"), not an engine error:
	// tiering refers those applications without a PROVIDER_ERROR reason.
	var docBytes, selfieBytes []byte
	if req.DocumentRef != "" {
		if docBytes, err = p.fetchMedia(ctx, req.DocumentRef); err != nil {
			return Result{}, err
		}
	}
	if req.SelfieRef != "" {
		if selfieBytes, err = p.fetchMedia(ctx, req.SelfieRef); err != nil {
			return Result{}, err
		}
	}

	if docBytes != nil {
		ext, err := p.extract(ctx, docBytes)
		if err != nil {
			return Result{}, err
		}
		res.DocumentVerified = p.documentVerified(req, ext)
	}

	if docBytes != nil && selfieBytes != nil {
		fm, err := p.faceMatch(ctx, docBytes, selfieBytes)
		if err != nil {
			return Result{}, err
		}
		res.FaceMatchScore = fm.Score
		res.LivenessPassed = fm.SelfieFaceFound
	}

	return res, nil
}

// documentVerified applies the field-confidence and declared-value checks.
func (p *Inhouse) documentVerified(req Request, ext *extractResponse) bool {
	name, okName := ext.Fields["fullName"]
	num, okNum := ext.Fields["documentNumber"]
	if !okName || !okNum ||
		name.Confidence < p.cfg.MinFieldConfidence ||
		num.Confidence < p.cfg.MinFieldConfidence {
		return false
	}
	if req.FullName != "" && nameSimilarity(req.FullName, name.Value) < p.cfg.NameMatchThreshold {
		return false
	}
	if req.NationalID != "" && !docNumbersMatch(req.NationalID, num.Value) {
		return false
	}
	if req.DateOfBirth != "" {
		if dob, ok := ext.Fields["dateOfBirth"]; ok &&
			dob.Confidence >= p.cfg.MinFieldConfidence && dob.Value != req.DateOfBirth {
			return false
		}
	}
	return true
}

// ─── engine calls ────────────────────────────────────────────────────────────

func (p *Inhouse) fetchMedia(ctx context.Context, ref string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/v1/media/download/%s", p.cfg.MediaURL, ref)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("inhouse ekyc: build media request: %w", err)
	}
	resp, err := p.media.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("inhouse ekyc: fetch media %s: %w", ref, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inhouse ekyc: fetch media %s: status %d", ref, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, fmt.Errorf("inhouse ekyc: read media %s: %w", ref, err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("inhouse ekyc: media %s is empty", ref)
	}
	return b, nil
}

func (p *Inhouse) extract(ctx context.Context, doc []byte) (*extractResponse, error) {
	var out extractResponse
	err := p.postMultipart(ctx, "/v1/document/extract",
		map[string][]byte{"file": doc}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *Inhouse) faceMatch(ctx context.Context, doc, selfie []byte) (*faceMatchResponse, error) {
	var out faceMatchResponse
	err := p.postMultipart(ctx, "/v1/face/match",
		map[string][]byte{"document": doc, "selfie": selfie}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *Inhouse) screen(ctx context.Context, fullName string) (*screenResponse, error) {
	body, _ := json.Marshal(map[string]string{"fullName": fullName})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.EngineURL+"/v1/screen", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("inhouse ekyc: build screen request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	var out screenResponse
	if err := p.doEngine(httpReq, "/v1/screen", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// postMultipart posts image parts to the engine and decodes JSON into out.
func (p *Inhouse) postMultipart(ctx context.Context, path string, parts map[string][]byte, out any) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	// deterministic part order for testability
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fw, err := mw.CreateFormFile(name, name+".jpg")
		if err != nil {
			return fmt.Errorf("inhouse ekyc: build %s form: %w", path, err)
		}
		if _, err := fw.Write(parts[name]); err != nil {
			return fmt.Errorf("inhouse ekyc: build %s form: %w", path, err)
		}
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("inhouse ekyc: build %s form: %w", path, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.EngineURL+path, &buf)
	if err != nil {
		return fmt.Errorf("inhouse ekyc: build %s request: %w", path, err)
	}
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())
	return p.doEngine(httpReq, path, out)
}

func (p *Inhouse) doEngine(req *http.Request, path string, out any) error {
	resp, err := p.engine.Do(req)
	if err != nil {
		return fmt.Errorf("inhouse ekyc: engine %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("inhouse ekyc: engine %s: status %d: %s",
			path, resp.StatusCode, truncate(string(body), 200))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("inhouse ekyc: engine %s: bad response: %w", path, err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ─── declared-vs-extracted comparison (mirrors engine/names.py) ─────────────

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// normalizeName lowercases and strips punctuation/extra whitespace.
func normalizeName(s string) string {
	s = nonAlnum.ReplaceAllString(strings.ToLower(s), " ")
	return strings.Join(strings.Fields(s), " ")
}

// nameSimilarity is an order-insensitive token similarity in [0,1]: the max
// of token-sort sequence similarity and a containment/coverage blend, so
// "MWANGI JANE" matches "Jane Mwangi" and a missing middle name still scores
// high while a single shared common token does not.
func nameSimilarity(a, b string) float64 {
	na, nb := normalizeName(a), normalizeName(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}
	ta, tb := strings.Fields(na), strings.Fields(nb)
	sort.Strings(ta)
	sort.Strings(tb)
	best := seqRatio(strings.Join(ta, " "), strings.Join(tb, " "))

	setA, setB := toSet(ta), toSet(tb)
	inter := 0
	for t := range setA {
		if setB[t] {
			inter++
		}
	}
	if inter > 0 {
		minLen, maxLen := len(setA), len(setB)
		if minLen > maxLen {
			minLen, maxLen = maxLen, minLen
		}
		containment := float64(inter) / float64(minLen)
		coverage := float64(inter) / float64(maxLen)
		if s := 0.55*containment + 0.45*coverage; s > best {
			best = s
		}
	}
	return best
}

// docNumbersMatch compares document numbers on alphanumerics only.
func docNumbersMatch(declared, extracted string) bool {
	d := strings.ToUpper(nonAlnum.ReplaceAllString(strings.ToLower(declared), ""))
	e := strings.ToUpper(nonAlnum.ReplaceAllString(strings.ToLower(extracted), ""))
	if d == "" || e == "" {
		return false
	}
	return d == e
}

func toSet(ts []string) map[string]bool {
	m := make(map[string]bool, len(ts))
	for _, t := range ts {
		m[t] = true
	}
	return m
}

// seqRatio is difflib.SequenceMatcher.ratio(): 2*matches/(len(a)+len(b)),
// with matches counted over longest common substrings recursively.
func seqRatio(a, b string) float64 {
	if len(a)+len(b) == 0 {
		return 0
	}
	return 2 * float64(lcsMatches(a, b)) / float64(len(a)+len(b))
}

func lcsMatches(a, b string) int {
	if a == "" || b == "" {
		return 0
	}
	ai, bi, size := longestCommonSubstring(a, b)
	if size == 0 {
		return 0
	}
	return size +
		lcsMatches(a[:ai], b[:bi]) +
		lcsMatches(a[ai+size:], b[bi+size:])
}

func longestCommonSubstring(a, b string) (ai, bi, size int) {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
				if cur[j] > size {
					size = cur[j]
					ai, bi = i-size, j-size
				}
			} else {
				cur[j] = 0
			}
		}
		prev, cur = cur, prev
	}
	return ai, bi, size
}
