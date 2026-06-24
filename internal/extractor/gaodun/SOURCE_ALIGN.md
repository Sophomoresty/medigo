# gaodun 源码对齐对照

## URL 常量

| .cdc.py 行 | gaodun.go 行/名 | 一致? |
|---|---|---|
| Gaodun_Course.py:37 `course_url = 'https://apigateway.gaodun.com/ep-course/api/v2/front/space/vcourse/pc'` | `course_url` | ✓ |
| Gaodun_Course.py:38 `info_url = 'https://apigateway.gaodun.com/ep-study/front/course/{cid}/syllabus'` | `info_url` with `%s` | ✓ |
| Gaodun_Course.py:39-41 gradation/glive/syllabus APIs | `info_gradation_url`, `info_glive_url`, `info_syllabus_url` | ✓ |
| Gaodun_Course.py:42-45 glive2-vod resource/check/old record APIs | `video_play_url`, `live_token_url`, `live_play_url`, `live_old_url` | ✓ |
| Gaodun_Course.py:46-51 handout/download/price/token APIs | `source_category_url`, `source_gradation_url`, `file_url`, `price_url`, `pc_token_url`, `pe_token_url` | ✓ |

## HTTP 调用

| 源码方法 | Go 函数 | method | 一致? |
|---|---|---|---|
| `_get_infos` line 158 | `resolveCourse` | GET | ✓ |
| `_get_chapter_info` / `_get_gradation_info` | `collectVideoNodes` over syllabus payloads | parse | ✓ |
| `_get_video_url` line 575 | `resolveDirect` | GET `live/resource` | ✓ |
| `_get_live_url` line 598 | `resolveDirect` | GET `live/resource` | ✓ |
| `_get_live_old_url` line 626 | `resolveDirect` | GET record info | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 映射 | 一致? |
|---|---|---|
| `result.syllabus.children[].resource.videoId/did` | `collectVideoNodes` keys `children`, `resource`, `videoId`, `did` | ✓ |
| `result.children/items` gradation nodes | recursive `collectVideoNodes` | ✓ |
| `result.list[].path` | `findMediaURL` key `path` | ✓ |
| `result.playUrls[].urls[].playUrl` | recursive `findMediaURL` key `playUrl` | ✓ |
| handout `path`, `format`, `file_url`, `file_fmt` | `findMediaURL` file keys | ✓ |

## 阻塞步骤

无. 没有解析到 `path` / `playUrl` 时返回错误, 不返回空 Streams.
