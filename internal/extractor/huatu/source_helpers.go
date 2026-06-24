package huatu

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	fileURLKeys  = []string{"downloadUrl", "downloadURL", "fileUrl", "fileURL", "resourceUrl", "resourceURL", "attachUrl", "attachmentUrl", "materialUrl", "handoutUrl", "pdfUrl", "pptUrl", "docUrl", "url"}
	fileListKeys = []string{"fileList", "files", "attachments", "attachList", "materials", "materialList", "handouts", "resourceList", "downloadList", "data"}
	fileNodeKeys = []string{"file", "attachment", "resource", "resourceInfo", "fileInfo", "material", "materialInfo", "handout", "downloadInfo"}
	priceKeys    = []string{"actualPrice", "discountPrice", "salePrice", "goodsPrice", "originPrice", "originalPrice", "price", "money", "cashPrice", "realPrice", "payPrice"}
)

func itemTitle(item map[string]any, def string) string {
	for _, k := range []string{"title", "name", "lessonName", "classTitle"} {
		if s := str(item[k]); s != "" {
			return s
		}
	}
	return def
}

func itemFileURL(item map[string]any) string {
	if len(item) == 0 {
		return ""
	}
	for _, k := range fileURLKeys {
		if s := str(item[k]); strings.HasPrefix(s, "http") {
			return s
		}
	}
	for _, k := range fileNodeKeys {
		if s := itemFileURL(asMap(item[k])); s != "" {
			return s
		}
	}
	return ""
}

func iterFileNodes(item map[string]any, depth int) []map[string]any {
	if depth > 2 || len(item) == 0 {
		return nil
	}
	var out []map[string]any
	if itemFileURL(item) != "" {
		out = append(out, item)
	}
	for _, k := range fileListKeys {
		for _, child := range listMaps(item[k]) {
			out = append(out, iterFileNodes(child, depth+1)...)
		}
		child := asMap(item[k])
		if len(child) > 0 {
			out = append(out, iterFileNodes(child, depth+1)...)
		}
	}
	for _, k := range fileNodeKeys {
		child := asMap(item[k])
		if len(child) > 0 {
			out = append(out, iterFileNodes(child, depth+1)...)
		}
	}
	return out
}

func lessonID(item map[string]any) string {
	if len(item) == 0 {
		return ""
	}
	if s := firstNonEmpty(str(item["lessonId"]), str(item["clazzLessonId"])); s != "" {
		return s
	}
	level := str(item["level"])
	if level == "3" && (str(item["videoId"]) != "" || str(item["modularId"]) != "") {
		return firstNonEmpty(str(item["id"]), str(item["videoId"]), str(item["modularId"]))
	}
	return ""
}

func detectFileFormat(fileName, fileURL string, item map[string]any) string {
	for _, s := range []string{fileName, fileURL} {
		if m := regexp.MustCompile(`\.([A-Za-z0-9]{1,8})(?:$|[?#])`).FindStringSubmatch(s); len(m) > 1 {
			return strings.ToLower(m[1])
		}
	}
	for _, k := range fileURLKeys {
		if str(item[k]) == "" {
			continue
		}
		kl := strings.ToLower(k)
		switch {
		case strings.HasPrefix(kl, "pdf"):
			return "pdf"
		case strings.HasPrefix(kl, "ppt"):
			return "ppt"
		case strings.HasPrefix(kl, "doc"):
			return "doc"
		}
	}
	return "bin"
}

func extractPrice(data any, depth int) float64 {
	if depth > 2 || data == nil {
		return 0
	}
	var vals []float64
	m := asMap(data)
	if len(m) > 0 {
		for _, k := range priceKeys {
			if v, ok := m[k]; ok {
				if p := normalizePrice(v, k); p > 0 {
					vals = append(vals, p)
				}
			}
		}
		for _, k := range []string{"priceInfo", "priceData", "goods", "goodsInfo", "course", "courseInfo"} {
			if p := extractPrice(m[k], depth+1); p > 0 {
				vals = append(vals, p)
			}
		}
	} else {
		for _, child := range listMaps(data) {
			if p := extractPrice(child, depth+1); p > 0 {
				vals = append(vals, p)
			}
		}
	}
	if len(vals) == 0 {
		return 0
	}
	best := vals[0]
	for _, v := range vals[1:] {
		if v < best {
			best = v
		}
	}
	return best
}

func normalizePrice(v any, key string) float64 {
	if strings.Contains(strings.ToLower(key), "bean") || strings.Contains(strings.ToLower(key), "score") {
		return 0
	}
	var f float64
	switch t := v.(type) {
	case bool:
		return 0
	case float64:
		f = t
	case int:
		f = float64(t)
	case string:
		s := strings.TrimSpace(t)
		if s == "" || strings.Contains(s, "免费") {
			return 0
		}
		m := regexp.MustCompile(`(\d+(?:\.\d+)?)`).FindStringSubmatch(strings.ReplaceAll(s, ",", ""))
		if len(m) == 0 {
			return 0
		}
		f, _ = strconv.ParseFloat(m[1], 64)
	default:
		return 0
	}
	if f <= 0 {
		return 0
	}
	if f > 1000 && float64(int64(f)) == f {
		f /= 100
	}
	return f
}

func isExpiredCourse(item map[string]any, depth int) bool {
	if len(item) == 0 || depth > 2 {
		return false
	}
	for _, k := range []string{"isExpired", "expired", "hasExpired", "isHidden"} {
		s := strings.ToLower(str(item[k]))
		if s == "1" || s == "true" || s == "yes" || s == "y" {
			return true
		}
	}
	for _, k := range []string{"status", "goodsStatus", "courseStatus", "learnStatus", "viewStatus"} {
		s := strings.ToLower(str(item[k]))
		switch s {
		case "2", "3", "expired", "expire", "ended", "closed", "invalid":
			return true
		}
	}
	for _, k := range []string{"expireTime", "expiredTime", "expireAt", "expireEndTime", "invalidTime", "endTime", "endAt", "deadline", "deadlineTime", "validEndTime", "validityEndTime", "learnEndTime", "saleEndTime"} {
		if ts := atoi(str(item[k])); ts > 946684800 {
			if ts > 1e12 {
				ts /= 1000
			}
			if int64(ts) <= time.Now().Unix() {
				return true
			}
		}
	}
	for _, k := range []string{"statusInfo", "stateInfo", "goodsStatusInfo", "courseStatusInfo", "examStatusInfo", "priceStatusInfo", "ext", "extra", "meta", "saleInfo", "timeInfo", "courseInfo", "goodsInfo"} {
		if isExpiredCourse(asMap(item[k]), depth+1) {
			return true
		}
	}
	return false
}

func appendToken(rawURL, token string) string {
	if rawURL == "" || token == "" || strings.Contains(rawURL, "token=") {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "token=" + quoteHuatuToken(token)
}

func quoteHuatuFileURL(raw string) string {
	if raw == "" {
		return ""
	}
	return quoteHuatuBytes(raw, isHuatuFileURLSafeByte)
}

func quoteHuatuToken(raw string) string {
	if raw == "" {
		return ""
	}
	return quoteHuatuBytes(raw, isHuatuTokenSafeByte)
}

func quoteHuatuBytes(raw string, safe func(byte) bool) string {
	var b strings.Builder
	b.Grow(len(raw) * 3)
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if safe(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte("0123456789ABCDEF"[c>>4])
		b.WriteByte("0123456789ABCDEF"[c&0x0f])
	}
	return b.String()
}

func isHuatuFileURLSafeByte(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '-', '_', '.', '~', ':', '/', '?', '=', '&', '%':
		return true
	default:
		return false
	}
}

func isHuatuTokenSafeByte(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '-' || c == '_' || c == '.' || c == '~':
		return true
	default:
		return false
	}
}
