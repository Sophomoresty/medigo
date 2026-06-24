package huatu

import (
	"fmt"
	"strings"
)

type sourceItem struct {
	Kind     string
	Name     string
	URL      string
	Format   string
	LessonID string
	Raw      map[string]any
}

func (x *huatuCtx) myCourseParams(page, pageSize int) map[string]string {
	return map[string]string{
		"isHidden":      "0",
		"teacherId":     "",
		"priceStatus":   "",
		"examStatus":    "",
		"keyword":       "",
		"recentlyStudy": "0",
		"goodsType":     "1",
		"page":          fmt.Sprint(page),
		"pageSize":      fmt.Sprint(pageSize),
	}
}

func (x *huatuCtx) firstCourse() (map[string]any, error) {
	courses, err := x.courseList(false)
	if err != nil || len(courses) == 0 {
		return nil, err
	}
	return courses[0], nil
}

func (x *huatuCtx) findCourse(cid string) (map[string]any, error) {
	courses, err := x.courseList(true)
	if err != nil {
		return nil, err
	}
	for _, course := range courses {
		if str(course["course_id"]) == cid {
			return course, nil
		}
	}
	return nil, nil
}

func (x *huatuCtx) courseList(includeExpired bool) ([]map[string]any, error) {
	var out []map[string]any
	seen := map[string]bool{}
	pageSize := 100
	for page := 1; page <= 20; page++ {
		var resp apiResp
		root, err := x.getJSON(my_course_url, x.myCourseParams(page, pageSize), nil, &resp)
		if err != nil {
			return nil, err
		}
		if !successCode(root["code"]) {
			break
		}
		items, meta := dataPayload(root)
		if len(items) == 0 {
			break
		}
		for _, raw := range items {
			cid := firstNonEmpty(str(raw["goodsNum"]), str(raw["goodsNo"]), str(raw["goodsId"]), str(raw["courseId"]))
			if cid == "" || seen[cid] {
				continue
			}
			seen[cid] = true
			course := map[string]any{
				"raw":       raw,
				"course_id": cid,
				"title":     cleanName(firstNonEmpty(str(raw["title"]), str(raw["name"]), str(raw["goodsName"]), cid)),
				"price":     extractPrice(raw, 0),
				"expired":   isExpiredCourse(raw, 0),
			}
			if includeExpired || !course["expired"].(bool) {
				out = append(out, course)
			}
		}
		pc := pageCount(meta)
		if pc > 0 && page >= pc {
			break
		}
		if len(items) < pageSize && pc == 0 {
			break
		}
	}
	return out, nil
}

func (x *huatuCtx) applyCourse(course map[string]any) {
	if course == nil {
		return
	}
	x.cid = firstNonEmpty(str(course["course_id"]), x.cid)
	if x.title == "" {
		x.title = cleanName(firstNonEmpty(str(course["title"]), x.cid))
	}
}

func dataPayload(root map[string]any) ([]map[string]any, map[string]any) {
	data := root["data"]
	if arr := listMaps(data); len(arr) > 0 {
		return arr, map[string]any{}
	}
	m := asMap(data)
	if len(m) == 0 {
		return nil, nil
	}
	if arr := listMaps(m["data"]); len(arr) > 0 {
		return arr, m
	}
	if arr := listMaps(m["list"]); len(arr) > 0 {
		return arr, m
	}
	return nil, m
}

func (x *huatuCtx) syllabusItems(level int, extra map[string]string) ([]map[string]any, error) {
	var out []map[string]any
	for page := 1; page <= 20; page++ {
		params := map[string]string{"goodsNum": x.cid, "level": fmt.Sprint(level), "page": fmt.Sprint(page)}
		for k, v := range extra {
			if v != "" {
				params[k] = v
			}
		}
		root, err := x.getJSON(syllabus_url, params, nil, nil)
		if err != nil {
			return nil, err
		}
		if !successCode(root["code"]) {
			break
		}
		items, meta := dataPayload(root)
		if len(items) == 0 {
			break
		}
		out = append(out, items...)
		pc := pageCount(meta)
		if pc > 0 && page >= pc {
			break
		}
		if len(items) == 0 && pc == 0 {
			break
		}
	}
	return out, nil
}

func (x *huatuCtx) collectItems() ([]sourceItem, error) {
	if x.cid == "" {
		return nil, fmt.Errorf("huatu: empty course id")
	}
	chapters, err := x.syllabusItems(1, nil)
	if err != nil {
		return nil, err
	}
	var out []sourceItem
	seenVideo := map[string]bool{}
	seenFile := map[string]bool{}
	for ci, ch := range chapters {
		chapterIndex := ci + 1
		x.appendItemSources(ch, chapterIndex, &out, seenVideo, seenFile)
		stageID := str(ch["id"])
		level2, err := x.syllabusItems(2, map[string]string{"stageId": stageID})
		if err != nil {
			return nil, err
		}
		if len(level2) == 0 {
			level2 = []map[string]any{ch}
		}
		for _, module := range level2 {
			x.appendItemSources(module, chapterIndex, &out, seenVideo, seenFile)
			modularID := firstNonEmpty(str(module["modularId"]), str(module["id"]))
			level3, err := x.syllabusItems(3, map[string]string{"modularId": modularID})
			if err != nil {
				return nil, err
			}
			if len(level3) == 0 && (str(module["level"]) == "3" || lessonID(module) != "") {
				level3 = []map[string]any{module}
			}
			for _, lesson := range level3 {
				x.appendItemSources(lesson, chapterIndex, &out, seenVideo, seenFile)
			}
		}
	}
	if len(out) == 0 && x.target.LessonID != "" {
		out = append(out, sourceItem{Kind: "video", Name: cleanName(x.title), LessonID: x.target.LessonID})
	}
	return out, nil
}

func (x *huatuCtx) appendItemSources(item map[string]any, chapterIndex int, out *[]sourceItem, seenVideo, seenFile map[string]bool) {
	if v := buildVideoInfo(item, chapterIndex, len(*out)+1); v != nil {
		id := v.LessonID
		if id != "" && !seenVideo[id] {
			seenVideo[id] = true
			*out = append(*out, *v)
		}
	}
	for _, node := range iterFileNodes(item, 0) {
		file := buildFileInfo(node, chapterIndex, len(*out)+1)
		if file == nil || file.URL == "" || seenFile[file.URL] {
			continue
		}
		seenFile[file.URL] = true
		*out = append(*out, *file)
	}
}

func buildVideoInfo(item map[string]any, chapterIndex, index int) *sourceItem {
	id := lessonID(item)
	if id == "" {
		return nil
	}
	title := itemTitle(item, "未命名视频")
	return &sourceItem{Kind: "video", Name: cleanName(fmt.Sprintf("[%d.%d]--%s", chapterIndex, index, title)), LessonID: id, Raw: item}
}

func buildFileInfo(item map[string]any, chapterIndex, index int) *sourceItem {
	fileURL := itemFileURL(item)
	if fileURL == "" {
		return nil
	}
	fileName := "未命名资料"
	for _, k := range []string{"title", "name", "fileName", "filename", "resourceName"} {
		if s := str(item[k]); s != "" {
			fileName = s
			break
		}
	}
	fmtName := detectFileFormat(fileName, fileURL, item)
	if strings.HasSuffix(strings.ToLower(fileName), "."+fmtName) {
		fileName = fileName[:len(fileName)-len(fmtName)-1]
	}
	return &sourceItem{Kind: "file", Name: cleanName(fmt.Sprintf("(%d.%d)-%s", chapterIndex, index, fileName)), URL: quoteHuatuFileURL(fileURL), Format: fmtName, Raw: item}
}
