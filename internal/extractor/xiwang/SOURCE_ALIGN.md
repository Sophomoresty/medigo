# xiwang 源码对齐对照

参考源码: `/home/sophomores/code/xwz-downloader-source-release/restored_source/Mooc/Courses/Xiwang/`, 字节码补充参考同目录 `decompiled_full/Mooc/Courses/Xiwang/*.das`.

## 站点与入口

| Python 源码 | Go 实现 | 状态 |
|---|---|---|
| `Xiwang_Base._check_cookie` | `Extract` 调 brand `checkLogin`, 用 `loginOKRe` 校验 `stat/status == 1` | ✓ |
| `Xiwang_Course`, `Xiwang_Suyang`, `Xiwang_Youke` URL 常量 | `brandConfig` 三组 endpoints, appVersion, couTypes, extra headers | ✓ |
| `_get_cid`, `_select_course` | `firstMatch`, `fetchCoursesBrand`, `selectCourse` | ✓ |
| `_get_price` | `fetchPriceBrand` 调 `/mall/detail/1/{cid}`, 解析 `data.priceModule.price`, 默认隐藏价 999 | ✓ |

## 课程列表与章节

| Python 源码 | Go 实现 | 状态 |
|---|---|---|
| `_get_course_list` | `fetchCoursesBrand` 按 `couType`, position+=8 分页 POST form, 合并 `learningCourses/endedCourses` | ✓ |
| `_get_infos` video list | `fetchLessonsBrand` POST form `planListV2`, 解析 `planId/planName/bizId` | ✓ |
| `_get_infos` file_dict | `fetchCoursewareFiles`, `appendCoursewareFiles` 递归解析 category/files/url | ✓ |

## 视频源解析

| Python 源码 | Go 实现 | 状态 |
|---|---|---|
| `_get_video_url` | `resolveLessonBrand`, `videoURLsForBizBrand` 先 biz=3, 主视频为空再 biz=2 | ✓ |
| playback enter payload | `fetchXiwangPlayInfoData` POST JSON `{acceptPlanVersion:42,bizId,planId,stuCouId}` | ✓ |
| `beforeClassFileId/videoFile/afterClassFileId` | `resolveXiwangMediaRef` + field aliases, 分别输出 before/main/after | ✓ |
| `_get_m3u8_urls` | `m3u8URLsBrand` 访问 vodshow, 解析 `content.addrs[].addr`, 支持嵌套 addr fallback | ✓ |
| `_select_available_media_url` | `prioritizeReachableMediaURLs` 用 HEAD 把可达候选提前, 不丢弃备用地址 | ✓ |
| direct CDN/mp4/m3u8 fallback | `xiwangDirectMediaURLsForKeys`, `looksXiwangVideoURL` 扫描 play-info 直接媒体 URL | ✓ |

## PPT, board 与文件

| Python 源码 | Go 实现 | 状态 |
|---|---|---|
| `_get_ppt_url_list` primary | `pptImagesBrand` 解析 `getTeacherNoteListV2 data.picData[].pic_url` | ✓ |
| `_get_xiwang_play_info_data` | `fetchXiwangPlayInfoData` | ✓ |
| `_get_xiwang_metadata` | `fetchXiwangMetadata` 优先 configs.urls `getMetadataUrl/getFdMetadataUrl`, fallback metadata/get | ✓ |
| `_extract_xiwang_board_resources` / `_get_xiwang_page_image_url_list` | `extractXiwangBoardResources`, `xiwangPageImageURLList` 解析 category=50 board event, pages/timeline, 输出页面图片 URL | ✓ |
| `_download_one_file` | `fileEntry`, `fileExtFromURL` 保留下载 URL 与扩展名 | ✓ |

## 验证

- `go vet ./internal/extractor/xiwang/...`
- `go test ./internal/extractor/xiwang/...`
