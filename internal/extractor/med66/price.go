package med66

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Sophomoresty/mediago/internal/util"
)

const MED66_CC_REPLAY_VERSION = "3.6.1"

func courseUpgradeReferer(course med66Course, isAI string) string {
	repl := strings.NewReplacer(
		"courseId={}", "courseId="+url.QueryEscape(course.CourseID),
		"classId={}", "classId="+url.QueryEscape(course.ClassID),
		"classType={}", "classType="+url.QueryEscape(course.ClassType),
		"isAi={}", "isAi="+url.QueryEscape(isAI),
	)
	return repl.Replace(COURSE_UPGRADE_REFERER)
}

func resolveCoursePrice(c *util.Client, headers map[string]string, course med66Course) float64 {
	if p := firstPositive(collectPriceCandidates(course.Raw)); p > 0 {
		return normalizeCoursePrice(p)
	}
	if p := extractPriceFromUpgrade(c, headers, course); p > 0 {
		return normalizeCoursePrice(p)
	}
	return guessPriceFromTitle(course)
}

func extractPriceFromUpgrade(c *util.Client, headers map[string]string, course med66Course) float64 {
	if course.CourseID == "" {
		return 0
	}
	candidates := uniqueNonEmpty(course.IsAI, "0", "1")
	for _, isAI := range candidates {
		h := map[string]string{}
		for k, v := range headers {
			h[k] = v
		}
		h["Referer"] = courseUpgradeReferer(course, isAI)
		body, err := c.PostForm(COURSE_UPGRADE_URL, map[string]string{"courseId": course.CourseID}, h)
		if err != nil {
			continue
		}
		var root any
		if err := json.Unmarshal([]byte(body), &root); err != nil {
			continue
		}
		if p := firstPositive(collectPriceCandidates(root)); p > 0 {
			return p
		}
	}
	return 0
}

func firstPositive(values []float64) float64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func guessPriceFromTitle(course med66Course) float64 {
	title := strings.Join([]string{
		course.Title,
		fmt.Sprint(course.Raw["list_name"]),
		fmt.Sprint(course.Raw["detail_name"]),
		fmt.Sprint(course.Raw["home_title"]),
		fmt.Sprint(course.Raw["sel_course_title"]),
		firstString(course.Raw, "listName", "detailName", "homeTitle", "selCourseTitle"),
	}, " ")
	switch {
	case strings.Contains(title, "高效定制班") || strings.Contains(title, "定制班"):
		return 2380
	case strings.Contains(title, "无忧实验班"):
		return 1480
	case strings.Contains(title, "超值精品班") || strings.Contains(title, "精品班"):
		return 880
	case strings.Contains(title, "直播密押班") || strings.Contains(title, "密押班"):
		return 0
	default:
		return 0
	}
}
