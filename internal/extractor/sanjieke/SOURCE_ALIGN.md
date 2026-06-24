# sanjieke 源码对齐对照

## URL 常量

| .cdc.py 行 | sanjieke.go 行/名 | 一致? |
|---|---|---|
| Sanjieke_Base.py:33 `referer = 'https://study.sanjieke.cn/'` | sanjieke.go:18 `urlReferer` | ✓ |
| Sanjieke_Base.py:35-39 classroom/course/user URLs | sanjieke.go:20-24 `urlClassroom/urlCourseList/urlCourseCatalog/urlUserInfo` | ✓ |
| Sanjieke_Base.py:40-41 api key/domain prefix | sanjieke.go:33-34 `apiKey/domainPrefix` | ✓ |
| Sanjieke_Course.py:30 `study_api_root = 'https://web-api.sanjieke.cn/b-side/api/web/study/{project_id}/{course_id}'` | sanjieke.go:25 `urlStudyAPIRoot = .../%s/%s` | ✓ |
| Sanjieke_Course.py:31-34 info/tree/content/attachment | sanjieke.go:26-29 `urlStudyInfo/urlTree/urlSection/urlAttachmentList` | ✓ |
| Sanjieke_Course.py:35 `study_page_url = 'https://study.sanjieke.cn/course/{project_id}/{course_id}'` | sanjieke.go:30 `urlStudyPage = .../%s/%s` | ✓ |
| Sanjieke_Course.py:36 `video_auth_url = 'https://service.sanjieke.cn/video/master/auth'` | sanjieke.go:31 `urlVideoAuth` | ✓ |
| Sanjieke_Course.py:37 `public_product_url = 'https://www.sanjieke.cn/course/detail/sjk/{}'` | sanjieke.go:32 `urlPublicProduct = .../%s` | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Sanjieke_Base._request_course_list_json line 422 | sanjieke.go:157-164 `fetchCourseList` | GET | ✓ |
| Sanjieke_Course._get_study_info decrypted `t300` | sanjieke.go:131-137 | GET | ✓ |
| Sanjieke_Course._get_tree_data decrypted `t308` | sanjieke.go:139-145 | GET | ✓ |
| Sanjieke_Course._get_content_data decrypted `t316` | sanjieke.go:203-210 `entriesFromContent` | GET | ✓ |
| Sanjieke_Course._get_video_auth_url decrypted `t601` | sanjieke.go:238-247 `authVideoURL` | GET | ✓ |
| Sanjieke_Base._resolve_http_download_m3u8 decrypted `t503` | sanjieke.go:250-260 `fetchMediaM3U8` | GET | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct tag | 一致? |
|---|---|---|
| `_get_course_list`: `code`, `data.list[]` | `courseListResp.Code`, `Data.List json:"list"` | ✓ |
| `_normalize_course_info`: `class_id`, `course_id`, `study_course_id`, `project_id`, `projectId`, `studying_url` | `courseItem` matching tags | ✓ |
| `_get_study_info`: `code`, `data.title`, `data.name` | `infoResp.Code`, `Data.Title/Name` | ✓ |
| `_get_infos`: `code`, `data.tree`, `data.nodes`, `data.children` | `treeResp.Data.Tree/Nodes/Children` | ✓ |
| `_parse_content_branch`: `nodeId`, `id`, `name`, `title`, `children` | `node.NodeID/ID/Name/Title/Children` | ✓ |
| `_parse_content_branch`: `data.videoContent.contentId`, `resolutionRatioObjList[].url`, `items[].url`, `url` | `videoContent.ContentID/Ratios/Items/URL` | ✓ |
| `_get_video_auth_url`: `code`, `data.url` | anonymous auth response `Data.URL json:"url"` | ✓ |
| `_http_fetch_m3u8_payload`: `status`, `m3u8Text`, `keyPairs` | `fetchMediaM3U8` response tags | ✓ |

## 阻塞步骤

无。
