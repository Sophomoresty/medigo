package cctalk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func cookieHeaderFromJar(jar http.CookieJar) string {
	if jar == nil {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	for _, raw := range []string{CCTALK_BASE_URL + "/", cctalkMobileURL + "/", "https://cctalk.hjapi.com/"} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, cookie := range jar.Cookies(u) {
			if cookie == nil || cookie.Name == "" || cookie.Value == "" {
				continue
			}
			key := strings.ToLower(cookie.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			parts = append(parts, cookie.Name+"="+cookie.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func parseCookieHeader(header string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		out[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return out
}

func hasCCTalkLoginCookie(jar http.CookieJar, header string) bool {
	cookies := parseCookieHeader(header)
	if len(cookies) == 0 && jar != nil {
		cookies = parseCookieHeader(cookieHeaderFromJar(jar))
	}
	for name, value := range cookies {
		if strings.TrimSpace(value) == "" {
			continue
		}
		switch strings.ToLower(name) {
		case "clubauth", "access_token", "accesstoken", "hjuseragent", "hjid", "hjuid", "token", "refresh_token", "refreshtoken":
			return true
		}
	}
	return false
}

func (a *apiClient) ensureLogin() error {
	if a == nil || a.c == nil {
		return fmt.Errorf("cctalk requires login cookies")
	}
	if !hasCCTalkLoginCookie(a.jar, a.cookieHeader) {
		return fmt.Errorf("cctalk requires valid login cookies")
	}
	probe := a.requestLoginProbe()
	if payloadSuccess(probe) {
		return nil
	}
	uid := firstNonEmpty(a.currentUserID(), loginPayloadUserUID(probe))
	if user := a.requestCurrentUserInfo(uid); len(user) > 0 {
		return nil
	}
	if uid != "" && probe == nil {
		return nil
	}
	return fmt.Errorf("cctalk login check failed: cookies are missing or expired")
}

func (a *apiClient) requestLoginProbe() map[string]any {
	if a == nil || a.c == nil {
		return nil
	}
	probeURLs := []string{
		fmt.Sprintf(cctalkMyGroupListURL, 0, 1, ""),
		"https://cctalk.hjapi.com/content/v1.1/group/81840438/new_video_list?start=0&limit=1&withScattered=0",
		"https://cctalk.hjapi.com/sns/v1.1/follow/followinglist",
	}
	for _, probeURL := range probeURLs {
		body, err := a.c.GetString(probeURL, mergeStringMaps(a.headers, map[string]string{"Referer": cctalkMyCourseURL, "Origin": cctalkMobileURL, "Accept": "*/*"}))
		if err != nil {
			continue
		}
		var payload map[string]any
		if json.Unmarshal([]byte(body), &payload) == nil && len(payload) > 0 {
			return payload
		}
	}
	return nil
}

func (a *apiClient) requestCurrentUserInfo(uid string) map[string]any {
	if a == nil || a.c == nil {
		return nil
	}
	uid = firstNonEmpty(uid, a.currentUserID())
	paths := []string{"/webapi/sns/v1.1/user/current_user_info", "/webapi/sns/v1.1/user/user_info"}
	if uid != "" {
		paths = append([]string{fmt.Sprintf("/webapi/sns/v1.1/user/%s/info", url.PathEscape(uid)), fmt.Sprintf("https://m.cctalk.com/webapi/sns/v1.1/user/%s/info", url.PathEscape(uid)), fmt.Sprintf("https://cctalk.hjapi.com/sns/v1.1/user/%s/info", url.PathEscape(uid))}, paths...)
	}
	for _, path := range paths {
		reqURL := path
		if strings.HasPrefix(path, "/") {
			reqURL = CCTALK_BASE_URL + path
		}
		data := asMap(extractData(a.requestJSON(reqURL, nil, "")))
		if len(data) > 0 {
			return data
		}
	}
	return nil
}

func payloadSuccess(payload map[string]any) bool {
	if len(payload) == 0 {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(textAny(payload["status"])))
	code := strings.ToLower(strings.TrimSpace(textAny(payload["code"])))
	if status == "-1" || code == "-1" || status == "false" {
		return false
	}
	if data := extractData(payload); data != nil {
		switch data.(type) {
		case []any:
			return true
		case map[string]any:
			return true
		}
	}
	return status == "" || status == "0" || status == "true" || status == "success" || code == "" || code == "0" || code == "200"
}

func loginPayloadUserUID(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	var found string
	var walk func(any, int)
	walk = func(value any, depth int) {
		if found != "" || value == nil || depth > 6 {
			return
		}
		switch x := value.(type) {
		case map[string]any:
			for _, key := range []string{"UserId", "userId", "user_id", "uid"} {
				if uid := textAny(x[key]); onlyDigits(uid) {
					found = uid
					return
				}
			}
			for _, nested := range x {
				walk(nested, depth+1)
			}
		case []any:
			for _, nested := range x {
				walk(nested, depth+1)
			}
		}
	}
	walk(payload, 0)
	return found
}
