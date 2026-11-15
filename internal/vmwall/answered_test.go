package vmwall

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestADownloadKnowsAServerThatHasNoSuchFileFromOneThatFailed(t *testing.T) {
	codes := map[string]int{"/gone": http.StatusNotFound, "/broken": http.StatusInternalServerError}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(codes[r.URL.Path]) }))
	defer server.Close()
	gone := OverHTTP(t.Context(), server.URL+"/gone", io.Discard)
	if !answeredNotFound(fmt.Errorf("the release: %w", gone)) {
		t.Fatalf("a 404 was not known as no such file: %v", gone)
	}
	if broken := OverHTTP(t.Context(), server.URL+"/broken", io.Discard); broken == nil || answeredNotFound(broken) {
		t.Fatalf("a 500 was taken for no such file: %v", broken)
	}
}
