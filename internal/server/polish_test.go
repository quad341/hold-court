package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeHTTP_StaticServesFavicon(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/static/favicon.svg", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "<svg") {
		t.Error("favicon response body does not look like an SVG")
	}
}

func TestServeHTTP_IndexHasFaviconLink(t *testing.T) {
	h := newTestHandler(t)
	body := getIndexBody(t, h)
	if !strings.Contains(body, `rel="icon"`) {
		t.Error("index page missing <link rel=\"icon\"> for favicon")
	}
}

func TestServeHTTP_IndexHasHeaderBar(t *testing.T) {
	h := newTestHandler(t)
	body := getIndexBody(t, h)
	if !strings.Contains(body, `id="app-header"`) {
		t.Error("index page missing app header/status bar")
	}
}

func TestServeHTTP_StyleCSSHasDarkTheme(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "prefers-color-scheme") {
		t.Error("style.css missing a prefers-color-scheme dark theme block")
	}
}
