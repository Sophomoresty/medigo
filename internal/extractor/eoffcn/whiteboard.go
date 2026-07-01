package eoffcn

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Sophomoresty/mediago/internal/util"
)

var eoffcnAssetURLRe = regexp.MustCompile(`(?i)(?:https?://|//)?[^\s"'<>\\]+(?:\.wbx|\.wbr|\.wb|\.png|\.jpe?g|\.webp|\.gif|\.bmp|\.svg|\.ttf|\.otf|\.woff2?|\.m3u8|\.mp3|\.m4a|\.aac|\.wav)(?:\?[^\s"'<>\\]*)?`)

type eoffcnBoardAsset struct {
	URL    string
	Source string
	AtMS   int
	Idx    int
}

type eoffcnBoardAssets struct {
	WBX    []map[string]any
	Images []map[string]any
	Font   map[string]any
	Audio  map[string]any
}

func hydrateEoffcnWhiteboardPlayback(c *util.Client, headers map[string]string, title string, playback eoffcnPlayback) eoffcnPlayback {
	if !playback.Whiteboard.Whiteboard {
		return playback
	}
	extra := mergeEoffcnExtra(playback.Extra, eoffcnWhiteboardExtra(playback.Whiteboard))
	apiURL := firstNonEmpty(playback.Whiteboard.APIURL, extraString(extra, "whiteboard_api_url"))
	manifest, fetched, fetchErr := buildEoffcnWhiteboardManifest(c, headers, title, playback, apiURL)
	htmlURL := eoffcnWhiteboardHTMLDataURL(title, manifest)
	extra = mergeEoffcnExtra(extra, map[string]any{
		"whiteboard_manifest":  manifest,
		"whiteboard_html_url":  htmlURL,
		"whiteboard_fetched":   fetched,
		"whiteboard_data_url":  apiURL,
		"whiteboard_html_type": "eoffcn_pcvod",
	})
	if fetchErr != "" {
		extra["whiteboard_fetch_error"] = fetchErr
	}
	playback.Extra = extra
	if playback.URL == "" || playback.URL == apiURL || needsEoffcnBoardReferer(playback.URL) || !isMediaURL(playback.URL) {
		playback.URL = htmlURL
	}
	return playback
}

func buildEoffcnWhiteboardManifest(c *util.Client, headers map[string]string, title string, playback eoffcnPlayback, apiURL string) (map[string]any, bool, string) {
	manifest := map[string]any{
		"site":     "eoffcn",
		"type":     "pcvod_whiteboard",
		"title":    title,
		"renderer": "eoffcn_wboffcn",
		"sdk": map[string]any{
			"wboffcn_js_url":  eoffcnWbOffcnJSURL,
			"wboffcn_mem_url": eoffcnWbOffcnMemURL,
		},
	}
	if apiURL != "" {
		manifest["api_url"] = apiURL
	}
	if len(playback.Whiteboard.Params) > 0 {
		manifest["params"] = playback.Whiteboard.Params
	}
	if playback.URL != "" && isMediaURL(playback.URL) {
		manifest["media_url"] = playback.URL
	}

	var payload any
	var body string
	var fetched bool
	var fetchErr string
	if apiURL != "" && c != nil {
		var err error
		body, err = c.GetString(apiURL, eoffcnWhiteboardHeaders(headers))
		if err != nil {
			fetchErr = err.Error()
		} else {
			fetched = true
			body = strings.TrimSpace(body)
			if body != "" {
				if json.Unmarshal([]byte(body), &payload) == nil {
					manifest["payload"] = payload
				} else {
					manifest["raw_text"] = body
					if strings.Contains(strings.ToLower(body), "<html") {
						manifest["raw_html"] = body
					}
				}
			}
		}
	}
	if payload == nil && body == "" && apiURL != "" {
		payload = map[string]any{"api_url": apiURL}
	}

	assets := collectEoffcnBoardAssets(firstNonNil(payload, body, manifest), apiURL)
	board := map[string]any{}
	if len(assets.WBX) > 0 {
		board["wbx"] = assets.WBX
	}
	if len(assets.Images) > 0 {
		board["images"] = assets.Images
	}
	if len(assets.Font) > 0 {
		board["font"] = assets.Font
	}
	if len(assets.Audio) > 0 {
		manifest["audio"] = assets.Audio
	}
	if len(board) > 0 {
		manifest["eoffcn"] = board
	}
	return manifest, fetched, fetchErr
}

func eoffcnWhiteboardHeaders(headers map[string]string) map[string]string {
	out := cloneStringMap(headers)
	if out == nil {
		out = map[string]string{}
	}
	out["Accept"] = "application/json,text/plain,text/html,*/*"
	out["Referer"] = eoffcnBoardReferer
	out["Origin"] = strings.TrimRight(eoffcnBoardReferer, "/")
	return out
}

func collectEoffcnBoardAssets(value any, baseURL string) eoffcnBoardAssets {
	seenWB := map[string]bool{}
	seenImage := map[string]bool{}
	out := eoffcnBoardAssets{}
	var walk func(any, string, map[string]any)
	walk = func(v any, key string, ctx map[string]any) {
		switch x := v.(type) {
		case map[string]any:
			next := mergeAnyMap(ctx, x)
			for k, child := range x {
				if s := textAnyEoffcn(child); s != "" {
					considerEoffcnBoardAsset(&out, seenWB, seenImage, s, k, next, baseURL)
				}
				walk(child, k, next)
			}
		case []any:
			for _, item := range x {
				walk(item, key, ctx)
			}
		case string:
			s := strings.TrimSpace(x)
			considerEoffcnBoardAsset(&out, seenWB, seenImage, s, key, ctx, baseURL)
			for _, found := range eoffcnAssetURLRe.FindAllString(s, -1) {
				considerEoffcnBoardAsset(&out, seenWB, seenImage, found, key, ctx, baseURL)
			}
			if nested, ok := parseNestedJSON(s); ok {
				walk(nested, key, ctx)
			}
		}
	}
	walk(value, "", nil)
	sort.SliceStable(out.WBX, func(i, j int) bool {
		return intFromAny(out.WBX[i]["atMs"]) < intFromAny(out.WBX[j]["atMs"])
	})
	return out
}

func considerEoffcnBoardAsset(out *eoffcnBoardAssets, seenWB, seenImage map[string]bool, raw, key string, ctx map[string]any, baseURL string) {
	u := normalizeEoffcnAssetURL(raw, baseURL)
	if u == "" {
		return
	}
	lowURL := strings.ToLower(u)
	lowKey := strings.ToLower(key)
	switch {
	case looksEoffcnWBAsset(lowURL, lowKey):
		if seenWB[u] {
			return
		}
		seenWB[u] = true
		out.WBX = append(out.WBX, map[string]any{
			"url":  u,
			"src":  u,
			"atMs": eoffcnContextTimeMS(ctx),
			"idx":  eoffcnContextIndex(ctx, len(out.WBX)),
		})
	case looksEoffcnImageAsset(lowURL, lowKey):
		if seenImage[u] {
			return
		}
		seenImage[u] = true
		out.Images = append(out.Images, map[string]any{
			"url": u,
			"src": u,
			"idx": eoffcnContextIndex(ctx, len(out.Images)),
		})
	case out.Font == nil && looksEoffcnFontAsset(lowURL, lowKey):
		out.Font = map[string]any{"url": u, "src": u}
	case out.Audio == nil && looksEoffcnAudioAsset(lowURL, lowKey):
		out.Audio = map[string]any{"url": u, "src": u}
	}
}

func normalizeEoffcnAssetURL(raw, baseURL string) string {
	if strings.ContainsAny(strings.TrimSpace(raw), "<>\n\r\t ") {
		return ""
	}
	s := normalizeURL(raw)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "data:") {
		return s
	}
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	if baseURL == "" {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	ref, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

func looksEoffcnWBAsset(lowURL, lowKey string) bool {
	ext := strings.ToLower(path.Ext(strings.SplitN(lowURL, "?", 2)[0]))
	if ext == ".wb" || ext == ".wbx" || ext == ".wbr" {
		return true
	}
	return strings.Contains(lowKey, "wbx") || strings.Contains(lowKey, "wbr") || strings.Contains(lowURL, ".wbx") || strings.Contains(lowURL, ".wbr")
}

func looksEoffcnImageAsset(lowURL, lowKey string) bool {
	if strings.Contains(lowKey, "font") || looksEoffcnWBAsset(lowURL, lowKey) {
		return false
	}
	ext := strings.ToLower(path.Ext(strings.SplitN(lowURL, "?", 2)[0]))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".svg":
		return true
	default:
		return strings.Contains(lowKey, "image") || strings.Contains(lowKey, "pic") || strings.Contains(lowKey, "bg")
	}
}

func looksEoffcnFontAsset(lowURL, lowKey string) bool {
	ext := strings.ToLower(path.Ext(strings.SplitN(lowURL, "?", 2)[0]))
	return strings.Contains(lowKey, "font") || ext == ".ttf" || ext == ".otf" || ext == ".woff" || ext == ".woff2"
}

func looksEoffcnAudioAsset(lowURL, lowKey string) bool {
	if !strings.Contains(lowKey, "audio") && !strings.Contains(lowKey, "voice") {
		return false
	}
	ext := strings.ToLower(path.Ext(strings.SplitN(lowURL, "?", 2)[0]))
	return ext == ".m3u8" || ext == ".mp3" || ext == ".m4a" || ext == ".aac" || ext == ".wav"
}

func eoffcnContextTimeMS(ctx map[string]any) int {
	for _, key := range []string{"atMs", "AtMs", "Tm", "tm", "time", "timestamp", "startTime", "start_time"} {
		if n, ok := numberFromAny(ctx[key]); ok {
			lowerKey := strings.ToLower(key)
			if n > 0 && n < 36000 && (strings.Contains(lowerKey, "second") || strings.Contains(lowerKey, "_s")) {
				return int(math.Round(n * 1000))
			}
			return int(math.Round(n))
		}
	}
	return 0
}

func eoffcnContextIndex(ctx map[string]any, fallback int) int {
	for _, key := range []string{"idx", "Idx", "index", "page", "pageIndex", "page_index"} {
		if n, ok := numberFromAny(ctx[key]); ok {
			return int(math.Round(n))
		}
	}
	return fallback
}

func eoffcnWhiteboardHTMLDataURL(title string, manifest map[string]any) string {
	return eoffcnDataURL("text/html", buildEoffcnWhiteboardHTML(title, manifest))
}

func buildEoffcnWhiteboardHTML(title string, manifest map[string]any) string {
	payload, _ := json.Marshal(manifest)
	manifestJSON := strings.ReplaceAll(string(payload), "</script", "<\\/script")
	escapedTitle := html.EscapeString(firstNonEmpty(title, "Eoffcn Board"))
	return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + escapedTitle + `</title><style>
body{margin:0;background:#eef1f6;color:#1b2430;font-family:"Microsoft YaHei",Arial,sans-serif}.app{min-height:100vh;display:flex;flex-direction:column;gap:12px;padding:14px}.panel{background:#fff;border:1px solid #d9e0ea;border-radius:14px;box-shadow:0 12px 34px rgba(26,36,48,.10);padding:12px}.stage{position:relative;flex:1;min-height:360px;display:flex;align-items:center;justify-content:center}.canvas-wrap{width:min(96vw,1280px);aspect-ratio:16/9;background:#fff;border:1px solid #d9e0ea;border-radius:12px;overflow:hidden;position:relative}canvas,#fallback{position:absolute;inset:0;width:100%;height:100%;background:#fff}#fallback{display:flex;align-items:center;justify-content:center;text-align:center;padding:18px;box-sizing:border-box;color:#667487}.row{display:flex;gap:10px;align-items:center;flex-wrap:wrap}button{padding:6px 12px;border:1px solid #ccd4e0;border-radius:8px;background:#f8fafc}input[type=range]{flex:1;min-width:220px}pre{white-space:pre-wrap;word-break:break-word;max-height:220px;overflow:auto;background:#f8fafc;padding:10px;border-radius:10px}
</style><script>window.__EOFFCN_BOARD_MANIFEST__=` + manifestJSON + `;</script><script src="` + html.EscapeString(eoffcnWbOffcnJSURL) + `"></script></head><body><div class="app"><div class="panel"><h2 id="title">` + escapedTitle + `</h2><div id="meta"></div></div><div class="stage"><div class="canvas-wrap"><canvas id="zg-white-board"></canvas><div id="fallback">正在加载白板...</div></div></div><div class="panel"><div class="row"><button id="prev">上一页</button><input id="range" type="range" min="0" max="0" value="0"><button id="next">下一页</button><span id="page"></span></div><audio id="audio" controls style="width:100%;margin-top:8px;display:none"></audio><pre id="raw"></pre></div></div><script>
(function(){const manifest=window.__EOFFCN_BOARD_MANIFEST__||{};const board=manifest.eoffcn||{};const entries=Array.isArray(board.wbx)?board.wbx:[];const images=Array.isArray(board.images)?board.images:[];const canvas=document.getElementById('zg-white-board');const fallback=document.getElementById('fallback');const range=document.getElementById('range');const page=document.getElementById('page');const raw=document.getElementById('raw');const audio=document.getElementById('audio');const meta=document.getElementById('meta');let module=null,handle=null,surface=null,current=-1;range.max=String(Math.max(0,entries.length-1));meta.textContent=[manifest.api_url?'API: '+manifest.api_url:'',manifest.fetched===false?'未预取, 将使用嵌入 manifest/远程 URL 渲染':''].filter(Boolean).join(' | ');if(manifest.audio&&manifest.audio.src){audio.src=manifest.audio.src;audio.style.display='block'}function showRaw(){raw.textContent=JSON.stringify(manifest,null,2)}function resize(){canvas.width=1280;canvas.height=720}async function fetchBuf(u){const r=await fetch(u,{credentials:'include'});if(!r.ok)throw new Error('fetch '+r.status+' '+u);return await r.arrayBuffer()}function put(buf){const bytes=new Uint8Array(buf);const ptr=module._malloc(bytes.length);new Uint8Array(module.HEAP8.buffer,ptr,bytes.length).set(bytes);return{ptr:ptr,len:bytes.length}}async function init(){resize();if(module||!window.WbOffcnInit)return;module=await WbOffcnInit();module.MakeCanvasSurface('zg-white-board');surface=module.zg_wb_create_surface(canvas.width,canvas.height);handle=module.zg_wb_create()}function drawImageFor(i){const item=images[i]||images.find(x=>Number(x.idx)===Number(entries[i]&&entries[i].idx));if(!item||!item.src)return;const img=new Image();img.crossOrigin='anonymous';img.onload=function(){const ctx=canvas.getContext('2d');ctx.drawImage(img,0,0,canvas.width,canvas.height)};img.src=item.src}async function draw(i){if(!entries.length){fallback.textContent='没有发现可直接渲染的 wbx/wbr 条目, 已保留原始 manifest.';showRaw();return}i=Math.max(0,Math.min(entries.length-1,i));current=i;range.value=String(i);page.textContent='页码: '+(i+1)+' / '+entries.length;fallback.style.display='none';drawImageFor(i);try{await init();if(!module||!handle)throw new Error('wboffcn runtime unavailable');const item=entries[i];const buf=await fetchBuf(item.src||item.url);module.zg_wb_clear(handle,false);const data=put(buf);try{module.zg_wb_draw_wb_data(handle,data.ptr,data.len,Number(item.idx)||0)}finally{module._free(data.ptr)}if(surface&&surface.flush)surface.flush()}catch(e){fallback.style.display='flex';fallback.textContent='白板 SDK 渲染失败: '+e.message;showRaw()}}function entryAtMs(i){const item=entries[i]||{};return Number(item.atMs||item.at||item.time||item.Tm)||0}function durationMs(){let max=0;for(let i=0;i<entries.length;i++)max=Math.max(max,entryAtMs(i));return Math.max(max,Number(manifest.durationMs||manifest.duration_ms)||0)}function indexForMs(value){value=Number(value)||0;let idx=0;for(let i=0;i<entries.length;i++){if(entryAtMs(i)<=value)idx=i;else break}return idx}async function saveFrameImage(filePath,imageType,quality){if(current<0)await draw(0);const data=canvas.toDataURL(imageType||'image/png',quality||0.92);if(filePath&&window.require){try{const fs=window.require('fs');const b64=data.split(',')[1]||'';fs.writeFileSync(filePath,Buffer.from(b64,'base64'));return true}catch(e){return data}}return data}async function saveFrameSequence(specs,imageType,quality){const out=[];const list=Array.isArray(specs)?specs:[];for(const spec of list){const target=typeof spec==='number'?spec:Number(spec&&(spec.time_ms||spec.timeMs||spec.ms||spec.time))||0;await draw(indexForMs(target));out.push({time_ms:target,result:await saveFrameImage(spec&&spec.path,imageType,quality)})}return out}window.__MEDIGO_WHITEBOARD_PLAYER__=window.__PLASO_OFFLINE_RENDERER__={preload:init,seekToMs:async function(value){await draw(indexForMs(value));return true},getDurationMs:durationMs,saveFrameImage:saveFrameImage,saveFrameSequence:saveFrameSequence};document.getElementById('prev').onclick=function(){draw(current-1)};document.getElementById('next').onclick=function(){draw(current+1)};range.oninput=function(){draw(Number(range.value)||0)};showRaw();draw(0);})();
</script></body></html>`
}

func eoffcnDataURL(mime, content string) string {
	return "data:" + mime + ";charset=utf-8," + url.PathEscape(content)
}

func mergeAnyMap(a, b map[string]any) map[string]any {
	if len(a) == 0 {
		return b
	}
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
				continue
			}
			return value
		}
	}
	return nil
}

func extraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	return strings.TrimSpace(textAnyEoffcn(extra[key]))
}

func textAnyEoffcn(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "<nil>" {
			return ""
		}
		return s
	}
}

func intFromAny(value any) int {
	n, _ := numberFromAny(value)
	return int(math.Round(n))
}

func numberFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return n, err == nil
	default:
		return 0, false
	}
}
