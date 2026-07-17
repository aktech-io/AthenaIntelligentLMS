package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func descCount(c prometheus.Collector) int {
	ch := make(chan *prometheus.Desc, 32)
	c.Describe(ch)
	close(ch)
	n := 0
	for range ch {
		n++
	}
	return n
}

func TestHandlerServesDefaultRegistry(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("GET /metrics = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Error("default registry output missing go_goroutines")
	}
}

func TestDBCollectorDescribe(t *testing.T) {
	// nil pool is fine for Describe; Collect is exercised in integration.
	c := NewDBCollector(nil, "gl", GLGauges()...)
	if got, want := descCount(c), len(GLGauges())+1; got != want {
		t.Errorf("gl collector descs = %d, want %d (gauges + up)", got, want)
	}
	c = NewDBCollector(nil, "payments", PaymentGauges()...)
	if got, want := descCount(c), len(PaymentGauges())+1; got != want {
		t.Errorf("payments collector descs = %d, want %d", got, want)
	}
}

func TestGaugeNamesAreNemoNamespaced(t *testing.T) {
	for _, g := range append(GLGauges(), PaymentGauges()...) {
		if !strings.HasPrefix(g.Name, "nemo_") {
			t.Errorf("gauge %q not namespaced with nemo_", g.Name)
		}
		if g.Help == "" || g.Query == "" {
			t.Errorf("gauge %q missing help or query", g.Name)
		}
	}
}
