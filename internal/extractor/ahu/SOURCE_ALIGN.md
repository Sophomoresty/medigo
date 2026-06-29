# Ahu 源码对齐

## URL 常量

| Python 源码/反编译 | Go 位置 | 一致? |
|---|---|---|
| `Ahu_Course.course_list_url`, `course_info_url`, `video_play_url` | `ahu.go` constants | ✓ |

## 入口与登录

| Python 方法 | Go 实现 | 一致? |
|---|---|---|
| `Ahu_Base._check_cookie` 检查 `PHPSESSID`/`laravel_session`, GET `/center/mycourse.html?_=...` | `validateAhuLogin`, `hasAhuLoginCookie` | ✓ |
| `_get_cid` URL 解析失败时枚举 `mycourse` | `ahuCourse.prepare` + `fetchCourseList` | ✓ |
| `_get_detail_soup` GET 课程详情页 | `ahuCourse.getInfos` | ✓ |

## 下载信息树

| Python 方法 | Go 实现 | 一致? |
|---|---|---|
| `_tree_node`, `_node_has_sources`, `_tree_to_download_info`, `_trees_to_download_info`, `_sort_tree_nodes` | `ahuTreeNode`, `treeNode`, `nodeHasSources`, `treeToDownloadInfo`, `treeMapToDownloadInfo`, `sortTreeNodes` | ✓ |
| `_parse_course_videos` 解析课时并生成视频 source | `parseCourseVideos` | ✓ |
| `_parse_handouts_from_scripts`, `_extract_js_array_assignments`, `_parse_course_files` | `extractJSONArrayAssignments`, `parseCourseFilesTree`, `resourceRefsFromAny` | ✓ |
| `_normalize_match_text`, `_match_text_variants`, `_best_variant_match_len`, `_find_file_target_outline_node` | `normalizeMatchText`, `matchTextVariants`, `bestVariantMatchLen`, `findFileTargetOutlineNode` | ✓ |
| `_ensure_file_path_node`, `_ensure_named_file_node`, `_append_file_to_nodes`, `_build_file_info` | `ensureFilePathNode`, `ensureNamedFileNode`, `appendFileToNodes`, `buildFileInfo` | ✓ |
| `_get_infos` 同时填充课程视频和资料 `_source_info` | `ahuCourse.getInfos` 填充 `infos`/`sourceInfo` | ✓ |

## 播放与下载链路

| Python 方法 | Go 实现 | 一致? |
|---|---|---|
| `_get_play_info` 返回 Aliyun/Baijiayun/direct 分支 | `ahuCourse.getPlayInfo`, `resolveLesson` | ✓ |
| `_decode_aliyun_play_auth` | `decodeAliyunPlayAuth` + `shared.AliyunDecodePlayAuth` | ✓ |
| `_request_aliyun_play_info_by_rand`, `_request_aliyun_play_info_legacy`, `_request_aliyun_play_info` | `requestAliyunPlayInfoByRand`, `requestAliyunPlayInfoLegacy`, `requestAliyunPlayInfo` | ✓ |
| `_extract_aliyun_key_material`, `_request_aliyun_license`, `_build_aliyun_key_func` | `extractAliyunKeyMaterial`, `requestAliyunLicense`, `buildAliyunKeyFunc` | ✓ |
| `_download_baijiayun_playback` | `downloadBaijiayunPlayback` + `shared.BaijiayunResolvePlayback` | ✓ |
| `_download_video`, `_download_video_list`, `_download_one_file`, `_download_file_list` | `downloadVideo`, `downloadVideoList`, `downloadOneFile`, `downloadFileList` | ✓ |
| `_split_download_info`, `_download_source_tree`, `_download_course`, `_download_files`, `_download`, `download` | `splitDownloadInfo`, `downloadSourceTree`, `downloadCourse`, `downloadFiles`, `DownloadAhu`, `ahuCourse.download` | ✓ |

## 测试覆盖

- `ahu_aliyun_test.go`: AliyunVoDEncryption m3u8 variant, MTS license, data URL key inline.
- `download_test.go`: 文件树匹配, split/download info, legacy/by-rand/license/key-material 兼容 API, `DownloadAhu` 实际落盘.
- `ahu_golden_test.go`: 登录检查, 课程页同时产出视频和资料 entries.

## 阻塞步骤

无。
