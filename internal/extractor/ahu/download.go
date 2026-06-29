package ahu

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Sophomoresty/mediago/internal/download"
	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
	"github.com/Sophomoresty/mediago/internal/util"
)

const (
	ahuModeFHD     = 1
	ahuModeHD      = 2
	ahuModeSD      = 3
	ahuModeOnlyPDF = 4
)

type ahuCourse struct {
	client      *util.Client
	cookies     http.CookieJar
	headers     map[string]string
	mode        int
	cid         string
	title       string
	infos       ahuDownloadInfo
	sourceInfo  ahuDownloadInfo
	outline     []*ahuTreeNode
	playCache   map[string]ahuPlayInfo
	licenseMemo map[string][]byte
	trialVideos int
	trialFiles  int
}

type AhuDownloadOptions struct {
	Cookies     http.CookieJar
	Mode        int
	OutputDir   string
	Quality     string
	ListOnly    bool
	Overwrite   bool
	Concurrency int
	NoProgress  bool
	Proxy       string
	Context     context.Context
}

type ahuTreeNode struct {
	DirTitle   string
	RawTitle   string
	IndexTuple []int
	VideoCnt   int
	FileCnt    int
	Children   []*ahuTreeNode
	Sources    []ahuSource
}

type ahuSource struct {
	Type      string
	Title     string
	Name      string
	VideoName string
	LessonID  string
	PlayURL   string
	Duration  string
	FileName  string
	FileURL   string
	FileFmt   string
	Index     []int
}

type ahuDownloadInfo map[string]any

type ahuPlayKind string

const (
	ahuPlayAliyun    ahuPlayKind = "aliyun"
	ahuPlayBaijiayun ahuPlayKind = "baijiayun"
	ahuPlayM3U8      ahuPlayKind = "m3u8"
	ahuPlayVideo     ahuPlayKind = "video"
)

type ahuPlayInfo struct {
	Kind       ahuPlayKind
	VideoID    string
	PlayAuth   string
	Token      string
	PlayID     string
	DirectURL  string
	SourceURL  string
	LessonID   string
	PlayPage   string
	EncryptTyp string
}

func NewAhuDownloadInfo(rawURL string, opts AhuDownloadOptions) (*extractor.MediaInfo, error) {
	ctx, err := newAhuCourse(opts)
	if err != nil {
		return nil, err
	}
	if err := ctx.prepare(rawURL, true); err != nil {
		return nil, err
	}
	return ctx.mediaInfo()
}

func DownloadAhu(rawURL string, opts AhuDownloadOptions) ([]string, error) {
	ctx, err := newAhuCourse(opts)
	if err != nil {
		return nil, err
	}
	if err := ctx.prepare(rawURL, true); err != nil {
		return nil, err
	}
	return ctx.download(opts)
}

func newAhuCourse(opts AhuDownloadOptions) (*ahuCourse, error) {
	if opts.Cookies == nil {
		return nil, fmt.Errorf("ahu requires login cookies")
	}
	c := util.NewClient()
	if opts.Proxy != "" {
		pc, err := util.NewClientWithProxy(opts.Proxy)
		if err != nil {
			return nil, err
		}
		c = pc
	}
	c.SetCookieJar(opts.Cookies)
	mode := opts.Mode
	if mode == 0 {
		mode = modeFromQuality(opts.Quality)
	}
	if mode == 0 {
		mode = ahuModeFHD
	}
	h := map[string]string{"Referer": course_list_url, "referer": course_list_url}
	return &ahuCourse{
		client:      c,
		cookies:     opts.Cookies,
		headers:     h,
		mode:        mode,
		playCache:   map[string]ahuPlayInfo{},
		licenseMemo: map[string][]byte{},
	}, nil
}

func modeFromQuality(q string) int {
	switch strings.ToLower(strings.TrimSpace(q)) {
	case "only_pdf", "pdf", "file", "files", "source", "sources":
		return ahuModeOnlyPDF
	case "sd", "ld", "fd":
		return ahuModeSD
	case "hd":
		return ahuModeHD
	case "fhd", "od", "2k", "4k", "best":
		return ahuModeFHD
	default:
		return 0
	}
}

func (x *ahuCourse) prepare(rawURL string, parseInfos bool) error {
	x.outline = nil
	x.playCache = map[string]ahuPlayInfo{}
	if err := validateAhuLogin(x.client, x.cookies, x.headers); err != nil {
		return err
	}
	cid := extractFirst(cidRe, rawURL)
	if cid == "" {
		courses := fetchCourseList(x.client, x.headers)
		if len(courses) == 0 {
			return fmt.Errorf("cannot parse courseId from URL: %s", rawURL)
		}
		cid = courses[0].ID
		x.title = util.SanitizeFilename(courses[0].Title)
	}
	if cid == "" {
		return fmt.Errorf("cannot parse courseId from URL: %s", rawURL)
	}
	x.cid = cid
	if !parseInfos {
		return nil
	}
	return x.getInfos()
}

func (x *ahuCourse) getInfos() error {
	detailURL := fmt.Sprintf(course_info_url, x.cid)
	body, err := x.client.GetString(detailURL, x.headers)
	if err != nil {
		return fmt.Errorf("fetch ahu course info: %w", err)
	}
	if t := extractTitle(body); t != "" {
		x.title = util.SanitizeFilename(t)
	}
	if x.title == "" {
		x.title = "阿虎课程" + x.cid
	}
	videos := x.parseCourseVideos(body, detailURL)
	files := x.parseCourseFilesTree(body, fmt.Sprintf(course_info_url, x.cid))
	x.infos = treeMapToDownloadInfo(videos)
	x.sourceInfo = treeMapToDownloadInfo(files)
	if len(x.infos) == 0 && len(x.sourceInfo) == 0 {
		return fmt.Errorf("未解析到该课程的视频或课件内容。")
	}
	return nil
}

func (x *ahuCourse) mediaInfo() (*extractor.MediaInfo, error) {
	entries := x.entriesFromInfo(x.infos, true, false)
	entries = append(entries, x.entriesFromInfo(x.sourceInfo, false, true)...)
	entries = dedupeEntries(entries)
	if len(entries) == 0 {
		return nil, fmt.Errorf("ahu: no downloadable video or course files found")
	}
	return &extractor.MediaInfo{Site: "ahu", Title: util.SanitizeFilename(firstNonEmpty(x.title, "ahu_"+x.cid)), Entries: entries, Extra: map[string]any{"course_id": x.cid}}, nil
}

func (x *ahuCourse) download(optsList ...AhuDownloadOptions) ([]string, error) {
	var opts AhuDownloadOptions
	if len(optsList) > 0 {
		opts = optsList[0]
	}
	outDir := opts.OutputDir
	if outDir == "" {
		outDir = "."
	}
	title := util.SanitizeFilename(firstNonEmpty(x.title, "ahu_"+x.cid))
	var entries []*extractor.MediaInfo
	if x.mode != ahuModeOnlyPDF {
		entries = append(entries, x.entriesFromInfo(x.infos, true, false)...)
	}
	entries = append(entries, x.entriesFromInfo(x.sourceInfo, false, true)...)
	entries = dedupeEntries(entries)
	if len(entries) == 0 {
		return nil, fmt.Errorf("ahu: no downloadable video or course files found")
	}
	root := filepath.Join(outDir, title)
	var paths []string
	for _, entry := range entries {
		stream, ok := bestEntryStream(entry)
		if !ok {
			continue
		}
		eng := download.New(download.Opts{OutputDir: entryOutputDir(root, entry), Overwrite: opts.Overwrite, Concurrency: opts.Concurrency, NoProgress: opts.NoProgress, Proxy: opts.Proxy, Context: opts.Context})
		path, err := eng.Download(entry, stream)
		if err != nil {
			return paths, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func entryOutputDir(root string, entry *extractor.MediaInfo) string {
	if entry == nil || entry.Extra == nil {
		return root
	}
	if parts, ok := entry.Extra["chapter_path"].([]string); ok && len(parts) > 0 {
		join := append([]string{root}, parts...)
		return filepath.Join(join...)
	}
	if values, ok := entry.Extra["chapter_path"].([]any); ok && len(values) > 0 {
		join := []string{root}
		for _, v := range values {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				join = append(join, util.SanitizeFilename(s))
			}
		}
		return filepath.Join(join...)
	}
	return root
}

func bestEntryStream(entry *extractor.MediaInfo) (extractor.Stream, bool) {
	if entry == nil || len(entry.Streams) == 0 {
		return extractor.Stream{}, false
	}
	if s, ok := entry.Streams["best"]; ok {
		return s, true
	}
	for _, s := range entry.Streams {
		return s, true
	}
	return extractor.Stream{}, false
}

func (x *ahuCourse) entriesFromInfo(info ahuDownloadInfo, wantVideos, wantFiles bool) []*extractor.MediaInfo {
	var out []*extractor.MediaInfo
	var walk func(any, []string)
	walk = func(v any, path []string) {
		switch t := v.(type) {
		case ahuDownloadInfo:
			keys := sortedKeys(map[string]any(t))
			for _, key := range keys {
				walk(t[key], append(path, key))
			}
		case []ahuSource:
			for _, src := range t {
				if src.Type == "video" && wantVideos {
					entry, err := x.videoEntry(src, path)
					if err == nil && entry != nil {
						out = append(out, entry)
					}
				}
				if src.Type == "file" && wantFiles {
					out = append(out, fileSourceEntry(src, x.headers, path))
				}
			}
		case map[string]any:
			keys := sortedKeys(t)
			for _, key := range keys {
				walk(t[key], append(path, key))
			}
		case []any:
			for _, child := range t {
				walk(child, path)
			}
		case ahuSource:
			if t.Type == "video" && wantVideos {
				entry, err := x.videoEntry(t, path)
				if err == nil && entry != nil {
					out = append(out, entry)
				}
			}
			if t.Type == "file" && wantFiles {
				out = append(out, fileSourceEntry(t, x.headers, path))
			}
		}
	}
	walk(info, nil)
	return out
}

func (x *ahuCourse) videoEntry(src ahuSource, path []string) (*extractor.MediaInfo, error) {
	play, err := x.getPlayInfo(src.LessonID)
	if err != nil {
		return nil, err
	}
	stream, extra, err := x.streamFromPlay(play)
	if err != nil {
		return nil, err
	}
	title := util.SanitizeFilename(firstNonEmpty(src.Title, src.VideoName, src.Name, src.LessonID, "未命名课时"))
	if extra == nil {
		extra = map[string]any{}
	}
	extra["type"] = "video"
	extra["lesson_id"] = src.LessonID
	extra["course_id"] = x.cid
	if len(path) > 0 {
		extra["chapter_path"] = append([]string(nil), path...)
	}
	return &extractor.MediaInfo{Site: "ahu", Title: title, Streams: map[string]extractor.Stream{"best": stream}, Extra: extra}, nil
}

func fileSourceEntry(src ahuSource, headers map[string]string, path []string) *extractor.MediaInfo {
	src.FileURL = quoteResourceURL(src.FileURL)
	fmtName := firstNonEmpty(src.FileFmt, pickFormat(src.FileURL), "dat")
	title := strings.TrimSuffix(util.SanitizeFilename(firstNonEmpty(src.FileName, src.Title, fileTitleFromURL(src.FileURL), "课程资料")), "."+fmtName)
	extra := map[string]any{"type": "file", "source_url": src.FileURL, "file_fmt": fmtName}
	if len(path) > 0 {
		extra["chapter_path"] = append([]string(nil), path...)
	}
	return &extractor.MediaInfo{Site: "ahu", Title: title, Streams: map[string]extractor.Stream{"best": {Quality: "best", URLs: []string{src.FileURL}, Format: fmtName, Headers: cloneHeaders(headers)}}, Extra: extra}
}

func (x *ahuCourse) getPlayInfo(lessonID string) (ahuPlayInfo, error) {
	lessonID = strings.TrimSpace(lessonID)
	if lessonID == "" {
		return ahuPlayInfo{}, fmt.Errorf("ahu: empty lesson_id")
	}
	if cached, ok := x.playCache[lessonID]; ok {
		return cached, nil
	}
	playURL := fmt.Sprintf(video_play_url, x.cid, lessonID, lessonID)
	h := cloneHeaders(x.headers)
	h["Referer"] = fmt.Sprintf(course_info_url, x.cid)
	h["referer"] = fmt.Sprintf(course_info_url, x.cid)
	body, err := x.client.GetString(playURL, h)
	if err != nil {
		return ahuPlayInfo{}, fmt.Errorf("fetch ahu play page: %w", err)
	}
	videoID := firstNonEmpty(jsVar(body, "aliyunVideoId"), jsVar(body, "videoId"), jsVar(body, "vodVideoId"), jsVar(body, "aliyunVid"), jsonField(body, "VideoId"), jsonField(body, "videoId"), jsonField(body, "vid"))
	playAuth := firstNonEmpty(jsVar(body, "playAuth"), jsVar(body, "PlayAuth"), jsonField(body, "PlayAuth"), jsonField(body, "playAuth"))
	if videoID != "" && playAuth != "" {
		pi := ahuPlayInfo{Kind: ahuPlayAliyun, VideoID: videoID, PlayAuth: playAuth, LessonID: lessonID, PlayPage: body}
		x.playCache[lessonID] = pi
		return pi, nil
	}
	hlsToken := firstNonEmpty(jsVar(body, "hlsToken"), jsonField(body, "hlsToken"))
	playID := firstNonEmpty(jsVar(body, "playId"), jsVar(body, "roomId"), jsVar(body, "room_id"), jsonField(body, "playId"), jsonField(body, "roomId"), jsonField(body, "room_id"))
	if hlsToken != "" && playID != "" {
		pi := ahuPlayInfo{Kind: ahuPlayBaijiayun, Token: hlsToken, PlayID: playID, LessonID: lessonID, PlayPage: body}
		x.playCache[lessonID] = pi
		return pi, nil
	}
	if direct := normalizeResourceURLWithBase(extractFirst(directURLRe, body), playURL); direct != "" {
		kind := ahuPlayVideo
		if strings.Contains(strings.ToLower(direct), ".m3u8") {
			kind = ahuPlayM3U8
		}
		pi := ahuPlayInfo{Kind: kind, DirectURL: direct, LessonID: lessonID, PlayPage: body}
		x.playCache[lessonID] = pi
		return pi, nil
	}
	return ahuPlayInfo{}, fmt.Errorf("ahu: no direct/aliyun/baijiayun media URL for lesson %s", lessonID)
}

func (x *ahuCourse) streamFromPlay(play ahuPlayInfo) (extractor.Stream, map[string]any, error) {
	switch play.Kind {
	case ahuPlayAliyun:
		info, err := x.requestAliyunPlayInfo(play.PlayAuth, play.VideoID)
		if err != nil {
			return extractor.Stream{}, nil, err
		}
		mediaURL := normalizeResourceURL(info.URL)
		if info.M3U8Text != "" {
			mediaURL = ahuM3U8DataURL(info.M3U8Text)
		}
		stream := mediaStream(mediaURL, ahuM3U8Headers(x.headers))
		stream.Size = info.Size
		stream.NeedMerge = info.NeedMerge || stream.Format == "m3u8"
		extra := map[string]any{"playback": "aliyun", "aliyun_vid": play.VideoID, "definition": info.Definition, "encrypt_type": info.EncryptType, "source_type": info.SourceType, "aliyun_api": info.APIURL}
		if info.M3U8Text != "" {
			extra["m3u8_text"] = info.M3U8Text
		}
		return stream, extra, nil
	case ahuPlayBaijiayun:
		playbackURL, err := shared.BaijiayunResolvePlayback(x.client, play.PlayID, play.Token, x.headers)
		if err != nil {
			return extractor.Stream{}, nil, err
		}
		return mediaStream(playbackURL, x.headers), map[string]any{"playback": "baijiayun", "play_id": play.PlayID}, nil
	case ahuPlayM3U8, ahuPlayVideo:
		return mediaStream(play.DirectURL, x.headers), map[string]any{"playback": string(play.Kind)}, nil
	default:
		return extractor.Stream{}, nil, fmt.Errorf("ahu: unsupported play kind %q", play.Kind)
	}
}

func (x *ahuCourse) requestAliyunPlayInfo(playAuth, videoID string) (*shared.AliyunPlayInfo, error) {
	payload := x.decodeAliyunPlayAuth(playAuth)
	payload.Region = firstNonEmpty(payload.Region, "cn-shanghai")
	payload.AuthTimeout = firstNonEmpty(payload.AuthTimeout, "7200")
	if payload.AccessKeyID == "" || payload.AccessKeySecret == "" || payload.AuthInfo == "" {
		return nil, fmt.Errorf("ahu aliyun playAuth missing access/authInfo")
	}
	playCfg := `{"EncryptType":"AliyunVoDEncryption"}`
	opts := shared.AliyunPlayOptions{Headers: ahuAliyunHeaders(x.headers), Referer: firstNonEmpty(x.headers["Referer"], x.headers["referer"], course_list_url), Origin: "https://www.ahuyikao.com", Formats: "m3u8,mp4", Definitions: x.definitions(), FetchM3U8: true, RewriteM3U8Keys: true, ExtraParams: map[string]string{"PlayConfig": playCfg}}
	info, err := shared.AliyunResolvePlayInfo(x.client, payload, videoID, opts)
	if err != nil {
		return nil, fmt.Errorf("ahu aliyun GetPlayInfo: %w", err)
	}
	if err := x.prepareAliyunM3U8(info, payload, opts); err != nil {
		return nil, err
	}
	return info, nil
}

func (x *ahuCourse) requestAliyunPlayInfoByRand(playAuth, videoID, randValue string) (*shared.AliyunPlayInfo, error) {
	return x.requestAliyunPlayInfoWithExtra(playAuth, videoID, map[string]string{"Rand": randValue, "Channel": "HTML5", "PlayerVersion": "2.32.0", "PlayConfig": "{}", "ReAuthInfo": "{}", "StreamType": "video"})
}

func (x *ahuCourse) requestAliyunPlayInfoLegacy(playAuth, videoID string) (*shared.AliyunPlayInfo, error) {
	return x.requestAliyunPlayInfoWithExtra(playAuth, videoID, nil)
}

func (x *ahuCourse) requestAliyunPlayInfoWithExtra(playAuth, videoID string, extra map[string]string) (*shared.AliyunPlayInfo, error) {
	payload := x.decodeAliyunPlayAuth(playAuth)
	payload.Region = firstNonEmpty(payload.Region, "cn-shanghai")
	payload.AuthTimeout = firstNonEmpty(payload.AuthTimeout, "7200")
	if payload.AccessKeyID == "" || payload.AccessKeySecret == "" || payload.AuthInfo == "" {
		return nil, fmt.Errorf("ahu aliyun playAuth missing access/authInfo")
	}
	opts := shared.AliyunPlayOptions{Headers: ahuAliyunHeaders(x.headers), Referer: firstNonEmpty(x.headers["Referer"], x.headers["referer"], course_list_url), Origin: "https://www.ahuyikao.com", Formats: "m3u8,mp4", Definitions: x.definitions(), FetchM3U8: true, RewriteM3U8Keys: true, ExtraParams: extra}
	info, err := shared.AliyunResolvePlayInfo(x.client, payload, videoID, opts)
	if err != nil {
		return nil, err
	}
	if err := x.prepareAliyunM3U8(info, payload, opts); err != nil {
		return nil, err
	}
	return info, nil
}

func (x *ahuCourse) prepareAliyunM3U8(info *shared.AliyunPlayInfo, payload shared.AliyunPlayPayload, opts shared.AliyunPlayOptions) error {
	if info == nil {
		return nil
	}
	text := strings.TrimSpace(info.M3U8Text)
	if strings.EqualFold(info.EncryptType, "AliyunVoDEncryption") && info.NeedMerge {
		sourceURL := info.URL
		if text == "" {
			fetched, err := x.client.GetString(info.URL, ahuM3U8Headers(x.headers))
			if err != nil {
				return err
			}
			text = fetched
		}
		if strings.Contains(text, "#EXT-X-STREAM-INF") && !strings.Contains(text, "#EXT-X-KEY") {
			if variant := ahuFirstVariantURL(text, sourceURL); variant != "" {
				if vt, err := x.client.GetString(variant, ahuM3U8Headers(x.headers)); err == nil && strings.TrimSpace(vt) != "" {
					text = vt
					sourceURL = variant
					info.URL = variant
				}
			}
		}
		if strings.Contains(text, "#EXT-X-KEY") && !ahuM3U8KeysPrepared(text) {
			rewritten, err := shared.AliyunRewriteM3U8Keys(x.client, text, payload, info.EncryptType, sourceURL, opts)
			if err != nil {
				return fmt.Errorf("ahu aliyun GetLicense: %w", err)
			}
			text = rewritten
		}
		info.M3U8Text = ahuInlineHexKeysAsDataURLs(ahuAbsolutizeM3U8Text(text, sourceURL))
		info.SourceType = "m3u8_text"
		return nil
	}
	if text != "" {
		info.M3U8Text = ahuInlineHexKeysAsDataURLs(ahuAbsolutizeM3U8Text(text, info.URL))
	}
	return nil
}

func ahuM3U8KeysPrepared(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "data:application/octet-stream") || strings.Contains(lower, `uri="0x`) || strings.Contains(lower, `uri=0x`)
}

func (x *ahuCourse) decodeAliyunPlayAuth(playAuth any) shared.AliyunPlayPayload {
	payload := shared.AliyunDecodePlayAuth(playAuth)
	payload.Region = firstNonEmpty(payload.Region, "cn-shanghai")
	payload.AuthTimeout = firstNonEmpty(payload.AuthTimeout, "7200")
	return payload
}

func (x *ahuCourse) extractAliyunKeyMaterial(keyToken string) (string, string) {
	return shared.AliyunExtractKeyMaterial([]byte(keyToken))
}

func (x *ahuCourse) requestAliyunLicense(payload shared.AliyunPlayPayload, mediaID, challenge, encType string) ([]byte, error) {
	key := mediaID + ":" + challenge
	if b := x.licenseMemo[key]; len(b) > 0 {
		return b, nil
	}
	opts := shared.AliyunPlayOptions{Headers: ahuAliyunHeaders(x.headers), Referer: firstNonEmpty(x.headers["Referer"], x.headers["referer"], course_list_url), Origin: "https://www.ahuyikao.com"}
	b, err := shared.AliyunRequestLicense(x.client, payload, mediaID, challenge, encType, opts)
	if err != nil {
		return nil, err
	}
	x.licenseMemo[key] = b
	return b, nil
}

func (x *ahuCourse) definitions() string {
	switch x.mode {
	case ahuModeSD:
		return "SD,LD,FD,HD,OD,2K,4K"
	case ahuModeHD:
		return "HD,SD,LD,FD,OD,2K,4K"
	default:
		return "FD,LD,SD,HD,OD,2K,4K"
	}
}
