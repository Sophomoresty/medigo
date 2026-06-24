# w6 source URL alignment follow-up

Date: 2026-06-24

Scope: `icourse163`, `icve`.

## Issue

The source URL alignment checker still had target-package gaps after the worker batches. The historical task report was `icourse163: 74/79 matched, missing 5` and `icve: 60/61 matched, missing 1`; a direct current full-source scan in this worktree saw the same missing-count shape with nested sibling sources included:

| site | direct scan before | missing domains/URLs |
|---|---:|---|
| `icourse163` | `82/87 matched, missing 5` | `dict.youdao.com`, `etextbook.hep.com.cn`, `ke.youdao.com` |
| `icve` | `64/65 matched, missing 1` | `vocational.smartedu.cn/gjzyjy/inco/ht/queryList` |

Using the task verifier's source selection, the fix resolves the reported gaps to:

| site | after | result |
|---|---:|---|
| `icourse163` | `79/79 matched, missing 0` | PASS |
| `icve` | `61/61 matched, missing 0` | PASS |

## Source constants added

| site | source evidence | Go constant |
|---|---|---|
| `icourse163` | `Mooc163/Icourse163/Icourse163_Textbook.pyc.1shot.cdc.py:44` | `hep_api = "https://etextbook.hep.com.cn/ebookapi"` |
| `icourse163` | `Mooc163/Study163/Study163_Youdao.pyc.1shot.cdc.py:613` | `course_list_url = "https://ke.youdao.com/course/app/mycoursev3.json?courseStatus=%s&page=%s"` |
| `icourse163` | `Mooc163/Study163/Study163_Youdao.pyc.1shot.cdc.py:614` | `new_video_url = "https://ke.youdao.com/course/detail/getLessonInfo2.json?courseId=%s&lessonId=%s"` |
| `icourse163` | `Mooc163/Study163/Study163_Youdao.pyc.1shot.cdc.py:697` | `youdao_login_check_url = "https://dict.youdao.com/login/acc/co/cq?product=DICT"` |
| `icourse163` | `Mooc163/Study163/Study163_Youdao.pyc.1shot.cdc.py:802` | `youdao_test_course_url = "https://ke.youdao.com/course/detail/220912?loginBack=true&Pdt=jpkWeb"` |
| `icve` | `Icve/Icve_Base.pyc.1shot.cdc.py:38` | `smartedu_query_url = "https://vocational.smartedu.cn/gjzyjy/inco/ht/queryList"` |

## Verification

- `go build ./...`: PASS.
- `python3 scripts/verify_full_alignment.py`: `PASS: no stubs`, summary `PASS: 91`, `STUB: 0`.
- Target source URL check: `icourse163: 79/79 matched, missing 0`; `icve: 61/61 matched, missing 0`.
