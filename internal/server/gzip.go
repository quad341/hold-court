package server

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strings"
	"sync"
)

// gzipHandler compresses text responses for clients that accept gzip.
// Hold Court's biggest response, the holds document, is markdown rendered
// to HTML for every hold; it compresses several-fold, which matters once
// the bench is served over a tailnet (tailscale serve does not compress)
// rather than localhost. Only 200 responses with a compressible
// Content-Type are encoded: a 304 has no body to compress, and a 206
// partial response's byte ranges refer to the uncompressed entity.
func gzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The representation depends on Accept-Encoding whether or not this
		// response ends up compressed, so a shared cache never serves an
		// encoded body to a client that did not ask for one.
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.Close()
		next.ServeHTTP(gw, r)
	})
}

func acceptsGzip(r *http.Request) bool {
	for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, _, _ := strings.Cut(strings.TrimSpace(enc), ";")
		if strings.EqualFold(name, "gzip") {
			return true
		}
	}
	return false
}

// compressibleTypes lists the media types worth encoding: the page, the
// holds and other JSON documents, and the static assets. Images other
// than SVG, and anything already encoded, are passed through.
var compressibleTypes = map[string]bool{
	"text/html":              true,
	"text/plain":             true,
	"text/css":               true,
	"text/javascript":        true,
	"application/javascript": true,
	"application/json":       true,
	"image/svg+xml":          true,
}

var gzipWriters = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// gzipResponseWriter decides on the first WriteHeader/Write, once the
// handler has set its status and Content-Type, whether to encode the body.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	decided bool
}

func (w *gzipResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.decide(status)
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if !w.decided {
		w.decide(http.StatusOK)
		w.ResponseWriter.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

func (w *gzipResponseWriter) decide(status int) {
	if w.decided {
		return
	}
	w.decided = true
	h := w.Header()
	if status != http.StatusOK || h.Get("Content-Encoding") != "" {
		return
	}
	mediaType, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil || !compressibleTypes[mediaType] {
		return
	}
	// Content-Length, if a handler set one (http.FileServer does), describes
	// the uncompressed body; the encoded length is not known up front.
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	w.gz = gzipWriters.Get().(*gzip.Writer)
	w.gz.Reset(w.ResponseWriter)
}

// Close flushes the encoded body. A handler that wrote nothing at all
// (neither headers nor body) leaves the response untouched.
func (w *gzipResponseWriter) Close() {
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	gzipWriters.Put(w.gz)
	w.gz = nil
}
