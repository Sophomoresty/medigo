# dongao 源码对齐对照

## URL 常量

| .cdc.py 行 | dongao.go 行/名 | 一致? |
|---|---|---|
| Dongao_Base.py:37-42 referer/origin/member/login/member_service/stage_probe | same constants | ✓ |
| Dongao_Course.py:37 `stage_list_url = 'https://course.dongao.com/v4/liveAndCourseList'` | `stage_list_url` | ✓ |
| Dongao_Course.py:38 `detail_infos_url = 'https://course.dongao.com/v4/liveAndCourseDetailInfos'` | `detail_infos_url` | ✓ |
| Dongao_Course.py:39-40 live number / linked lecture APIs | `live_number_list_url`, `live_linked_lecture_url` | ✓ |
| Dongao_Course.py:41 `catalog_url = 'https://course.dongao.com/catalog/{course_id}'` | `catalog_url` with `%s` | ✓ |
| Dongao_Course.py:42 `lecture_url = 'https://course.dongao.com/lecture/{lecture_id}'` | `lecture_url` with `%s` | ✓ |

## HTTP 调用

| 源码方法 | Go 函数 | method | 一致? |
|---|---|---|---|
| `_get_catalog_html` line 784 | `resolveCourse` | GET catalog | ✓ |
| `_request_stage_list` line 740 | `requestCourseAPIs` | POST form | ✓ |
| `_request_detail_infos` line 761 | `requestCourseAPIs` | POST form | ✓ |
| `_get_lecture_page` decrypted t1018 | `resolveLecture` | POST `playerType=h5`, GET fallback | ✓ |
| `_parse_json_text` line 811 | `parseJSONText` | JSON parse | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 映射 | 一致? |
|---|---|---|
| catalog `courseCatalog.chapterList[].lectureList[]` | recursive `collectLectureNodes` | ✓ |
| detail keys `lectureId`, `lectureID`, `listenLectureId`, `liveNumberId`, `liveLectureId` | `collectLectureNodes` ID keys | ✓ |
| titles `lectureName`, `lectureTitle`, `title`, `name`, `videoName`, `courseName` | `pickTitle` / `parseTitle` | ✓ |
| listen param `source`, `url`, `path`, `playUrl` | `findMediaInText` / `findMediaURL` | ✓ |
| lecture page direct media `.m3u8/.mp4` | `directMediaRe` | ✓ |

## 阻塞步骤

无. 若讲次页没有可解析媒体源, 返回明确错误, 不返回空 Streams.
