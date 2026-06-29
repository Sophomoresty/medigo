package cto51

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
	"github.com/Sophomoresty/mediago/internal/util"
)

type courseRef struct {
	ID         string
	Title      string
	URL        string
	TrainID    string
	IsTraining bool
	Price      string
	Raw        map[string]any
}

type fileRef struct {
	URL          string
	Title        string
	Format       string
	LessonID     string
	ChapterTitle string
	Scope        string
	Size         int64
	Raw          map[string]any
}

type lessonContext struct {
	CourseID      string
	TrainID       string
	TrainCourseID string
	ChapterTitle  string
	SourceKind    string
}

func fetchCoursePayloads(c *util.Client, cid string, h map[string]string) []any {
	var payloads []any
	payloads = append(payloads, fetchJSONPayloads(c, h, []apiReq{
		{urlCourseIndexAPI, map[string]string{"course_id": cid, "course_id_str": cid, "id": cid}},
	})...)
	payloads = append(payloads, fetchPagedJSONPayloads(c, h, urlLessonListAPI, map[string]string{"id": cid}, 50, 0)...)
	payloads = append(payloads, fetchPagedJSONPayloads(c, h, urlLessonFileListAPI, map[string]string{"course_id": cid, "size": "100"}, 100, 100)...)
	payloads = append(payloads, fetchPagedJSONPayloads(c, h, urlCourseFileListAPI, map[string]string{"course_id": cid, "id": cid, "size": "100"}, 100, 100)...)
	payloads = append(payloads, fetchPagedJSONPayloads(c, h, urlMaterialListAPI, map[string]string{"course_id": cid, "id": cid, "size": "100"}, 100, 100)...)
	return payloads
}

func fetchTrainingPayloads(c *util.Client, r route, h map[string]string) []any {
	payloads := fetchJSONPayloads(c, h, []apiReq{
		{urlTrainStageAPI, map[string]string{"train_id": r.TrainID}},
		{urlTrainCourseAPI, map[string]string{"train_id": r.TrainID}},
		{urlTrainLiveAPI, map[string]string{"train_id": r.TrainID}},
		{urlTrainNextAPI, map[string]string{"train_id": r.TrainID}},
	})
	payloads = append(payloads, fetchPagedJSONPayloads(c, h, urlTrainFileAPI, map[string]string{"train_id": r.TrainID, "size": "100"}, 100, 100)...)
	trainCourseIDs := uniqueStrings([]string{r.TrainCourseID})
	for _, m := range walkMaps(payloads) {
		trainCourseIDs = append(trainCourseIDs, textValue(m, "train_course_id", "trainCourseId", "trainCourseID"))
		if u := textValue(m, "lesson_url", "lessonUrl", "play_url", "playUrl", "url"); u != "" {
			trainCourseIDs = append(trainCourseIDs, trainCourseIDFromURL(u))
		}
	}
	for _, tcid := range uniqueStrings(trainCourseIDs) {
		if tcid == "" {
			continue
		}
		payloads = append(payloads, fetchPagedJSONPayloads(c, h, urlTrainInfoAPI, map[string]string{"train_id": r.TrainID, "train_course_id": tcid, "size": "100"}, 100, 100)...)
	}
	if len(trainCourseIDs) == 0 || r.TrainCourseID == "" {
		payloads = append(payloads, fetchJSONPayloads(c, h, []apiReq{{urlTrainInfoAPI, map[string]string{"train_id": r.TrainID}}})...)
	}
	return payloads
}

func fetchMyCoursePayloads(c *util.Client, h map[string]string) []any {
	var payloads []any
	payloads = append(payloads, fetchJSONPayloads(c, h, []apiReq{{urlStudyCourse, nil}, {urlCourseTypeAPI, nil}})...)
	typeIDs := extractCourseTypeIDs(payloads)
	var variants []map[string]string
	variants = append(variants, map[string]string{})
	for _, id := range typeIDs {
		variants = append(variants, map[string]string{"type": id})
	}
	for _, id := range []string{"0", "1", "2"} {
		variants = append(variants, map[string]string{"type": id})
	}
	seenVariant := map[string]bool{}
	for _, params := range variants {
		key := fmt.Sprint(params)
		if seenVariant[key] {
			continue
		}
		seenVariant[key] = true
		params["pageSize"] = "100"
		payloads = append(payloads, fetchPagedJSONPayloads(c, h, urlCourseListAPI, params, 50, 100)...)
	}
	payloads = append(payloads, fetchJSONPayloads(c, h, []apiReq{{urlTrainingAPI, map[string]string{"method": "study.index", "type": "1"}}})...)
	payloads = append(payloads, fetchPagedJSONPayloads(c, h, urlOrderListAPI, map[string]string{"pageSize": "100"}, 20, 100)...)
	payloads = append(payloads, fetchPagedJSONPayloads(c, h, urlOrderListLegacyAPI, map[string]string{"pageSize": "100", "status": "3"}, 20, 100)...)
	return payloads
}

func fetchPagedJSONPayloads(c *util.Client, h map[string]string, api string, base map[string]string, maxPages, pageSize int) []any {
	if maxPages <= 0 {
		maxPages = 1
	}
	var out []any
	seenPage := map[string]bool{}
	for page := 1; page <= maxPages; page++ {
		params := cloneStringMap(base)
		params["page"] = fmt.Sprint(page)
		if pageSize > 0 && params["size"] == "" && params["pageSize"] == "" {
			params["size"] = fmt.Sprint(pageSize)
		}
		body, err := c.GetString(addQuery(api, params), h)
		if err != nil {
			if page == 1 {
				continue
			}
			break
		}
		var payload any
		if jsonErr := decodeJSON(body, &payload); jsonErr != nil {
			if page == 1 {
				out = append(out, body)
			}
			break
		}
		items := pageItems(payload)
		sig := pageSignature(items)
		if page > 1 && sig != "" && seenPage[sig] {
			break
		}
		if sig != "" {
			seenPage[sig] = true
		}
		if page > 1 && len(items) == 0 {
			break
		}
		out = append(out, payload)
		total := deepIntValue(payload, "totalPage", "total_page", "pageCount", "page_count", "countPage", "count_page")
		if total > 0 && page >= total {
			break
		}
		if page > 1 && len(items) == 0 {
			break
		}
	}
	return out
}

func courseRefsFromPayloads(payloads []any) []courseRef {
	var out []courseRef
	seen := map[string]bool{}
	for _, m := range walkMaps(payloads) {
		ref := courseRefFromMap(m)
		if ref.ID == "" && ref.TrainID == "" {
			continue
		}
		key := "course:" + ref.ID
		if ref.IsTraining {
			key = "train:" + ref.TrainID
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ref)
	}
	return out
}

func courseRefsFromHTML(text string) []courseRef {
	seen := map[string]bool{}
	var out []courseRef
	for _, m := range fileAnchorRe.FindAllStringSubmatch(text, -1) {
		rawURL := normalizeURL(m[1], "https://edu.51cto.com/")
		if rawURL == "" || looksLikeFileDownloadURL(rawURL) {
			continue
		}
		title := cleanText(m[2])
		if title == "" {
			continue
		}
		if trainID := trainIDFromURL(rawURL); trainID != "" {
			key := "train:" + trainID
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, courseRef{ID: "train_" + trainID, TrainID: trainID, IsTraining: true, Title: title, URL: rawURL, Price: defaultCoursePrice})
			continue
		}
		cid := courseIDFromURL(rawURL)
		if cid == "" {
			continue
		}
		key := "course:" + cid
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, courseRef{ID: cid, Title: title, URL: rawURL, Price: defaultCoursePrice})
	}
	return out
}

func courseRefFromMap(m map[string]any) courseRef {
	rawURL := normalizeURL(textValue(m, "course_url", "courseUrl", "detail_url", "detailUrl", "jump_url", "jumpUrl", "study_url", "studyUrl", "url"), "https://edu.51cto.com/")
	trainID := firstNonEmpty(textValue(m, "train_id", "trainId", "training_id", "trainingId"), trainIDFromURL(rawURL))
	if trainID != "" {
		title := textValue(m, "name", "title", "train_name", "trainName", "training_name", "trainingName", "good_title", "goodTitle", "course_name", "courseName")
		if title == "" {
			return courseRef{}
		}
		price := firstNonEmpty(normalizePriceValue(textValue(m, "price", "train_price", "trainPrice", "sale_price", "salePrice", "pay_price", "payPrice", "original_price", "originalPrice")), defaultCoursePrice)
		return courseRef{ID: "train_" + trainID, TrainID: trainID, IsTraining: true, Title: cleanText(title), URL: firstNonEmpty(rawURL, fmt.Sprintf(urlWejobCourse, trainID)), Price: price, Raw: m}
	}
	cid := firstNonEmpty(courseIDFromURL(rawURL), textValue(m, "course_id", "courseId"))
	if cid == "" {
		if id := textValue(m, "id"); id != "" && (textValue(m, "course_name", "courseName") != "" || strings.Contains(rawURL, "/course/")) {
			cid = id
		}
	}
	if cid == "" || textValue(m, "lesson_id", "lessonId") != "" || looksLikeFileMap(m) {
		return courseRef{}
	}
	title := textValue(m, "course_name", "courseName", "title", "name", "good_title", "goodTitle")
	if title == "" {
		return courseRef{}
	}
	price := normalizePriceOrDefault(textValue(m, "price", "course_price", "coursePrice", "pay_price", "payPrice"))
	return courseRef{ID: cid, Title: cleanText(title), URL: firstNonEmpty(rawURL, fmt.Sprintf(urlCourse, cid)), Price: price, Raw: m}
}

func courseRefEntries(courses []courseRef) []*extractor.MediaInfo {
	entries := make([]*extractor.MediaInfo, 0, len(courses))
	for _, c := range courses {
		title := util.SanitizeFilename(firstNonEmpty(c.Title, c.ID, c.TrainID, "51cto"))
		extra := map[string]any{"url": c.URL, "raw": c.Raw}
		if c.IsTraining {
			extra["train_id"] = c.TrainID
			extra["course_id"] = "train_" + c.TrainID
			extra["type"] = "training"
		} else {
			extra["course_id"] = c.ID
			extra["type"] = "course"
		}
		extra["price"] = normalizePriceOrDefault(c.Price)
		entries = append(entries, &extractor.MediaInfo{Site: "cto51", Title: title, Extra: extra})
	}
	return entries
}

func lessonsFromPayloads(payloads []any, ctx lessonContext) []lessonRef {
	var out []lessonRef
	for _, payload := range payloads {
		out = append(out, lessonsFromAny(payload, ctx)...)
	}
	return out
}

func lessonsFromAny(v any, ctx lessonContext) []lessonRef {
	var out []lessonRef
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			out = append(out, lessonsFromAny(item, ctx)...)
		}
	case map[string]any:
		next := ctx
		if chapter := textValue(x, "chapter_name", "chapterName", "section_name", "sectionName", "catalog_name", "catalogName", "dir_name", "dirName", "stage_title", "stageTitle"); chapter != "" {
			next.ChapterTitle = cleanText(chapter)
		} else if strings.EqualFold(textValue(x, "type", "lesson_type", "lessonType"), "chapter") {
			next.ChapterTitle = cleanText(textValue(x, "title", "name"))
		}
		if ref := lessonRefFromMap(x, next); ref.ID != "" || ref.URL != "" || ref.LiveID != "" {
			out = append(out, ref)
		}
		for _, child := range x {
			out = append(out, lessonsFromAny(child, next)...)
		}
	}
	return out
}

func lessonRefFromMap(m map[string]any, ctx lessonContext) lessonRef {
	if looksLikeFileMap(m) && !looksLikeLiveMap(m) {
		return lessonRef{}
	}
	rawURL := normalizeURL(textValue(m, "lesson_url", "lessonUrl", "play_url", "playUrl", "video_url", "videoUrl", "replay_url", "replayUrl", "playback_url", "playbackUrl", "live_url", "liveUrl", "m3u8_url", "m3u8Url", "url"), "https://edu.51cto.com/")
	id := firstNonEmpty(textValue(m, "lesson_id", "lessonId", "lessonID", "lid"), lessonIDFromURL(rawURL))
	trainCourseID := firstNonEmpty(textValue(m, "train_course_id", "trainCourseId", "trainCourseID"), ctx.TrainCourseID, trainCourseIDFromURL(rawURL))
	if id == "" {
		if rawID := textValue(m, "id"); strings.Contains(rawID, "_") {
			parts := strings.SplitN(rawID, "_", 2)
			trainCourseID = firstNonEmpty(trainCourseID, parts[0])
			id = parts[1]
		} else if rawID != "" && (textValue(m, "lesson_name", "lessonName") != "" || rawURL != "" || looksLikeLiveMap(m)) {
			id = rawID
		}
	}
	title := textValue(m, "lesson_name", "lessonName", "title", "name", "video_name", "videoName", "live_name", "liveName")
	liveID := textValue(m, "live_id", "liveId", "replay_id", "replayId", "live_uuid", "liveUuid")
	sourceKind := ctx.SourceKind
	if looksLikeLiveMap(m) {
		sourceKind = "live"
		if rawURL == "" && liveID != "" {
			rawURL = fmt.Sprintf(urlTrainLiveView, liveID)
		}
	} else if trainCourseID != "" || ctx.TrainID != "" {
		sourceKind = "training"
	}
	if id == "" && rawURL == "" && liveID == "" {
		return lessonRef{}
	}
	if title == "" {
		title = firstNonEmpty(id, liveID, "课时")
	}
	return lessonRef{
		ID:            id,
		Title:         cleanText(title),
		URL:           rawURL,
		CourseID:      firstNonEmpty(textValue(m, "course_id", "courseId"), ctx.CourseID),
		TrainID:       firstNonEmpty(textValue(m, "train_id", "trainId"), ctx.TrainID, trainIDFromURL(rawURL)),
		TrainCourseID: trainCourseID,
		ChapterTitle:  ctx.ChapterTitle,
		SourceKind:    sourceKind,
		LiveID:        liveID,
		Preview:       boolValue(m, "preview", "is_preview", "isPreview", "try_see", "trySee", "is_look", "isLook"),
		Size:          int64Value(m, "size", "video_size", "videoSize", "fileSize", "file_size"),
		Raw:           m,
	}
}

func lessonEntry(c *util.Client, item lessonRef, h map[string]string, listOnly bool, index int) (*extractor.MediaInfo, error) {
	if listOnly {
		return lessonListEntry(item, index), nil
	}
	title := indexedTitle(index, firstNonEmpty(item.Title, item.ID, item.LiveID, "课时"))
	if m := videoFromText(item.URL); m.URL != "" {
		m.Title = title
		m.Size = item.Size
		return mediaInfoFromMedia(m, h), nil
	}
	if item.SourceKind == "live" || item.LiveID != "" {
		for _, pageURL := range uniqueStrings([]string{item.URL, fmt.Sprintf(urlTrainLiveView, item.LiveID), fmt.Sprintf(urlTrainLivePlay, item.LiveID)}) {
			if pageURL == "" {
				continue
			}
			if m, err := resolvePlayPage(c, pageURL, h); err == nil && m.URL != "" {
				m.Title = title
				return mediaInfoFromMedia(m, h), nil
			}
		}
	}
	if item.SourceKind == "training" || item.TrainCourseID != "" {
		for _, pageURL := range uniqueStrings([]string{item.URL, trainingLessonURL(item.TrainID, item.TrainCourseID, item.ID), fmt.Sprintf(urlTrainLessonPlay, item.ID)}) {
			if pageURL == "" {
				continue
			}
			if m, err := resolvePlayPage(c, pageURL, h); err == nil && m.URL != "" {
				m.Title = title
				return mediaInfoFromMedia(m, h), nil
			}
		}
	}
	if item.URL != "" {
		if pageURL := playPageURL(item.URL); pageURL != "" {
			if m, err := resolvePlayPage(c, pageURL, h); err == nil && m.URL != "" {
				m.Title = title
				return mediaInfoFromMedia(m, h), nil
			}
		}
	}
	if item.ID != "" {
		entry, err := resolveLesson(c, item.ID, h, false)
		if err == nil && entry != nil {
			entry.Title = util.SanitizeFilename(title)
			if entry.Extra == nil {
				entry.Extra = map[string]any{}
			}
			entry.Extra["lesson_id"] = item.ID
			if item.ChapterTitle != "" {
				entry.Extra["chapter_title"] = item.ChapterTitle
			}
			return entry, nil
		}
	}
	return nil, fmt.Errorf("51cto lesson %s: no source", firstNonEmpty(item.ID, item.URL, item.LiveID))
}

func lessonListEntry(item lessonRef, index int) *extractor.MediaInfo {
	title := indexedTitle(index, firstNonEmpty(item.Title, item.ID, item.LiveID, "课时"))
	extra := map[string]any{"type": "lesson", "raw": item.Raw}
	for k, v := range map[string]string{
		"lesson_id":       item.ID,
		"lesson_url":      item.URL,
		"course_id":       item.CourseID,
		"train_id":        item.TrainID,
		"train_course_id": item.TrainCourseID,
		"chapter_title":   item.ChapterTitle,
		"source_kind":     item.SourceKind,
		"live_id":         item.LiveID,
	} {
		if v != "" {
			extra[k] = v
		}
	}
	if item.Preview {
		extra["preview"] = true
	}
	return &extractor.MediaInfo{Site: "cto51", Title: util.SanitizeFilename(title), Extra: extra}
}

func filesFromPayloads(payloads []any, defaultScope string) []fileRef {
	var out []fileRef
	for _, payload := range payloads {
		out = append(out, filesFromAny(payload, "", defaultScope)...)
	}
	return out
}

func filesFromAny(v any, chapterTitle, defaultScope string) []fileRef {
	var out []fileRef
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			out = append(out, filesFromAny(item, chapterTitle, defaultScope)...)
		}
	case map[string]any:
		nextChapter := chapterTitle
		if chapter := textValue(x, "chapter_name", "chapterName", "section_name", "sectionName", "catalog_name", "catalogName", "dir_name", "dirName", "stage_title", "stageTitle"); chapter != "" {
			nextChapter = cleanText(chapter)
		}
		for _, f := range fileRefsFromMap(x, nextChapter, defaultScope) {
			out = append(out, f)
		}
		for _, child := range x {
			out = append(out, filesFromAny(child, nextChapter, defaultScope)...)
		}
	}
	return out
}

func fileRefsFromMap(m map[string]any, chapterTitle, defaultScope string) []fileRef {
	var out []fileRef
	scope := firstNonEmpty(textValue(m, "file_scope", "fileScope", "scope"), defaultScope, "material")
	if pack := firstNonEmpty(textValue(m, "packFileUrl", "pack_file_url"), nestedFileURL(m["packFileUrl"]), nestedFileURL(m["pack_file_url"])); pack != "" {
		out = append(out, buildFileRef(pack, firstNonEmpty(textValue(m, "packFileName", "pack_file_name"), "整课资料"), textValue(m, "packFileExt", "pack_file_ext", "file_ext", "fileExt"), "", chapterTitle, scope, m))
	}
	for _, key := range []string{"fileUrl", "file_url", "downloadUrl", "download_url", "downUrl", "attach_url", "attachUrl", "link", "href", "fileurl", "url"} {
		raw := textValue(m, key)
		if raw == "" {
			continue
		}
		fmtv := firstNonEmpty(textValue(m, "file_ext", "fileExt", "fileType", "file_type", "ext", "suffix"), mediaFormat(raw))
		if !isFileFormat(fmtv) && !looksLikeFileDownloadURL(raw) {
			continue
		}
		out = append(out, buildFileRef(raw, textValue(m, "fileName", "file_name", "attach_name", "attachName", "title", "name", "lesson_name", "lessonName"), fmtv, textValue(m, "lesson_id", "lessonId", "id"), chapterTitle, scope, m))
	}
	return out
}

func filesFromHTML(text, chapterTitle, lessonID, scope string) []fileRef {
	var out []fileRef
	seen := map[string]bool{}
	for _, m := range fileAnchorRe.FindAllStringSubmatch(text, -1) {
		u := normalizeURL(m[1], "https://edu.51cto.com/")
		if u == "" || seen[u] || !isFileFormat(mediaFormat(u)) {
			continue
		}
		seen[u] = true
		out = append(out, buildFileRef(u, cleanText(m[2]), mediaFormat(u), lessonID, chapterTitle, firstNonEmpty(scope, "material"), nil))
	}
	for _, raw := range mediaURLRe.FindAllString(text, -1) {
		u := normalizeURL(raw, "https://edu.51cto.com/")
		if u == "" || seen[u] || !isFileFormat(mediaFormat(u)) {
			continue
		}
		seen[u] = true
		out = append(out, buildFileRef(u, path.Base(parsedPath(u)), mediaFormat(u), lessonID, chapterTitle, firstNonEmpty(scope, "material"), nil))
	}
	return out
}

func buildFileRef(rawURL, title, fmtv, lessonID, chapterTitle, scope string, raw map[string]any) fileRef {
	u := normalizeURL(rawURL, "https://edu.51cto.com/")
	fmtv = strings.Trim(strings.ToLower(firstNonEmpty(fmtv, mediaFormat(u), "bin")), ". ")
	if title == "" {
		title = path.Base(parsedPath(u))
	}
	return fileRef{URL: u, Title: cleanText(firstNonEmpty(title, "资料")), Format: fmtv, LessonID: lessonID, ChapterTitle: chapterTitle, Scope: firstNonEmpty(scope, "material"), Size: int64Value(raw, "size", "fileSize", "file_size"), Raw: raw}
}

func fileEntry(c *util.Client, f fileRef, h map[string]string, index int) (*extractor.MediaInfo, error) {
	if f.URL == "" {
		return nil, fmt.Errorf("51cto file: empty url")
	}
	if looksLikeFileDownloadURL(f.URL) || !isFileFormat(f.Format) || f.Title == "" {
		meta := resolveFileDownloadMeta(c, f.URL, h)
		if meta.URL != "" {
			f.URL = meta.URL
		}
		if f.Title == "" && meta.Title != "" {
			f.Title = meta.Title
		}
		if !isFileFormat(f.Format) && meta.Format != "" {
			f.Format = meta.Format
		}
		if f.Size <= 0 && meta.Size > 0 {
			f.Size = meta.Size
		}
	}
	title := indexedFileTitle(index, firstNonEmpty(f.Title, path.Base(parsedPath(f.URL)), "资料"))
	fmtv := strings.Trim(strings.ToLower(firstNonEmpty(f.Format, mediaFormat(f.URL), "bin")), ". ")
	extra := map[string]any{"type": "file", "file_url": f.URL, "file_scope": f.Scope}
	if f.LessonID != "" {
		extra["lesson_id"] = f.LessonID
	}
	if f.ChapterTitle != "" {
		extra["chapter_title"] = f.ChapterTitle
	}
	if f.Raw != nil {
		extra["raw"] = f.Raw
	}
	return &extractor.MediaInfo{
		Site:  "cto51",
		Title: util.SanitizeFilename(title),
		Streams: map[string]extractor.Stream{"file": {
			Quality: "file",
			URLs:    []string{f.URL},
			Format:  fmtv,
			Size:    f.Size,
			Headers: h,
		}},
		Extra: extra,
	}, nil
}

func dedupeLessons(in []lessonRef) []lessonRef {
	seen := map[string]bool{}
	var out []lessonRef
	for _, item := range in {
		key := firstNonEmpty(item.SourceKind, "lesson") + ":" + firstNonEmpty(item.ID, item.URL, item.LiveID)
		if key == ":" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func dedupeFiles(in []fileRef) []fileRef {
	seen := map[string]bool{}
	var out []fileRef
	for _, f := range in {
		key := f.Scope + ":" + f.URL
		if f.URL == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

func courseTitleFromPayloads(payloads []any) string {
	for _, m := range walkMaps(payloads) {
		if s := textValue(m, "course_name", "courseName", "train_name", "trainName", "training_name", "trainingName", "title", "name"); s != "" && textValue(m, "lesson_id", "lessonId") == "" {
			return cleanText(s)
		}
	}
	return ""
}

func extractCourseTypeIDs(payloads []any) []string {
	var out []string
	for _, m := range walkMaps(payloads) {
		out = append(out, textValue(m, "type_id", "typeId"))
	}
	return uniqueStrings(out)
}

func pageItems(payload any) []any {
	for _, m := range []map[string]any{asMap(payload), asMap(asMap(payload)["data"])} {
		if len(m) == 0 {
			continue
		}
		for _, key := range []string{"lessonList", "lesson_list", "fileList", "file_list", "list", "items", "records", "rows", "data"} {
			if arr := asList(m[key]); len(arr) > 0 {
				return arr
			}
		}
	}
	if arr := asList(payload); len(arr) > 0 {
		return arr
	}
	return nil
}

func pageSignature(items []any) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		m := asMap(item)
		parts = append(parts, firstNonEmpty(textValue(m, "lesson_id", "lessonId", "file_id", "fileId", "course_id", "courseId", "id"), textValue(m, "url", "fileUrl", "file_url")))
	}
	return strings.Join(parts, "|")
}

func walkMaps(v any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(node any) {
		switch x := node.(type) {
		case []any:
			for _, item := range x {
				walk(item)
			}
		case []map[string]any:
			for _, item := range x {
				walk(item)
			}
		case map[string]any:
			out = append(out, x)
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(v)
	return out
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func asList(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case []map[string]any:
		out := make([]any, 0, len(t))
		for _, m := range t {
			out = append(out, m)
		}
		return out
	default:
		return nil
	}
}

func cloneStringMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func courseIDFromURL(raw string) string { return extractFirst(courseRe, raw) }
func trainIDFromURL(raw string) string  { return extractFirst(trainRe, raw) }

func lessonIDFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		if id := u.Query().Get("id"); strings.Contains(id, "_") {
			return strings.SplitN(id, "_", 2)[1]
		}
		if id := firstNonEmpty(u.Query().Get("lesson_id"), u.Query().Get("lessonId"), u.Query().Get("lid")); id != "" {
			return id
		}
	}
	return extractFirst(lessonRe, raw)
}

func trainCourseIDFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	if id := extractFirst(trainCourseRe, raw); id != "" {
		return id
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if id := u.Query().Get("id"); strings.Contains(id, "_") {
		return strings.SplitN(id, "_", 2)[0]
	}
	return firstNonEmpty(u.Query().Get("train_course_id"), u.Query().Get("trainCourseId"))
}

func trainingLessonURL(trainID, trainCourseID, lessonID string) string {
	if trainCourseID != "" && lessonID != "" {
		return fmt.Sprintf(urlTrainLessonPlay, trainCourseID+"_"+lessonID)
	}
	if lessonID != "" {
		return fmt.Sprintf(urlTrainLessonPlay, lessonID)
	}
	if trainID != "" {
		return fmt.Sprintf(urlWejobCourse, trainID)
	}
	return ""
}

func looksLikeFileMap(m map[string]any) bool {
	for _, key := range []string{"fileUrl", "file_url", "downloadUrl", "download_url", "downUrl", "attach_url", "attachUrl", "fileName", "file_name", "attachName", "file_ext", "fileExt", "packFileUrl", "pack_file_url"} {
		if textValue(m, key) != "" {
			return true
		}
	}
	if raw := textValue(m, "url", "href", "link"); raw != "" {
		return isFileFormat(mediaFormat(raw)) || looksLikeFileDownloadURL(raw)
	}
	return false
}

func looksLikeLiveMap(m map[string]any) bool {
	if textValue(m, "live_id", "liveId", "replay_id", "replayId", "live_uuid", "liveUuid") != "" {
		return true
	}
	t := strings.ToLower(textValue(m, "type", "lesson_type", "lessonType", "resource_type", "resourceType"))
	if t == "4" || t == "live" || t == "replay" || t == "playback" || strings.HasPrefix(t, "wejoblive") {
		return true
	}
	return textValue(m, "live_name", "liveName", "replay_url", "replayUrl", "playback_url", "playbackUrl", "live_url", "liveUrl") != ""
}

func looksLikeFileDownloadURL(raw string) bool {
	raw = strings.ToLower(normalizeText(raw))
	return strings.Contains(raw, "/download/") ||
		strings.Contains(raw, "/center/file/download/") ||
		strings.Contains(raw, "/center/course/download/") ||
		strings.Contains(raw, "/center/wejob/data/") ||
		strings.Contains(raw, "/center/wejob/index/file")
}

func indexedTitle(index int, title string) string {
	if strings.HasPrefix(title, "[") {
		return title
	}
	if index <= 0 {
		return title
	}
	return fmt.Sprintf("[%d]--%s", index, title)
}

func indexedFileTitle(index int, title string) string {
	if strings.HasPrefix(title, "(") {
		return title
	}
	if index <= 0 {
		return title
	}
	return fmt.Sprintf("(%d)--%s", index, title)
}

func parsedPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

func deepIntValue(v any, keys ...string) int {
	for _, m := range walkMaps(v) {
		if n := int64Value(m, keys...); n > 0 {
			return int(n)
		}
	}
	return 0
}
