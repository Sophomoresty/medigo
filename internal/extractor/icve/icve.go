// Package icve implements source-aligned Icve AI extraction.
package icve

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/nichuanfang/medigo/internal/extractor"
	"github.com/nichuanfang/medigo/internal/util"
)

const (
	COURSENAME         = "{1}--课程"
	FILENAME           = "{2}--资源"
	MATERIAL           = "【全部素材】"
	IS_HD              = 1
	IS_SD              = 2
	ONLY_PDF           = 3
	LEN_S              = 96
	LEN_               = 48
	TIME_SLEEP         = 3.6
	referer            = "https://www.icve.com.cn"
	smartedu_referer   = "https://vocational.smartedu.cn"
	url_title          = "https://ai.icve.com.cn/prod-api/course/courseInfo/getLatestInfoByCourseId?courseId=%s"
	url_info           = "https://ai.icve.com.cn/prod-api/course/courseDesign/getDesignList?courseInfoId=%s&courseId=%s"
	url_cell           = "https://ai.icve.com.cn/prod-api/course/courseDesign/getCellList?courseInfoId=%s&courseId=%s&parentId=%s"
	url_source_status  = "https://upload.icve.com.cn/%s/status"
	smartedu_query_url = "https://vocational.smartedu.cn/gjzyjy/inco/ht/queryList"
)

var patterns = []string{`\s*((https?://ai\.icve\.com\.cn/.*?excellent.*?/(?P<cid1>[-\w]+))|(https?://ai\.icve\.com\.cn/.*?course.*?/(?P<cid2>[-\w]+)))`}

func init() {
	extractor.Register(&Icve{}, extractor.SiteInfo{Name: "Icve", URL: "icve.com.cn", NeedAuth: false})
}

type Icve struct{}

func (i *Icve) Patterns() []string { return patterns }

type aiCtx struct {
	c       *util.Client
	headers map[string]string
	mode    int
	cid     string
	infoID  string
	title   string
}

type aiItem struct {
	Name string
	Info string
	Kind string
	Ext  string
}

type aiTitleResp struct {
	Data struct {
		ID         any `json:"id"`
		CourseName any `json:"courseName"`
		SchoolName any `json:"schoolName"`
	} `json:"data"`
}

func (i *Icve) Extract(rawURL string, opts *extractor.ExtractOpts) (*extractor.MediaInfo, error) {
	if opts == nil {
		opts = &extractor.ExtractOpts{}
	}
	jar := opts.Cookies
	if jar == nil {
		jar, _ = cookiejar.New(nil)
	}
	x := newCtx(jar, modeFromQuality(opts.Quality))
	x.cid = parseCID(rawURL)
	if x.cid == "" {
		return nil, fmt.Errorf("icve: cannot parse course id from URL")
	}
	if err := x.loadTitle(); err != nil {
		return nil, err
	}
	items, err := x.loadItems()
	if err != nil {
		return nil, err
	}
	return x.mediaFromItems(items)
}

func newCtx(jar http.CookieJar, mode int) *aiCtx {
	c := util.NewClient()
	c.SetCookieJar(jar)
	headers := map[string]string{
		"Sec-Fetch-Site":     "same-origin",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Dest":     "empty",
		"Sec-Ch-Ua-Platform": `"Windows"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua":          `"Not/A)Brand";v="99", "Google Chrome";v="115", "Chromium";v="115"`,
		"Referer":            referer,
		"cookie":             cookieHeader(jar, []string{referer + "/", "https://ai.icve.com.cn/", "https://upload.icve.com.cn/"}),
		"User-Agent":         util.RandomUA(),
	}
	return &aiCtx{c: c, headers: headers, mode: mode}
}

func (x *aiCtx) loadTitle() error {
	body, err := x.c.GetString(fmt.Sprintf(url_title, url.QueryEscape(x.cid)), x.headers)
	if err != nil {
		return err
	}
	var resp aiTitleResp
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		resp = aiTitleResp{}
	}
	x.infoID = str(resp.Data.ID)
	x.title = cleanTitle(fmt.Sprintf("%s_%s", str(resp.Data.CourseName), str(resp.Data.SchoolName)))
	return nil
}

func (x *aiCtx) loadItems() ([]aiItem, error) {
	body, err := x.c.GetString(fmt.Sprintf(url_info, url.QueryEscape(x.infoID), url.QueryEscape(x.cid)), x.headers)
	if err != nil {
		return nil, err
	}
	root := parseJSONMap(body)
	data := listAt(root, "data")
	sortBySort(data)
	var items []aiItem
	items = append(items, collectAIItems(data, nil)...)
	for i, node := range data {
		if id := str(node["id"]); id != "" {
			cellItems, err := x.loadCellItems(id)
			if err != nil {
				continue
			}
			items = append(items, collectAIItems(cellItems, []int{i + 1})...)
		}
	}
	return dedupeAIItems(items), nil
}

func (x *aiCtx) loadCellItems(parentID string) ([]map[string]any, error) {
	body, err := x.c.GetString(fmt.Sprintf(url_cell, url.QueryEscape(x.infoID), url.QueryEscape(x.cid), url.QueryEscape(parentID)), x.headers)
	if err != nil {
		return nil, err
	}
	root := parseJSONMap(body)
	data := listAt(root, "data")
	sortBySort(data)
	return data, nil
}

func (x *aiCtx) mediaFromItems(items []aiItem) (*extractor.MediaInfo, error) {
	var entries []*extractor.MediaInfo
	var lastErr error
	for _, item := range items {
		switch item.Kind {
		case "video":
			if x.mode == ONLY_PDF {
				continue
			}
			url, ext := x.getVideoURL(item.Info)
			if url == "" {
				continue
			}
			if ext == "" {
				ext = pickExt(url)
			}
			if ext == "" {
				ext = "mp4"
			}
			entries = append(entries, &extractor.MediaInfo{
				Site:  "icve",
				Title: item.Name,
				Streams: map[string]extractor.Stream{
					ext: {Quality: ext, URLs: []string{url}, Format: ext, NeedMerge: ext == "m3u8", Headers: cloneHeaders(x.headers)},
				},
				Extra: map[string]any{"kind": "video"},
			})
		case "file":
			url, ext := x.getFileURL(item.Info)
			if url == "" {
				continue
			}
			if ext == "" {
				ext = pickExt(url)
			}
			if ext == "" {
				ext = "html"
			}
			entries = append(entries, &extractor.MediaInfo{
				Site:  "icve",
				Title: item.Name,
				Streams: map[string]extractor.Stream{
					ext: {Quality: ext, URLs: []string{url}, Format: ext, Headers: cloneHeaders(x.headers)},
				},
				Extra: map[string]any{"kind": "file"},
			})
		default:
			lastErr = fmt.Errorf("icve: unknown item kind %q", item.Kind)
		}
	}
	if len(entries) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("icve: no playable entries")
	}
	if len(entries) == 1 {
		if x.title != "" {
			entries[0].Extra["course_title"] = x.title
		}
		return entries[0], nil
	}
	return &extractor.MediaInfo{Site: "icve", Title: firstNonEmpty(x.title, x.cid, "icve"), Entries: entries, Extra: map[string]any{"course_id": x.cid, "info_id": x.infoID}}, nil
}

func (x *aiCtx) getVideoURL(videoInfo string) (string, string) {
	data := parseJSONMap(videoInfo)
	if len(data) == 0 {
		return "", ""
	}
	oriURL := str(data["ossOriUrl"])
	ext := pickExt(oriURL)
	if ext == "" {
		ext = "mp4"
	}
	if genURL := str(data["ossGenUrl"]); genURL != "" && strings.HasPrefix(genURL, "http") {
		if content := str(data["content"]); content != "" {
			statusBody, err := x.c.GetString(fmt.Sprintf(url_source_status, strings.TrimLeft(content, "/")), x.headers)
			if err == nil {
				status := parseJSONMap(statusBody)
				if u := x.selectTranscodedURL(genURL, ext, status); u != "" {
					return u, pickExt(u)
				}
			}
		}
		if u := x.selectTranscodedURL(genURL, ext, map[string]any{}); u != "" {
			return u, pickExt(u)
		}
	}
	if oriURL != "" {
		return oriURL, ext
	}
	return str(data["url"]), pickExt(str(data["url"]))
}

func (x *aiCtx) getFileURL(fileInfo string) (string, string) {
	data := parseJSONMap(fileInfo)
	if len(data) == 0 {
		return "", ""
	}
	oriURL := str(data["ossOriUrl"])
	if oriURL != "" {
		return oriURL, pickExt(oriURL)
	}
	return str(data["url"]), pickExt(str(data["url"]))
}

func (x *aiCtx) selectTranscodedURL(genURL, originExt string, status map[string]any) string {
	args := mapAt(status, "args")
	qualityOrder := x.videoQualityCandidates()
	if q := x.selectVideoQuality(args); q != "" {
		qualityOrder = append([]string{q}, filterOtherQualities(qualityOrder, q)...)
	}
	extOrder := []string{"mp4", "m3u8"}
	if originExt == "m3u8" {
		extOrder = []string{"m3u8", "mp4"}
	}
	if typ := strings.ToLower(firstNonEmpty(str(status["type"]), str(args["type"]))); strings.Contains(typ, "m3u8") {
		extOrder = []string{"m3u8", "mp4"}
	}
	for _, q := range qualityOrder {
		for _, ext := range extOrder {
			u := fmt.Sprintf("%s/%s.%s", strings.TrimRight(genURL, "/"), q, ext)
			if x.checkURL(u) {
				return u
			}
		}
	}
	return ""
}

func (x *aiCtx) checkURL(raw string) bool {
	resp, err := x.c.Get(raw, x.headers)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return true
}

func (x *aiCtx) videoQualityCandidates() []string {
	switch x.mode {
	case IS_HD:
		return []string{"720p", "480p", "360p", "1080p"}
	case IS_SD:
		return []string{"480p", "360p", "720p", "1080p"}
	default:
		return []string{"360p", "480p", "720p", "1080p"}
	}
}

func (x *aiCtx) selectVideoQuality(args map[string]any) string {
	for _, q := range x.videoQualityCandidates() {
		v := args[q]
		if v == true || strings.EqualFold(str(v), "true") {
			return q
		}
	}
	return ""
}
