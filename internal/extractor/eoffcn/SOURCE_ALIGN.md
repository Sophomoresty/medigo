# eoffcn 源码对齐对照

## URL 常量

| .cdc.py 行 | eoffcn.go 行/名 | 一致? |
|---|---|---|
| Eoffcn_Course.py:41 `order_url = 'https://xue.eoffcn.com/api/order/complete'` | `order_url` | ✓ |
| Eoffcn_Course.py:42 `new_order_url = 'https://xue.eoffcn.com/api/new/goods/list'` | `new_order_url` | ✓ |
| Eoffcn_Course.py:43 `package_url = 'https://xue.eoffcn.com/api/package/list?system_order={system_order:}&coding={coding:}'` | `package_url` with `%s` | ✓ |
| Eoffcn_Course.py:44 `catagory_url = 'https://xue.eoffcn.com/api/lesson/catagory?package_id={package_id:}&system_order={system_order:}'` | `catagory_url` with `%s` | ✓ |
| Eoffcn_Course.py:45 `course_list_url = 'https://xue.eoffcn.com/api/new/course/list?system_order={system_order:}'` | `course_list_url` with `%s` | ✓ |
| Eoffcn_Course.py:46 `lesson_url = 'https://xue.eoffcn.com/api/lesson/detail?lesson_id={lid:}&package_id={cid:}&module_type={m_type:}&system_order={system_order:}'` | `lesson_url` with `%s` | ✓ |
| Eoffcn_Base.py:118 `https://xue.eoffcn.com/api/check/member` | `check_member_url` | ✓ |
| Eoffcn_Course.py:47-48 public key / watch demand APIs | `pub_key_url`, `encrypt_url` | ✓ |
| Eoffcn_Local.py `https://pcvod.offcncloud.com/` + `https://vod-live.offcncloud.com/base/wb1004/wboffcn.js` | `eoffcnBoardReferer`, `eoffcnWbOffcnJSURL`, `eoffcnWbOffcnMemURL` | ✓ |

## HTTP 调用

| 源码方法 | Go 函数 | method | 一致? |
|---|---|---|---|
| `_check_cookie` line 118 | `Extract` (cookie validation) | GET | ✓ |
| `_get_order_list` / `_get_new_order_list` / `_get_cid` lines 72-214 | `fetchOldOrders`, `selectOrder`, `fetchSpuIDFromPage`, `mergeOrderParams` | GET | ✓ |
| `_get_courses_list` line 175 | `resolveCourse` | GET | ✓ |
| `_get_package_list` line 155 | `resolveCourse` | GET | ✓ |
| `_get_catagory_info` line 199 | `resolveCourse` | GET | ✓ |
| `_get_lesson_info` line 265 | `resolveLesson` | GET | ✓ |
| `_get_m3u8_info` line 330 (AES decrypt live_url) | `findMediaURL` + `aesDecryptLiveURL` | - | ✓ |
| `_get_pub_key` / `_decrypt_video_key` / `_request_watch_demand_data` | `getPubKey` + `requestWatchDemand` | GET public key + RSA-encrypted POST form | ✓ |
| `_download_eoffcn_board_playback` / pcvod board manifest | `hydrateEoffcnWhiteboardPlayback` / `buildEoffcnWhiteboardManifest` / `buildEoffcnWhiteboardHTML` | GET pcvod API + wboffcn HTML replay | ✓ |

## AES Decryption

| 源码 | Go 映射 | 一致? |
|---|---|---|
| `_get_m3u8_info` line 330: AES key `1234567898882222`, IV `8NONwyJtHesysWpM` | `aesDecryptLiveURL` with `aesKey`/`aesIV` | ✓ |
| `_get_file` line 697: same AES key/IV for download_path | `aesDecryptLiveURL` (same function) | ✓ |
| `_get_pub_key`: AES key/IV `wwwoffcncloudcom`; watch demand response uses random key as key/IV | `aesDecryptWithStatic` inside `requestWatchDemand` | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 映射 | 一致? |
|---|---|---|
| `data.list[]` | recursive `collectLessonNodes` over `data`, `list`, `items` | ✓ |
| `level_name`, `module_type`, `id`, `room_id`, `file_id`, `child` | `lessonNode` fields | ✓ |
| lesson `data.video_url` / `data.live_url` | `findMediaURL` keys `video_url`, `live_url` | ✓ |
| lesson `data.live_url` (AES-encrypted) | `findMediaURL` → `aesDecryptLiveURL` | ✓ |
| watch demand `data.live_url` | `requestWatchDemand` + `findMediaURL` | ✓ |
| board API payload: `api_url`, `room_num`, `room_id`, wbx/wbr, page image, font, audio URLs | `extractEoffcnWhiteboardInfo`, `collectEoffcnBoardAssets`, `eoffcnWhiteboardHeaders` | ✓ |

## 阻塞步骤

无。`_download_eoffcn_board_playback` 已对齐为 HTML canvas 播放导出: Go 侧预取 `pcvod.offcncloud.com` 白板数据, 归一化 wbx/wbr, 背景图, 字体和音频资源, 并嵌入 `wboffcn.js` 播放器; 对同时存在视频流的课时会附加 `whiteboard` HTML side stream。未解析到 `live_url` / `video_url` 且 `requestWatchDemand` 未返回媒体 URL 时返回明确错误, 不返回空 Streams.
