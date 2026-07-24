package imgview_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/imgview"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func serve(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetch_DecodesValidPNG(t *testing.T) {
	srv := serve(t, http.StatusOK, pngBytes(t, 4, 3))
	img, err := imgview.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 4 || b.Dy() != 3 {
		t.Errorf("bounds = %v, want 4x3", b)
	}
}

func TestFetch_RejectsOversizedBody(t *testing.T) {
	srv := serve(t, http.StatusOK, make([]byte, 10<<20+1))
	_, err := imgview.Fetch(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Errorf("err = %v, want size limit error", err)
	}
}

func TestFetch_RejectsExcessiveDimensions(t *testing.T) {
	// GIF89a header declaring a 65535x65535 logical screen (~4.3 gigapixels)
	// with no pixel data: DecodeConfig reads only the header.
	header := []byte("GIF89a\xff\xff\xff\xff\x00\x00\x00")
	srv := serve(t, http.StatusOK, header)
	_, err := imgview.Fetch(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Errorf("err = %v, want dimensions error", err)
	}
}

func TestFetch_RejectsNon200(t *testing.T) {
	srv := serve(t, http.StatusNotFound, nil)
	_, err := imgview.Fetch(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want 404 error", err)
	}
}
