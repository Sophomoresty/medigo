// Package dingtalk implements an extractor for n.dingtalk.com / h5.dingtalk.com
// live replay shares and alidocs.dingtalk.com document previews.
//
// API endpoints used (ported from Dingtalk_Video.pyc / Dingtalk_Live_Client.pyc):
//   - https://n.dingtalk.com/dingding/live-room/index.html?roomId={cid}&liveUuid={lid}
//   - https://h5.dingtalk.com/group-live-share/index.htm?encCid={encCid}&liveUuid={lid}
//   - https://alidocs.dingtalk.com/nt/api/docs/preset            (document metadata)
//   - https://alidocs.dingtalk.com/nt/api/docs/preset/binary     (preview binary)
//
// The real Python source authenticates with an LWP WebSocket client (LwpClient)
// using a session-bound lwp_token derived from cookies and calls multiple gRPC-
// style RPCs to resolve playback URLs. This is significant work outside the
// scope of a Go port: the extractor parses the live-room/group-live-share URL
// shapes, surfaces alidocs preview info for static document URLs, but returns a
// clear blocked error for live replay since the LWP handshake isn't reimplemented.
package dingtalk

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/nichuanfang/medigo/internal/extractor"
	"github.com/nichuanfang/medigo/internal/util"
)

var patterns = []string{
	`(?:[\w-]+\.)*dingtalk\.com/(?:dingding/live-room|group-live-share|nt/api)`,
	`alidocs\.dingtalk\.com/`,
}

const (
	liveRoomURL       = "https://n.dingtalk.com/dingding/live-room/index.html"
	groupLiveShareURL = "https://h5.dingtalk.com/group-live-share/index.htm"
	alidocsPresetURL  = "https://alidocs.dingtalk.com/nt/api/docs/preset"
)

func init() {
	extractor.Register(&DingTalk{}, extractor.SiteInfo{
		Name:     "DingTalk",
		URL:      "dingtalk.com",
		NeedAuth: true,
	})
}

type DingTalk struct{}

func (d *DingTalk) Patterns() []string { return patterns }

func (d *DingTalk) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("dingtalk requires login cookies (use --cookies or --cookies-from-browser)")
	}

	if dentryKey := extractDentry(rawURL); dentryKey != "" {
		return previewDoc(opts, dentryKey)
	}

	roomID, encCid, liveUUID := extractLiveIDs(rawURL)
	if liveUUID == "" || (roomID == "" && encCid == "") {
		return nil, fmt.Errorf("cannot parse dingtalk URL — expected live-room or group-live-share format: %s", rawURL)
	}

	return nil, fmt.Errorf("dingtalk live replay (%s/%s liveUuid=%s) requires the LWP WebSocket client (probe_public_live_share / probe_live_replay); Go port not implemented",
		coalesce(roomID, encCid), liveRoomURL, liveUUID)
}

// previewDoc calls alidocs.dingtalk.com/nt/api/docs/preset to surface document
// metadata (filename, mimeType, content URL). This is the simplest of the four
// dingtalk endpoints and works without LWP because alidocs uses standard
// cookie auth.
func previewDoc(opts *extractor.ExtractOpts, dentryKey string) (*extractor.MediaInfo, error) {
	c := util.NewClient()
	c.SetCookieJar(opts.Cookies)

	body, err := c.PostForm(alidocsPresetURL, map[string]string{"dentryKey": dentryKey}, map[string]string{
		"Referer": "https://alidocs.dingtalk.com/",
	})
	if err != nil {
		return nil, fmt.Errorf("alidocs preset: %w", err)
	}
	var preset struct {
		Data struct {
			Name        string `json:"name"`
			DownloadURL string `json:"downloadUrl"`
			PreviewURL  string `json:"previewUrl"`
			MimeType    string `json:"mimeType"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &preset); err != nil {
		return nil, fmt.Errorf("parse preset: %w", err)
	}
	url := preset.Data.DownloadURL
	if url == "" {
		url = preset.Data.PreviewURL
	}
	if url == "" {
		return nil, fmt.Errorf("alidocs preset returned no downloadUrl/previewUrl")
	}
	title := preset.Data.Name
	if title == "" {
		title = "dingtalk_doc_" + dentryKey
	}
	return &extractor.MediaInfo{
		Site:  "dingtalk",
		Title: title,
		Streams: map[string]extractor.Stream{
			"default": {
				Quality: "best",
				URLs:    []string{url},
				Format:  "binary",
				Headers: map[string]string{"Referer": "https://alidocs.dingtalk.com/"},
			},
		},
	}, nil
}

var (
	liveRoomRe   = regexp.MustCompile(`live-room/[^?]*\?(?:[^&]*&)*?roomId=([^&]+)(?:&[^&]*?liveUuid=([^&]+))?`)
	groupShareRe = regexp.MustCompile(`group-live-share/[^?]*\?(?:[^&]*&)*?encCid=([^&]+)(?:&[^&]*?liveUuid=([^&]+))?`)
	liveUUIDRe   = regexp.MustCompile(`liveUuid=([^&]+)`)
	dentryRe     = regexp.MustCompile(`(?:dentryKey|dentryUuid)=([^&\s]+)`)
)

func extractLiveIDs(u string) (roomID, encCid, liveUUID string) {
	if m := liveRoomRe.FindStringSubmatch(u); len(m) > 1 {
		roomID = m[1]
		if len(m) > 2 {
			liveUUID = m[2]
		}
	}
	if m := groupShareRe.FindStringSubmatch(u); len(m) > 1 {
		encCid = m[1]
		if len(m) > 2 && liveUUID == "" {
			liveUUID = m[2]
		}
	}
	if liveUUID == "" {
		if m := liveUUIDRe.FindStringSubmatch(u); len(m) > 1 {
			liveUUID = m[1]
		}
	}
	return roomID, encCid, liveUUID
}

func extractDentry(u string) string {
	if m := dentryRe.FindStringSubmatch(u); len(m) > 1 {
		return m[1]
	}
	return ""
}

func coalesce(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}
