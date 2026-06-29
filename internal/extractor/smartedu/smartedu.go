// Package smartedu implements source-aligned Smartedu static JSON extraction.
package smartedu

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	refererURL  = "https://basic.smartedu.cn/"
	loginURL    = "https://auth.smartedu.cn/uias/login"
	staticBase0 = "https://bdcs-file-2.ykt.cbern.com.cn/zxx_secondary"
	staticBase1 = "https://bdcs-file-1.ykt.cbern.com.cn/zxx_secondary"
	special0    = "https://bdcs-file-2.ykt.cbern.com.cn/zxx"
	special1    = "https://bdcs-file-1.ykt.cbern.com.cn/zxx"
	special2    = "https://s-file-2.ykt.cbern.com.cn/zxx"
	special3    = "https://s-file-1.ykt.cbern.com.cn/zxx"

	nationalResourceDetailURL            = "%s/ndrv2/national_lesson/resources/details/%s.json"
	nationalRelationResourceURL          = "%s/ndrs/national_lesson/resources/%s/relation_resource.json"
	nationalTeachingmaterialDetailURL    = "%s/ndrs/national_lesson/teachingmaterials/details/%s.json"
	nationalTeachingmaterialPartsURL     = "%s/ndrs/national_lesson/teachingmaterials/version/data_version.json"
	nationalTeachingmaterialResourcesURL = "%s/ndrs/national_lesson/teachingmaterials/%s/resources/parts.json"
	nationalTreeURL                      = "%s/ndrv2/national_lesson/trees/%s.json"
	prepareResourceDetailURL             = "%s/ndrv2/prepare_lesson/resources/details/%s.json"
	prepareSubTypeResourceDetailURL      = "%s/ndrv2/prepare_sub_type/resources/details/%s.json"
	prepareRelationResourceURL           = "%s/ndrs/prepare_lesson/resources/%s/relation_resource.json"
	prepareTeachingmaterialDetailURL     = "%s/ndrs/prepare_lesson/teachingmaterials/details/%s.json"
	prepareTeachingmaterialPartsURL      = "%s/ndrs/prepare_lesson/teachingmaterials/parts.json"
	prepareTeachingmaterialResourcesURL  = "%s/ndrs/prepare_lesson/teachingmaterials/%s/resources/parts.json"
	prepareTreeURL                       = "%s/ndrs/prepare_lesson/trees/%s.json"
	tchMaterialDetailURL                 = "%s/ndrv2/resources/tch_material/details/%s.json"
	tchMaterialContentURL                = "%s/api_static/contents/%s.json"
	tchMaterialThematicDetailURL         = "%s/ndrs/special_edu/resources/details/%s.json"
	tchMaterialThematicTreeURL           = "%s/ndrs/special_edu/thematic_course/trees/%s.json"
	tchMaterialThematicResourcesURL      = "%s/ndrs/special_edu/thematic_course/%s/resources/list.json"
)

var (
	privateHosts = []string{
		"https://r1-ndr-private.ykt.cbern.com.cn",
		"https://r2-ndr-private.ykt.cbern.com.cn",
		"https://r3-ndr-private.ykt.cbern.com.cn",
	}
	publicHosts = []string{
		"https://r1-ndr.ykt.cbern.com.cn",
		"https://r2-ndr.ykt.cbern.com.cn",
		"https://r3-ndr.ykt.cbern.com.cn",
	}
	overseaHosts = []string{
		"https://r1-ndr-oversea.ykt.cbern.com.cn",
		"https://r2-ndr-oversea.ykt.cbern.com.cn",
		"https://r3-ndr-oversea.ykt.cbern.com.cn",
	}
	privateHost = privateHosts[0]
)

var patterns = []string{`(?:[\w-]+\.)?smartedu\.cn/`}

func init() {
	extractor.Register(&Smartedu{}, extractor.SiteInfo{Name: "Smartedu", URL: "smartedu.cn", NeedAuth: true})
}

type Smartedu struct{}

func (s *Smartedu) Patterns() []string { return patterns }

type smCtx struct {
	c                         *util.Client
	headers                   map[string]string
	accessToken, refreshToken string
	macKey                    string
	diff                      int64
	lastVideoKeySign          string
}

type sourceItem struct {
	kind, url, fmt, name, title, id string
	urls                            []string
	headers                         map[string]string
	urlHeaders                      map[string]map[string]string
	size                            int64
	extra                           map[string]any
}

type smarteduAuth struct {
	accessToken  string
	refreshToken string
	macKey       string
	diff         int64
}

func (s *Smartedu) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil || opts.Cookies == nil {
		return nil, fmt.Errorf("smartedu requires login cookies")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	ctx := newCtx(opts.Cookies)
	q := u.Query()
	p := strings.TrimRight(u.Path, "/")
	var sources []sourceItem
	var chapters []extractor.Chapter
	extra := map[string]any{"price": 0, "purchased": true, "login_checked": ctx.macKey != ""}
	title := "smartedu"

	switch p {
	case "/tchMaterial/detail":
		extra["link_type"] = "tch_material"
		contentID := firstQuery(q, "contentId", "contentid")
		if contentID == "" {
			return nil, fmt.Errorf("smartedu: missing contentId")
		}
		contentType := firstQuery(q, "contentType", "contenttype")
		if contentType == "thematic_course" {
			resources, err := ctx.loadTchMaterialThematic(contentID)
			if err != nil {
				return nil, err
			}
			sources = ctx.extractSources(resources, true, false, contentType)
			title = "thematic_" + contentID
		} else {
			detail, err := ctx.getFirst(tplURLs(tchMaterialDetailURL, staticBases(), contentID))
			if err != nil {
				return nil, err
			}
			_, _ = ctx.getFirst(tplURLs(tchMaterialContentURL, staticBases(), contentID))
			sources = ctx.extractResources(detail, contentID, false, false, contentType)
			title = firstNonEmpty(globalTitle(detail), contentID)
		}
	case "/syncClassroom", "/syncClassroom/classActivity":
		activityID := firstQuery(q, "activityId", "activityid")
		fromPrepare := firstQuery(q, "fromPrepare", "fromprepare") == "1"
		extra["from_prepare"] = fromPrepare
		if activityID != "" {
			extra["link_type"] = "sync_classroom"
			extra["activity_id"] = activityID
			detail, err := ctx.loadActivity(activityID, fromPrepare)
			if err != nil {
				return nil, err
			}
			sources = ctx.extractResources(detail, activityID, true, fromPrepare, "")
			title = firstNonEmpty(globalTitle(detail), activityID)
		} else {
			extra["link_type"] = "sync_classroom_course"
			teachingID := firstQuery(q, "teachingmaterialId", "teachingmaterialid")
			if teachingID == "" {
				teachingID = ctx.findTeachingMaterialByTags(smarteduDefaultTags(q), fromPrepare)
			}
			if teachingID == "" {
				return nil, fmt.Errorf("smartedu: missing activityId or teachingmaterialId")
			}
			extra["teachingmaterial_id"] = teachingID
			resources, err := ctx.loadTeachingResourcesKind(teachingID, fromPrepare)
			if err != nil {
				return nil, err
			}
			chapters = chaptersFromSmarteduTree(ctx.loadTeachingTree(teachingID, fromPrepare))
			sources = ctx.extractSources(resources, true, fromPrepare, "")
			title = firstNonEmpty(ctx.loadTeachingTitle(teachingID, fromPrepare), teachingID)
		}
	default:
		extra["link_type"] = "resource"
		resourceID := firstQuery(q, "activityId", "activityid", "contentId", "contentid", "resourceId", "resourceid")
		if resourceID == "" {
			return nil, fmt.Errorf("smartedu: unsupported URL path %s", p)
		}
		detail, err := ctx.loadActivity(resourceID, false)
		if err != nil {
			return nil, err
		}
		sources = ctx.extractResources(detail, resourceID, true, false, "")
		title = firstNonEmpty(globalTitle(detail), resourceID)
	}
	info, err := mediaFromSources(title, sources)
	if err != nil {
		return nil, err
	}
	if len(chapters) > 0 {
		info.Chapters = chapters
	}
	if info.Extra == nil {
		info.Extra = map[string]any{}
	}
	for k, v := range extra {
		info.Extra[k] = v
	}
	return info, nil
}

func newCtx(jar http.CookieJar) *smCtx {
	c := util.NewClient()
	c.SetCookieJar(jar)
	cookie := cookieHeader(jar, []string{refererURL, loginURL, "https://www.smartedu.cn/"})
	h := map[string]string{"Origin": "https://basic.smartedu.cn", "Referer": refererURL, "Accept": "application/json,text/plain,*/*"}
	if cookie != "" {
		h["Cookie"] = cookie
	}
	auth := decodeSmarteduAuth(cookie)
	return &smCtx{c: c, headers: h, accessToken: auth.accessToken, refreshToken: auth.refreshToken, macKey: auth.macKey, diff: auth.diff}
}

func (x *smCtx) getFirst(urls []string) (map[string]any, error) {
	var last error
	for _, raw := range urls {
		body, err := x.c.GetString(raw, x.requestHeaders(raw, true))
		if err != nil {
			last = err
			continue
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			last = err
			continue
		}
		if len(v) > 0 {
			return v, nil
		}
	}
	if last != nil {
		return nil, last
	}
	return nil, fmt.Errorf("smartedu: empty JSON candidates")
}

func (x *smCtx) requestHeaders(raw string, auth bool) map[string]string {
	h := make(map[string]string, len(x.headers)+1)
	for k, v := range x.headers {
		h[k] = v
	}
	if auth {
		if a := x.authHeader(raw, "GET"); a != "" {
			h["X-ND-AUTH"] = a
		}
	}
	return h
}

func (x *smCtx) authHeader(raw, method string) string {
	if x.accessToken == "" || x.macKey == "" || raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	uri := u.EscapedPath()
	if uri == "" {
		uri = "/"
	}
	if u.RawQuery != "" {
		uri += "?" + u.RawQuery
	}
	nonce := fmt.Sprintf("%d:%s", time.Now().UnixMilli()+x.diff, util.RandomAlphanumeric(8))
	base := fmt.Sprintf("%s\n%s\n%s\n%s\n", nonce, strings.ToUpper(firstNonEmpty(method, "GET")), uri, u.Host)
	mac := hmac.New(sha256.New, []byte(x.macKey))
	_, _ = mac.Write([]byte(base))
	return fmt.Sprintf(`MAC id="%s",nonce="%s",mac="%s"`, x.accessToken, nonce, base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

func (x *smCtx) loadActivity(id string, prepare bool) (map[string]any, error) {
	var urls []string
	if prepare {
		urls = append(urls, tplURLs(prepareSubTypeResourceDetailURL, specialBases(), id)...)
		urls = append(urls, tplURLs(prepareResourceDetailURL, specialBases(), id)...)
	}
	urls = append(urls, tplURLs(nationalResourceDetailURL, staticBases(), id)...)
	return x.getFirst(urls)
}

func (x *smCtx) loadRelation(id string, prepare bool) []map[string]any {
	tpl, bases := nationalRelationResourceURL, staticBases()
	if prepare {
		tpl, bases = prepareRelationResourceURL, specialBases()
	}
	m, err := x.getFirst(tplURLs(tpl, bases, id))
	if err != nil {
		return nil
	}
	return collectResourceMaps(m)
}

func (x *smCtx) loadTeachingResources(id string) ([]map[string]any, error) {
	return x.loadTeachingResourcesKind(id, false)
}

func (x *smCtx) loadTeachingResourcesKind(id string, prepare bool) ([]map[string]any, error) {
	tpl, bases := nationalTeachingmaterialResourcesURL, staticBases()
	if prepare {
		tpl, bases = prepareTeachingmaterialResourcesURL, specialBases()
	}
	m, err := x.getFirst(tplURLs(tpl, bases, id))
	if err != nil {
		return nil, err
	}
	return collectResourceMaps(m), nil
}

func (x *smCtx) loadTeachingTitle(id string, prepare bool) string {
	tpl, bases := nationalTeachingmaterialDetailURL, staticBases()
	if prepare {
		tpl, bases = prepareTeachingmaterialDetailURL, specialBases()
	}
	m, err := x.getFirst(tplURLs(tpl, bases, id))
	if err != nil {
		return ""
	}
	return globalTitle(m)
}

func (x *smCtx) loadTeachingTree(id string, prepare bool) any {
	tpl, bases := nationalTreeURL, staticBases()
	if prepare {
		tpl, bases = prepareTreeURL, specialBases()
	}
	m, err := x.getFirst(tplURLs(tpl, bases, id))
	if err != nil {
		return nil
	}
	return m
}

func (x *smCtx) findTeachingMaterialByTags(tags []string, prepare bool) string {
	if len(tags) == 0 {
		return ""
	}
	tpl, bases := nationalTeachingmaterialPartsURL, staticBases()
	if prepare {
		tpl, bases = prepareTeachingmaterialPartsURL, specialBases()
	}
	urls := make([]string, 0, len(bases))
	for _, b := range bases {
		urls = append(urls, fmt.Sprintf(tpl, b))
	}
	m, err := x.getFirst(urls)
	if err != nil {
		return ""
	}
	want := map[string]bool{}
	for _, tag := range tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			want[tag] = true
		}
	}
	for _, item := range collectResourceMaps(m) {
		have := tagSet(item)
		ok := len(want) > 0
		for tag := range want {
			if !have[tag] {
				ok = false
				break
			}
		}
		if ok {
			return str(item["id"])
		}
	}
	return ""
}

func (x *smCtx) loadTchMaterialThematic(id string) ([]map[string]any, error) {
	_, _ = x.getFirst(tplURLs(tchMaterialThematicDetailURL, specialBases(), id))
	_, _ = x.getFirst(tplURLs(tchMaterialThematicTreeURL, specialBases(), id))
	m, err := x.getFirst(tplURLs(tchMaterialThematicResourcesURL, specialBases(), id))
	if err != nil {
		return nil, err
	}
	return collectResourceMaps(m), nil
}

func (x *smCtx) extractResources(detail map[string]any, id string, enrich bool, prepare bool, contentType string) []sourceItem {
	resources := relationResources(detail)
	if len(resources) == 0 {
		resources = []map[string]any{detail}
	}
	if len(resources) == 1 && id != "" {
		resources = append(resources, x.loadRelation(id, prepare)...)
	}
	return x.extractSources(resources, enrich, prepare, contentType)
}

func (x *smCtx) extractSources(resources []map[string]any, enrich bool, prepare bool, contentType string) []sourceItem {
	seen := map[string]bool{}
	var out []sourceItem
	for i, r := range resources {
		if enrich && len(items(r)) == 0 {
			if id := str(r["id"]); id != "" {
				if d, err := x.loadActivity(id, prepare); err == nil {
					r = d
				}
			}
		}
		if src := x.sourceFromResource(r, i+1, contentType); src.url != "" {
			key := src.url + "|" + src.id
			if !seen[key] {
				seen[key] = true
				out = append(out, src)
			}
		}
	}
	return out
}

func (x *smCtx) sourceFromResource(r map[string]any, idx int, contentType string) sourceItem {
	title := firstNonEmpty(globalTitle(r), str(r["id"]), fmt.Sprintf("resource_%02d", idx))
	id := str(r["id"])
	if it := selectVideoItem(r); it != nil {
		fmtv := strings.ToLower(firstNonEmpty(str(it["ti_format"]), extFormat(itemURL(it))))
		urls := x.withAccesses(itemURLs(it))
		u := firstURL(urls)
		headers := x.requestHeaders(u, true)
		extra := map[string]any{"source_url": u}
		if len(urls) > 1 {
			extra["source_urls"] = urls
		}
		if isM3U8URL(u, fmtv) {
			if dataURL, manifest, srcURL, err := x.prepareM3U8Candidates(urls); err == nil && dataURL != "" {
				extra["m3u8_text"] = manifest
				extra["source_url"] = srcURL
				u = dataURL
				headers = x.requestHeaders(srcURL, true)
				urls = []string{dataURL}
				fmtv = "m3u8"
			}
		}
		return sourceItem{kind: "video", url: u, urls: urls, fmt: firstNonEmpty(fmtv, "m3u8"), name: fmt.Sprintf("(%d)--%s", idx, title), title: title, id: id, headers: headers, urlHeaders: x.requestHeadersForURLs(urls, true), size: itemSize(it), extra: extra}
	}
	if it := selectFileItem(r); it != nil {
		urls := x.withAccesses(itemURLs(it))
		if contentType == "thematic_course" && str(it["ti_file_flag"]) == "source" {
			urls = privateURLsToPublic(urls)
		}
		u := firstURL(urls)
		extra := map[string]any{"source_url": u}
		if len(urls) > 1 {
			extra["source_urls"] = urls
		}
		return sourceItem{kind: "file", url: u, urls: urls, fmt: strings.ToLower(firstNonEmpty(str(it["ti_format"]), extFormat(u))), name: fmt.Sprintf("(%d)--%s", idx, title), title: title, id: id, headers: x.requestHeaders(u, true), urlHeaders: x.requestHeadersForURLs(urls, true), size: itemSize(it), extra: extra}
	}
	return sourceItem{}
}

func (x *smCtx) withAccess(raw string) string {
	raw = normalize(raw, "")
	if raw == "" || x.accessToken == "" || !isPrivate(raw) || strings.Contains(strings.ToLower(raw), "accesstoken=") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("accessToken", x.accessToken)
	u.RawQuery = q.Encode()
	return u.String()
}

func (x *smCtx) withAccesses(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		out = append(out, x.withAccess(raw))
	}
	return dedupeStrings(out)
}

func (x *smCtx) requestHeadersForURLs(urls []string, auth bool) map[string]map[string]string {
	out := make(map[string]map[string]string, len(urls))
	for _, raw := range urls {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		out[raw] = x.requestHeaders(raw, auth)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstURL(urls []string) string {
	for _, u := range urls {
		if strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u)
		}
	}
	return ""
}

func mediaFromSources(title string, srcs []sourceItem) (*extractor.MediaInfo, error) {
	if len(srcs) == 0 {
		return nil, fmt.Errorf("smartedu: no playable resource found")
	}
	mk := func(src sourceItem) *extractor.MediaInfo {
		headers := src.headers
		if len(headers) == 0 {
			headers = map[string]string{"Referer": refererURL}
		}
		format := firstNonEmpty(src.fmt, "mp4")
		urls := dedupeStrings(src.urls)
		if len(urls) == 0 && src.url != "" {
			urls = []string{src.url}
		}
		stream := extractor.Stream{Quality: src.kind, URLs: urls, Format: format, Size: src.size, Headers: headers}
		if len(urls) > 1 {
			stream.Extra = map[string]any{"url_mode": "mirror", "cdn_nodes": true}
			if len(src.urlHeaders) > 0 {
				stream.Extra["url_headers"] = src.urlHeaders
			}
		}
		if format == "m3u8" {
			stream.NeedMerge = true
		}
		extra := map[string]any{"id": src.id, "kind": src.kind, "title": src.title}
		for k, v := range src.extra {
			extra[k] = v
		}
		return &extractor.MediaInfo{Site: "smartedu", Title: src.name, Streams: map[string]extractor.Stream{"default": stream}, Extra: extra}
	}

	if len(srcs) == 1 {
		m := mk(srcs[0])
		m.Title = firstNonEmpty(srcs[0].title, title)
		return m, nil
	}
	entries := make([]*extractor.MediaInfo, 0, len(srcs))
	for _, src := range srcs {
		entries = append(entries, mk(src))
	}
	return &extractor.MediaInfo{Site: "smartedu", Title: title, Entries: entries}, nil
}

func smarteduDefaultTags(q url.Values) []string {
	raw := firstQuery(q, "defaultTag", "defaulttag")
	if raw == "" {
		return nil
	}
	if decoded, err := url.QueryUnescape(raw); err == nil {
		raw = decoded
	}
	parts := strings.Split(raw, "/")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}

func chaptersFromSmarteduTree(v any) []extractor.Chapter {
	var chapters []extractor.Chapter
	seen := map[string]bool{}
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case []any:
			for _, item := range t {
				walk(item)
			}
		case map[string]any:
			id := str(t["id"])
			title := firstNonEmpty(globalTitle(t), str(t["rich_title"]), id)
			if id != "" && title != "" && !seen[id] {
				seen[id] = true
				chapters = append(chapters, extractor.Chapter{Title: title, URL: id, Index: len(chapters) + 1})
			}
			for _, key := range []string{"child_nodes", "children"} {
				walk(t[key])
			}
		}
	}
	walk(v)
	return chapters
}
