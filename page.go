package main

import (
	"html"
	"net/http"
	"strings"
)

const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="14" fill="#2f6feb"/><path d="M32 14l14 15h-9v13H27V29h-9z" fill="#fff"/><rect x="18" y="46" width="28" height="5" rx="2.5" fill="#fff"/></svg>`

// 公共样式，列表页与设置页共用
const sharedCSS = `
*{box-sizing:border-box}
:root{
  color-scheme:light;
  --bg:#f3f5f8;--panel:#fff;--panel2:#fafbfc;--border:#e3e7ec;--border2:#eef1f4;
  --text:#1b1f24;--text2:#5b6672;--text3:#8b96a3;
  --accent:#2f6feb;--accent-bg:#eaf1ff;--hover:#f1f5fa;
  --shadow:0 1px 2px rgba(16,24,40,.06),0 1px 3px rgba(16,24,40,.05);
  --dir:#e0a020;--file:#98a2b3;--ok:#12a150;--err:#e5484d;
}
html[data-theme="dark"]{
  color-scheme:dark;
  --bg:#131519;--panel:#1b1e24;--panel2:#1f232a;--border:#2b3038;--border2:#232830;
  --text:#e6e9ee;--text2:#a3adba;--text3:#79828f;
  --accent:#6ea8fe;--accent-bg:#1d2738;--hover:#222731;
  --shadow:0 1px 2px rgba(0,0,0,.35);
  --dir:#e8b339;--file:#9aa5b4;--ok:#3dd68c;--err:#ff6369;
}
html,body{margin:0;padding:0}
body{
  background:var(--bg);color:var(--text);
  font:14px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI","Microsoft YaHei",Roboto,"Helvetica Neue",Arial,sans-serif;
  -webkit-font-smoothing:antialiased;
}
.wrap{max-width:1120px;margin:0 auto;padding:0 20px 48px}
header{position:sticky;top:0;z-index:20;background:var(--panel);border-bottom:1px solid var(--border);box-shadow:var(--shadow)}
header .inner{max-width:1120px;margin:0 auto;padding:14px 20px;display:flex;align-items:center;gap:12px;flex-wrap:wrap}
.logo{display:flex;align-items:center;gap:10px;font-weight:650;font-size:16px;letter-spacing:.2px}
.logo svg,.logo img{width:26px;height:26px;flex:none;border-radius:7px;object-fit:contain}
.ver{font-size:11px;font-weight:500;color:var(--text3);border:1px solid var(--border);border-radius:20px;padding:1px 8px}
.spacer{flex:1}
.chip{display:inline-flex;align-items:center;gap:6px;max-width:100%;font-size:12px;color:var(--text2);
  background:var(--panel2);border:1px solid var(--border);border-radius:8px;padding:4px 10px;
  font-family:ui-monospace,SFMono-Regular,Consolas,"Courier New",monospace;
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.btn{appearance:none;cursor:pointer;font:inherit;font-size:13px;background:var(--panel);color:var(--text);
  border:1px solid var(--border);border-radius:8px;padding:6px 12px;transition:.15s;
  display:inline-flex;align-items:center;gap:6px;white-space:nowrap;text-decoration:none}
.btn:hover{background:var(--hover)}
.btn:disabled{opacity:.45;cursor:not-allowed}
.btn.icon{padding:6px 9px}
.btn.primary{background:var(--accent);border-color:var(--accent);color:#fff}
.btn.primary:hover{filter:brightness(1.07)}
.card{background:var(--panel);border:1px solid var(--border);border-radius:12px;overflow:hidden;box-shadow:var(--shadow)}
.mini{font-size:12px;color:var(--text2)}
.bar{display:flex;align-items:center;gap:14px;flex-wrap:wrap;margin:12px 2px 0;font-size:12.5px;color:var(--text2)}
.bar .dot{color:var(--text3)}
.notice{margin:14px 0 0;padding:10px 14px;border-radius:9px;font-size:12.5px;white-space:pre-wrap;
  background:var(--accent-bg);color:var(--accent);border:1px solid var(--border)}
.notice.warn{background:rgba(224,160,32,.12);color:var(--dir)}
.notice.err{background:rgba(229,72,77,.12);color:var(--err)}
.notice.ok{background:rgba(18,161,80,.12);color:var(--ok)}
#toast{position:fixed;left:50%;bottom:34px;transform:translate(-50%,16px);background:#111418;color:#fff;
  font-size:13px;padding:9px 18px;border-radius:9px;opacity:0;pointer-events:none;transition:.22s;z-index:99;
  box-shadow:0 6px 22px rgba(0,0,0,.28)}
html[data-theme="dark"] #toast{background:#e9edf3;color:#14171c}
#toast.show{opacity:1;transform:translate(-50%,0)}
footer{margin-top:26px;text-align:center;font-size:12px;color:var(--text3);line-height:1.9}
footer code{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;color:var(--text2)}
.foot-intro{color:var(--text2)}
.foot-intro:empty{display:none}
.badge-warn{font-size:11px;color:var(--err);border:1px solid var(--err);border-radius:4px;
  padding:0 5px;margin-left:6px;font-weight:400;white-space:nowrap}
@media (max-width:720px){.hide-sm{display:none}.wrap{padding:0 12px 40px}header .inner{padding:12px}}
`

const sharedJS = `
var THEME_KEY='localdrive-theme';
function applyTheme(t){document.documentElement.setAttribute('data-theme',t);}
function initTheme(){
  var s=null;try{s=localStorage.getItem(THEME_KEY);}catch(e){}
  if(s){applyTheme(s);return;}
  var dark=window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches;
  applyTheme(dark?'dark':'light');
}
function toggleTheme(){
  var cur=document.documentElement.getAttribute('data-theme');
  var next=cur==='dark'?'light':'dark';
  applyTheme(next);
  try{localStorage.setItem(THEME_KEY,next);}catch(e){}
}
var toastTimer=null;
function toast(msg){
  var t=document.getElementById('toast');
  if(!t)return;
  t.textContent=msg;t.classList.add('show');
  clearTimeout(toastTimer);
  toastTimer=setTimeout(function(){t.classList.remove('show');},1800);
}
function esc(s){return String(s).replace(/[&<>"']/g,function(c){
  return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c];});}
function fmtSize(n){
  if(n===null||n===undefined)return '-';
  if(n<1024)return n+' B';
  var u=['KB','MB','GB','TB','PB'],i=-1;
  do{n/=1024;i++;}while(n>=1024&&i<u.length-1);
  return (n<10?n.toFixed(2):(n<100?n.toFixed(1):n.toFixed(0)))+' '+u[i];
}
function pad(x){return x<10?'0'+x:''+x;}
function fmtTime(ms){if(!ms)return '-';var d=new Date(ms);
  return d.getFullYear()+'-'+pad(d.getMonth()+1)+'-'+pad(d.getDate())+' '+pad(d.getHours())+':'+pad(d.getMinutes());}
function encPath(p){return String(p).split('/').map(encodeURIComponent).join('/');}
function natcmp(a,b){
  a=String(a);b=String(b);
  var re=/(\d+)|(\D+)/g,ax=a.match(re)||[],bx=b.match(re)||[];
  var n=Math.max(ax.length,bx.length);
  for(var i=0;i<n;i++){
    var x=ax[i]||'',y=bx[i]||'';
    if(x===y)continue;
    var nx=/^\d/.test(x),ny=/^\d/.test(y);
    if(nx&&ny){var d=parseInt(x,10)-parseInt(y,10);if(d)return d;if(x.length!==y.length)return x.length-y.length;}
    else{return x<y?-1:1;}
  }
  return 0;
}
function copyText(text){
  function fallback(){
    var ta=document.createElement('textarea');
    ta.value=text;ta.style.position='fixed';ta.style.opacity='0';
    document.body.appendChild(ta);ta.select();
    try{document.execCommand('copy');toast('已复制: '+text);}catch(e){toast('复制失败');}
    document.body.removeChild(ta);
  }
  if(navigator.clipboard&&window.isSecureContext){
    navigator.clipboard.writeText(text).then(function(){toast('已复制: '+text);},fallback);
  }else{fallback();}
}
`

// ---------------------------------------------------------------- 列表页

const pageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{PAGE_TITLE}}</title>
<link rel="icon" href="{{FAVICON_URL}}">
<style>` + sharedCSS + `
.toolbar{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin:20px 0 12px}
.search{position:relative;flex:1;min-width:220px;max-width:420px}
.search input{width:100%;font:inherit;font-size:13px;color:var(--text);background:var(--panel);
  border:1px solid var(--border);border-radius:9px;padding:8px 32px;outline:none;transition:.15s}
.search input:focus{border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-bg)}
.search .si{position:absolute;left:10px;top:50%;transform:translateY(-50%);color:var(--text3);pointer-events:none}
.search .kbd{position:absolute;right:8px;top:50%;transform:translateY(-50%);font-size:10px;color:var(--text3);
  border:1px solid var(--border);border-radius:4px;padding:0 5px;pointer-events:none}
.crumbs{display:flex;align-items:center;gap:4px;flex-wrap:wrap;margin:0 0 12px;font-size:13px;color:var(--text2);min-height:24px}
.crumbs a{color:var(--accent);text-decoration:none;cursor:pointer;padding:2px 5px;border-radius:5px}
.crumbs a:hover{background:var(--accent-bg)}
.crumbs .sep{color:var(--text3)}
.crumbs .cur{color:var(--text);font-weight:600;padding:2px 5px}
table{width:100%;border-collapse:collapse;table-layout:fixed}
thead th{text-align:left;font-size:12px;font-weight:600;color:var(--text2);background:var(--panel2);
  border-bottom:1px solid var(--border);padding:9px 14px;white-space:nowrap;user-select:none}
thead th.sortable{cursor:pointer}
thead th.sortable:hover{color:var(--accent)}
th .arw{color:var(--accent);font-size:10px;margin-left:3px}
tbody td{padding:8px 14px;border-bottom:1px solid var(--border2);vertical-align:middle}
tbody tr:last-child td{border-bottom:0}
tbody tr:hover{background:var(--hover)}
td.c-name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
td.c-name a{color:var(--text);text-decoration:none;display:inline-flex;align-items:center;max-width:100%;cursor:pointer}
td.c-name a:hover .fname{color:var(--accent);text-decoration:underline}
.ico{width:18px;height:18px;flex:none;display:inline-block;vertical-align:middle;margin-right:9px}
.dim{color:var(--text3);font-weight:400}
.fname{font-weight:500;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
td.c-size,td.c-time{color:var(--text2);font-size:12.5px;white-space:nowrap;font-variant-numeric:tabular-nums}
td.c-act{text-align:right;white-space:nowrap}
td.c-act .btn{font-size:12px;padding:3px 9px;margin-left:6px;opacity:.55;transition:opacity .15s}
tbody tr:hover td.c-act .btn{opacity:1}
.empty{padding:52px 20px;text-align:center;color:var(--text3)}
.empty .big{font-size:26px;margin-bottom:8px;opacity:.6}
.pager{display:flex;align-items:center;gap:12px;flex-wrap:wrap;padding:11px 14px;
  border-top:1px solid var(--border);background:var(--panel2);font-size:12.5px;color:var(--text2)}
.pager .grow{flex:1}
.pager button{appearance:none;cursor:pointer;font:inherit;font-size:13px;color:var(--text);
  background:var(--panel);border:1px solid var(--border);border-radius:7px;padding:4px 10px;min-width:34px}
.pager button:hover:not(:disabled){background:var(--hover)}
.pager button:disabled{opacity:.4;cursor:not-allowed}
.pager input[type=number]{width:64px;font:inherit;font-size:13px;color:var(--text);background:var(--panel);
  border:1px solid var(--border);border-radius:7px;padding:4px 7px;outline:none;text-align:center}
.pager input[type=number]:focus{border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-bg)}
.pager select{font:inherit;font-size:13px;color:var(--text);background:var(--panel);
  border:1px solid var(--border);border-radius:7px;padding:4px 7px;cursor:pointer;outline:none}
.pager .pgs{display:flex;align-items:center;gap:6px}
/* ---- 预览弹窗 ---- */
#pv{position:fixed;inset:0;z-index:80;display:none;align-items:center;justify-content:center;
  background:rgba(10,12,16,.62);backdrop-filter:blur(3px);padding:24px}
#pv.show{display:flex;animation:pvfade .16s ease}
@keyframes pvfade{from{opacity:0}to{opacity:1}}
.pv-box{background:var(--panel);border:1px solid var(--border);border-radius:14px;overflow:hidden;
  display:flex;flex-direction:column;max-width:min(1180px,94vw);max-height:92vh;width:auto;min-width:min(520px,92vw);
  box-shadow:0 24px 70px rgba(0,0,0,.45)}
.pv-head{display:flex;align-items:center;gap:8px;padding:11px 14px;border-bottom:1px solid var(--border);
  background:var(--panel2);flex:none}
.pv-title{font-size:13.5px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1;min-width:0}
.pv-meta{font-size:12px;color:var(--text3);white-space:nowrap}
.pv-media{position:relative;flex:1 1 auto;display:flex;min-height:0}
.pv-body{flex:1;overflow:hidden;display:flex;align-items:center;justify-content:center;
  background:var(--bg);min-height:120px;padding:0}
.pv-body img{max-width:100%;max-height:78vh;display:block;object-fit:contain}
.pv-body video{max-width:100%;max-height:78vh;display:block;background:#000}
.pv-body audio{width:min(560px,80vw);margin:36px 24px}
.pv-body iframe{width:min(1100px,92vw);height:78vh;border:0;background:#fff}
.pv-body pre{margin:0;padding:16px 18px;width:100%;max-height:78vh;overflow:auto;align-self:stretch;
  font-family:ui-monospace,SFMono-Regular,Consolas,"Courier New",monospace;font-size:12.5px;line-height:1.6;
  white-space:pre-wrap;word-break:break-word;color:var(--text);text-align:left}
/* 图片缩放 / 拖动 */
.pv-zoom{display:flex;align-items:center;justify-content:center;width:100%;height:78vh;overflow:hidden;
  cursor:grab;touch-action:none;position:relative}
.pv-zoom.grabbing{cursor:grabbing}
.pv-zoom img{max-width:100%;max-height:100%;display:block;object-fit:contain;
  transform-origin:center center;user-select:none;-webkit-user-drag:none;will-change:transform}
/* 左右切换 */
.pv-nav{position:absolute;top:50%;transform:translateY(-50%);width:44px;height:44px;border-radius:50%;
  display:flex;align-items:center;justify-content:center;cursor:pointer;z-index:3;padding:0;
  background:rgba(18,22,28,.55);color:#fff;border:1px solid rgba(255,255,255,.2);font-size:22px;line-height:1;
  backdrop-filter:blur(4px);transition:background .15s,opacity .15s;user-select:none;font-family:inherit}
.pv-nav:hover{background:rgba(18,22,28,.82)}
.pv-nav.prev{left:12px}
.pv-nav.next{right:12px}
.pv-fallback{padding:52px 40px;text-align:center;color:var(--text2);font-size:13.5px}
.pv-fallback .big{font-size:34px;margin-bottom:12px;opacity:.5}
.pv-fallback .sub{color:var(--text3);font-size:12.5px;margin-top:8px}
.pv-box:fullscreen{max-width:100vw;max-height:100vh;width:100vw;height:100vh;border-radius:0;border:0}
.pv-box:fullscreen .pv-body img,
.pv-box:fullscreen .pv-body video{max-height:100%}
.pv-box:fullscreen .pv-zoom{height:100%}
.pv-box:fullscreen .pv-body iframe{width:100%;height:100%}
.pv-box:fullscreen .pv-body pre{max-height:100%}
@media (max-width:720px){#pv{padding:0}.pv-box{max-width:100vw;max-height:100vh;border-radius:0;width:100vw;height:100vh}
  .pv-nav{width:38px;height:38px;font-size:19px}}
</style>
</head>
<body>
<header>
  <div class="inner">
    <span class="logo">
      <img src="{{FAVICON_URL}}" alt="">
      {{SITE_NAME}}
      <span class="ver" id="verChip">v</span>
    </span>
    <span class="chip" id="rootChip">加载中 ...</span>
    <span class="spacer"></span>
    <button class="btn icon" id="themeBtn" title="切换深色 / 浅色">◐</button>
    <a class="btn" href="/settings" title="设置">⚙ 设置</a>
    <button class="btn" id="refreshBtn">↻ 刷新</button>
  </div>
</header>

<div class="wrap">
  <div class="toolbar">
    <div class="search">
      <span class="si">⌕</span>
      <input id="q" type="search" placeholder="在当前目录中筛选 ..." autocomplete="off" spellcheck="false">
      <span class="kbd">/</span>
    </div>
    <span class="spacer"></span>
    <span class="mini" id="statTop">加载中 ...</span>
  </div>

  <div class="crumbs" id="crumbs"></div>

  <div class="card">
    <table>
      <colgroup>
        <col><col style="width:110px" class="hide-sm">
        <col style="width:160px" class="hide-sm"><col style="width:210px">
      </colgroup>
      <thead>
        <tr>
          <th class="sortable" data-key="name">名称<span class="arw" id="arw-name"></span></th>
          <th class="sortable hide-sm" data-key="size">大小<span class="arw" id="arw-size"></span></th>
          <th class="sortable hide-sm" data-key="mtime">修改时间<span class="arw" id="arw-mtime"></span></th>
          <th style="text-align:right">操作</th>
        </tr>
      </thead>
      <tbody id="tb"></tbody>
    </table>
    <div class="pager" id="pager"></div>
  </div>

  <div class="bar" id="statBar"></div>
  <div id="noticeWrap"></div>

  <footer>
    <div>展示目录 <code id="rootFoot">-</code> · 发布临时目录 <code id="pubFoot">-</code></div>
    <div class="foot-intro">{{FOOTER_TEXT}}</div>
  </footer>
</div>

<div id="pv">
  <div class="pv-box">
    <div class="pv-head">
      <span class="pv-title" id="pvTitle">预览</span>
      <span class="pv-meta" id="pvMeta"></span>
      <button class="btn icon" id="pvFull" title="全屏">⛶</button>
      <button class="btn" id="pvDl">下载</button>
      <button class="btn icon" id="pvClose" title="关闭 (Esc)">✕</button>
    </div>
    <div class="pv-media">
      <div class="pv-body" id="pvBody"></div>
      <button class="pv-nav prev" id="pvPrev" title="上一个 (←)">‹</button>
      <button class="pv-nav next" id="pvNext" title="下一个 (→)">›</button>
    </div>
  </div>
</div>

<div id="toast"></div>

<script>
` + sharedJS + `
(function(){
'use strict';
var $=function(id){return document.getElementById(id);};
var tb=$('tb');

var state={
  path:'', parent:'', entries:[], rows:[], q:'',
  sortKey:'name', sortDir:1,
  page:1, pageSize:100,
  info:null, current:null,
  mode:'dir', roots:[], notice:'',
  pvList:[], pvIndex:0
};

var PSIZE_KEY='localdrive-pagesize';

var ICONS={
  dir:'<svg class="ico" viewBox="0 0 24 24" fill="none"><path d="M3 6.5A2.5 2.5 0 0 1 5.5 4h3.7c.6 0 1.2.25 1.6.7l1.1 1.2h6.6A2.5 2.5 0 0 1 21 8.4v9.1A2.5 2.5 0 0 1 18.5 20h-13A2.5 2.5 0 0 1 3 17.5v-11z" fill="var(--dir)" opacity=".9"/></svg>',
  img:'<svg class="ico" viewBox="0 0 24 24" fill="none"><rect x="3" y="4.5" width="18" height="15" rx="2.5" fill="var(--file)"/><circle cx="8.6" cy="9.6" r="1.7" fill="#fff" opacity=".85"/><path d="M4.5 17l4.4-4.6 3.1 3.2 2.6-2.5 4.9 4.4" stroke="#fff" stroke-width="1.5" fill="none" opacity=".85"/></svg>',
  video:'<svg class="ico" viewBox="0 0 24 24" fill="none"><rect x="3" y="5" width="18" height="14" rx="2.5" fill="var(--file)"/><path d="M10.4 9.2l5 2.8-5 2.8z" fill="#fff" opacity=".9"/></svg>',
  audio:'<svg class="ico" viewBox="0 0 24 24" fill="none"><path d="M9 18V7.4l9-1.8v10.6" stroke="var(--file)" stroke-width="1.8" fill="none"/><circle cx="6.9" cy="18" r="2.4" fill="var(--file)"/><circle cx="15.9" cy="16.2" r="2.4" fill="var(--file)"/></svg>',
  zip:'<svg class="ico" viewBox="0 0 24 24" fill="none"><path d="M6 3h9l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" fill="var(--file)" opacity=".55"/><path d="M11 3v5h2V3z" fill="#fff" opacity=".9"/><path d="M11 9.5v2h2v-2zM11 13v2h2v-2z" fill="#fff" opacity=".9"/></svg>',
  code:'<svg class="ico" viewBox="0 0 24 24" fill="none"><path d="M8.5 8.5L4.8 12l3.7 3.5M15.5 8.5L19.2 12l-3.7 3.5M13.4 6.2l-2.8 11.6" stroke="var(--file)" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" fill="none"/></svg>',
  doc:'<svg class="ico" viewBox="0 0 24 24" fill="none"><path d="M6 3h8l5 5v13a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" fill="var(--file)" opacity=".5"/><path d="M14 3v5h5" stroke="var(--file)" stroke-width="1.5" fill="none"/><path d="M8.5 12.5h7M8.5 15.5h7M8.5 18h4" stroke="#fff" stroke-width="1.4" stroke-linecap="round" opacity=".95"/></svg>',
  file:'<svg class="ico" viewBox="0 0 24 24" fill="none"><path d="M6 3h8l5 5v13a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" fill="var(--file)" opacity=".45"/><path d="M14 3v5h5" stroke="var(--file)" stroke-width="1.5" fill="none"/></svg>'
};

var EXT_ICON={png:'img',jpg:'img',jpeg:'img',gif:'img',bmp:'img',webp:'img',svg:'img',ico:'img',avif:'img',
  mp4:'video',mkv:'video',avi:'video',mov:'video',wmv:'video',flv:'video',webm:'video',m4v:'video',
  mp3:'audio',wav:'audio',flac:'audio',aac:'audio',ogg:'audio',m4a:'audio',wma:'audio',
  zip:'zip',rar:'zip','7z':'zip',tar:'zip',gz:'zip',bz2:'zip',xz:'zip',jar:'zip',
  js:'code',ts:'code',jsx:'code',tsx:'code',go:'code',py:'code',java:'code',c:'code',h:'code',
  cpp:'code',cs:'code',php:'code',rb:'code',rs:'code',html:'code',css:'code',json:'code',
  xml:'code',yml:'code',yaml:'code',sh:'code',bat:'code',ps1:'code',sql:'code',vue:'code',
  txt:'doc',md:'doc',log:'doc',pdf:'doc',doc:'doc',docx:'doc',xls:'doc',xlsx:'doc',
  ppt:'doc',pptx:'doc',csv:'doc',ini:'doc',conf:'doc',toml:'doc'};

function iconFor(e){if(e.isDir)return ICONS.dir;return ICONS[EXT_ICON[e.ext]]||ICONS.file;}

/* ---------------- 预览类型 ---------------- */
var KIND={
  img:['png','jpg','jpeg','gif','bmp','webp','svg','ico','avif','apng','jfif'],
  video:['mp4','webm','ogv','m4v','mov'],
  videoMaybe:['mkv','avi','wmv','flv','ts','rmvb'],
  audio:['mp3','wav','ogg','oga','flac','aac','m4a','opus','weba'],
  pdf:['pdf'],
  text:['txt','md','markdown','log','json','xml','yml','yaml','ini','conf','cfg','csv','tsv',
        'js','mjs','cjs','ts','tsx','jsx','css','scss','less','html','htm','go','py','java','c','h',
        'cpp','hpp','cc','cs','php','rb','rs','sh','bash','bat','cmd','ps1','sql','vue','toml',
        'gitignore','env','properties','gradle','makefile','dockerfile','patch','diff','srt','vtt']
};
function kindOf(ext){
  ext=String(ext||'').toLowerCase();
  if(KIND.img.indexOf(ext)>=0)return 'img';
  if(KIND.video.indexOf(ext)>=0)return 'video';
  if(KIND.videoMaybe.indexOf(ext)>=0)return 'videoMaybe';
  if(KIND.audio.indexOf(ext)>=0)return 'audio';
  if(KIND.pdf.indexOf(ext)>=0)return 'pdf';
  if(KIND.text.indexOf(ext)>=0)return 'text';
  return '';
}
function canPreview(ext){return kindOf(ext)!=='';}

/* ---------------- 数据加载 ---------------- */
function loadInfo(){
  return fetch('/api/info').then(function(r){return r.json();}).then(function(d){
    state.info=d;
    state.roots=d.roots||[];
    var n=state.roots.length;
    var label=n===0?'未配置目录':(n===1?state.roots[0].path:(n+' 个目录'));
    $('rootChip').textContent=label;
    $('rootChip').title=state.roots.map(function(x){return x.path;}).join('\n');
    $('rootFoot').textContent=n?(n===1?state.roots[0].path:state.roots.map(function(x){return x.name;}).join(' · ')):'（未配置）';
    $('pubFoot').textContent=d.publishDir+(d.publishTTL>0?'（保留 '+(d.publishTTL>=60?Math.round(d.publishTTL/60)+' 小时':d.publishTTL+' 分钟')+'）':'');
    $('verChip').textContent='v'+d.version;
    if(d.pageTitle)document.title=d.pageTitle;
    var saved=null;
    try{saved=parseInt(localStorage.getItem(PSIZE_KEY),10);}catch(e){}
    state.pageSize=(saved&&saved>=10)?saved:(d.pageSize||100);
  }).catch(function(){});
}
function reload(){
  tb.innerHTML='<tr><td colspan="4" class="empty">正在读取目录 ...</td></tr>';
  $('pager').innerHTML='';
  return fetch('/api/list?p='+encodeURIComponent(state.path)).then(function(r){
    return r.json().then(function(d){
      if(!r.ok)throw new Error(d.error||('HTTP '+r.status));
      return d;});
  }).then(function(d){
    state.entries=d.entries||[];
    state.parent=d.parent||'';
    state.mode=d.mode||'dir';
    state.notice=d.notice||'';
    if(d.roots)state.roots=d.roots;
    state.page=1;
    render();
  }).catch(function(err){
    tb.innerHTML='<tr><td colspan="4" class="empty"><div class="big">!</div>'+esc(err.message||'加载失败')+'</td></tr>';
    $('statBar').textContent='';$('statTop').textContent='';$('pager').innerHTML='';
  });
}
function render(){renderCrumbs();computeRows();renderPage();renderStats();renderNotice();}

/* ---------------- 面包屑 ---------------- */
function rootNameOf(i){
  var rs=state.roots||[];
  return (i>=0&&i<rs.length)?rs[i].name:'';
}
function rootPathOf(i){
  var rs=state.roots||[];
  return (i>=0&&i<rs.length)?rs[i].path:'';
}
function renderCrumbs(){
  var el=$('crumbs');el.innerHTML='';
  var multi=(state.roots||[]).length>1;
  var segs=(state.path||'')?state.path.split('/'):[];
  var rootIdx=-1;
  if(segs.length&&segs[0].charAt(0)==='@'){
    rootIdx=parseInt(segs[0].slice(1),10);
    if(isNaN(rootIdx)||rootIdx<0)rootIdx=-1;
    segs=segs.slice(1);
  }
  function add(text,p,isCur,title){
    if(isCur){var c=document.createElement('span');c.className='cur';c.textContent=text;if(title)c.title=title;el.appendChild(c);}
    else{var a=document.createElement('a');a.textContent=text;if(title)a.title=title;
      a.onclick=function(){loadDir(p);};el.appendChild(a);}
  }
  function sep(){var s=document.createElement('span');s.className='sep';s.textContent='/';el.appendChild(s);}
  var atTop=(rootIdx<0&&segs.length===0);
  add(multi?'所有目录':'根目录','',atTop);
  var acc='';
  if(rootIdx>=0){
    acc='@'+rootIdx;
    sep();
    add(rootNameOf(rootIdx)||('目录 '+(rootIdx+1)),acc,segs.length===0,rootPathOf(rootIdx));
  }
  for(var i=0;i<segs.length;i++){
    sep();
    acc=acc?acc+'/'+segs[i]:segs[i];
    add(segs[i],acc,i===segs.length-1);
  }
}
function loadDir(p){
  state.path=p||'';
  var u=new URL(location.href);
  if(state.path)u.searchParams.set('p',state.path);else u.searchParams.delete('p');
  history.replaceState(null,'',u.toString());
  reload();
}

/* ---------------- 排序 / 过滤 / 分页 ---------------- */
function computeRows(){
  var q=state.q.trim().toLowerCase();
  var rows=state.entries;
  if(q){rows=rows.filter(function(e){
    return e.name.toLowerCase().indexOf(q)>=0||(e.path||'').toLowerCase().indexOf(q)>=0||
      (e.realPath||'').toLowerCase().indexOf(q)>=0;});}
  var k=state.sortKey,d=state.sortDir;
  state.rows=rows.slice().sort(function(a,b){
    if(a.isDir!==b.isDir)return a.isDir?-1:1;
    var r=0;
    if(k==='size')r=a.size-b.size;
    else if(k==='mtime')r=a.modTime-b.modTime;
    else r=natcmp(a.name,b.name);
    if(r===0)r=natcmp(a.path||a.name,b.path||b.name);
    return r*d;
  });
}
function pageCount(){
  return Math.max(1,Math.ceil(state.rows.length/state.pageSize));
}
function renderPage(){
  var total=state.rows.length;
  var pages=pageCount();
  if(state.page>pages)state.page=pages;
  if(state.page<1)state.page=1;
  var start=(state.page-1)*state.pageSize;
  var end=Math.min(start+state.pageSize,total);

  if(!total){
    tb.innerHTML='<tr><td colspan="4" class="empty"><div class="big">◌</div>'+
      (state.q?'没有匹配的文件':(state.mode==='roots'?'没有可展示的目录':'此目录为空'))+'</td></tr>';
    renderPager(0,0);
    return;
  }
  var buf=[];
  for(var i=start;i<end;i++){buf.push(rowHTML(state.rows[i]));}
  tb.innerHTML=buf.join('');
  renderPager(start+1,end);
}
function renderPager(from,to){
  var total=state.rows.length,pages=pageCount();
  var el=$('pager');
  if(!total){el.innerHTML='';return;}
  var sizes=[10,20,50,100,200,500,1000];
  // 配置里的值可能不在预设列表中，补进去以免下拉框显示错误
  if(sizes.indexOf(state.pageSize)<0){
    sizes.push(state.pageSize);
    sizes.sort(function(a,b){return a-b;});
  }
  var opts='';
  for(var i=0;i<sizes.length;i++){
    opts+='<option value="'+sizes[i]+'"'+(sizes[i]===state.pageSize?' selected':'')+'>'+sizes[i]+'</option>';
  }
  el.innerHTML=
    '<span>共 <b>'+total+'</b> 项，当前显示第 '+from+'-'+to+' 条</span>'+
    '<span class="grow"></span>'+
    '<span class="pgs">'+
      '<button data-pg="first" title="第一页"'+(state.page<=1?' disabled':'')+'>«</button>'+
      '<button data-pg="prev" title="上一页"'+(state.page<=1?' disabled':'')+'>‹</button>'+
      '<span>第 <input type="number" id="pgNum" min="1" max="'+pages+'" value="'+state.page+'"> / '+pages+' 页</span>'+
      '<button data-pg="next" title="下一页"'+(state.page>=pages?' disabled':'')+'>›</button>'+
      '<button data-pg="last" title="最后一页"'+(state.page>=pages?' disabled':'')+'>»</button>'+
    '</span>'+
    '<span class="pgs">每页 <select id="pgSize">'+opts+'</select> 条</span>';

  el.querySelectorAll('button[data-pg]').forEach(function(b){
    b.addEventListener('click',function(){
      var a=b.getAttribute('data-pg');
      if(a==='first')state.page=1;
      else if(a==='prev')state.page--;
      else if(a==='next')state.page++;
      else if(a==='last')state.page=pageCount();
      renderPage();renderStats();scrollTop();
    });
  });
  var num=$('pgNum');
  if(num){
    num.addEventListener('keydown',function(ev){
      if(ev.key==='Enter'){
        var v=parseInt(num.value,10)||1;
        state.page=Math.min(Math.max(v,1),pageCount());
        renderPage();renderStats();scrollTop();
      }
    });
    num.addEventListener('blur',function(){
      var v=parseInt(num.value,10)||1;
      state.page=Math.min(Math.max(v,1),pageCount());
      renderPage();renderStats();
    });
  }
  var sel=$('pgSize');
  if(sel){
    sel.addEventListener('change',function(){
      state.pageSize=parseInt(sel.value,10)||100;
      try{localStorage.setItem(PSIZE_KEY,String(state.pageSize));}catch(e){}
      state.page=1;
      renderPage();renderStats();
    });
  }
}
function scrollTop(){
  var top=document.querySelector('.card');
  if(top&&top.getBoundingClientRect().top<0){
    window.scrollTo({top:top.offsetTop-70,behavior:'smooth'});
  }
}

/* ---------------- 行渲染 ---------------- */
function rowHTML(e){
  var p=e.path||e.name;
  var real=e.realPath||'';
  var url='/raw/'+encPath(p);
  var prev=!e.isDir&&canPreview(e.ext);
  var tip=real||p;
  var badge=e.missing?'<span class="badge-warn">目录不存在</span>':'';
  var nameCell=e.isDir
    ?'<a href="javascript:void(0)" data-act="open" title="'+esc(tip)+'">'+iconFor(e)+
      '<span class="fname">'+esc(e.name)+'</span>'+badge+'</a>'
    :'<a href="'+url+'" target="_blank" rel="noopener" data-act="'+(prev?'preview':'newtab')+'" title="'+esc(tip)+'">'+
      iconFor(e)+'<span class="fname">'+esc(e.name)+'</span></a>';
  var acts=e.isDir
    ?'<button class="btn" data-act="open">进入</button><button class="btn" data-act="copydir">复制路径</button>'
    :'<button class="btn" data-act="dl">下载</button><button class="btn" data-act="copy">复制链接</button>';
  return '<tr data-path="'+esc(p)+'" data-real="'+esc(real)+'" data-dir="'+(e.isDir?'1':'0')+'" data-ext="'+esc(e.ext)+'">'+
    '<td class="c-name" title="'+esc(tip)+'">'+nameCell+'</td>'+
    '<td class="c-size hide-sm">'+(e.isDir?'-':fmtSize(e.size))+'</td>'+
    '<td class="c-time hide-sm">'+fmtTime(e.modTime)+'</td>'+
    '<td class="c-act">'+acts+'</td></tr>';
}

function renderStats(){
  var rows=state.rows||[];
  if(state.mode==='roots'){
    $('statTop').textContent='共 '+rows.length+' 个展示目录';
    $('statBar').innerHTML='<b>'+rows.length+'</b> 个展示目录 <span class="dot">·</span> 点目录名进入下一级';
    return;
  }
  var files=0,total=0,dirs=0;
  for(var i=0;i<rows.length;i++){
    if(rows[i].isDir)dirs++;else{files++;total+=rows[i].size;}
  }
  $('statTop').textContent='当前目录 · '+files+' 个文件'+(dirs?' / '+dirs+' 个文件夹':'')+' · '+fmtSize(total);
  var parts=['文件 <b>'+files+'</b> 个'];
  if(dirs)parts.push('文件夹 <b>'+dirs+'</b> 个');
  parts.push('合计 <b>'+fmtSize(total)+'</b>');
  if(state.q)parts.push('（已筛选，原始 '+state.entries.length+' 项）');
  $('statBar').innerHTML=parts.join(' <span class="dot">·</span> ');
}

function renderNotice(){
  var el=$('noticeWrap');el.innerHTML='';
  if(state.notice){el.innerHTML='<div class="notice warn">'+esc(state.notice)+'</div>';return;}
  if(state.q&&!state.rows.length){
    el.innerHTML='<div class="notice warn">没有匹配「'+esc(state.q)+'」的文件。</div>';
  }
}

/* ---------------- 预览弹窗 ---------------- */
var pvEl=$('pv'),pvBody=$('pvBody'),pvTitle=$('pvTitle'),pvMeta=$('pvMeta');
var pvPrev=$('pvPrev'),pvNext=$('pvNext'),pvFull=$('pvFull');
var pvNote='';
var zoom={scale:1,tx:0,ty:0,img:null,wrap:null,dragging:false,sx:0,sy:0};

function pvListFromRows(){
  var out=[];
  for(var i=0;i<state.rows.length;i++){
    var e=state.rows[i];
    if(!e.isDir&&canPreview(e.ext))out.push(e);
  }
  return out;
}
function pvStopMedia(){
  var m=pvBody.querySelector('video,audio');
  if(m){try{m.pause();m.removeAttribute('src');m.load();}catch(e){}}
}
function pvResetZoom(){
  zoom.scale=1;zoom.tx=0;zoom.ty=0;zoom.img=null;zoom.wrap=null;zoom.dragging=false;
}
function closePreview(){
  pvStopMedia();
  pvBody.innerHTML='';
  pvResetZoom();
  pvEl.classList.remove('show');
  document.body.style.overflow='';
  state.current=null;
  if(document.fullscreenElement){try{document.exitFullscreen();}catch(e){}}
}
function fallbackHTML(msg,sub){
  return '<div class="pv-fallback"><div class="big">◌</div>'+esc(msg)+
    (sub?'<div class="sub">'+esc(sub)+'</div>':'')+'</div>';
}
function pvUpdateMeta(){
  var parts=[];
  if(state.pvList.length>1)parts.push((state.pvIndex+1)+' / '+state.pvList.length);
  if(zoom.img)parts.push(Math.round(zoom.scale*100)+'%');
  if(pvNote)parts.push(pvNote);
  pvMeta.textContent=parts.join(' · ');
}
function pvUpdateNav(){
  var show=state.pvList.length>1;
  pvPrev.style.display=show?'':'none';
  pvNext.style.display=show?'':'none';
}
function pvApplyZoom(){
  if(!zoom.img||!zoom.wrap)return;
  var wrap=zoom.wrap,img=zoom.img;
  var maxX=Math.max(0,(img.offsetWidth*zoom.scale-wrap.clientWidth)/2)+24;
  var maxY=Math.max(0,(img.offsetHeight*zoom.scale-wrap.clientHeight)/2)+24;
  if(zoom.tx>maxX)zoom.tx=maxX; if(zoom.tx<-maxX)zoom.tx=-maxX;
  if(zoom.ty>maxY)zoom.ty=maxY; if(zoom.ty<-maxY)zoom.ty=-maxY;
  img.style.transform='translate('+zoom.tx+'px,'+zoom.ty+'px) scale('+zoom.scale+')';
  pvUpdateMeta();
}
function pvBindZoom(img){
  var wrap=document.createElement('div');
  wrap.className='pv-zoom';
  wrap.appendChild(img);
  pvBody.innerHTML='';
  pvBody.appendChild(wrap);
  zoom.img=img;zoom.wrap=wrap;zoom.scale=1;zoom.tx=0;zoom.ty=0;zoom.dragging=false;

  wrap.addEventListener('wheel',function(ev){
    ev.preventDefault();
    var rect=wrap.getBoundingClientRect();
    var cx=ev.clientX-rect.left-rect.width/2;
    var cy=ev.clientY-rect.top-rect.height/2;
    var ns=zoom.scale*Math.exp(-ev.deltaY*0.0016);
    if(ns>20)ns=20; if(ns<0.1)ns=0.1;
    if(Math.abs(ns-zoom.scale)<1e-6)return;
    var k=ns/zoom.scale;
    zoom.tx=cx-(cx-zoom.tx)*k;
    zoom.ty=cy-(cy-zoom.ty)*k;
    zoom.scale=ns;
    pvApplyZoom();
  },{passive:false});

  wrap.addEventListener('dblclick',function(){
    if(zoom.scale>1.02){zoom.scale=1;zoom.tx=0;zoom.ty=0;}
    else{zoom.scale=2;zoom.tx=0;zoom.ty=0;}
    pvApplyZoom();
  });

  wrap.addEventListener('pointerdown',function(ev){
    if(ev.button!==0)return;
    zoom.dragging=true;
    zoom.sx=ev.clientX-zoom.tx;
    zoom.sy=ev.clientY-zoom.ty;
    try{wrap.setPointerCapture(ev.pointerId);}catch(e){}
    wrap.classList.add('grabbing');
  });
  wrap.addEventListener('pointermove',function(ev){
    if(!zoom.dragging)return;
    zoom.tx=ev.clientX-zoom.sx;
    zoom.ty=ev.clientY-zoom.sy;
    pvApplyZoom();
  });
  function endDrag(ev){
    if(!zoom.dragging)return;
    zoom.dragging=false;
    wrap.classList.remove('grabbing');
    try{wrap.releasePointerCapture(ev.pointerId);}catch(e){}
  }
  wrap.addEventListener('pointerup',endDrag);
  wrap.addEventListener('pointercancel',endDrag);
}
function pvGo(delta){
  var n=state.pvList.length;
  if(n<2)return;
  var i=state.pvIndex+delta;
  if(i<0)i=n-1;
  if(i>=n)i=0;
  pvOpenIndex(i);
}
function pvOpenIndex(i){
  var list=state.pvList;
  if(!list||!list.length)return;
  if(i<0)i=0;
  if(i>=list.length)i=list.length-1;
  state.pvIndex=i;

  var e=list[i];
  var path=e.path||e.name;
  var name=e.name||String(path).split('/').pop();
  var ext=String(e.ext||'').toLowerCase();
  if(!ext){var dot=name.lastIndexOf('.');if(dot>=0)ext=name.slice(dot+1).toLowerCase();}
  var url='/raw/'+encPath(path);

  pvStopMedia();
  pvBody.innerHTML='';
  pvResetZoom();
  pvNote='';
  state.current={path:path,url:url,name:name,ext:ext};

  pvTitle.textContent=name;
  pvTitle.title=path;
  pvEl.classList.add('show');
  document.body.style.overflow='hidden';
  pvUpdateNav();

  var kind=kindOf(ext);

  if(kind==='img'){
    pvNote='滚轮缩放 · 拖动平移 · 双击复位';
    var im=document.createElement('img');
    im.alt=name;
    im.onload=function(){pvApplyZoom();};
    im.onerror=function(){pvBody.innerHTML=fallbackHTML('图片加载失败','文件可能已被移动或删除');pvNote='';pvUpdateMeta();};
    im.src=url;
    pvBindZoom(im);
    pvUpdateMeta();
  }else if(kind==='video'||kind==='videoMaybe'){
    var v=document.createElement('video');
    v.src=url;v.controls=true;v.preload='metadata';
    v.onerror=function(){
      pvBody.innerHTML=fallbackHTML('浏览器无法播放该视频格式','可以点右上角「下载」后用本地播放器打开');
    };
    pvBody.appendChild(v);
    if(kind==='videoMaybe')pvNote='该格式可能不被浏览器支持';
    pvUpdateMeta();
  }else if(kind==='audio'){
    var a=document.createElement('audio');
    a.src=url;a.controls=true;a.preload='metadata';
    a.onerror=function(){pvBody.innerHTML=fallbackHTML('浏览器无法播放该音频格式','可以点右上角「下载」后用本地播放器打开');};
    pvBody.appendChild(a);
    pvUpdateMeta();
  }else if(kind==='pdf'){
    var f=document.createElement('iframe');
    f.src=url;f.title=name;
    pvBody.appendChild(f);
    pvUpdateMeta();
  }else if(kind==='text'){
    pvBody.innerHTML=fallbackHTML('正在加载 ...','');
    pvUpdateMeta();
    fetch(url).then(function(r){
      if(!r.ok)throw new Error('HTTP '+r.status);
      return r.text();
    }).then(function(txt){
      var pre=document.createElement('pre');
      pre.textContent=txt.length>600000?txt.slice(0,600000)+'\n\n... （文件过大，仅显示前 600KB）':txt;
      pvBody.innerHTML='';
      pvBody.appendChild(pre);
    }).catch(function(e){
      pvBody.innerHTML=fallbackHTML('文本读取失败',e.message);
    });
  }else{
    pvBody.innerHTML=fallbackHTML('该文件类型不支持在线预览','可以点右上角「下载」保存到本地');
    pvUpdateMeta();
  }
}

/* ---------------- 事件 ---------------- */
tb.addEventListener('click',function(ev){
  var t=ev.target.closest('[data-act]');if(!t)return;
  var tr=t.closest('tr');if(!tr)return;
  var p=tr.getAttribute('data-path'),isDir=tr.getAttribute('data-dir')==='1';
  var ext=tr.getAttribute('data-ext')||'',act=t.getAttribute('data-act');
  if(act==='open'){if(isDir)loadDir(p);else window.open('/raw/'+encPath(p),'_blank');}
  else if(act==='preview'){
    ev.preventDefault();
    var list=pvListFromRows();
    var idx=0;
    for(var i=0;i<list.length;i++){
      if((list[i].path||list[i].name)===p){idx=i;break;}
    }
    state.pvList=list;
    pvOpenIndex(idx);
  }
  else if(act==='newtab'){/* 由 <a target=_blank> 原生处理 */}
  else if(act==='dl'){window.location.href='/dl/'+encPath(p);}
  else if(act==='copy'){copyText(location.origin+'/raw/'+encPath(p));}
  else if(act==='copydir'){copyText(tr.getAttribute('data-real')||p);}
});

$('pvClose').addEventListener('click',closePreview);
pvPrev.addEventListener('click',function(){pvGo(-1);});
pvNext.addEventListener('click',function(){pvGo(1);});
pvEl.addEventListener('mousedown',function(ev){if(ev.target===pvEl)closePreview();});
$('pvDl').addEventListener('click',function(){
  if(state.current)window.location.href='/dl/'+encPath(state.current.path);
});
function pvToggleFull(){
  var box=document.querySelector('.pv-box');
  if(!box)return;
  if(document.fullscreenElement){document.exitFullscreen();return;}
  var fn=box.requestFullscreen||box.webkitRequestFullscreen||box.msRequestFullscreen;
  if(!fn)return;
  try{
    var pr=fn.call(box);
    if(pr&&pr.catch)pr.catch(function(){});
  }catch(e){}
}
pvFull.addEventListener('click',pvToggleFull);
document.addEventListener('fullscreenchange',function(){
  var on=!!document.fullscreenElement;
  pvFull.textContent=on?'⤡':'⛶';
  pvFull.title=on?'退出全屏':'全屏';
  if(zoom.img)setTimeout(pvApplyZoom,60);
});
document.addEventListener('keydown',function(ev){
  if(pvEl.classList.contains('show')){
    if(ev.key==='Escape'){closePreview();return;}
    if(ev.key==='ArrowLeft'){ev.preventDefault();pvGo(-1);return;}
    if(ev.key==='ArrowRight'){ev.preventDefault();pvGo(1);return;}
    return;
  }
  if(ev.key==='Escape'){
    if(document.activeElement===$('q')){$('q').value='';state.q='';computeRows();state.page=1;renderPage();renderStats();$('q').blur();}
    return;
  }
  if(ev.key==='/'&&document.activeElement!==$('q')){ev.preventDefault();$('q').focus();}
});

var qTimer=null;
$('q').addEventListener('input',function(){
  clearTimeout(qTimer);var v=this.value;
  qTimer=setTimeout(function(){
    state.q=v;computeRows();state.page=1;renderPage();renderStats();renderNotice();
  },140);
});

document.querySelectorAll('thead th.sortable').forEach(function(th){
  th.addEventListener('click',function(){
    var k=th.getAttribute('data-key');
    if(state.sortKey===k)state.sortDir=-state.sortDir;else{state.sortKey=k;state.sortDir=1;}
    updateArrows();computeRows();state.page=1;renderPage();renderStats();
  });
});
function updateArrows(){
  ['name','size','mtime'].forEach(function(k){
    document.getElementById('arw-'+k).textContent=(state.sortKey===k)?(state.sortDir>0?'▲':'▼'):'';
  });
}
$('refreshBtn').addEventListener('click',function(){reload();toast('已重新读取目录');});
$('themeBtn').addEventListener('click',toggleTheme);

/* ---------------- 启动 ---------------- */
initTheme();updateArrows();
var params=new URLSearchParams(location.search);
if(params.get('p'))state.path=params.get('p');

// 支持 ?preview=<相对路径> 直接打开预览弹窗（可用于分享单个文件的预览链接）
function openPreviewByPath(p){
  var name=String(p).split('/').pop();
  var dot=name.lastIndexOf('.');
  var ext=dot>=0?name.slice(dot+1).toLowerCase():'';
  var list=pvListFromRows();
  var idx=-1;
  for(var i=0;i<list.length;i++){
    if((list[i].path||list[i].name)===p){idx=i;break;}
  }
  if(idx<0){list=[{name:name,path:p,ext:ext}];idx=0;}
  state.pvList=list;
  pvOpenIndex(idx);
}

loadInfo().then(reload).then(function(){
  var pv=params.get('preview');
  if(pv)openPreviewByPath(pv);
});
})();
</script>
</body>
</html>
`

// ---------------------------------------------------------------- 设置页

const settingsHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{PAGE_TITLE}} · 设置</title>
<link rel="icon" href="{{FAVICON_URL}}">
<style>` + sharedCSS + `
.form{max-width:760px;margin:24px 0 0}
.row{padding:16px 20px;border-bottom:1px solid var(--border2);display:flex;gap:16px;align-items:flex-start}
.row:last-child{border-bottom:0}
.row>label{width:130px;flex:none;font-size:13.5px;font-weight:600;padding-top:8px}
.row .ctl{flex:1;min-width:0}
.row .hint{font-size:12px;color:var(--text3);margin-top:6px;line-height:1.7}
.row .hint code{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;color:var(--text2)}
.fav-row{display:flex;align-items:center;gap:10px}
.fav-row img{width:34px;height:34px;flex:none;border-radius:8px;border:1px solid var(--border);
  object-fit:contain;background:var(--panel2)}
.fav-row input[type=text]{flex:1;min-width:0}
input[type=text],input[type=number],textarea{width:100%;font:inherit;font-size:13.5px;color:var(--text);
  background:var(--panel2);border:1px solid var(--border);border-radius:8px;padding:8px 11px;outline:none;transition:.15s}
input[type=text]:focus,input[type=number]:focus,textarea:focus{border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-bg)}
textarea{resize:vertical;line-height:1.75;white-space:pre;overflow-wrap:normal;overflow-x:auto}
.mono{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:12.5px}
.row .switch{display:flex;align-items:center;gap:10px;padding-top:6px;cursor:pointer;user-select:none;
  width:auto;flex:1 1 auto;font-weight:400;font-size:13.5px}
.row .switch input{width:17px;height:17px;flex:none;accent-color:var(--accent);cursor:pointer}
.actions{padding:18px 20px;display:flex;gap:10px;align-items:center;background:var(--panel2)}
.kv{margin:24px 0 0;font-size:12.5px;color:var(--text2);line-height:2;word-break:break-all}
.kv code{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;color:var(--text)}
.sec{padding:11px 20px;background:var(--panel2);border-bottom:1px solid var(--border);
  font-size:12px;font-weight:600;color:var(--text2);letter-spacing:.4px}
</style>
</head>
<body>
<header>
  <div class="inner">
    <span class="logo">
      <img src="{{FAVICON_URL}}" alt="">
      {{SITE_NAME}}
      <span class="ver" id="verChip">v</span>
    </span>
    <span class="spacer"></span>
    <button class="btn icon" id="themeBtn" title="切换深色 / 浅色">◐</button>
    <a class="btn" href="/">← 返回文件列表</a>
  </div>
</header>

<div class="wrap">
  <div id="msg"></div>
  <div class="card form">
    <div class="sec">站点外观</div>
    <div class="row">
      <label for="siteName">左上角名称</label>
      <div class="ctl">
        <input type="text" id="siteName" placeholder="文件服务">
        <div class="hint">显示在网页左上角（图标右侧）。</div>
      </div>
    </div>
    <div class="row">
      <label for="pageTitle">浏览器标题</label>
      <div class="ctl">
        <input type="text" id="pageTitle" placeholder="留空 = 与左上角名称相同">
        <div class="hint">浏览器标签页上显示的文字。</div>
      </div>
    </div>
    <div class="row">
      <label for="footerText">底部介绍</label>
      <div class="ctl">
        <input type="text" id="footerText" placeholder="留空 = 不显示">
        <div class="hint">页面底部的一行说明文字。留空则不显示该行。</div>
      </div>
    </div>
    <div class="row">
      <label for="favicon">浏览器图标</label>
      <div class="ctl">
        <div class="fav-row">
          <img id="favPrev" src="{{FAVICON_URL}}" alt="">
          <input type="text" id="favicon" class="mono" placeholder="留空 = 内置图标" spellcheck="false">
          <button class="btn" id="favPick" type="button">选择图片</button>
          <input type="file" id="favFile" accept="image/*,.ico" hidden>
        </div>
        <div class="hint">
          可填：<b>图片文件路径</b>（相对路径按程序目录解析，支持 png / ico / jpg / gif / webp / svg）、
          <b>emoji 或 1-4 个字符</b>（自动生成纯色图标）、内联 <code>&lt;svg&gt;...&lt;/svg&gt;</code>、
          <code>http(s)://</code> 图片地址。<br>
          点「选择图片」可把小于 48 KB 的图片直接内联进 config.ini；更大的图片请把文件放到程序目录后填写文件名。
        </div>
      </div>
    </div>

    <div class="sec">服务</div>
    <div class="row">
      <label for="port">监听端口</label>
      <div class="ctl">
        <input type="number" id="port" min="1" max="65535" step="1">
        <div class="hint">修改后会立即重启服务。若端口被占用，程序会自动顺延到下一个可用端口。<br>
          当前实际监听端口：<b id="curPort">-</b></div>
      </div>
    </div>
    <div class="row">
      <label>启动行为</label>
      <div class="ctl">
        <label class="switch"><input type="checkbox" id="autoOpen"> 启动后自动打开浏览器</label>
      </div>
    </div>

    <div class="sec">展示目录</div>
    <div class="row">
      <label for="roots">目录列表</label>
      <div class="ctl">
        <textarea id="roots" class="mono" rows="5" spellcheck="false" placeholder="D:\共享资料&#10;E:\照片"></textarea>
        <div class="hint">
          一行一个目录（也支持用 <code>|</code> 分隔）。<br>
          · 只填 <b>一个</b> 目录 → 网页直接展示该目录下的文件；<br>
          · 填 <b>多个</b> 目录 → 网页先展示目录列表，再分别进入各自的下一级。<br>
          留空 = 不展示任何目录。
        </div>
      </div>
    </div>
    <div class="row">
      <label for="publishDir">发布目录</label>
      <div class="ctl">
        <input type="text" id="publishDir" class="mono" placeholder="留空表示 程序目录\published">
        <div class="hint">右键「发布」的文件会复制到这里，是<b>临时</b>存放点，会按下面的时长自动清理。<br>
          留空 = 程序目录下的 <code>published</code> 文件夹。为安全起见，这里不能填程序目录本身或盘符根目录。</div>
      </div>
    </div>
    <div class="row">
      <label for="publishTTL">保留时长</label>
      <div class="ctl">
        <input type="number" id="publishTTL" min="0" max="43200" step="10">
        <div class="hint">单位：分钟。发布目录里超过这个时长的文件会被自动删除；填 <code>0</code> 表示不自动清理。默认 60（1 小时）。</div>
      </div>
    </div>
    <div class="row">
      <label for="urlHost">对外 IP</label>
      <div class="ctl">
        <select id="urlHost" style="width:100%;font:inherit;font-size:13.5px;color:var(--text);background:var(--panel2);border:1px solid var(--border);border-radius:8px;padding:8px 11px;outline:none"></select>
        <div class="hint">生成分享链接时使用的本机 IP。局域网内其他人通过这个地址访问你的文件。</div>
      </div>
    </div>
    <div class="row">
      <label for="publicURL">对外地址</label>
      <div class="ctl">
        <input type="text" id="publicURL" class="mono" placeholder="留空 = http://本机IP:端口" spellcheck="false">
        <div class="hint">生成分享链接时使用的前缀，<b>填写后优先于上面的 IP</b>。<br>
          反向代理 / 域名访问时填这里，例：<code>https://files.example.com</code>、<code>http://nas.local:8080</code>。</div>
      </div>
    </div>

    <div class="sec">列表</div>
    <div class="row">
      <label for="pageSize">每页条数</label>
      <div class="ctl">
        <input type="number" id="pageSize" min="10" max="5000" step="10">
        <div class="hint">网页列表的分页大小。浏览页右下角也可以临时调整（仅对当前浏览器生效）。</div>
      </div>
    </div>
    <div class="row">
      <label>显示隐藏文件</label>
      <div class="ctl">
        <label class="switch"><input type="checkbox" id="showHidden"> 列举以「.」开头的文件与文件夹</label>
      </div>
    </div>
    <div class="row">
      <label for="maxFiles">列举上限</label>
      <div class="ctl">
        <input type="number" id="maxFiles" min="100" max="1000000" step="100">
        <div class="hint">全量列举接口的文件数量上限，防止超大目录拖慢接口。</div>
      </div>
    </div>

    <div class="sec">隐藏规则</div>
    <div class="row">
      <label for="hideTypes">隐藏文件类型</label>
      <div class="ctl">
        <input type="text" id="hideTypes" class="mono" placeholder="tmp, log, bak">
        <div class="hint">按<b>扩展名</b>隐藏，逗号分隔，不区分大小写，点号可省略。<br>
          例：<code>tmp, temp, log, bak, swp</code></div>
      </div>
    </div>
    <div class="row">
      <label for="hideRules">隐藏文件规则</label>
      <div class="ctl">
        <input type="text" id="hideRules" class="mono" placeholder="~$*, .git, node_modules">
        <div class="hint">按<b>文件名通配符</b>隐藏（支持 <code>*</code> 与 <code>?</code>），逗号分隔，不区分大小写。<br>
          例：<code>~$*, .git, .svn, node_modules, Thumbs.db</code></div>
      </div>
    </div>

    <div class="actions">
      <button class="btn primary" id="saveBtn">保存设置</button>
      <button class="btn" id="reloadBtn">重新载入</button>
      <span class="mini" id="saveHint"></span>
    </div>
  </div>

  <div class="kv">
    当前访问地址：<code id="curURL">-</code><br>
    展示目录：<code id="curRoots">-</code><br>
    发布目录：<code id="curPub">-</code><br>
    配置文件：<code id="curCfg">config.ini</code>
  </div>
</div>

<div id="toast"></div>

<script>
` + sharedJS + `
(function(){
'use strict';
var $=function(id){return document.getElementById(id);};
var info=null;

function showMsg(cls,text){
  $('msg').innerHTML=text?'<div class="notice '+cls+'">'+esc(text)+'</div>':'';
}
var favTimer=null;
function refreshFav(){
  if(favTimer)clearTimeout(favTimer);
  favTimer=setTimeout(function(){
    var v=($('favicon').value||'').trim();
    $('favPrev').src='/api/favicon?v='+encodeURIComponent(v)+'&t='+Date.now();
  },280);
}
function fill(d){
  $('port').value=d.port;
  $('roots').value=d.roots||'';
  $('publishDir').value=d.publishDir||'';
  $('publishTTL').value=(d.publishTTL===undefined||d.publishTTL===null)?60:d.publishTTL;
  $('pageSize').value=d.pageSize||100;
  $('showHidden').checked=!!d.showHidden;
  $('maxFiles').value=d.maxFiles;
  $('autoOpen').checked=!!d.autoOpen;
  $('hideTypes').value=d.hideTypes||'';
  $('hideRules').value=d.hideRules||'';
  var sel=$('urlHost');sel.innerHTML='';
  var o=document.createElement('option');o.value='';o.textContent='自动选择（推荐）';sel.appendChild(o);
  var ips=(info&&info.ips)||[];
  for(var i=0;i<ips.length;i++){
    var op=document.createElement('option');op.value=ips[i];op.textContent=ips[i];sel.appendChild(op);
  }
  sel.value=d.urlHost||'';
  if(sel.value!==(d.urlHost||'')){sel.value='';}
  $('siteName').value=d.siteName||'';
  $('pageTitle').value=d.pageTitle||'';
  $('footerText').value=d.footerText||'';
  $('favicon').value=d.favicon||'';
  $('publicURL').value=d.publicURL||'';
  refreshFav();
}
function load(){
  return Promise.all([
    fetch('/api/info').then(function(r){return r.json();}),
    fetch('/api/config').then(function(r){return r.json();})
  ]).then(function(res){
    info=res[0];
    $('verChip').textContent='v'+info.version;
    $('curURL').textContent=info.baseURL+'/';
    var rs=(info.roots||[]).map(function(x){return x.path;});
    $('curRoots').textContent=rs.length?rs.join('    |    '):'（未配置）';
    $('curPub').textContent=info.publishDir;
    $('curPort').textContent=info.port;
    $('curCfg').textContent=info.configPath||'config.ini';
    fill(res[1]);
  }).catch(function(e){showMsg('err','加载配置失败：'+e.message);});
}
function save(){
  var ttl=parseInt($('publishTTL').value,10);
  if(isNaN(ttl))ttl=60;
  var body={
    port:parseInt($('port').value,10)||8080,
    roots:$('roots').value,
    publishDir:$('publishDir').value.trim(),
    publishTTL:ttl,
    urlHost:$('urlHost').value,
    showHidden:$('showHidden').checked,
    pageSize:parseInt($('pageSize').value,10)||100,
    maxFiles:parseInt($('maxFiles').value,10)||20000,
    autoOpen:$('autoOpen').checked,
    hideTypes:$('hideTypes').value,
    hideRules:$('hideRules').value,
    siteName:$('siteName').value.trim(),
    pageTitle:$('pageTitle').value.trim(),
    footerText:$('footerText').value.trim(),
    favicon:$('favicon').value.trim(),
    publicURL:$('publicURL').value.trim()
  };
  $('saveBtn').disabled=true;$('saveHint').textContent='正在保存 ...';
  fetch('/api/config',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})
    .then(function(r){return r.json().then(function(d){return {ok:r.ok,d:d};});})
    .then(function(res){
      $('saveBtn').disabled=false;$('saveHint').textContent='';
      if(!res.ok){showMsg('err',res.d.error||'保存失败');return;}
      if(res.d.restarted){
        showMsg('ok','设置已保存，服务已切换到端口 '+res.d.port+'，3 秒后自动跳转 ...');
        var next=res.d.baseURL.replace(/^http:\/\//,'').split(':')[0];
        setTimeout(function(){location.href='http://'+next+':'+res.d.port+'/settings';},3000);
      }else{
        showMsg('ok','设置已保存，访问地址：'+res.d.baseURL+'/');
        setTimeout(function(){location.reload();},900);
      }
    })
    .catch(function(e){$('saveBtn').disabled=false;$('saveHint').textContent='';showMsg('err','保存失败：'+e.message);});
}
$('favicon').addEventListener('input',refreshFav);
$('favPick').addEventListener('click',function(){$('favFile').click();});
$('favFile').addEventListener('change',function(){
  var f=this.files&&this.files[0];
  this.value='';
  if(!f)return;
  if(f.size>48*1024){
    showMsg('warn','所选图片 '+Math.round(f.size/1024)+' KB，超过 48 KB 内联上限。请把图片放到程序目录，然后在「浏览器图标」里填写文件名，例如 favicon.png');
    return;
  }
  var fr=new FileReader();
  fr.onload=function(){$('favicon').value=fr.result;refreshFav();showMsg('ok','图片已载入，点「保存设置」后生效。');};
  fr.onerror=function(){showMsg('err','读取图片失败');};
  fr.readAsDataURL(f);
});
$('saveBtn').addEventListener('click',save);
$('reloadBtn').addEventListener('click',function(){showMsg('','');load();toast('已重新载入');});
$('themeBtn').addEventListener('click',toggleTheme);
document.addEventListener('keydown',function(ev){
  if((ev.ctrlKey||ev.metaKey)&&ev.key==='s'){ev.preventDefault();save();}
});
initTheme();load();
})();
</script>
</body>
</html>
`

// ---------------------------------------------------------------- 处理器

// renderPage 把页面模板里的占位符替换成当前配置。
// 站点名称 / 标题 / 底部文字都来自 config.ini 的 [site] 段。
func renderPage(tpl string) []byte {
	cfg := getConfig()
	r := strings.NewReplacer(
		"{{SITE_NAME}}", html.EscapeString(cfg.SiteName),
		"{{PAGE_TITLE}}", html.EscapeString(cfg.PageTitle),
		"{{FOOTER_TEXT}}", html.EscapeString(cfg.FooterText),
		"{{FAVICON_URL}}", faviconURL(cfg),
	)
	return []byte(r.Replace(tpl))
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(renderPage(pageHTML))
}

func serveSettings(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<h2>403</h2><p>设置页面仅允许在本机打开。</p>"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(renderPage(settingsHTML))
}
