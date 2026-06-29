package dingtalk

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	dingtalkURLRe      = regexp.MustCompile(`https?://[^\s"'<>]+`)
	dingtalkLiveUUIDRe = regexp.MustCompile(`(?i)/live/([^/?#]+)`)
)

// extractDingtalkURLsFromText mirrors Dingtalk_Config.extract_dingtalk_urls:
// accept pasted/batch text and keep only DingTalk-related video/document URLs.
func extractDingtalkURLsFromText(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, match := range dingtalkURLRe.FindAllString(text, -1) {
		u := strings.TrimRight(strings.TrimSpace(match), "，。,.、;；)）]】>\"'")
		lower := strings.ToLower(u)
		if !strings.Contains(lower, "dingtalk.com/") && !strings.Contains(lower, "alidocs.dingtalk.com/") && !strings.Contains(lower, "shanji.dingtalk.com/") {
			continue
		}
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

func dingtalkM3U8DataURL(text string) string {
	return "data:application/vnd.apple.mpegurl;base64," + base64.StdEncoding.EncodeToString([]byte(text))
}

func addOrReplaceQueryParam(rawURL, key, value string) string {
	if rawURL == "" || key == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	q.Set(key, value)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// prepareDingTalkM3U8Text makes segment/key URLs absolute and applies the
// DingTalk client replay ding_token signature to TS segment URLs when a
// playbackToken is available.
func prepareDingTalkM3U8Text(content, sourceURL, playbackToken string) string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			lines = append(lines, line)
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			if strings.Contains(line, `URI="`) {
				line = uriLineRe.ReplaceAllStringFunc(line, func(match string) string {
					sub := uriLineRe.FindStringSubmatch(match)
					if len(sub) < 2 {
						return match
					}
					abs := resolveURL(sourceURL, sub[1])
					return fmt.Sprintf(`URI="%s"`, abs)
				})
			}
			lines = append(lines, line)
			continue
		}
		segmentURL := resolveURL(sourceURL, trimmed)
		if playbackToken != "" {
			if token := makeDingToken(segmentURL, playbackToken); token != "" {
				segmentURL = addOrReplaceQueryParam(segmentURL, "ding_token", token)
			}
		}
		lines = append(lines, segmentURL)
	}
	return strings.Join(lines, "\n") + "\n"
}

func extractLiveUUIDFromMediaURL(mediaURL string) string {
	if m := dingtalkLiveUUIDRe.FindStringSubmatch(mediaURL); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if parsed, err := url.Parse(mediaURL); err == nil {
		return firstNonEmptyText(parsed.Query().Get("liveUuid"), parsed.Query().Get("uuid"))
	}
	return ""
}

func dingtalkSourceTypes(urls []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range urls {
		t := dingtalkSourceType(u)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func dingtalkSourceType(rawURL string) string {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.Contains(lower, "aliyun") || strings.Contains(lower, "alicdn") || strings.Contains(lower, "aliyuncs.com"):
		return "aliyun"
	case strings.Contains(lower, "polyv") || strings.Contains(lower, "videocc.net"):
		return "polyv"
	case strings.Contains(lower, ".m3u8"):
		return "m3u8"
	case strings.Contains(lower, ".mp4"):
		return "mp4"
	case strings.HasPrefix(lower, "data:application/vnd.apple.mpegurl"):
		return "m3u8_text"
	case strings.HasPrefix(lower, "http"):
		return "cdn"
	default:
		return ""
	}
}

func dingtalkChargeBeans(videoCount int) int {
	if videoCount <= 0 {
		return 0
	}
	return videoCount * 10
}
