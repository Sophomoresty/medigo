# nmkjxy 源码对齐对照

## URL 常量

| .cdc.py 行 | nmkjxy.go 行/名 | 一致? |
|---|---|---|
| Nmkjxy_Base.py:33 check_login_url = 'https://api.nmkjxy.com/api/V520/RecentCourse?PageSize=1&PageIndex=1&RecentMonth=false&status=1' | nmkjxy.go:20 check_login_url | ✓ |
| Nmkjxy_Course.py:30 course_url = 'https://api.nmkjxy.com/api/V520/RecentCourse?PageSize={page_size}&PageIndex={page_index}&RecentMonth=false&status=1' | nmkjxy.go:21 course_url | ✓, 占位符用 %s |
| Nmkjxy_Course.py:31 product_url = 'https://api.nmkjxy.com/api/product/{cid}' | nmkjxy.go:22 product_url | ✓, {cid}→%s |
| Nmkjxy_Course.py:32 video_list_url = 'https://api.nmkjxy.com/api/video/{cid}' | nmkjxy.go:23 video_list_url | ✓, {cid}→%s |
| Nmkjxy_Course.py:33 courseware_url = 'https://api.nmkjxy.com/api/V310/Courseware/{cid}' | nmkjxy.go:24 courseware_url | ✓, {cid}→%s |
| Nmkjxy_Course.py:34 legacy_courseware_url = 'https://api.nmkjxy.com/api/Courseware/{cid}' | nmkjxy.go:25 legacy_courseware_url | ✓, {cid}→%s |
| Nmkjxy_Course.py:35 recorded_video_url = 'https://apim.ningmengyun.com/api/MyOrder/RecordedVideoCourse?orderSn={order_sn}&productId={cid}' | nmkjxy.go:26 recorded_video_url | ✓, 保留常量 |
| Nmkjxy_Course.py:36 video_play_url = 'https://apim.ningmengyun.com/api/MyOrder/VideoPlayed?courseId={cid}&videoSn={video_sn}' | nmkjxy.go:27 video_play_url | ✓, {cid}/{video_sn}→%s |
| Nmkjxy_Course.py:37 video_played_url = 'https://apim.ningmengyun.com/api/MyOrder/VideoPlayed' | nmkjxy.go:28 video_played_url | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---|---|
| Nmkjxy_Base._check_cookie line 290-318 | headers/tokenFromJar/parseToken line 255-299 | GET check_login_url + Authorization | ✓ |
| Nmkjxy_Course._get_course_list line 389-431 | Extract line 53 + requestJSON line 108 | GET course list | ✓ |
| Nmkjxy_Course._load_product_info line 465-473 | Extract line 53 | GET product_url | ✓ |
| Nmkjxy_Course._get_infos line 568-619 | Extract line 59 + iterItems line 120 | GET video_list_url | ✓ |
| Nmkjxy_Course._get_video_play_info line 918-930 | Extract line 75 | GET video_play_url | ✓ |
| Nmkjxy_Course._get_courseware_info line 888-899 | fetchCourseware line 188 | GET courseware_url + legacy_courseware_url | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct / helper | 一致? |
|---|---|---|
| result.get('data') | dataMap / requestJSON | ✓ |
| item.get('id') | firstText(..., "id") | ✓ |
| item.get('productId') | firstText(..., "productId") | ✓ |
| item.get('prodName') | firstText(..., "prodName") | ✓ |
| item.get('name') | firstText(..., "name") | ✓ |
| item.get('videoId') / item.get('vodId') | firstText(..., "videoId", "vodId") | ✓ |
| item.get('videoSn') | firstText(..., "videoSn", "videoSN", "sectionSn", "id") | ✓ |
| item.get('videoNId') / item.get('id') | firstText(..., "videoNId", "id") | ✓ |
| data.get('playInfoList') | pickPlayInfo(playData["playInfoList"]) | ✓ |
| playInfo.get('playURL') | firstText(..., "playURL") | ✓ |
| playInfo.get('definition') | firstText(..., "definition") | ✓ |
| playInfo.get('size') | sizeBytes(...) | ✓ |
| subtitlePath/subtitleUrl/subTitlePath/subTitleUrl/srtPath/vttPath | subtitles(...) | ✓ |
| courseware groups files / legacy files | fetchCourseware(...) | ✓ |

## 阻塞步骤 (如果有)

无。
