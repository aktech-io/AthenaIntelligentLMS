package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// HIGH-6: the scoring client must fail CLOSED — never return a fabricated
// score when the scoring service is down, erroring, or replies incompletely.

func TestGetLatestScore_FailsClosedOnTransportError(t *testing.T) {
	c := NewScoringClient("http://127.0.0.1:1", "key", zap.NewNop())
	if _, err := c.GetLatestScore(context.Background(), "cust-1"); err == nil {
		t.Fatal("expected error on unreachable scoring service, got fabricated score")
	}
}

func TestGetLatestScore_FailsClosedOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewScoringClient(srv.URL, "key", zap.NewNop())
	if _, err := c.GetLatestScore(context.Background(), "cust-1"); err == nil {
		t.Fatal("expected error on 500 from scoring service, got fabricated score")
	}
}

func TestGetLatestScore_FailsClosedOnIncompleteResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewScoringClient(srv.URL, "key", zap.NewNop())
	if _, err := c.GetLatestScore(context.Background(), "cust-1"); err == nil {
		t.Fatal("expected error on incomplete scoring payload, got fabricated score")
	}
}

func TestGetLatestScore_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"finalScore": 720, "scoreBand": "A"}`))
	}))
	defer srv.Close()

	c := NewScoringClient(srv.URL, "key", zap.NewNop())
	got, err := c.GetLatestScore(context.Background(), "cust-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Score != 720 || got.Band != "A" {
		t.Errorf("got %+v, want {Score:720 Band:A}", got)
	}
}
