package seniors_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/genxcare/api/internal/seniors"
)

// `/seniors/summary` and `/seniors/{seniorID}` are the same shape to a router.
// Senior IDs are UUIDs so the two can never collide in practice, but relying on
// that rather than on the literal path winning would be a trap for whoever adds
// the next sibling route.
func TestSummaryRouteWinsOverTheSeniorParameter(t *testing.T) {
	reached := false
	summary := chi.NewRouter()
	summary.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	router := seniors.NewHandler(nil, nil).Routes(seniors.SubRoutes{Summary: summary})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/summary", nil))

	if !reached {
		t.Fatalf("want /summary handled by the summary router, got status %d", recorder.Code)
	}
}
