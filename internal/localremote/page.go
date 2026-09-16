package localremote

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/aklkbqx/wol/internal/ui"
)

var pageCSS = ui.DeskCSS() + pageLocal

const pageLocal = `html,body{margin:0;min-height:100%;background:var(--ink);color:var(--paper);font:16px/1.45 "Avenir Next","Segoe UI",system-ui,sans-serif}
button,input{font:inherit}
a,button{color:inherit}
:focus-visible{outline:2px solid var(--amber);outline-offset:3px}`

var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>{{.Title}} · wol</title>
<style>
` + pageCSS + `
body{min-height:100vh;display:grid;place-items:center;padding:max(28px,env(safe-area-inset-top)) max(20px,env(safe-area-inset-right)) max(28px,env(safe-area-inset-bottom)) max(20px,env(safe-area-inset-left))}
main{width:min(100%,22rem)}
.brand{color:var(--muted);letter-spacing:.08em;font-size:.8rem;margin:0 0 2.2rem}
.machine{display:flex;gap:.75rem;align-items:flex-start;margin:0 0 1.75rem}
.tick{color:var(--amber);font-weight:700;line-height:1.2}
h1{margin:0;font:600 1.6rem/1.15 inherit}
.meta{margin:.35rem 0 0;color:var(--muted);font-size:.92rem}
.error{margin:0 0 1.2rem;color:var(--danger)}
label{display:block;margin:0 0 .35rem;color:var(--muted);font-size:.86rem}
.field{margin:0 0 1rem}
.field input{display:block;width:100%;min-height:2.75rem;padding:.7rem .85rem;border:1px solid var(--line);border-radius:0;background:transparent;color:var(--paper)}
.field input::placeholder{color:#7A7F7488}
.field input:-webkit-autofill,.field input:-webkit-autofill:hover,.field input:-webkit-autofill:focus{
-webkit-text-fill-color:var(--paper);caret-color:var(--paper);
box-shadow:0 0 0 1000px var(--ink) inset;border:1px solid var(--line);transition:background-color 9999s
}
.remember{display:flex;gap:.65rem;align-items:flex-start;margin:1.2rem 0 0;color:var(--muted);font-size:.9rem}
.remember input{margin-top:.28rem;accent-color:var(--amber);flex:none}
.actions{display:grid;gap:.75rem;margin-top:1.6rem}
button.connect{min-height:2.85rem;border:0;background:var(--amber);color:var(--ink);font-weight:650;cursor:pointer}
button.connect:hover{filter:brightness(1.06)}
button.forget{border:0;background:transparent;color:var(--muted);min-height:2.4rem;cursor:pointer;text-align:center;padding:0}
button.forget:hover{color:var(--paper)}
.note{margin:1.6rem 0 0;color:var(--muted);font-size:.8rem}
@media(max-width:420px){body{place-items:stretch}main{width:100%}}
body.login{background:radial-gradient(ellipse at 50% 30%,var(--panel) 0%,var(--ink) 62%)}
body.login main{width:min(100%,22rem);animation:desk-enter 600ms cubic-bezier(0.22,1,0.36,1) both}
@keyframes desk-enter{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}
body.login .signal{height:2px;margin:.75rem 0 0;background:linear-gradient(90deg,transparent 0%,var(--amber) 50%,transparent 100%);background-size:40% 100%;background-repeat:no-repeat;background-position:50% 0;opacity:.35}
body.login main.is-sending .signal{opacity:1;background-position:-40% 0;animation:signal-travel 700ms cubic-bezier(0.22,1,0.36,1) infinite}
@keyframes signal-travel{from{background-position:-40% 0}to{background-position:140% 0}}
body.login .field input{transition:border-color 200ms cubic-bezier(0.165,0.84,0.44,1)}
body.login .field input:focus{border-color:var(--amber);background:var(--panel)}
body.login .field input::placeholder{color:color-mix(in srgb,var(--muted) 53%,transparent)}
body.login button.connect{transition:transform 120ms cubic-bezier(0.165,0.84,0.44,1)}
body.login button.connect:hover{filter:brightness(1.06)}
body.login button.connect:active{transform:scale(0.98)}
@media(prefers-reduced-motion:reduce){body.login button.connect:active{transform:none}}
</style></head>
<body class="login">
<main>
<p class="brand">wol</p>
<div class="machine"><span class="tick" aria-hidden="true">›</span><div>
<h1>{{.Title}}</h1>
<p class="meta">{{.ProtocolLabel}} · {{.Host}}:{{.Port}}</p>
</div></div>
<div class="signal" aria-hidden="true"></div>
{{if .Error}}<p class="error" role="alert">{{.Error}}</p>{{end}}
<form action="/connect" method="post" autocomplete="on" onsubmit="this.closest('main').classList.add('is-sending')">
<input type="hidden" name="csrf" value="{{.CSRF}}">
{{if ne .Protocol "vnc"}}
<div class="field"><label for="username">Username</label>
<input id="username" name="username" type="text" value="{{.Username}}" autocomplete="username" spellcheck="false" {{if ne .Protocol "vnc"}}autofocus{{end}}></div>
{{end}}
{{if eq .Protocol "rdp"}}
<div class="field"><label for="domain">Domain <span style="opacity:.7">(optional)</span></label>
<input id="domain" name="domain" type="text" value="{{.Domain}}" autocomplete="organization" spellcheck="false"></div>
{{end}}
<div class="field"><label for="password">Password</label>
<input id="password" name="password" type="password" autocomplete="current-password" placeholder="{{.PasswordHint}}" {{if eq .Protocol "vnc"}}autofocus{{end}}></div>
{{if .CanRemember}}
<label class="remember"><input type="checkbox" name="remember" value="1" {{if .Saved}}checked{{end}}><span>Remember on this Mac</span></label>
{{end}}
<div class="actions">
<button class="connect" type="submit" name="action" value="connect">Connect</button>
{{if .Saved}}<button class="forget" type="submit" name="action" value="forget">Forget saved sign-in</button>{{end}}
</div>
</form>
<p class="note">Saved in the system keychain, never in the inventory. Closing wol closes this session.</p>
</main>
</body></html>`))

var sessionPage = template.Must(template.New("session").Parse(`<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>{{.Title}} · wol</title>
<style>
` + pageCSS + `
html,body{height:100%;overflow:hidden}
body{display:grid;grid-template-rows:auto 1fr}
header{display:flex;flex-wrap:wrap;align-items:center;gap:1rem;min-height:3rem;padding:.55rem 1rem;padding-left:max(1rem,env(safe-area-inset-left));padding-right:max(1rem,env(safe-area-inset-right));border-bottom:0}
.brand{color:var(--muted)}
.tick{color:var(--amber)}
#status{color:var(--muted);min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.spacer{flex:1}
.account{color:var(--muted);text-decoration:none;font-size:.9rem;white-space:nowrap}
.account:hover{color:var(--paper)}
.disconnect{margin:0}
.disconnect button{min-height:2.2rem;padding:.35rem .8rem;border:1px solid var(--danger);background:transparent;color:var(--danger);cursor:pointer}
.disconnect button:hover{background:#C45C4A14}
header .signal{flex:1 0 100%;height:2px;margin:0;background:linear-gradient(90deg,transparent 0%,var(--amber) 50%,transparent 100%);background-size:40% 100%;background-repeat:no-repeat;background-position:50% 0;opacity:.35}
header.is-waiting .signal{opacity:1;background-position:-40% 0;animation:signal-travel 700ms cubic-bezier(0.22,1,0.36,1) infinite}
@keyframes signal-travel{from{background-position:-40% 0}to{background-position:140% 0}}
.frame{position:relative;min-height:0;background:var(--ink)}
.stage{position:absolute;inset:0;z-index:0;display:grid;place-content:center;text-align:center;padding:1.5rem;color:var(--muted);pointer-events:none}
.stage strong{display:block;color:var(--paper);font-size:1.25rem;font-weight:600;margin-bottom:.4rem}
iframe{position:absolute;inset:0;z-index:1;width:100%;height:100%;border:0;background:var(--ink);opacity:0;pointer-events:none;transition:opacity 300ms cubic-bezier(0.165,0.84,0.44,1)}
iframe.ready{opacity:1;pointer-events:auto}
@media(max-width:520px){#status{display:none}.disconnect button{min-height:2.5rem}}
</style></head>
<body>
<header class="is-waiting">
<span class="brand">wol</span>
<span class="tick" aria-hidden="true">›</span>
<span id="status" role="status">Connecting</span>
<span class="spacer"></span>
<a class="account" href="/session?manual=1">Switch account</a>
<form class="disconnect" action="/disconnect" method="post">
<input type="hidden" name="csrf" value="{{.CSRF}}">
<button type="submit">Disconnect</button>
</form>
<div class="signal" aria-hidden="true"></div>
</header>
<section class="frame">
<div class="stage" id="stage"><div><strong>{{.Title}}</strong>Waiting for the remote desktop.</div></div>
<iframe id="remote" title="Remote desktop" tabindex="0" allow="clipboard-read; clipboard-write" referrerpolicy="no-referrer" data-title="{{.Title}}"></iframe>
</section>
<script>
const f=document.getElementById('remote'),s=document.getElementById('status'),stage=document.getElementById('stage');
const title=f.dataset.title||'';
let n=0;
function grab(){
  f.focus();
  try{
    if(f.contentWindow) f.contentWindow.focus();
    const d=f.contentDocument;
    if(!d) return;
    const el=d.querySelector('canvas,.display,.guacamole-viewer')||d.body;
    if(!el) return;
    if(el.tabIndex<0) el.tabIndex=0;
    el.focus();
  }catch(e){}
}
async function ready(){
  try{
    const r=await fetch('/guacamole/',{cache:'no-store'});
    if(r.status<500){
      s.textContent='Opening '+title;
      f.src={{.FrameURL}};
      f.onload=()=>{
        f.className='ready';
        if(stage) stage.hidden=true;
        s.textContent=title;
        document.querySelector('header')?.classList.remove('is-waiting');
        grab();
        setTimeout(grab,200);
        setTimeout(grab,800);
      };
      return;
    }
  }catch(e){}
  if(++n<90)setTimeout(ready,1000); else s.textContent='Remote did not become ready';
}
document.addEventListener('pointerdown',e=>{
  if(e.target.closest&&e.target.closest('.disconnect')) return;
  grab();
});
window.addEventListener('keydown',e=>{
  if(e.target.closest&&e.target.closest('.disconnect')) return;
  if(document.activeElement!==f) grab();
},true);
window.addEventListener('focus',grab);
ready();
</script>
</body></html>`))

var closedPage = template.Must(template.New("closed").Parse(`<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Disconnected · wol</title>
<style>
` + pageCSS + `
body{min-height:100vh;display:grid;place-items:center;padding:2rem;text-align:center}
.tick{color:var(--amber);margin:0 0 .6rem;font-size:1.2rem}
h1{margin:0 0 .5rem;font:600 1.5rem/1.2 inherit}
p{margin:0;color:var(--muted)}
@keyframes desk-enter{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}
body.closed .tick{animation:desk-enter 500ms cubic-bezier(0.22,1,0.36,1) both}
</style></head>
<body class="closed">
<main>
<p class="tick" aria-hidden="true">›</p>
<h1>Remote disconnected</h1>
<p>You can close this tab.</p>
</main>
</body></html>`))

type loginView struct {
	Title         string
	Protocol      string
	ProtocolLabel string
	Host          string
	Port          int
	Username      string
	Domain        string
	CSRF          string
	Error         string
	CanRemember   bool
	Saved         bool
	PasswordHint  string
}

func sessionTitle(cfg Config) string {
	if name := strings.TrimSpace(cfg.Name); name != "" {
		return name
	}
	return cfg.Host
}

func loginFromConfig(cfg Config, csrfToken, message string) loginView {
	view := loginView{
		Title:         sessionTitle(cfg),
		Protocol:      strings.ToLower(cfg.Protocol),
		ProtocolLabel: strings.ToUpper(cfg.Protocol),
		Host:          cfg.Host,
		Port:          cfg.Port,
		Username:      cfg.UsernameHint,
		Domain:        cfg.DomainHint,
		CSRF:          csrfToken,
		Error:         message,
		CanRemember:   cfg.Vault != nil,
		PasswordHint:  "",
	}
	if cfg.Vault == nil {
		return view
	}
	saved, err := cfg.Vault.Get(VaultKey(cfg.Protocol, cfg.Host, cfg.Port))
	if err != nil {
		return view
	}
	view.Saved = true
	if saved.Username != "" {
		view.Username = saved.Username
	}
	if saved.Domain != "" {
		view.Domain = saved.Domain
	}
	if saved.Password != "" {
		view.PasswordHint = "Saved on this Mac"
	}
	return view
}

func serveLoginPage(w http.ResponseWriter, cfg Config, csrfToken, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := loginPage.Execute(w, loginFromConfig(cfg, csrfToken, message)); err != nil {
		http.Error(w, "Unable to render local sign-in.", http.StatusInternalServerError)
	}
}

func servePage(w http.ResponseWriter, launchToken, csrfToken string, cfg Config) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	frameURL := "/guacamole/?data=" + url.QueryEscape(launchToken)
	if err := sessionPage.Execute(w, map[string]any{
		"FrameURL": template.JS(strconv.Quote(frameURL)),
		"CSRF":     csrfToken,
		"Title":    sessionTitle(cfg),
	}); err != nil {
		http.Error(w, "Unable to render local session.", http.StatusInternalServerError)
	}
}

func serveClosedPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := closedPage.Execute(w, nil); err != nil {
		http.Error(w, "Unable to render disconnect page.", http.StatusInternalServerError)
	}
}
