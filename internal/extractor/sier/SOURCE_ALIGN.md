# sier 源码对齐对照

## URL 常量

| .cdc.py 行 | sier.go 行/名 | 一致? |
|---|---|---|
| Sier_Base.py:38 `referer = 'https://player.sieredu.com/'` | `referer` | ✓ |
| Sier_Base.py:39 `user_info_api = 'https://www.sieredu.com/web/user/getUserInfo'` | `user_info_api` | ✓ |
| Sier_Course.py:36 `course_list_api = 'https://www.sieredu.com/web/uc/course/myCourse'` | `course_list_api` | ✓ |
| Sier_Course.py:37-40 plan/catalog/check/load APIs | `plan_api`, `catalog_api`, `check_play_api`, `load_play_data_api` | ✓ |
| Sier_Course.py:41-42 token APIs | `token_api`, `legacy_token_api` | ✓ |
| Sier_Course.py:43-45 open course + VOD APIs | `open_course_detail_api`, `open_course_check_api`, `getplayinfo_api` | ✓ |
| Sier_Config.py:17-20 app secret/key/token key/appid | `SIER_TOKEN_AES_KEY_B64`, `SIER_VOD_DEFAULT_APPID` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 | method | 一致? |
|---|---|---|---|
| `_get_course_list` 205-242 | `fetchCourseList` | POST | ✓ |
| `_get_open_course_detail` 349-357 | `extractOpenCourse` | POST | ✓ |
| `_get_normal_course_infos` 674-713 | `extractNormalCourse` | POST | ✓ |
| `_check_video_play` 761-784 | `resolveVideo` | POST | ✓ |
| `_get_token_info` 790-804 | `getTokenInfo` | POST JSON | ✓ |
| `_request_vod_playinfo` 825-850 | `requestVODPlayInfo` | GET | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct / map 访问 | 一致? |
|---|---|---|
| `entity` / `data` | `unwrapMap` | ✓ |
| `excellentCourseList/courseList/list/records` | `extractLists(..., ...)` | ✓ |
| `catalogList[].catalogId` | `extractNormalCourse` / `collectVideos` | ✓ |
| `resource.videoId/fileId`, `material.videoId/fileId`, `playUrl` | `collectVideos` / `directPlayURL` | ✓ |
| `sign` -> `loadPlayData` | `resolveVideo` | ✓ |
| `verificationCode`, `fileId`, `token`, `iv` | `getTokenInfo` / `decryptPsign` | ✓ |
| `media.*.url`, `drmToken`, `size` | `requestVODPlayInfo` | ✓ |

## 阻塞步骤

无.
