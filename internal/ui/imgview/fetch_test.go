package imgview_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
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

// TestFetch_SendsUserAgent guards against a regression to Go's default
// "Go-http-client/1.1", which Wikimedia's edge (and presumably other image
// hosts with a similar bot policy) rejects outright with 403 — confirmed
// live: the exact same request against a real Wikimedia-hosted GIF failed
// without this header and succeeded with it.
func TestFetch_SendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write(pngBytes(t, 1, 1))
	}))
	t.Cleanup(srv.Close)

	if _, err := imgview.Fetch(context.Background(), srv.URL); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotUA == "" || strings.HasPrefix(gotUA, "Go-http-client") {
		t.Errorf("User-Agent = %q, want a non-empty, non-default value", gotUA)
	}
}

func TestDimensions_ReturnsDeclaredSize(t *testing.T) {
	srv := serve(t, http.StatusOK, pngBytes(t, 7, 5))
	w, h, err := imgview.Dimensions(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Dimensions: %v", err)
	}
	if w != 7 || h != 5 {
		t.Errorf("Dimensions() = %dx%d, want 7x5", w, h)
	}
}

func TestDimensions_RejectsNon200(t *testing.T) {
	srv := serve(t, http.StatusNotFound, nil)
	_, _, err := imgview.Dimensions(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want 404 error", err)
	}
}

// gifBytes encodes an n-frame, wxh GIF, each frame a solid color from
// palette[i % len(palette)] so frames are visually distinguishable.
func gifBytes(t *testing.T, w, h, n int) []byte {
	t.Helper()
	palette := []color.Color{
		color.RGBA{R: 255, A: 255},
		color.RGBA{G: 255, A: 255},
		color.RGBA{B: 255, A: 255},
	}
	g := &gif.GIF{}
	for i := 0; i < n; i++ {
		pal := color.Palette{color.RGBA{}, palette[i%len(palette)]}
		frame := image.NewPaletted(image.Rect(0, 0, w, h), pal)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				frame.SetColorIndex(x, y, 1)
			}
		}
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 5)
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return buf.Bytes()
}

func TestFetchGIF_DecodesFrames(t *testing.T) {
	srv := serve(t, http.StatusOK, gifBytes(t, 4, 3, 3))
	g, err := imgview.FetchGIF(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchGIF: %v", err)
	}
	if len(g.Image) != 3 {
		t.Errorf("frame count = %d, want 3", len(g.Image))
	}
}

func TestFetchGIF_RejectsTooManyFrames(t *testing.T) {
	srv := serve(t, http.StatusOK, gifBytes(t, 1, 1, 513))
	_, err := imgview.FetchGIF(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "frame limit") {
		t.Errorf("err = %v, want frame limit error", err)
	}
}

func TestFetchGIF_RejectsExcessiveDimensions(t *testing.T) {
	// GIF89a header declaring a 65535x65535 logical screen (~4.3 gigapixels)
	// with no pixel data: DecodeConfig reads only the header.
	header := []byte("GIF89a\xff\xff\xff\xff\x00\x00\x00")
	srv := serve(t, http.StatusOK, header)
	_, err := imgview.FetchGIF(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Errorf("err = %v, want dimensions error", err)
	}
}
