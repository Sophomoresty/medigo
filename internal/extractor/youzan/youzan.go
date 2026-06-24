// Package youzan implements an extractor for youzan.com knowledge shops.
package youzan

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/nichuanfang/medigo/internal/extractor"
	"github.com/nichuanfang/medigo/internal/util"
)

const (
	refererURL        = "https://www.youzan.com"
	goodsURL          = "/wscvis/course/detail/goods.json"
	columnChaptersURL = "/wscvis/knowledge/getColumnChapters.json"
	columnContentsURL = "/wscvis/knowledge/contentAndLive.json"
	simpleURL         = "/wscvis/course/getSimple.json"
	liveLinkURL       = "/wscvis/knowledge/getLiveLink.json"
	eduLiveLinkURL    = "/wscvis/course/live/video/getEduLiveLink.json"
	roomURL           = "/wscvis/course/live/video/room"
	assetStateURL     = "/wscvis/course/detail/getAssetStateV2.json"
)

var (
	patterns     = []string{`(?:[\w-]+\.)?youzan\.com/`}
	mediaRe      = regexp.MustCompile(`https?://[^"'\s<>]+(?:\.m3u8|\.mp4|\.mp3|\.m4a|\.aac|\.wav)[^"'\s<>]*`)
	htmlRe       = regexp.MustCompile(`<[^>]+>`)
	titleCleanRe = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`)
)

func init() {
	extractor.Register(&Youzan{}, extractor.SiteInfo{Name: "Youzan", URL: "youzan.com", NeedAuth: true})
}

type Youzan struct{}

func (y *Youzan) Patterns() []string { return patterns }

type yzContext struct {
	c        *util.Client
	shopHost string
	shopBase string
	kdtID    string
	alias    string
	column   string
	headers  map[string]string
}

type yzMedia struct{ Title, URL, ContentType string }

func (y *Youzan) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("youzan requires login cookies")
	}
	ctx := &yzContext{c: util.NewClient()}
	ctx.c.SetCookieJar(opts.Cookies)
	if err := ctx.configure(rawURL); err != nil {
		return nil, err
	}
	ctx.headers = ctx.buildHeaders(opts.Cookies, ctx.detailURL(ctx.alias))
	goods, err := ctx.requestJSON(goodsURL, map[string]string{"alias": ctx.alias, "kdtId": ctx.kdtID}, ctx.detailURL(ctx.alias))
	if err != nil {
		return nil, err
	}
	title := firstNonEmpty(resolveTitle(goods), ctx.alias)
	media := ctx.buildMediaEntries(goods, ctx.alias, title)
	if len(media) == 0 {
		return nil, fmt.Errorf("youzan: no media URLs found for alias %s", ctx.alias)
	}
	entries := make([]*extractor.MediaInfo, 0, len(media))
	for i, m := range media {
		name := cleanTitle(firstNonEmpty(m.Title, fmt.Sprintf("[%02d]--%s", i+1, title)))
		entries = append(entries, &extractor.MediaInfo{Site: "youzan", Title: name, Streams: map[string]extractor.Stream{"default": {Quality: "source", URLs: []string{m.URL}, Format: pickFormat(m.URL, m.ContentType), Headers: map[string]string{"Referer": ctx.detailURL(ctx.alias)}}}})
	}
	return &extractor.MediaInfo{Site: "youzan", Title: cleanTitle(title), Entries: entries}, nil
}

func (x *yzContext) configure(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("youzan: invalid URL")
	}
	x.shopHost = u.Host
	x.shopBase = u.Scheme + "://" + u.Host
	q := u.Query()
	if strings.Contains(u.Fragment, "?") {
		if fq, e := url.ParseQuery(strings.SplitN(u.Fragment, "?", 2)[1]); e == nil {
			for k, vs := range fq {
				for _, v := range vs {
					q.Add(k, v)
				}
			}
		}
	}
	x.alias = firstNonEmpty(q.Get("alias"), q.Get("courseAlias"), q.Get("goodsAlias"), q.Get("contentAlias"))
	x.column = firstNonEmpty(q.Get("columnAlias"), q.Get("fromColumn"))
	x.kdtID = firstNonEmpty(q.Get("kdt_id"), q.Get("kdtId"))
	if x.alias == "" && strings.Contains(u.Path, "/wscvis/course/detail/") {
		x.alias = strings.Trim(strings.TrimPrefix(u.Path, "/wscvis/course/detail/"), "/")
	}
	if x.alias == "" && strings.Contains(u.Path, "/wscvis/knowledge/") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		x.alias = parts[len(parts)-1]
	}
	if x.alias == "" {
		return fmt.Errorf("youzan: cannot parse alias from URL")
	}
	return nil
}

func (x *yzContext) detailURL(alias string) string {
	if x.shopBase == "" {
		return refererURL
	}
	p := "/wscvis/course/detail/" + alias
	return x.apiURL(p, map[string]string{"alias": alias, "kdt_id": x.kdtID})
}

func (x *yzContext) buildHeaders(jar http.CookieJar, referer string) map[string]string {
	h := map[string]string{"accept": "application/json, text/plain, */*", "referer": referer, "x-requested-with": "XMLHttpRequest", "origin": x.shopBase}
	var parts []string
	for _, raw := range []string{x.shopBase + "/", refererURL} {
		u, _ := url.Parse(raw)
		for _, ck := range jar.Cookies(u) {
			parts = append(parts, ck.Name+"="+ck.Value)
			if ck.Name == "_kdt_id_" && x.kdtID == "" {
				x.kdtID = ck.Value
			}
		}
	}
	if x.kdtID != "" {
		parts = append(parts, "_kdt_id_="+x.kdtID)
	}
	if len(parts) > 0 {
		h["cookie"] = uniqueCookie(parts)
	}
	return h
}

func (x *yzContext) apiURL(path string, params map[string]string) string {
	if strings.HasPrefix(path, "http") {
		return path
	}
	u, _ := url.Parse(strings.TrimRight(x.shopBase, "/") + path)
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	if x.kdtID != "" {
		q.Set("kdt_id", x.kdtID)
		q.Set("kdtId", x.kdtID)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (x *yzContext) requestText(path string, params map[string]string, referer string) (string, error) {
	return x.c.GetString(x.apiURL(path, params), x.headersWithReferer(referer))
}

func (x *yzContext) requestJSON(path string, params map[string]string, referer string) (map[string]any, error) {
	body, err := x.requestText(path, params, referer)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (x *yzContext) headersWithReferer(referer string) map[string]string {
	h := map[string]string{}
	for k, v := range x.headers {
		h[k] = v
	}
	if referer != "" {
		h["referer"] = referer
	}
	return h
}

func (x *yzContext) buildMediaEntries(goods map[string]any, alias, title string) []yzMedia {
	seen := map[string]bool{}
	var out []yzMedia
	add := func(urls []string, label string) {
		for _, u := range urls {
			if u != "" && !seen[u] {
				seen[u] = true
				out = append(out, yzMedia{Title: label, URL: u})
			}
		}
	}
	add(extractMediaURLs(goods), title)
	if state, err := x.requestJSON(assetStateURL, map[string]string{"aliasList": alias, "alias": alias, "t_vis_get": fmt.Sprint(1000)}, x.detailURL(alias)); err == nil {
		add(extractMediaURLs(state), title)
	}
	for _, path := range []string{liveLinkURL, eduLiveLinkURL, simpleURL} {
		if data, err := x.requestJSON(path, map[string]string{"alias": alias}, x.detailURL(alias)); err == nil {
			add(extractMediaURLs(data), title)
		}
	}
	if text, err := x.requestText("/wscvis/course/detail/"+alias, map[string]string{"alias": alias}, x.detailURL(alias)); err == nil {
		add(mediaRe.FindAllString(text, -1), title)
	}
	if text, err := x.requestText(roomURL, map[string]string{"alias": alias}, x.detailURL(alias)); err == nil {
		add(mediaRe.FindAllString(text, -1), title)
	}
	return out
}

func resolveTitle(data map[string]any) string {
	for _, node := range walkMaps(data) {
		if t := firstString(node, "title", "name", "alias"); t != "" {
			return htmlRe.ReplaceAllString(t, "")
		}
	}
	return ""
}

func extractMediaURLs(v any) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			for _, m := range mediaRe.FindAllString(strings.ReplaceAll(t, `\/`, `/`), -1) {
				if !seen[m] {
					seen[m] = true
					out = append(out, m)
				}
			}
		case []any:
			for _, it := range t {
				walk(it)
			}
		case map[string]any:
			for k, v := range t {
				if strings.Contains(strings.ToLower(k), "url") || strings.Contains(strings.ToLower(k), "source") {
					walk(v)
				}
			}
			for _, v := range t {
				walk(v)
			}
		}
	}
	walk(v)
	return out
}

func walkMaps(v any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			out = append(out, t)
			for _, v := range t {
				walk(v)
			}
		case []any:
			for _, v := range t {
				walk(v)
			}
		}
	}
	walk(v)
	return out
}

func uniqueCookie(parts []string) string {
	seen := map[string]string{}
	order := []string{}
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			if _, ok := seen[kv[0]]; !ok {
				order = append(order, kv[0])
			}
			seen[kv[0]] = kv[1]
		}
	}
	out := []string{}
	for _, k := range order {
		out = append(out, k+"="+seen[k])
	}
	return strings.Join(out, "; ")
}
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := strings.TrimSpace(fmt.Sprint(m[k])); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func cleanTitle(s string) string { return titleCleanRe.ReplaceAllString(strings.TrimSpace(s), "_") }
func pickFormat(u, ct string) string {
	low := strings.ToLower(u + " " + ct)
	if strings.Contains(low, ".m3u8") {
		return "m3u8"
	}
	if strings.Contains(low, ".mp3") {
		return "mp3"
	}
	if strings.Contains(low, ".m4a") {
		return "m4a"
	}
	return "mp4"
}
