package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateProviderRefusesTheBridgeName(t *testing.T) {
	body := `{"name":" Claude (abonnement) ","kind":"openai","base_url":"https://x/v1"}`
	rec := httptest.NewRecorder()
	(&Server{}).createProvider(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
}
