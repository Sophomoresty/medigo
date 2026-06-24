# w3 extractor assessment

Scope: `huke88 icourse163 icourses icve imooc itbaizhan jianshe99 jinbangshidai jingtongxue kaimingzhixue kaoyanvip keqq koolearn kuke ledu`.

Note: `.codex/audit/w3-full-audit-r2.md` is not present in this worktree, so this assessment uses the current W3 audit, current extractor code, and the final code-review fixes in this branch as the authoritative baseline.

| site | rating | summary |
|---|---|---|
| huke88 | PASS | URL constants, auth cookie probe, course/list/video/file POST flows, confirm retries, and material output are source-aligned. |
| icourse163 | PARTIAL | Common MOOC lesson/video extraction works, but App/Textbook/Column/Kaoyan/package flows and some subtitle/attachment/course-list coverage remain outside the Go implementation. |
| icourses | PASS | CUOC and MOOC API roots, bearer/cookie auth, JSON keys, chapter/resource/doc APIs, and resource normalization are aligned. |
| icve | PASS | AI course/title/design/cell/status APIs, flexible JSON traversal, and video/file URL resolution match the source flow. |
| imooc | BLOCKED | Basic plain-HLS/free response handling exists, but paid playback is explicitly blocked on the original `imooc_decode` JS transform and course-level enumeration remains incomplete. |
| itbaizhan | PASS | Nav/stage/rightlist/play-page parsing and Polyv secure playback resolution are aligned with the source. |
| jianshe99 | PASS | Replay payload lookup, CSSLcloud play-info resolution, and m3u8 key rewriting use the shared helper and match the source flow. |
| jinbangshidai | PASS | Auth token/device handling, course/detail/play/token POST JSON APIs, syllabus traversal, and Baijiayun resolution are aligned. |
| jingtongxue | PASS | Course list/status pagination, detail/chapter/lecture/play-param APIs, direct media keys, and BokeCC fallback are aligned. |
| kaimingzhixue | PARTIAL | VOD/live playback paths are implemented, but source file/material nodes and public `courseBasis` price metadata are still missing. |
| kaoyanvip | PASS | Delivery/uuid outline traversal, VOD HLS/key-token rewrite, MP4 fallback, and live-record Polyv signing are aligned. |
| keqq | PASS | Purchased list fallback, `__NEXT_DATA__`, detail chapter merge, rec-video info/subtitles, and DRM token generation are aligned. |
| koolearn | PARTIAL | Direct Roombox class lesson/playback extraction works, but order/my-data/study-course discovery and broader Koolearn course trees are still missing. |
| kuke | PASS | Signed POST, order/course/SVIP/detail/subcourse APIs, Polyv node-info, and secure JS/m3u8 handling are aligned. |
| ledu | PASS | PC auth validation, class/detail/video/handout APIs, flexible payload extraction, and media/material URL mapping are aligned after the final error-handling fix. |

Final code-review fixes in this branch:

- `icourse163`: checked `home.htm`, `getLessonUnitLearnVo`, and timestamp request errors, and surfaces the first per-video resolution failure when no playable entries are produced.
- `imooc`: checked `startlearn` / `endlearn` errors for class/coding flows and stopped calling fake lifecycle endpoints for free `www.imooc.com` URLs.
- `koolearn`: propagated `module/info?module=playback` fallback errors instead of silently dropping the lesson.
- `ledu`: made the source PC cookie-validation request an explicit failure gate instead of discarding its error.
