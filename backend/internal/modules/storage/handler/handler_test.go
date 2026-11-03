package handler

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/storage/model"
)

// multipartBody builds a multipart/form-data body with a single "file" part of
// fileBytes bytes, returning the body and its Content-Type.
func multipartBody(t *testing.T, fileBytes int) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "big.bin")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(bytes.Repeat([]byte("a"), fileBytes)); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

func TestUploadRejectsOversizeBody(t *testing.T) {
	// Small per-file cap; body exceeds cap + multipart overhead, so the body
	// reader trips before svc is ever reached (nil svc is therefore safe).
	h := New(nil, 1)
	body, ctype := multipartBody(t, int(h.maxUploadBytes+multipartOverhead)+1024)

	req := httptest.NewRequest(http.MethodPost, "/v1/platform/storage/objects", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Fatalf("body missing PAYLOAD_TOO_LARGE: %s", rec.Body.String())
	}
}

func TestUploadRejectsFileOverExactLimit(t *testing.T) {
	// File fits within the body cap (limit + 1MiB slack) so ParseMultipartForm
	// succeeds, but exceeds the exact per-file limit, so the post-parse check
	// returns 413 before svc is reached.
	const maxBytes = 100 << 10
	h := New(nil, maxBytes)
	body, ctype := multipartBody(t, maxBytes+(50<<10)) // 150KiB file, 100KiB cap

	req := httptest.NewRequest(http.MethodPost, "/v1/platform/storage/objects", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Fatalf("body missing PAYLOAD_TOO_LARGE: %s", rec.Body.String())
	}
}

func TestUploadRejectsMalformedForm(t *testing.T) {
	// Claims to be multipart but the body does not match the boundary.
	req := httptest.NewRequest(http.MethodPost, "/v1/platform/storage/objects",
		strings.NewReader("not a real multipart body"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=zzz")
	rec := httptest.NewRecorder()

	New(nil, 1<<20).Upload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestNewClampsNonPositiveLimit(t *testing.T) {
	if got := New(nil, 0).maxUploadBytes; got != defaultMaxUploadBytes {
		t.Errorf("maxUploadBytes = %d, want default %d", got, defaultMaxUploadBytes)
	}
	if got := New(nil, -5).maxUploadBytes; got != defaultMaxUploadBytes {
		t.Errorf("maxUploadBytes = %d, want default %d", got, defaultMaxUploadBytes)
	}
}

func TestWriteFileForcesDownloadForActiveContent(t *testing.T) {
	cases := map[string]string{
		"text/html":              "attachment",
		"image/svg+xml":          "attachment",
		"application/xhtml+xml":  "attachment",
		"text/html; charset=utf": "attachment",
		"":                       "attachment",
		"image/png":              "inline",
		"application/pdf":        "inline",
		"video/mp4":              "inline",
	}
	for mime, want := range cases {
		rec := httptest.NewRecorder()
		writeFile(rec, model.OpenObject{
			Object: model.Object{Name: "a\"b\r\n.html", MimeType: mime},
			Body:   io.NopCloser(strings.NewReader("x")),
		})
		disp := rec.Header().Get("Content-Disposition")
		if !strings.HasPrefix(disp, want+";") {
			t.Fatalf("%q: disposition %q, want %s", mime, disp, want)
		}
		if strings.ContainsAny(disp, "\r\n") || strings.Count(disp, `"`) != 2 {
			t.Fatalf("%q: unsafe filename in %q", mime, disp)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%q: nosniff missing", mime)
		}
	}
}
