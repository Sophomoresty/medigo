# lizhiweike 源码对齐对照

## URL 常量

| .cdc.py 行 | Go 行/名 | 一致? |
|---|---|---|
| `Lizhiweike_Base.py:33 referer = 'https://m.lizhiweike.com'` | `lizhiweike.go:20 urlMobile = "https://m.lizhiweike.com"` | ✓ |
| `Lizhiweike_Base.py:34 order_url = 'https://apiv1.lizhiweike.com/api/history/buy_record'` | `lizhiweike.go:19 urlBuyRecord = "https://apiv1.lizhiweike.com/api/history/buy_record"` | ✓ |
| `Lizhiweike_Base.py:296 'https://open.lizhiweike.com/oauth2/check_token?token={}'` | `lizhiweike.go:18 urlCheckToken = "https://open.lizhiweike.com/oauth2/check_token?token=%s"` | ✓ |
| `Lizhiweike_Course.py:33 course_list_url = '.../my_weike/{wid:}/my_lectures?token={token:}&offset={next_offset}&limit=10'` | `lizhiweike.go:21 urlCourseList = ".../my_weike/%s/my_lectures?token=%s&offset=%s&limit=10"` | ✓ |
| `Lizhiweike_Course.py:34 info_url = 'https://apiv1.lizhiweike.com/api/{type:}/{cid:}/info?token={token:}'` | `lizhiweike.go:22 urlInfo = "https://apiv1.lizhiweike.com/api/%s/%s/info?token=%s"` | ✓ |
| `Lizhiweike_Course.py:35 video_url = 'https://apiv1.lizhiweike.com/api/lecture/{vid:}/info?token={token:}'` | `lizhiweike.go:23 urlVideo = "https://apiv1.lizhiweike.com/api/lecture/%s/info?token=%s"` | ✓ |
| `Lizhiweike_Course.py:36 live_url = 'https://gateway-weike.lizhiweike.com/tic/record?lecture_id={vid:}&object_type=lecture&version=1.0&token={token:}'` | `lizhiweike.go:24 urlLive = "https://gateway-weike.lizhiweike.com/tic/record?lecture_id=%s&object_type=lecture&version=1.0&token=%s"` | ✓ |
| `Lizhiweike_Course.py:37 m3u8_url = 'https://apiv1.lizhiweike.com/api/bridge/qcvideo/{vfid:}?token={token:}&al=drm'` | `lizhiweike.go:25 urlM3U8 = "https://apiv1.lizhiweike.com/api/bridge/qcvideo/%s?token=%s&al=drm"` | ✓ |
| `Lizhiweike_Course.py:38 join_url = 'https://apiv1.lizhiweike.com/api/channel/{cid:}/subscribe?token={token:}'` | `lizhiweike.go:26 urlJoin = "https://apiv1.lizhiweike.com/api/channel/%s/subscribe?token=%s"` | ✓ |
| `Lizhiweike_Course.py:39 audio_list_url = 'https://apiv1.lizhiweike.com/api/classroom/{audio_id:}/message/get/voice?token={token:}'` | `lizhiweike.go:27 urlAudioList = "https://apiv1.lizhiweike.com/api/classroom/%s/message/get/voice?token=%s"` | ✓ |
| `Lizhiweike_Course.py:40 video_list_url = 'https://apiv1.lizhiweike.com/api/classroom/{video_id:}/message/list?token={token:}&new_classroom=1&is_reverse=0&limit=2000'` | `lizhiweike.go:28 urlVideoList = "https://apiv1.lizhiweike.com/api/classroom/%s/message/list?token=%s&new_classroom=1&is_reverse=0&limit=2000"` | ✓ |

## HTTP 调用

| 源码方法 | Go 函数 | method | 一致? |
|---|---|---|---|
| `Lizhiweike_Base._check_cookie line 290` | `lizhiBuildSession line 94` | GET `check_token` | ✓ |
| `Lizhiweike_Course._get_course_list line 101` | `lizhiFetchCourseList line 148` | GET `course_list_url` | ✓ |
| `Lizhiweike_Course._get_cid line 269` | `lizhiResolveTarget line 119` | GET `video_url`, then `info_url` | ✓ |
| `Lizhiweike_Course._get_infos line 358` | `Extract line 60` | GET `info_url` | ✓ |
| `Lizhiweike_Course._get_video_url line 429` | `lizhiVideoURL line 247` | GET `video_url`, then `m3u8_url` | ✓ |
| `Lizhiweike_Course._get_live_url .das t320` | `lizhiLiveURL line 270` | GET `live_url` | ✓ |
| `Lizhiweike_Course._get_audio_url .das t335` | `lizhiAudioURL line 278` | GET `video_url` | ✓ |
| `Lizhiweike_Course._get_audio_url_list .das t347` | `lizhiAudioURLList line 286` | GET `audio_list_url` | ✓ |
| `Lizhiweike_Course._get_video_url_list .das t368` | `lizhiVideoURLList line 300` | GET `video_list_url` | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 解析 | 一致? |
|---|---|---|
| `_check_cookie: "code" == 0, "is_valid" == true` | `out["code"]`, `out["is_valid"]` / `out["data"]["is_valid"]` | ✓ |
| `_get_course_list: data.lectures[].status/name/id/liveroom_id/type` | `nested(resp,"data","lectures")`, `status/name/id/liveroom_id/type` | ✓ |
| `_get_cid: data.channel.id`, `data.object_id` | `nestedText(info,"data","channel","id")`, `nestedText(chInfo,"data","object_id")` | ✓ |
| `_get_infos: data.share_info.share_title`, `data.lectures`, `data.lecture` | `nestedText(info,"data","share_info","share_title")`, `lizhiLecturesFromInfo` | ✓ |
| `_get_video_url: data.video_info.qcloud_video_file_id` | `nestedText(info,"data","video_info","qcloud_video_file_id")` | ✓ |
| `_get_video_url: data.play_list[].definition/url/size` | `nested(play,"data","play_list")`, `definition/url/size` | ✓ |
| `_get_live_url: data.mp4.media_url`, `data.mp4.file_size` | `nestedText(resp,"data","mp4","media_url")`, `file_size` | ✓ |
| `_get_audio_url: data.audio_info.audio_url` | `nestedText(resp,"data","audio_info","audio_url")` | ✓ |
| `_get_audio_url_list: data[].audio` | `records(resp["data"])`, `audio` | ✓ |
| `_get_video_url_list: data.messages[].type == video`, `meta.video_url` | `nested(resp,"data","messages")`, `type`, `meta.video_url` | ✓ |

## 阻塞步骤

无.
