package downloader

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestResumeDownload(t *testing.T) {
	full := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rg := r.Header.Get("Range")
		if rg == "" {
			w.Header().Set("ETag", `"e1"`)
			w.Write(full)
			return
		}
		var off int
		fmt := "bytes=%d-"
		if _, err := http.ParseCookie("x"); err == nil {
			_ = fmt
		}
		n, _ := strconv.Atoi(rg[6 : len(rg)-1])
		off = n
		w.Header().Set("Content-Range", "bytes "+strconv.Itoa(off)+"-"+strconv.Itoa(len(full)-1)+"/"+strconv.Itoa(len(full)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(full[off:])
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "part.bin")
	if err := os.WriteFile(dst, full[:10], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst+".meta", []byte(`"e1"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var done int64 = -1
	err := DownloadToFileResume(context.Background(), srv.URL+"/f", dst, ResumeOptions{OnProgress: func(d, t int64) { done = d }})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(full) {
		t.Fatalf("got %q want %q", got, full)
	}
	if done <= 0 {
		t.Fatal("no progress callback")
	}
}

func TestRedirectToHTTPRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.example/x", 302)
	}))
	defer srv.Close()
	if err := DownloadToFileResume(context.Background(), srv.URL+"/f", filepath.Join(t.TempDir(), "x"), ResumeOptions{}); err == nil {
		t.Fatal("http redirect accepted")
	}
}

func rangeServer(t *testing.T, full []byte, etag string, stall bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
		rg := r.Header.Get("Range")
		if rg == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(full)))
			w.WriteHeader(200)
			if stall {
				w.(http.Flusher).Flush()
				time.Sleep(60 * time.Second)
				return
			}
			w.Write(full)
			return
		}
		var off int
		fmt.Sscanf(rg, "bytes=%d-", &off)
		if r.Header.Get("If-Range") != "" && r.Header.Get("If-Range") != etag && r.Header.Get("If-Range") != "Mon, 01 Jan 2024 00:00:00 GMT" {
			// If-Range mismatch: 200 full
			w.WriteHeader(200)
			w.Write(full)
			return
		}
		w.Header().Set("Content-Range", "bytes "+strconv.Itoa(off)+"-"+strconv.Itoa(len(full)-1)+"/"+strconv.Itoa(len(full)))
		w.WriteHeader(206)
		w.Write(full[off:])
	}))
}

func TestRange200Restarts(t *testing.T) {
	full := []byte("0123456789")
	srv := rangeServer(t, full, `"e2"`, false)
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "f")
	os.WriteFile(dst, full[:4], 0o600)
	os.WriteFile(dst+".meta", []byte(`"other"`+"\n"), 0o600)
	if err := DownloadToFileResume(context.Background(), srv.URL, dst, ResumeOptions{}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(full) {
		t.Fatalf("got %q", got)
	}
}

func TestSizeLimitRejects(t *testing.T) {
	full := make([]byte, 1024)
	srv := rangeServer(t, full, `"e3"`, false)
	defer srv.Close()
	if err := DownloadToFileResume(context.Background(), srv.URL, filepath.Join(t.TempDir(), "f"), ResumeOptions{MaxBytes: 10}); err == nil {
		t.Fatal("oversize accepted")
	}
}

func TestProgressMonotonic(t *testing.T) {
	full := []byte("0123456789abcdef0123456789abcdef")
	srv := rangeServer(t, full, `"e4"`, false)
	defer srv.Close()
	var last int64 = -1
	var vals []int64
	err := DownloadToFileResume(context.Background(), srv.URL, filepath.Join(t.TempDir(), "f"), ResumeOptions{OnProgress: func(d, tot int64) {
		if d <= last && last != -1 {
			t.Error("non-monotonic")
		}
		last = d
		vals = append(vals, d)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if last != int64(len(full)) {
		t.Fatalf("ended at %d", last)
	}
}

// B5: redirect to plain http is refused.
func TestRedirectHTTPRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/x", 302)
	}))
	defer srv.Close()
	f, _ := os.Create(filepath.Join(t.TempDir(), "o"))
	defer f.Close()
	if err := downloadToFileOnce(context.Background(), srv.URL, f); err == nil {
		t.Fatal("http redirect followed")
	}
}

// B5: body over the cap is refused.
func TestSizeCapRefused(t *testing.T) {
	old := maxDownloadBytes
	maxDownloadBytes = 8
	defer func() { maxDownloadBytes = old }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0123456789abcdef"))
	}))
	defer srv.Close()
	f, _ := os.Create(filepath.Join(t.TempDir(), "o"))
	defer f.Close()
	if err := downloadToFileOnce(context.Background(), srv.URL, f); err == nil {
		t.Fatal("oversize body accepted")
	}
}
