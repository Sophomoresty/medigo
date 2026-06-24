# xsteach 源码对齐对照

## URL 常量

| .cdc.py 行 / 常量 | Go 行 / 名称 | 一致? |
|---|---|---|
| Xsteach_Base.py:29 `login_check_url = 'https://www.xsteach.com/api/user/my-course/list-v3'` | `xsteach.go:24` `loginCheckURL` / `courseListURL` | ✓ |
| Xsteach_Course.py:46 `course_combobox_url = 'https://www.xsteach.com/api/common/my-course-combobox'` | `xsteach.go:26` `courseComboboxURL` | ✓ |
| Xsteach_Course.py:47 `course_detail_url = 'https://www.xsteach.com/api/course/course-detail'` | `xsteach.go:27` `courseDetailURL` | ✓ |
| Xsteach_Course.py:48 `period_url = 'https://www.xsteach.com/api/course/period'` | `xsteach.go:28` `periodURL` | ✓ |
| Xsteach_Course.py:49 `period_play_list_url = 'https://www.xsteach.com/api/period/get-period-list'` | `xsteach.go:29` `periodPlayListURL` | ✓ |
| Xsteach_Course.py:50 `video_play_url = 'https://www.xsteach.com/api/vod/period/play'` | `xsteach.go:30` `videoPlayURL` | ✓ |
| Xsteach_Course.py:51 `teach_coach_play_url = 'https://www.xsteach.com/api/vod/teach-coach/play'` | `xsteach.go:31` `teachCoachPlayURL` | ✓ |
| Xsteach_Course.py:52 `live_play_url = 'https://www.xsteach.com/api/live/enter/play'` | `xsteach.go:32` `livePlayURL` | ✓ |
| Xsteach_Course.py:53 `qcloud_play_api = 'https://playvideo.qcloud.com/getplayinfo/v4/{}/{}'` | `xsteach.go:33` `qcloudPlayAPI` | ✓ |
| Xsteach_Base.py:29-30 `referer/origin` | `xsteach.go:22-23` `refererURL` / `originURL` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 / line | method | 一致? |
|---|---|---|---|
| `Xsteach_Base._check_cookie` lines 217-230 | `xsteach.go:55-70` `Extract` + `apiGet` | GET + JSON | ✓ |
| `Xsteach_Course._get_course_list` lines 242-288 | `xsteach.go:108-135` `fetchCourses` | GET + JSON | ✓ |
| `Xsteach_Course._get_course_detail` lines 313-327 | `xsteach.go:138-146` `courseDetail` | GET + JSON | ✓ |
| `Xsteach_Course._get_period_body` lines 427-443 | `xsteach.go:149-187` `fetchPeriods`/`periodBody` | GET + JSON | ✓ |
| `Xsteach_Course._get_period_play_list_body` lines 449-464 | `xsteach.go:189-203` `enrichWithPlayList` | GET + JSON | ✓ |
| `Xsteach_Course._request_play_data` lines 1114-1144 | `xsteach.go:206-220` `requestPlayData` | GET + JSON | ✓ |
| `Xsteach_Course._request_qcloud_play_info` lines 1176-1243 | `xsteach.go:234-247`, `269-293` `resolveVideo`/`qcloudMediaURL` | GET + JSON | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 解析 | 一致? |
|---|---|---|
| `result.get('body')` / `code == 1` | `xsteach.go:65-70`, `138-146`, `174-203`, `223-247` | ✓ |
| `body` list from `my-course-combobox` | `helpers.go:15-23` + `108-135` | ✓ |
| `body.records` from `my-course/list-v3` | `helpers.go:15-23` + `108-135` | ✓ |
| `courseId`, `course_id`, `id`, `value`, `name`, `label`, `courseName`, `title`, `classScheduleId`, `lectureType` | `helpers.go:25-35` `normalizeCourse` | ✓ |
| `schedules[]`, `scheduleSeq`, `beginDate` | `helpers.go:45-63`, `149-171` | ✓ |
| `periods`, `directoryList`, `periodList`, `list`, `items` | `helpers.go:45-84` `periodsFromBody` / `flattenPeriods` | ✓ |
| `id` / `periodId`, `teachCoachId`, `teach_coach_id`, `teachingAidsId`, `teaching_aids_id` | `helpers.go:85-108`, `331-347` | ✓ |
| `appId/appID/app_id`, `fileId/fileID/file_id/videoId`, `sign/psign/pSign/playAuth` | `xsteach.go:295-305` `qcloudAuth` | ✓ |
| `masterPlayList.url`, `videoUrl`, `playUrl`, `m3u8Url`, `hlsUrl`, `mediaUrl`, `url`, `addr`, `fileUrl`, `resourceUrl` | `helpers.go:145-187` `firstMediaURL` / `media` | ✓ |

## 阻塞步骤

无.
