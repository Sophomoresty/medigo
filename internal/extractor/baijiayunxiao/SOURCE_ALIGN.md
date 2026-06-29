# baijiayunxiao 源码对齐对照

## URL 常量

| .cdc.py 行 | Go 行/名 | 一致? |
|---|---|---|
| `Baijiayun_Video.pyc.1shot.cdc.py:40-41` | `baijiayunxiao.go:24-27` `urlGetPlayInfo/urlGetPlayURL/urlHome` | ✓ |
| `Baijiayunxiao_Course.pyc.1shot.cdc.py:41-47` | `baijiayunxiao.go:29-31` `urlCourseInfo/urlToken/urlPlayToken` | ✓ |
| 自定义机构域名 `.../course/{id}?type=bjyx` | `parseCourseURL` | ✓ |

## HTTP 调用

| 源码方法 | Go 函数 | method | 一致? |
|---|---|---|---|
| `Baijiayun_Video._resolve_live_enter_url` `221-262` | `resolveLiveEnter` `205-235` | POST JSON | ✓ |
| `Baijiayun_Video._get_play_info_data` `319-345` | `shared.BaijiayunResolvePlayback/ResolveVOD` `67-113` | GET | ✓ |
| `Baijiayunxiao_Course._get_infos` `1278-1363` | `resolveCourse` `130-163` | GET + parse | ✓ |
| `Baijiayunxiao_Course._get_token` `1368-1393` | `fetchLessonToken` `166-184` | GET + regex/JSON | ✓ |

## JSON / 结构映射

| 源码 key 链 | Go struct / parse | 一致? |
|---|---|---|
| `data.periods`, `data.chapter` | `courseInfoResponse.Data.Periods/Chapter` | ✓ |
| `id`, `video_id`, `room_id`, `title`, `name`, `periods_title`, `child`, `children` | `courseNode` tags | ✓ |
| `token`, `video_id`, `room_id`, `classid` | `playTokenResponse` tags + regex fallback | ✓ |
| `data.play_info`, `data.video_url`, `data.playback_url`, `data.video[].url` | `shared.BaijiayunPlaybackResponse` | ✓ |
| `mp4/m3u8/ev1/ev2` 媒体格式 | `pickFormat` + `mediaInfo` | m3u8 自动 `NeedMerge` |

## 云端课堂 Yunduan_Course.py

| Python 参考 | Go 实现 | 说明 |
|---|---|---|
| `Yunduan_Course.entry_url` | `yunduanEntryURL` + `discoverYunduanDomain` | 支持 `www.baijiayun.com/entry` 页面/Cookie 中发现 `*.at.baijiayun.com` 域名 |
| `account_url` | `validateYunduanLogin` | 使用 `ORGSUPERSESSID` 校验机构后台登录态 |
| `course_list_url` | `fetchYunduanCourses` | 拉取 `/org/course_playback/getCourseList` 并构造课程候选 |
| `course_lesson_url`, `api_lesson_url`, `course_recent_url` | `getYunduanCourseLessons` + `appendYunduanRecentLessonsIfIncomplete` | 合并课程课时和近期回放补全 |
| `long_room_url`, `long_lesson_url`, `short_lesson_url`, `class_recent_url` | `fetchYunduanCourses` + `getYunduanCourseLessons` | 支持长期班课房间和长/短期班课回放 |
| `_extract_room_token` | `extractYunduanRoomToken` | 从字段或 `play_url` query 中提取 `room_id/classid` 与 token |
| 父类百家云回放解析 | `resolvePlayback` + `shared.BaijiayunResolvePlayback` | 复用百家云 `getPlayInfo` 回放链路 |


## 阻塞步骤

无。
