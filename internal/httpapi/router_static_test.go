package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"project-helper/internal/config"
)

func TestFrontendFallbackServesIndexForNonAPIRoutes(t *testing.T) {
	router := NewRouter(testConfig(), nil, nil, nil, testFrontendFS())

	resp := performRequest(router, http.MethodGet, "/projects/123")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "project-helper") {
		t.Fatalf("expected index.html fallback, got %q", resp.Body.String())
	}
}

func TestFrontendServesEmbeddedAsset(t *testing.T) {
	router := NewRouter(testConfig(), nil, nil, nil, testFrontendFS())

	resp := performRequest(router, http.MethodGet, "/assets/app.js")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if resp.Body.String() != "console.log('ok')\n" {
		t.Fatalf("unexpected asset body %q", resp.Body.String())
	}
}

func TestFrontendDoesNotFallbackForUnknownAPI(t *testing.T) {
	router := NewRouter(testConfig(), nil, nil, nil, testFrontendFS())

	resp := performRequest(router, http.MethodGet, "/api/not-found")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.Code)
	}
	if strings.Contains(resp.Body.String(), "project-helper") {
		t.Fatalf("expected JSON 404 instead of frontend fallback, got %q", resp.Body.String())
	}
}

func TestFrontendHeadReturnsHeadersOnly(t *testing.T) {
	router := NewRouter(testConfig(), nil, nil, nil, testFrontendFS())

	resp := performRequest(router, http.MethodHead, "/assets/app.js")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if resp.Body.Len() != 0 {
		t.Fatalf("expected empty HEAD body, got %q", resp.Body.String())
	}
}

func testConfig() config.Config {
	return config.Config{FrontendURL: "http://localhost:5173"}
}

func testFrontendFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>project-helper</title>")},
		"assets/app.js": {Data: []byte("console.log('ok')\n")},
		"favicon.svg":   {Data: []byte("<svg></svg>")},
	}
}

func performRequest(handler http.Handler, method string, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}
