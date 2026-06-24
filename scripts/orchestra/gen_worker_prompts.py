#!/usr/bin/env python3
"""Generate worker prompts for medigo-extractors-v2 batch 1."""
import json
import os

DISPATCH = os.path.expanduser("~/code/medigo/.codex/tasks/medigo-extractors-v2/dispatch.json")
OUT_DIR = os.path.expanduser("~/code/medigo/.codex/tasks/medigo-extractors-v2/prompts")

PROMPT_TEMPLATE = """# medigo-extractors-v2 worker-{worker} (batch 1)

你的工作目录: `{worktree}` (git branch `{branch}`).
任务清单: {site_list}

## 硬性规则 (违反任何一条都得重做该站)

R1: 每站 `internal/extractor/<site>/<site>.go` 的 `Extract()` 函数必须真实调 HTTP + 真实解析响应。
    禁止: `return nil, fmt.Errorf("...not yet implemented...")`
    禁止: 返回 `*MediaInfo` 但 `Streams` 为空且加 "api_response_received" 标记的假成功。

R2: URL 常量原样照抄反编译源码:
    `~/code/xwz-downloader-source-release/decompiled_full/Mooc/Courses/<SourceDir>/`
    源码 .cdc.py 里 `url_xxx = 'https://...'` 复制成 Go 常量, 不允许改名/改路径。
    占位符 `{{cid}}` 在 Go 里变 `%s` (用 fmt.Sprintf 拼)。

R3: JSON 解析路径照抄源码 `dict.get('xxx')` 的 key 链 → Go `struct` tag `json:"xxx"`。
    源码用 result.get('data',{{}}).get('list',[]) 就 Go struct `Data: {{ List: ... }}`。
    不允许改 key 名, 不允许换嵌套层级。

R4: 认证流程照抄源码 (cookie 名/Referer/header)。
    `_check_cookie` / `set_cookie` 怎么写的, Go 也怎么写。

R5: 多视频课程返回 `*MediaInfo.Entries`, 单视频站返回 `Streams`。

R6: csslcloud / polyv / bokecc / baijiayun 平台必须走 `internal/extractor/shared/` 的现有 helpers,
    不要在站包里重写签名/解密逻辑。csslcloud helper 入口是 `shared.CssLcloudResolvePlayInfo`,
    polyv 是 `shared.PolyvResolveSecure`, bokecc 是 `shared.BokeCCResolve`,
    baijiayun 是 `shared.BaijiayunResolveVOD` / `shared.BaijiayunResolvePlayback`。

R7: 加密方法体在 .cdc.py 里看到 `b'\\x81...'` 或方法体空白时, 查
    `~/code/xwz-downloader-source-release/decrypted_full/all_decrypted.json`
    key 格式: `Courses/<SourceDir>/<SourceDir>_Course__t<line>_<funcname>.pyc`
    取 `decrypted` + `readable_consts` 重组逻辑。

## 工作流 (每站 30-90 分钟, 严格按顺序)

### Step 1: 读源码

```bash
SITE=<site_name>          # 站名小写
SRC_DIR=<SourceDir>       # 源码目录名 (大写)
SRC=~/code/xwz-downloader-source-release

# 列出该站所有 .cdc.py 文件
ls $SRC/decompiled_full/Mooc/Courses/$SRC_DIR/

# 抽 URL 常量
rg -on "https?://[^'\\\"]{{15,}}|url_[a-z_]+\\s*=" \\
   $SRC/decompiled_full/Mooc/Courses/$SRC_DIR/*.cdc.py | head -30

# 抽 URL regex (用于 Go pattern)
rg -n "${{SRC_DIR}}_(Course|Base|Mooc|Class)':" \\
   $SRC/decompiled_full/Mooc/Mooc_Config.pyc.1shot.cdc.py
```

### Step 2: 加密方法体查 all_decrypted.json

```bash
python3 -c "
import json
d=json.load(open('/home/sophomores/code/xwz-downloader-source-release/decrypted_full/all_decrypted.json'))
for k in sorted(d):
    if '$SRC_DIR' in k and ('infos' in k or 'video' in k or 'play' in k or 'login' in k or 'cid' in k):
        print(k)
        print(' ', d[k][0]['readable_consts'][:8] if d[k][0].get('readable_consts') else 'no consts')
"
```

### Step 3: 写 Go (internal/extractor/$SITE/$SITE.go, 上限 400 行)

参考已完成的 PASS 样本:
- `internal/extractor/icourse163/icourse163.go` (DWR + signed VOD chain)
- `internal/extractor/xuetang/xuetang.go` (course tree + leaf + playurl)
- `internal/extractor/zhihuishu/zhihuishu.go` (initVideo + changeVideoLine)
- `internal/extractor/ahu/ahu.go` (HTML parse 模板, 单 API 站)

### Step 4: self-review 对照表 (强制)

每写完一站, 在 `internal/extractor/<site>/SOURCE_ALIGN.md` 写一份对照表:

```markdown
# <site> 源码对齐对照

## URL 常量

| .cdc.py 行                                          | <site>.go 行/名                  | 一致? |
|-----------------------------------------------------|----------------------------------|-------|
| Ahu_Course.py:39 url_courseinfo = '.../courseinfo?courseId={{cid}}' | ahu.go:23 urlCourseInfo = ".../courseinfo?courseId=%s" | ✓ |

## HTTP 调用

| 源码方法 (line)                | Go 函数 (line)                | method | 一致? |
|--------------------------------|-------------------------------|--------|-------|
| Ahu_Course._get_infos line 65  | ahu.go fetchLessons line 47   | GET    | ✓ |

## JSON 字段映射

| 源码 key 链                              | Go struct tag                | 一致? |
|------------------------------------------|------------------------------|-------|
| result.get('data',{{}}).get('list',[])     | Data.List `json:"list"`       | ✓ |

## 阻塞步骤 (如果有)

无 / <步骤> 因 <原因> 阻塞, 已返回 "blocked: needs <X>" error.
```

### Step 5: 验证

```bash
cd $WORKTREE
go build ./... && go vet ./internal/extractor/$SITE/...
python3 scripts/verify_full_alignment.py | grep $SITE
# 必须看到 "$SITE  has HTTP+parse" (PASS)

# 跑测试 (如果该站写了 _test.go)
go test ./internal/extractor/$SITE/... 2>&1 | tail -5
```

### Step 6: commit (一站一 commit)

```bash
git add internal/extractor/$SITE/
git commit -m "$SITE: STUB → PASS

Source-aligned chain:
- API: <primary URL>
- Parses: <JSON path or HTML regex>
- Returns: <Streams|Entries>
- Source: Mooc/Courses/$SRC_DIR/$SRC_DIR_Course.pyc

Self-review: see internal/extractor/$SITE/SOURCE_ALIGN.md"
```

## 你的具体任务清单

{site_details}

## 防偷懒检查 (每个 commit 前必跑)

```bash
cd $WORKTREE
# 1. STUB 计数必须减少
python3 scripts/verify_full_alignment.py | grep -E "PASS:|STUB:"

# 2. 你的站必须不在 STUB 列表里
python3 scripts/verify_full_alignment.py | grep STUB | grep <your sites>
# 输出空 = 你的站都不是 STUB
```

## 完成后

```bash
git push -u origin {branch}
# 然后报告: "worker-{worker} done, N/M sites PASS, branch pushed"
```

我 (orchestrator) 会从主仓 cherry-pick 你的 commits。
"""

SITE_DETAIL_TEMPLATE = """### {idx}. {name} ({tier})

源码目录: `~/code/xwz-downloader-source-release/decompiled_full/Mooc/Courses/{source_dir}/`
Go 目标文件: `internal/extractor/{name}/{name}.go`
当前状态: STUB (verify_full_alignment.py 报)

预估工作量: {estimate}
关键提示: {hints}
"""


def estimate(tier):
    return {
        "single-api": "30-60 分钟",
        "csslcloud": "60-120 分钟 (调用 shared.CssLcloudResolvePlayInfo)",
        "csslcloud+bokecc": "90-150 分钟 (csslcloud + bokecc 双平台)",
        "polyv": "60-120 分钟 (调用 shared.PolyvResolveSecure)",
        "multi-api": "60-120 分钟",
    }.get(tier, "60 分钟")


def hints(name, tier):
    h = {
        "single-api": "看源码 _get_infos / _get_video_url 主流程, 直接 HTTP+JSON parse 即可",
        "csslcloud": "1) 父站登录拿 LiveRoomID/AccessID/RecordID/ViewerToken; 2) 喂给 shared.CssLcloudResolvePlayInfo; 3) 拿到 m3u8 URL 后用 shared.CssLcloudRewriteM3U8Keys 处理 EXT-X-KEY",
        "csslcloud+bokecc": "csslcloud 流程同上 + bokecc 部分调 shared.BokeCCResolve(vid, siteid) (qihang siteid='A183AC83A2983CCC')",
        "polyv": "1) 拿 vid; 2) shared.PolyvResolveSecure(vid); 3) 取 paths[0] 作 m3u8 URL; 4) Encrypted=true 时返 blocked",
        "multi-api": "course list → chapter tree → leaf detail 多步, 用 *MediaInfo.Entries",
    }
    return h.get(tier, "")


def main():
    with open(DISPATCH) as f:
        workers = json.load(f)
    os.makedirs(OUT_DIR, exist_ok=True)

    for w in workers:
        site_list = ", ".join(s["name"] for s in w["sites"])
        site_details = "\n".join(
            SITE_DETAIL_TEMPLATE.format(
                idx=i + 1,
                name=s["name"],
                tier=s["tier"],
                source_dir=s["source_dir"],
                estimate=estimate(s["tier"]),
                hints=hints(s["name"], s["tier"]),
            )
            for i, s in enumerate(w["sites"])
        )
        prompt = PROMPT_TEMPLATE.format(
            worker=w["worker"],
            worktree=w["worktree"],
            branch=w["branch"],
            site_list=site_list,
            site_details=site_details,
        )
        out = os.path.join(OUT_DIR, f"worker-{w['worker']}.md")
        with open(out, "w") as f:
            f.write(prompt)
        print(f"Wrote {out}")


if __name__ == "__main__":
    main()
