# xiaoetech 源码对齐对照

## URL 常量

| .cdc.py 行 | xiaoetech.go / helpers.go | 一致? |
|---|---|---|
| Xiaoetech_Base.py:37 `referer = 'https://study.xiaoe-tech.com'` | xiaoetech.go:22 `refererURL` | ✓ |
| Xiaoetech_Base.py:40 `course_list_url` | xiaoetech.go:24 `courseListURL` | ✓ |
| Xiaoetech_Base.py:41 `quanzi_list_url` | xiaoetech.go:25 `quanziListURL` | ✓ |
| Xiaoetech_Base.py:44 `living_live_list_url` | xiaoetech.go:27 `livingLiveListURL` | ✓ |
| Xiaoetech_Course.py:36 `course_url` | xiaoetech.go:26 `courseURL` | ✓ |
| Xiaoetech_Course.py:37 `pc_course_url` | xiaoetech.go:28 `pcCourseURL` | ✓ |
| Xiaoetech_Course.py:38 `info_url` / 39 `pc_info_url` | xiaoetech.go:29 `infoURL` | ✓ |
| Xiaoetech_Course.py:42 `video_play_url` | xiaoetech.go:30 `videoPlayURL` | ✓ |
| Xiaoetech_Course.py:43 `source_url` / 44 `pc_source_url` | xiaoetech.go:31 `sourceURL`, `pcSourceURL` | ✓ |
| Xiaoetech_Course.py:45 `live_url` / 46 `protected_live_url` | xiaoetech.go:32-33 `liveURL`, `protectedLiveURL` | ✓ |
| Xiaoetech_Course.py:48 `pc_live_url` | xiaoetech.go:34 `pcLiveURL` | ✓ |
| Xiaoetech_Course.py:50 `audio_url` / 51 `pc_audio_url` | xiaoetech.go:35-36 `audioURL`, `pcAudioURL` | ✓ |
| Xiaoetech_Course.py:52 `text_url` | xiaoetech.go:37 `textURL` | ✓ |
| Xiaoetech_Course.py:58 `file_url` | xiaoetech.go:38 `fileURL` | ✓ |

## URL / 域名匹配

| Python 参考 | Go 实现 | 一致? |
|---|---|---|
| Mooc_Config.py:24-26 `XIAOETECH_H5_HOST_PATTERN` 支持 `{app_id}.h5[.*].xiaoeknow.com`, `xiaoecloud.com`, `xe-live.com`, `xet.*` | xiaoetech.go `appHostRe` + `normalizeXETDomain` | ✓ |
| Mooc_Config.py:240 `Xiaoetech_App` 与 241-249 各资源 regex 支持多级小鹅通域名 | xiaoetech.go `patterns` 使用 `(?:[\w-]+\.)*...` 通配商户子域 | ✓ |
| Xiaoetech_Live.py:105/116 等 H5 `referer = https://{app_id}{xet_domain}` | helpers.go `referer` H5 分支返回商户根域, `media` 透传条目 referer | ✓ |
| Xiaoetech_App.py:281-288, Xiaoetech_Course.py:736-744 支持 `/vN/course/alive`, `/vN/course/*`, `/vN/goods/goods_detail` | xiaoetech.go `pageRe`, `parseCtx`, `typeFromResourceID` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 | method | 一致? |
|---|---|---|---|
| Xiaoetech_App._fetch_attend_list | helpers.go `fetchCourseList` | GET + JSON parse | ✓ |
| Xiaoetech_App._fetch_living_live_list | helpers.go `fetchCourseList` | GET + JSON parse | ✓ |
| Xiaoetech_Course._get_m3u8_info | helpers.go `videoMediaURL` | POST + JSON parse | ✓ |
| Xiaoetech_Course._get_m3u8_info live branch | helpers.go `liveMediaURL` | GET + JSON parse | ✓ |
| Xiaoetech_Course._get_m3u8_info audio/text/column branch | helpers.go `postDetailURL` | POST + JSON parse | ✓ |
| Xiaoetech_Course._get_infos / URL sync | xiaoetech.go `parseCtx`, `enrichFromHTML` | GET + regex parse | ✓ |

## JSON 字段映射

| 源码 key 链 | Go 解析 | 一致? |
|---|---|---|
| `data.list` from attend list | helpers.go `listUnder(root["data"], "list")` | ✓ |
| `resource_id`, `cid`, `id` | helpers.go `itemFromMap` | ✓ |
| `resource_type`, `course_type` | helpers.go `itemFromMap` + `normType` | ✓ |
| `h5_url`, `url`, `live_share_url` | helpers.go `itemFromMap` | ✓ |
| `video_urls`, `video_info.play_sign` | helpers.go `videoMediaURL` / `deepText` | ✓ |
| `aliveVideoUrl`, `aliveVideoMp4Url`, `aliveVideoUrlEncrypt`, `miniAliveVideoUrl`, `aliveReviewUrl` | helpers.go `firstMediaURL` | ✓ |
| `window.APPID`, `window.USERID`, `<title>` | helpers.go `enrichFromHTML` | ✓ |

## 阻塞步骤

无.

## R2 critical follow-up

| 缺口 | 处理结果 |
|---|---|
| 多资源类型 endpoint 路由 | `resolveItem` 已拆分 text -> `xe.course.business.get.detail`, book -> `xe.course.business.ebook.info`, document/file -> `xe.course.business.courseware_list.get`, audio -> `xe.course.business.audio.info.get`; column/member/ecourse/train 保持 column items API. |
| protected live/private lookback | live/video 分支现在检测 `aliveVideoUrlEncrypt`, `__ba`, `distribute.vod.pri.get`; 解码私有 m3u8 后追加 `time`/`uuid`, 拉取并重写 m3u8, 相对分片转绝对 URL, 按 `ext.host/path/param` 重写分片与 `BYTERANGE`, 私有 key 通过 `uid` 请求并内联为 hex key. |
