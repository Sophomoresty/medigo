# wangxiao233 源码对齐对照

## URL 常量

| .cdc.py 行 | wangxiao233.go 行/名 | 一致? |
|---|---|---|
| Wangxiao233_Base.py:32 `referer = 'https://wx.233.com/'` | wangxiao233.go:23 `refererURL` | ✓ |
| Wangxiao233_Base.py:33 `login_url = 'https://passport.233.com/login/?redirecturl=...'` | wangxiao233.go:24 `loginURL` | ✓ |
| Wangxiao233_Base.py:34 `user_info_url = 'https://japi.233.com/ess-ucs-api/doz/members/userInfo'` | wangxiao233.go:25 `urlUserInfo` | ✓ |
| Wangxiao233_Course.py:37 `course_url` | wangxiao233.go:26 `urlVktCourse` | ✓ |
| Wangxiao233_Course.py:38 `user_course_url` | wangxiao233.go:27 `urlUserCourse` | ✓ |
| Wangxiao233_Course.py:39 `buy_domain_url` | wangxiao233.go:28 `urlBuyDomain` | ✓ |
| Wangxiao233_Course.py:40 `tag_url` | wangxiao233.go:29 `urlTag` | ✓ |
| Wangxiao233_Course.py:41 `version_url` | wangxiao233.go:30 `urlVersion` | ✓ |
| Wangxiao233_Course.py:42 `chapter_url` | wangxiao233.go:31 `urlChapter` | ✓ |
| Wangxiao233_Course.py:47-50 VOD / playAuth URLs | wangxiao233.go:36-39 `urlVodDetail` / `urlVodPoly` / `urlVodEss` / `urlPlayAuth` | ✓ |
| Wangxiao233_Course.py:54 `polyv_token_url` | wangxiao233.go:43 `urlPolyvToken` | ✓ |
| Wangxiao233_Course.py:55 `polyv_secure_url = 'https://player.polyv.net/secure/{vid}.json'` | wangxiao233.go:44 `urlPolyvSecure`, `{vid}` → `%s` | ✓ |
| Wangxiao233_Course.py:56 `polyv_key_url` | wangxiao233.go:45 `urlPolyvKey`, placeholders → `%s` | ✓ |
| Wangxiao233_Config.py:29 `WANGXIAO233_SECRET = 'RZRRNN9RXYCP'` | wangxiao233.go:46 `signSecret` | ✓ |
| Wangxiao233_Config.py:30 `WANGXIAO233_SID_PRE = 'study'` | wangxiao233.go:47 `sidPrefix` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Wangxiao233_Base._check_cookie line 413 → `user_info_url` | wangxiao233.go:64 `Extract` / line 74 | GET signed JSON | ✓ |
| Wangxiao233_Course._api_get line 77 | wangxiao233.go:127 `apiGet` | GET signed query | ✓ |
| Wangxiao233_Course._api_post line 106 | wangxiao233.go:138 `apiPost` | POST compact JSON + signed header | ✓ |
| Wangxiao233_Base._signed_header line 369 | wangxiao233.go:148 `signedHeaders` | token/sid/sign headers | ✓ |
| Wangxiao233_Course._get_tag_info line 593 | wangxiao233.go:85 `urlTag` call | GET | ✓ |
| Wangxiao233_Course._get_version_info line 729 | wangxiao233.go:195 `fillVersion` | GET `urlVersion` | ✓ |
| Wangxiao233_Course._get_chapter_list line 817 | wangxiao233.go:94 `urlChapter` call | GET | ✓ |
| Wangxiao233_Course._get_video_play_info line 1162 | wangxiao233.go:225 `resolveVideo` | GET VOD detail/poly/ess path | ✓ |
| Wangxiao233_Course._get_polyv_play_source line 1814 | wangxiao233.go:250 `resolvePolyv` | GET token + shared PolyV secure | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct tag / 代码 | 一致? |
|---|---|---|
| `_api_get` / `_safe_json_loads`: response JSON object | wangxiao233.go:273 `json.Unmarshal` | ✓ |
| `_check_cookie`: `status`, `code`, cookie `clientauthentication`, header `token` | wangxiao233.go:64-78, 340-349 | ✓ |
| `_parse_url_info`: query `domain`, `systemType`, `lmProductId`, `teacherId`, `versionProductId`, `childProductId`, `productId` | wangxiao233.go:155-165 `parseCourse` | ✓ |
| `_get_child_course_list`: `thirdLevelLabelList[].productCourseRspList[]`, `childProductId`, `productId`, `versionProductId`, `currentProductId`, `teacherId`, `courseName`, `childProductName` | wangxiao233.go:183-193 `childCourses` | ✓ |
| `_get_version_info`: `courseTeacherList[]`, `versionId`, `courseId`, `productId`, `childProductId`, `teacherId` | wangxiao233.go:195-212 `fillVersion` | ✓ |
| `_get_chapter_list`: `data.courseChapterRspList`, `isBuy`, `learnCourseOrderRsp`, `coursePdfLectureId` | wangxiao233.go:94-101 + recursive `collectVideos` | ✓ |
| `_parse_video_info`: `detailName/name/title`, `detailId/id`, `polyVid/polyvVid`, `essVid`, `aliyunVid/aliyunVideoId`, `mp3Url` | wangxiao233.go:215-223 `collectVideos` | ✓ |
| `_get_video_play_info`: `detailIds`, `polyVid`, `essVid`, `playChannel/channel` | wangxiao233.go:225-238 `resolveVideo` | ✓ |
| `_get_polyv_play_source`: JSONP `s`, `list`, `token/playsafe` and PolyV secure path | wangxiao233.go:250-267 `resolvePolyv` + `shared.PolyvResolveSecure` | ✓ |
| `_extract_aliyun_play_response`: `PlayURL/PlayUrl/playUrl/url/source/videoUrl` | wangxiao233.go:298-307 `firstMediaURL` | ✓ |

## 阻塞步骤

无.

## R2 critical follow-up

| 缺口 | 处理结果 |
|---|---|
| Aliyun VOD `getPlayInfoAndAuth` 后续签名流程 | `resolveVideo` 现在解码 `data.playAuth`, 规范化 `regionId`, 调用 `shared.AliyunResolvePlayInfo` 生成 VOD `GetPlayInfo` HMAC-SHA1 签名请求. |
| Aliyun MTS key/license | shared helper 在 m3u8 加密时抓取 manifest key token 并请求 `mts.{region}.aliyuncs.com` `GetLicense`, 成功后写入 `Extra.m3u8_text`; 失败返回 `blocked: needs Aliyun STS SDK / DRM engine`, 不再假成功. |
