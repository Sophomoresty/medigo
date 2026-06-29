package sanjieke

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/util"
)

func normalizeCourseItem(item courseItem) courseKey {
	price := 0.0
	for _, v := range []any{item.Price, item.CoursePrice, item.OriginPrice, item.CurrentPrice} {
		if price = normalizeSJKPrice(v); price > 0 {
			break
		}
	}
	return courseKey{
		classID:   anyString(item.ClassID),
		courseID:  firstNonEmpty(anyString(item.CourseID), anyString(item.StudyCourse)),
		projectID: firstNonEmpty(anyString(item.ProjectID), anyString(item.ProjectID2), extractProjectID(item.StudyingURL), "0"),
		productID: firstNonEmpty(anyString(item.ProductID), anyString(item.ProductID2)),
		title:     sanitizeSJKTitle(firstNonEmpty(item.Name, item.Title, item.Subtitle)),
		price:     price,
		purchased: true,
	}
}

func mergeCourseKey(base, overlay courseKey) courseKey {
	if base.classID == "" {
		base.classID = overlay.classID
	}
	if base.courseID == "" {
		base.courseID = overlay.courseID
	}
	if base.projectID == "" || base.projectID == "0" {
		base.projectID = firstNonEmpty(overlay.projectID, base.projectID)
	}
	if base.productID == "" {
		base.productID = overlay.productID
	}
	if base.title == "" {
		base.title = overlay.title
	}
	if base.price == 0 {
		base.price = overlay.price
	}
	if overlay.purchased {
		base.purchased = true
	}
	return base
}

func hasSJKCookie(jar http.CookieJar) bool {
	if jar == nil {
		return false
	}
	for _, raw := range []string{urlReferer, urlClassroom, urlUserInfo, urlOrigin, urlClassroomOrigin} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if len(jar.Cookies(u)) > 0 {
			return true
		}
	}
	return false
}

func checkSJKLogin(c *util.Client, jar http.CookieJar) bool {
	apiURL := urlCourseList + "?teacherId=&keyword=&sortDirection=&sortField=lastStudyAt&tab=all&page=1&limit=1"
	if okJSONCode(c, apiURL, classroomHeaders(jar), 200, false) {
		return true
	}
	if okJSONCode(c, urlUserInfo, classroomHeaders(jar), 200, true) {
		return true
	}
	body, err := c.GetString(urlCourseCatalog, studyHeaders(jar, urlReferer))
	if err != nil {
		return false
	}
	var root map[string]any
	if json.Unmarshal([]byte(body), &root) != nil {
		return false
	}
	code := strings.TrimSpace(fmt.Sprint(root["code"]))
	msg := fmt.Sprint(root["msg"])
	return (code == "200" || code == "403") && strings.Contains(msg, "优课清单")
}

func okJSONCode(c *util.Client, rawURL string, headers map[string]string, want int, acceptData bool) bool {
	body, err := c.GetString(rawURL, headers)
	if err != nil {
		return false
	}
	var root map[string]any
	if json.Unmarshal([]byte(body), &root) != nil {
		return false
	}
	if strings.TrimSpace(fmt.Sprint(root["code"])) == strconv.Itoa(want) {
		return true
	}
	if acceptData && root["data"] != nil {
		return true
	}
	return false
}

func fetchPublicProductPrice(c *util.Client, productID string) float64 {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return 0
	}
	apiURL := fmt.Sprintf(urlPublicProduct, url.PathEscape(productID))
	body, err := c.GetString(apiURL, publicProductHeaders(apiURL))
	if err != nil {
		return 0
	}
	for _, pat := range []string{
		`(?i)"(?:price|salePrice|currentPrice|originPrice)"\s*:\s*"?([0-9][0-9,]*(?:\.[0-9]+)?)"?`,
		`(?i)(?:¥|￥)\s*([0-9][0-9,]*(?:\.[0-9]+)?)`,
		`(?i)([0-9][0-9,]*(?:\.[0-9]+)?)\s*元`,
	} {
		for _, match := range regexp.MustCompile(pat).FindAllStringSubmatch(body, -1) {
			if len(match) > 1 {
				if price := normalizeSJKPrice(match[1]); price > 0 {
					return price
				}
			}
		}
	}
	return 0
}

func publicProductHeaders(referer string) map[string]string {
	return map[string]string{
		"Referer":                   firstNonEmpty(referer, "https://www.sanjieke.cn/"),
		"Upgrade-Insecure-Requests": "1",
		"Pragma":                    "no-cache",
		"Cache-Control":             "no-cache",
		"Accept-Language":           "zh-CN,zh;q=0.9",
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"User-Agent":                browserUA,
	}
}

func normalizeSJKPrice(v any) float64 {
	if v == nil {
		return 0
	}
	s := anyString(v)
	if s == "" {
		return 0
	}
	s = strings.NewReplacer(",", "", "\\xa5", "", "\\uffe5", "", "¥", "", "￥", "", "元", "").Replace(s)
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || n < 0 {
		return 0
	}
	if n >= 1000 && n == float64(int64(n)) {
		n = n / 100
	}
	out, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", n), 64)
	return out
}

func compactSJKExtra(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		switch t := v.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(t) == "" {
				continue
			}
		case float64:
			if t == 0 {
				continue
			}
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeSJKTitle(s string) string {
	return strings.Trim(regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`).ReplaceAllString(s, "_"), " .")
}
