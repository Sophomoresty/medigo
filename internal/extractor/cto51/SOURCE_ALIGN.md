# cto51 源码对齐对照

## URL 常量

| .cdc.py 行 | cto51.go 行/名 | 一致? |
|---|---|---|
| `Cto51_Base.pyc.1shot.cdc.py:31-32` `referer/study_url` | `helpers.go:29`, `cto51.go:22` | ✓ |
| `Cto51_Course.pyc.1shot.cdc.py:55-65` course/lesson/API/qcloud URL | `cto51.go:26-36` | ✓ |
| `Cto51_Course.pyc.1shot.cdc.py:66-73` training/order URL | `cto51.go:37-44` | ✓ |
| `Cto51_Course.pyc.1shot.cdc.py:75-79` regex for lesson/train/aliplayparam | `cto51.go:60-69` | ✓ |

## HTTP 调用

| 源码方法 | Go 函数 | method | 一致? |
|---|---|---|---|
| `_request_json_get` `1497-1510` | `fetchJSONPayloads` `190-205` | GET + JSON | ✓ |
| `_request_text` `1517-1531` | `resolveLesson/resolveCourse` `130-149`, `93-128` | GET HTML | ✓ |
| `_fetch_lesson_page_payloads` `3342-3379` | `resolveCourse` `93-128` + `urlLessonListAPI` | GET + JSON | ✓ |
| training fetch `_fetch_training_course_payloads` `2164-2191` | `resolveTraining` `152-172` | GET + JSON | ✓ |
| `_request_qcloud_play_info` / `qcloud_play_api` | `resolveAuth` `259-272` | GET qcloud playinfo | ✓ |

## JSON 字段映射

| 源码 key 链 | Go parse | 一致? |
|---|---|---|
| `data.lessonList` / `lesson_list` / nested lesson nodes | `collectMedia` recursive maps | ✓ |
| `lesson_id/lessonId`, `lesson_name/lessonName`, `title/name` | `collectMedia` title and route extraction | ✓ |
| `var aliplayparam = {...}` | `parseAliPlayParam` | ✓ |
| `app_id/appId`, `file_id/fileId/fileID/vid`, `psign/pSign/playAuth/sign/token` | `authFromMap` + `decodePlayAuth` | ✓ |
| `play_url/playUrl`, `video_url/videoUrl`, `m3u8`, `file_url/fileUrl`, `path` | `collectMedia` + `mediaFromText` | ✓ |

## 阻塞步骤

无。
