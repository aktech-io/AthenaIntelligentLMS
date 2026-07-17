package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/scoring/model"
)

func TestGetScore_Success_DecodesContractAndSendsAPIKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		if r.URL.Path != "/api/v1/credit-score/42" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"customer_id": 42,
			"final_score": 712,
			"score_band": "Very Good",
			"pd_probability": 0.031,
			"scored_at": "2026-07-18T10:00:00Z",
			"status": "SCORED",
			"data_sufficiency": "FULL",
			"pd_source": "lgbm:champion",
			"model_version": "3"
		}`))
	}))
	defer srv.Close()

	c := NewAthenaScoreClient(srv.URL, "test-key", zap.NewNop())
	score, err := c.GetScore(context.Background(), 42)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if gotKey != "test-key" {
		t.Errorf("X-Api-Key not sent, got %q", gotKey)
	}
	if score.FinalScore.IntPart() != 712 {
		t.Errorf("final_score = %s, want 712", score.FinalScore)
	}
	if score.Status != "SCORED" {
		t.Errorf("status = %q, want SCORED", score.Status)
	}
	if band := model.ScoreBandFromString(score.ScoreBand); band != model.ScoreBandVeryGood {
		t.Errorf("band %q normalized to %q, want VERY_GOOD", score.ScoreBand, band)
	}
}

func TestGetScore_Non2xx_ReturnsErrorNotMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewAthenaScoreClient(srv.URL, "test-key", zap.NewNop())
	score, err := c.GetScore(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error on 500, got nil (mock fallback regression?)")
	}
	if score != nil {
		t.Fatalf("expected nil score on failure, got %+v (mock fallback regression?)", score)
	}
}

func TestGetScore_Unreachable_ReturnsError(t *testing.T) {
	// Nothing listens here — must fail closed, never fabricate a score.
	c := NewAthenaScoreClient("http://127.0.0.1:1", "test-key", zap.NewNop())
	score, err := c.GetScore(context.Background(), 42)
	if err == nil || score != nil {
		t.Fatal("expected error when scoring API is unreachable")
	}
}

func TestScoreBandNormalization(t *testing.T) {
	cases := map[string]model.ScoreBand{
		"Excellent": model.ScoreBandExcellent,
		"Very Good": model.ScoreBandVeryGood,
		"VERY_GOOD": model.ScoreBandVeryGood,
		"good":      model.ScoreBandGood,
		"Marginal":  model.ScoreBandMarginal,
		"unknown":   model.ScoreBandPoor,
	}
	for in, want := range cases {
		if got := model.ScoreBandFromString(in); got != want {
			t.Errorf("ScoreBandFromString(%q) = %q, want %q", in, got, want)
		}
	}
}
