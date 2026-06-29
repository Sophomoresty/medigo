package lexueyun

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func decodeLiveData(playURL string) map[string]any {
	u, err := url.Parse(playURL)
	if err != nil {
		return nil
	}
	raw := u.Query().Get("liveData")
	if raw == "" && strings.Contains(u.Fragment, "?") {
		q, _ := url.ParseQuery(strings.SplitN(u.Fragment, "?", 2)[1])
		raw = q.Get("liveData")
	}
	if raw == "" {
		return nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) == nil {
		return m
	}
	if dec, err := url.QueryUnescape(raw); err == nil {
		_ = json.Unmarshal([]byte(dec), &m)
	}
	return m
}

var coursePathRe = regexp.MustCompile(`(?:subject|course)/(\d+)`)

func parseCourse(raw string) courseSel {
	var sel courseSel
	if u, err := url.Parse(strings.TrimSpace(raw)); err == nil {
		q := u.Query()
		sel.subjectID = firstNonEmpty(q.Get("cid"), q.Get("subjectId"), q.Get("subject_id"))
		sel.ordSerialNo = firstNonEmpty(q.Get("ordSerialNo"), q.Get("orderSerialNo"), q.Get("ordNo"))
		sel.orderID = firstNonEmpty(q.Get("orderId"), q.Get("order_id"))
		if sel.subjectID == "" {
			if m := coursePathRe.FindStringSubmatch(u.Path); len(m) > 1 {
				sel.subjectID = m[1]
			}
		}
	}
	return sel
}

func userAuthFromJar(jar http.CookieJar) string {
	for _, raw := range []string{urlOrigin, urlReferer} {
		if u, err := url.Parse(raw); err == nil {
			for _, c := range jar.Cookies(u) {
				if c.Name == "lexueyun-pc-userAuth" || c.Name == "userAuth" || c.Name == "token" {
					return strings.TrimSpace(c.Value)
				}
			}
		}
	}
	return ""
}
func lexueHeaders(auth string) map[string]string {
	return map[string]string{"X-Requested-With": "XMLHttpRequest", "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8", "Accept": "application/json, text/plain, */*", "Origin": urlOrigin, "Referer": urlReferer, "userAuth": auth}
}
func liveType(les lesson) int {
	if toInt(les.LiveStatus) == 5 {
		return 2
	}
	if toInt(les.IsNewLive) == 1 {
		return 3
	}
	return 1
}

func lessonRoomID(les lesson) string {
	if status := toInt(les.LiveStatus); status == 1 || status == 2 {
		return firstNonEmpty(anyString(les.LiveLessonID), anyString(les.LivePlaybackID))
	}
	return firstNonEmpty(anyString(les.LivePlaybackID), anyString(les.LiveLessonID))
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "/") {
		return urlOrigin + raw
	}
	return raw
}
func extractList(v any, keys []string) []map[string]any {
	if arr, ok := v.([]any); ok {
		out := []map[string]any{}
		for _, it := range arr {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	if m, ok := v.(map[string]any); ok {
		for _, k := range keys {
			if out := extractList(m[k], keys); len(out) > 0 {
				return out
			}
		}
		for _, k := range keys {
			if _, ok := m[k]; ok {
				return []map[string]any{m}
			}
		}
	}
	return nil
}
func anyString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func toInt(v any) int {
	var f float64
	switch x := v.(type) {
	case int:
		return x
	case float64:
		f = x
	case string:
		fmt.Sscanf(x, "%f", &f)
	}
	return int(f)
}
func toFloat(v any) float64 {
	var f float64
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case string:
		fmt.Sscanf(x, "%f", &f)
	}
	return f
}
func pickFormat(u string) string {
	if strings.Contains(strings.ToLower(u), ".m3u8") {
		return "m3u8"
	}
	return "mp4"
}

var lexuePriceNumberRe = regexp.MustCompile(`\d+(?:\.\d+)?`)

func normalizePrice(v any) float64 {
	if v == nil {
		return 0
	}
	text := strings.TrimSpace(fmt.Sprint(v))
	if text == "" || text == "<nil>" {
		return 0
	}
	text = strings.NewReplacer(",", "", "¥", "", "￥", "", "元", "").Replace(text)
	if match := lexuePriceNumberRe.FindString(text); match != "" {
		text = match
	}
	price, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0
	}
	if price >= 1000 && price == float64(int64(price)) {
		price /= 100
	}
	if price < 0 {
		return 0
	}
	return price
}

func extractPrice(v any) float64 {
	switch m := v.(type) {
	case map[string]any:
		for _, key := range []string{"price", "salePrice", "payPrice", "realPrice", "originPrice", "originalPrice", "amount", "orderPrice", "totalPrice"} {
			if p := normalizePrice(m[key]); p > 0 {
				return p
			}
		}
		for _, child := range m {
			if p := extractPrice(child); p > 0 {
				return p
			}
		}
	case []any:
		for _, child := range m {
			if p := extractPrice(child); p > 0 {
				return p
			}
		}
	}
	return 0
}
