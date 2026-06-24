# wendao 源码对齐对照

## URL 常量

| .cdc.py 行 | wendao.go 行/名 | 一致? |
|---|---|---|
| Wendao_Base.py:32 `pc_referer = 'https://pc.wendao101.com/'` | wendao.go:19 `pcReferer` | ✓ |
| Wendao_Base.py:33 `pc_origin = 'https://pc.wendao101.com'` | wendao.go:20 `pcOrigin` | ✓ |
| Wendao_Base.py:34 `wap_referer = 'https://wap.wendao101.com/'` | wendao.go:21 `wapReferer` | ✓ |
| Wendao_Base.py:35 `wap_origin = 'https://wap.wendao101.com'` | wendao.go:22 `wapOrigin` | ✓ |
| Wendao_Base.py:38 `login_url = 'https://wap.wendao101.com/#/pages_mine/myCourse/myCourse'` | wendao.go:23 `loginURL` | ✓ |
| Wendao_Base.py:39 `api_host = 'https://pc.wendao101.com/prod-api'` | wendao.go:24 `apiHost` | ✓ |
| Wendao_Base.py:40 `wap_api_host = 'https://wap.wendao101.com'` | wendao.go:25 `wapAPIHost` | ✓ |
| Wendao_Base.py:41-43 app/order platform constants | wendao.go:26-28 `appNameType`, `defaultOrderPlatform`, `wapOrderPlatform` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Wendao_Base._request_json line 392 | wendao.go:145 `requestJSON` | POST JSON | ✓ |
| Wendao_Base._request_api_data line 453 | wendao.go:131 `requestData` | unwrap `code` + `data` | ✓ |
| Wendao_Base._request_wap_api_data line 470 | wendao.go:131 `requestData` with WAP host | POST JSON | ✓ |
| Wendao_Base._check_cookie line 487 | wendao.go:50 `Extract`, wendao.go:180 `headers` | token / Authorization / openId headers | ✓ |
| Wendao_Course._request_course_page line 261 | wendao.go:95 `firstCourse` | POST `/wap/home_page/course/purchased` then PC fallback | ✓ |
| Wendao_Course._load_detail line 380 | wendao.go:115 `loadDetail` | POST `/wap/course/detail` then PC fallback | ✓ |
| Wendao_Course._get_infos line 560 | wendao.go:161 `lessonsFromDetail` | parse course detail lesson lists | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct tag / 代码 | 一致? |
|---|---|---|
| `_request_json`: JSON response body | wendao.go:155 `json.Unmarshal` | ✓ |
| `_request_api_data`: `code in (0,200,'0','200')`, `data` | wendao.go:135-141 | ✓ |
| `_extract_auth_info`: `token/Admin-Token/adminToken/Authorization/accessToken/access_token/Access-Token`, `openId/openid/OpenId` | wendao.go:189-216 | ✓ |
| `_request_course_page`: `appNameType`, `pageSize`, `pageNum`, `orderPlatform`, `openId` | wendao.go:96-100 | ✓ |
| `_get_course_list`: `list/rows/records/data`, `courseId/course_id/id`, `title/courseTitle/courseUploadTitle/name` | wendao.go:103-109 recursive data walk | ✓ |
| `_load_detail`: `needReferer`, `dataId`, `platform`, `appNameType`, `tempSeeSecret`, `openId`, `courseId` | wendao.go:116-120 | ✓ |
| `_build_lesson_info`: `id/courseDirectoryId/directoryId`, `courseDirectoryUrl/studyFileUrl/videoUrl/audioUrl/fileUrl/url`, `directoryType`, `directoryName/studyFileName/name/title` | wendao.go:161-172 | ✓ |
| `_download_media_lesson`: media URL extension `.m3u8/.mp4/.mp3/.m4a/.aac/.wav` | wendao.go:251-272 `isMediaURL` / `mediaFormat` | ✓ |

## 阻塞步骤

无.
