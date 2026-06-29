package classin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/extractor"
)

type classinAuth struct {
	UID string
	Key string
}

func (a classinAuth) normalized() classinAuth {
	a.UID = strings.TrimSpace(a.UID)
	a.Key = strings.TrimSpace(a.Key)
	if a.UID == "" {
		a.UID = classinUID
	}
	if a.Key == "" {
		a.Key = classinKey
	}
	return a
}

func classinAuthFromOpts(opts *extractor.ExtractOpts) classinAuth {
	auth := classinAuth{UID: classinUID, Key: classinKey}
	if opts == nil || opts.Cookies == nil {
		return auth
	}
	for _, raw := range []string{
		"https://www.eeo.cn/",
		"https://t0d-cdn.eeo.cn/",
		"https://w0d-cdn.eeo.cn/",
		"https://a0d-cdn.eeo.cn/",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, ck := range opts.Cookies.Cookies(u) {
			mergeClassinCookie(&auth, ck)
		}
	}
	return auth.normalized()
}

func mergeClassinCookie(auth *classinAuth, ck *http.Cookie) {
	if ck == nil {
		return
	}
	name := strings.ToLower(strings.TrimSpace(ck.Name))
	value := strings.TrimSpace(ck.Value)
	switch name {
	case "uid", "user_id", "userid", "user-id":
		if value != "" {
			auth.UID = value
		}
	case "key", "sign_key", "signkey", "app_secret", "appsecret":
		if value != "" {
			auth.Key = value
		}
	}
	mergeClassinAuthText(auth, value)
}

func mergeClassinAuthText(auth *classinAuth, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	if unescaped, err := url.QueryUnescape(raw); err == nil {
		raw = strings.TrimSpace(unescaped)
	}
	if vals, err := url.ParseQuery(raw); err == nil && len(vals) > 0 {
		if mergeClassinValues(auth, vals) {
			return
		}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		for _, key := range []string{"uid", "UID", "userId", "user_id"} {
			if val, ok := obj[key]; ok {
				if s := strings.TrimSpace(toClassinString(val)); s != "" {
					auth.UID = s
					break
				}
			}
		}
		for _, key := range []string{"key", "sign_key", "signKey", "app_secret", "appSecret"} {
			if val, ok := obj[key]; ok {
				if s := strings.TrimSpace(toClassinString(val)); s != "" {
					auth.Key = s
					break
				}
			}
		}
	}
}

func mergeClassinValues(auth *classinAuth, vals url.Values) bool {
	changed := false
	for _, key := range []string{"uid", "UID", "userId", "user_id"} {
		if v := vals.Get(key); strings.TrimSpace(v) != "" {
			auth.UID = strings.TrimSpace(v)
			changed = true
			break
		}
	}
	for _, key := range []string{"key", "sign_key", "signKey", "app_secret", "appSecret"} {
		if v := vals.Get(key); strings.TrimSpace(v) != "" {
			auth.Key = strings.TrimSpace(v)
			changed = true
			break
		}
	}
	return changed
}

func toClassinString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}
