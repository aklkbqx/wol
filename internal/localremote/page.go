package localremote

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
)

var sessionPage = template.Must(template.New("session").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>WOL Local Remote</title><style>
:root{color-scheme:dark;--bg:#0b1012;--panel:#11191c;--text:#e6f2f1;--muted:#91a6a7;--cyan:#55e6dc;--line:#26383b}*{box-sizing:border-box}html,body{height:100%;margin:0}body{font:15px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--bg);color:var(--text);overflow:hidden}.shell{height:100%;display:grid;grid-template-rows:auto 1fr;padding:max(14px,env(safe-area-inset-top)) max(14px,env(safe-area-inset-right)) max(14px,env(safe-area-inset-bottom)) max(14px,env(safe-area-inset-left));gap:12px}.bar{display:flex;align-items:center;gap:12px;min-height:42px;padding:9px 12px;border:1px solid var(--line);background:var(--panel)}.mark{color:var(--cyan);font-weight:700;letter-spacing:.08em}.status{color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.pulse{width:8px;height:8px;border-radius:50%;background:var(--cyan);box-shadow:0 0 0 0 #55e6dc80;animation:pulse 1.4s ease-out infinite}.frame{position:relative;min-height:0;border:1px solid var(--line);background:#080b0c}.stage{position:absolute;inset:0;display:grid;place-content:center;text-align:center;padding:24px;color:var(--muted)}.stage strong{display:block;color:var(--text);font-size:clamp(18px,3vw,30px);margin-bottom:8px}iframe{position:absolute;inset:0;width:100%;height:100%;border:0;background:#080b0c;opacity:0;transition:opacity .25s ease}iframe.ready{opacity:1}@keyframes pulse{70%{box-shadow:0 0 0 9px #55e6dc00}}@media(prefers-reduced-motion:reduce){.pulse{animation:none}iframe{transition:none}}@media(max-width:520px){.shell{padding:0;gap:0}.bar,.frame{border-left:0;border-right:0}.bar{min-height:48px}}
</style></head><body><main class="shell"><header class="bar"><span class="pulse" aria-hidden="true"></span><span class="mark">LOCAL REMOTE</span><span id="status" class="status" role="status">Starting isolated session…</span></header><section class="frame"><div class="stage"><div><strong>Preparing your remote session</strong>The browser gateway stays on this computer.</div></div><iframe id="remote" title="Remote desktop" allow="clipboard-read; clipboard-write" referrerpolicy="no-referrer"></iframe></section></main>
<script>const f=document.getElementById('remote'),s=document.getElementById('status');let n=0;async function ready(){try{const r=await fetch('/guacamole/',{cache:'no-store'});if(r.status<500){f.src={{.FrameURL}};f.onload=()=>{f.className='ready';s.textContent='Connected through localhost'};return}}catch(e){}if(++n<90)setTimeout(ready,1000);else s.textContent='Remote engine did not become ready'}ready();</script></body></html>`))

func servePage(w http.ResponseWriter, launchToken string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	frameURL := "/guacamole/?data=" + url.QueryEscape(launchToken)
	if err := sessionPage.Execute(w, map[string]any{"FrameURL": template.JS(strconv.Quote(frameURL))}); err != nil {
		http.Error(w, "Unable to render local session.", http.StatusInternalServerError)
	}
}
