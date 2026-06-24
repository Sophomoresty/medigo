# houda 源码对齐对照

## URL 常量

| .cdc.py 行 | houda.go 行/名 | 一致? |
|---|---|---|
| Houda_Base.py:31-32 `referer = 'http://www.houdask.com/'`, `origin = 'http://www.houdask.com'` | houda.go:28-29 `urlOrigin`, `urlHome` | ✓ |
| Houda_Course.py:35-41 父站课程/课节/CSSL 回放 API | houda.go:31-36 `urlCourseList` / `urlStageLaw` / `urlLearnCourse` / `urlCCViewPlayback` | ✓, `{room_id}/{record_id}` -> `%s/%s` |
| Houda_Course.py:42-45 CSSLCloud replay URL | houda.go:37-40 `urlCsslLogin` / `urlCsslPlay` / `urlCsslMeta` / `urlCsslOrigin` | ✓ |
| Houda_Course.py:46-50 CSSLCloud device/tpl/terminal/material type | houda.go:41-45 `csslDeviceType` / `csslDeviceVersion` / `csslTpl` / `csslTerminal` / `materialServiceTyp` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Houda_Base._check_cookie line 147, `ifLogin` 解析 `code == '1'` | houda.go:113 `checkHoudaCookie` | GET | ✓ |
| Houda_Course._get_lesson_list lines 289-295, `getLearnCourse` + `classId` | houda.go:131 `fetchHoudaLessons` | POST form | ✓ |
| Houda_Course._resolve_cc_callback_url / _parse_cc_info decrypted consts `room_id`, `recordId`, `viewertoken` | houda.go:256 `resolveHoudaCCCallback` | GET | ✓ |
| Houda_Course._get_cc_session / _get_cc_video_url decrypted chain | houda.go:228 `resolveHoudaCSSL` -> `shared.CssLcloudResolvePlayInfo` | POST+GET helper | ✓ |
| Houda_Course._download_media_url decrypted `.m3u8` branch | houda.go:296 `rewriteHoudaM3U8` -> `shared.CssLcloudRewriteM3U8Keys` | GET | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct tag | 一致? |
|---|---|---|
| result.get('data', {}).get('liveList', []) | `Data.LiveList` `json:"liveList"` in houda.go:144-146 | ✓ |
| lesson.get('title'/'name'/'courseName') | `Title/Name/CourseName` `json:"title"/"name"/"courseName"` in houda.go:162-164 | ✓ |
| lesson.get('ccLiveId'/'roomId'/'mainRoomId'/'recordId') | `CCLiveID/RoomID/MainRoomID/RecordID` tags in houda.go:168-171 | ✓ |
| callback query `userId`, `roomId`, `recordId`, `viewerToken` | `resolveHoudaCCCallback` query keys in houda.go:268-273 | ✓ |

## 阻塞步骤

无.
