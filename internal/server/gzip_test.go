package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gunzip(t *testing.T, body []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	return string(out)
}

func TestGzip_CompressesPageAndJSONForAcceptingClients(t *testing.T) {
	h := newTestHandler(t)
	tests := []struct {
		path        string
		contentType string
		marker      string
	}{
		{"/", "text/html; charset=utf-8", `id="pane-reading"`},
		{"/api/holds", "application/json", `"title":"Push-tier relaxation"`},
		{"/static/app.js", "text/javascript; charset=utf-8", "pollHolds"},
		{"/static/style.css", "text/css; charset=utf-8", "prefers-color-scheme"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Accept-Encoding", "gzip, deflate, br")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d", w.Code)
			}
			if got := w.Header().Get("Content-Encoding"); got != "gzip" {
				t.Fatalf("Content-Encoding = %q, want gzip", got)
			}
			if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("Vary = %q, want Accept-Encoding", got)
			}
			if got := w.Header().Get("Content-Type"); got != tt.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.contentType)
			}
			if w.Header().Get("Content-Length") != "" {
				t.Error("Content-Length of the uncompressed body leaked onto the encoded response")
			}
			if body := gunzip(t, w.Body.Bytes()); !strings.Contains(body, tt.marker) {
				t.Errorf("decoded body missing %q", tt.marker)
			}

			plain := httptest.NewRecorder()
			h.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if plain.Header().Get("Content-Encoding") != "" {
				t.Error("client without Accept-Encoding received an encoded body")
			}
			if plain.Header().Get("Vary") != "Accept-Encoding" {
				t.Error("uncompressed response must still declare Vary: Accept-Encoding")
			}
			if !strings.Contains(plain.Body.String(), tt.marker) {
				t.Errorf("plain body missing %q", tt.marker)
			}
		})
	}
}

func TestGzip_LeavesBodylessAndNonTextResponsesAlone(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/holds", nil))
	etag := w.Header().Get("ETag")

	req := httptest.NewRequest(http.MethodGet, "/api/holds", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotModified || w.Header().Get("Content-Encoding") != "" || w.Body.Len() != 0 {
		t.Errorf("304 must not be encoded: status %d, encoding %q, %d body bytes", w.Code, w.Header().Get("Content-Encoding"), w.Body.Len())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/holds/"+fixtureHoldID+"/read", strings.NewReader(`{"unread":true}`))
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || w.Header().Get("Content-Encoding") != "" {
		t.Errorf("204 must not be encoded: status %d, encoding %q", w.Code, w.Header().Get("Content-Encoding"))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/holds/"+fixtureHoldID+"/history", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("JSON history should be encoded: status %d, encoding %q", w.Code, w.Header().Get("Content-Encoding"))
	}
}
