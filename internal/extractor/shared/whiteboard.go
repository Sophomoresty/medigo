package shared

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// WhiteboardTimeline is a provider-neutral board replay model.
type WhiteboardTimeline struct {
	Provider   string               `json:"provider,omitempty"`
	Title      string               `json:"title,omitempty"`
	Width      int                  `json:"width,omitempty"`
	Height     int                  `json:"height,omitempty"`
	DurationMS int64                `json:"duration_ms,omitempty"`
	Pages      []WhiteboardPage     `json:"pages,omitempty"`
	Events     []WhiteboardEvent    `json:"events,omitempty"`
	Resources  []WhiteboardResource `json:"resources,omitempty"`
	RawKind    string               `json:"raw_kind,omitempty"`
}

type WhiteboardPage struct {
	ID        string            `json:"id,omitempty"`
	Title     string            `json:"title,omitempty"`
	StartMS   int64             `json:"start_ms,omitempty"`
	EndMS     int64             `json:"end_ms,omitempty"`
	ImageURL  string            `json:"image_url,omitempty"`
	BoardURL  string            `json:"board_url,omitempty"`
	BoardID   string            `json:"board_id,omitempty"`
	ImageID   string            `json:"image_id,omitempty"`
	X         float64           `json:"x,omitempty"`
	Y         float64           `json:"y,omitempty"`
	Width     float64           `json:"width,omitempty"`
	Height    float64           `json:"height,omitempty"`
	ImageRect map[string]any    `json:"image_rect,omitempty"`
	BoardRect map[string]any    `json:"board_rect,omitempty"`
	Events    []WhiteboardEvent `json:"events,omitempty"`
}

type WhiteboardResource struct {
	ID   string `json:"id,omitempty"`
	URL  string `json:"url,omitempty"`
	Type string `json:"type,omitempty"`
}

type WhiteboardEvent struct {
	TimeMS       int64             `json:"time_ms,omitempty"`
	StartMS      int64             `json:"start_ms,omitempty"`
	EndMS        int64             `json:"end_ms,omitempty"`
	EraseMS      int64             `json:"erase_ms,omitempty"`
	Type         string            `json:"type,omitempty"`
	Kind         string            `json:"kind,omitempty"`
	Page         string            `json:"page,omitempty"`
	Color        string            `json:"color,omitempty"`
	Width        float64           `json:"width,omitempty"`
	Text         string            `json:"text,omitempty"`
	Points       []WhiteboardPoint `json:"points,omitempty"`
	CanvasWidth  float64           `json:"canvas_width,omitempty"`
	CanvasHeight float64           `json:"canvas_height,omitempty"`
	Raw          map[string]any    `json:"raw,omitempty"`
}

type WhiteboardPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// HTMLDataURL returns a base64 data URL suitable for extractor.Stream.URLs.
func HTMLDataURL(doc string) string {
	return "data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(doc))
}

// ParseGenericWhiteboardJSON extracts a best-effort timeline from common whiteboard JSON layouts.
func ParseGenericWhiteboardJSON(data []byte, provider, title string) (WhiteboardTimeline, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return WhiteboardTimeline{}, err
	}
	return ParseGenericWhiteboard(root, provider, title), nil
}

// ParseGenericWhiteboard extracts events/pages from arbitrary decoded JSON.
func ParseGenericWhiteboard(root any, provider, title string) WhiteboardTimeline {
	t := WhiteboardTimeline{Provider: provider, Title: title, RawKind: "json"}
	seenEvents := map[string]bool{}
	seenPages := map[string]bool{}
	var walk func(any, string, int)
	walk = func(v any, pageHint string, depth int) {
		if v == nil || depth > 10 {
			return
		}
		switch x := v.(type) {
		case map[string]any:
			if t.Width == 0 {
				t.Width = int(firstNumber(x, 0, "width", "canvasWidth", "canvaswidth", "w"))
			}
			if t.Height == 0 {
				t.Height = int(firstNumber(x, 0, "height", "canvasHeight", "canvasheight", "h"))
			}
			if d := parseTimeMS(firstValue(x, "duration", "durationMs", "duration_ms", "totalTime", "mediaTotalTime", "endTime")); d > t.DurationMS {
				t.DurationMS = d
			}
			if page, ok := pageFromMap(x, pageHint); ok {
				key := firstNonEmpty(page.ID, fmt.Sprintf("%d:%d:%s:%s", page.StartMS, page.EndMS, page.ImageURL, page.BoardURL))
				if !seenPages[key] {
					seenPages[key] = true
					t.Pages = append(t.Pages, page)
				}
				if page.ID != "" {
					pageHint = page.ID
				}
			}
			if ev, ok := eventFromMap(x, pageHint); ok {
				key := eventKey(ev)
				if !seenEvents[key] {
					seenEvents[key] = true
					t.Events = append(t.Events, ev)
				}
			}
			for _, key := range []string{"pages", "pageList", "normalPages", "normalPage", "slides", "timeline"} {
				if nested, ok := x[key]; ok {
					walk(nested, pageHint, depth+1)
				}
			}
			for _, key := range []string{"whiteBoardPen", "whiteboardPen", "white_board_pen", "pen", "events", "eventList", "records", "record", "actions", "ops", "items", "list", "data", "payload", "result", "courseware", "contentInfo"} {
				if nested, ok := x[key]; ok {
					walk(nested, pageHint, depth+1)
				}
			}
			for _, nested := range x {
				switch nested.(type) {
				case map[string]any, []any:
					walk(nested, pageHint, depth+1)
				}
			}
		case []any:
			for _, item := range x {
				walk(item, pageHint, depth+1)
			}
		case string:
			if nested, ok := decodeNestedJSON(x); ok {
				walk(nested, pageHint, depth+1)
			}
		}
	}
	walk(root, "", 0)
	NormalizeWhiteboardTimeline(&t)
	return t
}

// ParseCCTalkWhiteboardXML extracts courseware pages/resources from CCtalk/Mddclass OCS XML content.
func ParseCCTalkWhiteboardXML(content []byte, title string) (WhiteboardTimeline, error) {
	content = bytes.TrimSpace(content)
	if len(content) == 0 {
		return WhiteboardTimeline{}, fmt.Errorf("empty whiteboard xml")
	}
	dec := xml.NewDecoder(bytes.NewReader(content))
	t := WhiteboardTimeline{Provider: "cctalk", Title: title, Width: 1280, Height: 720, RawKind: "xml"}
	resources := map[string]WhiteboardResource{}
	var currentPage *WhiteboardPage
	var currentElement string
	var currentElementAttrs map[string]string
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			if strings.Contains(err.Error(), "EOF") {
				break
			}
			return WhiteboardTimeline{}, err
		}
		switch e := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(e.Name.Local)
			attrs := xmlAttrs(e.Attr)
			switch name {
			case "courseware":
				if t.Title == "" {
					t.Title = firstMapValue(attrs, "name", "title")
				}
				if d := parseTimeMS(firstMapValue(attrs, "totalTime", "duration", "durationMs")); d > t.DurationMS {
					t.DurationMS = d
				}
			case "style":
				if currentElement != "" && currentElementAttrs != nil {
					for k, v := range attrs {
						if _, ok := currentElementAttrs[k]; !ok {
							currentElementAttrs[k] = v
						}
					}
				} else if currentPage == nil {
					if w := int(parseFloat(firstMapValue(attrs, "width", "w"))); w > 0 {
						t.Width = w
					}
					if h := int(parseFloat(firstMapValue(attrs, "height", "h"))); h > 0 {
						t.Height = h
					}
				}
			case "res":
				id := firstMapValue(attrs, "id", "resid", "resourceId", "resId")
				u := firstMapValue(attrs, "url", "src", "path", "m3u8", "href", "filePath", "objectKey", "key")
				if id != "" || u != "" {
					res := WhiteboardResource{ID: id, URL: u, Type: strings.ToLower(firstMapValue(attrs, "type", "mediaType", "extension", "ext"))}
					if id != "" {
						resources[id] = res
					}
					t.Resources = append(t.Resources, res)
				}
			case "normalpage", "page":
				p := WhiteboardPage{ID: firstNonEmpty(firstMapValue(attrs, "id", "number", "page", "pageId"), fmt.Sprintf("page_%d", len(t.Pages)+1)), Title: firstMapValue(attrs, "name", "title"), StartMS: parseTimeMS(firstMapValue(attrs, "startTime", "start", "beginTime")), EndMS: parseTimeMS(firstMapValue(attrs, "endTime", "end", "stopTime"))}
				currentPage = &p
			case "image", "whiteboard":
				if currentPage != nil {
					currentElement = name
					currentElementAttrs = attrs
				}
			}
		case xml.EndElement:
			name := strings.ToLower(e.Name.Local)
			if (name == "image" || name == "whiteboard") && currentPage != nil && currentElement == name {
				applyCCTalkPageElement(currentPage, currentElement, currentElementAttrs, resources)
				currentElement, currentElementAttrs = "", nil
			}
			if (name == "normalpage" || name == "page") && currentPage != nil {
				p := *currentPage
				if p.EndMS <= p.StartMS {
					p.EndMS = p.StartMS + 1000
				}
				t.Pages = append(t.Pages, p)
				currentPage = nil
			}
		}
	}
	NormalizeWhiteboardTimeline(&t)
	return t, nil
}

func NormalizeWhiteboardTimeline(t *WhiteboardTimeline) {
	if t == nil {
		return
	}
	if t.Width <= 0 {
		t.Width = 1280
	}
	if t.Height <= 0 {
		t.Height = 720
	}
	if len(t.Pages) == 0 {
		t.Pages = []WhiteboardPage{{ID: "page_1", StartMS: 0}}
	}
	var pageEvents []WhiteboardEvent
	for i := range t.Pages {
		if t.Pages[i].ID == "" {
			t.Pages[i].ID = fmt.Sprintf("page_%d", i+1)
		}
		if len(t.Pages[i].Events) > 0 {
			for _, ev := range t.Pages[i].Events {
				if ev.Page == "" {
					ev.Page = t.Pages[i].ID
				}
				pageEvents = append(pageEvents, ev)
			}
			t.Pages[i].Events = nil
		}
		if t.Pages[i].ImageRect == nil {
			t.Pages[i].ImageRect = pageRectMap(t.Pages[i], t.Width, t.Height)
		}
		if t.Pages[i].BoardRect == nil {
			t.Pages[i].BoardRect = pageRectMap(t.Pages[i], t.Width, t.Height)
		}
		if t.Pages[i].EndMS > t.DurationMS {
			t.DurationMS = t.Pages[i].EndMS
		}
	}
	if len(pageEvents) > 0 {
		seen := make(map[string]bool, len(t.Events)+len(pageEvents))
		for _, ev := range t.Events {
			seen[eventKey(ev)] = true
		}
		for _, ev := range pageEvents {
			key := eventKey(ev)
			if seen[key] {
				continue
			}
			seen[key] = true
			t.Events = append(t.Events, ev)
		}
	}
	sort.SliceStable(t.Pages, func(i, j int) bool {
		if t.Pages[i].StartMS == t.Pages[j].StartMS {
			return t.Pages[i].ID < t.Pages[j].ID
		}
		return t.Pages[i].StartMS < t.Pages[j].StartMS
	})
	defaultPage := t.Pages[0].ID
	for i := range t.Events {
		if t.Events[i].Page == "" {
			t.Events[i].Page = pageAt(t.Pages, eventStartMS(t.Events[i]), defaultPage)
		}
		if t.Events[i].Type == "" {
			t.Events[i].Type = firstNonEmpty(t.Events[i].Kind, "path")
		}
		if t.Events[i].Kind == "" {
			t.Events[i].Kind = t.Events[i].Type
		}
		if t.Events[i].Color == "" {
			t.Events[i].Color = "#111111"
		}
		if t.Events[i].Width <= 0 {
			t.Events[i].Width = 2
		}
		start := eventStartMS(t.Events[i])
		if start > t.DurationMS {
			t.DurationMS = start
		}
		if t.Events[i].EndMS > t.DurationMS {
			t.DurationMS = t.Events[i].EndMS
		}
		if t.Events[i].EraseMS > t.DurationMS {
			t.DurationMS = t.Events[i].EraseMS
		}
	}
	sort.SliceStable(t.Events, func(i, j int) bool { return eventStartMS(t.Events[i]) < eventStartMS(t.Events[j]) })
	if t.DurationMS <= 0 {
		t.DurationMS = 1000
	}
	for i := range t.Pages {
		if t.Pages[i].EndMS <= t.Pages[i].StartMS {
			if i+1 < len(t.Pages) && t.Pages[i+1].StartMS > t.Pages[i].StartMS {
				t.Pages[i].EndMS = t.Pages[i+1].StartMS
			} else {
				t.Pages[i].EndMS = t.DurationMS
			}
		}
	}
	for _, ev := range t.Events {
		pageID := ev.Page
		for i := range t.Pages {
			if t.Pages[i].ID == pageID {
				t.Pages[i].Events = append(t.Pages[i].Events, ev)
				break
			}
		}
	}
}

func pageRectMap(p WhiteboardPage, width, height int) map[string]any {
	w, h := p.Width, p.Height
	if w <= 0 {
		w = float64(width)
	}
	if h <= 0 {
		h = float64(height)
	}
	return map[string]any{"x": p.X, "y": p.Y, "width": w, "height": h}
}

func eventStartMS(ev WhiteboardEvent) int64 {
	if ev.StartMS > 0 {
		return ev.StartMS
	}
	return ev.TimeMS
}

// WhiteboardPlayableHTML returns a self-contained browser player.
func WhiteboardPlayableHTML(t WhiteboardTimeline) string {
	NormalizeWhiteboardTimeline(&t)
	payload, _ := json.Marshal(t)
	title := html.EscapeString(firstNonEmpty(t.Title, "Whiteboard"))
	return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + title + `</title><style>:root{--bg:#edf1f7;--line:#d9e0ea;--text:#172033;--muted:#667487;--primary:#315cff}*{box-sizing:border-box}body{margin:0;background:var(--bg);font-family:"Microsoft YaHei",Arial,sans-serif;color:var(--text)}.app{min-height:100vh;padding:16px;display:flex;flex-direction:column;gap:12px}.bar,.controls{background:rgba(255,255,255,.94);border:1px solid var(--line);border-radius:14px;padding:12px 14px;box-shadow:0 14px 36px rgba(20,30,45,.10)}.title{font-weight:700;word-break:break-all}.meta{font-size:12px;color:var(--muted);margin-top:4px}.stage-wrap{flex:1;display:flex;align-items:center;justify-content:center;min-height:220px}canvas{max-width:100%;max-height:72vh;background:#fff;border:1px solid var(--line);border-radius:12px;box-shadow:0 14px 36px rgba(20,30,45,.12)}.controls{display:grid;grid-template-columns:auto 1fr auto;gap:10px;align-items:center}button{border:0;background:var(--primary);color:white;border-radius:9px;padding:8px 14px;cursor:pointer}input[type=range]{width:100%}.hint{font-size:12px;color:var(--muted);grid-column:1/-1}</style></head><body><div class="app"><div class="bar"><div class="title" id="title"></div><div class="meta" id="meta"></div></div><div class="stage-wrap"><canvas id="stage"></canvas></div><div class="controls"><button id="play">播放</button><input id="seek" type="range" min="0" max="1000" value="0"><span id="clock">00:00 / 00:00</span><div class="hint">白板 HTML 播放器: 支持页面图片, 线条, 橡皮和文本. 页面 board_url 将在浏览器中延迟加载.</div></div></div><script>
const model=` + string(payload) + `;
const canvas=document.getElementById('stage'),ctx=canvas.getContext('2d'),seek=document.getElementById('seek'),playBtn=document.getElementById('play'),clock=document.getElementById('clock'),meta=document.getElementById('meta');
const state={images:new Map(),loadedBoards:new Set(),playing:false,lastTick:0,currentMs:0};
function ms(v){v=Math.round(Number(v)||0);return v<0?0:v}function fmt(v){let n=Math.floor(ms(v)/1000),h=Math.floor(n/3600);n-=h*3600;let m=Math.floor(n/60),s=n-m*60;return (h?String(h).padStart(2,'0')+':':'')+String(m).padStart(2,'0')+':'+String(s).padStart(2,'0')}
function resize(){canvas.width=Math.max(2,Number(model.width)||1280);canvas.height=Math.max(2,Number(model.height)||720)}
function pages(){return Array.isArray(model.pages)?model.pages:[]}function pageAtMs(t){let cur=pages()[0]||{};for(const p of pages()){if(ms(p.start_ms)<=t)cur=p;else break}return cur}
function parseTime(v){if(v&&typeof v==='object')return ms(v.start_ms||v.time_ms||v.drawtime||v.drawTime||v.time||v.startTime);return ms(v)}
function parsePoints(v){if(!v)return[];if(typeof v==='string'){try{return parsePoints(JSON.parse(v))}catch(e){}const nums=(v.match(/-?\d+(?:\.\d+)?/g)||[]).map(Number);let out=[];for(let i=0;i+1<nums.length;i+=2)out.push({x:nums[i],y:nums[i+1]});return out}if(Array.isArray(v)){let out=[];for(const it of v){if(Array.isArray(it)&&it.length>=2)out.push({x:Number(it[0])||0,y:Number(it[1])||0});else if(it&&typeof it==='object')out.push({x:Number(it.x??it.left)||0,y:Number(it.y??it.top)||0})}return out}return[]}
function eventKind(e){const s=String(e.kind||e.type||e.eventType||e.action||e.tool||'pen').toLowerCase();if(s.includes('eras'))return'eraser';if(s.includes('text'))return'text';if(s.includes('clear'))return'clear';return'pen'}
function collectEvents(obj,page){let out=[];function walk(v){if(!v)return;if(Array.isArray(v)){for(const x of v)walk(x);return}if(typeof v==='string'){try{walk(JSON.parse(v))}catch(e){}return}if(typeof v!=='object')return;const arr=v.whiteBoardPen||v.whiteboardPen||v.pen||v.events||v.eventList||v.drawEvents;if(Array.isArray(arr))for(const item of arr)walk(item);const pts=parsePoints(v.points||v.point||v.path||v.pts);const text=String(v.text||v.content||v.value||'');if(pts.length||text){out.push({start_ms:parseTime(v),erase_ms:ms(v.eraseTime||v.erasetime||v.erase),kind:eventKind(v),page:page||v.page||v.pageId||'',text,points:pts,color:v.color||v.strokeColor||v.penColor||'#000000',width:Number(v.pen||v.brushSize||v.size||v.lineWidth||v.strokeWidth)||2,canvas_width:Number(v.canvasWidth||v.canvaswidth)||0,canvas_height:Number(v.canvasHeight||v.canvasheight)||0})}for(const k of ['data','payload','result','items','list'])if(v[k])walk(v[k])}walk(obj);return out.sort((a,b)=>ms(a.start_ms)-ms(b.start_ms))}
async function loadImage(src){if(!src)return null;if(state.images.has(src))return state.images.get(src);const p=new Promise(resolve=>{const img=new Image();img.crossOrigin='anonymous';img.onload=()=>resolve(img);img.onerror=()=>resolve(null);img.src=src});state.images.set(src,p);return p}
async function ensurePage(page){if(!page)return;if(page.image_url)await loadImage(page.image_url);if(page.board_url&&!state.loadedBoards.has(page.id||page.board_url)){state.loadedBoards.add(page.id||page.board_url);try{const r=await fetch(page.board_url);if(r.ok){const j=await r.json();page.events=(page.events||[]).concat(collectEvents(j,page.id)).sort((a,b)=>ms(a.start_ms)-ms(b.start_ms));}}catch(e){}}}
function rect(r){return r||{x:0,y:0,width:canvas.width,height:canvas.height}}function drawImageContain(img,r){if(!img)return;r=rect(r);const iw=img.naturalWidth||img.width,ih=img.naturalHeight||img.height;if(!iw||!ih)return;const s=Math.min((r.width||canvas.width)/iw,(r.height||canvas.height)/ih),w=iw*s,h=ih*s;ctx.drawImage(img,(r.x||0)+((r.width||canvas.width)-w)/2,(r.y||0)+((r.height||canvas.height)-h)/2,w,h)}
function scalePoint(p,e,page){const br=rect(page.board_rect),cw=Number(e.canvas_width)||0,ch=Number(e.canvas_height)||0;let sx=1,sy=1;if(cw>0&&ch>0){sx=(br.width||canvas.width)/cw;sy=(br.height||canvas.height)/ch}else{let maxX=Math.max(1,...(page.events||[]).flatMap(ev=>(ev.points||[]).map(p=>Number(p.x)||0)));let maxY=Math.max(1,...(page.events||[]).flatMap(ev=>(ev.points||[]).map(p=>Number(p.y)||0)));if(maxX<=650&&maxY<=510){maxX=630;maxY=495}sx=(br.width||canvas.width)/maxX;sy=(br.height||canvas.height)/maxY}return{x:(br.x||0)+(Number(p.x)||0)*sx,y:(br.y||0)+(Number(p.y)||0)*sy}}
function drawPath(e,page,kind){const pts=(e.points||[]).map(p=>scalePoint(p,e,page));if(!pts.length)return;ctx.save();ctx.lineCap='round';ctx.lineJoin='round';ctx.strokeStyle=kind==='eraser'?'#fff':(e.color||'#000');ctx.lineWidth=Math.max(1,Number(e.width)||2)*(kind==='eraser'?4:1);ctx.beginPath();ctx.moveTo(pts[0].x,pts[0].y);for(let i=1;i<pts.length;i++)ctx.lineTo(pts[i].x,pts[i].y);ctx.stroke();ctx.restore()}
function drawText(e,page){const pts=(e.points||[]).map(p=>scalePoint(p,e,page));const p=pts[0]||{x:0,y:0};ctx.save();ctx.fillStyle=e.color||'#000';ctx.font=Math.max(12,(Number(e.width)||16)*3)+'px sans-serif';ctx.fillText(String(e.text||''),p.x,p.y);ctx.restore()}
async function drawFrame(t){state.currentMs=ms(t);seek.value=String(Math.round(state.currentMs/Math.max(1,model.duration_ms||1)*1000));clock.textContent=fmt(state.currentMs)+' / '+fmt(model.duration_ms);const page=pageAtMs(state.currentMs)||{};await ensurePage(page);ctx.fillStyle='#fff';ctx.fillRect(0,0,canvas.width,canvas.height);if(page.image_url){drawImageContain(await loadImage(page.image_url),page.image_rect)}const events=(page.events||[]).slice().sort((a,b)=>ms(a.start_ms||a.time_ms)-ms(b.start_ms||b.time_ms));for(const e of events){const st=ms(e.start_ms||e.time_ms);if(st>state.currentMs)break;if(ms(e.erase_ms||e.end_ms)>0&&ms(e.erase_ms||e.end_ms)<=state.currentMs)continue;const k=eventKind(e);if(k==='clear'){ctx.fillStyle='#fff';ctx.fillRect(0,0,canvas.width,canvas.height);continue}if(k==='text')drawText(e,page);else drawPath(e,page,k)}}
function tick(ts){if(!state.playing)return;if(!state.lastTick)state.lastTick=ts;const dt=ts-state.lastTick;state.lastTick=ts;let next=state.currentMs+dt;if(next>ms(model.duration_ms)){next=ms(model.duration_ms);state.playing=false;playBtn.textContent='播放'}drawFrame(next);requestAnimationFrame(tick)}
playBtn.onclick=()=>{state.playing=!state.playing;playBtn.textContent=state.playing?'暂停':'播放';state.lastTick=0;if(state.playing)requestAnimationFrame(tick)};seek.oninput=()=>drawFrame(Number(seek.value)/1000*ms(model.duration_ms));
window.__MEDIGO_WHITEBOARD_PLAYER__={seekToMs:drawFrame,getDurationMs:()=>ms(model.duration_ms),saveFrameImage:function(type,quality){return canvas.toDataURL(type||'image/png',quality||0.92)}};
document.getElementById('title').textContent=model.title||'Whiteboard';meta.textContent=` + "`" + `${model.provider||model.raw_kind||'whiteboard'} | ${model.width||canvas.width}x${model.height||canvas.height} | pages: ${pages().length} | events: ${(model.events||[]).length}` + "`" + `;resize();drawFrame(0);
</script></body></html>`
}

func pageAt(pages []WhiteboardPage, t int64, fallback string) string {
	out := fallback
	for _, p := range pages {
		if p.StartMS <= t {
			out = p.ID
		} else {
			break
		}
	}
	return out
}

func eventFromMap(m map[string]any, pageHint string) (WhiteboardEvent, bool) {
	points := parsePoints(firstValue(m, "points", "point", "path", "data", "p", "line"))
	text := firstText(m, "text", "content", "value", "label")
	typeName := normalizeEventType(firstText(m, "type", "eventType", "event_type", "action", "shape", "cmd", "method", "name", "drawType", "penType", "kind", "tool"))
	if len(points) == 0 && strings.TrimSpace(text) == "" && typeName != "clear" && typeName != "page" {
		return WhiteboardEvent{}, false
	}
	start := parseTimeMS(firstValue(m, "start_ms", "startMS", "time_ms", "timeMS", "atMs", "at", "drawtime", "drawTime", "time", "timestamp", "ts", "startTime", "start_time", "seek", "pos"))
	ev := WhiteboardEvent{TimeMS: start, StartMS: start, EndMS: parseTimeMS(firstValue(m, "end_ms", "endMS", "endTime", "end_time")), EraseMS: parseTimeMS(firstValue(m, "erase_ms", "eraseMS", "eraseTime", "erasetime", "erase")), Type: typeName, Kind: typeName, Page: firstNonEmpty(firstText(m, "page", "pageId", "pageID", "page_id", "number"), pageHint), Color: normalizeColor(firstValue(m, "color", "stroke", "strokeStyle", "stroke_style", "strokeColor", "penColor", "c")), Width: firstNumber(m, 0, "width", "lineWidth", "line_width", "strokeWidth", "stroke_width", "pen", "brushSize", "brushsize", "thickness", "size", "font"), Text: text, Points: points, CanvasWidth: firstNumber(m, 0, "canvasWidth", "canvaswidth"), CanvasHeight: firstNumber(m, 0, "canvasHeight", "canvasheight"), Raw: compactRaw(m)}
	return ev, true
}

func pageFromMap(m map[string]any, pageHint string) (WhiteboardPage, bool) {
	imageURL := firstURLText(m, "image_url", "imageUrl", "imageURL", "img", "image", "pic", "picUrl", "background", "backgroundUrl")
	boardURL := firstURLText(m, "board_url", "boardUrl", "whiteboard_url", "whiteboardUrl", "wb", "whiteboard", "board", "url")
	if imageURL == "" && boardURL == "" && firstText(m, "page", "pageId", "number", "id") == "" {
		return WhiteboardPage{}, false
	}
	p := WhiteboardPage{ID: firstNonEmpty(firstText(m, "page", "pageId", "pageID", "page_id", "number", "id"), pageHint), Title: firstText(m, "title", "name"), StartMS: parseTimeMS(firstValue(m, "start_ms", "startMS", "startTime", "start", "beginTime")), EndMS: parseTimeMS(firstValue(m, "end_ms", "endMS", "endTime", "end", "stopTime")), ImageURL: imageURL, BoardURL: boardURL, X: firstNumber(m, 0, "x", "left"), Y: firstNumber(m, 0, "y", "top"), Width: firstNumber(m, 0, "width", "w"), Height: firstNumber(m, 0, "height", "h")}
	return p, true
}

func applyCCTalkPageElement(page *WhiteboardPage, kind string, attrs map[string]string, resources map[string]WhiteboardResource) {
	if page == nil || attrs == nil {
		return
	}
	resID := firstMapValue(attrs, "res", "id", "resourceId", "resId")
	rawURL := firstMapValue(attrs, "url", "src", "path", "href")
	if rawURL == "" && resID != "" {
		rawURL = resources[resID].URL
	}
	if kind == "image" {
		page.ImageID = resID
		page.ImageURL = rawURL
	} else {
		page.BoardID = resID
		page.BoardURL = rawURL
	}
	if page.X == 0 {
		page.X = parseFloat(firstMapValue(attrs, "x", "left"))
	}
	if page.Y == 0 {
		page.Y = parseFloat(firstMapValue(attrs, "y", "top"))
	}
	if page.Width == 0 {
		page.Width = parseFloat(firstMapValue(attrs, "width", "w"))
	}
	if page.Height == 0 {
		page.Height = parseFloat(firstMapValue(attrs, "height", "h"))
	}
}

func parsePoints(v any) []WhiteboardPoint {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		if nested, ok := decodeNestedJSON(s); ok {
			return parsePoints(nested)
		}
		nums := numberRe.FindAllString(s, -1)
		out := make([]WhiteboardPoint, 0, len(nums)/2)
		for i := 0; i+1 < len(nums); i += 2 {
			out = append(out, WhiteboardPoint{X: parseFloat(nums[i]), Y: parseFloat(nums[i+1])})
		}
		return out
	case []any:
		var out []WhiteboardPoint
		var nums []float64
		for _, item := range x {
			switch p := item.(type) {
			case []any:
				if len(p) >= 2 {
					out = append(out, WhiteboardPoint{X: numberAny(p[0]), Y: numberAny(p[1])})
				}
			case map[string]any:
				out = append(out, WhiteboardPoint{X: firstNumber(p, 0, "x", "left", "0"), Y: firstNumber(p, 0, "y", "top", "1")})
			case float64, int, int64, json.Number:
				nums = append(nums, numberAny(p))
			}
		}
		for i := 0; i+1 < len(nums); i += 2 {
			out = append(out, WhiteboardPoint{X: nums[i], Y: nums[i+1]})
		}
		return out
	case map[string]any:
		if xval, ok := x["x"]; ok {
			return []WhiteboardPoint{{X: numberAny(xval), Y: numberAny(x["y"])}}
		}
	}
	return nil
}

func normalizeEventType(raw string) string {
	low := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(low, "eras") || strings.Contains(low, "rubber"):
		return "eraser"
	case strings.Contains(low, "text") || strings.Contains(low, "font"):
		return "text"
	case strings.Contains(low, "clear"):
		return "clear"
	case strings.Contains(low, "page") || strings.Contains(low, "switch"):
		return "page"
	default:
		return "path"
	}
}

func parseTimeMS(v any) int64 {
	n := numberAny(v)
	if n <= 0 {
		return 0
	}
	if n < 10000 && strings.Contains(fmt.Sprint(v), ".") {
		n *= 1000
	}
	return int64(n + 0.5)
}

func normalizeColor(v any) string {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "<nil>" {
		return ""
	}
	if strings.HasPrefix(s, "#") || strings.HasPrefix(strings.ToLower(s), "rgb") {
		return s
	}
	if n, err := strconv.ParseInt(strings.TrimPrefix(s, "0x"), 10, 64); err == nil && n >= 0 && n <= 0xffffff {
		return fmt.Sprintf("#%06x", n)
	}
	if n, err := strconv.ParseInt(strings.TrimPrefix(s, "0x"), 16, 64); err == nil && n >= 0 && n <= 0xffffff {
		return fmt.Sprintf("#%06x", n)
	}
	return s
}

var numberRe = regexp.MustCompile(`-?\d+(?:\.\d+)?`)

func xmlAttrs(attrs []xml.Attr) map[string]string {
	out := map[string]string{}
	for _, attr := range attrs {
		out[attr.Name.Local] = attr.Value
	}
	return out
}

func firstValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" && strings.TrimSpace(fmt.Sprint(v)) != "<nil>" {
			return v
		}
		for k, v := range m {
			if strings.EqualFold(k, key) && v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" && strings.TrimSpace(fmt.Sprint(v)) != "<nil>" {
				return v
			}
		}
	}
	return nil
}

func firstText(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := firstValue(m, key); v != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
}

func firstURLText(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := firstText(m, key); looksURLOrPath(s) {
			return s
		}
	}
	return ""
}

func firstNumber(m map[string]any, def float64, keys ...string) float64 {
	for _, key := range keys {
		if v := firstValue(m, key); v != nil {
			if n := numberAny(v); n != 0 {
				return n
			}
		}
	}
	return def
}

func firstMapValue(m map[string]string, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(m[key]); v != "" {
			return v
		}
		for k, v := range m {
			if strings.EqualFold(k, key) && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func numberAny(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		return parseFloat(x)
	default:
		return parseFloat(fmt.Sprint(v))
	}
}

func parseFloat(raw string) float64 {
	s := strings.TrimSpace(raw)
	if s == "" || s == "<nil>" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func looksURLOrPath(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "//") || strings.HasPrefix(low, "/") || strings.Contains(low, ".json") || strings.Contains(low, ".jpg") || strings.Contains(low, ".png") || strings.Contains(low, ".webp") || strings.Contains(low, ".gif")
}

func decodeNestedJSON(raw string) (any, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, false
	}
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil && len(decoded) > 0 {
		plain := strings.TrimSpace(string(decoded))
		if strings.HasPrefix(plain, "{") || strings.HasPrefix(plain, "[") {
			s = plain
		}
	}
	if !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "[") {
		return nil, false
	}
	var out any
	if json.Unmarshal([]byte(s), &out) != nil {
		return nil, false
	}
	return out, true
}

func compactRaw(m map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"id", "type", "eventType", "drawtime", "drawTime", "time", "startTime", "eraseTime"} {
		if v := firstValue(m, key); v != nil {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func eventKey(ev WhiteboardEvent) string {
	return fmt.Sprintf("%d:%d:%s:%s:%s:%v", eventStartMS(ev), ev.EndMS+ev.EraseMS, ev.Type, ev.Page, ev.Text, ev.Points)
}

// PathEscapedDataURL is kept for callers that need percent-encoded data URLs.
func PathEscapedDataURL(mime, content string) string {
	return "data:" + mime + ";charset=utf-8," + url.PathEscape(content)
}
