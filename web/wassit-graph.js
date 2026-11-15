// ---- Wassit Agent Graph: Obsidian-style vault graph of the agent's skills + workflow simulator ----
// Standalone. Include with ONE line in index.html:  <script defer src="wassit-graph.js"></script>
// It injects its own CSS, adds a launcher card under the login card, and lazy-loads
// wassit-graph-data.js (same folder) the first time the graph opens. It never calls the backend:
// every workflow is replayed from the data file, so the showcase works with the agent offline.
(() => {
  'use strict';
  const SCRIPT_SRC = (document.currentScript && document.currentScript.src) || '';
  const DATA_SRC = SCRIPT_SRC ? SCRIPT_SRC.replace(/[^/]*$/, 'wassit-graph-data.js') : 'wassit-graph-data.js';
  const REDUCED = matchMedia('(prefers-reduced-motion: reduce)').matches;

  // ---------------------------------------------------------------- styles
  const CSS = `
.wzg-launch{display:flex;align-items:center;gap:16px;margin-top:14px;padding:14px 16px;border-radius:20px;border:1px solid var(--line,#638d8b66);
  background:radial-gradient(ellipse at 0% 50%,#7c5cff26,transparent 60%),linear-gradient(145deg,#1d2a33ee,#121c22ee);color:#e9eef6;
  box-shadow:0 14px 40px #040b0e50,inset 0 1px #ffffff14;position:relative;overflow:hidden;animation:wzg-in .45s ease-out both}
.wzg-launch::after{content:"";position:absolute;inset:0;background:linear-gradient(100deg,transparent 30%,#ffffff10 50%,transparent 70%);transform:translateX(-100%);animation:wzg-sheen 5s ease-in-out 1.2s infinite;pointer-events:none}
.wzg-launch canvas{flex:0 0 auto;width:64px;height:64px;border-radius:16px;background:#0e151b;border:1px solid #ffffff1c}
.wzg-launch div{flex:1 1 auto;min-width:0}
.wzg-launch strong{display:block;font-size:1rem;letter-spacing:-.01em}
.wzg-launch small{display:block;color:#a9b6c6;font-size:.82rem;line-height:1.4;margin-top:2px}
.wzg-launch button{flex:0 0 auto;min-height:44px;padding:9px 16px;border-radius:999px;border:1px solid #b794f680;background:#b794f6;color:#1a1230;font:inherit;font-weight:700;cursor:pointer;display:inline-flex;align-items:center;gap:8px;transition:transform .18s,background .18s}
.wzg-launch button:hover{transform:translateY(-1px);background:#cdb4ff}
.wzg-launch button:focus-visible{outline:2px solid #f4c98a;outline-offset:3px}
[data-theme=light] .wzg-launch{background:radial-gradient(ellipse at 0% 50%,#7c5cff1f,transparent 60%),#fff;color:#17343b;box-shadow:0 6px 22px #153c4012}
[data-theme=light] .wzg-launch small{color:#435b63}
@media(max-width:560px){.wzg-launch{flex-wrap:wrap}.wzg-launch button{width:100%;justify-content:center}}
@keyframes wzg-in{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}
@keyframes wzg-sheen{0%,60%{transform:translateX(-100%)}100%{transform:translateX(100%)}}

.wzg{--bg:#15171c;--panel:#1b1e24f2;--panel2:#22262e;--ink:#dcdde3;--muted:#8b8f9c;--line:#ffffff17;--accent:#b794f6;--accent-ink:#1a1230;
  position:fixed;inset:0;z-index:9999;display:grid;grid-template-columns:300px minmax(0,1fr) 340px;grid-template-rows:56px minmax(0,1fr) auto;
  background:var(--bg);color:var(--ink);font:14px/1.5 "DM Sans",system-ui,sans-serif;animation:wzg-fade .25s ease-out both}
.wzg[data-theme=light]{--bg:#f5f5f7;--panel:#fffffff2;--panel2:#eef0f4;--ink:#22252c;--muted:#5d6370;--line:#0000001a;--accent:#7c4ddb;--accent-ink:#fff}
@keyframes wzg-fade{from{opacity:0}to{opacity:1}}
.wzg *{box-sizing:border-box}
.wzg button,.wzg input,.wzg select{font:inherit;color:inherit}
.wzg button{cursor:pointer}
.wzg :focus-visible{outline:2px solid var(--accent);outline-offset:2px}
.wzg-top{grid-column:1/-1;display:flex;align-items:center;gap:12px;padding:0 14px;border-bottom:1px solid var(--line);background:var(--panel);min-width:0}
.wzg-title{display:flex;align-items:center;gap:10px;font-weight:700;white-space:nowrap;min-width:0}
.wzg-title span{color:var(--muted);font-weight:500;overflow:hidden;text-overflow:ellipsis}
.wzg-logo{width:28px;height:28px;border-radius:8px;display:grid;place-items:center;background:linear-gradient(140deg,#b794f6,#5b3fb5);color:#fff;font-size:15px;flex:none}
.wzg-spacer{flex:1}
.wzg-btn{min-height:36px;padding:6px 12px;border-radius:9px;border:1px solid var(--line);background:var(--panel2);display:inline-flex;align-items:center;gap:6px;white-space:nowrap;transition:border-color .15s,background .15s}
.wzg-btn:hover{border-color:var(--accent)}
.wzg-btn.primary{background:var(--accent);color:var(--accent-ink);border-color:var(--accent);font-weight:700}
.wzg-btn[aria-pressed=true]{border-color:var(--accent);background:color-mix(in srgb,var(--accent) 20%,transparent)}
.wzg-btn.icon{width:36px;padding:0;justify-content:center}
.wzg-side,.wzg-info{background:var(--panel);overflow-y:auto;min-height:0;padding:14px;display:grid;align-content:start;gap:16px}
.wzg-side{border-right:1px solid var(--line)}
.wzg-info{border-left:1px solid var(--line)}
.wzg h3{margin:0 0 8px;font-size:.72rem;letter-spacing:.12em;text-transform:uppercase;color:var(--muted);font-weight:700}
.wzg-search{width:100%;min-height:38px;border-radius:9px;border:1px solid var(--line);background:var(--panel2);padding:7px 11px}
.wzg-groups{display:flex;flex-wrap:wrap;gap:6px}
.wzg-group{display:inline-flex;align-items:center;gap:6px;padding:4px 10px;border-radius:99px;border:1px solid var(--line);background:transparent;font-size:.8rem}
.wzg-group i{width:9px;height:9px;border-radius:50%;flex:none}
.wzg-group[aria-pressed=false]{opacity:.4;text-decoration:line-through}
.wzg-group b{color:var(--muted);font-weight:500}
.wzg-wfs{display:grid;gap:8px}
.wzg-wf{text-align:start;padding:10px 12px;border-radius:12px;border:1px solid var(--line);background:var(--panel2);display:grid;gap:2px;transition:border-color .15s,transform .15s}
.wzg-wf:hover{border-color:var(--accent);transform:translateX(2px)}
.wzg-wf[aria-current=true]{border-color:var(--accent);box-shadow:inset 3px 0 0 var(--accent)}
.wzg-wf strong{font-size:.92rem}
.wzg-wf small{color:var(--muted);font-size:.78rem;line-height:1.35}
.wzg-wf em{font-style:normal;font-size:.7rem;color:var(--accent);font-weight:700;letter-spacing:.05em}
.wzg-stage{position:relative;min-width:0;min-height:0;overflow:hidden}
.wzg-stage canvas{position:absolute;inset:0;width:100%;height:100%;display:block;touch-action:none;cursor:grab}
.wzg-stage canvas.drag{cursor:grabbing}
.wzg-stage canvas.hover{cursor:pointer}
.wzg-hud{position:absolute;left:12px;top:12px;display:flex;gap:6px;flex-wrap:wrap;pointer-events:none}
.wzg-hud>*{pointer-events:auto}
.wzg-zoom{position:absolute;right:12px;top:12px;display:grid;gap:6px}
.wzg-caption{position:absolute;left:50%;bottom:16px;transform:translateX(-50%);max-width:min(640px,calc(100% - 24px));padding:10px 14px;border-radius:14px;background:var(--panel);border:1px solid var(--line);box-shadow:0 12px 30px #0006;text-align:center;font-size:.9rem;pointer-events:none;transition:opacity .3s}
.wzg-caption:empty{opacity:0}
.wzg-caption b{color:var(--accent)}
.wzg-tip{position:absolute;pointer-events:none;padding:6px 10px;border-radius:9px;background:var(--panel);border:1px solid var(--line);font-size:.8rem;max-width:260px;box-shadow:0 8px 22px #0005;display:none;z-index:2}
.wzg-tip small{display:block;color:var(--muted)}
.wzg-bar{grid-column:1/-1;display:flex;align-items:center;gap:10px;padding:10px 14px;border-top:1px solid var(--line);background:var(--panel);flex-wrap:wrap}
.wzg-bar label{display:inline-flex;align-items:center;gap:6px;color:var(--muted);font-size:.82rem}
.wzg-progress{flex:1 1 220px;display:flex;gap:3px;align-items:center;min-width:160px}
.wzg-progress button{flex:1;height:10px;border-radius:5px;border:0;background:var(--panel2);padding:0;position:relative}
.wzg-progress button.done{background:color-mix(in srgb,var(--accent) 55%,transparent)}
.wzg-progress button.now{background:var(--accent);box-shadow:0 0 0 3px color-mix(in srgb,var(--accent) 30%,transparent)}
.wzg-progress button:hover{outline:1px solid var(--accent)}
.wzg-step-count{font-variant-numeric:tabular-nums;color:var(--muted);font-size:.82rem;min-width:62px}
.wzg-node-head{display:flex;align-items:center;gap:10px}
.wzg-node-head i{width:14px;height:14px;border-radius:50%;flex:none;box-shadow:0 0 12px currentColor}
.wzg-node-head strong{font-size:1.1rem;line-height:1.25}
.wzg-pill{display:inline-flex;align-items:center;padding:2px 8px;border-radius:99px;font-size:.68rem;font-weight:700;letter-spacing:.06em;text-transform:uppercase;border:1px solid currentColor;white-space:nowrap}
.wzg-desc{margin:8px 0 0;color:var(--ink);opacity:.9}
.wzg-file{display:block;margin-top:8px;font:12px/1.4 ui-monospace,Consolas,monospace;color:var(--muted);overflow-wrap:anywhere}
.wzg-tags{display:flex;flex-wrap:wrap;gap:5px;margin-top:8px}
.wzg-tags span{font-size:.72rem;padding:1px 7px;border-radius:6px;background:var(--panel2);color:var(--muted)}
.wzg-links{display:grid;gap:4px}
.wzg-links button{display:flex;align-items:center;gap:8px;text-align:start;padding:5px 8px;border-radius:8px;border:1px solid transparent;background:transparent;font-size:.85rem}
.wzg-links button:hover{border-color:var(--line);background:var(--panel2)}
.wzg-links button i{width:8px;height:8px;border-radius:50%;flex:none}
.wzg-links button small{margin-inline-start:auto;color:var(--muted);font-size:.7rem}
.wzg-log{display:grid;gap:10px}
.wzg-entry{display:grid;grid-template-columns:26px minmax(0,1fr);gap:10px;animation:wzg-in .3s ease-out both}
.wzg-entry>b{width:26px;height:26px;border-radius:50%;display:grid;place-items:center;font-size:.75rem;background:var(--panel2);border:1px solid var(--line)}
.wzg-entry.now>b{background:var(--accent);color:var(--accent-ink);border-color:var(--accent)}
.wzg-entry div{padding:9px 11px;border-radius:12px;border:1px solid var(--line);background:var(--panel2)}
.wzg-entry.now div{border-color:var(--accent)}
.wzg-entry strong{display:block;font-size:.9rem}
.wzg-entry p{margin:3px 0 0;color:var(--muted);font-size:.82rem;line-height:1.45;overflow-wrap:anywhere}
.wzg-entry .row{display:flex;align-items:center;gap:6px;flex-wrap:wrap;margin-top:6px;font-size:.72rem;color:var(--muted)}
.wzg-empty{color:var(--muted);font-size:.85rem;padding:12px;border:1px dashed var(--line);border-radius:12px}
.wzg-stats{display:grid;grid-template-columns:repeat(3,1fr);gap:6px}
.wzg-stats div{padding:8px;border-radius:10px;background:var(--panel2);text-align:center}
.wzg-stats strong{display:block;font-size:1.2rem;line-height:1.1}
.wzg-stats small{color:var(--muted);font-size:.7rem}
.wzg-keys{color:var(--muted);font-size:.76rem;line-height:1.7}
.wzg-keys kbd{font:11px ui-monospace,Consolas,monospace;padding:1px 5px;border-radius:4px;border:1px solid var(--line);background:var(--panel2)}
.wzg-tabs{display:flex;gap:4px;padding:3px;border-radius:10px;background:var(--panel2)}
.wzg-tabs button{flex:1;min-height:30px;border:0;border-radius:8px;background:transparent;font-size:.82rem;font-weight:600;color:var(--muted)}
.wzg-tabs button[aria-selected=true]{background:var(--panel);color:var(--ink);box-shadow:0 1px 4px #0003}
.wzg-mobile{display:none}
@media(max-width:1100px){.wzg{grid-template-columns:260px minmax(0,1fr) 300px}}
@media(max-width:860px){
  .wzg{grid-template-columns:minmax(0,1fr);grid-template-rows:52px minmax(0,1fr) auto}
  .wzg-mobile{display:inline-flex}
  .wzg-title span{display:none}
  .wzg-side,.wzg-info{position:fixed;top:52px;bottom:0;width:min(340px,88vw);z-index:3;transition:transform .25s ease;box-shadow:0 0 40px #0008}
  .wzg-side{left:0;transform:translateX(-105%)} .wzg-info{right:0;transform:translateX(105%)}
  .wzg.show-side .wzg-side,.wzg.show-info .wzg-info{transform:none}
  .wzg-stage{grid-row:2} .wzg-bar{grid-row:3}
  .wzg-hide-sm{display:none}
}
@media(prefers-reduced-motion:reduce){.wzg,.wzg *,.wzg-launch,.wzg-launch::after{animation:none!important;transition:none!important}}
`;

  // ---------------------------------------------------------------- palette
  const GROUPS = {
    core: { label: 'Agent core', dark: '#b794f6', light: '#7c4ddb' },
    skill: { label: 'Skills', dark: '#6fd3a4', light: '#1f8a5a' },
    tool: { label: 'Tools', dark: '#8ec5ff', light: '#1d64b5' },
    endpoint: { label: 'API endpoints', dark: '#f0c060', light: '#9a6a00' },
    rule: { label: 'Rules & guardrails', dark: '#f08a80', light: '#c0392b' },
    actor: { label: 'People', dark: '#f9a8d4', light: '#b83280' },
    data: { label: 'Data', dark: '#5eead4', light: '#0f8a7a' },
    workflow: { label: 'Workflows', dark: '#fdba74', light: '#c2610c' }
  };
  const RING = { core: 0, skill: 170, rule: 270, tool: 360, endpoint: 470, data: 560, actor: 560, workflow: 650 };
  const DECIDERS = {
    rules: ['Rules', '#8ec5ff', '#1d5fa8'], human: ['Human', '#f0c060', '#8a5a00'], pharmacy: ['Pharmacy', '#d9a86c', '#6b4a1f'],
    client: ['Client', '#6fd3a4', '#1f7a4d'], wassit: ['Wassit', '#b794f6', '#7c4ddb']
  };
  const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const isLight = () => document.documentElement.dataset.theme === 'light';
  const gcol = g => (GROUPS[g] || GROUPS.skill)[isLight() ? 'light' : 'dark'];
  const dpill = d => { const v = DECIDERS[d] || [d, '#999', '#555']; return '<span class="wzg-pill" style="color:' + (isLight() ? v[2] : v[1]) + '">' + esc(v[0]) + '</span>'; };

  function injectCSS() {
    if (document.getElementById('wzg-style')) return;
    const st = document.createElement('style');
    st.id = 'wzg-style';
    st.textContent = CSS;
    document.head.appendChild(st);
  }

  // ---------------------------------------------------------------- data loading
  let dataPromise = null;
  function loadData() {
    if (window.WZ_GRAPH_DATA) return Promise.resolve(window.WZ_GRAPH_DATA);
    if (dataPromise) return dataPromise;
    dataPromise = new Promise((resolve, reject) => {
      const s = document.createElement('script');
      s.src = DATA_SRC;
      s.onload = () => window.WZ_GRAPH_DATA ? resolve(window.WZ_GRAPH_DATA) : (dataPromise = null, reject(new Error('wassit-graph-data.js loaded but defined no WZ_GRAPH_DATA')));
      s.onerror = () => { dataPromise = null; reject(new Error('Could not load ' + DATA_SRC)); };
      document.head.appendChild(s);
    });
    return dataPromise;
  }

  // ---------------------------------------------------------------- spider drawing (shared)
  function drawSpider(ctx, x, y, angle, size, phase, color) {
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(angle);
    ctx.strokeStyle = color;
    ctx.lineWidth = Math.max(1, size * 0.13);
    ctx.lineCap = 'round';
    for (let side = -1; side <= 1; side += 2) {
      for (let i = 0; i < 4; i++) {
        const base = (-0.9 + i * 0.6);
        const wig = Math.sin(phase + i * 1.7 + (side > 0 ? Math.PI : 0)) * 0.28;
        const a = base + wig;
        const kx = Math.cos(a) * size * 0.9, ky = side * (Math.abs(Math.sin(a)) * size * 0.5 + size * 0.55);
        const fx = Math.cos(a) * size * 1.6, fy = side * size * 1.35;
        ctx.beginPath();
        ctx.moveTo(size * 0.1, side * size * 0.15);
        ctx.quadraticCurveTo(kx, ky * 1.2, fx, fy);
        ctx.stroke();
      }
    }
    ctx.fillStyle = color;
    ctx.beginPath(); ctx.ellipse(-size * 0.55, 0, size * 0.75, size * 0.6, 0, 0, Math.PI * 2); ctx.fill();
    ctx.beginPath(); ctx.ellipse(size * 0.35, 0, size * 0.38, size * 0.34, 0, 0, Math.PI * 2); ctx.fill();
    ctx.fillStyle = '#ff6b6b';
    ctx.beginPath(); ctx.arc(size * 0.58, -size * 0.12, size * 0.08, 0, Math.PI * 2); ctx.arc(size * 0.58, size * 0.12, size * 0.08, 0, Math.PI * 2); ctx.fill();
    ctx.restore();
  }

  // ---------------------------------------------------------------- launcher
  const launchers = new Set();
  function launcherHTML() {
    return '<canvas width="128" height="128" aria-hidden="true"></canvas>' +
      '<div><strong>Explore Wassit\'s mind</strong><small>An interactive graph of every skill, tool, rule and endpoint the agent uses. Replay any workflow from A to Z without running the agent.</small></div>' +
      '<button type="button" data-wzg-open>Open agent graph <span aria-hidden="true">↗</span></button>';
  }
  function animateLauncher(cv) {
    const ctx = cv.getContext('2d');
    let t = 0;
    const tick = () => {
      if (!cv.isConnected) { launchers.delete(cv); return; }
      t += 0.016;
      ctx.clearRect(0, 0, 128, 128);
      ctx.save(); ctx.translate(64, 64);
      ctx.strokeStyle = '#b794f655'; ctx.lineWidth = 1;
      for (let i = 0; i < 10; i++) { const a = i / 10 * Math.PI * 2; ctx.beginPath(); ctx.moveTo(0, 0); ctx.lineTo(Math.cos(a) * 62, Math.sin(a) * 62); ctx.stroke(); }
      for (let r = 12; r <= 56; r += 11) {
        ctx.beginPath();
        for (let i = 0; i <= 10; i++) { const a = i / 10 * Math.PI * 2; const rr = r - Math.sin(i * 2.3) * 1.5; i ? ctx.lineTo(Math.cos(a) * rr, Math.sin(a) * rr) : ctx.moveTo(Math.cos(a) * rr, Math.sin(a) * rr); }
        ctx.stroke();
      }
      const nodes = [0, 2, 4, 7].map((k, i) => { const a = k / 10 * Math.PI * 2; const r = 23 + (i % 2) * 22; return [Math.cos(a) * r, Math.sin(a) * r]; });
      Object.values(GROUPS).slice(1, 5).forEach((g, i) => { ctx.fillStyle = g.dark; ctx.beginPath(); ctx.arc(nodes[i][0], nodes[i][1], 3.4, 0, 7); ctx.fill(); });
      ctx.fillStyle = '#b794f6'; ctx.beginPath(); ctx.arc(0, 0, 5, 0, 7); ctx.fill();
      const a = t * 0.7, r = 34 + Math.sin(t * 1.3) * 10;
      drawSpider(ctx, Math.cos(a) * r, Math.sin(a) * r, a + Math.PI / 2, 5.5, t * 12, '#e9e2ff');
      ctx.restore();
      if (!REDUCED) requestAnimationFrame(tick);
    };
    tick();
  }
  function mountLaunchers() {
    const hosts = document.querySelectorAll('main.entry-account .access-card, main .entry-doors');
    hosts.forEach(host => {
      const next = host.nextElementSibling;
      if (next && next.classList.contains('wzg-launch')) return;
      const el = document.createElement('section');
      el.className = 'wzg-launch';
      el.setAttribute('aria-label', 'Wassit agent graph showcase');
      el.innerHTML = launcherHTML();
      host.after(el);
      const cv = el.querySelector('canvas');
      launchers.add(cv);
      animateLauncher(cv);
    });
  }

  // ---------------------------------------------------------------- the graph app
  let app = null;
  let opening = false;

  function open(opts = {}) {
    injectCSS();
    if (app) { if (opts.workflow) app.loadWorkflow(opts.workflow); return; }
    if (opening) return;
    opening = true;
    const shell = document.createElement('div');
    shell.className = 'wzg';
    shell.setAttribute('role', 'dialog');
    shell.setAttribute('aria-modal', 'true');
    shell.setAttribute('aria-label', 'Wassit agent graph');
    shell.dataset.theme = isLight() ? 'light' : 'dark';
    shell.innerHTML = '<div class="wzg-top"><div class="wzg-title"><span class="wzg-logo">✦</span>Wassit agent graph</div><div class="wzg-spacer"></div><div class="wzg-empty" style="border:0">Loading the vault…</div></div>';
    document.body.appendChild(shell);
    const prevOverflow = document.documentElement.style.overflow;
    document.documentElement.style.overflow = 'hidden';
    const opener = document.activeElement;
    loadData().then(data => {
      opening = false;
      app = new GraphApp(shell, data, () => {
        document.documentElement.style.overflow = prevOverflow;
        app = null;
        opener && opener.focus && opener.focus();
      });
      if (opts.workflow) app.loadWorkflow(opts.workflow);
    }).catch(err => {
      opening = false;
      shell.innerHTML = '<div class="wzg-top"><div class="wzg-title"><span class="wzg-logo">✦</span>Wassit agent graph</div><div class="wzg-spacer"></div><button class="wzg-btn" data-x>Close</button></div><div class="wzg-empty" style="grid-column:1/-1;margin:20px">' + esc(err.message) + '</div>';
      shell.querySelector('[data-x]').onclick = () => { shell.remove(); document.documentElement.style.overflow = prevOverflow; };
    });
  }

  class GraphApp {
    constructor(root, data, onClose) {
      this.root = root;
      this.onClose = onClose;
      this.prepare(data);
      this.cam = { x: 0, y: 0, k: 0.8 };
      this.camTarget = null;
      this.hover = null;
      this.selected = null;
      this.hiddenGroups = new Set();
      this.query = '';
      this.layout = 'web';
      this.alpha = 1;
      this.trails = [];
      this.ripples = [];
      this.ambient = !REDUCED;
      this.speed = 1;
      this.sim = null;
      this.tour = false;
      this.spider = { x: 0, y: 0, angle: 0, node: this.byId.get('wassit') || this.nodes[0], path: [], seg: 0, t: 0, phase: 0, wait: 1 };
      this.render();
      this.bind();
      this.resize();
      if (this.spider.node) { this.spider.x = this.spider.node.x; this.spider.y = this.spider.node.y; }
      for (let i = 0; i < 260; i++) this.physics(true);
      this.fit(true);
      this.last = performance.now();
      this.frame = this.frame.bind(this);
      this.raf = requestAnimationFrame(this.frame);
      this.selectNode(this.byId.get('wassit') || null, false);
    }

    // ---- data
    prepare(data) {
      this.data = data;
      this.nodes = data.nodes.map(n => ({ ...n, x: 0, y: 0, vx: 0, vy: 0, deg: 0, fx: null, fy: null, heat: 0 }));
      this.byId = new Map(this.nodes.map(n => [n.id, n]));
      this.links = data.links.filter(l => this.byId.has(l.source) && this.byId.has(l.target)).map(l => ({ ...l, s: this.byId.get(l.source), t: this.byId.get(l.target), heat: 0 }));
      this.adj = new Map(this.nodes.map(n => [n, []]));
      this.links.forEach(l => { l.s.deg++; l.t.deg++; this.adj.get(l.s).push([l.t, l]); this.adj.get(l.t).push([l.s, l]); });
      this.workflows = (data.workflows || []).map(w => ({ ...w, steps: (w.steps || []).filter(s => this.byId.has(s.node)) }));
      // nodes that are workflows link to every step node implicitly for "used in" lookups
      this.usedIn = new Map();
      this.workflows.forEach(w => w.steps.forEach(s => {
        const arr = this.usedIn.get(s.node) || [];
        if (!arr.includes(w)) arr.push(w);
        this.usedIn.set(s.node, arr);
      }));
      const groupsOrder = Object.keys(RING);
      const byGroup = {};
      this.nodes.forEach(n => (byGroup[n.group] = byGroup[n.group] || []).push(n));
      Object.entries(byGroup).forEach(([g, list]) => {
        const off = groupsOrder.indexOf(g) * 0.7;
        list.forEach((n, i) => {
          const a = off + i / list.length * Math.PI * 2;
          const r = (RING[g] ?? 400) + (Math.random() - 0.5) * 40;
          n.x = Math.cos(a) * r; n.y = Math.sin(a) * r;
          n.r = n.group === 'core' ? 16 : 4.5 + Math.sqrt(n.deg) * 1.7;
        });
      });
    }

    // ---- DOM
    render() {
      const counts = {};
      this.nodes.forEach(n => counts[n.group] = (counts[n.group] || 0) + 1);
      const totalSteps = this.workflows.reduce((a, w) => a + w.steps.length, 0);
      this.root.innerHTML =
        '<header class="wzg-top">' +
          '<button class="wzg-btn icon wzg-mobile" data-act="side" aria-label="Show menu">☰</button>' +
          '<div class="wzg-title"><span class="wzg-logo" aria-hidden="true">✦</span>Wassit agent graph<span>· skills, tools, rules and workflows</span></div>' +
          '<div class="wzg-spacer"></div>' +
          '<div class="wzg-tabs wzg-hide-sm" role="tablist" aria-label="Layout">' +
            '<button role="tab" data-layout="web" aria-selected="true">Spider web</button>' +
            '<button role="tab" data-layout="vault" aria-selected="false">Vault</button>' +
          '</div>' +
          '<button class="wzg-btn wzg-hide-sm" data-act="ambient" aria-pressed="' + this.ambient + '" title="Let the agent crawl the web while idle">🕷 Idle crawl</button>' +
          '<button class="wzg-btn wzg-hide-sm" data-act="png" title="Save a PNG of the graph">Snapshot</button>' +
          '<button class="wzg-btn icon wzg-mobile" data-act="info" aria-label="Show details">ⓘ</button>' +
          '<button class="wzg-btn" data-act="close" aria-label="Close graph">✕ <span class="wzg-hide-sm">Close</span></button>' +
        '</header>' +
        '<aside class="wzg-side">' +
          '<div><h3>Search the vault</h3><input class="wzg-search" type="search" placeholder="Search a skill, tool, rule… ( / )" aria-label="Search nodes"></div>' +
          '<div class="wzg-stats"><div><strong>' + this.nodes.length + '</strong><small>nodes</small></div><div><strong>' + this.links.length + '</strong><small>links</small></div><div><strong>' + this.workflows.length + '</strong><small>workflows</small></div></div>' +
          '<div><h3>Groups</h3><div class="wzg-groups">' + Object.entries(GROUPS).filter(([g]) => counts[g]).map(([g, v]) =>
            '<button class="wzg-group" data-group="' + g + '" aria-pressed="true"><i style="background:' + gcol(g) + '"></i>' + esc(v.label) + ' <b>' + counts[g] + '</b></button>').join('') + '</div></div>' +
          '<div><h3>Simulate a workflow</h3><div class="wzg-wfs">' +
            '<button class="wzg-wf" data-tour><em>GUIDED TOUR · ' + totalSteps + ' STEPS</em><strong>A → Z: play every workflow</strong><small>The agent crawls through each workflow in order. Nothing calls the backend.</small></button>' +
            this.workflows.map((w, i) => '<button class="wzg-wf" data-wf="' + esc(w.id) + '"><em>' + String.fromCharCode(65 + i) + ' · ' + w.steps.length + ' STEPS</em><strong>' + esc(w.name) + '</strong><small>' + esc(w.summary || '') + '</small></button>').join('') +
          '</div></div>' +
          '<div class="wzg-keys"><h3>Keys</h3><kbd>Space</kbd> play/pause · <kbd>←</kbd><kbd>→</kbd> step · <kbd>F</kbd> fit · <kbd>/</kbd> search · <kbd>Esc</kbd> close<br>Drag to pan, scroll to zoom, drag a node to move it, click to inspect.</div>' +
        '</aside>' +
        '<div class="wzg-stage"><canvas aria-label="Graph of the Wassit agent. Use the menu to inspect nodes and simulate workflows." role="img"></canvas>' +
          '<div class="wzg-zoom"><button class="wzg-btn icon" data-act="zin" aria-label="Zoom in">+</button><button class="wzg-btn icon" data-act="zout" aria-label="Zoom out">−</button><button class="wzg-btn icon" data-act="fit" aria-label="Fit graph">⤢</button></div>' +
          '<div class="wzg-tip"></div><div class="wzg-caption" aria-live="polite"></div></div>' +
        '<aside class="wzg-info"><div class="wzg-tabs" role="tablist"><button role="tab" data-tab="node" aria-selected="true">Node</button><button role="tab" data-tab="log" aria-selected="false">Simulation log</button></div><div class="wzg-panel"></div></aside>' +
        '<footer class="wzg-bar">' +
          '<button class="wzg-btn" data-act="prev" aria-label="Previous step">⏮</button>' +
          '<button class="wzg-btn primary" data-act="play" aria-label="Play">▶ Play</button>' +
          '<button class="wzg-btn" data-act="next" aria-label="Next step">⏭</button>' +
          '<button class="wzg-btn" data-act="reset" aria-label="Reset simulation">↺</button>' +
          '<span class="wzg-step-count">—</span>' +
          '<div class="wzg-progress" aria-label="Workflow steps"></div>' +
          '<label>Speed <select data-speed><option value="0.5">0.5×</option><option value="1" selected>1×</option><option value="2">2×</option><option value="4">4×</option></select></label>' +
          '<label class="wzg-hide-sm"><input type="checkbox" data-follow checked> Follow agent</label>' +
        '</footer>';
      this.canvas = this.root.querySelector('canvas');
      this.ctx = this.canvas.getContext('2d');
      this.tip = this.root.querySelector('.wzg-tip');
      this.caption = this.root.querySelector('.wzg-caption');
      this.panel = this.root.querySelector('.wzg-panel');
      this.progress = this.root.querySelector('.wzg-progress');
      this.stepCount = this.root.querySelector('.wzg-step-count');
      this.playBtn = this.root.querySelector('[data-act=play]');
      this.follow = true;
      this.tab = 'node';
      this.root.querySelector('[data-act=close]').focus();
    }

    bind() {
      const r = this.root;
      r.addEventListener('click', e => {
        const t = e.target.closest('button');
        if (!t || !r.contains(t)) return;
        if (t.dataset.act) this.action(t.dataset.act);
        else if (t.dataset.layout) this.setLayout(t.dataset.layout);
        else if (t.dataset.group) {
          const g = t.dataset.group;
          this.hiddenGroups.has(g) ? this.hiddenGroups.delete(g) : this.hiddenGroups.add(g);
          t.setAttribute('aria-pressed', String(!this.hiddenGroups.has(g)));
          this.alpha = Math.max(this.alpha, 0.4);
        } else if (t.dataset.wf) { this.tour = false; this.loadWorkflow(t.dataset.wf); this.play(); r.classList.remove('show-side'); }
        else if (t.hasAttribute('data-tour')) { this.tour = true; this.loadWorkflow(this.workflows[0]?.id); this.play(); r.classList.remove('show-side'); }
        else if (t.dataset.node) { const n = this.byId.get(t.dataset.node); if (n) { this.selectNode(n, true); } }
        else if (t.dataset.tab) this.setTab(t.dataset.tab);
        else if (t.dataset.jump != null) this.jumpTo(Number(t.dataset.jump));
      });
      r.querySelector('.wzg-search').addEventListener('input', e => { this.query = e.target.value.trim().toLowerCase(); });
      r.querySelector('.wzg-search').addEventListener('keydown', e => {
        if (e.key === 'Enter') { const m = this.nodes.find(n => this.matches(n)); if (m) this.selectNode(m, true); }
      });
      r.querySelector('[data-speed]').addEventListener('change', e => { this.speed = Number(e.target.value); });
      r.querySelector('[data-follow]').addEventListener('change', e => { this.follow = e.target.checked; });
      this.onKey = e => {
        if (!this.root.isConnected) return;
        const typing = /INPUT|SELECT|TEXTAREA/.test(document.activeElement?.tagName || '');
        if (e.key === 'Escape') { if (typing) document.activeElement.blur(); else this.close(); e.preventDefault(); return; }
        if (typing) return;
        if (e.key === ' ') { e.preventDefault(); this.togglePlay(); }
        else if (e.key === 'ArrowRight') this.action('next');
        else if (e.key === 'ArrowLeft') this.action('prev');
        else if (e.key === 'f' || e.key === 'F') this.fit();
        else if (e.key === '/') { e.preventDefault(); this.root.querySelector('.wzg-search').focus(); }
      };
      document.addEventListener('keydown', this.onKey);
      this.onResize = () => this.resize();
      addEventListener('resize', this.onResize);
      this.themeObs = new MutationObserver(() => { this.root.dataset.theme = isLight() ? 'light' : 'dark'; this.render2(); });
      this.themeObs.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });

      // pointer: pan, zoom, drag nodes, pinch
      const cv = this.canvas;
      const pts = new Map();
      let drag = null, pinch = null, moved = 0;
      cv.addEventListener('pointerdown', e => {
        cv.setPointerCapture(e.pointerId);
        pts.set(e.pointerId, [e.offsetX, e.offsetY]);
        moved = 0;
        if (pts.size === 2) { const [a, b] = [...pts.values()]; pinch = { d: Math.hypot(a[0] - b[0], a[1] - b[1]) || 1, k: this.cam.k }; if (drag && drag.node) drag.node.fx = drag.node.fy = null; drag = null; return; }
        const n = this.pick(e.offsetX, e.offsetY);
        drag = n ? { node: n } : { pan: [e.offsetX, e.offsetY, this.cam.x, this.cam.y] };
        this.camTarget = null;
        cv.classList.add('drag');
      });
      cv.addEventListener('pointermove', e => {
        const p = [e.offsetX, e.offsetY];
        if (pts.has(e.pointerId)) { const o = pts.get(e.pointerId); moved += Math.abs(o[0] - p[0]) + Math.abs(o[1] - p[1]); pts.set(e.pointerId, p); }
        if (pinch && pts.size === 2) { const [a, b] = [...pts.values()]; this.cam.k = clamp(pinch.k * Math.hypot(a[0] - b[0], a[1] - b[1]) / pinch.d, 0.15, 4); return; }
        if (drag && drag.node && moved > 3) { const w = this.toWorld(...p); drag.node.fx = w[0]; drag.node.fy = w[1]; this.alpha = Math.max(this.alpha, 0.35); }
        else if (drag && drag.pan) { this.cam.x = drag.pan[2] - (p[0] - drag.pan[0]) / this.cam.k; this.cam.y = drag.pan[3] - (p[1] - drag.pan[1]) / this.cam.k; }
        else if (!drag) this.setHover(this.pick(...p), p);
      });
      const up = e => {
        pts.delete(e.pointerId);
        if (pts.size < 2) pinch = null;
        if (drag && drag.node) { if (moved <= 3) this.selectNode(drag.node, false); drag.node.fx = drag.node.fy = null; }
        else if (drag && drag.pan && moved <= 3) this.selectNode(null, false);
        drag = null;
        cv.classList.remove('drag');
      };
      cv.addEventListener('pointerup', up);
      cv.addEventListener('pointercancel', up);
      cv.addEventListener('pointerleave', () => this.setHover(null));
      cv.addEventListener('wheel', e => {
        e.preventDefault();
        const before = this.toWorld(e.offsetX, e.offsetY);
        this.cam.k = clamp(this.cam.k * Math.exp(-e.deltaY * 0.0015), 0.15, 4);
        const after = this.toWorld(e.offsetX, e.offsetY);
        this.cam.x += before[0] - after[0]; this.cam.y += before[1] - after[1];
        this.camTarget = null;
      }, { passive: false });
    }

    render2() { // re-colour legend on theme change
      this.root.querySelectorAll('.wzg-group').forEach(b => b.querySelector('i').style.background = gcol(b.dataset.group));
      this.renderPanel();
    }

    action(a) {
      switch (a) {
        case 'close': return this.close();
        case 'side': this.root.classList.toggle('show-side'); this.root.classList.remove('show-info'); return;
        case 'info': this.root.classList.toggle('show-info'); this.root.classList.remove('show-side'); return;
        case 'zin': this.zoomBy(1.3); return;
        case 'zout': this.zoomBy(1 / 1.3); return;
        case 'fit': this.fit(); return;
        case 'ambient': this.ambient = !this.ambient; this.root.querySelector('[data-act=ambient]').setAttribute('aria-pressed', String(this.ambient)); return;
        case 'png': return this.snapshot();
        case 'play': return this.togglePlay();
        case 'next': if (!this.sim) this.loadWorkflow(this.workflows[0]?.id); if (!this.sim) return; this.pause(); return this.jumpTo(this.sim.i + 1);
        case 'prev': if (!this.sim) return; this.pause(); return this.jumpTo(this.sim.i - 1);
        case 'reset': return this.resetSim();
      }
    }

    close() {
      cancelAnimationFrame(this.raf);
      clearTimeout(this.tourTimer); clearTimeout(this.fitTimer);
      document.removeEventListener('keydown', this.onKey);
      removeEventListener('resize', this.onResize);
      this.themeObs.disconnect();
      this.root.remove();
      this.onClose();
    }

    setLayout(l) {
      this.layout = l;
      this.root.querySelectorAll('[data-layout]').forEach(b => b.setAttribute('aria-selected', String(b.dataset.layout === l)));
      this.alpha = 1;
      clearTimeout(this.fitTimer); this.fitTimer = setTimeout(() => this.fit(), 900);
    }

    setTab(t) {
      this.tab = t;
      this.root.querySelectorAll('[data-tab]').forEach(b => b.setAttribute('aria-selected', String(b.dataset.tab === t)));
      this.renderPanel();
    }

    // ---- geometry
    resize() {
      const dpr = Math.min(devicePixelRatio || 1, 2);
      const rect = this.canvas.getBoundingClientRect();
      this.W = Math.max(1, rect.width); this.H = Math.max(1, rect.height);
      this.canvas.width = this.W * dpr; this.canvas.height = this.H * dpr;
      this.dpr = dpr;
    }
    toWorld(sx, sy) { return [(sx - this.W / 2) / this.cam.k + this.cam.x, (sy - this.H / 2) / this.cam.k + this.cam.y]; }
    toScreen(wx, wy) { return [(wx - this.cam.x) * this.cam.k + this.W / 2, (wy - this.cam.y) * this.cam.k + this.H / 2]; }
    visible(n) { return !this.hiddenGroups.has(n.group); }
    pick(sx, sy) {
      const [wx, wy] = this.toWorld(sx, sy);
      let best = null, bd = Infinity;
      for (const n of this.nodes) {
        if (!this.visible(n)) continue;
        const d = Math.hypot(n.x - wx, n.y - wy);
        const lim = n.r + 6 / this.cam.k;
        if (d < lim && d < bd) { bd = d; best = n; }
      }
      return best;
    }
    zoomBy(f) { this.camTarget = { x: this.cam.x, y: this.cam.y, k: clamp(this.cam.k * f, 0.15, 4) }; }
    fit(instant) {
      const vis = this.nodes.filter(n => this.visible(n));
      if (!vis.length) return;
      let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
      vis.forEach(n => { x0 = Math.min(x0, n.x); y0 = Math.min(y0, n.y); x1 = Math.max(x1, n.x); y1 = Math.max(y1, n.y); });
      const k = clamp(Math.min(this.W / (x1 - x0 + 160), this.H / (y1 - y0 + 160)), 0.15, 2);
      const t = { x: (x0 + x1) / 2, y: (y0 + y1) / 2, k };
      if (instant) Object.assign(this.cam, t); else this.camTarget = t;
    }
    focus(n, k) { this.camTarget = { x: n.x, y: n.y, k: k || Math.max(this.cam.k, 1.1) }; }

    // ---- physics (Barnes-Hut is overkill at <200 nodes; plain O(n²))
    physics(warm) {
      const a = warm ? 1 : this.alpha;
      if (a < 0.004) return;
      const N = this.nodes;
      const web = this.layout === 'web';
      for (let i = 0; i < N.length; i++) {
        const p = N[i];
        for (let j = i + 1; j < N.length; j++) {
          const q = N[j];
          let dx = q.x - p.x, dy = q.y - p.y; let d2 = dx * dx + dy * dy;
          if (d2 < 0.01) { dx = Math.random() - 0.5; dy = Math.random() - 0.5; d2 = 0.5; }
          if (d2 > 250000) continue;
          const d = Math.sqrt(d2);
          let f = (web ? 1500 : 2600) / d2;
          const min = p.r + q.r + 10;
          if (d < min) f += (min - d) * 0.25;
          const fx = dx / d * f * a, fy = dy / d * f * a;
          p.vx -= fx; p.vy -= fy; q.vx += fx; q.vy += fy;
        }
      }
      for (const l of this.links) {
        const p = l.s, q = l.t;
        const dx = q.x - p.x, dy = q.y - p.y, d = Math.hypot(dx, dy) || 1;
        const len = (p.group === 'core' || q.group === 'core') ? 150 : 70;
        const k = (web ? 0.012 : 0.04) * a * (d - len) / d;
        const div = Math.min(p.deg, q.deg) > 6 ? 1.6 : 1;
        p.vx += dx * k / div; p.vy += dy * k / div; q.vx -= dx * k / div; q.vy -= dy * k / div;
      }
      for (const n of N) {
        if (n.group === 'core') { n.vx += -n.x * 0.08 * a; n.vy += -n.y * 0.08 * a; }
        else if (web) {
          const r = Math.hypot(n.x, n.y) || 1, target = RING[n.group] ?? 400;
          const f = (target - r) * 0.06 * a;
          n.vx += n.x / r * f; n.vy += n.y / r * f;
        } else { n.vx -= n.x * 0.003 * a; n.vy -= n.y * 0.003 * a; }
        if (n.fx != null) { n.x = n.fx; n.y = n.fy; n.vx = n.vy = 0; continue; }
        n.vx *= 0.6; n.vy *= 0.6;
        n.x += n.vx; n.y += n.vy;
      }
      if (!warm) this.alpha *= 0.992;
    }

    // ---- interaction state
    setHover(n, p) {
      this.hover = n;
      this.canvas.classList.toggle('hover', !!n);
      if (!n) { this.tip.style.display = 'none'; return; }
      this.tip.innerHTML = '<strong>' + esc(n.label) + '</strong><small>' + esc((GROUPS[n.group] || {}).label || n.group) + ' · ' + n.deg + ' links</small>';
      this.tip.style.display = 'block';
      const x = Math.min(p[0] + 14, this.W - 270), y = Math.min(p[1] + 14, this.H - 60);
      this.tip.style.left = x + 'px'; this.tip.style.top = y + 'px';
    }
    selectNode(n, move) {
      this.selected = n;
      if (n && move) this.focus(n);
      if (this.tab !== 'node' && n && !this.sim?.playing) this.setTab('node'); else this.renderPanel();
      if (n && move && innerWidth <= 860) this.root.classList.add('show-info');
    }
    matches(n) {
      if (!this.query) return false;
      return (n.label + ' ' + n.id + ' ' + (n.desc || '') + ' ' + (n.tags || []).join(' ') + ' ' + (n.file || '')).toLowerCase().includes(this.query);
    }

    renderPanel() {
      if (this.tab === 'log') return this.renderLog();
      const n = this.selected;
      if (!n) { this.panel.innerHTML = '<p class="wzg-empty">Click any node to see what it does, where it lives in the code, and which workflows pass through it.</p>'; return; }
      const nb = this.adj.get(n) || [];
      const wfs = this.usedIn.get(n.id) || [];
      this.panel.innerHTML =
        '<div style="margin-top:14px"><div class="wzg-node-head"><i style="color:' + gcol(n.group) + ';background:' + gcol(n.group) + '"></i><strong>' + esc(n.label) + '</strong></div>' +
        '<div style="margin-top:8px"><span class="wzg-pill" style="color:' + gcol(n.group) + '">' + esc((GROUPS[n.group] || {}).label || n.group) + '</span></div>' +
        (n.desc ? '<p class="wzg-desc">' + esc(n.desc) + '</p>' : '') +
        (n.file ? '<code class="wzg-file">' + esc(n.file) + '</code>' : '') +
        ((n.tags || []).length ? '<div class="wzg-tags">' + n.tags.map(t => '<span>#' + esc(t) + '</span>').join('') + '</div>' : '') + '</div>' +
        (wfs.length ? '<div><h3 style="margin-top:18px">In workflows (' + wfs.length + ')</h3><div class="wzg-wfs">' + wfs.map(w => '<button class="wzg-wf" data-wf="' + esc(w.id) + '"><strong>' + esc(w.name) + '</strong><small>▶ Simulate</small></button>').join('') + '</div></div>' : '') +
        '<div><h3 style="margin-top:18px">Linked nodes (' + nb.length + ')</h3><div class="wzg-links">' + nb.map(([m, l]) =>
          '<button data-node="' + esc(m.id) + '"><i style="background:' + gcol(m.group) + '"></i>' + esc(m.label) + '<small>' + esc(l.s === n ? l.kind + ' →' : '← ' + l.kind) + '</small></button>').join('') + '</div></div>';
    }

    renderLog() {
      const s = this.sim;
      if (!s) { this.panel.innerHTML = '<p class="wzg-empty" style="margin-top:14px">Pick a workflow on the left. The agent crawls to each step and this log shows what happened and who decided: the rules, a human, the pharmacy, the client, or Wassit.</p>'; return; }
      const done = s.wf.steps.slice(0, s.i + 1);
      this.panel.innerHTML = '<div style="margin-top:14px"><h3>' + esc(s.wf.name) + '</h3><p style="margin:0 0 12px;color:var(--muted);font-size:.85rem">' + esc(s.wf.summary || '') + '</p>' +
        '<div class="wzg-log">' + (done.length ? done.map((st, i) => {
          const n = this.byId.get(st.node);
          return '<div class="wzg-entry' + (i === s.i ? ' now' : '') + '"><b>' + (i + 1) + '</b><div><strong>' + esc(st.label) + '</strong>' +
            (st.detail ? '<p>' + esc(st.detail) + '</p>' : '') +
            '<div class="row">' + dpill(st.decider || 'wassit') + '<button class="wzg-btn" style="min-height:24px;padding:1px 8px;font-size:.72rem" data-node="' + esc(st.node) + '">' + esc(n ? n.label : st.node) + '</button></div></div></div>';
        }).join('') : '<p class="wzg-empty">Press play. The agent starts at its core and crawls to step 1.</p>') + '</div></div>';
      const last = this.panel.querySelector('.wzg-entry.now');
      last && last.scrollIntoView({ block: 'nearest', behavior: REDUCED ? 'auto' : 'smooth' });
    }

    renderProgress() {
      const s = this.sim;
      if (!s) { this.progress.innerHTML = ''; this.stepCount.textContent = '—'; return; }
      this.progress.innerHTML = s.wf.steps.map((st, i) => '<button data-jump="' + i + '" class="' + (i < s.i ? 'done' : i === s.i ? 'now' : '') + '" title="' + (i + 1) + '. ' + esc(st.label) + '" aria-label="Step ' + (i + 1) + ': ' + esc(st.label) + '"></button>').join('');
      this.stepCount.textContent = 'Step ' + Math.max(0, s.i + 1) + ' / ' + s.wf.steps.length;
      this.playBtn.innerHTML = s.playing ? '❚❚ Pause' : (s.i >= s.wf.steps.length - 1 && s.i >= 0 ? '↺ Replay' : '▶ Play');
      this.root.querySelectorAll('.wzg-wf[data-wf]').forEach(b => b.setAttribute('aria-current', String(b.dataset.wf === s.wf.id && !!b.closest('.wzg-side'))));
    }

    // ---- simulation
    loadWorkflow(id) {
      const wf = this.workflows.find(w => w.id === id) || this.workflows[0];
      if (!wf) return;
      clearTimeout(this.tourTimer);
      this.sim = { wf, i: -1, playing: false, dwell: 0, arrived: true };
      this.trails = [];
      this.nodes.forEach(n => n.heat = 0);
      this.links.forEach(l => l.heat = 0);
      this.moveSpiderTo(this.byId.get('wassit') || wf.steps[0] && this.byId.get(wf.steps[0].node), true);
      this.caption.innerHTML = '<b>' + esc(wf.name) + '</b> · ' + esc(wf.summary || '') + ' · press ▶';
      this.setTab('log');
      this.renderProgress();
    }
    resetSim() { if (this.sim) this.loadWorkflow(this.sim.wf.id); }
    play() {
      if (!this.sim) this.loadWorkflow(this.workflows[0]?.id);
      const s = this.sim; if (!s) return;
      if (s.i >= s.wf.steps.length - 1) { this.loadWorkflow(s.wf.id); }
      this.sim.playing = true;
      if (this.sim.arrived) this.advance();
      this.renderProgress();
    }
    pause() { if (this.sim) { this.sim.playing = false; this.renderProgress(); } }
    togglePlay() { this.sim && this.sim.playing ? this.pause() : this.play(); }
    advance() {
      const s = this.sim;
      if (s.i + 1 >= s.wf.steps.length) {
        s.playing = false;
        if (this.tour) {
          const idx = this.workflows.indexOf(s.wf);
          if (idx + 1 < this.workflows.length) { this.tourTimer = setTimeout(() => { if (this.tour && this.root.isConnected) { this.loadWorkflow(this.workflows[idx + 1].id); this.play(); } }, 1600 / this.speed); }
          else this.tour = false;
        }
        this.caption.innerHTML = '<b>Done.</b> ' + esc(s.wf.name) + ' finished in ' + s.wf.steps.length + ' steps.' + (this.tour ? ' Next workflow…' : '');
        this.renderProgress();
        return;
      }
      const target = this.byId.get(s.wf.steps[s.i + 1].node);
      s.arrived = false;
      this.crawlTo(target, () => { s.i++; s.arrived = true; s.dwell = 0; this.arrive(s.i); });
    }
    arrive(i) {
      const s = this.sim, st = s.wf.steps[i], n = this.byId.get(st.node);
      n.heat = 1;
      this.ripples.push({ n, t: 0, color: gcol(n.group) });
      this.caption.innerHTML = '<b>' + (i + 1) + '. ' + esc(st.label) + '</b> — ' + esc(st.detail || n.label);
      if (this.tab === 'log') this.renderLog(); else this.setTab('log');
      this.renderProgress();
    }
    jumpTo(i) {
      const s = this.sim; if (!s) return;
      i = clamp(i, -1, s.wf.steps.length - 1);
      this.trails = []; this.nodes.forEach(n => n.heat = 0); this.links.forEach(l => l.heat = 0);
      let prev = this.byId.get('wassit');
      for (let k = 0; k <= i; k++) {
        const n = this.byId.get(s.wf.steps[k].node);
        const path = this.route(prev, n);
        for (let m = 1; m < path.length; m++) this.markTrail(path[m - 1], path[m]);
        n.heat = k === i ? 1 : 0.5; prev = n;
      }
      s.i = i; s.arrived = true; s.dwell = 0;
      this.moveSpiderTo(prev, true);
      if (i >= 0) this.arrive(i); else { this.renderProgress(); this.renderLog(); }
    }

    route(a, b) { // BFS shortest path over visible + hidden nodes (the web is one piece)
      if (!a || !b || a === b) return [a || b];
      const prev = new Map([[a, null]]);
      const q = [a];
      while (q.length) {
        const n = q.shift();
        if (n === b) break;
        for (const [m] of this.adj.get(n)) if (!prev.has(m)) { prev.set(m, n); q.push(m); }
      }
      if (!prev.has(b)) return [a, b];
      const path = [];
      for (let n = b; n; n = prev.get(n)) path.unshift(n);
      return path;
    }
    linkBetween(a, b) { return (this.adj.get(a) || []).find(([m]) => m === b)?.[1]; }
    markTrail(a, b) {
      const l = this.linkBetween(a, b);
      if (l) l.heat = 1;
      this.trails.push({ a, b, born: performance.now(), sim: !!this.sim });
      if (this.trails.length > 400) this.trails.shift();
    }
    crawlTo(target, done) {
      const sp = this.spider;
      sp.path = this.route(sp.node, target);
      sp.seg = 0; sp.t = 0; sp.done = done;
      if (sp.path.length < 2) { sp.path = []; done && done(); }
    }
    moveSpiderTo(n, instant) {
      const sp = this.spider;
      if (!n) return;
      sp.node = n; sp.path = []; sp.done = null;
      if (instant) { sp.x = n.x; sp.y = n.y; }
    }

    // ---- loop
    frame(now) {
      if (!this.root.isConnected) return;
      const dt = clamp((now - this.last) / 1000, 0, 0.05);
      this.last = now;
      this.physics(false);
      this.step(dt, now);
      if (this.camTarget) {
        const c = this.cam, t = this.camTarget, e = REDUCED ? 1 : 1 - Math.pow(0.001, dt);
        c.x += (t.x - c.x) * e; c.y += (t.y - c.y) * e; c.k += (t.k - c.k) * e;
        if (Math.abs(t.x - c.x) < 0.3 && Math.abs(t.k - c.k) < 0.002) this.camTarget = null;
      }
      this.draw(now);
      this.raf = requestAnimationFrame(this.frame);
    }

    step(dt, now) {
      const sp = this.spider;
      sp.phase += dt * 14 * (sp.path.length ? 1 : 0.15);
      if (sp.path.length > 1) {
        const a = sp.path[sp.seg], b = sp.path[sp.seg + 1];
        const len = Math.hypot(b.x - a.x, b.y - a.y) || 1;
        sp.t += dt * 320 * this.speed / len * (this.sim ? 1 : 0.5);
        if (sp.t >= 1) {
          this.markTrail(a, b);
          sp.seg++; sp.t = 0; sp.node = b;
          if (sp.seg >= sp.path.length - 1) { sp.path = []; sp.x = b.x; sp.y = b.y; const d = sp.done; sp.done = null; d && d(); }
        }
        if (sp.path.length > 1) {
          const a2 = sp.path[sp.seg], b2 = sp.path[sp.seg + 1];
          const tx = a2.x + (b2.x - a2.x) * sp.t, ty = a2.y + (b2.y - a2.y) * sp.t;
          sp.angle = Math.atan2(b2.y - a2.y, b2.x - a2.x);
          sp.x = tx; sp.y = ty;
        }
      } else if (sp.node) { sp.x = sp.node.x; sp.y = sp.node.y; }

      if (this.follow && this.sim && (sp.path.length > 1 || this.sim.playing)) {
        const c = this.cam;
        const [sx, sy] = this.toScreen(sp.x, sp.y);
        if (sx < this.W * 0.2 || sx > this.W * 0.8 || sy < this.H * 0.2 || sy > this.H * 0.8) this.camTarget = { x: sp.x, y: sp.y, k: Math.max(c.k, 0.9) };
      }

      const s = this.sim;
      if (s && s.playing && s.arrived) {
        s.dwell += dt * this.speed;
        if (s.dwell > 1.8) this.advance();
      } else if (!s && this.ambient && !sp.path.length) {
        sp.wait -= dt;
        if (sp.wait <= 0) {
          const opts = (this.adj.get(sp.node) || []).filter(([m]) => this.visible(m));
          if (opts.length) { const [m] = opts[Math.floor(Math.random() * opts.length)]; this.crawlTo(m, () => { sp.wait = 0.4 + Math.random() * 1.2; m.heat = Math.max(m.heat, 0.6); }); }
        }
      }
      this.nodes.forEach(n => { if (!s) n.heat *= Math.pow(0.4, dt); });
      this.links.forEach(l => { if (!s) l.heat *= Math.pow(0.5, dt); });
      this.ripples = this.ripples.filter(r => (r.t += dt) < 1.4);
    }

    draw(now) {
      const ctx = this.ctx, light = isLight();
      const c = this.cam;
      ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
      ctx.fillStyle = light ? '#f5f5f7' : '#15171c';
      ctx.fillRect(0, 0, this.W, this.H);
      ctx.save();
      ctx.translate(this.W / 2, this.H / 2);
      ctx.scale(c.k, c.k);
      ctx.translate(-c.x, -c.y);
      const core = this.byId.get('wassit') || { x: 0, y: 0 };
      const thin = 1 / c.k;

      // spider-web backdrop
      const webAlpha = this.layout === 'web' ? (light ? 0.14 : 0.1) : (light ? 0.05 : 0.04);
      ctx.strokeStyle = light ? `rgba(60,40,120,${webAlpha})` : `rgba(200,190,255,${webAlpha})`;
      ctx.lineWidth = thin;
      const spokes = 24, maxR = 760;
      for (let i = 0; i < spokes; i++) { const a = i / spokes * Math.PI * 2; ctx.beginPath(); ctx.moveTo(core.x, core.y); ctx.lineTo(core.x + Math.cos(a) * maxR, core.y + Math.sin(a) * maxR); ctx.stroke(); }
      for (let r = 40; r < maxR; r += 36) {
        ctx.beginPath();
        for (let i = 0; i <= spokes; i++) {
          const a = i / spokes * Math.PI * 2, a0 = (i - 0.5) / spokes * Math.PI * 2;
          const x = core.x + Math.cos(a) * r, y = core.y + Math.sin(a) * r;
          if (!i) ctx.moveTo(x, y);
          else ctx.quadraticCurveTo(core.x + Math.cos(a0) * r * 0.96, core.y + Math.sin(a0) * r * 0.96, x, y);
        }
        ctx.stroke();
      }
      if (this.layout === 'web' && c.k > 0.35) {
        ctx.font = `600 ${11 * thin}px DM Sans, system-ui, sans-serif`;
        ctx.fillStyle = light ? 'rgba(60,40,120,.35)' : 'rgba(200,190,255,.28)';
        ctx.textAlign = 'center';
        const seen = new Set();
        Object.entries(RING).forEach(([g, r]) => { if (!r || seen.has(r) || this.hiddenGroups.has(g)) return; seen.add(r); ctx.fillText(g === 'data' ? 'DATA · PEOPLE' : (GROUPS[g].label).toUpperCase(), core.x, core.y - r - 14); });
      }

      // focus set
      const focus = this.hover || this.selected;
      const labelFocus = this.hover || (this.selected && this.selected.group !== "core" ? this.selected : null);
      const near = new Set();
      if (labelFocus) { near.add(labelFocus); (this.adj.get(labelFocus) || []).forEach(([m]) => near.add(m)); }
      const searching = !!this.query;
      const dimmed = n => (searching && !this.matches(n)) || (this.hover && !near.has(n));

      // links
      for (const l of this.links) {
        if (!this.visible(l.s) || !this.visible(l.t)) continue;
        const hot = focus && (l.s === focus || l.t === focus);
        const dim = dimmed(l.s) && dimmed(l.t) || (this.hover && !hot);
        ctx.strokeStyle = hot ? gcol(focus.group) : light ? `rgba(30,30,40,${dim ? 0.05 : 0.16})` : `rgba(255,255,255,${dim ? 0.03 : 0.11})`;
        ctx.lineWidth = (hot ? 1.6 : 1) * thin;
        ctx.beginPath(); ctx.moveTo(l.s.x, l.s.y); ctx.lineTo(l.t.x, l.t.y); ctx.stroke();
      }

      // silk trails
      for (const tr of this.trails) {
        const age = (now - tr.born) / 1000;
        const alpha = tr.sim ? Math.max(0.35, 1 - age * 0.15) : Math.max(0, 1 - age * 0.5);
        if (alpha <= 0) continue;
        ctx.strokeStyle = light ? `rgba(124,77,219,${alpha * 0.8})` : `rgba(233,226,255,${alpha * 0.8})`;
        ctx.lineWidth = 2.4 * thin;
        ctx.shadowColor = light ? '#7c4ddb' : '#cdb4ff'; ctx.shadowBlur = 10;
        ctx.setLineDash([6 * thin, 4 * thin]); ctx.lineDashOffset = -now / 40 * thin;
        ctx.beginPath(); ctx.moveTo(tr.a.x, tr.a.y); ctx.lineTo(tr.b.x, tr.b.y); ctx.stroke();
      }
      ctx.setLineDash([]); ctx.shadowBlur = 0;
      this.trails = this.trails.filter(tr => tr.sim || now - tr.born < 2200);

      // silk thread from current node to spider
      const sp = this.spider;
      if (sp.path.length > 1) {
        ctx.strokeStyle = light ? 'rgba(124,77,219,.9)' : 'rgba(240,235,255,.9)';
        ctx.lineWidth = 1.4 * thin;
        ctx.beginPath(); ctx.moveTo(sp.path[sp.seg].x, sp.path[sp.seg].y); ctx.lineTo(sp.x, sp.y); ctx.stroke();
      }

      // ripples
      for (const r of this.ripples) {
        ctx.strokeStyle = r.color; ctx.globalAlpha = 1 - r.t / 1.4; ctx.lineWidth = 2 * thin;
        ctx.beginPath(); ctx.arc(r.n.x, r.n.y, r.n.r + r.t * 50, 0, Math.PI * 2); ctx.stroke();
      }
      ctx.globalAlpha = 1;

      // nodes
      const activeStep = this.sim && this.sim.i >= 0 ? this.byId.get(this.sim.wf.steps[this.sim.i].node) : null;
      for (const n of this.nodes) {
        if (!this.visible(n)) continue;
        const col = gcol(n.group);
        const dim = dimmed(n);
        ctx.globalAlpha = dim ? 0.18 : 1;
        if (n.heat > 0.05 || n === focus || n === activeStep || (searching && this.matches(n))) {
          ctx.shadowColor = col; ctx.shadowBlur = 18 * Math.max(n.heat, 0.6);
        }
        ctx.fillStyle = col;
        ctx.beginPath(); ctx.arc(n.x, n.y, n.r * (1 + n.heat * 0.35), 0, Math.PI * 2); ctx.fill();
        ctx.shadowBlur = 0;
        if (n === this.selected || n === activeStep) {
          ctx.strokeStyle = light ? '#22252c' : '#fff'; ctx.lineWidth = 2 * thin;
          ctx.beginPath(); ctx.arc(n.x, n.y, n.r + 4 * thin + 2, 0, Math.PI * 2); ctx.stroke();
        }
        if (n.group === 'core') {
          ctx.fillStyle = light ? '#fff' : '#1a1230';
          ctx.font = `700 ${n.r * 0.9}px DM Sans, system-ui`; ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
          ctx.fillText('W', n.x, n.y + 1); ctx.textBaseline = 'alphabetic';
        }
      }
      ctx.globalAlpha = 1;

      // labels (Obsidian fades them in with zoom)
      ctx.textAlign = 'center';
      const fs = 12 * thin;
      ctx.font = `500 ${fs}px DM Sans, system-ui, sans-serif`;
      for (const n of this.nodes) {
        if (!this.visible(n)) continue;
        const important = n.group === 'core' || near.has(n) || n === activeStep || n.heat > 0.4 || (searching && this.matches(n));
        let a = important ? 1 : clamp((c.k - (this.sim ? 1.35 : 1.0)) / 0.4, 0, 1);
        if (dimmed(n) && !important) a *= 0.15;
        if (a <= 0.02) continue;
        ctx.globalAlpha = a;
        ctx.fillStyle = light ? '#22252c' : '#dcdde3';
        if (important) { ctx.font = `700 ${fs * 1.05}px DM Sans, system-ui, sans-serif`; }
        ctx.fillText(n.label, n.x, n.y + n.r + 14 * thin);
        if (important) ctx.font = `500 ${fs}px DM Sans, system-ui, sans-serif`;
      }
      ctx.globalAlpha = 1;

      // the agent
      const size = Math.max(7, 9 / Math.sqrt(c.k));
      drawSpider(ctx, sp.x, sp.y - (sp.path.length || !sp.node ? 0 : sp.node.r + size * 0.4), sp.path.length ? sp.angle : -Math.PI / 2, size, sp.phase, light ? '#3b2a6b' : '#f2edff');
      ctx.restore();
    }

    snapshot() {
      try {
        const a = document.createElement('a');
        a.download = 'wassit-agent-graph.png';
        a.href = this.canvas.toDataURL('image/png');
        a.click();
      } catch (e) { this.caption.textContent = 'Snapshot failed: ' + e.message; }
    }
  }

  function clamp(v, a, b) { return Math.max(a, Math.min(b, v)); }

  // ---------------------------------------------------------------- boot
  function boot() {
    injectCSS();
    mountLaunchers();
    const target = document.getElementById('app') || document.body;
    let queued = false;
    new MutationObserver(() => {
      if (queued) return;
      queued = true;
      requestAnimationFrame(() => { queued = false; mountLaunchers(); });
    }).observe(target, { childList: true, subtree: true });
    document.addEventListener('click', e => {
      const b = e.target.closest('[data-wzg-open]');
      if (!b) return;
      e.preventDefault();
      open({ workflow: b.dataset.wzgOpen || undefined });
    });
    if (/[?&]graph\b/.test(location.search)) open();
  }
  window.WassitGraph = { open, mount: mountLaunchers };
  document.readyState === 'loading' ? document.addEventListener('DOMContentLoaded', boot) : boot();
})();
