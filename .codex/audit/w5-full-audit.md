# w5 full extractor audit

Scope: `renrenjiang sanjieke shanxiang sier smartedu speiyou tmooc unipus wallstreets wangxiao wangxiao233 wendao wowtiku xiaoeapp xiaoetech`.

Checks performed:
- Go implementation: `internal/extractor/<site>/<site>.go` plus same-package helpers where used.
- Source reference: `~/code/xwz-downloader-source-release/decompiled_full/Mooc/Courses/<Site>/` decompiled source.
- Machine checks run before writing this report: `python3 scripts/verify_full_alignment.py` showed all scoped sites as `has HTTP+parse`; `go vet` and `go test` for scoped extractor packages returned success.

## renrenjiang

- Auth-flow issue: source auth check calls `/api/v3/account/get_user_info` (`Renrenjiang_Base.pyc.1shot.cdc.py:539`), but Go has no matching URL constant/call and can enter a direct `activity`/`column` path without that probe (`internal/extractor/renrenjiang/renrenjiang.go:46-75`). Cookie/token headers are used, but the source check endpoint is not copied.
- Code review: several HTTP errors are intentionally discarded (`internal/extractor/renrenjiang/renrenjiang.go:56,65,73,107,113,164-165`). No nil panic observed because values pass through nil-safe map helpers, but failures can be silently hidden.

## sanjieke

- Source-alignment issue: `urlUserInfo` is copied (`internal/extractor/sanjieke/sanjieke.go:24`) but never called, while the source auth flow includes the user-info check (`Sanjieke_Base.pyc.1shot.cdc.py` user/info path). This weakens cookie validation parity.
- Source-alignment issue: source defines and fetches `attachment_list_url = .../attachment/list` via `_get_attachment_list` (`Sanjieke_Course.pyc.1shot.cdc.py:34,541-557`), but Go only defines `urlAttachmentList` and never calls it (`internal/extractor/sanjieke/sanjieke.go:29`). Attachment/file entries from that branch are therefore omitted.

## shanxiang

- Auth-flow issue: source `_check_cookie` performs `GET https://www.sx1211.com/user/course.html` with redirects disabled (`Shanxiang_Base.pyc.1shot.cdc.py:198-205`). Go uses the same URL only as `Referer` while fetching the study page (`internal/extractor/shanxiang/shanxiang.go:40-41,89-90`), so the explicit login-check request is not copied.
- No CSSLcloud helper issue: playback resolution delegates to `shared.CssLcloudResolvePlayInfo`, not inline CSSL parsing.

## sier

- Source-alignment issue: source price/product discovery uses inline POST URLs `https://www.sieredu.com/web/product/detail` and `https://www.sieredu.com/web/course/getProductByCourseId` (`Sier_Course.pyc.1shot.cdc.py:261-275`), but Go has no constants/calls for these URLs (`internal/extractor/sier/sier.go:17-30`). This omits source metadata/price flow.
- Code review: unchecked/ignored errors in discovery/playback helpers can hide request failures (`internal/extractor/sier/sier.go:59,97,115,196,199`). `io.ReadAll` response bodies are closed and checked in the raw POST helper (`internal/extractor/sier/sier.go:292-303`).

## smartedu

- Source-alignment issue: source declares full private/public/oversea host triples for r1/r2/r3 (`Smartedu_Base.pyc.1shot.cdc.py:71-73`), but Go keeps only `r1-ndr-private` as a constant and rewrites private to public generically (`internal/extractor/smartedu/smartedu.go:24`, `internal/extractor/smartedu/helpers.go:219-225`). This is not byte-level coverage of the source URL constants.
- Code review: optional enrichment fetches discard errors (`internal/extractor/smartedu/smartedu.go:94,200-201`). No nil panic or response-body leak observed.

## speiyou

- Code review: source cookie/subject probe is present but its error is discarded (`internal/extractor/speiyou/speiyou.go:53`). A bad cookie may only fail later in course/live requests. No nil panic or response-body leak observed.

## tmooc

NO ISSUE

## unipus

- Code review: source free-join side effect is mirrored, but both POST and GET errors are discarded (`internal/extractor/unipus/unipus.go:153-157`). This is probably best-effort by design, but it is still an unchecked HTTP error path. Raw response bodies are closed and `ReadAll` is checked (`internal/extractor/unipus/unipus.go:136-145`).

## wallstreets

- Source-alignment issue: source `_get_m3u8_text` fetches the m3u8, fetches the key URI, applies `qiqiuyun_key_decode`, and rewrites the key URI in the returned text (`Wallstreets_Course.pyc.1shot.cdc.py:350-366`). Go only records metadata (`key_uri`, `key_bytes`, `key_decode`) and returns the selected playlist URL, without returning/rewriting the decoded m3u8 text (`internal/extractor/wallstreets/wallstreets.go:371-387`).
- Code review: unchecked HTTP errors in classroom discovery and key fetch (`internal/extractor/wallstreets/wallstreets.go:197,200,382`). No raw response-body leak observed.

## wangxiao

- Source-alignment issue: source extracts handout/download links from player pages and then forces `down.aspx` / `DownHandOut` URLs (`Wangxiao_Course.pyc.1shot.cdc.py:1616-1626`). Go defines `urlPlayerDown` and `urlLiveHandout` but `resolveRef` only resolves BokeCC video and never creates file/handout entries (`internal/extractor/wangxiao/wangxiao.go:26-29,110-145`).
- Code review: no nil panic or response-body leak observed.

## wangxiao233

- [CRITICAL] Source-alignment issue: the Aliyun branch is incomplete. Source calls `getPlayInfoAndAuth`, decodes `playAuth`, signs `https://vod.{region}.aliyuncs.com/?...Action=GetPlayInfo...AuthInfo=...`, and has a private license POST to `https://mts.{region}.aliyuncs.com/?` (`Wangxiao233_Course.pyc.1shot.cdc.py:1277-1300,1545-1554,1646-1654`). Go calls only `urlPlayAuth` and then searches that JSON directly for media URLs (`internal/extractor/wangxiao233/wangxiao233.go:241-246`); the VOD signed request and MTS license flow are absent, so Aliyun videos can fail.
- Code review: `apiPost` ignores `json.Marshal` and `io.ReadAll` errors (`internal/extractor/wangxiao233/wangxiao233.go:139,145`). Body is closed.

## wendao

- Code review: `requestJSON` ignores `json.Marshal` and `io.ReadAll` errors before JSON parse (`internal/extractor/wendao/wendao.go:146,153`). Body is closed and no nil panic observed.

## wowtiku

- [CRITICAL] Source-alignment issue: source Aliyun playback includes private-rand helper / `Rand`, `PlayConfig={"EncryptType":"AliyunVoDEncryption"}`, signed VOD request, and MTS `GetLicense` POST (`Wowtiku_Course.pyc.1shot.cdc.py:251-281,1133-1157,1229-1252,1296-1316`). Go only signs a simpler `GetPlayInfo` URL from STS (`internal/extractor/wowtiku/wowtiku.go:205-229`) and never implements the MTS license/key callback, so encrypted Aliyun streams can fail.
- Source-alignment issue: `playTokenAPI` is copied (`internal/extractor/wowtiku/wowtiku.go:31`) but the source `_get_play_token` branch (`Wowtiku_Course.pyc.1shot.cdc.py:1010-1019`) is not wired into Go playback.

## xiaoeapp

- [CRITICAL] Source-alignment issue: protected/private live lookback is missing. Source first calls H5 `/_alive/v3/get_lookback_list`, decrypts private m3u8 URLs, appends time/uuid, fetches m3u8 text, rewrites private segments/keys, and calls `/app/xe.vod.privatekey.get/1.0.0` for AES key material (`Xiaoeapp_Course.pyc.1shot.cdc.py:1185,1277,1346-1370,1426-1508,1614-1639,1809-1811`). Go only posts `/app/alive/xe.alive.lookbackurl.get/1.0.0` and returns the first URL (`internal/extractor/xiaoeapp/xiaoeapp.go:137-145`), so protected lookbacks can return unusable encrypted/private playlists.
- Code review: `postAppAPI` ignores `io.ReadAll` error (`internal/extractor/xiaoeapp/xiaoeapp.go:187-192`). Body is closed.

## xiaoetech

- [CRITICAL] Source-alignment issue: resource-type endpoint dispatch is wrong/incomplete. Source has distinct endpoints for column, member, topic, text, ebook, file, audio, video, and rich-text iframe media (`Xiaoetech_Course.pyc.1shot.cdc.py:52-58` plus related endpoint constants). Go routes `text`, `book`, `document`, `column`, `bigcolumn`, `member`, `ecourse`, and `train` all through `infoURL = ...column.items.get` and leaves `textURL`/`fileURL` unused (`internal/extractor/xiaoetech/xiaoetech.go:24,33-34`; `internal/extractor/xiaoetech/helpers.go:83-85`). This can fabricate the wrong API path for non-column resources.
- [CRITICAL] Source-alignment issue: protected/private live m3u8 normalization is missing. Source decrypts protected live URLs, fetches m3u8 text, rewrites private segments, and inlines private keys (`Xiaoetech_Course.pyc.1shot.cdc.py:979,1072,1141-1165,1212-1223,1252-1287`). Go `liveMediaURL` only calls protected/live JSON endpoints and returns the first URL (`internal/extractor/xiaoetech/helpers.go:123-142`).
- Source-alignment issue: source rich-text video/audio endpoints `https://iframe.xiaoeknow.com/api/richtext/get_video_data` and `.../get_audio_data` are not present in Go (`Xiaoetech_Course.pyc.1shot.cdc.py:56-57,1344-1353`).
