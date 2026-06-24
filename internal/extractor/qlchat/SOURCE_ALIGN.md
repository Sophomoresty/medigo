# qlchat 源码对齐对照

## URL 常量

| .cdc.py 行 | qlchat.go 行/名 | 一致? |
|---|---|---|
| Qlchat_Course.py:37 `course_list_url = 'https://m.qlchat.com/api/wechat/transfer/h5/live/purchaseCourse'` | `course_list_url` | ✓ |
| Qlchat_Course.py:38 `learn_list_url = 'https://m.qlchat.com/api/wechat/transfer/h5/topic/listRecentLearn'` | `learn_list_url` | ✓ |
| Qlchat_Course.py:39 `info_url = 'https://m.qlchat.com/api/wechat/transfer/h5/interact/getCourseList'` | `info_url` | ✓ |
| Qlchat_Course.py:40 `video_url = 'https://m.qlchat.com/api/wechat/topic/getMediaActualUrl'` | `video_url` | ✓ |
| Qlchat_Course.py:41 `h5_video_url = 'https://m.qlchat.com/api/wechat/topic/media-url?topicId={topic_id:}'` | `h5_video_url = "...topicId=%s"` | ✓ |
| Qlchat_Course.py:42 `topic_intro_url = 'https://m.qlchat.com/wechat/page/topic-intro?topicId={topic_id}'` | `topic_intro_url = "...topicId=%s"` | ✓ |
| Qlchat_Course.py:43 `topic_url = 'https://m.qlchat.com/wechat/page/topic-simple-video?topicId={video_id:}'` | `topic_url = "...topicId=%s"` | ✓ |
| Qlchat_Course.py:44 `audio_url = 'https://m.qlchat.com/api/wechat/topic/media-url?topicId={audio_id:}'` | `audio_url = "...topicId=%s"` | ✓ |
| Qlchat_Course.py:45 `live_url = 'https://m.qlchat.com/api/wechat/topic/getLivePlayUrl'` | `live_url` | ✓ |
| Qlchat_Course.py:52-57 price/purchased/auth/free URLs | `price_url`, `purchased_url`, `channel_auth_url`, `topic_auth_url`, `topic_auth_transfer_url`, `join_free_course_url` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 | method | 一致? |
|---|---|---|---|
| `_get_course_list` 116-145 | `fetchCourseList` | POST JSON | ✓ |
| `_get_cid` 201-222 | `loadTopicIntro` | GET | ✓ |
| `_get_title` / `_get_price` 236-264 | `Extract` price probe | GET | ✓ |
| `_get_purchased` 442-451 | `Extract` purchased probe | POST JSON | ✓ |
| `_get_infos` 566-574 | `fetchCourseItems` | POST JSON | ✓ |
| `_get_source_id` 646-655 | `sourceTopicID` | GET + regex | ✓ |
| `_get_video_url` 667-714 | `resolveVideo` | POST JSON + GET JSON | ✓ |
| `_get_audio_url` 778-789 | `resolveAudio` | GET JSON | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct tag | 一致? |
|---|---|---|
| `data.list[].courseType/bussinessId/skuTitle/liveId` | `Data.List[].CourseType/BussinessID/SkuTitle/LiveID` | ✓ |
| `data.learningList[]` | `Data.LearningList` | ✓ |
| `data.dataList[].businessType/businessName/businessId/topicPo` | `Data.DataList[].BusinessType/BusinessName/BusinessID/TopicPo` | ✓ |
| `topicPo.audioAssemblyUrl/isAuthTopic/type` | `TopicPo.AudioAssemblyURL/IsAuthTopic/Type` | ✓ |
| `data.list[].type/playUrl` | `Data.List[].Type/PlayURL` | ✓ |
| `data.video[].height/width/playUrl` | `Data.Video[].Height/Width/PlayURL` | ✓ |
| `data.audio.playUrl` | `Data.Audio.PlayURL` | ✓ |

## 阻塞步骤

无.
