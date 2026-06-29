# Plaso 源码对齐对照

## Python 参考

- `Plaso_Base.py`: cookie/access-token 头, Plaso/Aiwenyun/Jhpy host 分支, player URL XOR hex 加密.
- `Plaso_Course.py`: 课程列表, 历史课, 作业, 分享单课, 文件详情, 目录任务, Aliyun/Polyv/STS 端点.
- `Plaso_Local*.pyc` 函数级反编译: plist/local media, `playUrls` 清晰度选择, m3u8 文本/白板资源链路.

## Go 对齐点

| Python 逻辑 | Go 实现 |
|---|---|
| Plaso/Aiwenyun/Jhpy 同路径不同 host | `newPlasoEndpoints`, endpoint path 常量动态拼 base |
| cookie 中 `access_token` 写入 `access-token`, 登录校验 | `plasoEndpoints.headers`, `checkCookie` |
| `course_url`, `course_list_url`, `package_list_url`, `history_list_url`, `homework_list_url` | `fetchCourseList`, `courseInfoFromMap` |
| 分享单课 `newGetShareInfo`, 文件详情 `getfileinfo` | `fetchShareOrFile` |
| 包/目录/章节/任务资料 `package/task/list`, `getXFileGroupInfo`, `getXfgTask` | `fetchPackageFiles` + `collectFileItems`, 保留 `Chapter`/`Index` |
| `fileId`, `originId`, `_id`, `myid`, `location`, `locationPath`, `vid`, `storageId` 等字段 | `buildFileItem`, `expandFileDetails` |
| Aliyun `getPlayInfo`, STS + GetPlayInfo/GetLicense | `fetchAliPlaySource`, `fetchAliSTSPlaySource`, shared Aliyun helper |
| Polyv `getPolyvVidInfoV2`, secure manifest, `polyvViewSign`, video-info fallback | `fetchPolyvSource`, shared Polyv helper |
| plist/local media `media`, `playUrls`, `m3u8Url`, `path`, audio | `fetchPlistSource`, `pickPlistMedia`; `audioPath` 使用 `file.plaso.cn/teaching` |
| OSS direct document STS signed URL | `buildDirectDocumentSource`, `buildPlasoCourseSTSSignedURL` (OSS V4, V1 fallback) |
| player URL XOR hex/兜底 HTML | `plasoPlayerURLEncrypt`, `buildPlayerSource` |
| `file.plaso.cn` / `filecdn.plaso.*` 资源 URL | `resolveDirectResourceMedia`, `buildPlasoEventAudioURL` |
| `ppt-player-*.plaso.com` native/static 资源入口 | `resolveNativeStaticMedia`, `plasoNativeStaticResources` |

## 验证

- `go test ./internal/extractor/plaso`
- `go vet ./internal/extractor/plaso/...`
