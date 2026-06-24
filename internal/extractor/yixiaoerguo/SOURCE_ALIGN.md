# yixiaoerguo 源码对齐对照

Source: `~/code/xwz-downloader-source-release/decompiled_full/Mooc/Courses/Yixiaoerguo/`.
Encrypted/truncated sections were cross-checked with `~/code/xwz-downloader-source-release/decrypted_source/Yixiaoerguo.py` and `decrypted_full/all_decrypted.json`.

## URL 常量

| .cdc.py 行 | yixiaoerguo.go 行/名 | 一致? |
| --- | --- | --- |
| `Yixiaoerguo_Base.py:52 referer = 'https://www.biguo.cn/my/course'` | `yixiaoerguo.go:18 refererURL` | ✓ |
| `Yixiaoerguo_Base.py:53 api_base = 'https://api.biguo.cn'` | `yixiaoerguo.go:20 apiBase` | ✓ |
| `Yixiaoerguo_Course.py:81 qx_recordquery_urls = ('https://bjs1.qianxuecloud.com/recordquery', '...backup', '...mu')` | `yixiaoerguo.go:21-23 qxRecordQuery*` | ✓ |
| `Yixiaoerguo_Course.py:82 qx_playbackquerywebhls_url = 'https://vodquerys1.qianxuecloud.com/playbackquerywebhls'` | `yixiaoerguo.go:24 qxPlaybackQueryWebHLS` | ✓ |
| `Yixiaoerguo_Course.py:83 qx_dataplaybackqueryh5_url = 'https://vodquerydatas1.qianxuecloud.com/dataplaybackqueryh5'` | `yixiaoerguo.go:25 qxDataPlaybackQueryH5` | ✓ |
| `Yixiaoerguo_Course.py:84 qx_replaysvr_url = 'https://s1rqs.qianxuecloud.com/rqs/wsreplaysvr'` | `yixiaoerguo.go:26 qxReplaySVRURL` | ✓ |
| `Yixiaoerguo_Course.py:85 qx_hls_encrypt_url = 'https://svrquerys1.qianxuecloud.com/rqs/hls_encrypt'` | `yixiaoerguo.go:27 qxHLSEncryptURL` | ✓ |
| `Yixiaoerguo_Course.py:86 qx_media_referer = 'https://lives1.qianxuecloud.com/live_sc/'` | `yixiaoerguo.go:28 qxMediaReferer` | ✓ |

## HTTP 调用

| 源码方法 | Go 函数 | method | 一致? |
| --- | --- | --- | --- |
| `Yixiaoerguo_Base._check_cookie` GET `/api/courses` with `current/page/pageSize/countTotal/free` | `checkCookie` | GET | ✓ |
| `Yixiaoerguo_Course._request_api` signed `requests.request` | `requestAPI` | GET/POST JSON | ✓ |
| `_get_title` GET `/api/courses/{cid}` | `courseTitle` | GET | ✓ |
| `_get_chapters_payload` GET `/api/courses/{cid}/chapters`, fallback `/api/courses/products/{cid}/chapters` | `chaptersPayload` | GET | ✓ |
| `_get_section_play_info` GET `/api/courses/sections/{section_id}/{playback_info|record_info|live_info}` | `sectionPlayInfo` | GET | ✓ |
| `_unlock_audition` POST `/api/courses/audition/unlock` | `sectionPlayInfo` fallback | POST JSON | ✓ |
| `_get_qx_record_media` GET qianxue `recordquery*` then media URL | `getQXRecordMedia` | GET | ✓ |
| `_get_qx_hls_url` GET qianxue `playbackquerywebhls` / `dataplaybackqueryh5` | `getQXHLSURL` | GET | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 解析 | 一致? |
| --- | --- | --- |
| `_api_headers`: `XSC-CLIENT`, `XSC-API-VERSION`, `XSC-SIGN`, `XSC-TIMESTAMP`, `XSC-NONSTR`, `Authorization` | `apiHeaders` emits the same header names | ✓ |
| `_extract_course_list_from_payload`: `data.list/records/items/rows/content/courseList/courses` | `extractItems` accepts the same list keys | ✓ |
| `_get_chapters_payload`: `data.chapters` | `chaptersPayload`, `collectVideos` | ✓ |
| `_get_infos`: chapter `name/title`, `sections/children`, section `id/sectionId/periodId`, `type/sectionType`, `state`, `can_try/canTry` | `collectVideos` maps the same keys into `yxVideo` | ✓ |
| `_get_section_play_info`: response `data` | `sectionPlayInfo` returns non-empty `data` | ✓ |
| `_extract_qx_token`: `qx.app.token` or URL query `token` from `url/h5Ur` | `extractQXToken` | ✓ |
| `_get_qx_record_media`: `urlMedia`, `url`, fetched media `data[].cdn_url`, `size`, `duration` | `getQXRecordMedia`, `bestMedia` | ✓ |
| `_get_qx_hls_url`: `cdn_url/url/playUrl/hlsUrl/address`, `.m3u8` | `getQXHLSURL`, `findURLs` | ✓ |

## 阻塞步骤

无. Board rendering / websocket replay synthesis is downloader-only behavior; extractor output focuses on the primary qianxuecloud mp4 or hls media URL used by `_download_qx_video`.
