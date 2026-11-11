package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPagesUseEmbeddedTemplates(t *testing.T) {
	router := NewHandler(nil).RegisterRouter()
	for _, route := range []string{"/login", "/register"} {
		t.Run(route, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s: status %d", route, response.Code)
			}
			if !strings.Contains(response.Header().Get("Content-Type"), "text/html") || !strings.Contains(response.Body.String(), "<form") {
				t.Fatalf("GET %s did not render an HTML form", route)
			}
		})
	}
}
