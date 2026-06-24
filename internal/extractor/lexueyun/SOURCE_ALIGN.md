# lexueyun 源码对齐对照

## URL 常量

| .cdc.py 行 | lexueyun.go 行/名 | 一致? |
|---|---|---|
| Lexueyun_Base.py:31 `origin = 'https://my.lexue-cloud.com'` | lexueyun.go:27 `urlOrigin = "https://my.lexue-cloud.com"` | ✓ |
| Lexueyun_Base.py:32 `referer = origin + '/home'` | lexueyun.go:28 `urlReferer = urlOrigin + "/home"` | ✓ |
| Lexueyun_Base.py:33 `channel_code = 'lexueyun-pc'` | lexueyun.go:29 `channelCode = "lexueyun-pc"` | ✓ |
| Lexueyun_Base.py:34 `user_info_url = origin + '/happyStudy/user/userInfo'` | lexueyun.go:30 `userInfoPath = "/happyStudy/user/userInfo"` | ✓ |
| Lexueyun_Course.py:34 `merchant_list_path = '/happyStudy/proxy/lexuesv/app/myMerchantList/v2'` | lexueyun.go:31 `merchantListPath` | ✓ |
| Lexueyun_Course.py:35 `order_list_path = '/happyStudy/proxy/lexuesv/app/getOrdersByMerchant/v2'` | lexueyun.go:32 `orderListPath` | ✓ |
| Lexueyun_Course.py:36 `subject_detail_path = '/happyStudy/proxy/lexuesv/pc/getSubjectDetail'` | lexueyun.go:33 `subjectDetailPath` | ✓ |
| Lexueyun_Course.py:37 `lesson_list_path = '/happyStudy/proxy/lexuesv/pc/getLessonsBySubject'` | lexueyun.go:34 `lessonListPath` | ✓ |
| Lexueyun_Course.py:38-40 datum/order/progress paths | lexueyun.go:35-37 `datumPath` / `orderInfoPath` / `lessonProgressPath` | ✓ |
| Lexueyun_Course.py:41 `live_play_path = '/happyStudy/live/getPlayUrl'` | lexueyun.go:38 `livePlayPath` | ✓ |
| Lexueyun_Course.py:42 `livepro_play_path = '/happyStudy/livePro/getPlayUrl'` | lexueyun.go:39 `liveProPlayPath` | ✓ |
| Lexueyun_Course.py:43 `sunlands_video_entry = 'https://video.sunlands.com/video'` | lexueyun.go:40 `sunlandsVideoEntry` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Lexueyun_Base._request_lexue line 338 | lexueyun.go:274 `requestLexue` | POST form `channelCode` + `data` | ✓ |
| Lexueyun_Base._check_cookie line 375 | lexueyun.go:172 `loginSession` | POST `/happyStudy/user/userInfo` | ✓ |
| Lexueyun_Course._get_merchants line 225 | lexueyun.go:193 `firstCourse` | POST merchant list | ✓ |
| Lexueyun_Course._get_orders line 251 | lexueyun.go:193 `firstCourse` | POST order list | ✓ |
| Lexueyun_Course._get_subject_detail line 554 | lexueyun.go:217 `fillSubjectDetail` | POST subject detail | ✓ |
| Lexueyun_Course._get_lessons line 597 | lexueyun.go:230 `getLessons` | POST lesson list | ✓ |
| Lexueyun_Course._get_play_url line 916 | lexueyun.go:245 `resolveLesson` | POST live/livePro play URL | ✓ |
| Lexueyun_Course._get_sunlands_session line 976 | lexueyun.go:304 `sunlandsMediaURL` | POST `thirdLogin` JSON | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct tag / 代码 | 一致? |
|---|---|---|
| `_request_lexue`: form `channelCode`, `data=json.dumps(params, separators=(',', ':'))` | lexueyun.go:278-284 `params["channelCode"]`, `PostForm` | ✓ |
| `_check_cookie`: `flag`, `data.id/stuId/stu_id` | lexueyun.go:65-75 `userInfoResp`; lexueyun.go:187-189 | ✓ |
| `_get_course_list`: `merchantList/myMerchantList/list/records/items/rows` | lexueyun.go:194 `extractList(... merchant keys ...)` | ✓ |
| `_get_course_list`: `orderList/orders/courseList/list/records/items/rows` | lexueyun.go:200 `extractList(... order keys ...)` | ✓ |
| `_get_course_list`: `subjectList/subjects/courseList/courses/courseInfoList/subjectInfoList/classList` | lexueyun.go:205 `extractList(... subject keys ...)` | ✓ |
| `_get_subject_detail`: `data.packageId`, `data.merchantId` | lexueyun.go:78-83 `subjectDetailResp` | ✓ |
| `_get_lessons`: `data.resourceList` | lexueyun.go:84-88 `lessonsResp` | ✓ |
| `_make_video_info`: `resourceName`, `lessonList`, `lessonName/name/title`, `lessonId`, `livePlaybackId`, `liveLessonId`, `teachUnitId`, `liveSource`, `liveStatus`, `isNewLive` | lexueyun.go:89-113 `resource` / `lesson` | ✓ |
| `_live_type`: `liveStatus == 5`, `isNewLive == 1` | helpers.go:68-76 `liveType` | ✓ |
| `_get_play_url`: `data.playUrl` | lexueyun.go:115-119 `playResp` | ✓ |
| `_decode_live_data`: query/fragment `liveData` JSON | helpers.go:12-33 `decodeLiveData` | ✓ |
| `_get_sunlands_session`: `token`, `videoPlayUrls` | lexueyun.go:120-130 `sunlandsResp` / `sunlandsVideo` | ✓ |
| `_select_video_play_url`: prefer mp4, sort by `lFileSize`, use `sHttpsUrl` then `sUrl` | lexueyun.go:326-338 `sunlandsMediaURL` | ✓ |

## 阻塞步骤

无.
