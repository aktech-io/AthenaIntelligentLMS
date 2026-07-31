package liveness

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePAD is a stub ekyc-ml-service liveness endpoint: canned JSON or status.
type fakePAD struct {
	body string
	code int

	frames []int // frame count per call
}

func (f *fakePAD) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/face/liveness" {
			t.Errorf("engine: unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Errorf("liveness: bad multipart: %v", err)
		}
		frames := 0
		if r.MultipartForm != nil {
			frames = len(r.MultipartForm.File["frame"])
		}
		f.frames = append(f.frames, frames)
		code := f.code
		if code == 0 {
			code = http.StatusOK
		}
		w.WriteHeader(code)
		fmt.Fprint(w, f.body)
	}))
}

func frames(n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = []byte(fmt.Sprintf("frame-%d", i))
	}
	return out
}

func TestInhouseScore(t *testing.T) {
	pad := fakePAD{body: `{"liveScore":0.87,"label":"LIVE","model":"minifasnet_v2"}`}
	srv := pad.server(t)
	defer srv.Close()

	p := NewInhouse(InhouseConfig{EngineURL: srv.URL})
	res, err := p.Score(context.Background(), frames(3))
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if res.LiveScore != 0.87 || res.Label != "LIVE" {
		t.Errorf("result = (%v, %s), want (0.87, LIVE)", res.LiveScore, res.Label)
	}
	if res.Provider != "inhouse" {
		t.Errorf("Provider = %q, want inhouse", res.Provider)
	}
	if !strings.HasPrefix(res.AuditRef, "inhouse-liveness-") {
		t.Errorf("AuditRef = %q, want inhouse-liveness- prefix", res.AuditRef)
	}
	if len(pad.frames) != 1 || pad.frames[0] != 3 {
		t.Errorf("frames per call = %v, want one call with 3", pad.frames)
	}
}

func TestInhouseScoreCapsFrames(t *testing.T) {
	pad := fakePAD{body: `{"liveScore":0.5,"label":"UNKNOWN","model":"fallback"}`}
	srv := pad.server(t)
	defer srv.Close()

	p := NewInhouse(InhouseConfig{EngineURL: srv.URL})
	if _, err := p.Score(context.Background(), frames(9)); err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(pad.frames) != 1 || pad.frames[0] != maxFrames {
		t.Errorf("frames per call = %v, want one call capped at %d", pad.frames, maxFrames)
	}
}

// Fail-closed: config gaps, empty input and engine failures are all errors —
// the caller's shadow/enforce logic decides what to do with them.
func TestInhouseScoreFailsClosed(t *testing.T) {
	t.Run("no engine url", func(t *testing.T) {
		if _, err := NewInhouse(InhouseConfig{}).Score(context.Background(), frames(1)); err == nil {
			t.Fatal("want configuration error, got nil")
		}
	})
	t.Run("no frames", func(t *testing.T) {
		if _, err := NewInhouse(InhouseConfig{EngineURL: "http://e"}).Score(context.Background(), nil); err == nil {
			t.Fatal("want no-frames error, got nil")
		}
	})
	t.Run("engine 503", func(t *testing.T) {
		pad := fakePAD{body: "model missing", code: 503}
		srv := pad.server(t)
		defer srv.Close()
		_, err := NewInhouse(InhouseConfig{EngineURL: srv.URL}).Score(context.Background(), frames(1))
		if err == nil || !strings.Contains(err.Error(), "status 503") {
			t.Fatalf("error = %v, want status 503", err)
		}
	})
	t.Run("garbage engine JSON", func(t *testing.T) {
		pad := fakePAD{body: "<html>gateway error</html>"}
		srv := pad.server(t)
		defer srv.Close()
		_, err := NewInhouse(InhouseConfig{EngineURL: srv.URL}).Score(context.Background(), frames(1))
		if err == nil || !strings.Contains(err.Error(), "bad response") {
			t.Fatalf("error = %v, want bad response", err)
		}
	})
	t.Run("engine unreachable", func(t *testing.T) {
		if _, err := NewInhouse(InhouseConfig{EngineURL: "http://127.0.0.1:1"}).Score(context.Background(), frames(1)); err == nil {
			t.Fatal("want transport error, got nil")
		}
	})
}

func TestRegistryFromEnv(t *testing.T) {
	Register(NewInhouse(InhouseConfig{EngineURL: "http://e"}))

	t.Run("default is inhouse", func(t *testing.T) {
		t.Setenv("LIVENESS_PROVIDER", "")
		p, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv() error = %v", err)
		}
		if p.Name() != "inhouse" {
			t.Errorf("provider = %q, want inhouse", p.Name())
		}
	})
	t.Run("explicit selection is case-insensitive", func(t *testing.T) {
		t.Setenv("LIVENESS_PROVIDER", "InHouse")
		p, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv() error = %v", err)
		}
		if p.Name() != "inhouse" {
			t.Errorf("provider = %q, want inhouse", p.Name())
		}
	})
	t.Run("unknown provider errors", func(t *testing.T) {
		t.Setenv("LIVENESS_PROVIDER", "no-such-vendor")
		if _, err := FromEnv(); err == nil {
			t.Fatal("want unknown-provider error, got nil")
		}
	})
}
