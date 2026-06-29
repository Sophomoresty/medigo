# smartedu 源码对齐对照

## URL 常量

| .cdc.py 行 | smartedu.go 行/名 | 一致? |
|---|---|---|
| `Smartedu_Base.py:42 referer = 'https://basic.smartedu.cn/'` | `smartedu.go:16 refererURL` | ✓ |
| `Smartedu_Base.py:43 login_url = 'https://auth.smartedu.cn/uias/login'` | `smartedu.go:17 loginURL` | ✓ |
| `Smartedu_Base.py:44-46 static_bases/static_base` | `smartedu.go:18-24 staticBase0/staticBase1/special0..special3` | ✓ |
| `Smartedu_Base.py:61-63 host_private/host_public/host_oversea` | `smartedu.go:49-67 privateHosts/publicHosts/overseaHosts` + `helpers.go:82-160 normalizeStorageCandidates/expandSmarteduCDNHosts` | ✓ |
| `Smartedu_Base.py:47-70 各资源模板 URL` | `smartedu.go:26-37` | ✓ |

## 认证 / Header

| 源码方法 (line) | Go 函数 (line) | 一致? |
|---|---|---|
| `_load_auth_from_cookie` line 292 | `decodeAccessToken` + `newCtx` lines 135, 242 | ✓ |
| `_request_headers` line 350 | `newCtx` line 137-145 | ✓ |
| `_check_cookie` line 391 | `Extract` 前置 cookie 约束 + `newCtx` line 135 | ✓ |

## HTTP 调用

| 源码方法 (line) | Go 函数 (line) | method | 一致? |
|---|---|---:|---|
| `_json_get` line 140 | `getFirst` line 146-167 | GET + JSON parse | ✓ |
| `_load_teachingmaterial_detail` line 268 | `loadActivity` line 169-178 / `loadTchMaterialThematic` line 199-208 | GET | ✓ |
| `_load_teachingmaterial_resources` line 305 | `loadTeachingResources` line 191-197 | GET | ✓ |
| `_load_tch_material_detail` line 493-515 | `loadTchMaterialThematic` line 199-208 | GET | ✓ |
| `_append_resource_source` line 586 | `sourceFromResource` line 242-257 | JSON select + URL build | ✓ |

## JSON 字段映射

| 源码 key 链 | Go struct/tag 或解析 | 一致? |
|---|---|---|
| `contentId/contentType/catalogType/subCatalog` | `Extract` query parse lines 77-96 | ✓ |
| `activityId/chapterId/teachingmaterialId/classHourId/fromPrepare` | `Extract` query parse lines 99-107 | ✓ |
| `result.data / root JSON object` | `getFirst` + `json.Unmarshal` line 155 | ✓ |
| `relations.national_course_resource/tch_materials/basic_works/prepare_lessons/elite_lessons` | `relationResources` line 78-85 | ✓ |
| `ti_items` | `items`/`selectVideoItem`/`selectFileItem` in helpers.go | ✓ |
| `ti_storage / ti_storages / ti_file_flag / ti_format / ti_size` | `itemURL`/`itemURLs`, `selectVideoItem`, `selectFileItem`, `itemSize` in helpers.go | ✓ |
| `custom_properties.identification` / private URL accessToken | `withAccess` line 259-273 + `decodeAccessToken` helpers.go | ✓ |
| `cs_path:${ref-path}` + r1/r2/r3 ndr CDN nodes | `normalizeStorageCandidates`, `privateURLsToPublic`, stream `url_mode=mirror`, download mirror fallback | ✓ |

## 阻塞步骤

无。smartedu 的实现已覆盖查询参数分流, JSON GET 候选, 资源选择, private/public/oversea CDN 节点候选, private URL accessToken 和课程/单资源返回。
