package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/getlatentic/gusto/pkg/proxy"
)

func TestAListOfModelsDoesNotCarryTheKeyToWhereARedirectPoints(t *testing.T) {
	var elsewhere atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Store(true)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer other.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v1/models", http.StatusFound)
	}))
	defer provider.Close()
	_, err := providerModels(context.Background(), provider.URL, proxy.Anthropic, "sk-a-key-that-stays")
	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Errorf("a redirect is an answer that is not a list: %v", err)
	}
	if elsewhere.Load() {
		t.Error("the redirect was followed")
	}
}
