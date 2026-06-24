# youzan 源码对齐对照

Source: `~/code/xwz-downloader-source-release/decompiled_full/Mooc/Courses/Youzan/`.
Encrypted/truncated sections were cross-checked with `~/code/xwz-downloader-source-release/decrypted_source/Youzan.py`.

## URL 常量

| .cdc.py 行 | youzan.go 行/名 | 一致? |
| --- | --- | --- |
| `Youzan_Base.py:34 referer = 'https://www.youzan.com'` | `youzan.go:18 refererURL` | ✓ |
| `Youzan_Base.py:38 goods_url = '/wscvis/course/detail/goods.json'` | `youzan.go:19 goodsURL` | ✓ |
| `Youzan_Base.py:39 column_chapters_url = '/wscvis/knowledge/getColumnChapters.json'` | `youzan.go:20 columnChaptersURL` | ✓ |
| `Youzan_Base.py:40 column_contents_url = '/wscvis/knowledge/contentAndLive.json'` | `youzan.go:21 columnContentsURL` | ✓ |
| `Youzan_Base.py:41 simple_url = '/wscvis/course/getSimple.json'` | `youzan.go:22 simpleURL` | ✓ |
| `Youzan_Base.py:42 live_link_url = '/wscvis/knowledge/getLiveLink.json'` | `youzan.go:23 liveLinkURL` | ✓ |
| `Youzan_Base.py:43 edu_live_link_url = '/wscvis/course/live/video/getEduLiveLink.json'` | `youzan.go:24 eduLiveLinkURL` | ✓ |
| `Youzan_Base.py:44 room_url = '/wscvis/course/live/video/room'` | `youzan.go:25 roomURL` | ✓ |
| `Youzan_Base.py:45 asset_state_url = '/wscvis/course/detail/getAssetStateV2.json'` | `youzan.go:26 assetStateURL` | ✓ |

## HTTP 调用

| 源码方法 | Go 函数 | method | 一致? |
| --- | --- | --- | --- |
| `_configure_from_url` parses shop host, alias, kdt id | `configure` | local parse | ✓ |
| `_api_url`, `_request_text`, `_request_json` | `apiURL`, `requestText`, `requestJSON` | GET + JSON parse | ✓ |
| `_get_goods` requests `goods_url` with `alias/kdtId` | `Extract` calls `requestJSON(goodsURL, alias/kdtId)` | GET | ✓ |
| `_get_asset_state` requests `asset_state_url` | `buildMediaEntries` | GET | ✓ |
| `_live_media_urls` requests `live_link_url`, `edu_live_link_url`, `simple_url`, `room_url` | `buildMediaEntries` | GET | ✓ |
| `_page_media_urls` requests `/wscvis/course/detail/{alias}` | `buildMediaEntries` | GET | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 解析 | 一致? |
| --- | --- | --- |
| `_configure_from_url`: query/fragment `alias`, `columnAlias`, `fromColumn`, `kdt_id`, `kdtId`, path `/wscvis/course/detail/{alias}` and `/wscvis/knowledge/...` | `configure` parses same keys/forms | ✓ |
| `_json_params` / `_legacy_params`: `kdtId` / `kdt_id` | `apiURL` sends both when known | ✓ |
| `_resolve_goods_title`: `data.goodsData.content/column.title/name/alias` | `resolveTitle` walks maps for `title/name/alias` | ✓ |
| `_extract_media_urls`: regex media URLs and fields containing `url` or `source` | `extractMediaURLs` uses the same media regex family and url/source walk | ✓ |
| `_build_media_entries`: `data.goodsData`, `content`, `videoContentDTO`, `url`, `size`, `content_type` | `buildMediaEntries` extracts media from goods JSON and fallback API payloads | ✓ |

## 阻塞步骤

无. The extractor implements the source's request/parse cascade and returns media entries rather than doing downloader-side file conversion.
