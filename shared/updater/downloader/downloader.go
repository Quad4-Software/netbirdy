package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/version"
)

const (
	userAgent         = "NetBirdy agent installer/%s"
	DefaultRetryDelay = 3 * time.Second
)

func DownloadToFile(ctx context.Context, retryDelay time.Duration, url, dstFile string) error {
	log.Debugf("starting download from %s", url)

	out, err := os.Create(dstFile)
	if err != nil {
		return fmt.Errorf("failed to create destination file %q: %w", dstFile, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Warnf("error closing file %q: %v", dstFile, cerr)
		}
	}()

	// First attempt
	err = downloadToFileOnce(ctx, url, out)
	if err == nil {
		log.Infof("successfully downloaded file to %s", dstFile)
		return nil
	}

	// If retryDelay is 0, don't retry
	if retryDelay == 0 {
		return err
	}

	log.Warnf("download failed, retrying after %v: %v", retryDelay, err)

	// Sleep before retry
	if sleepErr := sleepWithContext(ctx, retryDelay); sleepErr != nil {
		return fmt.Errorf("download cancelled during retry delay: %w", sleepErr)
	}

	// Truncate file before retry
	if err := out.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate file on retry: %w", err)
	}
	if _, err := out.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek to beginning of file: %w", err)
	}

	// Second attempt
	if err := downloadToFileOnce(ctx, url, out); err != nil {
		return fmt.Errorf("download failed after retry: %w", err)
	}

	log.Infof("successfully downloaded file to %s", dstFile)
	return nil
}

func DownloadToMemory(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add User-Agent header
	req.Header.Set("User-Agent", fmt.Sprintf(userAgent, version.NetbirdVersion()))

	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform HTTP request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Warnf("error closing response body: %v", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return data, nil
}

func downloadToFileOnce(ctx context.Context, url string, out *os.File) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add User-Agent header
	req.Header.Set("User-Agent", fmt.Sprintf(userAgent, version.NetbirdVersion()))

	resp, err := downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to perform HTTP request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Warnf("error closing response body: %v", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	if resp.ContentLength > maxDownloadBytes {
		return fmt.Errorf("response too large: %d bytes", resp.ContentLength)
	}
	if _, err := io.Copy(out, io.LimitReader(resp.Body, maxDownloadBytes+1)); err != nil {
		return fmt.Errorf("failed to write response body to file: %w", err)
	}

	return nil
}

var maxDownloadBytes int64 = 1 << 30

var downloadClient = &http.Client{
	Timeout: 10 * time.Minute,
	CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || len(via) > 5 {
			return fmt.Errorf("disallowed redirect to %s", r.URL.Scheme)
		}
		return nil
	},
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	select {
	case <-time.After(duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ResumeOptions controls a resumable download.
type ResumeOptions struct {
	MaxBytes       int64
	IdleTimeout    time.Duration
	OverallTimeout time.Duration
	OnProgress     func(done, total int64)
}

// DownloadToFileResume downloads with HTTP Range resume against a partial
// file at dstFile (the server sends 206 with Content-Range matching the
// saved offset, or the file restarts from zero).
func DownloadToFileResume(ctx context.Context, url, dstFile string, opt ResumeOptions) error {
	if opt.IdleTimeout <= 0 {
		opt.IdleTimeout = 30 * time.Second
	}
	if opt.OverallTimeout <= 0 {
		opt.OverallTimeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, opt.OverallTimeout)
	defer cancel()

	var offset int64
	var etag, lastmod string
	if st, err := os.Stat(dstFile); err == nil && st.Size() > 0 {
		offset = st.Size()
	}
	if b, err := os.ReadFile(dstFile + ".meta"); err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(b)), "\n", 2)
		if len(parts) > 0 {
			etag = parts[0]
		}
		if len(parts) > 1 {
			lastmod = parts[1]
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		if etag != "" {
			req.Header.Set("If-Range", etag)
		} else if lastmod != "" {
			req.Header.Set("If-Range", lastmod)
		}
	}

	client := &http.Client{
		Timeout: 0, // governed by ctx and idle detection below
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if r.URL.Scheme != "https" || len(via) > 5 {
				return errors.New("disallowed redirect")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	appendMode := false
	switch resp.StatusCode {
	case http.StatusOK:
		offset = 0
	case http.StatusPartialContent:
		var start, total int64
		if _, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, new(int64), &total); err != nil {
			return errors.New("bad Content-Range")
		}
		if start != offset {
			offset = 0
		} else {
			appendMode = true
			if opt.MaxBytes > 0 && total > opt.MaxBytes {
				return errors.New("size limit")
			}
		}
	default:
		return fmt.Errorf("download: %d", resp.StatusCode)
	}

	flag := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(dstFile, flag, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()

	var total int64 = -1
	if cl := resp.ContentLength; cl > 0 {
		total = cl + offset
	}
	if opt.MaxBytes > 0 && total > 0 && total > opt.MaxBytes {
		return errors.New("size limit")
	}

	var done = offset
	idle := time.NewTimer(opt.IdleTimeout)
	defer idle.Stop()
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-idle.C:
			return errors.New("idle timeout")
		default:
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if !idle.Stop() {
				<-idle.C
			}
			idle.Reset(opt.IdleTimeout)
			m, werr := f.Write(buf[:n])
			done += int64(m)
			if werr != nil {
				return werr
			}
			if opt.OnProgress != nil {
				opt.OnProgress(done, total)
			}
			if opt.MaxBytes > 0 && done > opt.MaxBytes {
				return errors.New("size limit")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if e := resp.Header.Get("ETag"); e != "" {
		os.WriteFile(dstFile+".meta", []byte(e+"\n"+resp.Header.Get("Last-Modified")), 0o600)
	}
	return nil
}
