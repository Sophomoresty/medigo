# xiaoeapp 源码对齐对照

## URL 常量

| .cdc.py 行 | xiaoeapp.go 行/名 | 一致? |
|---|---|---|
| Xiaoeapp_Base.py:64 `APP_API_BASE = 'https://xiaoeapp-server.xiaoeknow.com'` | xiaoeapp.go:22 `appAPIBase` | ✓ |
| Xiaoeapp_Base.py:65 `SIGN_SALT_KEY = 'xiaoeapp2024'` | xiaoeapp.go:23 `signSaltKey` | ✓ |
| Xiaoeapp_Course.py:43 `_COURSE_LIST_API = '/app/my.all.course.lists.get/2.0.0'` | xiaoeapp.go:24 `courseListAPI` | ✓ |
| Xiaoeapp_Course.py:44 `_VIDEO_INFO_API = '/app/goods/xe.goods.detail.get/1.0.3'` | xiaoeapp.go:25 `videoInfoAPI` | ✓ |
| Xiaoeapp_Course.py:45 `_LOOKBACK_URL_API = '/app/alive/xe.alive.lookbackurl.get/1.0.0'` | xiaoeapp.go:26 `lookbackURLAPI` | ✓ |
| Xiaoeapp_Course.py:50 `_COURSE_LIST_PAGE_SIZE = 200` | xiaoeapp.go:27 `courseListPageSize` | ✓ |
| Xiaoeapp_Base.py:67-70 `_BASE_HEADERS` | xiaoeapp.go:181 `Content-Type`, `User-Agent`, `app-type` | ✓ |
| Xiaoeapp_Course.py:1111-1124 H5 course URL templates | xiaoeapp.go:36 `idRe`, xiaoeapp.go:270 `typeFromURL` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Xiaoeapp_Base._post_app_api lines 202-228 | xiaoeapp.go:167 `postAppAPI` | POST JSON | ✓ |
| Xiaoeapp_Base._check_cookie lines 432-443 `/app/xe.user.info/1.0.0` | xiaoeapp.go:62 | POST JSON | ✓ |
| Xiaoeapp_Course._fetch_course_list lines 466-490 `_COURSE_LIST_API` | xiaoeapp.go:98-111 `fetchCourseList` | POST JSON | ✓ |
| Xiaoeapp_Course._get_goods_detail lines 1520-1547 `_VIDEO_INFO_API` | xiaoeapp.go:144-160 `resolveItemURL` | POST JSON | ✓ |
| Xiaoeapp_Course._get_live_url lines 1614-1658 `_LOOKBACK_URL_API` | xiaoeapp.go:134-143 `resolveItemURL` | POST JSON | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 解析 | 一致? |
|---|---|---|
| `_post_app_api` response `.json()` then `code` | xiaoeapp.go:188-191 `json.Unmarshal`, xiaoeapp.go:384 `code` | ✓ |
| `_fetch_course_list`: `data.list`, `data.total` | xiaoeapp.go:108-119 `listUnder(root["data"], "list")`, `total` | ✓ |
| `_get_course_item_id`: `resource_id/goods_id/course_id/id` | xiaoeapp.go:253 `firstVal` | ✓ |
| `_get_course_item_title`: `title/resource_title/goods_title/name/goods_name` | xiaoeapp.go:253 `firstVal` | ✓ |
| `_get_course_item_raw_type`: `resource_type/goods_type` + `_RTYPE_MAP` | xiaoeapp.go:252-262 `typeMap` | ✓ |
| `_get_goods_detail`: `data.resource_info`, `video_urls`/URL keys | xiaoeapp.go:157-164 `pickURL(data)` and `mapsUnder(data)` | ✓ |
| `_get_live_url`: `data.aliveVideoUrl/aliveVideoMp4Url/aliveVideoUrlEncrypt/miniAliveVideoUrl/aliveReviewUrl` | xiaoeapp.go:309 `pickURL` URL-key list | ✓ |
| `_load_cookie_state`: `token`, `b_user_id`, `app_user_id`, `union_id` | xiaoeapp.go:226-239 `sessionFromCookies` | ✓ |

## 阻塞步骤

无.

## R2 critical follow-up

| 缺口 | 处理结果 |
|---|---|
| protected/private lookback m3u8 | live 分支现在先检查 `aliveVideoUrlEncrypt`, `private_info`, `private_m3u8`, `__ba`, `distribute.vod.pri.get`; 命中时返回 `blocked: needs xiaoe private lookback m3u8 decrypt`, 不再把加密/私有 URL 当作可下载直链. |
| private video m3u8 | goods detail 分支同样检测私有 manifest/key 元数据, 命中后显式 blocked, 避免假成功. |
