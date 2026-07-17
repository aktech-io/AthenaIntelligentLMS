package ekyc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeEngine is a stub ekyc-ml-service: per-endpoint canned JSON or status.
type fakeEngine struct {
	extractBody string
	extractCode int
	faceBody    string
	faceCode    int
	screenBody  string
	screenCode  int

	screenRequests []map[string]any
}

func (f *fakeEngine) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeCanned := func(code int, body string) {
			if code == 0 {
				code = http.StatusOK
			}
			w.WriteHeader(code)
			fmt.Fprint(w, body)
		}
		switch r.URL.Path {
		case "/v1/document/extract":
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				t.Errorf("extract: bad multipart: %v", err)
			}
			if _, _, err := r.FormFile("file"); err != nil {
				t.Errorf("extract: missing file part: %v", err)
			}
			writeCanned(f.extractCode, f.extractBody)
		case "/v1/face/match":
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				t.Errorf("face: bad multipart: %v", err)
			}
			for _, part := range []string{"document", "selfie"} {
				if _, _, err := r.FormFile(part); err != nil {
					t.Errorf("face: missing %s part: %v", part, err)
				}
			}
			writeCanned(f.faceCode, f.faceBody)
		case "/v1/screen":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.screenRequests = append(f.screenRequests, req)
			writeCanned(f.screenCode, f.screenBody)
		default:
			t.Errorf("engine: unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// fakeMedia is a stub media-service serving refs from a map; it also asserts
// service-key auth headers arrive.
func fakeMedia(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Service-Key"); got != "test-key" {
			t.Errorf("media: X-Service-Key = %q, want test-key", got)
		}
		if got := r.Header.Get("X-Service-Tenant"); got == "" {
			t.Error("media: X-Service-Tenant missing")
		}
		ref := strings.TrimPrefix(r.URL.Path, "/api/v1/media/download/")
		body, ok := files[ref]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write(body)
	}))
}

const (
	goodExtract = `{"engine":"tesseract","fields":{
		"fullName":{"value":"JANE WANJIKU MWANGI","confidence":0.9},
		"documentNumber":{"value":"23456789","confidence":0.95},
		"dateOfBirth":{"value":"1990-02-14","confidence":0.85}},"mrz":null}`
	goodFace    = `{"engine":"sface","score":0.93,"documentFaceFound":true,"selfieFaceFound":true}`
	cleanScreen = `{"sanctionsHit":false,"pepHit":false,"matches":[]}`
)

func goodRequest() Request {
	return Request{
		FullName:    "Jane Wanjiku Mwangi",
		NationalID:  "23456789",
		Phone:       "+254700000001",
		DateOfBirth: "1990-02-14",
		DocumentRef: "doc-1",
		SelfieRef:   "selfie-1",
	}
}

func newTestProvider(engineURL, mediaURL string) *Inhouse {
	return NewInhouse(InhouseConfig{
		EngineURL:  engineURL,
		MediaURL:   mediaURL,
		ServiceKey: "test-key",
	})
}

func TestInhouseVerify(t *testing.T) {
	media := map[string][]byte{"doc-1": []byte("doc-image"), "selfie-1": []byte("selfie-image")}

	cases := []struct {
		name    string
		engine  fakeEngine
		req     func() Request
		wantErr string
		check   func(t *testing.T, res Result)
	}{
		{
			name:   "all checks pass",
			engine: fakeEngine{extractBody: goodExtract, faceBody: goodFace, screenBody: cleanScreen},
			req:    goodRequest,
			check: func(t *testing.T, res Result) {
				if !res.DocumentVerified {
					t.Error("DocumentVerified = false, want true")
				}
				if !res.LivenessPassed {
					t.Error("LivenessPassed = false, want true")
				}
				if res.FaceMatchScore != 0.93 {
					t.Errorf("FaceMatchScore = %v, want 0.93", res.FaceMatchScore)
				}
				if res.SanctionsHit || res.PEPHit {
					t.Error("unexpected screening hit")
				}
				if !strings.HasPrefix(res.ProviderRef, "inhouse-") {
					t.Errorf("ProviderRef = %q, want inhouse- prefix", res.ProviderRef)
				}
			},
		},
		{
			name: "low field confidence fails document verification",
			engine: fakeEngine{
				extractBody: `{"fields":{
					"fullName":{"value":"JANE WANJIKU MWANGI","confidence":0.3},
					"documentNumber":{"value":"23456789","confidence":0.95}}}`,
				faceBody: goodFace, screenBody: cleanScreen,
			},
			req: goodRequest,
			check: func(t *testing.T, res Result) {
				if res.DocumentVerified {
					t.Error("DocumentVerified = true, want false (low confidence)")
				}
			},
		},
		{
			name: "missing required field fails document verification",
			engine: fakeEngine{
				extractBody: `{"fields":{"fullName":{"value":"JANE WANJIKU MWANGI","confidence":0.9}}}`,
				faceBody:    goodFace, screenBody: cleanScreen,
			},
			req: goodRequest,
			check: func(t *testing.T, res Result) {
				if res.DocumentVerified {
					t.Error("DocumentVerified = true, want false (no documentNumber)")
				}
			},
		},
		{
			name:   "declared name mismatch fails document verification",
			engine: fakeEngine{extractBody: goodExtract, faceBody: goodFace, screenBody: cleanScreen},
			req: func() Request {
				r := goodRequest()
				r.FullName = "Peter Otieno Ochieng"
				return r
			},
			check: func(t *testing.T, res Result) {
				if res.DocumentVerified {
					t.Error("DocumentVerified = true, want false (name mismatch)")
				}
			},
		},
		{
			name:   "reordered declared name still verifies",
			engine: fakeEngine{extractBody: goodExtract, faceBody: goodFace, screenBody: cleanScreen},
			req: func() Request {
				r := goodRequest()
				r.FullName = "MWANGI Jane Wanjiku"
				return r
			},
			check: func(t *testing.T, res Result) {
				if !res.DocumentVerified {
					t.Error("DocumentVerified = false, want true (token order must not matter)")
				}
			},
		},
		{
			name:   "declared national id mismatch fails document verification",
			engine: fakeEngine{extractBody: goodExtract, faceBody: goodFace, screenBody: cleanScreen},
			req: func() Request {
				r := goodRequest()
				r.NationalID = "99999999"
				return r
			},
			check: func(t *testing.T, res Result) {
				if res.DocumentVerified {
					t.Error("DocumentVerified = true, want false (id mismatch)")
				}
			},
		},
		{
			name:   "dob mismatch fails document verification",
			engine: fakeEngine{extractBody: goodExtract, faceBody: goodFace, screenBody: cleanScreen},
			req: func() Request {
				r := goodRequest()
				r.DateOfBirth = "1991-01-01"
				return r
			},
			check: func(t *testing.T, res Result) {
				if res.DocumentVerified {
					t.Error("DocumentVerified = true, want false (dob mismatch)")
				}
			},
		},
		{
			name: "sanctions and pep hits propagate",
			engine: fakeEngine{
				extractBody: goodExtract, faceBody: goodFace,
				screenBody: `{"sanctionsHit":true,"pepHit":true,"matches":[{"list":"sanctions"}]}`,
			},
			req: goodRequest,
			check: func(t *testing.T, res Result) {
				if !res.SanctionsHit || !res.PEPHit {
					t.Errorf("hits = (%v,%v), want (true,true)", res.SanctionsHit, res.PEPHit)
				}
			},
		},
		{
			name: "no selfie face means liveness failed",
			engine: fakeEngine{
				extractBody: goodExtract,
				faceBody:    `{"engine":"sface","score":0,"documentFaceFound":true,"selfieFaceFound":false}`,
				screenBody:  cleanScreen,
			},
			req: goodRequest,
			check: func(t *testing.T, res Result) {
				if res.LivenessPassed {
					t.Error("LivenessPassed = true, want false")
				}
				if res.FaceMatchScore != 0 {
					t.Errorf("FaceMatchScore = %v, want 0", res.FaceMatchScore)
				}
			},
		},
		{
			name:   "missing media refs are facts not errors",
			engine: fakeEngine{screenBody: cleanScreen},
			req: func() Request {
				r := goodRequest()
				r.DocumentRef, r.SelfieRef = "", ""
				return r
			},
			check: func(t *testing.T, res Result) {
				if res.DocumentVerified || res.LivenessPassed || res.FaceMatchScore != 0 {
					t.Errorf("want all-negative result for missing evidence, got %+v", res)
				}
			},
		},
		// ─── fail-closed: every engine/media failure is an error ────────────
		{
			name:    "screen endpoint 500 fails closed",
			engine:  fakeEngine{extractBody: goodExtract, faceBody: goodFace, screenBody: "boom", screenCode: 500},
			req:     goodRequest,
			wantErr: "/v1/screen",
		},
		{
			name:    "extract endpoint 503 fails closed",
			engine:  fakeEngine{extractBody: "no ocr", extractCode: 503, faceBody: goodFace, screenBody: cleanScreen},
			req:     goodRequest,
			wantErr: "/v1/document/extract",
		},
		{
			name:    "face endpoint 422 fails closed",
			engine:  fakeEngine{extractBody: goodExtract, faceBody: "bad image", faceCode: 422, screenBody: cleanScreen},
			req:     goodRequest,
			wantErr: "/v1/face/match",
		},
		{
			name:    "garbage engine JSON fails closed",
			engine:  fakeEngine{extractBody: "<html>gateway error</html>", faceBody: goodFace, screenBody: cleanScreen},
			req:     goodRequest,
			wantErr: "bad response",
		},
		{
			name:   "unknown media ref fails closed",
			engine: fakeEngine{extractBody: goodExtract, faceBody: goodFace, screenBody: cleanScreen},
			req: func() Request {
				r := goodRequest()
				r.DocumentRef = "missing-ref"
				return r
			},
			wantErr: "status 404",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := tc.engine
			es := engine.server(t)
			defer es.Close()
			ms := fakeMedia(t, media)
			defer ms.Close()

			p := newTestProvider(es.URL, ms.URL)
			res, err := p.Verify(context.Background(), tc.req())

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil (res=%+v)", tc.wantErr, res)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			tc.check(t, res)
		})
	}
}

func TestInhouseFailsClosedWithoutConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  InhouseConfig
	}{
		{"no engine url", InhouseConfig{MediaURL: "http://media"}},
		{"no media url", InhouseConfig{EngineURL: "http://engine"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewInhouse(tc.cfg).Verify(context.Background(), goodRequest()); err == nil {
				t.Fatal("want configuration error, got nil")
			}
		})
	}
}

func TestInhouseEngineUnreachableFailsClosed(t *testing.T) {
	p := newTestProvider("http://127.0.0.1:1", "http://127.0.0.1:1")
	if _, err := p.Verify(context.Background(), goodRequest()); err == nil {
		t.Fatal("want transport error, got nil")
	}
}

func TestInhouseScreensDeclaredName(t *testing.T) {
	engine := fakeEngine{extractBody: goodExtract, faceBody: goodFace, screenBody: cleanScreen}
	es := engine.server(t)
	defer es.Close()
	ms := fakeMedia(t, map[string][]byte{"doc-1": []byte("d"), "selfie-1": []byte("s")})
	defer ms.Close()

	p := newTestProvider(es.URL, ms.URL)
	if _, err := p.Verify(context.Background(), goodRequest()); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(engine.screenRequests) != 1 {
		t.Fatalf("screen called %d times, want 1", len(engine.screenRequests))
	}
	if got := engine.screenRequests[0]["fullName"]; got != "Jane Wanjiku Mwangi" {
		t.Errorf("screened name = %v, want declared full name", got)
	}
}

func TestInhouseRegisteredInRegistry(t *testing.T) {
	Register(NewInhouse(InhouseConfig{EngineURL: "http://e", MediaURL: "http://m"}))
	t.Setenv("EKYC_PROVIDER", "inhouse")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Name() != "inhouse" {
		t.Errorf("provider = %q, want inhouse", p.Name())
	}
}

func TestNameSimilarity(t *testing.T) {
	cases := []struct {
		a, b string
		min  float64
		max  float64
	}{
		{"Jane Mwangi", "JANE MWANGI", 1, 1},
		{"Mwangi Jane Wanjiku", "Jane Wanjiku Mwangi", 1, 1},
		{"Jane Mwangi", "Jane Wanjiku Mwangi", 0.75, 1},
		{"Jane Mwangi", "Peter Ochieng", 0, 0.5},
		{"Jane Mwangi", "Jane Ochieng", 0, 0.749},
		{"", "Jane", 0, 0},
	}
	for _, tc := range cases {
		got := nameSimilarity(tc.a, tc.b)
		if got < tc.min || got > tc.max {
			t.Errorf("nameSimilarity(%q,%q) = %v, want [%v,%v]", tc.a, tc.b, got, tc.min, tc.max)
		}
	}
}

func TestDocNumbersMatch(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"23456789", "23456789", true},
		{"23 456 789", "23456789", true},
		{"a1234567", "A1234567", true},
		{"23456789", "23456780", false},
		{"", "23456789", false},
	}
	for _, tc := range cases {
		if got := docNumbersMatch(tc.a, tc.b); got != tc.want {
			t.Errorf("docNumbersMatch(%q,%q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
