package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const downloadBufSize = 1024 * 1024

// Progress reports staged progress as done/total (total is fixed for the whole update flow).
type Progress func(done, total int64)

var defaultHTTPClient = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		MaxIdleConns:    10,
		IdleConnTimeout: 90 * time.Second,
	},
}

func httpClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return defaultHTTPClient
}

func throttleProgress(fn Progress, minInterval time.Duration) Progress {
	if fn == nil {
		return nil
	}
	var (
		lastAt   time.Time
		lastDone int64
	)
	return func(done, total int64) {
		now := time.Now()
		complete := total > 0 && done >= total
		bigJump := total > 0 && done-lastDone >= total/50
		stale := now.Sub(lastAt) >= minInterval
		if complete || bigJump || stale || lastAt.IsZero() {
			fn(done, total)
			lastAt = now
			lastDone = done
		}
	}
}

func scaleProgress(progress Progress, base, weight, stageTotal int64) Progress {
	if progress == nil {
		return nil
	}
	return func(done, total int64) {
		if total <= 0 {
			return
		}
		p := base + int64(float64(done)/float64(total)*float64(weight))
		if p > base+weight {
			p = base + weight
		}
		progress(p, stageTotal)
	}
}

func downloadURL(ctx context.Context, client *http.Client, url, dest string, progress Progress) error {
	client = httpClient(client)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "svpchain-gui")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download %s: HTTP %d: %s", url, resp.StatusCode, string(body))
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}

	total := resp.ContentLength
	var done int64
	buf := make([]byte, downloadBufSize)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				os.Remove(tmp)
				return werr
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			os.Remove(tmp)
			return readErr
		}
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if progress != nil && total > 0 {
		progress(total, total)
	}
	return os.Rename(tmp, dest)
}

func downloadBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	client = httpClient(client)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "svpchain-gui")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
