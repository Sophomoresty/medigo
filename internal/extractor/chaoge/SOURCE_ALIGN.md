# chaoge source alignment

Decompiler source: `~/code/xwz-downloader-source-release/decompiled_full/Mooc/Courses/Chaoge/Chaoge_Course.pyc.1shot.cdc.py`.

| Source construct | Decompiled line(s) | Go implementation | Status |
| --- | ---: | --- | --- |
| Course detail API `https://chaogejiaoyu.com/course/index/getCourseDetailAjax?id={cid}&get_offline_info=0` | 37, 459-474 | `courseDetailURL`, `fetchCourseDetail` in `chaoge.go:18,135-146` | Aligned |
| Course file API `https://chaogejiaoyu.com/course/index/getCourseFileListAjax?course_id={course_id}` | 40, 579-591 | `courseFileURL`, `fetchCourseFiles` in `chaoge.go:21,148-163` | Aligned |
| Series API `https://chaogejiaoyu.com/course/index/getSeriesCourseListAjax?pid={pid}&is_end={is_end}&page={page}&huifang_sort=1&page_size=1000` | 38, 594-619 | `seriesURL`, `fetchSeriesItems` in `chaoge.go:19,165-201` | Aligned |
| Room HTML `https://chaogejiaoyu.com/course/room/{course_id}` | 39, 762-777 | `enterCourseURL`, `parseCCInfo` in `chaoge.go:20,244-260` | Aligned |
| `let ccInfo = {...}` regex and key/value extraction | 1620-1697 | `ccInfoBlockRe`, `ccKeyValueRe`, `url.QueryUnescape` in `chaoge.go:39-40,250-258` | Aligned |
| CSSLCloud replay login/play/meta endpoints | 43-45, 1698-1791 | Uses required shared helper `shared.CssLcloudResolvePlayInfo` in `chaoge.go:208-224`; source endpoint strings retained in `Extra` | Aligned by shared helper |
| CSSLCloud encrypted m3u8 key rewrite | playback helper chain | Uses required shared helper `shared.CssLcloudRewriteM3U8Keys` in `chaoge.go:262-273` | Aligned by shared helper |
| File material entries | 1081-1157 | `resolveFileEntry` maps `path/url/file_url/file` and suffix/extension in `chaoge.go:234-242` | Aligned |
| Output entries | `_get_infos` 1581-1619 | `Extract` returns top-level `MediaInfo{Entries: entries}` in `chaoge.go:52-102` | Aligned |

## Self-review

| Check | Result |
| --- | --- |
| Real HTTP calls present | `GetString` for login check, course detail, file list, series pages, room page, and optional m3u8 manifest. |
| Real parsing present | `json.Unmarshal` for API JSON and regex extraction for `ccInfo`. |
| Stub text absent | No stub sentinel phrases remain. |
| Csslcloud hard rule | `shared.CssLcloudResolvePlayInfo` is used; no inline platform login/play implementation. |
| Known deviation | Cookie validation is a Go-side guard before the source-aligned course flow; it does not alter source endpoint parsing. |
