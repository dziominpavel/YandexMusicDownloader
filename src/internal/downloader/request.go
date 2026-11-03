package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// statusError is an HTTP answer we do not retry blindly. The public
// probe/search methods translate its code into the sentinel errors below.
type statusError struct {
	Code int
	URL  string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("http %d: %s", e.Code, e.URL)
}

// doJSON performs a GET with the same auth/UA headers as the rest of the
// client, retrying only temporary failures (network, 429, 5xx). It is
// used by the quality probe/search paths, which need a caller-supplied
// context instead of the job-scoped one.
func (c *Client) doJSON(ctx context.Context, url string, out any) error {
	return c.doJSONWithUID(ctx, url, out, 0)
}

// doJSONWithUID adds the multi-auth header Yandex requires for lossless
// get-file-info. Search works without it, so uid=0 is allowed.
func (c *Client) doJSONWithUID(ctx context.Context, url string, out any, uid int64) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		req, err := c.newRequestWithContext(ctx, http.MethodGet, url)
		if err != nil {
			return err
		}
		req.Header.Set("x-yandex-music-client", "YandexMusicWebNext/1.0.0")
		req.Header.Set("Referer", "https://music.yandex.ru/")
		req.Header.Set("Origin", "https://music.yandex.ru")
		if uid > 0 {
			req.Header.Set("x-yandex-music-multi-auth-user-id", fmt.Sprintf("%d", uid))
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: %v", ErrCheckFailed, err)
			if attempt < 2 && isRetryableNetworkErr(err) {
				if !sleepCtx(ctx, retryDelay(attempt)) {
					return ctx.Err()
				}
				continue
			}
			return lastErr
		}

		if resp.StatusCode == http.StatusOK {
			err := json.NewDecoder(resp.Body).Decode(out)
			resp.Body.Close()
			if err != nil {
				return fmt.Errorf("%w: decode: %v", ErrCheckFailed, err)
			}
			return nil
		}
		resp.Body.Close()
		se := &statusError{Code: resp.StatusCode, URL: url}
		lastErr = se
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && attempt < 2 {
			if !sleepCtx(ctx, retryDelay(attempt)) {
				return ctx.Err()
			}
			continue
		}
		return se
	}
	return lastErr
}

func retryDelay(attempt int) time.Duration {
	return time.Duration(attempt+1) * 150 * time.Millisecond
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// classifyStatus turns a non-2xx answer into the quality sentinel error.
func classifyStatus(err error) error {
	var se *statusError
	if !errors.As(err, &se) {
		return err
	}
	switch se.Code {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %v", ErrAuth, err)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %v", ErrForbidden, err)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	default:
		return fmt.Errorf("%w: %v", ErrCheckFailed, err)
	}
}
