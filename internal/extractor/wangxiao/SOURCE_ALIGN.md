# wangxiao 源码对齐对照

## URL 常量

| .cdc.py 行 | wangxiao.go 行/名 | 一致? |
|---|---|---|
| Wangxiao_Base.py:29 `referer = 'https://k.wangxiao.cn'` | wangxiao.go:18 `refererURL` | ✓ |
| Wangxiao_Base.py:30 `user_url = 'https://k.wangxiao.cn/user/'` | wangxiao.go:19 `userURL` | ✓ |
| Wangxiao_Course.py:255 `play_url = 'https://k.wangxiao.cn/play?activityid={activity_id}&productsid={product_id}'` | wangxiao.go:20 `urlPlay`, `{}` → `%s` | ✓ |
| Wangxiao_Course.py:256 `item_url = 'https://k.wangxiao.cn/item/{item_num}.html'` | wangxiao.go:21 `urlItem`, `{}` → `%s` | ✓ |
| Wangxiao_Course.py:257 `ke_catalog_url = 'https://ke.wangxiao.cn/apis//products/skuSingleContent'` | wangxiao.go:22 `urlSku` | ✓ |
| Wangxiao_Course.py:258 `ke_api_token = '7209bbbc-cb34-438b-ad3b-742fa7fd9f2c'` | wangxiao.go:23 `keAPIToken` | ✓ |
| Wangxiao_Course.py:259 `user_directory_url = 'https://k.wangxiao.cn/Course/ProductsDirectory?...'` | wangxiao.go:24 `urlDirectory`, `{}` → `%s` | ✓ |
| Wangxiao_Course.py:260 `user_classhours_url = 'https://k.wangxiao.cn/Course/GetClasshours?cid={course_id}&pid={product_id}'` | wangxiao.go:25 `urlClasshours`, `{}` → `%s` | ✓ |
| Wangxiao_Course.py:261 `old_player_url = 'https://users.wangxiao.cn/player/Index.aspx?Id={activity_id}'` | wangxiao.go:26 `urlPlayer`, `{}` → `%s` | ✓ |
| Wangxiao_Course.py:262 `old_handout_url = 'https://users.wangxiao.cn/player/down.aspx?Id={activity_id}'` | wangxiao.go:27 `urlPlayerDown`, `{}` → `%s` | ✓ |
| Wangxiao_Course.py:263 `live_handout_url = 'https://live.wangxiao.cn/LiveActivity/DownHandOut/?Id={activity_id}'` | wangxiao.go:28 `urlLiveHandout`, `{}` → `%s` | ✓ |
| Wangxiao_Course.py:264 `video_play_url = 'https://p.bokecc.com/servlet/getvideofile?vid={vid}&siteid={siteid}'` | wangxiao.go:29 `urlVideoPlay`, `{}` → `%s` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Wangxiao_Base._check_cookie line 314 → GET `user_url` | wangxiao.go:60 `Extract` | GET source page / login page body parse | ✓ |
| Wangxiao_Course._request_text line 411 | wangxiao.go:60 `Extract`, wangxiao.go:110 `resolveRef` | GET HTML/player page | ✓ |
| Wangxiao_Course._get_infos line 1464 → play/old player page | wangxiao.go:110 `resolveRef` | GET `urlPlay` / `urlPlayer` | ✓ |
| Wangxiao_Course._get_ke_catalog_groups line 1379 → `ke_catalog_url` | wangxiao.go:178 `refsFromKeCatalog` | POST form `id=<setmealId>` | ✓ |
| Wangxiao_Course._get_video_url line 1637 → BokeCC `getvideofile` | wangxiao.go:137 `shared.BokeCCResolve` | GET via shared helper | ✓ |

## JSON / 字段映射

| 源码 key / regex | Go struct tag / 代码 | 一致? |
|---|---|---|
| `_parse_page_data`: `var pageData = {...}` JSON | wangxiao.go:169 `json.Unmarshal` into `map[string]any` | ✓ |
| `_extract_activity_id`: `activityid=...` or `[?&]Id=...` | wangxiao.go:48 `activityRe` | ✓ |
| `_extract_product_id`: `productsid=...` | wangxiao.go:49 `productRe` | ✓ |
| `_extract_item_num`: `/item/(\d+).html` | wangxiao.go:50 `itemRe` | ✓ |
| `_extract_bokecc_siteid`: `siteid=...` / JSON key `siteid` | wangxiao.go:53 `siteIDRe`, wangxiao.go:217 `extractSiteID` | ✓ |
| `_extract_cc_vid`: `cc_vid`, `vid`, JSON key `vid`, `ccVideoId` | wangxiao.go:54 `vidRe`, wangxiao.go:218 `extractVideoID` | ✓ |
| `_extract_course_title`: `.course-title` / `<title>` | wangxiao.go:55 `titleRe`, wangxiao.go:207 `extractTitle` | ✓ |
| `_get_ke_catalog_groups`: response `code`, `data`, `title`, `activityid`, `ccVideoId`, `ccUserId` | wangxiao.go:192-201 recursive `data` walk | ✓ |
| BokeCC response: `copies`, `playurl`, `quality` | `shared.BokeCCResolve` parses XML and picks best | ✓ |

## 阻塞步骤

无.
