# keqq 源码对齐对照

## URL 常量

| .cdc.py 行 | keqq.go 行/名 | 一致? |
|---|---|---|
| Keqq_Course.py:32 `course_list_url` | keqq.go:29 `urlCourseList` | ✓ |
| Keqq_Course.py:33 `course_url` | keqq.go:30 `urlCourse` | ✓ |
| Keqq_Course.py:34 `video_url` | keqq.go:31 `urlVideo` | ✓ |
| Keqq_Course.py:35 `file_url` | keqq.go:32 `urlFile` | ✓ |
| Keqq_Course.py:37 `detail_url = ...term_id_list=%5B{tid}%5D...` | keqq.go:33 `urlDetail` | ✓, `%5B/%5D` escaped in Go |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| `_get_course_list` lines 50-137: `get_plan_list?page` + JSON parse | keqq.go:138 `fetchKeqqCourseList` | GET | ✓ |
| `_get_infos` lines 189-230: course page HTML, `__NEXT_DATA__`, title, price, purchased, chapter tree | keqq.go:182 `fetchKeqqCoursePage`, 194 `keqqPageTitle`, 216 `keqqPriceAndPurchased`, 257 `keqqMergedChapters` | GET + HTML/JSON parse | ✓ |
| `_get_info_again` lines 236-269: `get_terms_detail` 复读章节树 | keqq.go:294 `fetchKeqqDetailChapters` | GET | ✓ |
| `_parse_chapter_list` lines 273-315: `task_info` 按 `type` 拆 video/file | keqq.go:322 `parseKeqqChapter` | local parse | ✓ |
| `_get_m3u8_info` lines 334-380: `describe_rec_video`, JSON parse, `infos/subtitles` | keqq.go:387 `getKeqqM3U8Info` | GET + JSON parse | ✓ |
| `_get_token` lines 320-328: base64 of `cid;term_id;vod_type;platform;cookie` | keqq.go:452 `keqqDRMToken` | local encode | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct tag / parse | 一致? |
|---|---|---|
| `result.map_list` | `fetchKeqqCourseList` lines 147-152 | ✓ |
| `map_courses[].cid/term_id/tname/cname/term_name/course_name` | `fetchKeqqCourseList` lines 153-167 | ✓ |
| `__NEXT_DATA__.props.pageProps.courseInfo.data.basic_info` | `nestedMap` + `keqqPageTitle`/`keqqPriceAndPurchased` lines 202-231 | ✓ |
| `courseInfo.data.pay_market_info.user_term_pay_info_list` | `keqqPriceAndPurchased` lines 230-237 | ✓ |
| `courseInfo.data.catalogMap[tid]` | `keqqMergedChapters` lines 257-267 | ✓ |
| `result.terms[].chapter_info[]` | `fetchKeqqDetailChapters` lines 303-320 | ✓ |
| `chapter.task_info[].type/resid_list/taid/aid/file/name` | `parseKeqqChapter` lines 325-350 | ✓ |
| `rec_video_info.infos` | `normalizeKeqqInfos` + `pickKeqqInfo` lines 397-432 | ✓ |
| `rec_video_info.subtitles[].type/url` | `getKeqqM3U8Info` lines 409-421 | ✓ |

## 阻塞步骤

无.
