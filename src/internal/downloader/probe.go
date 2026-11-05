package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Quality probe errors. They are deliberately distinct: an auth/preview
// answer must stop the monitor, while a codec answer is a normal status.
var (
	ErrPreview     = errors.New("token does not grant full access (preview only)")
	ErrAuth        = errors.New("authorization required")
	ErrForbidden   = errors.New("access forbidden")
	ErrUnavailable = errors.New("track unavailable")
	ErrCheckFailed = errors.New("quality check failed")
)

// Probe is the availability answer for one track, without downloading it.
type Probe struct {
	Available bool
	Codec     string // flac, flac-mp4, mp3, aac-mp4, ...
	Quality   string // lossless, nq, smart_preview, ...
	Bitrate   int
}

// AccountState returns the account uid and Plus flag, cached for the
// lifetime of the client. Every lossless request needs the uid; the
// monitor uses Plus to avoid marking a whole collection as "no FLAC"
// when the subscription is missing.
func (c *Client) AccountState(ctx context.Context) (int64, bool, error) {
	c.accountMu.Lock()
	defer c.accountMu.Unlock()
	if c.accountSet {
		return c.accountID, c.accountPlus, nil
	}
	var data struct {
		Result struct {
			Account struct {
				UID int64 `json:"uid"`
			} `json:"account"`
			Plus struct {
				HasPlus bool `json:"hasPlus"`
			} `json:"plus"`
		} `json:"result"`
	}
	if err := c.doJSON(ctx, c.baseURL()+"/account/status", &data); err != nil {
		return 0, false, classifyStatus(err)
	}
	if data.Result.Account.UID == 0 {
		return 0, false, fmt.Errorf("%w: no account uid", ErrAuth)
	}
	c.accountID = data.Result.Account.UID
	c.accountPlus = data.Result.Plus.HasPlus
	c.accountSet = true
	return c.accountID, c.accountPlus, nil
}

// accountUID is the short form used by the download pipeline.
func (c *Client) accountUID() (int64, error) {
	return c.accountUIDCtx(c.reqCtx())
}

// accountUIDCtx is accountUID for one task context: a cancelled task must
// not inherit the whole job's cancellation state.
func (c *Client) accountUIDCtx(ctx context.Context) (int64, error) {
	uid, _, err := c.AccountState(ctx)
	return uid, err
}

// ProbeLossless checks whether a track has a lossless source. It does
// not download audio; only the signed get-file-info answer is read.
func (c *Client) ProbeLossless(ctx context.Context, trackID string) (Probe, error) {
	if c.probeFn != nil {
		return c.probeFn(ctx, trackID)
	}
	uid, _, err := c.AccountState(ctx)
	if err != nil {
		return Probe{}, err
	}

	var raw json.RawMessage
	if err := c.doJSONWithUID(ctx, fileInfoURLAt(c.baseURL(), trackID, time.Now().Unix()), &raw, uid); err != nil {
		return Probe{}, classifyStatus(err)
	}
	info, err := parseLosslessInfo(raw)
	if err != nil {
		if errors.Is(err, ErrNoFLAC) {
			return Probe{}, fmt.Errorf("%w: empty download info", ErrCheckFailed)
		}
		return Probe{}, fmt.Errorf("%w: %v", ErrCheckFailed, err)
	}
	quality := strings.ToLower(strings.TrimSpace(info.Quality))
	if quality == "smart_preview" || quality == "preview" {
		return Probe{}, fmt.Errorf("%w: %s", ErrPreview, info.Quality)
	}
	if strings.TrimSpace(info.Codec) == "" {
		return Probe{}, fmt.Errorf("%w: empty codec", ErrCheckFailed)
	}
	return Probe{
		Available: isFLACCodec(info.Codec),
		Codec:     info.Codec,
		Quality:   info.Quality,
		Bitrate:   info.Bitrate,
	}, nil
}
