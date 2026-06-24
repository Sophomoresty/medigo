# xuelang 源码对齐对照

## URL 常量

| .cdc.py / .das 行 | xuelang.go 行/名 | 一致? |
|---|---|---|
| Xuelang_Base.py:34 `referer = 'https://student-api.iyincaishijiao.com'` | xuelang.go:19 `refererURL` | ✓ |
| Xuelang_Base.py:35 `order_url = 'https://student-api.iyincaishijiao.com/ep/trade/v2/order/list?anchor={anchor:}&count=50'` | xuelang.go:20 `orderURL` | ✓ |
| Xuelang_Base.py:275 profile `https://student-api.iyincaishijiao.com/ep/user/profile/` | xuelang.go:21 `profileURL` | ✓ |
| Xuelang_Course.py:41 `course_url = 'https://student-api.iyincaishijiao.com/ep/student/learn_data_v2/?course_count=999'` | xuelang.go:22 `courseURL` | ✓ |
| Xuelang_Course.py:42 `info_url = 'https://student-api.iyincaishijiao.com/ep/study_pc/course/lessons/?cursor={cursor:}&course_id={course_id:}&count=99&version_code=1.9.2.0&aid=4783&msToken={ms_token:}'` | xuelang.go:23 `infoURL` | ✓ |
| Xuelang_Course.py:43 `live_url = 'https://classroom.iyincaishijiao.com/classroom/playback/v1/enter_playback/?aid=2989'` | xuelang.go:24 `liveURL` | ✓ |
| Xuelang_Course.py:44 `m3u8_url = 'https://vod.bytedanceapi.com/?'` | xuelang.go:25 `m3u8URL` | ✓ |
| Xuelang_Course.py:45 `key_url = 'https://student-api.iyincaishijiao.com/video/drm/v1/play_licenses'` | xuelang.go:26 `keyURL` | ✓ |
| Xuelang_Course.py:46 `source_url = 'https://student-api.iyincaishijiao.com/ep/student/course_resource/?course_id={cid:}&token={token:}&count=999'` | xuelang.go:27 `sourceURL` | ✓ |
| Xuelang_Course.py:47 `file_url = 'https://student-api.iyincaishijiao.com/ep/student/preview_course_resource/?token={token:}&course_id={cid:}'` | xuelang.go:28 `fileURL` | ✓ |
| Xuelang_Course.py:48 `token_url = 'https://api.juejin.cn/user_api/v1/video/key_token'` | xuelang.go:29 `tokenURL` | ✓ |
| Xuelang_Course.py:49 `v3_key_url = 'https://kds.bytedance.com/kds/api/v3/keys?source=jarvis&ak={kid:}&token={token:}'` | xuelang.go:30 `v3KeyURL` | ✓ |
| Xuelang_Course.py:137 `https://mssdk.bytedance.com/web/common?msToken=` | xuelang.go:31 `msTokenURL` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| `Xuelang_Base._check_cookie` line 275 | xuelang.go:54-68 `Extract` | GET + regex | ✓ |
| `Xuelang_Course._get_course_list` line 47 | xuelang.go:98-119 `fetchCourses` | GET + JSON | ✓ |
| `Xuelang_Course._get_ms_token` line 89 | xuelang.go:157-165 `getMSToken` | POST + header parse | ✓ |
| `Xuelang_Course._get_infos` line 98 | xuelang.go:121-155 `fetchLessons` | GET + JSON | ✓ |
| `Xuelang_Course._get_live_token` line 144 | xuelang.go:186-197 `getLiveTokens` | POST JSON + JSON | ✓ |
| `Xuelang_Course._download_video` line 255 | xuelang.go:167-184 `resolveLesson` | base64 JSON parse | ✓ |
| `Xuelang_Course._get_m3u8_info` line 158 | xuelang.go:199-235 `getM3U8Info` | GET + JSON | ✓ |
| `Xuelang_Course._decrypt_m3u8_key` line 198 | xuelang.go:237-259 `decryptM3U8Key` | GET + regex | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 解析 | 一致? |
|---|---|---|
| `_check_cookie`: regex `"status_code":0` | xuelang.go:38, 62-67 | ✓ |
| `_get_course_list`: `data.student_course.data[].course_info.course_id/title` | xuelang.go:103-113 | ✓ |
| `_get_infos`: `data.data[]`, `forward_cursor.has_more/cursor` | xuelang.go:133-150 | ✓ |
| `_get_infos`: `lesson_info.related_room_id_str`, `lesson_info.video.play_auth_token`, `lesson_info.title` | xuelang.go:137-144 | ✓ |
| `_download_video`: base64 JSON `GetPlayInfoToken` | xuelang.go:167-175, 261-271 | ✓ |
| `_get_live_token`: `teacher_video_info.play_auth_token`, `external_video_infos[0].video.play_auth_token` | xuelang.go:186-197 | ✓ |
| `_get_m3u8_info`: `Result.Data.PlayInfoList[]`, `MainPlayUrl`, `BackupPlayUrl`, `Size`, `PlayAuthID`, `MediaType=audio`, `VideoID` | xuelang.go:199-235 | ✓ |
| `_decrypt_m3u8_key`: regex `"data"\s*:\s*"(.*?)"` from `token_url` and `v3_key_url` | xuelang.go:237-259 | ✓ |

## 阻塞步骤

无. 已按源码保留 `msToken` 请求与 `a_bogus` 参数位; HTTP 链请求 lessons API, 解析同一 JSON 字段并返回可播放 URL.
