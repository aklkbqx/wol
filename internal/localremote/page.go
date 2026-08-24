package localremote

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
)

var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>WOL Local Remote</title><style>
:root{color-scheme:dark;--bg:#0a0f11;--panel:#11191c;--text:#e8f4f2;--muted:#91a6a7;--cyan:#55e6dc;--amber:#ffc247;--line:#294044;--danger:#ff7b72}*{box-sizing:border-box}html,body{min-height:100%;margin:0}body{font:15px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;background:radial-gradient(circle at 50% 0,#153034 0,transparent 38rem),var(--bg);color:var(--text);padding:max(20px,env(safe-area-inset-top)) max(16px,env(safe-area-inset-right)) max(20px,env(safe-area-inset-bottom)) max(16px,env(safe-area-inset-left));display:grid;place-items:center}.card{width:min(100%,560px);border:1px solid var(--line);background:#101719eF;box-shadow:0 24px 80px #0008;padding:clamp(22px,5vw,42px)}.eyebrow{color:var(--cyan);font-weight:800;letter-spacing:.12em}.state{display:flex;align-items:center;gap:9px;color:var(--muted);margin:8px 0 28px}.dot{width:8px;height:8px;border-radius:50%;background:var(--cyan);box-shadow:0 0 16px var(--cyan)}h1{font:700 clamp(25px,6vw,38px)/1.15 inherit;margin:0 0 10px}p{color:var(--muted);margin:0 0 28px}.target{display:grid;grid-template-columns:1fr auto;gap:10px;border-block:1px solid var(--line);padding:14px 0;margin-bottom:24px}.target strong{overflow-wrap:anywhere}.tag{color:var(--amber);text-transform:uppercase}label{display:block;color:var(--muted);font-size:13px;margin:16px 0 7px}input{width:100%;min-height:46px;border:1px solid var(--line);background:#091012;color:var(--text);padding:10px 12px;border-radius:3px;font:16px inherit;outline:none}input:focus{border-color:var(--cyan);box-shadow:0 0 0 3px #55e6dc20}button{width:100%;min-height:48px;margin-top:24px;border:0;border-radius:3px;background:var(--cyan);color:#071011;font:800 15px inherit;cursor:pointer}button:hover{filter:brightness(1.08)}.error{border-left:3px solid var(--danger);padding:10px 12px;color:#ffd7d3;background:#ff7b7212;margin-bottom:16px}.note{font-size:12px;margin:15px 0 0;text-align:center}@media(max-width:420px){body{padding:0;place-items:stretch}.card{min-height:100vh;border:0;padding:24px 18px}.target{grid-template-columns:1fr}.tag{font-size:12px}}@media(prefers-reduced-motion:reduce){*{scroll-behavior:auto!important}}
</style></head><body><main class="card"><div class="eyebrow">WOL LOCAL REMOTE</div><div class="state"><span class="dot"></span>Gateway ready on this computer</div><h1>Sign in to {{.Protocol}}</h1><p>Credentials stay in memory only and are sent through this localhost session.</p><div class="target"><strong>{{.Host}}:{{.Port}}</strong><span class="tag">{{.Policy}}</span></div>{{if .Error}}<div class="error" role="alert">{{.Error}}</div>{{end}}<form action="/connect" method="post" autocomplete="on"><input type="hidden" name="csrf" value="{{.CSRF}}">{{if ne .Protocol "vnc"}}<label for="username">Username</label><input id="username" name="username" value="{{.Username}}" autocomplete="username" autofocus>{{end}}{{if eq .Protocol "rdp"}}<label for="domain">Domain <span aria-hidden="true">·</span> optional</label><input id="domain" name="domain" value="{{.Domain}}" autocomplete="organization">{{end}}<label for="password">Password</label><input id="password" name="password" type="password" autocomplete="current-password" {{if eq .Protocol "vnc"}}autofocus{{end}}><button type="submit">Connect through localhost</button></form><p class="note">Closing WOL removes this temporary gateway.</p></main></body></html>`))

var sessionPage = template.Must(template.New("session").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>WOL Local Remote</title><style>
:root{color-scheme:dark;--bg:#0b1012;--panel:#11191c;--text:#e6f2f1;--muted:#91a6a7;--cyan:#55e6dc;--line:#26383b}*{box-sizing:border-box}html,body{height:100%;margin:0}body{font:15px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--bg);color:var(--text);overflow:hidden}.shell{height:100%;display:grid;grid-template-rows:auto 1fr;padding:max(14px,env(safe-area-inset-top)) max(14px,env(safe-area-inset-right)) max(14px,env(safe-area-inset-bottom)) max(14px,env(safe-area-inset-left));gap:12px}.bar{display:flex;align-items:center;gap:12px;min-height:42px;padding:9px 12px;border:1px solid var(--line);background:var(--panel)}.mark{color:var(--cyan);font-weight:700;letter-spacing:.08em}.status{color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.pulse{width:8px;height:8px;border-radius:50%;background:var(--cyan);box-shadow:0 0 0 0 #55e6dc80;animation:pulse 1.4s ease-out infinite}.frame{position:relative;min-height:0;border:1px solid var(--line);background:#080b0c}.stage{position:absolute;inset:0;display:grid;place-content:center;text-align:center;padding:24px;color:var(--muted)}.stage strong{display:block;color:var(--text);font-size:clamp(18px,3vw,30px);margin-bottom:8px}iframe{position:absolute;inset:0;width:100%;height:100%;border:0;background:#080b0c;opacity:0;transition:opacity .25s ease}iframe.ready{opacity:1}@keyframes pulse{70%{box-shadow:0 0 0 9px #55e6dc00}}@media(prefers-reduced-motion:reduce){.pulse{animation:none}iframe{transition:none}}@media(max-width:520px){.shell{padding:0;gap:0}.bar,.frame{border-left:0;border-right:0}.bar{min-height:48px}}
</style></head><body><main class="shell"><header class="bar"><span class="pulse" aria-hidden="true"></span><span class="mark">LOCAL REMOTE</span><span id="status" class="status" role="status">Starting isolated session…</span></header><section class="frame"><div class="stage"><div><strong>Preparing your remote session</strong>The browser gateway stays on this computer.</div></div><iframe id="remote" title="Remote desktop" allow="clipboard-read; clipboard-write" referrerpolicy="no-referrer"></iframe></section></main>
<script>const f=document.getElementById('remote'),s=document.getElementById('status');let n=0;async function ready(){try{const r=await fetch('/guacamole/',{cache:'no-store'});if(r.status<500){s.textContent='Opening encrypted remote connection…';f.src={{.FrameURL}};f.onload=()=>{f.className='ready';s.textContent='Remote client loaded · connection status is shown below'};return}}catch(e){}if(++n<90)setTimeout(ready,1000);else s.textContent='Remote engine did not become ready'}ready();</script></body></html>`))

func serveLoginPage(w http.ResponseWriter, cfg Config, csrfToken, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	policy := "certificate strict"
	if cfg.CertificatePolicy == "trust-local" {
		policy = "trusted local certificate"
	}
	if err := loginPage.Execute(w, map[string]any{
		"Protocol": cfg.Protocol, "Host": cfg.Host, "Port": cfg.Port,
		"Username": cfg.UsernameHint, "Domain": cfg.DomainHint,
		"Policy": policy, "CSRF": csrfToken, "Error": message,
	}); err != nil {
		http.Error(w, "Unable to render local sign-in.", http.StatusInternalServerError)
	}
}

func servePage(w http.ResponseWriter, launchToken string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	frameURL := "/guacamole/?data=" + url.QueryEscape(launchToken)
	if err := sessionPage.Execute(w, map[string]any{"FrameURL": template.JS(strconv.Quote(frameURL))}); err != nil {
		http.Error(w, "Unable to render local session.", http.StatusInternalServerError)
	}
}
