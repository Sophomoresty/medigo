# gaotu 源码对齐对照

## URL 常量

| .cdc.py 行 | gaotu.go 行/名 | 一致? |
|---|---|---|
| Gaotu_Course.py:37 `course_url = 'https://api.gaotu.cn/studyPlatform/v1/unit/clazz/list?isDebounce=true&os=h5-pc&p_client=1'` | `course_url` | ✓ |
| Gaotu_Course.py:38 `info_url = 'https://interactive.gaotu.cn/live/api/studyCenter/v1/user/pc/clazz/detail'` | `info_url` | ✓ |
| Gaotu_Course.py:39 `video_url = 'https://api.gaotu.cn/live/zplan/login/videoLive'` | `video_url` | ✓ |
| Gaotu_Course.py:40 `live_url = 'https://interactive.gaotu.cn/live/api/live/zplan/playbackWeb'` | `live_url` | ✓ |
| Gaotu_Course.py:41-42 Wenzai `getPlayUrl` / `getPlaybackInfoV4` | `video_play_url`, `live_play_url` with `%s` | ✓ |
| Gaotu_Course.py:43-45 pan/file/price APIs | `source_url`, `file_url`, `price_url` | ✓ |

## HTTP 调用

| 源码方法 | Go 函数 | method | 一致? |
|---|---|---|---|
| `_get_course_list` line 61 | `resolveCourse` fallback | POST JSON | ✓ |
| `_get_infos` line 158 | `resolveCourse` | POST JSON | ✓ |
| `_get_video_url` line 200 | `resolveLesson` | POST JSON | ✓ |
| `_get_live_url` line 281 | `resolveLesson` | POST JSON | ✓ |
| `_decode_video_url` / `_decode_inner_live_url` | `decodePcURL` | GET | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 映射 | 一致? |
|---|---|---|
| `data.clazzDetailChapterPcVO.chapterItemVOList[].lessonCardList[]` | `collectLessons` keys `chapterItemVOList`, `lessonCardList` | ✓ |
| `userClazzLessonBaseVO.clazzLessonName` | `lessonNode.Title` | ✓ |
| `userClazzLessonBaseVO.clazzLessonNumber` | `lessonNode.ID` | ✓ |
| `data.pcUrl` | `mediaFromPayload` + `collectStrings("pcUrl")` | ✓ |
| Wenzai `data.cdn_list[].url/enc_url` | `findMediaURL` keys `url`, `enc_url` | ✓ |

## 阻塞步骤

无. 无法从 `pcUrl` 或 `cdn_list` 解出媒体时返回明确错误, 不返回空 Streams.
