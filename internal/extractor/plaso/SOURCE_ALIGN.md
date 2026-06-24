# plaso 源码对齐对照

## URL 常量

| .cdc.py 行 | plaso.go 行/名 | 一致? |
|---|---|---|
| Plaso_Base.py:34 referer = 'https://www.plaso.cn' | plaso.go:17 referer | ✓ |
| Plaso_Course.py:37 course_url = 'https://www.plaso.cn/gt/servlet/group/getGroupsByActive' | plaso.go:18 course_url | ✓ |
| Plaso_Course.py:38 course_list_url = 'https://www.plaso.cn/course/api/v1/m/package/student/list' | plaso.go:19 course_list_url | ✓ |
| Plaso_Course.py:39 package_list_url = 'https://www.plaso.cn/course/api/v1/m/package/list' | plaso.go:20 package_list_url | ✓ |
| Plaso_Course.py:40 history_list_url = 'https://www.plaso.cn/liveclassgo/api/v1/history/listRecord' | plaso.go:21 history_list_url | ✓ |
| Plaso_Course.py:42 share_url = 'https://www.plaso.cn/sc/nc/newGetShareInfo' | plaso.go:23 share_url | ✓ |
| Plaso_Course.py:43 file_url = 'https://www.plaso.cn/yxt/servlet/file/preview/getfileinfo' | plaso.go:24 file_url | ✓ |
| Plaso_Course.py:44 file_info_url = 'https://www.plaso.cn/yxt/servlet/file/getfileinfo' | plaso.go:25 file_info_url | ✓ |
| Plaso_Course.py:45 info_url = 'https://www.plaso.cn/cs/xfilegroup/getXFileGroupInfo' | plaso.go:26 info_url | ✓ |
| Plaso_Course.py:46 package_url = 'https://www.plaso.cn/course/api/v1/nct/m/package/task/list' | plaso.go:27 package_url | ✓ |
| Plaso_Course.py:48 m3u8_url = 'https://www.plaso.cn/yxt/servlet/ali/getPlayInfo' | plaso.go:29 m3u8_url | ✓ |
| Plaso_Course.py:49 poly_sign_url = 'https://www.plaso.cn/yxt/servlet/file/preview/getPolyvVidInfoV2' | plaso.go:30 poly_sign_url | ✓ |
| Plaso_Course.py:50 m3u8_sign_url = 'https://www.plaso.cn/yxt/servlet/org/nc/polyvViewSign' | plaso.go:31 m3u8_sign_url | ✓ |
| Plaso_Course.py:51 poly_video_url = 'https://api.polyv.net/v2/video/5153980715/get-video-info' | plaso.go:32 poly_video_url | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Plaso_Course._get_course_list decrypted line 203 -> course_url/package_list_url/course_list_url/history_list_url | fetchCourseList lines 115-152 | POST | ✓ |
| Plaso_Course._get_cid decrypted line 283 -> share_url | fetchShareOrFile lines 180-206 | POST | ✓ |
| Plaso_Course._get_infos decrypted line 319 -> file_url/file_info_url | fetchShareOrFile lines 180-206 | POST | ✓ |
| Plaso_Course._download_chapter_list decrypted line 1169 -> info_url/package_url | fetchPackageFiles lines 154-178 | POST | ✓ |
| Plaso_Course._get_m3u8_url decrypted line 385 -> m3u8_url | fetchAliPlayURL lines 229-235 | POST | ✓ |
| Plaso_Course._get_poly_video_url decrypted line 402 -> poly_sign_url/poly_video_url/m3u8_sign_url | fetchPolyvURL lines 237-258 | POST + shared Polyv | ✓ |

## JSON 字段映射

| 源码 key 链 | Go parser | 一致? |
|---|---|---|
| list[].get('id'/'originId'/'packageId'/'fileGroupId') | fetchCourseList lines 135-144 | ✓ |
| list[].get('title'/'groupName'/'name'/'packageName') | fetchCourseList lines 135-144 | ✓ |
| obj.get('fileId'/'id'/'originId'/'_id') | buildFileItem line 261 | ✓ |
| obj.get('myid'/'myId'/'my_id') | buildFileItem line 261 | ✓ |
| obj.get('location'), get('locationPath'/'location_path') | buildFileItem line 261 | ✓ |
| m3u8 result get('hdPlayUrl'/'sdPlayUrl'/'ldPlayUrl') | fetchAliPlayURL line 234 | ✓ |
| poly result get('vid'), regex '"playUrl" ... m3u8' | fetchPolyvURL lines 239-254 | ✓ |

## 阻塞步骤

无. Polyv 分支已按硬规则调用 `shared.PolyvResolveSecure` / `shared.PolyvPickBestManifest`; Plaso 本地白板无损渲染属于下载器渲染阶段, Extract() 返回可下载媒体/文件 URL.
