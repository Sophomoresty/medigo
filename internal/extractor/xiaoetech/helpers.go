package xiaoetech

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

func fetchCourseList(c *util.Client, jar http.CookieJar) ([]xetItem, error) {
	out := []xetItem{}
	seen := map[string]bool{}
	for _, spec := range []struct {
		tpl   string
		extra string
	}{{courseListURL, ""}, {courseListURL, "&resource_type=12"}, {courseListURL, "&resource_type=51"}, {quanziListURL, ""}} {
		for page := 1; page <= 32; page++ {
			body, err := c.GetString(fmt.Sprintf(spec.tpl, page)+spec.extra, headers(jar, refererURL))
			if err != nil {
				if len(out) > 0 {
					return out, nil
				}
				return nil, err
			}
			var root map[string]any
			if err := json.Unmarshal([]byte(body), &root); err != nil {
				return nil, fmt.Errorf("xiaoetech parse course list: %w", err)
			}
			list := listUnder(root["data"], "list")
			if len(list) == 0 {
				break
			}
			for _, m := range list {
				it := itemFromMap(m)
				if it.id == "" || seen[it.id] || val(m, "is_available") == "0" {
					continue
				}
				seen[it.id] = true
				out = append(out, it)
			}
			if len(list) < pageSize {
				break
			}
		}
	}
	if body, err := c.GetString(fmt.Sprintf(livingLiveListURL, pageSize, url.QueryEscape("1-0-0")), headers(jar, refererURL)); err == nil {
		var root map[string]any
		if json.Unmarshal([]byte(body), &root) == nil {
			for _, m := range listUnder(root["data"], "list") {
				it := itemFromMap(m)
				it.typ = "live"
				if it.id != "" && !seen[it.id] {
					seen[it.id] = true
					out = append(out, it)
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("xiaoetech attend list is empty")
	}
	return out, nil
}

func resolveItem(c *util.Client, jar http.CookieJar, ctx xetCtx, it xetItem) (string, map[string]any) {
	if u := firstMediaURL(it.raw); u != "" {
		return u, map[string]any{"resource_id": it.id, "resource_type": it.typ, "app_id": ctx.appID}
	}
	switch normType(firstNonEmpty(ctx.typ, it.typ)) {
	case "live":
		if u := liveMediaURL(c, jar, ctx, it.id); u != "" {
			return u, map[string]any{"resource_id": it.id, "resource_type": "live", "api": "lookback"}
		}
	case "audio":
		if u := postDetailURL(c, jar, ctx, it.id, audioURL, pcAudioURL, map[string]string{"bizData[resource_id]": it.id}); u != "" {
			return u, map[string]any{"resource_id": it.id, "resource_type": "audio", "api": audioURL}
		}
	case "text", "book", "document", "column", "bigcolumn", "member", "ecourse", "train":
		if u := postDetailURL(c, jar, ctx, it.id, infoURL, "", map[string]string{"bizData[resource_id]": it.id}); u != "" {
			return u, map[string]any{"resource_id": it.id, "resource_type": it.typ, "api": infoURL}
		}
	}
	return videoMediaURL(c, jar, ctx, it.id), map[string]any{"resource_id": it.id, "resource_type": "video", "api": sourceURL}
}

func videoMediaURL(c *util.Client, jar http.CookieJar, ctx xetCtx, vid string) string {
	h := headers(jar, referer(ctx))
	api := fmt.Sprintf(sourceURL, ctx.appID, firstNonEmpty(ctx.xetDomain, xetDomainDefault))
	form := map[string]string{"bizData[opr_sys]": "Win32", "bizData[product_id]": firstNonEmpty(ctx.cid, vid), "bizData[resource_id]": vid}
	if ctx.pc && ctx.domain != "" {
		api = fmt.Sprintf(pcSourceURL, ctx.domain)
		form = map[string]string{"opr_sys": "Win32", "product_id": firstNonEmpty(ctx.cid, vid), "resource_id": vid}
	}
	body, err := c.PostForm(api, form, h)
	if err != nil {
		return ""
	}
	var root map[string]any
	if json.Unmarshal([]byte(body), &root) != nil {
		return ""
	}
	if u := firstMediaURL(root["data"]); u != "" {
		return u
	}
	if s := deepText(root, "video_urls", "play_urls", "url"); s != "" {
		if u := firstURLInString(s); u != "" {
			return u
		}
	}
	if ps := deepText(root, "play_sign", "playSign"); ps != "" && ctx.appID != "" {
		if u := postDetailURL(c, jar, ctx, vid, videoPlayURL, "", map[string]string{"play_sign": ps}); u != "" {
			return u
		}
	}
	return ""
}

func liveMediaURL(c *util.Client, jar http.CookieJar, ctx xetCtx, id string) string {
	h := headers(jar, referer(ctx))
	urls := []string{}
	if ctx.appID != "" {
		urls = append(urls, fmt.Sprintf(protectedLiveURL, ctx.appID, firstNonEmpty(ctx.xetDomain, xetDomainDefault), ctx.appID, id), fmt.Sprintf(liveURL, ctx.appID, firstNonEmpty(ctx.xetDomain, xetDomainDefault), ctx.appID, id))
	}
	if ctx.pc && ctx.domain != "" {
		urls = append(urls, fmt.Sprintf(pcLiveURL, ctx.domain, ctx.appID))
	}
	for _, api := range urls {
		if body, err := c.GetString(api, h); err == nil {
			var root map[string]any
			if json.Unmarshal([]byte(body), &root) == nil {
				if u := firstMediaURL(root["data"]); u != "" {
					return u
				}
			}
		}
	}
	return ""
}

func postDetailURL(c *util.Client, jar http.CookieJar, ctx xetCtx, id, h5Tpl, pcTpl string, form map[string]string) string {
	api := ""
	if ctx.pc && pcTpl != "" && ctx.domain != "" {
		api = fmt.Sprintf(pcTpl, ctx.domain)
	} else if ctx.appID != "" {
		api = fmt.Sprintf(h5Tpl, ctx.appID, firstNonEmpty(ctx.xetDomain, xetDomainDefault))
	}
	if api == "" {
		return ""
	}
	body, err := c.PostForm(api, form, headers(jar, referer(ctx)))
	if err != nil {
		return ""
	}
	var root map[string]any
	if json.Unmarshal([]byte(body), &root) != nil {
		return ""
	}
	if u := firstMediaURL(root["data"]); u != "" {
		return u
	}
	return firstURLInString(body)
}

func itemFromMap(m map[string]any) xetItem {
	u := firstNonEmpty(val(m, "h5_url"), val(m, "url"), val(m, "live_share_url"))
	p := parseCtx(u)
	typ := normType(firstNonEmpty(val(m, "resource_type"), val(m, "course_type"), p.typ))
	return xetItem{id: firstNonEmpty(val(m, "resource_id"), val(m, "cid"), val(m, "id"), p.cid), title: firstNonEmpty(val(m, "title"), val(m, "resource_title"), val(m, "name")), typ: typ, appID: firstNonEmpty(val(m, "app_id"), p.appID), userID: firstNonEmpty(val(m, "user_id"), p.userID), pageURL: u, raw: m}
}
func headers(jar http.CookieJar, ref string) map[string]string {
	h := map[string]string{"Accept": "application/json, text/plain, */*", "Referer": firstNonEmpty(ref, refererURL), "Origin": refererURL, "X-Requested-With": "XMLHttpRequest"}
	if ck := cookieHeader(jar); ck != "" {
		h["Cookie"] = ck
	}
	return h
}
func cookieHeader(jar http.CookieJar) string {
	parts := []string{}
	for _, raw := range []string{refererURL, "https://study.xiaoe-tech.com", "https://www.xiaoeknow.com"} {
		if u, err := url.Parse(raw); err == nil {
			for _, c := range jar.Cookies(u) {
				parts = append(parts, c.Name+"="+c.Value)
			}
		}
	}
	return strings.Join(parts, "; ")
}
func referer(c xetCtx) string {
	if c.referer != "" {
		return c.referer
	}
	if c.pc && c.domain != "" {
		return "https://" + c.domain
	}
	if c.appID != "" {
		return fmt.Sprintf(courseURL, c.appID, firstNonEmpty(c.xetDomain, xetDomainDefault), firstNonEmpty(c.typ, "video"), firstNonEmpty(c.cid, ""))
	}
	return refererURL
}
func normType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if v := map[string]string{"1": "text", "2": "audio", "3": "video", "4": "live", "5": "member", "6": "column", "7": "column", "8": "bigcolumn", "10": "live", "12": "live", "20": "book", "25": "train", "50": "ecourse", "51": "document", "64": "ecourse", "alive": "live", "ebook": "book"}[t]; v != "" {
		return v
	}
	return t
}
func firstMediaURL(v any) string {
	for _, m := range mapsUnder(v) {
		for _, k := range []string{"video_m3u8_url", "video_hls", "video_url", "audio_m3u8_url", "audio_url", "video_audio_url", "aliveVideoUrl", "alive_video_url", "aliveVideoMp4Url", "aliveVideoUrlEncrypt", "miniAliveVideoUrl", "aliveReviewUrl", "file_url", "url", "m3u8_url", "play_url", "PlayURL"} {
			if u := normalizeURL(val(m, k)); isMediaURL(u) {
				return u
			}
		}
	}
	return ""
}
func firstURLInString(s string) string {
	if m := httpRe.FindString(s); m != "" {
		return normalizeURL(m)
	}
	return ""
}
func isMediaURL(u string) bool {
	return (strings.HasPrefix(u, "http") || strings.HasPrefix(u, "//")) && !regexp.MustCompile(`(?i)\.(?:jpg|jpeg|png|gif|webp)(?:[?#]|$)`).MatchString(u)
}
func media(title, u string, extra map[string]any) *extractor.MediaInfo {
	if title == "" {
		title = "xiaoetech_video"
	}
	return &extractor.MediaInfo{Site: "xiaoetech", Title: title, Streams: map[string]extractor.Stream{"default": {Quality: "source", URLs: []string{u}, Format: formatOf(u), Headers: map[string]string{"Referer": refererURL}}}, Extra: extra}
}
func listUnder(v any, key string) []map[string]any {
	for _, m := range mapsUnder(v) {
		if a, ok := m[key].([]any); ok {
			out := []map[string]any{}
			for _, x := range a {
				if mm, ok := x.(map[string]any); ok {
					out = append(out, mm)
				}
			}
			return out
		}
	}
	return nil
}
func mapsUnder(v any) []map[string]any {
	out := []map[string]any{}
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			out = append(out, t)
			for _, vv := range t {
				walk(vv)
			}
		case []any:
			for _, vv := range t {
				walk(vv)
			}
		}
	}
	walk(v)
	return out
}
func val(v any, k string) string {
	if m, ok := v.(map[string]any); ok {
		if x, ok := m[k]; ok && x != nil {
			return strings.TrimSpace(fmt.Sprint(x))
		}
	}
	return ""
}
func deepText(v any, keys ...string) string {
	for _, m := range mapsUnder(v) {
		for _, k := range keys {
			if s := val(m, k); s != "" {
				return s
			}
		}
	}
	return ""
}
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" && strings.TrimSpace(v) != "<nil>" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func firstRegex(pat, s string) string {
	m := regexp.MustCompile(pat).FindStringSubmatch(s)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
func normalizeURL(u string) string {
	u = strings.TrimSpace(strings.ReplaceAll(u, `\/`, "/"))
	u = strings.ReplaceAll(u, `\u002F`, "/")
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}
func formatOf(u string) string {
	l := strings.ToLower(u)
	if strings.Contains(l, ".m3u8") {
		return "m3u8"
	}
	if strings.Contains(l, ".mp3") {
		return "mp3"
	}
	return "mp4"
}
