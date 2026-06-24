# jinbangshidai 源码对齐对照

## URL 常量

| .cdc.py 行 | Go 行/名 | 一致? |
|---|---|---|
| `Jinbangshidai_Course.py:34 referer = 'https://pc.vkbrother.com'` | `jinbangshidai.go:19 referer` | ✓ |
| `Jinbangshidai_Course.py:35 api_base = 'https://app.vkbrother.com'` | `jinbangshidai.go:20 api_base` | ✓ |
| `Jinbangshidai_Course.py:37 student_info_url = api_base + '/app/student/info'` | `jinbangshidai.go:22 student_info_url` | ✓ |
| `Jinbangshidai_Course.py:38 course_list_url = api_base + '/app/course/me/courseList'` | `jinbangshidai.go:23 course_list_url` | ✓ |
| `Jinbangshidai_Course.py:39 room_course_list_url = api_base + '/app/room/jbCourseList'` | `jinbangshidai.go:24 room_course_list_url` | ✓ |
| `Jinbangshidai_Course.py:40 course_info_url = api_base + '/app/course/v2/info'` | `jinbangshidai.go:25 course_info_url` | ✓ |
| `Jinbangshidai_Course.py:41 course_play_url = api_base + '/app/course/v2/coursePlay'` | `jinbangshidai.go:26 course_play_url` | ✓ |
| `Jinbangshidai_Course.py:42 video_token_url = api_base + '/app/bjvod/videoPlayerToken'` | `jinbangshidai.go:27 video_token_url` | ✓ |
| `Jinbangshidai_Course.py:43 room_playback_token_url = api_base + '/app/bjvod/getPlaybackToken'` | `jinbangshidai.go:28 room_playback_token_url` | ✓ |
| `Jinbangshidai_Course.py:44 video_play_url = 'https://api.baijiayun.com/web/playback/getPlayInfo?...'` | `jinbangshidai.go:29 video_play_url` + `shared.BaijiayunResolvePlayback` | ✓ |
| `Jinbangshidai_Course.py:45 live_play_url = 'https://www.baijiayun.com/vod/video/getPlayUrl?...'` | `jinbangshidai.go:30 live_play_url` + `shared.BaijiayunResolveVOD` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---:|---|
| `_request_json` lines 214-222 via `request_json` | `postJSON` `jinbangshidai.go:132-157` | POST JSON + parse | ✓ |
| `_check_cookie` lines 227-242 | `checkCookie` `jinbangshidai.go:121-129` | POST JSON | ✓ |
| `_get_course_list` lines 291-320 | `getCourseList` `jinbangshidai.go:159-188` | POST JSON | ✓ |
| `_get_course_detail` lines 326-355 | `getCourseDetail` `jinbangshidai.go:207-222` | POST JSON | ✓ |
| `_get_course_play` lines 361-387 | `getCoursePlay` `jinbangshidai.go:224-240` | POST JSON | ✓ |
| `_get_infos` lines 500-538 | `loadInfos` `jinbangshidai.go:242-277` | JSON traversal | ✓ |
| `_request_video_token` lines 615-632 | `requestVideoToken` `jinbangshidai.go:355-365` | POST JSON | ✓ |
| `_request_room_playback_token` lines 639-652 | `requestRoomPlaybackToken` `jinbangshidai.go:367-377` | POST JSON | ✓ |
| `_download_video` lines 737-756 | `resolveVideo` `jinbangshidai.go:314-353` | token + Baijiayun helper | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct/tag 或解析 | 一致? |
|---|---|---|
| cookie `ToKen/token/Token/TOKEN`, JWT `deviceId` | `extractCookieToken` / `decodeJWTPayload` `helpers.go:110-189` | ✓ |
| `student_info_url -> code` | `checkCookie` `jinbangshidai.go:121-129` | ✓ |
| `courseList[] / jbCourseSelfVOS[] -> cid/courseId/name/title/price/oldPrice/realPrice` | `normalizeCourseInfo` `helpers.go:15-22` | ✓ |
| `course_info_url -> code/info.name/info.title/info.price/info.oldPrice/info.realPrice/buyed` | `getCourseDetail` `jinbangshidai.go:207-222` | ✓ |
| `course_play_url -> deviceType/deviceId/courseId -> syllabusList/info` | `getCoursePlay` + `loadInfos` `jinbangshidai.go:224-253` | ✓ |
| `syllabusList[].list/type/name/title/docName/url/videoId/roomString` | `collectSyllabusResources` + `makeResourceInfo` `helpers.go:24-57` | ✓ |
| `video_token_url -> data.token` | `requestVideoToken` `jinbangshidai.go:355-365` | ✓ |
| `room_playback_token_url -> data.token` | `requestRoomPlaybackToken` `jinbangshidai.go:367-377` | ✓ |
| Baijiayun JSONP `data.video_url/data.video[]/data.playback_url` | `shared.BaijiayunResolveVOD` / `shared.BaijiayunResolvePlayback` | ✓ |

## 阻塞步骤

无。Baijiayun VOD 和回放解析按任务要求走 `internal/extractor/shared/baijiayun.go`，站点包只负责父站 token, 课程树和资源字段解析。
