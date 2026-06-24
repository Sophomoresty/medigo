# huatu 源码对齐对照

## URL 常量

| .cdc.py 行 | Go 行/名 | 一致? |
|---|---|---|
| Huatu_Base.py:32 `referer = 'https://www.huatu.com/'` | `huatu.go:17` `referer` | ✓ |
| Huatu_Base.py:33 `check_url = 'https://ns.huatu.com/u/v3/member/user/icon'` | `huatu.go:18` `check_url` | ✓ |
| Huatu_Base.py:34 `course_check_url = 'https://ocfapi.huatu.com/api/user/my_course'` | `huatu.go:19` `course_check_url` | ✓ |
| Huatu_Course.py:35 `my_course_url = 'https://ocfapi.huatu.com/api/user/my_course'` | `huatu.go:21` `my_course_url` | ✓ |
| Huatu_Course.py:36 `syllabus_url = 'https://ocfapi.huatu.com/api/goods/syllabusBuy'` | `huatu.go:22` `syllabus_url` | ✓ |
| Huatu_Course.py:37 `player_url = 'https://ocfapi.huatu.com/api/course/goods/get_player'` | `huatu.go:23` `player_url` | ✓ |
| Huatu_Course.py:38 `vod_info_url = 'https://playvideo.vodplayvideo.net/getplayinfo/v4/{app_id}/{file_id}?psign={psign}'` | `huatu.go:24` `vod_info_url` with `%s` placeholders | ✓ |
| Huatu_Config.py:10 `USER_AGENT = ...Chrome/120...` | `huatu.go:26` `USER_AGENT` | ✓ |
| Mooc_Config.py:188 Huatu course regex accepts `goodsNum`, `v.huatu.com`, and huatu URLs | `huatu.go:30-34` `patterns`, `goodsNumRe`, `courseDetailRe`, `fallbackCIDRe` | ✓ |

## 认证与 Header

| 源码方法/常量 | Go 函数/行 | 一致? |
|---|---|---|
| Huatu_Config.py:18-25 `HUATU_HEADERS`: `terminal=3`, `Channel-Alias=ht_pc`, Referer, Origin, Accept, User-Agent | `huatu.go:104-114` | ✓ |
| `Huatu_Base._parse_cookie_dict` line 111 | `helpers.go:33-43` `parseCookieHeader` | ✓ |
| `Huatu_Base._apply_token_headers` line 121, token aliases `ht_token_preview`, `ht_token`, `token`, `Newuc-Token` | `helpers.go:84-92` `applyTokenHeaders` | ✓ |
| `Huatu_Base._get_cookie_token` line 128 token key order | `helpers.go:46-49` `cookieToken` | ✓ |
| `Huatu_Base._normalize_cookie_token_aliases` line 138 | `helpers.go:51-82` `normalizeCookieTokenAliases` | ✓ |
| `Huatu_Base._build_cookie_header` line 167 | `helpers.go:14-31` `cookieHeader`; `huatu.go:89-103` source origins | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| `Huatu_Base._request_json_get` line 67: URL encode params, GET, `.json()` | `huatu.go:197-226` `getJSON` | GET | ✓ |
| `Huatu_Course._get_course_list` line 402: `my_course_url`, paged params, `data/list` | `course.go:17-29`, `course.go:52-95` | GET | ✓ |
| `Huatu_Course._get_syllabus_items` line 223: `goodsNum`, `level`, `page`, pagination keys | `course.go:125-155`; `helpers.go:199-206` | GET | ✓ |
| `Huatu_Course._get_infos` line 670: walk syllabus levels, append video/file sources | `course.go:157-216` | GET chain | ✓ |
| `Huatu_Course._get_video_source` line 907: `player_url` with `goodsNum` + `lessonId` | `play.go:68-95` | GET | ✓ |
| `Huatu_Course._extract_baijiayun_play_source` line 981 | `play.go:98-178`, using `shared.BaijiayunResolveVOD` / `shared.BaijiayunResolvePlayback` | GET helper | ✓ |
| `Huatu_Course.vod_info_url` Tencent VOD fallback, raw `.format(app_id, file_id, psign)` | `play.go:83-95` | GET | ✓ |
| `Huatu_Course._load_final_m3u8_text` line 776 | `play.go:212-228` | GET | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct/tag or parser | 一致? |
|---|---|---|
| API root `code`, `msg`, `data`, `media` | `huatu.go:64-69` `apiResp` | ✓ |
| success code `(10000, 1000000)` | `huatu.go:228-231` `successCode` | ✓ |
| URL target keys `goodsNum/goodsNo/goodsId/courseId/course_id`, title keys, `stageId`, `modularId`, `lessonId` | `huatu.go:143-195` `parseTarget` | ✓ |
| response `data`, nested `data.data`, nested `data.list` | `course.go:107-123` `dataPayload` | ✓ |
| course ids `goodsNum`, `goodsNo`, `goodsId`, `courseId`; title `title/name/goodsName`; expired/price filters | `course.go:69-83`, `source_helpers.go:107-212` | ✓ |
| pagination `pageCount`, `last_page`, `totalPage`, `total_page`, `pages` | `helpers.go:199-206` `pageCount` | ✓ |
| syllabus video fields `lessonId`, `clazzLessonId`, `level`, `videoId`, `modularId` | `source_helpers.go:70-82` `lessonID` | ✓ |
| file keys, nested file arrays/nodes, and `parse.quote(file_url, safe=":/?=&%")` | `source_helpers.go:11-15`, `source_helpers.go:27-68`, `source_helpers.go:224-269`; `course.go:227-244` | ✓ |
| player fields `data.appId`, `data.videoId`, `data.token` | `play.go:83-89` | ✓ |
| VOD `media.basicInfo.size`, `media.streamingInfo.drmToken`, `drmOutput/plainOutput[].url` | `play.go:180-201` | ✓ |
| fallback `adaptive_streaming/video_list[].url` | `play.go:202-208` | ✓ |
| HLS highest `BANDWIDTH` variant and `EXT-X-KEY URI` token rewrite | `play.go:230-286`, `source_helpers.go:213-222` | ✓ |
| Baijiayun `playbackUrl/playUrl/url/videoUrl`, `roomId/classId/.../vid`, token aliases | `play.go:116-178` | ✓ |

## 返回结构

| 源码行为 | Go 实现 | 一致? |
|---|---|---|
| 课程多视频/资料列表 | `play.go:21-50` returns top-level `MediaInfo.Entries` | ✓ |
| 视频条目携带流 Header/Cookie | `play.go:37-43`, `play.go:64-66` | ✓ |
| 资料条目保留文件 URL 和 Referer/User-Agent/Cookie | `play.go:52-61` | ✓ |

## 阻塞步骤

无。
