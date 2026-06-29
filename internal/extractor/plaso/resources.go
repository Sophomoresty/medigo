package plaso

import (
	"net/url"
	"path"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
)

func (s *plasoSession) resolveNativeStaticMedia(rawURL string) *extractor.MediaInfo {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil
	}
	for _, r := range plasoNativeStaticResources() {
		if matchPlasoStaticResource(u, r) {
			return s.buildStaticResourceEntry(r)
		}
	}
	return nil
}

func (s *plasoSession) resolveDirectResourceMedia(rawURL string) *extractor.MediaInfo {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Hostname() == "" || !isPlasoDirectResourceHost(u.Hostname()) {
		return nil
	}
	raw := u.String()
	format := formatOf(raw, "")
	if format == "bin" && !looksDownloadable(raw) {
		return nil
	}
	title := clean(path.Base(u.EscapedPath()))
	if title == "" || title == "." || title == "/" {
		title = clean(u.Hostname())
	}
	extra := map[string]any{
		"source_type":   "direct_resource",
		"resource_host": strings.ToLower(u.Hostname()),
		"resource_url":  raw,
	}
	stream := extractor.Stream{
		Quality: "resource",
		URLs:    []string{raw},
		Format:  format,
		Headers: streamHeaders(s.headers),
		Extra:   cloneAnyMap(extra),
	}
	return &extractor.MediaInfo{Site: "plaso", Title: title, Streams: map[string]extractor.Stream{"best": stream}, Extra: extra}
}

func isPlasoDirectResourceHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	switch host {
	case "file.plaso.cn", "filecdn.plaso.cn", "filecdn.plaso.com":
		return true
	}
	return host == "ppt-player.plaso.com" ||
		(strings.HasPrefix(host, "ppt-player-") && strings.HasSuffix(host, ".plaso.com"))
}

func plasoNativeStaticResources() []plasoStaticResource {
	return []plasoStaticResource{
		{URL: "https://wwwr.plaso.cn/static/yxt/", Path: "index.html", Host: "wwwr.plaso.cn", Entry: true, Required: true},
		{URL: "https://wwwr.plaso.cn/static/ppt1.4/?appType=nppt", Path: "static/ppt1.4/index.html", Host: "wwwr.plaso.cn", Entry: true, Required: true},
		{URL: "https://www.plaso.cn/static/ispring/?static=1", Path: "static/ispring/index.html", Host: "www.plaso.cn", Entry: true, Required: true},
		{URL: "https://ppt-player-wwwr.plaso.com/static/ispring/Scripts/player.js?static=1&v=202304150933", Path: "static/ispring/Scripts/player.js", Host: "ppt-player-wwwr.plaso.com", Entry: true, Required: true},
	}
}

func matchPlasoStaticResource(u *url.URL, r plasoStaticResource) bool {
	if u == nil || strings.TrimSpace(r.URL) == "" {
		return false
	}
	ru, err := url.Parse(r.URL)
	if err != nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(u.Hostname()), strings.TrimSpace(ru.Hostname())) {
		return false
	}
	if strings.Trim(strings.TrimSpace(u.EscapedPath()), "/") != strings.Trim(strings.TrimSpace(ru.EscapedPath()), "/") {
		return false
	}
	wantedQuery := ru.Query()
	if len(wantedQuery) == 0 {
		return true
	}
	gotQuery := u.Query()
	for key, vals := range wantedQuery {
		if len(vals) == 0 {
			continue
		}
		gotVals := gotQuery[key]
		if len(gotVals) < len(vals) {
			return false
		}
		remaining := append([]string(nil), gotVals...)
		for _, want := range vals {
			matched := false
			for i, got := range remaining {
				if got == want {
					remaining = append(remaining[:i], remaining[i+1:]...)
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}

func (s *plasoSession) buildStaticResourceEntry(r plasoStaticResource) *extractor.MediaInfo {
	if strings.TrimSpace(r.URL) == "" || strings.TrimSpace(r.Path) == "" {
		return nil
	}
	title := clean(strings.TrimSuffix(strings.TrimPrefix(r.Path, "static/"), "/"))
	if title == "" {
		title = clean(r.Path)
	}
	stream := extractor.Stream{
		Quality: "static",
		URLs:    []string{r.URL},
		Format:  formatOf(r.Path, ""),
		Headers: streamHeaders(s.headers),
		Extra: map[string]any{
			"static_url":      r.URL,
			"static_path":     r.Path,
			"static_host":     r.Host,
			"static_entry":    r.Entry,
			"static_required": r.Required,
		},
	}
	return &extractor.MediaInfo{
		Site:  "plaso",
		Title: title,
		Streams: map[string]extractor.Stream{
			"best": stream,
		},
		Extra: map[string]any{
			"source_type":     "native_static_entry",
			"static_url":      r.URL,
			"static_path":     r.Path,
			"static_host":     r.Host,
			"static_entry":    r.Entry,
			"static_required": r.Required,
		},
	}
}
