package ahu

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sophomoresty/mediago/internal/download"
	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/extractor/shared"
	"github.com/Sophomoresty/mediago/internal/util"
)

func splitDownloadInfo(v any) (videos []ahuSource, files []ahuSource) {
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case []ahuSource:
			for _, src := range t {
				if src.Type == "file" {
					files = append(files, src)
				} else if src.Type == "video" {
					videos = append(videos, src)
				}
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		case map[string]any:
			for _, child := range t {
				walk(child)
			}
		case ahuDownloadInfo:
			for _, child := range t {
				walk(child)
			}
		case ahuSource:
			if t.Type == "file" {
				files = append(files, t)
			} else if t.Type == "video" {
				videos = append(videos, t)
			}
		}
	}
	walk(v)
	return videos, files
}

func (x *ahuCourse) shouldStopAfterTrialSuccess() bool {
	return x.mode != ahuModeOnlyPDF && (x.trialVideos >= 3 || x.trialFiles >= 3)
}

func (x *ahuCourse) downloadVideo(dirname string, videoInfo ahuSource) bool {
	entry, err := x.videoEntry(videoInfo, nil)
	if err != nil || entry == nil {
		return false
	}
	stream, ok := bestEntryStream(entry)
	if !ok {
		return false
	}
	eng := download.New(download.Opts{OutputDir: dirname, NoProgress: true})
	_, err = eng.Download(entry, stream)
	return err == nil
}

func (x *ahuCourse) downloadVideoList(dirname string, videoList []ahuSource, isTry bool) bool {
	for _, v := range videoList {
		x.downloadVideo(dirname, v)
		if isTry {
			x.trialVideos++
			if x.trialVideos >= 3 {
				return true
			}
		}
	}
	return false
}

func (x *ahuCourse) downloadOneFile(dirname string, fileInfo ahuSource) bool {
	entry := fileSourceEntry(fileInfo, x.headers, nil)
	if entry == nil {
		return false
	}
	stream, ok := bestEntryStream(entry)
	if !ok {
		return false
	}
	eng := download.New(download.Opts{OutputDir: dirname, NoProgress: true})
	_, err := eng.Download(entry, stream)
	return err == nil
}

func (x *ahuCourse) downloadFileList(dirname string, fileList []ahuSource, isTry bool) bool {
	for _, f := range fileList {
		x.downloadOneFile(dirname, f)
		if isTry {
			x.trialFiles++
			if x.trialFiles >= 3 {
				return true
			}
		}
	}
	return false
}

func (x *ahuCourse) downloadBaijiayunPlayback(token, playID, lessonName, lessonDir string) bool {
	playbackURL, err := shared.BaijiayunResolvePlayback(x.client, playID, token, x.headers)
	if err != nil || playbackURL == "" {
		return false
	}
	entry := &extractor.MediaInfo{Site: "ahu", Title: util.SanitizeFilename(firstNonEmpty(lessonName, playID, "百家云回放")), Streams: map[string]extractor.Stream{"best": mediaStream(playbackURL, x.headers)}, Extra: map[string]any{"playback": "baijiayun", "play_id": playID}}
	stream, ok := bestEntryStream(entry)
	if !ok {
		return false
	}
	eng := download.New(download.Opts{OutputDir: lessonDir, NoProgress: true})
	_, err = eng.Download(entry, stream)
	return err == nil
}

func (x *ahuCourse) buildAliyunKeyFunc(payload shared.AliyunPlayPayload, encType string) func(string) ([]byte, error) {
	return func(keyToken string) ([]byte, error) {
		mediaID, challenge := x.extractAliyunKeyMaterial(keyToken)
		if mediaID == "" || challenge == "" {
			return nil, fmt.Errorf("ahu aliyun key material missing")
		}
		return x.requestAliyunLicense(payload, mediaID, challenge, encType)
	}
}

func (x *ahuCourse) downloadSourceTree(dirname string, info any, indent string, downloadVideos, downloadFiles bool) bool {
	_ = os.MkdirAll(dirname, 0o755)
	stopped := false
	var walk func(string, any)
	walk = func(dir string, node any) {
		if stopped {
			return
		}
		switch t := node.(type) {
		case ahuDownloadInfo:
			walkMap(dir, map[string]any(t), walk)
		case map[string]any:
			walkMap(dir, t, walk)
		case []ahuSource:
			videos, files := splitDownloadInfo(t)
			if downloadVideos && x.mode != ahuModeOnlyPDF && len(videos) > 0 && x.downloadVideoList(dir, videos, false) {
				stopped = true
				return
			}
			if downloadFiles && len(files) > 0 && x.downloadFileList(dir, files, false) {
				stopped = true
			}
		case []any:
			if len(t) == 2 {
				if sources, ok := t[0].([]ahuSource); ok {
					walk(dir, sources)
					walk(dir, t[1])
					return
				}
			}
			for _, child := range t {
				walk(dir, child)
			}
		case ahuSource:
			if t.Type == "video" && downloadVideos && x.mode != ahuModeOnlyPDF {
				x.downloadVideo(dir, t)
			}
			if t.Type == "file" && downloadFiles {
				x.downloadOneFile(dir, t)
			}
		}
	}
	walk(dirname, info)
	return stopped
}

func walkMap(parent string, m map[string]any, walk func(string, any)) {
	keys := sortedKeys(m)
	for _, key := range keys {
		childDir := filepath.Join(parent, util.SanitizeFilename(key))
		_ = os.MkdirAll(childDir, 0o755)
		walk(childDir, m[key])
	}
}

func (x *ahuCourse) downloadCourse(courseDir string) bool {
	_ = os.MkdirAll(courseDir, 0o755)
	return x.downloadSourceTree(courseDir, x.infos, "  ", true, true)
}

func (x *ahuCourse) downloadFiles(filesDir string) bool {
	_ = os.MkdirAll(filesDir, 0o755)
	return x.downloadSourceTree(filesDir, x.sourceInfo, "  ", false, true)
}

// Compatibility aliases mirror Python/decompiled names in comments/searchable API.
func (x *ahuCourse) _get_infos() error { return x.getInfos() }
func (x *ahuCourse) _get_play_info(lessonID string) (ahuPlayInfo, error) {
	return x.getPlayInfo(lessonID)
}
func (x *ahuCourse) _decode_aliyun_play_auth(playAuth any) shared.AliyunPlayPayload {
	return x.decodeAliyunPlayAuth(playAuth)
}
func (x *ahuCourse) _request_aliyun_play_info(playAuth, videoID string) (*shared.AliyunPlayInfo, error) {
	return x.requestAliyunPlayInfo(playAuth, videoID)
}
func (x *ahuCourse) _request_aliyun_play_info_by_rand(playAuth, videoID, randValue string) (*shared.AliyunPlayInfo, error) {
	return x.requestAliyunPlayInfoByRand(playAuth, videoID, randValue)
}
func (x *ahuCourse) _request_aliyun_play_info_legacy(playAuth, videoID string) (*shared.AliyunPlayInfo, error) {
	return x.requestAliyunPlayInfoLegacy(playAuth, videoID)
}
func (x *ahuCourse) _extract_aliyun_key_material(keyToken string) (string, string) {
	return x.extractAliyunKeyMaterial(keyToken)
}
func (x *ahuCourse) _request_aliyun_license(payload shared.AliyunPlayPayload, mediaID, challenge, encType string) ([]byte, error) {
	return x.requestAliyunLicense(payload, mediaID, challenge, encType)
}
func (x *ahuCourse) _build_aliyun_key_func(payload shared.AliyunPlayPayload, encType string) func(string) ([]byte, error) {
	return x.buildAliyunKeyFunc(payload, encType)
}
func (x *ahuCourse) _download_baijiayun_playback(token, playID, lessonName, lessonDir string) bool {
	return x.downloadBaijiayunPlayback(token, playID, lessonName, lessonDir)
}
func (x *ahuCourse) _find_file_target_outline_node(fileTitle string) *ahuTreeNode {
	return x.findFileTargetOutlineNode(fileTitle)
}
func (x *ahuCourse) _ensure_file_path_node(nodes *[]*ahuTreeNode, outline *ahuTreeNode) *ahuTreeNode {
	return x.ensureFilePathNode(nodes, outline)
}
func (x *ahuCourse) _ensure_named_file_node(nodes *[]*ahuTreeNode, dirTitle, rawTitle string, index []int) *ahuTreeNode {
	return x.ensureNamedFileNode(nodes, dirTitle, rawTitle, index)
}
func (x *ahuCourse) _append_file_to_nodes(nodes *[]*ahuTreeNode, fileTitle, fileURL string, seen map[string]bool) {
	x.appendFileToNodes(nodes, fileTitle, fileURL, seen)
}
func (x *ahuCourse) _ensure_default_chapter(nodes *[]*ahuTreeNode) *ahuTreeNode {
	return x.ensureDefaultChapter(nodes)
}
func (x *ahuCourse) _download_video(videoInfo ahuSource, dirname string) bool {
	return x.downloadVideo(dirname, videoInfo)
}
func (x *ahuCourse) _download_video_list(dirname string, videoList []ahuSource, isTry bool) bool {
	return x.downloadVideoList(dirname, videoList, isTry)
}
func (x *ahuCourse) _download_one_file(fileInfo ahuSource, dirname string) bool {
	return x.downloadOneFile(dirname, fileInfo)
}
func (x *ahuCourse) _download_file_list(dirname string, fileList []ahuSource, isTry bool) bool {
	return x.downloadFileList(dirname, fileList, isTry)
}
func (x *ahuCourse) _should_stop_after_trial_success() bool { return x.shouldStopAfterTrialSuccess() }
func (x *ahuCourse) _split_download_info(info any) ([]ahuSource, []ahuSource) {
	return splitDownloadInfo(info)
}
func (x *ahuCourse) _download_source_tree(dirname string, info any, indent string, downloadVideos, downloadFiles bool) bool {
	return x.downloadSourceTree(dirname, info, indent, downloadVideos, downloadFiles)
}
func (x *ahuCourse) _download_course(courseDir string) bool { return x.downloadCourse(courseDir) }
func (x *ahuCourse) _download_files(filesDir string) bool   { return x.downloadFiles(filesDir) }
func (x *ahuCourse) _download(opts ...AhuDownloadOptions) ([]string, error) {
	return x.download(opts...)
}
func _match_text_variants(text string) []string { return matchTextVariants(text) }
func _best_variant_match_len(needle, haystack []string) int {
	return bestVariantMatchLen(needle, haystack)
}
func _build_file_info(fileTitle, fileURL string, index []int) ahuSource {
	return buildFileInfo(fileTitle, fileURL, index)
}
func _tree_node(dirTitle, rawTitle string, index []int) *ahuTreeNode {
	return treeNode(dirTitle, rawTitle, index)
}
func _node_has_sources(node *ahuTreeNode) bool { return nodeHasSources(node) }
func _tree_to_download_info(node *ahuTreeNode) any {
	return treeToDownloadInfo(node)
}
func _trees_to_download_info(nodes []*ahuTreeNode) ahuDownloadInfo {
	return treeMapToDownloadInfo(nodes)
}
func _sort_tree_nodes(nodes []*ahuTreeNode) []*ahuTreeNode { return sortTreeNodes(nodes) }
func _normalize_match_text(text string) string             { return normalizeMatchText(text) }
func _extract_js_array_assignments(htmlText, varName string) []string {
	return extractJSONArrayAssignments(htmlText, varName)
}
func (x *ahuCourse) _parse_course_videos(htmlText string, baseURL ...string) ahuDownloadInfo {
	base := ""
	if len(baseURL) > 0 {
		base = baseURL[0]
	}
	return treeMapToDownloadInfo(x.parseCourseVideos(htmlText, base))
}
func (x *ahuCourse) _parse_course_files(htmlText string, baseURL ...string) ahuDownloadInfo {
	base := ""
	if len(baseURL) > 0 {
		base = baseURL[0]
	}
	return treeMapToDownloadInfo(x.parseCourseFilesTree(htmlText, base))
}
func _parse_handouts_from_scripts(htmlText string, baseURL ...string) []resourceRef {
	base := ""
	if len(baseURL) > 0 {
		base = baseURL[0]
	}
	var out []resourceRef
	seen := map[string]bool{}
	for _, raw := range extractJSONArrayAssignments(htmlText, "handoutsList") {
		var payload any
		if jsonUnmarshal(raw, &payload) != nil {
			continue
		}
		for _, ref := range resourceRefsFromAny(payload, base) {
			if ref.URL == "" || seen[ref.URL] {
				continue
			}
			seen[ref.URL] = true
			out = append(out, ref)
		}
	}
	return out
}
func (x *ahuCourse) set_mode(mode ...int) bool {
	if len(mode) > 0 {
		x.mode = mode[0]
	}
	if x.mode == 0 {
		x.mode = ahuModeFHD
	}
	return true
}
