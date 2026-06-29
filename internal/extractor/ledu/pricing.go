package ledu

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const leduDefaultHiddenPrice = 999

var leduPriceKeys = []string{"salePrice", "price", "classPrice", "payPrice", "realPrice", "discountPrice", "activityPrice", "nowPrice", "originPrice", "originalPrice", "finalPrice"}

var leduFreeTitleKeywords = []string{"体验课", "试听课", "公开课", "免费", "试学", "试听", "体验", "赠课", "赠送", "福利课", "福利"}

var leduNoContentTitleKeywords = []string{"无内容", "仅拼团", "拼团", "邮寄讲义", "讲义专用", "邮寄使用", "专用无内容", "无课程内容", "邮寄用", "邮寄专用"}

func leduResolveCoursePrice(classInfo map[string]any, title string) float64 {
	price, found := leduExtractCoursePriceInfo(classInfo)
	if found {
		return price
	}
	if leduIsProbablyFreeCourse(classInfo, title) {
		return 0
	}
	return leduDefaultHiddenPrice
}

func leduPurchased(classInfo map[string]any) bool {
	if classInfo == nil {
		return true
	}
	for _, node := range nestedMaps(classInfo) {
		for _, key := range []string{"isPay", "is_buy", "isBuy", "buyStatus", "hasBuy"} {
			if v, ok := node[key]; ok && v != nil && firstText(v) != "" {
				return leduCoerceBool(v, true)
			}
		}
	}
	return true
}

func leduExtractCoursePriceInfo(classInfo map[string]any) (float64, bool) {
	if classInfo == nil {
		return 0, false
	}
	found := false
	for _, node := range nestedMaps(classInfo) {
		for _, key := range leduPriceKeys {
			value, ok := node[key]
			if !ok || value == nil {
				continue
			}
			switch value.(type) {
			case map[string]any, []any, []map[string]any:
				continue
			}
			if firstText(value) == "" {
				continue
			}
			found = true
			if price := leduNormalizePrice(value); price != 0 {
				return price, true
			}
		}
	}
	return 0, found
}

func leduIsProbablyFreeCourse(classInfo map[string]any, title string) bool {
	if leduLooksLikeFreeTitle(title) {
		return true
	}
	for _, node := range nestedMaps(classInfo) {
		for _, key := range []string{"className", "courseName", "name", "title", "subTitle", "tag", "tagName", "label", "labelName", "productName", "goodsName", "desc", "description"} {
			if leduLooksLikeFreeTitle(firstText(node[key])) {
				return true
			}
		}
		for _, key := range []string{"isFree", "free", "freeFlag", "isGift", "giftFlag", "isGiftCourse", "isTrial", "trialFlag", "isExperience", "experienceFlag", "isPublicCourse"} {
			if v, ok := node[key]; ok && v != nil && firstText(v) != "" && leduCoerceBool(v, false) {
				return true
			}
		}
		productType := strings.TrimSpace(firstText(node["productType"]))
		if productType == "5" && leduContainsKeyword(title, []string{"体验", "试听", "试学", "公开"}) {
			return true
		}
		for _, key := range []string{"priceType", "chargeType", "payType", "goodsType", "saleType", "courseType"} {
			value := strings.ToLower(strings.TrimSpace(firstText(node[key])))
			if value == "free" || value == "gratis" || value == "gift" || value == "trial" || value == "experience" || value == "public" {
				return true
			}
		}
	}
	return false
}

func leduLooksLikeFreeTitle(title string) bool {
	if leduContainsKeyword(title, leduNoContentTitleKeywords) {
		return false
	}
	return leduContainsKeyword(title, leduFreeTitleKeywords)
}

func leduContainsKeyword(text string, keywords []string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func leduNormalizePrice(value any) float64 {
	if value == nil {
		return 0
	}
	var f float64
	switch x := value.(type) {
	case float64:
		f = x
	case float32:
		f = float64(x)
	case int:
		f = float64(x)
	case int64:
		f = float64(x)
	case json.Number:
		n, err := x.Float64()
		if err != nil {
			return 0
		}
		f = n
	default:
		s := strings.TrimSpace(fmt.Sprint(value))
		s = strings.NewReplacer(",", "", "￥", "", "¥", "").Replace(s)
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		f = n
	}
	if f >= 1000 && math.Trunc(f) == f {
		f /= 100
	}
	if f < 0 {
		return 0
	}
	return math.Round(f*100) / 100
}

func leduCoerceBool(value any, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	switch x := value.(type) {
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case json.Number:
		n, err := x.Float64()
		if err != nil {
			return defaultValue
		}
		return n != 0
	default:
		s := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
		switch s {
		case "":
			return defaultValue
		case "1", "true", "yes", "y":
			return true
		case "0", "false", "no", "n":
			return false
		default:
			return defaultValue
		}
	}
}
