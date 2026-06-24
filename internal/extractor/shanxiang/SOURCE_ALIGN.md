# shanxiang 源码对齐对照

## URL 常量

| .cdc.py 行 | shanxiang.go 行/名 | 一致? |
|---|---|---|
| Shanxiang_Course.py:35 `course_list_url = 'https://www.sx1211.com/User/getAjaxCourseList'` | shanxiang.go:28 `urlCourseList` | ✓ |
| Shanxiang_Course.py:36 `study_url = 'https://www.sx1211.com/course/study.html?id={cid}&skuId={sku_id}'` | shanxiang.go:29 `urlStudy = ...id=%s&skuId=%s` | ✓ |
| Shanxiang_Course.py:37 `playback_url = 'https://www.sx1211.com/course/playbackView?id={playback_id}&skuId={sku_id}&scheduleId={schedule_id}'` | shanxiang.go:30 `urlPlayback = ...id=%s&skuId=%s&scheduleId=%s` | ✓ |
| Shanxiang_Course.py:38 `docview_url = 'https://www.sx1211.com/course/docview.html?product_id={cid}&doc_id={doc_id}'` | shanxiang.go:31 `urlDocview = ...product_id=%s&doc_id=%s` | ✓ |
| Shanxiang_Course.py:39-41 CSSL replay URLs | shanxiang.go:32-34 `urlCsslLogin/urlCsslPlay/urlCsslMeta`; playback resolution delegates to `shared.CssLcloudResolvePlayInfo` | ✓ |
| Shanxiang_Base.py:29-32 referer/origin/login check | shanxiang.go:40-41, 298-300 `urlReferer/urlLoginCheck/shanxiangHeaders` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Shanxiang_Base._check_cookie line 205 `request_get_raw(login_check_url, headers...)` | shanxiang.go:89-90 study page fetch with `Referer: login_check_url` | GET | ✓ |
| Shanxiang_Course._get_course_list decrypted `t477` | shanxiang.go:133-140 `fetchCourseFromList` | GET | ✓ |
| Shanxiang_Course._get_study_html decrypted `t607` | shanxiang.go:89-95 | GET | ✓ |
| Shanxiang_Course._get_playback_html decrypted `t615` | shanxiang.go:156-165 `resolvePlayback` | GET | ✓ |
| Shanxiang_Course._get_cc_session decrypted `t824`; task R6 requires shared helper | shanxiang.go:174-177 `shared.CssLcloudResolvePlayInfo` | POST+GET inside shared | ✓ |
| Shanxiang_Course.download_video decrypted `t921` `.m3u8` branch | shanxiang.go:187-193 `shared.CssLcloudRewriteM3U8Keys` | GET key fetch inside shared | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct tag / access | 一致? |
|---|---|---|
| `_get_course_list`: `result.get('success')` | `Success any json:"success"` | ✓ |
| `_parse_course_list_items`: `data.rows[].productid/id/skuid/skuId/productname/name/price/minprice/maxprice` | `Data.Rows[].ProductID/ID/SKUId/SKUId2/ProductName/Name/Price/MinPrice/MaxPrice` tags | ✓ |
| `_parse_cc_info_from_html`: `(userId|roomId|recordId|viewername|viewertoken|groupId)` | shanxiang.go:224-228 `ccPairRe`, 262-276 `parseCCInfo` | ✓ |
| `_parse_cc_info_from_html`: hidden `userId/roomId/recordId/viewerName/viewerId/liveId/videoId` | shanxiang.go:267-274 hidden input fallback | ✓ |
| `_get_cc_session`: `data.user.token`, `_get_video_url`: `play_data.data.video` | `shared.CssLcloudResolvePlayInfo` parses CSSL `sessionId` + `vod_info.video/audio` | ✓ |

## 阻塞步骤

无。
