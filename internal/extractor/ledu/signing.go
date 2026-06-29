package ledu

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	leduTalSignKey       = "pzpcZ3H5cjHhLFJFz3YfzaygNhpkAEpz"
	leduTalKeyVersion    = "and_1.26"
	leduAppAESKey        = "ZGZrXMl0BE2P1Avi"
	leduAppAESIV         = "boopOq3kRIxsZ6rm"
	leduAppAccessID      = "c2c67b0c9f964048aa8fd549c66efc74"
	leduAppAccessKey     = "db50e30780e244a4b155d7a6b1a71262"
	leduClientID         = "522203"
	leduAppPackage       = "com.dadaabc.zhuozan.dadateacher"
	leduDeviceModel      = "Redmi M2012K10C"
	leduAndroidVersion   = "13"
	leduArea             = "010"
	leduH5AccessID       = "wx550fedfd29aa3463"
	leduH5AccessKey      = "snz057wbu86tg43dflkvpjx9orecya1h"
	leduH5Version        = "3.10.5"
	leduH5ClientType     = "1"
	leduH5RegClientType  = "200"
	leduDefaultBranchID  = "1111"
	leduSignatureMethod  = "HmacMD5"
	leduH5SignatureAlgo  = "HmacSHA1"
	leduSignedJSONHeader = "application/json; charset=UTF-8"
)

type leduAuthContext struct {
	Cookie         string
	StudentID      string
	UserID         string
	UserIDStr      string
	PUID           string
	OpenID         string
	UnionID        string
	Token          string
	Authorization  string
	DeviceID       string
	ClientType     string
	AreaCode       string
	CategoryTypes  string
	BelongCitys    string
	HomeSelectCity string
}

func leduAuthFromHeaders(headers map[string]string) leduAuthContext {
	cookie := firstText(headers["Cookie"], headers["cookie"])
	auth := leduAuthContext{
		Cookie:         cookie,
		StudentID:      firstText(headers["stuId"], headers["studentid"], headers["studentId"], cookieValue(cookie, "stuId"), cookieValue(cookie, "stuIdStr"), cookieValue(cookie, "user_id"), cookieValue(cookie, "uid"), cookieValue(cookie, "puid"), cookieValue(cookie, "pu_uid"), cookieValue(cookie, "LEDU_STUID_PROD")),
		UserID:         firstText(cookieValue(cookie, "user_id"), cookieValue(cookie, "uid"), headers["uid"]),
		UserIDStr:      firstText(cookieValue(cookie, "user_id_str"), cookieValue(cookie, "stuIdStr"), cookieValue(cookie, "stuId"), headers["stuId"]),
		PUID:           firstText(cookieValue(cookie, "puid"), cookieValue(cookie, "pu_uid"), cookieValue(cookie, "user_uid"), headers["uid"]),
		OpenID:         cookieValue(cookie, "open_id"),
		UnionID:        cookieValue(cookie, "union_id"),
		Token:          firstText(headers["token"], headers["login-token"], cookieValue(cookie, "token"), cookieValue(cookie, "hb_token"), cookieValue(cookie, "classroom_token"), cookieValue(cookie, "LEDU_AUTHORIZATION_PROD")),
		Authorization:  firstText(headers["authorization"], headers["Authorization"], cookieValue(cookie, "authorization"), cookieValue(cookie, "LEDU_AUTHORIZATION_PROD")),
		DeviceID:       firstText(cookieValue(cookie, "device_id"), cookieValue(cookie, "deviceId"), cookieValue(cookie, "devid"), cookieValue(cookie, "d-id"), cookieValue(cookie, "dv")),
		ClientType:     firstText(cookieValue(cookie, "client_type"), leduH5ClientType),
		AreaCode:       firstText(cookieValue(cookie, "area_code"), leduArea),
		CategoryTypes:  cookieValue(cookie, "category_types"),
		BelongCitys:    cookieValue(cookie, "belong_citys"),
		HomeSelectCity: cookieValue(cookie, "home_select_city"),
	}
	if auth.StudentID == "" {
		auth.StudentID = firstText(auth.UserIDStr, auth.UserID, auth.PUID)
	}
	if auth.Authorization == "" && auth.Token != "" {
		auth.Authorization = auth.Token
	}
	if auth.DeviceID == "" {
		sum := md5.Sum([]byte(auth.Cookie + "|" + auth.StudentID))
		auth.DeviceID = "TAL2200" + strings.ToUpper(hex.EncodeToString(sum[:]))
	}
	return auth
}

func decorateLeduHeaders(host, path, method string, params map[string]string, body map[string]any, headers map[string]string) map[string]string {
	out := cloneHeaders(headers)
	if out == nil {
		out = map[string]string{}
	}
	host = strings.ToLower(host)
	method = strings.ToUpper(firstText(method, "GET"))
	switch {
	case strings.Contains(host, "classroom-api.ledupeiyou.com") && strings.Contains(path, "/home-pages/"):
		return leduHomepageHeaders(out)
	case strings.Contains(host, "app.ledupeiyou.com") && strings.Contains(path, "/backend-service/"):
		return leduH5Headers(host+path, method, params, body, out, nil)
	case strings.Contains(host, "app.ledupeiyou.com") && strings.Contains(path, "/wx-aggregation/"):
		return leduH5Headers(host+path, method, params, body, out, nil)
	case strings.Contains(host, "app.ledupeiyou.com"):
		return leduAppHeaders(body, out)
	case strings.Contains(host, "cloudlearn.ledupeiyou.com"):
		return leduCloudlearnHeaders(body, out)
	default:
		if out["reqTime"] == "" {
			out["reqTime"] = strconv.FormatInt(time.Now().UnixMilli(), 10)
		}
		return out
	}
}

func leduAppHeaders(body map[string]any, headers map[string]string) map[string]string {
	auth := leduAuthFromHeaders(headers)
	out := cloneHeaders(headers)
	if out == nil {
		out = map[string]string{}
	}
	for k, v := range map[string]string{
		"Content-Type":    leduSignedJSONHeader,
		"Accept":          "application/json, text/plain, */*",
		"User-Agent":      "okhttp/4.9.3",
		"Referer":         leduReferer,
		"Origin":          strings.TrimRight(leduReferer, "/"),
		"version":         browserVersion(),
		"ui":              firstText(auth.UserID, auth.StudentID),
		"stu_id":          auth.StudentID,
		"selectedGradeId": out["stdGrade"],
		"onlyv":           browserVersion(),
		"gradeId":         out["stdGrade"],
		"dv":              auth.DeviceID,
		"dn":              leduDeviceModel,
		"devid":           auth.DeviceID,
		"client_type":     "2",
		"client_id":       leduClientID,
		"area":            firstText(auth.AreaCode, leduArea),
		"enc-flag":        leduSignedJSONHeader,
		"algorithm":       leduSignatureMethod,
		"accessid":        leduAppAccessID,
	} {
		if v != "" {
			out[k] = v
		}
	}
	out["timestamp"] = strconv.FormatInt(time.Now().UnixMilli(), 10)
	out["nonce"] = leduNonce(8)
	signPayload := leduMergeSignValues(body, map[string]any{
		"version":         out["version"],
		"ui":              out["ui"],
		"timestamp":       out["timestamp"],
		"stu_id":          auth.StudentID,
		"selectedGradeId": out["selectedGradeId"],
		"onlyv":           out["onlyv"],
		"nonce":           out["nonce"],
		"gradeId":         out["gradeId"],
		"dv":              out["dv"],
		"dn":              out["dn"],
		"devid":           out["devid"],
		"client_type":     out["client_type"],
		"client_id":       out["client_id"],
		"area":            out["area"],
	})
	out["sign"] = leduHMACMD5Hex(leduAppAccessKey, leduSignParams(signPayload))
	if auth.Authorization != "" {
		if strings.HasPrefix(strings.ToLower(auth.Authorization), "bearer ") {
			out["Authorization"] = auth.Authorization
		} else {
			out["Authorization"] = "Bearer " + auth.Authorization
		}
	}
	return out
}

func leduCloudlearnHeaders(body map[string]any, headers map[string]string) map[string]string {
	auth := leduAuthFromHeaders(headers)
	out := leduAppHeaders(body, headers)
	for k, v := range map[string]string{
		"requestid":     leduTraceID(),
		"resourcescale": "1",
		"local_city":    firstText(auth.AreaCode, leduArea),
		"source":        "app",
		"uid":           auth.PUID,
		"studentid":     auth.StudentID,
		"login-token":   firstText(auth.Token, auth.Authorization),
		"appid":         leduClientID,
		"User-Agent":    "okhttp/4.9.3",
		"Referer":       cloudlearnHost + "/",
	} {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

func leduH5Headers(rawURL, method string, params map[string]string, body map[string]any, headers map[string]string, extraHeaderKeys []string) map[string]string {
	auth := leduAuthFromHeaders(headers)
	out := cloneHeaders(headers)
	if out == nil {
		out = map[string]string{}
	}
	for k, v := range map[string]string{
		"Content-Type":       "application/json;charset=UTF-8",
		"Accept":             "application/json, text/plain, */*",
		"User-Agent":         browserUA,
		"Referer":            leduReferer,
		"Origin":             strings.TrimRight(leduReferer, "/"),
		"homeSelectCity":     auth.HomeSelectCity,
		"belongCitys":        auth.BelongCitys,
		"categoryTypes":      auth.CategoryTypes,
		"recommand_emp_no":   "",
		"regist_client_type": leduH5RegClientType,
		"devid":              "h5",
		"local_city":         firstText(auth.AreaCode, leduArea),
		"area":               firstText(auth.AreaCode, leduArea),
		"area_code":          firstText(auth.AreaCode, leduArea),
		"uid":                auth.PUID,
		"stu_id":             auth.StudentID,
		"version":            leduH5Version,
		"authorization":      firstText(auth.Authorization, auth.Token),
		"client_type":        leduH5ClientType,
		"clientId":           "",
		"algorithm":          leduH5SignatureAlgo,
		"nonce":              strconv.FormatInt(time.Now().UnixMilli(), 10) + leduNonce(16),
		"accessid":           leduH5AccessID,
		"timestamp":          strconv.FormatInt(time.Now().UnixMilli(), 10),
	} {
		out[k] = v
	}
	if out["Cookie"] == "" {
		var cookieParts []string
		for _, kv := range [][2]string{
			{"LEDU_AUTHORIZATION_PROD", firstText(auth.Authorization, auth.Token)},
			{"LEDU_STUID_PROD", auth.StudentID},
			{"LEDU_PUID_PROD", auth.PUID},
			{"LEDU_OPENID_PROD", auth.OpenID},
			{"LEDU_UNIONID_PROD", auth.UnionID},
		} {
			if kv[1] != "" {
				cookieParts = append(cookieParts, kv[0]+"="+kv[1])
			}
		}
		out["Cookie"] = strings.Join(cookieParts, "; ")
	}
	out["sign"] = leduHMACSHA1Upper(leduH5AccessKey, leduH5SignPayload(rawURL, method, params, body, out, extraHeaderKeys))
	return out
}

func leduHomepageHeaders(headers map[string]string) map[string]string {
	auth := leduAuthFromHeaders(headers)
	out := cloneHeaders(headers)
	if out == nil {
		out = map[string]string{}
	}
	gradeID := firstText(out["stdGrade"], out["gradeId"])
	for k, v := range map[string]string{
		"Content-Type":  "application/json;charset=UTF-8",
		"Accept":        "application/json, text/plain, */*",
		"User-Agent":    browserUA,
		"clientTraceId": leduClientTraceID(),
		"timestamp":     strconv.FormatInt(time.Now().UnixMilli(), 10),
		"v":             leduH5Version,
		"clientType":    "9",
		"gradeId":       gradeID,
		"stuIdStr":      auth.StudentID,
		"stuId":         auth.StudentID,
		"authorization": firstText(auth.Authorization, auth.Token),
		"Referer":       "https://stu.ledupeiyou.com/",
		"Origin":        "https://stu.ledupeiyou.com",
		"Host":          "classroom-api.ledupeiyou.com",
	} {
		out[k] = v
	}
	if out["Cookie"] == "" {
		var cookieParts []string
		for _, kv := range [][2]string{
			{"LEDU_AUTHORIZATION_PROD", firstText(auth.Authorization, auth.Token)},
			{"LEDU_STUID_PROD", auth.StudentID},
			{"LEDU_PUID_PROD", auth.PUID},
			{"LEDU_OPENID_PROD", auth.OpenID},
			{"LEDU_UNIONID_PROD", auth.UnionID},
		} {
			if kv[1] != "" {
				cookieParts = append(cookieParts, kv[0]+"="+kv[1])
			}
		}
		out["Cookie"] = strings.Join(cookieParts, "; ")
	}
	return out
}

func leduMediaHeaders(headers map[string]string) map[string]string {
	out := map[string]string{
		"Origin":     strings.TrimRight(leduReferer, "/"),
		"Referer":    leduReferer,
		"User-Agent": browserUA,
		"Accept":     "*/*",
	}
	if cookie := firstText(headers["Cookie"], headers["cookie"]); cookie != "" {
		out["Cookie"] = cookie
	}
	return out
}

func applyLeduResponseHeaders(base map[string]string, requestHeaders map[string]string, responseHeaders http.Header) {
	if responseHeaders == nil {
		return
	}
	token := firstText(responseHeaders.Get("token"), responseHeaders.Get("hb-token"), responseHeaders.Get("classroom-token"), responseHeaders.Get("authorization"))
	if token == "" {
		return
	}
	for _, h := range []map[string]string{base, requestHeaders} {
		if h == nil {
			continue
		}
		h["token"] = token
		h["hb-token"] = token
		h["classroom-token"] = token
		h["login-token"] = token
		h["authorization"] = token
		if strings.HasPrefix(strings.ToLower(token), "bearer ") {
			h["Authorization"] = token
		} else {
			h["Authorization"] = "Bearer " + token
		}
	}
}

func leduSignParams(params map[string]any) string {
	if len(params) == 0 {
		return ""
	}
	items := make(map[string]string, len(params))
	for k, v := range params {
		s := leduSerializeSignValue(v)
		if s == "" {
			continue
		}
		items[k] = s
	}
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+items[k])
	}
	return strings.Join(parts, "&")
}

func leduH5SignPayload(rawURL, method string, params map[string]string, body map[string]any, headers map[string]string, extraHeaderKeys []string) string {
	items := map[string]any{}
	for k, v := range body {
		items[k] = v
	}
	for k, v := range params {
		items[k] = v
	}
	if u, err := url.Parse(rawURL); err == nil {
		for k, vs := range u.Query() {
			if len(vs) > 0 {
				items[k] = vs[len(vs)-1]
			}
		}
	}
	headerKeys := map[string]bool{
		"area": true, "gradeId": true, "devid": true, "v": true, "stu_id": true,
		"client_type": true, "timestamp": true, "accessid": true, "nonce": true,
		"algorithm": true, "version": true, "authorization": true,
	}
	for _, k := range extraHeaderKeys {
		headerKeys[k] = true
	}
	for k := range headerKeys {
		if v := firstText(headers[k]); v != "" {
			items[k] = v
		}
	}
	_ = method // the restored source signs data/header/query fields, not method.
	return leduSignParams(items)
}

func leduSerializeSignValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		x = strings.TrimSpace(x)
		if x == "" || x == "undefined" {
			return ""
		}
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case json.Number:
		return x.String()
	case map[string]any, []any, []map[string]any:
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		return string(b)
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" || s == "undefined" {
			return ""
		}
		return s
	}
}

func leduMergeSignValues(primary map[string]any, extra map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range extra {
		out[k] = v
	}
	for k, v := range primary {
		out[k] = v
	}
	return out
}

func leduHMACMD5Hex(key, data string) string {
	mac := hmac.New(md5.New, []byte(key))
	_, _ = mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func leduHMACSHA1Upper(key, data string) string {
	mac := hmac.New(sha1.New, []byte(key))
	_, _ = mac.Write([]byte(data))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
}

func leduTalSign(paramsStr, timestamp string) string {
	sum := sha1.Sum([]byte(leduTalSignKey + paramsStr + timestamp))
	raw := leduTalKeyVersion + ":" + hex.EncodeToString(sum[:])
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func leduAppAESEncrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher([]byte(leduAppAESKey))
	if err != nil {
		return "", err
	}
	data := leduPKCS7Pad([]byte(plaintext), aes.BlockSize)
	mode := cipher.NewCBCEncrypter(block, []byte(leduAppAESIV))
	mode.CryptBlocks(data, data)
	return base64.StdEncoding.EncodeToString(data), nil
}

func leduAppAESDecrypt(ciphertextB64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(ciphertextB64))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return "", fmt.Errorf("invalid ledu AES ciphertext length %d", len(raw))
	}
	block, err := aes.NewCipher([]byte(leduAppAESKey))
	if err != nil {
		return "", err
	}
	mode := cipher.NewCBCDecrypter(block, []byte(leduAppAESIV))
	mode.CryptBlocks(raw, raw)
	raw, err = leduPKCS7Unpad(raw, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func leduParseJSON(raw []byte) (any, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err == nil {
		if decoded, ok := leduMaybeDecryptPayload(payload); ok {
			return decoded, nil
		}
		return payload, nil
	}
	if raw[0] == '<' {
		return nil, fmt.Errorf("ledu response is HTML")
	}
	plain, err := leduAppAESDecrypt(string(raw))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(plain), &payload); err != nil {
		return plain, nil
	}
	if decoded, ok := leduMaybeDecryptPayload(payload); ok {
		return decoded, nil
	}
	return payload, nil
}

func leduMaybeDecryptPayload(payload any) (any, bool) {
	switch x := payload.(type) {
	case string:
		if plain, err := leduAppAESDecrypt(x); err == nil {
			var decoded any
			if json.Unmarshal([]byte(plain), &decoded) == nil {
				return decoded, true
			}
			return plain, true
		}
	case map[string]any:
		for _, k := range []string{"encContent", "encryptContent", "ciphertext", "cipherText"} {
			if s := firstText(x[k]); s != "" {
				if plain, err := leduAppAESDecrypt(s); err == nil {
					var decoded any
					if json.Unmarshal([]byte(plain), &decoded) == nil {
						return decoded, true
					}
					return plain, true
				}
			}
		}
	}
	return nil, false
}

func leduPKCS7Pad(data []byte, blockSize int) []byte {
	n := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+n)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(n)
	}
	return out
}

func leduPKCS7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid PKCS7 data length")
	}
	n := int(data[len(data)-1])
	if n == 0 || n > blockSize || n > len(data) {
		return nil, fmt.Errorf("invalid PKCS7 padding")
	}
	for _, b := range data[len(data)-n:] {
		if int(b) != n {
			return nil, fmt.Errorf("invalid PKCS7 padding byte")
		}
	}
	return data[:len(data)-n], nil
}

func leduNonce(length int) string {
	if length <= 0 {
		length = 8
	}
	sum := md5.Sum([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	s := hex.EncodeToString(sum[:])
	if length > len(s) {
		length = len(s)
	}
	return s[:length]
}

func leduClientTraceID() string {
	a := leduNonce(8)
	b := leduNonce(4)
	c := leduNonce(4)
	d := leduNonce(4)
	e := leduNonce(12)
	return a + "-" + b + "-" + c + "-" + d + "-" + e
}

func leduTraceID() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10) + leduNonce(8)
}

func browserVersion() string { return "7.76.91" }
