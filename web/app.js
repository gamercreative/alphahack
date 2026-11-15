const app = document.querySelector('#app');
const dialog = document.querySelector('#dialog');
const live = document.querySelector('#announce');

// ---- API Base URL & Strict Single-Hub Configuration -------------------------
// Reads VITE_API_BASE_URL / NEXT_PUBLIC_API_BASE_URL with fallback to http://10.31.216.148:8113.
// Communicates ONLY with the Go backend via REST and SSE.
const DEFAULT_API_BASE_URL = 'http://10.31.216.148:8113';
function getApiBaseUrl() {
  const fromWindow = typeof window !== 'undefined' && (window.VITE_API_BASE_URL || window.NEXT_PUBLIC_API_BASE_URL);
  const fromProcess = typeof globalThis.process !== 'undefined' && globalThis.process?.env && (
    globalThis.process.env.VITE_API_BASE_URL || globalThis.process.env.NEXT_PUBLIC_API_BASE_URL
  );
  let fromStorage = null;
  try { fromStorage = sessionStorage.getItem('wassit-api-base-url'); } catch {}
  return String(fromStorage || fromProcess || fromWindow || DEFAULT_API_BASE_URL).trim().replace(/\/+$/, '');
}
const API_BASE_URL = getApiBaseUrl();

// ---- Demo Branch Presets (All PIN 1234) -------------------------------------
const DEMO_BRANCH_PRESETS = [
  { code: 'A', email: 'a@wassit.demo', pin: '1234', branchId: 'branch-hamra', name: 'Cornilia Pharmacy Hamra', shortName: 'Hamra (A · Requester)', city: 'Beirut - Hamra', roleHint: 'Usual requesting pharmacy' },
  { code: 'B', email: 'b@wassit.demo', pin: '1234', branchId: 'branch-achrafieh', name: 'Cornilia Pharmacy Achrafieh', shortName: 'Achrafieh (B)', city: 'Beirut - Achrafieh', roleHint: 'Target peer pharmacy B' },
  { code: 'C', email: 'c@wassit.demo', pin: '1234', branchId: 'branch-baabda', name: 'Cornilia Pharmacy Baabda', shortName: 'Baabda (C)', city: 'Baabda', roleHint: 'Target peer pharmacy C' },
  { code: 'D', email: 'd@wassit.demo', pin: '1234', branchId: 'branch-verdun', name: 'Cornilia Pharmacy Verdun', shortName: 'Verdun (D)', city: 'Beirut - Verdun', roleHint: 'Target peer pharmacy D' },
  { code: 'E', email: 'e@wassit.demo', pin: '1234', branchId: 'branch-byblos', name: 'Cornilia Pharmacy Byblos', shortName: 'Byblos (E)', city: 'Byblos', roleHint: 'Target peer pharmacy E' }
];

const DEMO_ENQUIRY_PRESETS = [
  { label: 'Augmentin 1g x2 (Exact Match → Auto Propose)', raw_request: 'need augmentin 1g tabs x2', quantity: 2 },
  { label: 'Augmentin (Ambiguous → Asks Clarification)', raw_request: 'need augmentin', quantity: 2 },
  { label: 'Panadol Advance 500mg x3', raw_request: 'need panadol advance 500mg tablet x3', quantity: 3 },
  { label: 'Concor 5mg x2', raw_request: 'need concor 5mg tablet x2', quantity: 2 },
  { label: 'Ventolin 100mcg Inhaler x1', raw_request: 'need ventolin 100mcg inhaler x1', quantity: 1 }
];

// ---- Opaque Staff Session State (Never decoded as JWT) ----------------------
// Shape: { token: "<opaque hex string>", staff: {...}, branch: {...} } | null
let liveSession = null;
try {
  const rawSession = sessionStorage.getItem('wassit-staff-session');
  if (rawSession) {
    const parsed = JSON.parse(rawSession);
    if (parsed && typeof parsed.token === 'string' && parsed.token.length > 0 && parsed.staff && parsed.branch) {
      liveSession = parsed;
    }
  }
} catch {}

function saveLiveSession(session) {
  liveSession = session;
  try {
    if (session) sessionStorage.setItem('wassit-staff-session', JSON.stringify(session));
    else sessionStorage.removeItem('wassit-staff-session');
  } catch {}
}

// ---- Live Pharmacy Office State ---------------------------------------------
const liveState = {
  loading: false,
  error: '',
  actionError: '',
  actionNotice: '',
  outgoingEnquiries: [],
  incomingEnquiries: [],
  inboxItems: [],
  medications: [],
  branches: [],
  branchInventory: [],
  stats: { data: [], totals: null },
  activeEnquiryId: null,
  activeDetail: null,
  activeCandidates: [],
  activeOffers: [],
  offersNotice: '',
  openOfferForms: new Set(),
  call: null, // simulated call playback state
  callNotice: '',
  deselectedTargetIds: new Set(),
  sseSource: null,
  sseState: 'idle', // 'connecting' | 'connected' | 'polling' | 'idle'
  seenEventIds: new Set(),
  lastLoadedAt: null
};

// ---- Icons & Helpers --------------------------------------------------------
const paths = {
  pill:'M5 19a5 5 0 0 1 0-7l7-7a5 5 0 0 1 7 7l-7 7a5 5 0 0 1-7 0Zm4-11 7 7',
  mark:'M4 14h4l2.5-7 3 10 2-5H20M12 3v2M12 19v2',
  arrow:'M4 12h15m-6-6 6 6-6 6',
  search:'M15.5 15.5 21 21M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0Z',
  check:'m5 12 4 4L19 6',
  shield:'M12 22s8-4 8-11V5l-8-3-8 3v6c0 7 8 11 8 11Z m-3-10 2 2 4-4',
  pulse:'M2 12h5l3-7 4 14 3-7h5',
  clock:'M12 7v5l3 2m6-2a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z',
  link:'M10 13a4 4 0 0 0 6 0l3-3a4 4 0 0 0-6-6l-1 1m2 6a4 4 0 0 0-6 0l-3 3a4 4 0 0 0 6 6l1-1',
  home:'M3 11 12 3l9 8M5 10v10h14V10M10 20v-6h4v6',
  folder:'M3 7h7l2 2h9v10H3V7Z',
  star:'m12 3 2.7 5.7 6.3.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.3-.9L12 3Z',
  box:'M4 7 12 3l8 4v10l-8 4-8-4V7Zm0 0 8 4 8-4m-8 4v10',
  bell:'M5 17h14l-2-3V9A5 5 0 0 0 7 9v5l-2 3Zm5 3h4',
  pin:'M12 22s7-7 7-13A7 7 0 0 0 5 9c0 6 7 13 7 13Zm2-13a2 2 0 1 1-4 0 2 2 0 0 1 4 0Z'
};
const icon = name => '<svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="'+(paths[name]||paths.pulse)+'"/></svg>';
const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

// ---- REST Helper (Go Backend Only, Opaque Bearer Token) ---------------------
async function apiCall(method, path, body = undefined, requireAuth = true) {
  const headers = { 'Accept': 'application/json' };
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  if (requireAuth) {
    const token = liveSession?.token;
    if (!token) {
      const err = new Error('Please log in to a pharmacy branch first.');
      err.code = 'unauthorized';
      err.status = 401;
      throw err;
    }
    headers['Authorization'] = `Bearer ${token}`;
  }

  let response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined
    });
  } catch (networkErr) {
    const err = new Error(`Cannot reach Wassit backend at ${API_BASE_URL}. Check network or CORS_ALLOWED_ORIGINS.`);
    err.code = 'network_error';
    err.status = 0;
    throw err;
  }

  if (response.status === 204) return {};
  let payload = {};
  const rawText = await response.text();
  if (rawText) {
    try { payload = JSON.parse(rawText); } catch { payload = { raw: rawText }; }
  }

  if (!response.ok) {
    const apiErr = payload && typeof payload.error === 'object' && payload.error ? payload.error : null;
    const message = apiErr?.message || (typeof payload.error === 'string' ? payload.error : `Request failed (${response.status})`);
    const code = apiErr?.code || (response.status === 401 ? 'unauthorized' : 'http_error');
    if (response.status === 401 && requireAuth) {
      closeLiveSSE();
      saveLiveSession(null);
      status('Session expired or unauthorized. Please log in again.');
      if (page[0] === 'pharmacist') {
        location.hash = 'login/pharmacist';
      }
    }
    const err = new Error(message);
    err.code = code;
    err.status = response.status;
    err.requestId = apiErr?.request_id || '';
    throw err;
  }
  return payload;
}

// ---- Raw Audio Helper (POST /api/v1/speech returns audio bytes, not JSON) ----
async function apiAudio(method, path, body = undefined) {
  const token = liveSession?.token;
  if (!token) {
    const err = new Error('Please log in to a pharmacy branch first.');
    err.code = 'unauthorized';
    err.status = 401;
    throw err;
  }
  const headers = { 'Accept': 'audio/*', 'Authorization': `Bearer ${token}` };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  let response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined
    });
  } catch {
    const err = new Error(`Cannot reach Wassit backend at ${API_BASE_URL}. Check network or CORS_ALLOWED_ORIGINS.`);
    err.code = 'network_error';
    err.status = 0;
    throw err;
  }
  if (!response.ok) {
    let payload = {};
    try { payload = JSON.parse(await response.text()); } catch {}
    const apiErr = payload && typeof payload.error === 'object' && payload.error ? payload.error : null;
    const err = new Error(apiErr?.message || `Request failed (${response.status})`);
    err.code = apiErr?.code || 'http_error';
    err.status = response.status;
    throw err;
  }
  return response.blob();
}

// ---- Simulated Patient / Client Portal Data (Option A: Local Only) ----------
const meds = [
  ['Paracetamol','500 mg','Tablet'],['Cetirizine','10 mg','Tablet'],
  ['Salbutamol','100 mcg per actuation','Inhaler'],['Paracetamol','120 mg / 5 mL','Oral suspension'],
  ['Metformin','500 mg','Film-coated tablet'],['Amlodipine','5 mg','Tablet'],
  ['Omeprazole','20 mg','Gastro-resistant capsule']
];
const pharmacies = [
  ['Cedar Pharmacy','Byblos · Old Souk district'],
  ['Harbour Pharmacy','Jounieh · Waterfront district'],
  ['Olive Pharmacy','Beirut · Hamra district']
];
let stock = [[12,6,4,0,8,0,3],[0,4,2,5,0,7,2],[9,0,3,0,2,6,0]];
let stockDates={};
try {
  const saved=JSON.parse(localStorage.getItem('dosyn-demo-stock'));
  if(saved){stock=saved.stock;stockDates=saved.stockDates;}
} catch {}
const categories = [['Pain and fever',[0,3]],['Allergies',[1]],['Respiratory conditions',[2]],['Diabetes',[4]],['Blood pressure',[5]],['Digestive conditions',[6]]];
const prescriptions = {'demo-client':[{med:0,prescribed:true,paid:true,quantity:20,unit:'tablets',reviewed:true},{med:1,prescribed:true,paid:false,quantity:10,unit:'tablets',reviewed:true}]};
const eligible = () => (prescriptions[sessions.client.key] || []).filter(record=>record.prescribed && record.paid).map(record=>record.med);
function reviewedPrescription(med) {
  return (prescriptions[sessions.client.key] || []).find(r=>r.med===med && r.prescribed && r.reviewed && Number.isInteger(r.quantity) && r.quantity>0 && r.unit);
}
let selectedPharmacy=0, category=-1, language='en', theme='dark';
try { language=localStorage.getItem('findose-language')||'en'; theme=localStorage.getItem('findose-theme')||'dark'; } catch {}
if (!['en','ar'].includes(language)) language='en';
const navs = {
  client:[['home','Ask Wassit','search'],['tickets','My tickets','pulse'],['schedules','Scheduled medicines','clock'],['pharmacies','Pharmacies','pin'],['folders','Documents','folder'],['account','Account','shield']],
  pharmacist:[['home','Overview','home'],['requests','Client tickets','pulse'],['network','Network enquiries','link'],['approvals','Peer inbox & HITL','shield'],['inventory','Stockroom','box'],['reference','Gov reference','folder'],['notifications','Notifications & stats','bell'],['account','Branch & session','shield']]
};
const hiddenRoutes = { client: ['chat', 'ticket'], pharmacist: ['cticket'] };
const steps = [
  ['Enquiry received','Agent · request recorded'],
  ['Exact medicine matched','Catalogue · name, strength and form checked'],
  ['Enquiry routed','Demo channel · selected pharmacy notified'],
  ['Awaiting pharmacist','Human checkpoint · exact item and quantity'],
  ['Outcome recorded','Audit log · originator notified']
];
const results = {available:'Available · demo confirmation',unavailable:'Unavailable · demo confirmation',unresolved:'Unable to confirm'};
const now = () => new Date().toISOString();
const date = value => {
  if (!value) return '—';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? String(value) : d.toLocaleString(language, {dateStyle:'medium',timeStyle:'short'});
};
const id = () => globalThis.crypto?.randomUUID?.() ?? Math.random().toString(36).slice(2);
function event(request, actor, detail, at = now()) {
  request.events.push({actor,detail,at});
}
function newRequest({med, target, quantity = 1, origin = 'client', account = 'demo-client', sender = 0, stage = 0, at = now()}) {
  const request = {id:id(),med,target,quantity,origin,account,sender,stage,at,events:[],seen:false};
  for (let i = 0; i <= stage; i++) event(request,steps[i][1],steps[i][0],at);
  return request;
}
function initial() {
  return {
    requests:[newRequest({med:0,target:0,stage:3}),newRequest({med:2,target:0,origin:'pharmacist',sender:1,stage:3})],
    saved:{'demo-client':[1]},folders:{'demo-client':[{id:'sample',name:'Routine records'}]}
  };
}
let db;
try { db = JSON.parse(localStorage.getItem('findose-v1')) || initial(); } catch { db = initial(); }
const sessions = {
  client:{key:'demo-client',name:'Maya',entered:true},
  pharmacist:{key:'demo-pharmacist',name:'Pharmacist Hamra (A)',pharmacy:0}
};
try { Object.assign(sessions, JSON.parse(sessionStorage.getItem('findose-profiles'))); } catch {}
const documents = new Map();
let page = [], searchTerm = '', targetPreference, opener, requestInDialog;
function save() {
  try { localStorage.setItem('findose-v1',JSON.stringify(db)); }
  catch { live.textContent = 'Browser storage is unavailable. Changes are kept for this visit.'; }
}
function status(message) { live.textContent = message; }
const label = med => meds[med] ? meds[med].join(' · ') : 'Medicine';
const chip = (text, tone = '') => '<span class="tag '+tone+'">'+text+'</span>';
const btn = (text, action, kind = '', disabled = false) => '<button type="button" class="btn '+kind+'" data-action="'+action+'"'+(disabled?' disabled':'')+'>'+t(text)+'</button>';
const link = (text, route, kind = '') => '<a class="btn '+kind+'" href="#'+route+'">'+t(text)+'</a>';
const appLink = (text, view, kind = '') => link(text,page[0]+'/'+view,kind);
const title = (name, sub, badgeHTML = '') => '<div class="row split"><span class="eyebrow">'+(page[0] === 'client' ? 'Client workspace · Demo / Simulated' : 'Pharmacy Office · Live Wassit Backend')+'</span>'+badgeHTML+'</div><h1>'+t(name)+'</h1><p>'+t(sub)+'</p>';
const empty = message => '<div class="empty">'+t(message)+'</div>';
const tile = name => '<span class="tileicon">'+icon(name)+'</span>';
const brand = '<a class="brand" href="#" aria-label="DOSYN home"><span class="mark">'+icon('pill')+'</span>DOSYN</a>';

const words = {
  'Ask Wassit':['Demander à Wassit','اسأل وسيط'],'My tickets':['Mes tickets','طلباتي'],'Scheduled medicines':['Médicaments planifiés','الأدوية المجدولة'],'Client tickets':['Tickets clients','طلبات العملاء'],'Gov reference':['Référence officielle','المرجع الحكومي'],
  'Pharmacy floor':['Espace patient','واجهة الصيدلية'],'My medicines':['Mes médicaments','أدويتي'],'Enquiries':['Demandes','الاستفسارات'],'Pharmacies':['Pharmacies','الصيدليات'],'Documents':['Documents','المستندات'],'History':['Historique','السجل'],'Account':['Compte','الحساب'],'Overview':['Vue d’ensemble','نظرة عامة'],'Client requests (Demo)':['Demandes patients (Démo)','طلبات المرضى (تجريبي)'],'Network enquiries':['Demandes du réseau','استفسارات الشبكة'],'Peer inbox & HITL':['Boîte réseau & validations','صندوق الوارد والموافقات'],'Stockroom':['Stock','المخزون'],'Notifications & stats':['Notifications & stats','الإشعارات والإحصائيات'],'Branch & session':['Succursale & session','الفرع والجلسة'],'Light mode':['Mode clair','الوضع الفاتح'],'Dark mode':['Mode sombre','الوضع الداكن'],'Pharmacy map':['Carte des pharmacies','خريطة الصيدليات'],'Browse pharmacies':['Voir les pharmacies','عرض الصيدليات'],'View pharmacy':['Voir la pharmacie','عرض الصيدلية'],'Use this pharmacy':['Choisir cette pharmacie','اختيار هذه الصيدلية'],'Your prescribed and paid medicines':['Vos médicaments prescrits et payés','أدويتك الموصوفة والمدفوعة'],'No prescribed and paid medicines yet.':['Aucun médicament prescrit et payé pour le moment.','لا توجد أدوية موصوفة ومدفوعة حتى الآن.'],'Available demo inventory':['Stock disponible de démonstration','المخزون التجريبي المتاح'],'All categories':['Toutes les catégories','جميع الفئات'],'Pain and fever':['Douleur et fièvre','الألم والحمى'],'Allergies':['Allergies','الحساسية'],'Respiratory conditions':['Affections respiratoires','أمراض الجهاز التنفسي'],'Diabetes':['Diabète','السكري'],'Blood pressure':['Tension artérielle','ضغط الدم'],'Digestive conditions':['Troubles digestifs','اضطرابات الجهاز الهضمي'],'Close':['Fermer','إغلاق'],'Log in':['Connexion','تسجيل الدخول'],'Get started':['Commencer','ابدأ'],'Simulation':['Simulation','محاكاة'],'Pharmacist view':['Espace pharmacien','واجهة الصيدلي'],'Client view':['Espace patient','واجهة المريض'],'Search':['Rechercher','بحث'],'AI pharmacy assistant':['Assistant IA de pharmacie','مساعد الصيدلية بالذكاء الاصطناعي'],'Awaiting pharmacist':['En attente du pharmacien','بانتظار الصيدلي'],'In progress':['En cours','قيد المعالجة'],
  'Client workspace':['Espace patient','واجهة المريض'],'All enquiries':['Toutes les demandes','جميع الاستفسارات'],'Needs your response':['Réponse requise','يتطلب ردّك'],'Completed':['Terminées','مكتملة'],'Show map':['Afficher la carte','عرض الخريطة'],'Pharmacy workspace':['Espace pharmacien','واجهة الصيدلي'],'Selected pharmacy':['Pharmacie sélectionnée','الصيدلية المختارة'],'Illustrated coastal map · Fictional demo locations':['Carte côtière illustrée · Emplacements fictifs','خريطة ساحلية توضيحية · مواقع تجريبية خيالية'],'Document library':['Bibliothèque de documents','مكتبة المستندات'],'Your folders':['Vos dossiers','مجلداتك'],'Create folder':['Créer un dossier','إنشاء مجلد'],'My documents':['Mes documents','مستنداتي'],'Availability enquiries':['Demandes de disponibilité','استفسارات التوفر'],'Enquiry history':['Historique des demandes','سجل الاستفسارات'],'Pending confirmations':['Confirmations en attente','الموافقات المعلقة'],'Pharmacy overview':['Vue d’ensemble de la pharmacie','نظرة عامة على الصيدلية'],'Ask a pharmacy':['Interroger une pharmacie','اسأل صيدلية'],'Ask the pharmacy network':['Interroger le réseau','اسأل شبكة الصيدليات'],'Send simulated enquiry':['Envoyer une demande simulée','إرسال استفسار تجريبي'],'View enquiry':['Voir la demande','عرض الاستفسار'],'Ask network':['Interroger le réseau','اسأل الشبكة'],'No available demo stock matches this filter.':['Aucun stock de démonstration ne correspond.','لا يوجد مخزون تجريبي يطابق هذا الفلتر.'],'Ask about availability':['Vérifier la disponibilité','الاستفسار عن التوفر'],'Administrative support · Human pharmacist confirms stock':['Assistance administrative · Stock confirmé par le pharmacien','دعم إداري · يؤكد الصيدلي المخزون']
};
const t = text => words[text]?.[language==='fr'?0:language==='ar'?1:-1] || text;

function preferences() {
  return '<div class="preferences"><select id="language" aria-label="Language"><option value="en">English</option><option value="ar">العربية</option></select>'+btn(t(theme==='dark'?'Light mode':'Dark mode'),'theme','secondary small')+'</div>';
}

const badge = request => chip(t(request.result ? results[request.result] : request.stage === 3 ? 'Awaiting pharmacist' : 'In progress'), request.result ? (request.result === 'unresolved' ? 'neutral' : '') : 'warm');

function demoClientBanner() {
  return '<div class="demo-sim-banner" role="region" aria-label="Demo simulation notice">' +
    '<div>' + chip('Demo / Simulated', 'demo-badge') + ' <strong>Patient / Client Portal (Simulated Local State)</strong> — Patient prescriptions, documents, and client-side enquiries run on local browser demo state.</div>' +
    link('Switch to Live Pharmacy Office →', 'pharmacist/home', 'secondary small') +
  '</div>';
}

function branchSwitcherStrip() {
  const activeEmail = liveSession?.staff?.email || '';
  return '<div class="branch-switcher-strip" role="group" aria-label="Quick Branch Switcher">' +
    '<span class="caption" style="font-weight:700;padding-inline:4px">Branch:</span>' +
    DEMO_BRANCH_PRESETS.map(b =>
      '<button type="button" class="branch-pill '+(activeEmail===b.email?'active':'')+'" data-action="quick-branch:'+b.email+'" aria-pressed="'+(activeEmail===b.email)+'" title="Switch to '+escapeHTML(b.name)+' ('+b.email+')">' +
        '<strong>'+b.code+'</strong> '+escapeHTML(b.shortName.split(' (')[0]) +
      '</button>'
    ).join('') +
  '</div>';
}

function bar(workspace = false) {
  const role = page[0];
  if (!workspace) {
    const isClientAuth = ['login','register'].includes(page[0]) && page[1] === 'client';
    return '<header class="top"><div class="wrap">'+brand+preferences()+(isClientAuth ? '<div class="actions">'+chip('Demo / Simulated', 'demo-badge')+'</div>' : '')+'</div></header>';
  }
  if (role === 'client') {
    return '<header class="top appbar"><div class="wrap">'+brand+preferences()+
      '<div class="actions">'+
        chip('Demo / Simulated', 'demo-badge')+
        '<span class="role-name">'+escapeHTML(sessions.client.name)+'</span>'+
        link(t('Pharmacist view'), 'pharmacist/home', 'secondary small')+
      '</div></div></header>';
  }
  // Pharmacist live workspace bar
  const branchName = liveSession?.branch?.name || 'Not logged in';
  const staffEmail = liveSession?.staff?.email || '';
  return '<header class="top appbar"><div class="wrap">'+brand+
    branchSwitcherStrip()+
    '<div class="actions">'+
      chip('<span class="live-pulse"></span> Live · Wassit API', 'live')+
      '<span class="role-name" title="'+escapeHTML(staffEmail)+'">'+escapeHTML(branchName)+'</span>'+
      btn('Refresh', 'refresh-live', 'secondary small')+
      link('Client view (Demo)', 'client/home', 'ghost small')+
    '</div>'+
    preferences()+
  '</div></header>';
}

function timeline(stage, demo = false) {
  return '<ol class="timeline">'+steps.slice(0,demo?4:5).map(([name,detail],i) => {
    const complete = i < stage || (i === stage && stage === 4);
    const state = complete ? 'Complete' : i === stage ? (i === 3 ? 'Waiting' : 'In progress') : 'Queued';
    return '<li class="'+(i === stage?'waiting':'')+'"><span class="step" aria-hidden="true">'+(complete?icon('check'):i+1)+'</span><div><strong>'+name+'</strong><small>'+state+' · '+detail+'</small></div></li>';
  }).join('')+'</ol>';
}

function landing() {
  return '<div class="dispensary public-site">'+bar()+'<main id="main" class="wrap entry-hall entry-gallery enter" tabindex="-1"><div>'+wzAvatar('lg')+wzNametag()+'<h1>'+wzT('Meet Wassit','تعرّف على وسيط')+'</h1><p>'+wzT('Your AI pharmacist assistant. Wassit asks many pharmacies at once for the medicine you need, reads their answers, checks doses and box photos against the official reference, and brings back the best verified price and expiry.','مساعدك الصيدلي بالذكاء الاصطناعي. يسأل وسيط عدة صيدليات في وقت واحد عن الدواء الذي تحتاجه، يقرأ ردودهم، يتحقق من الجرعات وصور العلب مقابل المرجع الرسمي، ويعيد لك أفضل سعر وصلاحية موثّقين.')+'</p><div class="entry-doors">'+
    '<a class="card entry-door" href="#pharmacist/home">'+tile('shield')+'<div><div class="row">'+chip('<span class="live-pulse"></span> Live Backend', 'live')+'</div><strong>'+wzT('I’m a pharmacist','أنا صيدلي')+'</strong><span>'+wzT('Reply to Wassit’s requests, upload package photos, update stock, ask other pharmacies','رد على طلبات وسيط، ارفع صور العلب، حدّث المخزون، واسأل صيدليات أخرى')+'</span></div>'+icon('arrow')+'</a>'+
    '<a class="card entry-door" href="#login/client">'+tile('home')+'<div><div class="row">'+chip('Demo / Simulated', 'demo-badge')+'</div><strong>'+wzT('I need a medicine','أحتاج دواءً')+'</strong><span>'+wzT('Search, request once or every week, follow each pharmacy’s answer in your ticket','ابحث، اطلب مرة أو كل أسبوع، وتابع رد كل صيدلية في ملفك')+'</span></div>'+icon('arrow')+'</a>'+
  '</div><p class="caption">Connected to live Wassit Go API at <code>'+escapeHTML(API_BASE_URL)+'</code></p></div><div class="pill-display" aria-hidden="true"><div class="pill-ring"></div><div class="hero-pill"><i></i><strong>DOSYN</strong></div></div></main></div>';
}

// ---- Staff Login & Client Simulated Login -----------------------------------
let application;
let showRegistrationDemo = false;

function auth() {
  const register = page[0] === 'register';
  const pharmacy = page[1] === 'pharmacist';
  const role = page[1];

  if (pharmacy) {
    if (showRegistrationDemo) return pharmacyApplication();
    const activeEmail = liveSession?.staff?.email || 'a@wassit.demo';
    return '<div class="dispensary">'+bar()+'<main id="main" class="entry-account enter" tabindex="-1"><a class="btn secondary" href="#">← Back to entrance</a>'+
      '<div class="card access-card">'+
        '<div class="row split"><span class="eyebrow">Wassit Staff Authentication</span>'+chip('<span class="live-pulse"></span> Live Go Backend', 'live')+'</div>'+
        '<h1>Pharmacy Office Login</h1>'+
        '<p>Sign in with a participating branch account or use a 1-click demo preset below (all demo PINs are <code>1234</code>).</p>'+
        '<h3 style="margin-top:18px">1-Click Pitch Presets (Switch Branches Instantly)</h3>'+
        '<div class="preset-grid">'+
          DEMO_BRANCH_PRESETS.map(b =>
            '<button type="button" class="preset-card" data-action="quick-branch:'+b.email+'" aria-pressed="'+(activeEmail===b.email)+'">'+
              '<span class="tag '+(b.code==='A'?'live':'neutral')+'">Branch '+b.code+' · PIN 1234</span>'+
              '<strong>'+escapeHTML(b.name)+'</strong>'+
              '<small>'+escapeHTML(b.email)+' · '+escapeHTML(b.roleHint)+'</small>'+
            '</button>'
          ).join('')+
        '</div>'+
        '<form class="form" id="staff-login-form" style="margin-top:18px;border-top:1px solid var(--line);padding-top:16px">'+
          '<label class="field">Staff Email'+
            '<input type="email" name="email" id="staff-login-email" value="'+escapeHTML(activeEmail)+'" placeholder="a@wassit.demo" required>'+
          '</label>'+
          '<label class="field">Staff PIN'+
            '<input type="password" name="pin" id="staff-login-pin" value="1234" placeholder="1234" required>'+
          '</label>'+
          '<p id="verification-result" class="verify-result" role="status">'+escapeHTML(liveState.error)+'</p>'+
          '<button class="btn" type="submit">Sign in to Pharmacy Office '+icon('arrow')+'</button>'+
        '</form>'+
        '<p class="caption" style="margin-top:14px">Backend endpoint: <code>POST '+escapeHTML(API_BASE_URL)+'/api/v1/auth/login</code></p>'+
        '<p class="access-switch"><button type="button" class="btn ghost small" data-action="toggle-reg-demo">Preview new pharmacy registration screen (Demo)</button></p>'+
      '</div></main></div>';
  }

  // Client login / signup (Simulated Local State)
  return '<div class="dispensary">'+bar()+'<main id="main" class="entry-account enter" tabindex="-1"><a class="btn secondary" href="#">← Back to entrance</a>'+
    '<div class="card access-card">'+
      '<div class="row split"><span class="eyebrow">Client account</span>'+chip('Demo / Simulated', 'demo-badge')+'</div>'+
      '<h1>'+(register?'Client Sign up (Demo)':'Client Login (Demo)')+'</h1>'+
      '<p>'+(register?'Create a simulated client profile in local browser storage.':'Open the simulated patient portal profile. No password is required.')+'</p>'+
      '<form class="form" id="auth-form">'+
        (register?'<label class="field">Name<input name="name" maxlength="45" required></label>':'')+
        '<button class="btn">'+(register?'Create simulated profile':'Enter simulated client portal')+' '+icon('arrow')+'</button>'+
      '</form>'+
      '<p class="access-switch">'+(register?'Already have a profile? ':'New to DOSYN? ')+'<a href="#'+(register?'login':'register')+'/'+role+'">'+(register?'Log in':'Sign up')+'</a></p>'+
    '</div></main></div>';
}

function pharmacyApplication() {
  const content = application
    ? '<div class="row split"><span class="eyebrow">Onboarding simulation</span>'+chip('Demo / Simulated', 'demo-badge')+'</div><h1>Pending verification</h1><p>'+escapeHTML(application.name)+' · '+escapeHTML(application.town)+'</p><p class="notice">A DOSYN reviewer would match the pharmacy to an independent record and contact it to confirm your role. Stockroom access stays locked for this application.</p>'+btn('Start another application','reset-application','secondary')
    : '<div class="row split"><span class="eyebrow">Onboarding simulation</span>'+chip('Demo / Simulated', 'demo-badge')+'</div><h1>Register your pharmacy</h1><p>Enter your own pharmacy details. DOSYN reviews access before opening the stockroom.</p><form id="pharmacy-application" class="form"><label class="field">Pharmacy name<input name="pharmacyName" maxlength="100" required></label><label class="field">Town or district<input name="town" maxlength="100" required></label><label class="field">Work email<input type="email" name="email" placeholder="staff@example.com" required></label><label class="field">Demo password<input type="password" minlength="8" required></label><p class="caption">Use fictional details. This demo saves no passwords and sends no email.</p><button class="btn">Request pharmacy access</button></form>';
  return '<div class="dispensary">'+bar()+'<main id="main" class="entry-account enter" tabindex="-1"><button type="button" class="btn secondary" data-action="toggle-reg-demo">← Back to Staff Branch Login</button><div class="card access-card">'+content+'</div></main></div>';
}

// ---- Live Backend Actions & Data Fetching -----------------------------------
async function loginAsStaff(email, pin = '1234', navigateOnSuccess = false) {
  liveState.error = '';
  liveState.actionError = '';
  try {
    const res = await apiCall('POST', '/api/v1/auth/login', { email: email.trim(), pin: String(pin).trim() }, false);
    if (!res || typeof res.token !== 'string' || !res.token) {
      throw new Error('Login response did not include a session token.');
    }
    // Store opaque session token as-is (NEVER decode as JWT)
    saveLiveSession({
      token: res.token,
      staff: res.staff,
      branch: res.branch
    });
    const presetIdx = DEMO_BRANCH_PRESETS.findIndex(b => b.email === res.staff?.email || b.branchId === res.branch?.id);
    if (presetIdx >= 0) {
      sessions.pharmacist.pharmacy = presetIdx;
      sessions.pharmacist.verifiedBranch = presetIdx;
      sessions.pharmacist.name = res.staff?.full_name || DEMO_BRANCH_PRESETS[presetIdx].name;
    }
    liveState.actionNotice = `Signed in as ${res.staff?.full_name || email} (${res.branch?.name || ''})`;
    status(liveState.actionNotice);

    await refreshLivePharmacyData();

    // If an enquiry modal is currently open, check whether the new branch can view it
    if (liveState.activeEnquiryId && dialog.open) {
      try {
        await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
        connectLiveSSE(liveState.activeEnquiryId);
      } catch {
        closeLiveSSE();
        liveState.activeEnquiryId = null;
        liveState.activeDetail = null;
        dialog.close();
      }
    }

    if (navigateOnSuccess || page[0] !== 'pharmacist') {
      location.hash = 'pharmacist/home';
    } else {
      render();
    }
  } catch (err) {
    liveState.error = err.message || 'Login failed';
    liveState.actionError = liveState.error;
    const verifyEl = document.getElementById('verification-result');
    if (verifyEl) verifyEl.textContent = liveState.error;
    else render();
    status(liveState.error);
  }
}

async function verifyRestoredSession() {
  if (!liveSession?.token) return false;
  try {
    const me = await apiCall('GET', '/api/v1/me');
    if (me?.staff && me?.branch) {
      saveLiveSession({
        token: liveSession.token,
        staff: me.staff,
        branch: me.branch
      });
      await refreshLivePharmacyData();
      return true;
    }
  } catch {
    saveLiveSession(null);
  }
  return false;
}

async function refreshLivePharmacyData() {
  if (!liveSession?.token) return;
  liveState.loading = true;
  try {
    const branchId = liveSession.branch?.id;
    const [outgoingRes, incomingRes, inboxRes, medsRes, branchesRes, statsRes, invRes] = await Promise.all([
      apiCall('GET', '/api/v1/enquiries?scope=outgoing&limit=50').catch(() => ({ data: [] })),
      apiCall('GET', '/api/v1/enquiries?scope=incoming&limit=50').catch(() => ({ data: [] })),
      apiCall('GET', '/api/v1/inbox?limit=50').catch(() => ({ data: [] })),
      apiCall('GET', '/api/v1/medications?limit=100', undefined, false).catch(() => ({ data: [] })),
      apiCall('GET', '/api/v1/branches?limit=50', undefined, false).catch(() => ({ data: [] })),
      apiCall('GET', '/api/v1/stats?limit=50').catch(() => ({ data: [], totals: null })),
      branchId ? apiCall('GET', `/api/v1/branches/${encodeURIComponent(branchId)}/inventory?limit=100`, undefined, false).catch(() => ({ data: [] })) : Promise.resolve({ data: [] })
    ]);
    liveState.outgoingEnquiries = Array.isArray(outgoingRes?.data) ? outgoingRes.data : [];
    liveState.incomingEnquiries = Array.isArray(incomingRes?.data) ? incomingRes.data : [];
    liveState.inboxItems = Array.isArray(inboxRes?.data) ? inboxRes.data : [];
    liveState.medications = Array.isArray(medsRes?.data) ? medsRes.data : [];
    liveState.branches = Array.isArray(branchesRes?.data) ? branchesRes.data : [];
    liveState.stats = {
      data: Array.isArray(statsRes?.data) ? statsRes.data : [],
      totals: statsRes?.totals || null
    };
    liveState.branchInventory = Array.isArray(invRes?.data) ? invRes.data : [];
    liveState.lastLoadedAt = new Date().toISOString();
    liveState.error = '';
  } catch (err) {
    liveState.error = err.message || 'Could not load pharmacy data';
  } finally {
    liveState.loading = false;
  }
  updateDeskIfSafe();
}

function updateDeskIfSafe() {
  if (page[0] !== 'pharmacist') return;
  // Demo ticket/reference pages refresh themselves; a re-render would replay animations and drop a chosen photo.
  if (['cticket', 'reference'].includes(page[1])) return;
  const deskEl = document.querySelector('.desk');
  if (!deskEl) return;
  // Do not overwrite desk DOM while the user is typing in an input/textarea/select inside .desk
  const active = document.activeElement;
  if (active && deskEl.contains(active) && ['INPUT', 'TEXTAREA', 'SELECT'].includes(active.tagName)) {
    return;
  }
  deskEl.innerHTML = desk();
}

// Catalogue + branches for the client side (public routes, no staff login needed).
let publicCatalogueLoading = null;
function loadPublicCatalogue() {
  if (publicCatalogueLoading) return publicCatalogueLoading;
  publicCatalogueLoading = Promise.all([
    apiCall('GET', '/api/v1/medications?limit=100', undefined, false).catch(() => ({ data: [] })),
    apiCall('GET', '/api/v1/branches?limit=50', undefined, false).catch(() => ({ data: [] }))
  ]).then(([medsRes, branchesRes]) => {
    if (Array.isArray(medsRes?.data) && medsRes.data.length) liveState.medications = medsRes.data;
    if (Array.isArray(branchesRes?.data) && branchesRes.data.length) liveState.branches = branchesRes.data;
    if (page[0] !== 'client') return;
    const results = document.getElementById('wz-results');
    const deskEl = document.querySelector('.desk');
    if (results) results.innerHTML = wzResultsHTML();
    else if (deskEl && !deskEl.contains(document.activeElement)) deskEl.innerHTML = desk();
  }).finally(() => { setTimeout(() => { publicCatalogueLoading = null; }, 30000); });
  return publicCatalogueLoading;
}

// Government reference (public, live since migration 0007). Mapped to the demo adapter's shape.
async function loadGovReference() {
  try {
    const res = await apiCall('GET', '/api/v1/reference/products?limit=100', undefined, false);
    liveState.govReference = (res?.data || []).map(r => ({
      code: r.code, regNo: r.reg_no, brandName: r.brand_name, ingredient: r.ingredient,
      strength: r.strength, form: r.dosage_form, officialPriceCents: r.official_price_cents, source: r.source
    }));
  } catch { liveState.govReference = null; }
  return liveState.govReference;
}

// Staff stock update for the signed-in branch (PUT, staff only; the agent is refused).
async function saveLiveStock(medId, quantity) {
  const branchId = liveSession?.branch?.id;
  if (!branchId) throw new Error('Please log in to a pharmacy branch first.');
  await apiCall('PUT', `/api/v1/branches/${encodeURIComponent(branchId)}/inventory/${encodeURIComponent(medId)}`, { quantity_on_hand: quantity });
  await refreshLivePharmacyData();
}

// ---- Live SSE Stream + Clean Polling Fallback -------------------------------
function closeLiveSSE() {
  if (liveState.sseSource) {
    try { liveState.sseSource.close(); } catch {}
    liveState.sseSource = null;
  }
  liveState.sseState = 'idle';
}

function connectLiveSSE(enquiryId) {
  closeLiveSSE();
  if (!liveSession?.token || !enquiryId) return;
  liveState.sseState = 'connecting';
  // Pass opaque session token via access_token query param as required for EventSource
  const streamUrl = `${API_BASE_URL}/api/v1/enquiries/${encodeURIComponent(enquiryId)}/stream?access_token=${encodeURIComponent(liveSession.token)}`;
  try {
    const es = new EventSource(streamUrl);
    liveState.sseSource = es;

    es.onopen = () => {
      liveState.sseState = 'connected';
      updateSSEIndicator();
    };

    es.addEventListener('enquiry_event', (e) => {
      liveState.sseState = 'connected';
      try {
        const ev = JSON.parse(e.data);
        if (ev && ev.id) {
          const isNew = !liveState.seenEventIds.has(ev.id);
          liveState.seenEventIds.add(ev.id);
          if (isNew && liveState.activeEnquiryId === enquiryId) {
            fetchAndRenderLiveEnquiry(enquiryId, true);
            refreshLivePharmacyData();
          }
        }
      } catch {}
    });

    es.onerror = () => {
      // Browser EventSource automatically attempts reconnect; polling fallback covers any gap
      liveState.sseState = 'polling';
      updateSSEIndicator();
    };
  } catch {
    liveState.sseState = 'polling';
  }
}

function updateSSEIndicator() {
  const el = document.getElementById('sse-status-pill');
  if (!el) return;
  if (liveState.sseState === 'connected') {
    el.className = 'tag live';
    el.innerHTML = '<span class="live-pulse"></span> SSE Live Stream Connected';
  } else if (liveState.sseState === 'connecting') {
    el.className = 'tag info';
    el.textContent = 'SSE Connecting…';
  } else {
    el.className = 'tag warm';
    el.textContent = 'SSE Reconnecting · Polling Fallback Active';
  }
}

// ---- Formatting & Status Badges for Live Wassit Entities --------------------
function enquiryStatusBadge(enq) {
  if (!enq) return '';
  const st = enq.status;
  const out = enq.outcome;
  if (st === 'closed') {
    if (out === 'available') return chip('Closed · Available ✓', 'live');
    if (out === 'unavailable') return chip('Closed · Unavailable', 'danger');
    if (out === 'unable_to_confirm') return chip('Closed · Unable to confirm', 'warm');
    return chip('Closed · ' + escapeHTML(out || 'done'), 'neutral');
  }
  if (st === 'cancelled' || out === 'cancelled') return chip('Cancelled', 'danger');
  if (st === 'awaiting_clarification') return chip('Action Needed · Awaiting Clarification', 'warm');
  if (st === 'awaiting_approval') return chip('Action Needed · Awaiting Target Approval', 'info');
  if (st === 'waiting') return chip('Waiting for Pharmacy Replies / Timer', 'info');
  if (st === 'open') return chip(enq.handled_by === 'agent' ? 'Open · Wassit Agent Working' : 'Open · Manual', 'live');
  return chip(escapeHTML(st), 'neutral');
}

function targetStatusBadge(statusVal) {
  switch (statusVal) {
    case 'proposed': return chip('Proposed (Awaiting Approval)', 'info');
    case 'approved': return chip('Approved (Ready to Send)', 'info');
    case 'sent': return chip('Sent · Waiting Reply', 'warm');
    case 'available': return chip('Available ✓', 'live');
    case 'unavailable': return chip('Unavailable', 'danger');
    case 'alternative_offered': return chip('Alternative Offered (Option Only)', 'warm');
    case 'no_response': return chip('No Response (Silent)', 'neutral');
    case 'cancelled': return chip('Cancelled / Withdrawn', 'danger');
    default: return chip(escapeHTML(statusVal), 'neutral');
  }
}

function branchNameById(branchId) {
  if (!branchId) return '—';
  const preset = DEMO_BRANCH_PRESETS.find(b => b.branchId === branchId);
  if (preset) return preset.name;
  const found = liveState.branches.find(b => b.id === branchId);
  return found ? found.name : branchId;
}

function medDisplay(medId) {
  if (!medId) return 'Unmatched (Awaiting Agent / Clarification)';
  const m = liveState.medications.find(x => x.id === medId);
  if (m) return `${m.brand_name} ${m.strength} (${m.dosage_form})`;
  return medId;
}

function availableStockForMed(medId) {
  if (!medId) return null;
  const nowMs = Date.now();
  let total = 0;
  for (const b of liveState.branchInventory) {
    if (b.medication_id !== medId) continue;
    if (b.is_recalled) continue;
    if (b.expiry_date && Date.parse(b.expiry_date) <= nowMs) continue;
    const avail = Math.max(0, Number(b.quantity_on_hand || 0) - Number(b.quantity_reserved || 0));
    total += avail;
  }
  return total;
}

function formatEventDescription(ev) {
  let p = {};
  try {
    p = typeof ev.payload === 'string' ? JSON.parse(ev.payload) : (ev.payload || {});
  } catch {}
  switch (ev.type) {
    case 'enquiry_created':
      return `Enquiry created (${p.handled_by || 'agent'}): "${p.raw_request || ''}" × ${p.quantity || 1}`;
    case 'medication_set':
      return `Catalogue medicine matched: ${p.brand_name || ''} ${p.strength || ''} (${p.medication_id || ''})`;
    case 'clarification_requested':
      return `Wassit Agent asked requester: "${p.question || ''}"`;
    case 'clarified':
      return `Requester answered clarification: "${p.answer || ''}"`;
    case 'targets_proposed':
      return `Wassit Agent proposed ${(p.branch_ids || []).length} target pharmacies: ${(p.branch_ids || []).map(branchNameById).join(', ')}`;
    case 'targets_added':
      return `Staff added ${(p.branch_ids || []).length} target pharmacies: ${(p.branch_ids || []).map(branchNameById).join(', ')}`;
    case 'targets_approved':
      return `Requester approved ${(p.target_ids || []).length} proposed pharmacy target(s)`;
    case 'messages_queued':
      return `Queued ${(p.message_ids || []).length} outbound enquiry message(s) in outbox (undo window until ${date(p.send_after)})`;
    case 'message_cancelled':
      return `Outbound message cancelled inside undo window (target never sees it)`;
    case 'message_sent':
      return `Outbox dispatcher delivered ${p.kind || 'enquiry'} message to pharmacy inbox`;
    case 'follow_up_queued':
      return `Queued follow-up reminder for silent target pharmacy`;
    case 'marked_no_response':
      return `Marked silent target pharmacy as no_response after follow-up wait`;
    case 'reply_received':
      return `Peer pharmacy (${branchNameById(p.branch_id)}) replied ${p.kind}${p.quantity !== null && p.quantity !== undefined ? ` (qty ${p.quantity})` : ''}${p.text ? `: "${p.text}"` : ''}`;
    case 'requester_notified':
      return `Wassit Agent notified requester: "${p.body || ''}"`;
    case 'wait_set':
      return `Wassit Agent set wakeup timer (${p.minutes || 5} wait units)`;
    case 'enquiry_closed':
      return `Enquiry closed with outcome: ${p.outcome || ''}${p.branch_id ? ` at ${branchNameById(p.branch_id)}` : ''}`;
    case 'event_undone':
      return `Reverted event (${p.op || 'undo'})`;
    case 'offer_extracted':
      return `Offer read from ${branchNameById(p.branch_id)}'s reply into structured fields (confidence ${p.confidence !== undefined ? Math.round(Number(p.confidence) * 100) + '%' : 'n/a'})`;
    case 'offer_decided':
      return `Rules decided the ${branchNameById(p.branch_id)} offer: ${offerDecisionLabel(p.decision)}${Array.isArray(p.reasons) && p.reasons.length ? ' (' + p.reasons.join('; ') + ')' : ''}`;
    default:
      return `${ev.type}`;
  }
}

// Decider shown on every event: Rules, Human or Pharmacy decide; Wassit only relays and reads.
function actorLabel(ev) {
  if (ev.actor_type === 'agent') return 'Wassit · relayed';
  if (ev.actor_type === 'system' && ev.actor_id === 'rules') return 'Rules';
  if (ev.actor_type === 'system') return 'System · outbox';
  if (ev.type === 'reply_received') return 'Pharmacy';
  return 'Human';
}

// ---- Pharmacy Office Views (Live Backend) -----------------------------------
let selectedLiveQueue = 'all';

function renderNotificationsBanner() {
  let html = '';
  if (liveState.actionError) {
    html += '<div class="toast-error" role="alert">'+escapeHTML(liveState.actionError)+'</div>';
  }
  if (liveState.actionNotice) {
    html += '<p class="notice" role="status">'+escapeHTML(liveState.actionNotice)+'</p>';
  }
  return html;
}

function quickCreateAgentEnquiryCard() {
  return '<section class="card quick-create-card" aria-labelledby="quick-enq-heading">' +
    '<div class="row split">' +
      '<div><span class="eyebrow">Wassit Autonomous Coordinator</span><h2 id="quick-enq-heading" style="margin:4px 0 6px;font-size:1.65rem">Create Agent-Handled Enquiry</h2></div>' +
      chip('handled_by: "agent"', 'live') +
    '</div>' +
    '<p>Describe the needed medicine in English, French, Arabic, or Arabizi. Wassit AI matches the catalogue item, asks for clarification if ambiguous, ranks nearby pharmacies, and proposes targets for your approval.</p>' +
    '<div class="preset-chips" role="group" aria-label="Demo request presets">' +
      DEMO_ENQUIRY_PRESETS.map((p, idx) =>
        '<button type="button" class="preset-chip" data-action="fill-preset:'+idx+'">'+escapeHTML(p.label)+'</button>'
      ).join('') +
    '</div>' +
    '<form id="live-enquiry-form" class="form" style="margin-top:12px">' +
      '<div class="inbox-reply-grid" style="grid-template-columns:2fr 120px 1.2fr auto">' +
        '<label class="field">Medicine Request (raw_request)' +
          '<input type="text" name="raw_request" id="quick-raw-request" value="need augmentin 1g tabs x2" maxlength="1000" placeholder="e.g. need augmentin 1g tabs x2" required>' +
        '</label>' +
        '<label class="field">Quantity' +
          '<input type="number" name="quantity" id="quick-quantity" value="2" min="1" max="1000" step="1" required>' +
        '</label>' +
        '<label class="field">Catalogue Link (Optional)' +
          '<select name="medication_id" id="quick-med-id">' +
            '<option value="">Let Wassit AI match / clarify (Recommended)</option>' +
            liveState.medications.map(m =>
              '<option value="'+escapeHTML(m.id)+'">'+escapeHTML(m.brand_name+' '+m.strength+' · '+m.dosage_form)+'</option>'
            ).join('') +
          '</select>' +
        '</label>' +
        '<input type="hidden" name="handled_by" value="agent">' +
        '<button class="btn" type="submit">Launch Wassit Agent '+icon('arrow')+'</button>' +
      '</div>' +
    '</form>' +
  '</section>';
}

function renderLiveEnquiryCard(enq) {
  const hitlNeeded = enq.status === 'awaiting_clarification' || enq.status === 'awaiting_approval';
  return '<article class="card request" style="'+(hitlNeeded?'border-inline-start:5px solid #f59e0b':'')+'">' +
    '<div class="row">' +
      tile(enq.handled_by === 'agent' ? 'pulse' : 'link') +
      '<div>' +
        '<div class="row" style="gap:8px">' +
          '<h3>'+escapeHTML(enq.raw_request)+' <small>(×'+Number(enq.quantity)+')</small></h3>' +
          chip(enq.handled_by === 'agent' ? 'Wassit Agent' : 'Manual', enq.handled_by === 'agent' ? 'live' : 'neutral') +
        '</div>' +
        '<p>Medication: <strong>'+escapeHTML(medDisplay(enq.medication_id))+'</strong> · From '+escapeHTML(branchNameById(enq.requester_branch_id))+' · '+date(enq.created_at)+'</p>' +
        (enq.status === 'awaiting_clarification' && enq.clarification_question
          ? '<p class="notice" style="margin-top:8px"><strong>Agent Question:</strong> '+escapeHTML(enq.clarification_question)+'</p>'
          : '') +
      '</div>' +
    '</div>' +
    '<div class="actions">' +
      enquiryStatusBadge(enq) +
      btn(hitlNeeded ? 'Respond / Approve →' : 'Open Live Timeline', 'open-live-enquiry:' + enq.id, hitlNeeded ? '' : 'secondary small') +
    '</div>' +
  '</article>';
}

function renderInboxItemCard(item) {
  const canReply = item.target_status === 'sent' || item.target_status === 'no_response';
  const stockCount = availableStockForMed(item.medication_id);
  const stockBadge = stockCount !== null
    ? chip(`Your Branch Stock: ${stockCount} units`, stockCount >= item.quantity ? 'live' : 'warm')
    : chip('Catalogue item pending', 'neutral');

  return '<article class="card inbox-reply-card" style="margin-top:12px">' +
    '<div class="row split">' +
      '<div class="row">' +
        tile('shield') +
        '<div>' +
          '<h3>'+escapeHTML(item.medication_name || item.raw_request)+' · <span style="color:#8ad4ba">Qty '+Number(item.quantity)+'</span></h3>' +
          '<p>From <strong>'+escapeHTML(item.requester_branch_name)+'</strong> · Raw request: "'+escapeHTML(item.raw_request)+'" · Sent '+date(item.sent_at)+(item.follow_ups ? ' · Follow-ups: '+item.follow_ups : '')+'</p>' +
        '</div>' +
      '</div>' +
      '<div class="actions">' +
        stockBadge +
        targetStatusBadge(item.target_status) +
        btn('View Enquiry Detail', 'open-live-enquiry:' + item.enquiry_id, 'secondary small') +
      '</div>' +
    '</div>' +
    (canReply ? (
      '<div style="margin-top:14px;padding-top:12px;border-top:1px solid var(--line)">' +
        '<div class="row split">' +
          '<strong>Quick Pharmacist Reply (Human-Only Rule 2):</strong>' +
          '<div class="preset-chips">' +
            '<button type="button" class="preset-chip" data-action="quick-reply:'+item.target_id+':available:'+item.quantity+'">✓ Confirm Available (Qty '+item.quantity+')</button>' +
            '<button type="button" class="preset-chip" data-action="quick-reply:'+item.target_id+':unavailable:0">✕ Confirm Unavailable</button>' +
            '<button type="button" class="preset-chip" data-action="quick-reply-alt:'+item.target_id+'">⇄ Offer 625mg Alternative</button>' +
          '</div>' +
        '</div>' +
        '<form class="inbox-reply-form inbox-reply-grid" data-target-id="'+escapeHTML(item.target_id)+'">' +
          '<label class="field">Decision (kind)' +
            '<select name="kind" required>' +
              '<option value="available">available — In stock & confirmed</option>' +
              '<option value="unavailable">unavailable — Out of stock</option>' +
              '<option value="alternative">alternative — Different strength/form/brand (Option only)</option>' +
            '</select>' +
          '</label>' +
          '<label class="field">Quantity Available' +
            '<input type="number" name="quantity" min="0" max="1000" step="1" value="'+Number(item.quantity)+'">' +
          '</label>' +
          '<label class="field">Pharmacist Notes / Price / Alternative Details' +
            '<input type="text" name="text" maxlength="500" placeholder="e.g. Ready for pickup ($12.00) or 625mg tablets only">' +
          '</label>' +
          '<button class="btn" type="submit">Send Reply '+icon('check')+'</button>' +
        '</form>' +
      '</div>'
    ) : (
      '<p class="caption" style="margin-top:10px">' +
        (item.target_status === 'cancelled'
          ? 'This enquiry was withdrawn/cancelled by the requesting pharmacy.'
          : `Reply already recorded (${item.target_status}).`) +
      '</p>'
    )) +
  '</article>';
}

function pharmacyOverview() {
  const branchName = liveSession?.branch?.name || 'Pharmacy Branch';
  const staffName = liveSession?.staff?.full_name || '';
  const waitingInbox = liveState.inboxItems.filter(i => i.target_status === 'sent' || i.target_status === 'no_response');
  const hitlEnquiries = liveState.outgoingEnquiries.filter(e => e.status === 'awaiting_clarification' || e.status === 'awaiting_approval');
  const activeOutgoing = liveState.outgoingEnquiries.filter(e => e.status !== 'closed' && e.status !== 'cancelled');

  return wzPharmGreet() + '<section class="office-door">' +
      title('Pharmacy Office', `${branchName} · ${staffName} (${liveSession?.staff?.email || ''})`, chip('<span class="live-pulse"></span> Connected to ' + API_BASE_URL, 'live')) +
      '<div class="actions" style="margin-top:16px">' +
        btn('+ New Agent Enquiry', 'open-new-enquiry') +
        appLink('Open Peer Inbox (' + waitingInbox.length + ')', 'approvals', 'secondary') +
        appLink('Open Stockroom ' + icon('arrow'), 'inventory', 'secondary') +
      '</div>' +
    '</section>' +
    renderNotificationsBanner() +
    quickCreateAgentEnquiryCard() +
    '<div class="queue-overview">' +
      [
        ['shield', 'Peer Inbox (Needs Reply)', waitingInbox.length, 'approvals'],
        ['pulse', 'HITL Checkpoints Waiting', hitlEnquiries.length, 'network'],
        ['link', 'Active Outgoing Enquiries', activeOutgoing.length, 'network']
      ].map(([symbol, name, count, route]) =>
        '<a class="card queue-metric" href="#pharmacist/' + route + '">' + tile(symbol) + '<strong>' + count + '</strong><span>' + t(name) + '</span>' + icon('arrow') + '</a>'
      ).join('') +
    '</div>' +
    (hitlEnquiries.length ? (
      '<section style="margin-top:24px">' +
        '<div class="row split"><h2>Human-in-the-Loop Action Required (' + hitlEnquiries.length + ')</h2></div>' +
        '<div class="stack">' + hitlEnquiries.map(renderLiveEnquiryCard).join('') + '</div>' +
      '</section>'
    ) : '') +
    '<section style="margin-top:24px">' +
      '<div class="row split"><h2>Peer Pharmacy Inbox — Incoming Enquiries (' + waitingInbox.length + ' waiting)</h2>' + appLink('Open full Reply Desk', 'approvals', 'secondary small') + '</div>' +
      (liveState.inboxItems.length
        ? '<div class="stack">' + liveState.inboxItems.slice(0, 5).map(renderInboxItemCard).join('') + '</div>'
        : empty('No incoming peer pharmacy requests in ' + branchName + '’s inbox yet. Switch to a contacted target branch (B–E) after sending an enquiry!')) +
    '</section>' +
    '<section style="margin-top:28px">' +
      '<div class="row split"><h2>Outgoing Network Enquiries (' + liveState.outgoingEnquiries.length + ')</h2>' + appLink('View all network enquiries', 'network', 'secondary small') + '</div>' +
      (liveState.outgoingEnquiries.length
        ? '<div class="stack">' + liveState.outgoingEnquiries.slice(0, 8).map(renderLiveEnquiryCard).join('') + '</div>'
        : empty('No outgoing enquiries created from ' + branchName + ' yet. Use the form above to launch a Wassit Agent enquiry.')) +
    '</section>';
}

function networkEnquiriesView() {
  const outgoing = liveState.outgoingEnquiries;
  const hitl = outgoing.filter(e => e.status === 'awaiting_clarification' || e.status === 'awaiting_approval');
  const openList = outgoing.filter(e => e.status !== 'closed' && e.status !== 'cancelled');
  const closedList = outgoing.filter(e => e.status === 'closed' || e.status === 'cancelled');

  const tabs = [
    ['all', 'All Outgoing', outgoing.length],
    ['hitl', 'Needs My Action (Clarify / Approve)', hitl.length],
    ['open', 'Active / Waiting', openList.length],
    ['inbox', 'Incoming Peer Inbox', liveState.inboxItems.length],
    ['closed', 'Completed / Closed', closedList.length]
  ];

  let listHTML = '';
  if (selectedLiveQueue === 'inbox') {
    listHTML = liveState.inboxItems.length
      ? '<div class="stack">' + liveState.inboxItems.map(renderInboxItemCard).join('') + '</div>'
      : empty('No incoming requests in your branch inbox.');
  } else {
    const filtered = selectedLiveQueue === 'hitl' ? hitl
      : selectedLiveQueue === 'open' ? openList
      : selectedLiveQueue === 'closed' ? closedList
      : outgoing;
    listHTML = filtered.length
      ? '<div class="stack">' + filtered.map(renderLiveEnquiryCard).join('') + '</div>'
      : empty('No enquiries match this filter.');
  }

  return title('Network enquiries', 'Create agent-handled enquiries, monitor live SSE events, and approve targets.') +
    renderNotificationsBanner() +
    quickCreateAgentEnquiryCard() +
    '<div class="queue-filters" role="group" aria-label="Filter network enquiries">' +
      tabs.map(([key, name, count]) =>
        '<button type="button" data-action="live-queue:' + key + '" aria-pressed="' + (selectedLiveQueue === key) + '">' + escapeHTML(name) + ' <span>' + count + '</span></button>'
      ).join('') +
    '</div>' +
    listHTML;
}

function peerInboxAndApprovalsView() {
  const hitl = liveState.outgoingEnquiries.filter(e => e.status === 'awaiting_clarification' || e.status === 'awaiting_approval');
  return title('Peer Pharmacy Inbox & HITL Desk', `Incoming availability requests for ${liveSession?.branch?.name || 'this branch'} and pending requester checkpoints.`) +
    renderNotificationsBanner() +
    (hitl.length ? (
      '<section style="margin-top:20px">' +
        '<h2>Requester Checkpoints Awaiting Your Action (' + hitl.length + ')</h2>' +
        '<div class="stack">' + hitl.map(renderLiveEnquiryCard).join('') + '</div>' +
      '</section>'
    ) : '') +
    '<section style="margin-top:22px">' +
      '<div class="row split">' +
        '<h2>Incoming Peer Pharmacy Requests (' + liveState.inboxItems.length + ')</h2>' +
        btn('Refresh Inbox', 'refresh-live', 'secondary small') +
      '</div>' +
      '<p>Only a human pharmacist at the contacted target branch can reply <code>available</code>, <code>unavailable</code>, or <code>alternative</code> (Contract Rule 2). Switch branches in the top bar to reply on behalf of B, C, D, or E.</p>' +
      (liveState.inboxItems.length
        ? '<div class="stack">' + liveState.inboxItems.map(renderInboxItemCard).join('') + '</div>'
        : empty('No incoming requests waiting in ' + (liveSession?.branch?.name || 'this branch') + '’s inbox. Note: Outbound messages wait in the requester’s outbox for UNDO_WINDOW before appearing here.')) +
    '</section>';
}

function inventoryView() {
  const branchName = liveSession?.branch?.name || pharmacies[pharmacyId()][0];
  const liveMeds = liveState.medications;

  if (liveMeds.length > 0) {
    const q = searchTerm.trim().toLowerCase();
    const filtered = liveMeds.filter(m =>
      !q || `${m.brand_name} ${m.active_ingredient} ${m.strength} ${m.dosage_form} ${m.therapeutic_class}`.toLowerCase().includes(q)
    );
    return title('Medicine stockroom', `Live catalogue & reported batch inventory at ${branchName}`) +
      renderNotificationsBanner() +
      '<div class="card stock-explainer">' +
        '<h3>Live Catalogue & Stock Routing</h3>' +
        '<p>Stock counts below come from <code>GET /api/v1/branches/' + escapeHTML(liveSession?.branch?.id || '') + '/inventory</code> (unexpired, unrecalled batches). When a medicine is out of stock, click <strong>Ask network (Wassit Agent)</strong> to launch an agent-handled enquiry.</p>' +
      '</div>' +
      '<form id="search-form" class="search"><label class="field">' + t('Search') + '<input type="search" name="query" value="' + escapeHTML(searchTerm) + '" placeholder="Search brand, active ingredient, strength…"></label><button class="btn">' + t('Search') + '</button></form>' +
      '<div id="matches" style="margin-top:18px">' +
        (filtered.length ? '<div class="grid stock-grid">' + filtered.map((m, i) => {
          const count = availableStockForMed(m.id) ?? 0;
          const batches = liveState.branchInventory.filter(b => b.medication_id === m.id);
          const recalled = batches.some(b => b.is_recalled);
          const expired = batches.some(b => b.expiry_date && Date.parse(b.expiry_date) <= Date.now());
          const color = packageColors[i % packageColors.length];
          return '<article class="card stock-bin">' +
            '<span class="carton ' + color + '" aria-hidden="true"><span class="carton-front"><span class="carton-brand">' + icon('mark') + ' DOSYN</span><strong>' + escapeHTML(m.brand_name) + '</strong><span>' + escapeHTML(m.strength) + '</span><small>' + escapeHTML(m.dosage_form) + '</small><span class="carton-demo">' + escapeHTML(m.id) + '</span></span></span>' +
            '<h3>' + escapeHTML(m.brand_name) + ' · ' + escapeHTML(m.strength) + '</h3>' +
            '<p>' + escapeHTML(m.active_ingredient) + ' · ' + escapeHTML(m.therapeutic_class) + ' (' + escapeHTML(m.dosage_form) + ')</p>' +
            '<div class="row" style="margin-top:8px">' +
              chip(count > 0 ? `In stock · ${count} units` : 'Out of stock · 0 units', count > 0 ? 'live' : 'warm') +
              (recalled ? chip('Batch Recall Hold', 'danger') : '') +
              (expired ? chip('Expired Batch Excluded', 'neutral') : '') +
            '</div>' +
            '<div class="actions">' +
              btn('Ask network (Wassit Agent)', 'live-enquire-med:' + m.id, count === 0 ? '' : 'secondary small') +
            '</div>' +
          '</article>';
        }).join('') + '</div>' : empty('No catalogue medications match this search.')) +
      '</div>';
  }

  // Fallback if catalogue not loaded yet
  const quantities = stock[pharmacyId()];
  const items = meds.map((_,i)=>i).filter(i=>(category<0 || categories[category][1].includes(i)) && label(i).toLowerCase().includes(searchTerm.trim().toLowerCase())).sort((a,b)=>Number(!quantities[a])-Number(!quantities[b]));
  return title('Medicine stockroom', 'Stock records at ' + branchName) +
    '<div class="grid stock-grid">' + items.map(i => '<article class="card stock-bin">' + carton(i) + '<h3>' + label(i) + '</h3>' + chip(stockStatus(quantities[i]), quantities[i] ? '' : 'warm') + '</article>').join('') + '</div>';
}

function notificationsAndStatsView() {
  const totals = liveState.stats?.totals;
  const rows = liveState.stats?.data || [];
  return title('Notifications & Agent Telemetry', 'Live Wassit agent run metrics, HITL counts, and token cost tracking.') +
    renderNotificationsBanner() +
    (totals ? (
      '<div class="queue-overview">' +
        '<div class="card queue-metric">' + tile('pulse') + '<strong>' + Number(totals.agent_enquiries || 0) + ' / ' + Number(totals.enquiries || 0) + '</strong><span>Agent / Total Enquiries</span></div>' +
        '<div class="card queue-metric">' + tile('shield') + '<strong>' + Number(totals.hitl_count || 0) + '</strong><span>Total HITL Checkpoints</span></div>' +
        '<div class="card queue-metric">' + tile('clock') + '<strong>' + Number(totals.agent_runs || 0) + ' runs (' + Number(totals.agent_steps || 0) + ' steps)</strong><span>Cost: $' + (Number(totals.cost_micros || 0) / 1e6).toFixed(6) + '</span></div>' +
      '</div>'
    ) : '') +
    '<h2>Per-Enquiry Telemetry (<code>GET /api/v1/stats</code>)</h2>' +
    (rows.length ? (
      '<div class="stack">' + rows.map(r =>
        '<article class="card request">' +
          '<div>' +
            '<h3>' + escapeHTML(r.raw_request) + '</h3>' +
            '<p>Handled by: <strong>' + escapeHTML(r.handled_by) + '</strong> · Runs: ' + Number(r.agent_runs) + ' · Steps: ' + Number(r.agent_steps) + ' · Tokens: ' + Number(r.input_tokens) + ' in / ' + Number(r.output_tokens) + ' out · Cost: $' + (Number(r.cost_micros || 0) / 1e6).toFixed(6) + '</p>' +
          '</div>' +
          '<div class="actions">' +
            chip('HITL: ' + Number(r.hitl_count), 'info') +
            enquiryStatusBadge(r) +
            btn('Inspect', 'open-live-enquiry:' + r.enquiry_id, 'secondary small') +
          '</div>' +
        '</article>'
      ).join('') + '</div>'
    ) : empty('No enquiry telemetry recorded yet.'));
}

function pharmacistAccountView() {
  return title('Branch & Session Switcher', 'Switch between the 5 seeded demo pharmacy branches or manage your opaque Bearer session.') +
    renderNotificationsBanner() +
    '<article class="card">' +
      '<div class="row split">' +
        '<div>' +
          '<h3>' + escapeHTML(liveSession?.staff?.full_name || 'Not signed in') + '</h3>' +
          '<p>' + escapeHTML(liveSession?.branch?.name || '') + ' · <code>' + escapeHTML(liveSession?.staff?.email || '') + '</code> · Branch ID: <code>' + escapeHTML(liveSession?.branch?.id || '') + '</code></p>' +
        '</div>' +
        chip('Opaque Session Active', 'live') +
      '</div>' +
      '<h3 style="margin-top:18px">Switch Demo Branch (PIN 1234)</h3>' +
      '<div class="preset-grid">' +
        DEMO_BRANCH_PRESETS.map(b =>
          '<button type="button" class="preset-card" data-action="quick-branch:' + b.email + '" aria-pressed="' + (liveSession?.staff?.email === b.email) + '">' +
            '<span class="tag ' + (b.code === 'A' ? 'live' : 'neutral') + '">Branch ' + b.code + '</span>' +
            '<strong>' + escapeHTML(b.name) + '</strong>' +
            '<small>' + escapeHTML(b.email) + ' · ' + escapeHTML(b.roleHint) + '</small>' +
          '</button>'
        ).join('') +
      '</div>' +
      '<div class="actions" style="margin-top:16px">' +
        btn('Sign out of session', 'staff-logout', 'secondary') +
      '</div>' +
    '</article>';
}

// ---- Structured Offers (GET /enquiries/{id}/offers, POST /targets/{id}/offers) ----
// The model (or a pharmacist) reads a reply into fields with exact quotes; Go rules decide.
const OFFER_FIELDS = [
  ['in_stock', 'In stock'], ['strength', 'Strength'], ['quantity', 'Quantity'], ['price', 'Price'],
  ['pickup_by', 'Pickup by'], ['payment', 'Payment'], ['needs_prescription', 'Needs prescription']
];

function offerDecisionLabel(decision) {
  if (decision === 'matches') return 'Matches request';
  if (decision === 'escalated') return 'Needs a human';
  if (decision === 'rejected') return 'Rejected';
  return decision || 'Pending';
}

function offerDecisionChip(offer) {
  const tone = offer.decision === 'matches' ? 'live' : offer.decision === 'escalated' ? 'warm' : 'neutral';
  const who = offer.decided_by === 'human' ? 'Human' : 'Rules';
  return '<span class="tag ' + tone + ' offer-decision">' + escapeHTML(who) + ' · ' + escapeHTML(offerDecisionLabel(offer.decision)) + '</span>';
}

// highlightEvidence escapes text and wraps every quoted span in <mark> (case-insensitive,
// whitespace-tolerant, like the backend's check). Overlapping quotes are merged.
function highlightEvidence(text, evidence) {
  const raw = String(text || '');
  const ranges = [];
  for (const [field, quote] of Object.entries(evidence || {})) {
    const words = String(quote || '').trim().split(/\s+/).filter(Boolean);
    if (!words.length) continue;
    const pattern = words.map(w => w.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('\\s+');
    let re;
    try { re = new RegExp(pattern, 'giu'); } catch { continue; }
    for (const m of raw.matchAll(re)) {
      if (m[0].length) ranges.push({ start: m.index, end: m.index + m[0].length, fields: [field] });
    }
  }
  ranges.sort((a, b) => a.start - b.start || b.end - a.end);
  const merged = [];
  for (const r of ranges) {
    const last = merged[merged.length - 1];
    if (last && r.start < last.end) {
      last.end = Math.max(last.end, r.end);
      for (const f of r.fields) if (!last.fields.includes(f)) last.fields.push(f);
    } else {
      merged.push({ ...r, fields: [...r.fields] });
    }
  }
  let out = '';
  let pos = 0;
  for (const r of merged) {
    out += escapeHTML(raw.slice(pos, r.start));
    const label = r.fields.map(f => (OFFER_FIELDS.find(x => x[0] === f) || [f, f])[1]).join(', ');
    out += '<mark class="evidence" title="' + escapeHTML(label) + '">' + escapeHTML(raw.slice(r.start, r.end)) + '</mark>';
    pos = r.end;
  }
  return out + escapeHTML(raw.slice(pos));
}

function offerFieldValue(offer, field) {
  const yesNo = v => v === true ? 'Yes' : v === false ? 'No' : '';
  switch (field) {
    case 'in_stock': return yesNo(offer.in_stock);
    case 'needs_prescription': return yesNo(offer.needs_prescription);
    case 'quantity': return offer.quantity === null || offer.quantity === undefined ? '' : String(offer.quantity);
    case 'price':
      if (offer.price_cents === null || offer.price_cents === undefined) return '';
      return (Number(offer.price_cents) / 100).toLocaleString(undefined, { maximumFractionDigits: 2 }) + ' ' + (offer.currency || '');
    default: return offer[field] || '';
  }
}

function buildOfferCardHTML(offer) {
  const evidence = offer.evidence && typeof offer.evidence === 'object' ? offer.evidence : {};
  const reasons = Array.isArray(offer.reasons) ? offer.reasons : [];
  const rows = OFFER_FIELDS.map(([field, label]) => {
    const value = offerFieldValue(offer, field);
    if (!value) return '';
    const quote = evidence[field];
    return '<div><dt>' + escapeHTML(t(label)) + '</dt><dd>' + escapeHTML(value) +
      (quote ? ' <q class="evidence-quote" dir="auto">' + escapeHTML(quote) + '</q>' : ' <small class="offer-unquoted">no quote</small>') +
      '</dd></div>';
  }).join('');
  return '<article class="offer-card offer-' + escapeHTML(offer.decision || 'pending') + '">' +
    '<header class="row split">' +
      '<strong>' + escapeHTML(branchNameById(offer.branch_id)) + '</strong>' +
      offerDecisionChip(offer) +
    '</header>' +
    '<div class="offer-grid">' +
      '<div class="offer-source">' +
        '<span class="eyebrow">Pharmacist message</span>' +
        '<blockquote dir="auto">' + highlightEvidence(offer.source_text, evidence) + '</blockquote>' +
        '<small>Read by ' + (offer.extracted_by === 'agent' ? 'Wassit agent (reads only, never decides)' : 'pharmacy staff') +
          ' · confidence ' + Math.round(Number(offer.confidence || 0) * 100) + '% · ' + date(offer.created_at) + '</small>' +
      '</div>' +
      '<div class="offer-fields">' +
        '<span class="eyebrow">Structured offer</span>' +
        (rows ? '<dl>' + rows + '</dl>' : '<p class="caption">No fields read.</p>') +
      '</div>' +
    '</div>' +
    (reasons.length ? '<ul class="offer-reasons">' + reasons.map(r => '<li>' + escapeHTML(r) + '</li>').join('') + '</ul>' : '') +
  '</article>';
}

function buildOfferReadFormHTML(target) {
  const isOpen = liveState.openOfferForms?.has(target.id);
  const quote = (field, placeholder) => '<input type="text" name="ev_' + field + '" maxlength="200" dir="auto" placeholder="' + escapeHTML(placeholder) + '" aria-label="Quote for ' + escapeHTML(field) + '">';
  return '<details class="offer-read" data-offer-form="' + escapeHTML(target.id) + '"' + (isOpen ? ' open' : '') + '>' +
    '<summary>Read offer from ' + escapeHTML(target.branch_name || branchNameById(target.branch_id)) + '\'s reply</summary>' +
    '<p class="caption">Reply: <q dir="auto">' + escapeHTML(target.reply_text) + '</q>. Copy the exact words that support each field; the rules check every quote against the reply.</p>' +
    '<form class="offer-read-form" data-target-id="' + escapeHTML(target.id) + '">' +
      '<div class="offer-form-grid">' +
        '<label class="field">In stock<select name="in_stock"><option value="">not said</option><option value="true">yes</option><option value="false">no</option></select></label>' + quote('in_stock', 'e.g. 3ande') +
        '<label class="field">Strength<input type="text" name="strength" maxlength="40" placeholder="e.g. 625mg"></label>' + quote('strength', 'e.g. 625') +
        '<label class="field">Quantity<input type="number" name="quantity" min="0" max="1000"></label>' + quote('quantity', 'e.g. 2 boxes') +
        '<label class="field">Price<span class="row offer-price"><input type="number" name="price" min="0" step="0.01" placeholder="0.00"><input type="text" name="currency" maxlength="8" placeholder="USD"></span></label>' + quote('price', 'e.g. 5$') +
        '<label class="field">Pickup by<input type="text" name="pickup_by" maxlength="80" placeholder="e.g. before 18:00"></label>' + quote('pickup_by', 'e.g. abel l 6') +
        '<label class="field">Payment<select name="payment"><option value="">not said</option><option value="cash">cash</option><option value="card">card</option><option value="any">any</option></select></label>' + quote('payment', 'e.g. cash') +
        '<label class="field">Needs prescription<select name="needs_prescription"><option value="">not said</option><option value="true">yes</option><option value="false">no</option></select></label>' + quote('needs_prescription', 'e.g. badna wasfe') +
        '<label class="field">Confidence (0 to 1)<input type="number" name="confidence" min="0" max="1" step="0.05" value="0.9" required></label>' +
      '</div>' +
      '<button class="btn small" type="submit">Record offer · rules decide</button>' +
    '</form>' +
  '</details>';
}

function buildOffersSectionHTML(detail, isRequester) {
  if (!isRequester) return '';
  const offers = liveState.activeOffers || [];
  const replied = (detail.targets || []).filter(t =>
    ['available', 'unavailable', 'alternative_offered'].includes(t.status) && String(t.reply_text || '').trim());
  if (!offers.length && !replied.length) return '';
  return '<section class="offers-section" aria-labelledby="offers-heading">' +
    '<div class="row split"><h3 id="offers-heading">Pharmacy Offers (' + offers.length + ')</h3>' +
      chip('AI reads · Rules decide · Humans approve', 'info') + '</div>' +
    (liveState.offersNotice ? '<p class="notice" role="status">' + escapeHTML(liveState.offersNotice) + '</p>' : '') +
    (offers.length ? offers.map(buildOfferCardHTML).join('') : '<p class="caption">No offers read yet. Read one from a reply below.</p>') +
    replied.map(buildOfferReadFormHTML).join('') +
  '</section>';
}

// ---- Simulated Call (POST /api/v1/speech, audio bytes) --------------------------
// Built from sent outbound messages (agent voice) and pharmacist replies (pharmacy voice).
function buildCallLines(detail) {
  const targets = detail.targets || [];
  const lines = [];
  for (const m of detail.messages || []) {
    if (!m.target_id || m.status !== 'sent' || !['enquiry', 'follow_up', 'retraction'].includes(m.kind) || !m.body) continue;
    const tgt = targets.find(x => x.id === m.target_id);
    lines.push({ at: m.sent_at || m.created_at, speaker: 'agent', who: 'Wassit → ' + (tgt?.branch_name || branchNameById(tgt?.branch_id)), text: m.body });
  }
  for (const tgt of targets) {
    if (!String(tgt.reply_text || '').trim()) continue;
    lines.push({ at: tgt.replied_at || tgt.created_at, speaker: 'pharmacy', who: tgt.branch_name || branchNameById(tgt.branch_id), text: tgt.reply_text });
  }
  return lines.sort((a, b) => Date.parse(a.at || 0) - Date.parse(b.at || 0));
}

function buildCallSectionHTML(detail) {
  const lines = buildCallLines(detail);
  if (!lines.length) return '';
  const call = liveState.call;
  const playing = Boolean(call?.playing && call.enquiryId === detail.enquiry.id);
  return '<section class="call-section" aria-labelledby="call-heading">' +
    '<div class="row split"><h3 id="call-heading">Simulated Call (' + lines.length + ' lines)</h3>' +
      '<div class="actions">' +
        (playing ? btn('Stop', 'stop-call', 'secondary small') : btn('Play call', 'play-call', 'small')) +
      '</div>' +
    '</div>' +
    '<p class="caption">Text to speech of the real messages and replies. Nothing is dialled.</p>' +
    (liveState.callNotice ? '<p class="notice" role="status">' + escapeHTML(liveState.callNotice) + '</p>' : '') +
    '<ol class="call-lines">' + lines.map((l, i) =>
      '<li class="call-line ' + l.speaker + (playing && call.index === i ? ' playing' : '') + '" data-call-index="' + i + '">' +
        '<span class="call-who">' + escapeHTML(l.who) + '</span>' +
        '<span class="call-text" dir="auto">' + escapeHTML(l.text) + '</span>' +
      '</li>').join('') +
    '</ol>' +
  '</section>';
}

function markCallLine(index) {
  dialog.querySelectorAll('.call-line').forEach(el => el.classList.toggle('playing', Number(el.dataset.callIndex) === index));
}

function stopCall() {
  const call = liveState.call;
  if (!call) return;
  call.stopped = true;
  call.playing = false;
  if (call.audio) { call.audio.pause(); call.audio = null; }
  if (call.finish) { call.finish(); call.finish = null; }
}

async function playCall() {
  const detail = liveState.activeDetail;
  if (!detail?.enquiry) return;
  stopCall();
  const lines = buildCallLines(detail);
  const call = { enquiryId: detail.enquiry.id, index: 0, playing: true, stopped: false, audio: null, finish: null };
  liveState.call = call;
  liveState.callNotice = '';
  renderLiveEnquiryModal(true);
  const fetchLine = i => apiAudio('POST', '/api/v1/speech', { text: lines[i].text, speaker: lines[i].speaker });
  try {
    let next = lines.length ? fetchLine(0) : null;
    for (let i = 0; i < lines.length && !call.stopped; i++) {
      const blob = await next;
      next = i + 1 < lines.length ? fetchLine(i + 1) : null;
      next?.catch(() => {});
      if (call.stopped) break;
      call.index = i;
      markCallLine(i);
      const url = URL.createObjectURL(blob);
      try {
        await new Promise(resolve => {
          const audio = new Audio(url);
          call.audio = audio;
          call.finish = resolve;
          audio.onended = resolve;
          audio.onerror = resolve;
          audio.play().catch(resolve);
        });
      } finally {
        URL.revokeObjectURL(url);
      }
    }
  } catch (err) {
    liveState.callNotice = err.status === 409
      ? 'Speech is off on the backend (set SPEECH_PROVIDER to say or azure). The transcript is shown instead.'
      : 'Could not play the call: ' + err.message;
  }
  if (liveState.call === call) {
    call.playing = false;
    call.audio = null;
    if (dialog.open && liveState.activeEnquiryId === call.enquiryId) renderLiveEnquiryModal(true);
  }
}

// ---- Live Enquiry Detail Modal (SSE + Clarify + Approve + Undo + Cancel) ----
async function openLiveEnquiry(enquiryId) {
  liveState.activeEnquiryId = enquiryId;
  liveState.deselectedTargetIds = new Set();
  liveState.seenEventIds = new Set();
  liveState.actionError = '';
  liveState.actionNotice = '';
  liveState.offersNotice = '';
  liveState.callNotice = '';
  liveState.openOfferForms = new Set();
  stopCall();
  await fetchAndRenderLiveEnquiry(enquiryId, false);
  connectLiveSSE(enquiryId);
}

async function fetchAndRenderLiveEnquiry(enquiryId, isRefresh = false) {
  const detail = await apiCall('GET', `/api/v1/enquiries/${encodeURIComponent(enquiryId)}`);
  liveState.activeDetail = detail;
  if (Array.isArray(detail.events)) {
    for (const ev of detail.events) {
      if (ev?.id) liveState.seenEventIds.add(ev.id);
    }
  }
  // Fetch ranked candidates if requester is viewing and medication is set
  const enq = detail.enquiry;
  if (enq?.medication_id && liveSession?.branch?.id === enq.requester_branch_id && enq.status !== 'closed' && enq.status !== 'cancelled') {
    try {
      const candRes = await apiCall('GET', `/api/v1/enquiries/${encodeURIComponent(enquiryId)}/candidates?limit=5&radius_km=15`);
      liveState.activeCandidates = Array.isArray(candRes?.data) ? candRes.data : [];
    } catch {
      liveState.activeCandidates = [];
    }
  } else {
    liveState.activeCandidates = [];
  }
  // Offers are visible to the requesting pharmacy only (target pharmacies never see each other's).
  if (liveSession?.branch?.id === enq?.requester_branch_id) {
    try {
      const offRes = await apiCall('GET', `/api/v1/enquiries/${encodeURIComponent(enquiryId)}/offers`);
      liveState.activeOffers = Array.isArray(offRes?.data) ? offRes.data : [];
    } catch {
      liveState.activeOffers = [];
    }
  } else {
    liveState.activeOffers = [];
  }
  renderLiveEnquiryModal(isRefresh);
}

function buildLiveEnquiryModalHTML(detail) {
  const enq = detail.enquiry;
  const med = detail.medication;
  const targets = detail.targets || [];
  const messages = detail.messages || [];
  const events = detail.events || [];
  const runs = detail.runs || [];
  const isRequester = liveSession?.branch?.id === enq.requester_branch_id;
  const myTarget = targets.find(t => t.branch_id === liveSession?.branch?.id);
  const isFinalState = enq.status === 'closed' || enq.status === 'cancelled';

  const proposedTargets = targets.filter(t => t.status === 'proposed');
  const approvedTargets = targets.filter(t => t.status === 'approved');
  const availableTargets = targets.filter(t => t.status === 'available');
  const activeTargets = targets.filter(t => t.status !== 'cancelled');
  const allUnavailable = activeTargets.length > 0 && activeTargets.every(t => t.status === 'unavailable');

  // Notifications from agent to requester
  const notifications = messages.filter(m => m.kind === 'notification');
  // Queued outbox messages (inside undo window)
  const queuedMessages = messages.filter(m => m.status === 'queued' && m.kind !== 'notification');

  // Find latest reversible event (especially messages_queued or targets_proposed)
  const reversibleEvents = [...events].reverse().filter(ev => ev.inverse !== null && ev.inverse !== undefined && String(ev.inverse) !== 'null' && !ev.undone_at);
  const reversibleSendEvent = reversibleEvents.find(ev => ev.type === 'messages_queued');

  const totalTokensIn = runs.reduce((acc, r) => acc + Number(r.input_tokens || 0), 0);
  const totalTokensOut = runs.reduce((acc, r) => acc + Number(r.output_tokens || 0), 0);
  const totalSteps = runs.reduce((acc, r) => acc + Number(r.steps || 0), 0);
  const totalCostUsd = (runs.reduce((acc, r) => acc + Number(r.cost_micros || 0), 0) / 1e6).toFixed(6);

  // 1. Header & Summary Card
  let html = '<div class="request-detail">' +
    '<div class="row split" style="margin-bottom:10px">' +
      '<div class="row">' +
        enquiryStatusBadge(enq) +
        chip(enq.handled_by === 'agent' ? 'Wassit AI Agent' : 'Manual', enq.handled_by === 'agent' ? 'live' : 'neutral') +
        chip('HITL Checkpoints: ' + Number(detail.hitl_count || 0), 'info') +
      '</div>' +
      '<span id="sse-status-pill" class="tag ' + (liveState.sseState === 'connected' ? 'live' : 'info') + '">' +
        (liveState.sseState === 'connected' ? '<span class="live-pulse"></span> SSE Live Stream Connected' : 'SSE Stream Active') +
      '</span>' +
    '</div>' +
    (liveState.actionError ? '<div class="toast-error" role="alert">' + escapeHTML(liveState.actionError) + '</div>' : '') +
    (liveState.actionNotice ? '<p class="notice" role="status">' + escapeHTML(liveState.actionNotice) + '</p>' : '') +
    '<article class="confirmation-card ' + (enq.outcome === 'available' ? 'available' : enq.outcome === 'unavailable' ? 'unavailable' : '') + '">' +
      '<dl>' +
        '<div><dt>Requesting Pharmacy</dt><dd>' + escapeHTML(branchNameById(enq.requester_branch_id)) + '</dd></div>' +
        '<div><dt>Matched Catalogue Medicine</dt><dd>' + (med ? escapeHTML(`${med.brand_name} ${med.strength} · ${med.dosage_form} (${med.active_ingredient})`) : '<em>Pending match / clarification</em>') + '</dd></div>' +
        '<div><dt>Quantity Requested</dt><dd><strong>×' + Number(enq.quantity) + '</strong></dd></div>' +
        '<div><dt>Outcome</dt><dd>' + (enq.outcome ? '<strong>' + escapeHTML(enq.outcome) + '</strong>' + (enq.outcome_branch_id ? ' at ' + escapeHTML(branchNameById(enq.outcome_branch_id)) : '') : 'In progress') + '</dd></div>' +
      '</dl>' +
      '<div class="telemetry-bar">' +
        '<span><strong>Enquiry ID:</strong> <code>' + escapeHTML(enq.id.slice(0, 8)) + '…</code></span>' +
        '<span><strong>Agent Runs:</strong> ' + runs.length + ' (' + totalSteps + ' tool steps)</span>' +
        '<span><strong>Tokens:</strong> ' + totalTokensIn + ' in / ' + totalTokensOut + ' out</span>' +
        '<span><strong>Cost:</strong> $' + totalCostUsd + '</span>' +
      '</div>' +
    '</article>';

  // 2. Agent Notifications (e.g., Alternative Offered warning)
  if (notifications.length > 0) {
    html += '<div class="hitl-card" style="border-color:#38bdf8">' +
      '<h3>' + icon('bell') + ' Wassit Agent Notifications to Requester</h3>' +
      notifications.map(n =>
        '<p style="margin-top:6px"><strong>' + escapeHTML(n.body) + '</strong> <small>(' + date(n.created_at) + ')</small></p>'
      ).join('') +
    '</div>';
  }

  // 3. HITL Checkpoint A: Clarification Needed (POST /api/v1/enquiries/{id}/clarify)
  if (isRequester && enq.status === 'awaiting_clarification') {
    html += '<section class="hitl-card" aria-labelledby="clarify-heading">' +
      '<div class="row split"><h3 id="clarify-heading">Human-in-the-Loop Checkpoint: Clarification Needed</h3>' + chip('POST /enquiries/{id}/clarify', 'warm') + '</div>' +
      '<p style="font-size:1.05rem;margin:6px 0"><strong>Wassit Agent asks:</strong> "' + escapeHTML(enq.clarification_question || 'Please clarify the exact strength and dosage form.') + '"</p>' +
      '<div class="preset-chips">' +
        ['1000mg tablet', '625mg tablet', '500mg tablet', '20mg tablet', '100mcg inhaler'].map(ans =>
          '<button type="button" class="preset-chip" data-action="fill-clarify:' + escapeHTML(ans) + '">' + escapeHTML(ans) + '</button>'
        ).join('') +
      '</div>' +
      '<form id="live-clarify-form" class="form" data-enquiry-id="' + escapeHTML(enq.id) + '" style="margin-top:10px">' +
        '<div class="row" style="align-items:stretch">' +
          '<input type="text" name="answer" id="clarify-answer-input" placeholder="e.g. 1000mg tablets" maxlength="1000" required style="flex:1;min-height:44px;border-radius:8px;padding:8px 12px;border:1px solid var(--line);background:var(--white);color:var(--ink)">' +
          '<button class="btn" type="submit">Send Clarification Answer ' + icon('arrow') + '</button>' +
        '</div>' +
      '</form>' +
    '</section>';
  }

  // 4. HITL Checkpoint B: Proposed Targets Approval (POST /api/v1/enquiries/{id}/targets/approve)
  if (isRequester && proposedTargets.length > 0 && !isFinalState) {
    const selectedIds = proposedTargets.filter(t => !liveState.deselectedTargetIds.has(t.id)).map(t => t.id);
    const proposeEv = reversibleEvents.find(ev => ev.type === 'targets_proposed');
    html += '<section class="hitl-card" aria-labelledby="approve-heading">' +
      '<div class="row split"><h3 id="approve-heading">Human-in-the-Loop Checkpoint: Approve Target Pharmacies</h3>' + chip('POST /enquiries/{id}/targets/approve', 'info') + '</div>' +
      '<p>Wassit Agent ranked nearby participating branches by stock and distance. Select which pharmacies to contact:</p>' +
      '<div class="target-check-list">' +
        proposedTargets.map(t => {
          const checked = !liveState.deselectedTargetIds.has(t.id);
          const cand = liveState.activeCandidates.find(c => c.branch_id === t.branch_id);
          return '<label class="target-check-item">' +
            '<div class="row">' +
              '<input type="checkbox" class="target-approve-cb" value="' + escapeHTML(t.id) + '" ' + (checked ? 'checked' : '') + '>' +
              '<div>' +
                '<strong>' + escapeHTML(t.branch_name) + '</strong> <small>(' + escapeHTML(t.branch_id) + ')</small>' +
                (cand ? `<p class="caption">${cand.distance_km} km away · Reported stock: ${cand.reported_quantity} units</p>` : '') +
              '</div>' +
            '</div>' +
            targetStatusBadge(t.status) +
          '</label>';
        }).join('') +
      '</div>' +
      '<div class="actions">' +
        btn(`Approve Selected Pharmacies (${selectedIds.length})`, 'approve-targets:' + enq.id, '', selectedIds.length === 0) +
        (proposeEv ? btn('Undo Proposal', 'undo-event:' + proposeEv.id, 'secondary small') : '') +
      '</div>' +
    '</section>';
  }

  // 5. Outbox Undo Window (POST /api/v1/messages/{id}/cancel)
  if (isRequester && queuedMessages.length > 0 && !isFinalState) {
    html += '<section class="hitl-card" style="border-color:#8ad4ba">' +
      '<div class="row split"><h3>Outbox Undo Window (Queued Messages)</h3>' + chip('Cancel inside window → target never sees it', 'live') + '</div>' +
      queuedMessages.map(m => {
        const tgt = targets.find(t => t.id === m.target_id);
        const remainingSec = Math.max(0, Math.ceil((Date.parse(m.send_after) - Date.now()) / 1000));
        return '<div class="outbox-item">' +
          '<div>' +
            '<strong>To ' + escapeHTML(tgt?.branch_name || m.target_id || 'Target') + '</strong> (' + escapeHTML(m.kind) + ')' +
            '<p class="caption">' + escapeHTML(m.body) + '</p>' +
          '</div>' +
          '<div class="actions">' +
            '<span class="tag warm" data-send-after="' + escapeHTML(m.send_after) + '">Queued · sends in ' + remainingSec + 's</span>' +
            btn('Cancel Message', 'cancel-message:' + m.id, 'secondary small') +
          '</div>' +
        '</div>';
      }).join('') +
    '</section>';
  }

  // 6. Peer Pharmacy Reply Box inside Modal (when viewing as Target Branch B-E)
  if (myTarget && (myTarget.status === 'sent' || myTarget.status === 'no_response') && !isFinalState) {
    html += '<section class="hitl-card" style="border-color:#65b994">' +
      '<div class="row split"><h3>Reply as ' + escapeHTML(liveSession?.branch?.name || 'Target Pharmacy') + '</h3>' + chip('POST /targets/{id}/reply', 'live') + '</div>' +
      '<div class="preset-chips">' +
        '<button type="button" class="preset-chip" data-action="quick-reply:' + myTarget.id + ':available:' + enq.quantity + '">✓ Confirm Available (Qty ' + enq.quantity + ')</button>' +
        '<button type="button" class="preset-chip" data-action="quick-reply:' + myTarget.id + ':unavailable:0">✕ Confirm Unavailable</button>' +
        '<button type="button" class="preset-chip" data-action="quick-reply-alt:' + myTarget.id + '">⇄ Offer 625mg Alternative</button>' +
      '</div>' +
      '<form class="inbox-reply-form inbox-reply-grid" data-target-id="' + escapeHTML(myTarget.id) + '">' +
        '<label class="field">Decision<select name="kind"><option value="available">available</option><option value="unavailable">unavailable</option><option value="alternative">alternative</option></select></label>' +
        '<label class="field">Quantity<input type="number" name="quantity" min="0" max="1000" value="' + Number(enq.quantity) + '"></label>' +
        '<label class="field">Notes / Alternative<input type="text" name="text" maxlength="500" placeholder="Optional note or required alternative"></label>' +
        '<button class="btn" type="submit">Submit Reply</button>' +
      '</form>' +
    '</section>';
  }

  // 7. Contacted / Proposed Target Pharmacies List
  html += '<section style="margin-top:18px">' +
    '<div class="row split"><h3>Target Pharmacies (' + targets.length + ')</h3>' +
      '<div class="actions">' +
        (isRequester && approvedTargets.length > 0 && !isFinalState ? btn('Send to Approved Now', 'send-enquiry:' + enq.id, 'secondary small') : '') +
        (isRequester && reversibleSendEvent && !isFinalState ? btn('Undo Send / Retract All', 'undo-event:' + reversibleSendEvent.id, 'secondary small') : '') +
      '</div>' +
    '</div>' +
    (targets.length ? '<div class="branch-results">' + targets.map(t => {
      const preset = DEMO_BRANCH_PRESETS.find(b => b.branchId === t.branch_id);
      return '<article>' +
        '<div>' +
          '<strong>' + escapeHTML(t.branch_name) + '</strong>' +
          (t.reply_text || t.reply_quantity !== null ? `<p>Reply: <strong>${escapeHTML(t.reply_text || t.status)}</strong>${t.reply_quantity !== null ? ` · Qty: ${t.reply_quantity}` : ''}${t.replied_at ? ` · ${date(t.replied_at)}` : ''}</p>` : `<p>Follow-ups: ${Number(t.follow_ups || 0)}</p>`) +
        '</div>' +
        '<div class="actions">' +
          targetStatusBadge(t.status) +
          (preset && t.status === 'sent' ? '<button type="button" class="btn secondary small" data-action="quick-branch:' + preset.email + '">Switch to ' + preset.code + ' to Reply →</button>' : '') +
          (isRequester && t.status === 'sent' && Number(t.follow_ups || 0) === 0 && !isFinalState ? btn('Follow-up', 'followup-target:' + t.id, 'ghost small') : '') +
          (isRequester && t.status === 'sent' && Number(t.follow_ups || 0) >= 1 && !isFinalState ? btn('Mark No Response', 'noresponse-target:' + t.id, 'ghost small') : '') +
        '</div>' +
      '</article>';
    }).join('') + '</div>' : empty('No target pharmacies proposed yet — Wassit Agent is processing the enquiry.')) +
  '</section>';

  // 7b. Structured offers with highlighted evidence, and the simulated call
  html += buildOffersSectionHTML(detail, isRequester);
  html += buildCallSectionHTML(detail);

  // 8. Requester Close / Cancel Controls (POST /api/v1/enquiries/{id}/close)
  if (isRequester && !isFinalState) {
    html += '<section style="margin-top:18px;padding-top:14px;border-top:1px solid var(--line)">' +
      '<div class="row split">' +
        '<div><strong>Requester Enquiry Controls:</strong> <small>Rule 6 closing rules are enforced by the Go backend.</small></div>' +
        '<div class="actions">' +
          availableTargets.map(t =>
            btn('Close Available at ' + t.branch_name, 'close-enquiry:' + enq.id + ':available:' + t.branch_id, '')
          ).join('') +
          (allUnavailable ? btn('Close Unavailable', 'close-enquiry:' + enq.id + ':unavailable', 'secondary small') : '') +
          btn('Close Unable to Confirm', 'close-enquiry:' + enq.id + ':unable_to_confirm', 'secondary small') +
          btn('Cancel Enquiry', 'close-enquiry:' + enq.id + ':cancelled', 'ghost small') +
        '</div>' +
      '</div>' +
    '</section>';
  }

  // 9. Live Event Timeline & Audit Trail with Undo (POST /api/v1/events/{id}/undo)
  html += '<details open style="margin-top:18px">' +
    '<summary>Live Event Timeline & Audit Trail (' + events.length + ' events · reversible steps have Undo)</summary>' +
    '<div style="margin-top:8px">' +
      events.map(ev => {
        const hasInverse = ev.inverse !== null && ev.inverse !== undefined && String(ev.inverse) !== 'null';
        const isUndone = Boolean(ev.undone_at);
        return '<div class="event-row ' + (isUndone ? 'undone' : '') + '">' +
          '<div>' +
            '<span class="tag ' + (ev.actor_type === 'agent' ? 'live' : ev.actor_type === 'staff' ? 'info' : 'neutral') + '">#' + ev.seq + ' · ' + escapeHTML(actorLabel(ev)) + '</span> ' +
            '<strong>' + escapeHTML(formatEventDescription(ev)) + '</strong>' +
            '<small style="display:block;color:var(--muted);margin-top:3px">' + escapeHTML(ev.type) + ' · ' + date(ev.created_at) + (isUndone ? ' · Undone at ' + date(ev.undone_at) : '') + '</small>' +
          '</div>' +
          '<div class="actions">' +
            (hasInverse && !isUndone && isRequester ? btn('Undo', 'undo-event:' + ev.id, 'secondary small') : '') +
            (isUndone ? chip('Undone', 'neutral') : '') +
          '</div>' +
        '</div>';
      }).join('') +
    '</div>' +
  '</details>';

  html += '</div>';
  return html;
}

function renderLiveEnquiryModal(isRefresh = false) {
  const detail = liveState.activeDetail;
  if (!detail?.enquiry) return;
  const titleText = `${detail.enquiry.raw_request} (×${detail.enquiry.quantity})`;
  const bodyHTML = buildLiveEnquiryModalHTML(detail);

  dialog.classList.add('wide-dialog', 'dispensary-dialog');
  if (isRefresh && dialog.open) {
    const container = dialog.querySelector('.request-detail');
    if (container) {
      // Preserve in-progress input values if the user is typing
      const activeEl = document.activeElement;
      if (activeEl && container.contains(activeEl) && ['INPUT', 'TEXTAREA', 'SELECT'].includes(activeEl.tagName) && activeEl.type !== 'checkbox') {
        updateSSEIndicator();
        return;
      }
      const clarifyInput = container.querySelector('#clarify-answer-input');
      const savedClarify = clarifyInput ? clarifyInput.value : '';
      container.outerHTML = bodyHTML;
      const newClarify = dialog.querySelector('#clarify-answer-input');
      if (newClarify && savedClarify) newClarify.value = savedClarify;
      return;
    }
  }
  const wasOpen = dialog.open;
  if (!wasOpen) opener = document.activeElement;
  dialog.innerHTML = '<div class="row split"><span class="eyebrow">DOSYN · Live Wassit Enquiry Workspace</span>' + btn(t('Close'), 'close', 'ghost small') + '</div><h2 id="dialog-title" style="font-size:1.85rem">' + escapeHTML(titleText) + '</h2>' + bodyHTML;
  if (!wasOpen) dialog.showModal();
}

function openNewEnquiryModal(prefillMedId = '') {
  const med = prefillMedId ? liveState.medications.find(m => m.id === prefillMedId) : null;
  const defaultRaw = med ? `need ${med.brand_name.toLowerCase()} ${med.strength} ${med.dosage_form} x2` : 'need augmentin 1g tabs x2';
  dialog.classList.remove('wide-dialog');
  modal('New Agent-Handled Enquiry (Wassit AI)',
    '<p>Submit a medicine availability request. Wassit AI will match the catalogue item, request clarification if ambiguous, rank nearby branches, and propose targets.</p>' +
    '<div class="preset-chips" style="margin:10px 0">' +
      DEMO_ENQUIRY_PRESETS.map((p, idx) =>
        '<button type="button" class="preset-chip" data-action="fill-modal-preset:' + idx + '">' + escapeHTML(p.label) + '</button>'
      ).join('') +
    '</div>' +
    '<form id="live-enquiry-form" class="form">' +
      '<label class="field">Request Description (raw_request)' +
        '<input type="text" name="raw_request" id="modal-raw-request" value="' + escapeHTML(defaultRaw) + '" maxlength="1000" required>' +
      '</label>' +
      '<label class="field">Quantity Requested' +
        '<input type="number" name="quantity" id="modal-quantity" value="2" min="1" max="1000" required>' +
      '</label>' +
      '<label class="field">Catalogue Medicine (Optional — leave blank to let Wassit AI match)' +
        '<select name="medication_id">' +
          '<option value="">Let Wassit AI match or ask for clarification (Recommended)</option>' +
          liveState.medications.map(m =>
            '<option value="' + escapeHTML(m.id) + '" ' + (m.id === prefillMedId ? 'selected' : '') + '>' + escapeHTML(`${m.brand_name} ${m.strength} · ${m.dosage_form}`) + '</option>'
          ).join('') +
        '</select>' +
      '</label>' +
      '<input type="hidden" name="handled_by" value="agent">' +
      '<button class="btn" type="submit">Create Agent Enquiry ' + icon('arrow') + '</button>' +
    '</form>',
    false
  );
}

// ---- Simulated Client Portal Views (Option A: Local State + Demo Badge) -----
function pharmacyOptions(exclude = -1) {
  const preferred = ['login','register','demo'].includes(page[0]) ? sessions.pharmacist.pharmacy : targetPreference;
  return pharmacies.map(([name],i)=>i===exclude?'':'<option value="'+i+'" '+(i===preferred?'selected':'')+'>'+name+'</option>').join('');
}
const isClient = () => page[0] === 'client';
const ownClient = () => db.requests.filter(r=>r.origin==='client' && r.account===sessions.client.key);
const pharmacyId = () => sessions.pharmacist.pharmacy || 0;
const incoming = () => db.requests.filter(r=>r.target===pharmacyId());
const folders = () => {
  const key = sessions.client.key;
  const list = db.folders[key] ??= [];
  if (!list[0]?.primary) {
    list.unshift({id:id(),name:'My documents',primary:true});
    save();
  }
  return list;
};
function requestList(list, message = 'No enquiries here yet.', showEscalateBtn = false) {
  return list.length ? '<div class="stack">'+[...list].reverse().map(r=>'<article class="card request"><div class="row">'+tile(r.origin==='client'?'pulse':'link')+'<div><h3>'+escapeHTML(label(r.med))+' <small>(×'+r.quantity+')</small></h3><p>'+pharmacies[r.target][0]+' · '+(r.origin==='client'?'Simulated client enquiry':'From '+pharmacies[r.sender][0])+'</p></div></div><div class="actions">'+chip('Demo / Simulated', 'demo-badge')+badge(r)+btn('View simulated enquiry','view:'+r.id,'secondary small')+(showEscalateBtn ? btn('Escalate to Live Wassit Agent →', 'escalate-sim:' + r.id, 'small') : '')+'</div></article>').join('')+'</div>' : empty(message);
}
const queueFilters = [
  ['all','All enquiries',()=>true],
  ['review','Needs your response',r=>r.stage===3 && r.target===pharmacyId()],
  ['waiting','In progress',r=>!r.result && (isClient() || r.stage!==3 || r.target!==pharmacyId())],
  ['complete','Completed',r=>Boolean(r.result)]
];
let selectedQueue='all';
function enquiryQueue(requests, showEscalateBtn = false) {
  const filters=queueFilters.filter(([key])=>!isClient() || key!=='review');
  const filter=filters.find(([key])=>key===selectedQueue) || filters[0];
  return '<div class="queue-filters" role="group" aria-label="Filter enquiries">'+filters.map(([key,name,test])=>
    '<button type="button" data-action="queue:'+key+'" aria-pressed="'+(key===filter[0])+'">'+t(name)+' <span>'+requests.filter(test).length+'</span></button>'
  ).join('')+'</div><div class="stack">'+requestList(requests.filter(filter[2]),'No enquiries in this view.', showEscalateBtn)+'</div>';
}
function pharmacyFloor() {
  const latest=ownClient().at(-1);
  return demoClientBanner() +
    '<section class="floor-counter"><div>'+title('Your pharmacy','Choose one of your medicines to see simulated pharmacy replies.', chip('Demo / Simulated', 'demo-badge'))+'</div>'+appLink(icon('pin')+' '+t('Browse pharmacies'),'pharmacies','secondary')+'</section><div class="actions stock-actions">'+appLink('Open prescriptions','prescriptions','secondary')+'</div>'+
    '<section aria-labelledby="counter-medicines"><h2 id="counter-medicines">'+t('My medicines')+'</h2><p class="shelf-note">'+t('Your prescribed and paid medicines')+'</p>'+patientMedicines()+'</section>'+
    '<div class="counter-board"><section class="card counter-replies"><h2>Latest pharmacy reply (Simulated)</h2>'+(latest?confirmationCard(latest,true):empty('Your pharmacy replies will appear here.'))+appLink('All enquiries','requests','secondary')+'</section>'+
    '<section class="card counter-files">'+tile('folder')+'<h2>'+t('My documents')+'</h2><p>Open your personal folder and case files.</p>'+appLink('Open documents','folders','secondary')+'</section></div>';
}
function confirmationCard(request, compact = false) {
  const reply=request.events.findLast(entry=>entry.actor.startsWith('Demo pharmacist ·'));
  const [branch,area]=pharmacies[request.target];
  const guidance={available:'Contact the branch before travelling to ask whether it can hold the item.',unavailable:'Ask another participating branch about the same item.',unresolved:'No reply was received. Check with the branch or ask another pharmacy.'};
  return '<article class="confirmation-card '+(request.result||'pending')+'"><div class="row">'+badge(request)+chip('Demo / Simulated', 'demo-badge')+'</div><h3>'+escapeHTML(label(request.med))+'</h3>'+
    '<dl><div><dt>Pharmacy</dt><dd>'+branch+' · '+area+'</dd></div><div><dt>Quantity requested</dt><dd>'+request.quantity+(request.unit?' '+escapeHTML(request.unit):'')+'</dd></div>'+
    (reply?'<div><dt>Checked by</dt><dd>'+escapeHTML(reply.actor)+'</dd></div><div><dt>Checked at</dt><dd>'+date(reply.at)+'</dd></div>':'')+'</dl>'+
    '<p><strong>Next step</strong> '+(guidance[request.result]||'Check back for the pharmacist’s reply.')+'</p>'+
    (compact?btn('View enquiry','view:'+request.id,'secondary'):'')+'</article>';
}
function prescriptionList() {
  const list=(prescriptions[sessions.client.key]||[]).filter(r=>r.prescribed && !r.paid);
  return list.length?'<div class="stack">'+list.map(r=>'<article class="card request"><div><h3>'+label(r.med)+'</h3><p>'+ (reviewedPrescription(r.med)?r.quantity+' '+escapeHTML(r.unit)+' · Reviewed prescription (Simulated)':'Awaiting prescription review')+'</p></div><div class="actions">'+chip('Demo / Simulated', 'demo-badge')+btn('Check availability','product:'+r.med,'secondary')+'</div></article>').join('')+'</div>':empty('No prescriptions awaiting payment.');
}
function patientMedicines() {
  const items=eligible();
  return (targetPreference===undefined?'':'<p class="notice">'+t('Selected pharmacy')+' · '+pharmacies[targetPreference][0]+'</p>')+(items.length?shelves(items):empty(t('No prescribed and paid medicines yet.')));
}
let mapOpen=false;
function pharmacyMap() {
  const [name,area]=pharmacies[selectedPharmacy];
  const choices=pharmacies.map(([pharmacy,location],i)=>'<button type="button" class="map-choice" data-action="pin:'+i+'" aria-pressed="'+(i===selectedPharmacy)+'">'+icon('pin')+'<span><strong>'+pharmacy+'</strong><small>'+location+'</small></span></button>').join('');
  return demoClientBanner()+title('Pharmacies','Illustrated coastal map · Fictional demo locations', chip('Demo / Simulated', 'demo-badge'))+'<div class="map-layout directory-layout"><div class="map-list">'+choices+'<article class="card map-detail"><h2>'+name+'</h2><p>'+area+'</p><div class="actions">'+btn(t('Use this pharmacy'),'choose:'+selectedPharmacy)+'</div></article></div><details id="directory-map" class="card directory-map" '+(mapOpen?'open':'')+'><summary>'+t('Show map')+'</summary><div class="pharmacy-map" aria-label="Illustrated map of three demo pharmacies"><div class="coast" aria-hidden="true"></div>'+pharmacies.map(([pharmacy],i)=>'<button type="button" class="map-pin pin-'+i+'" data-action="pin:'+i+'" aria-pressed="'+(i===selectedPharmacy)+'" aria-label="'+pharmacy+'">'+icon('pin')+'<span>'+pharmacy+'</span></button>').join('')+'</div></details></div>';
}
function stockStatus(count) {
  return count===null?'Unknown stock':count?'In stock · '+count+' units':'Out of stock · 0 units';
}
const packageColors = ['coral','violet','aqua','amber','blue','green','violet'];
function carton(i) {
  const [name,strength,form] = meds[i];
  return '<span class="carton '+packageColors[i]+'" aria-hidden="true"><span class="carton-front"><span class="carton-brand">'+icon('mark')+' DOSYN</span><strong>'+name+'</strong><span>'+strength+'</span><small>'+form+'</small><span class="carton-demo">DEMO PACK</span></span></span>';
}
function shelves(items) {
  return '<div class="shelf-case"><div class="shelf-products">'+items.map(i=>'<button type="button" class="shelf-product" data-action="product:'+i+'" aria-label="View '+escapeHTML(label(i))+'">'+carton(i)+'<span class="shelf-label"><strong>'+meds[i][0]+'</strong><small>'+meds[i][1]+' · '+meds[i][2]+'</small></span></button>').join('')+'</div></div>';
}

function desk() {
  return (page[1]==='home' || isClient() || page[1]==='cticket'?'':'<div class="room-back">'+appLink(isClient()?'← Back to my pharmacy':'← Back to pharmacy office','home','secondary')+'</div>')+roomContent();
}

function roomContent() {
  const client = isClient();
  if (client) {
    switch (page[1]) {
      case 'home': return wzClientHome();
      case 'chat': return wzChatPage(page[2]);
      case 'ticket': return wzTicketPage(page[2]);
      case 'tickets': return wzTicketsPage();
      case 'schedules': return wzSchedulesPage();
      case 'prescriptions': return demoClientBanner() + title('Prescriptions','Check pharmacy availability before paying.', chip('Demo / Simulated', 'demo-badge'))+prescriptionList();
      case 'search': return demoClientBanner() + title(t('My medicines'),t('Your prescribed and paid medicines'), chip('Demo / Simulated', 'demo-badge'))+patientMedicines();
      case 'requests': return demoClientBanner() + title('Availability enquiries','Follow the simulated agent and see the demo pharmacy response.', chip('Demo / Simulated', 'demo-badge'))+enquiryQueue(ownClient());
      case 'pharmacies': return pharmacyMap();
      case 'folders': {
        const [primary,...cases] = folders();
        return demoClientBanner() + title('Document library','Your demo profile has its own document folder. Sample files stay in this tab and are never sent to a pharmacy.', chip('Demo / Simulated', 'demo-badge'))+
          '<div class="row split"><h2>'+t('Your folders')+'</h2>'+btn('Create folder','newfolder','secondary')+'</div>'+
          '<div class="folder-grid">'+[primary,...cases].map(f=>{
            const count=(documents.get(f.id)||[]).length;
            return '<button class="folder-tile" type="button" data-action="folder:'+f.id+'"><span class="folder-sheet" aria-hidden="true"></span><span class="folder-face">'+icon('folder')+'<strong>'+escapeHTML(t(f.name))+'</strong><small>'+count+' '+(count===1?'document':'documents')+'</small></span></button>';
          }).join('')+'</div>';
      }
      case 'history': return demoClientBanner() + title('Enquiry history','Review completed simulated outcomes and timestamped activity.', chip('Demo / Simulated', 'demo-badge'))+'<div class="stack">'+requestList(ownClient().filter(r=>r.result),'Completed enquiries will appear here.')+'</div>';
      case 'account': return demoClientBanner() + title('Account','Manage the simulated client profile you’re using in this tab.', chip('Demo / Simulated', 'demo-badge'))+
        '<article class="card"><h3>'+escapeHTML(sessions.client.name)+'</h3><p>Client demo profile (Simulated Local State)</p><div class="actions">'+link('Log in to demo','login/client','secondary')+link('Create a demo profile','register/client','secondary')+btn('Return to sample profile','sample-profile','secondary')+'</div></article>';
      default: return wzClientHome();
    }
  }

  // Pharmacist Office (Live Backend)
  switch (page[1]) {
    case 'home': return pharmacyOverview();
    case 'network': return networkEnquiriesView();
    case 'approvals': return peerInboxAndApprovalsView();
    case 'inventory': return inventoryView();
    case 'notifications': return notificationsAndStatsView();
    case 'requests': return wzPharmTicketsPage();
    case 'cticket': return wzPharmTicketPage(page[2]);
    case 'reference': return wzReferencePage();
    case 'old-requests':
      return '<div class="demo-sim-banner"><div>' + chip('Demo / Simulated', 'demo-badge') + ' <strong>Simulated Client Requests</strong> — Client-originated requests are simulated locally (Option A). You can escalate any item into a live Wassit Agent network enquiry.</div></div>' +
        title('Client requests (Simulated)', 'Incoming simulated client enquiries. Click "Escalate to Live Wassit Agent" to launch a real backend network enquiry.') +
        enquiryQueue(incoming().filter(r=>r.origin==='client'), true);
    case 'account': return pharmacistAccountView();
    default: return pharmacyOverview();
  }
}

function workspace() {
  const client=isClient();
  return '<div class="dispensary '+(client?'client-space':'pharmacist-space')+'">'+bar(true)+'<div class="workspace"><aside class="side"><span class="eyebrow">'+t(client?'Client workspace':'Pharmacy workspace')+'</span><nav aria-label="Workspace">'+navs[page[0]].map(([route,name,symbol])=>'<a '+((page[1]===route || ({chat:'home',ticket:'tickets',cticket:'requests'})[page[1]]===route)?'aria-current="page" ':'')+'href="#'+page[0]+'/'+route+'">'+icon(symbol)+t(name)+'</a>').join('')+'</nav></aside><main id="main" class="desk enter" tabindex="-1">'+desk()+'</main></div></div>';
}

function render(focus = false) {
  document.documentElement.lang=language;
  document.documentElement.dir=language==='ar'?'rtl':'ltr';
  document.documentElement.dataset.theme=theme;
  page = location.hash.slice(1).split('/');
  if ((page[0]==='client' && !sessions.client.entered) ||
      (page[0]==='pharmacist' && !liveSession?.token)) {
    const role=page[0];
    page=['login',role];
    location.replace('#login/'+role);
  }
  if (navs[page[0]]) {
    if (!navs[page[0]].some(([route])=>route===page[1]) && !hiddenRoutes[page[0]].includes(page[1])) page[1]='home';
    app.innerHTML = workspace();
  } else if (['login','register','demo'].includes(page[0])) {
    if (page[0]==='demo') page[1]='pharmacist';
    page[1] = page[1]==='pharmacist'?'pharmacist':'client';
    app.innerHTML = auth();
  } else {
    app.innerHTML = landing();
  }
  const langSelect = document.getElementById('language');
  if (langSelect) langSelect.value=language;
  if (focus) {
    window.scrollTo(0,0);
    document.getElementById('main')?.focus({preventScroll:true});
  }
}

function modal(titleText, body, isSimulated = true) {
  dialog.classList.remove('wide-dialog');
  dialog.classList.toggle('dispensary-dialog',Boolean(navs[page[0]]));
  const open = dialog.open;
  if (!open) opener = document.activeElement;
  dialog.innerHTML = '<div class="row split"><span class="eyebrow">'+(isSimulated ? 'DOSYN · Demo / Simulated' : 'DOSYN · Live Wassit Backend')+'</span>'+btn(t('Close'),'close','ghost small')+'</div><h2 id="dialog-title">'+titleText+'</h2>'+body;
  if (!open) dialog.showModal();
  else dialog.querySelector('input, select, .btn')?.focus();
}

function showRequest(r, refresh = false) {
  const review = !isClient() && r.target===pharmacyId() && r.stage===3;
  const owns = isClient() ? r.account===sessions.client.key && r.origin==='client' : r.origin==='pharmacist' && r.sender===pharmacyId();
  const body = confirmationCard(r)+
    '<div class="ai-guide"><span class="ai-avatar" aria-hidden="true"><i></i></span><div><strong>'+t('AI pharmacy assistant')+' (Simulated)</strong><small>'+t('Administrative support · Human pharmacist confirms stock')+'</small></div></div>'+timeline(r.stage)+(review?'<p class="notice"><strong>Branch inventory: '+stockStatus(stock[r.target][r.med])+'</strong><br>Check the exact item and requested quantity before replying.</p><div class="actions">'+btn('Confirm available','decide:'+r.id+':available') +btn('Confirm unavailable','decide:'+r.id+':unavailable','secondary')+btn('Ask Live Wassit Network','escalate-sim:'+r.id,'secondary')+'</div>':'')+
    (r.stage===3 && owns?'<div class="actions">'+btn('Simulate no reply','decide:'+r.id+':unresolved','secondary')+'</div>':'')+
    (r.result?'<p class="notice">'+(r.result==='unresolved'?'Demo follow-up expired without a pharmacy reply. Availability remains unknown.':'Simulated pharmacist response recorded '+date(r.events.at(-2).at)+'.')+'</p>':'')+
    '<details '+(refresh && dialog.querySelector('details')?.open?'open':'')+'><summary>Audit trail · '+r.events.length+' events</summary>'+r.events.map(e=>'<div class="audit"><strong>'+e.detail+'</strong><small>'+e.actor+' · '+date(e.at)+'</small></div>').join('')+'</details>';
  if (refresh && dialog.open) {
    const content = dialog.querySelector('.request-detail');
    if (content) {
      const active = content.contains(document.activeElement);
      content.innerHTML = body;
      if (active) dialog.querySelector('[data-action="close"]').focus();
    }
  } else {
    modal(escapeHTML(label(r.med)),'<div class="request-detail">'+body+'</div>', true);
    requestInDialog = r.id;
  }
}

function advance() {
  let changed = false;
  for (const r of db.requests) {
    if (r.stage>=3 || Date.now()-Date.parse(r.at)<(r.stage+1)*1350) continue;
    r.stage++;
    event(r,steps[r.stage][1],steps[r.stage][0]);
    if (r.id===requestInDialog && dialog.open) {
      showRequest(r,true);
      status(steps[r.stage][0]);
    }
    changed = true;
  }
  if (changed) {
    save();
    if (page[0]==='client' && ['home','requests','history'].includes(page[1])) {
      const deskEl = document.querySelector('.desk');
      if (deskEl && !deskEl.contains(document.activeElement)) deskEl.innerHTML = desk();
    }
  }
}

function showFolder(folder) {
  const files = documents.get(folder.id)||[];
  const name = folders()[0]===folder?'My documents':folder.name;
  modal(escapeHTML(name),'<p>Use sample PDFs, PNGs or JPEGs up to 10 MB each. Files stay in this tab and are never sent or shared.</p><form id="document-form" class="form upload" data-folder="'+folder.id+'"><label class="field">'+icon('folder')+' Choose sample documents<input name="file" type="file" accept="application/pdf,image/png,image/jpeg" multiple required></label><button class="btn">Add to folder</button><p role="alert" id="file-error"></p></form>'+
    (files.length?files.map((file,i)=>'<div class="audit"><strong>'+escapeHTML(file.name)+'</strong><div class="actions">'+btn('Preview','document:'+folder.id+':'+i,'secondary small')+btn('Remove','remove:'+folder.id+':'+i,'ghost small')+'</div></div>').join(''):empty('This folder has no documents yet.')), true);
}

function enquiry(med, source) {
  const client=isClient(), record=client?reviewedPrescription(med):null;
  if(client && !record) {
    modal('Prescription needs review','<p>A pharmacist needs to check the quantity on the doctor’s prescription before this enquiry can be sent.</p>', true);
    return;
  }
  const quantity=record||source
    ? '<p class="notice"><strong>Requested quantity: '+((record||source).quantity)+' '+escapeHTML((record||source).unit||'units')+'</strong><br>'+(source?'Copied from the client enquiry.':'From the reviewed sample prescription.')+'</p>'
    : '<label class="field">Quantity<input name="quantity" type="number" min="1" max="99" step="1" value="'+(source?.quantity ?? 1)+'" required></label>';
  modal(t(client?'Ask a pharmacy':'Ask the pharmacy network'),'<p>'+escapeHTML(label(med))+'</p><form class="form" id="enquiry-form" data-source="'+(source?.id||'')+'" data-med="'+med+'"><label class="field">Simulated pharmacy<select name="target">'+pharmacyOptions(client?-1:pharmacyId())+'</select></label>'+quantity+'<label><input type="checkbox" required> Send the exact medicine and quantity in this demo enquiry.</label><button class="btn">'+t('Send simulated enquiry')+' '+icon('arrow')+'</button></form>', true);
}

function showProduct(med) {
  const requests=ownClient().filter(r=>r.med===med);
  const branches=pharmacies.map(([name,area],i)=>{
    const reply=requests.findLast(r=>r.target===i);
    const checked=reply?.events.findLast(e=>e.actor.startsWith('Demo pharmacist ·'));
    return {i,name,area,reply,checked};
  }).sort((a,b)=>Number(b.reply?.result==='available')-Number(a.reply?.result==='available'));
  modal(escapeHTML(label(med)),'<h3>Pharmacy availability (Simulated)</h3><p>Demo responses for this exact medicine. A confirmation reflects the time it was checked.</p><div class="branch-results">'+branches.map(({i,name,area,reply,checked})=>
    '<article><div><strong>'+name+'</strong><p>'+area+'</p>'+(reply?badge(reply):chip('Not yet confirmed','neutral'))+(checked?'<p class="caption">Checked '+date(checked.at)+' · Quantity '+reply.quantity+(reply.unit?' '+escapeHTML(reply.unit):'')+'</p>':'')+'</div>'+
    (reply && !reply.result?btn('View enquiry','view:'+reply.id,'secondary'):btn('Ask this pharmacy','branch-enquiry:'+med+':'+i,'secondary'))+'</article>').join('')+'</div>', true);
}

function decide(request,outcome) {
  if (!request || request.stage!==3) return;
  const owns = isClient() ? request.account===sessions.client.key && request.origin==='client' : request.origin==='pharmacist' && request.sender===pharmacyId();
  if (outcome==='unresolved' ? !owns : isClient() || request.target!==pharmacyId() || !['available','unavailable'].includes(outcome)) return;
  request.result = outcome;
  request.stage = 4;
  request.seen = false;
  if (outcome==='unresolved') {
    event(request,'Agent · demo channel','Follow-up sent');
    event(request,'Agent · timer','Demo response window expired');
  } else {
    event(request,'Demo pharmacist · '+pharmacies[request.target][0],results[outcome]);
  }
  event(request,'Agent · audit log','Outcome recorded and originator notified');
  save();
  render();
  showRequest(request,true);
  status(results[outcome]);
}

// ---- Click Actions (Live & Simulated) ---------------------------------------
document.addEventListener('click', async e => {
  const cb = e.target.closest('.target-approve-cb');
  if (cb) {
    if (cb.checked) liveState.deselectedTargetIds.delete(cb.value);
    else liveState.deselectedTargetIds.add(cb.value);
    renderLiveEnquiryModal(true);
    return;
  }

  const button = e.target.closest('[data-action]');
  if (!button) return;
  const rawAction = button.dataset.action;
  const parts = rawAction.split(':');
  const [action, key, value, extra] = parts;

  switch (action) {
    case 'close':
      closeLiveSSE();
      liveState.activeEnquiryId = null;
      dialog.close();
      break;
    case 'toggle-reg-demo':
      showRegistrationDemo = !showRegistrationDemo;
      render(true);
      break;
    case 'reset-application':
      application = undefined;
      render(true);
      break;
    case 'theme':
      theme = theme === 'dark' ? 'light' : 'dark';
      try { localStorage.setItem('findose-theme', theme); } catch {}
      render();
      app.querySelector('[data-action="theme"]')?.focus();
      break;
    case 'quick-branch': {
      const email = key;
      await loginAsStaff(email, '1234', !navs[page[0]]);
      break;
    }
    case 'staff-logout':
      closeLiveSSE();
      saveLiveSession(null);
      location.hash = 'login/pharmacist';
      break;
    case 'refresh-live':
      liveState.actionError = '';
      liveState.actionNotice = '';
      await refreshLivePharmacyData();
      if (liveState.activeEnquiryId && dialog.open) {
        await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
      }
      status('Live pharmacy data refreshed');
      break;
    case 'live-queue':
      selectedLiveQueue = key;
      updateDeskIfSafe();
      break;
    case 'open-new-enquiry':
      openNewEnquiryModal();
      break;
    case 'live-enquire-med':
      openNewEnquiryModal(key);
      break;
    case 'escalate-sim': {
      const simReq = db.requests.find(r => r.id === key);
      if (dialog.open) dialog.close();
      openNewEnquiryModal();
      if (simReq) {
        const rawInput = document.getElementById('modal-raw-request');
        const qtyInput = document.getElementById('modal-quantity');
        if (rawInput) rawInput.value = `need ${label(simReq.med)} x${simReq.quantity}`;
        if (qtyInput) qtyInput.value = String(simReq.quantity || 1);
      }
      break;
    }
    case 'fill-preset': {
      const preset = DEMO_ENQUIRY_PRESETS[Number(key)];
      if (preset) {
        const rawEl = document.getElementById('quick-raw-request');
        const qtyEl = document.getElementById('quick-quantity');
        const medEl = document.getElementById('quick-med-id');
        if (rawEl) rawEl.value = preset.raw_request;
        if (qtyEl) qtyEl.value = String(preset.quantity);
        if (medEl) medEl.value = '';
      }
      break;
    }
    case 'fill-modal-preset': {
      const preset = DEMO_ENQUIRY_PRESETS[Number(key)];
      if (preset) {
        const rawEl = document.getElementById('modal-raw-request');
        const qtyEl = document.getElementById('modal-quantity');
        if (rawEl) rawEl.value = preset.raw_request;
        if (qtyEl) qtyEl.value = String(preset.quantity);
      }
      break;
    }
    case 'fill-clarify': {
      const ansText = parts.slice(1).join(':');
      const input = document.getElementById('clarify-answer-input');
      if (input) {
        input.value = ansText;
        input.focus();
      }
      break;
    }
    case 'open-live-enquiry':
      await openLiveEnquiry(key);
      break;
    case 'play-call':
      await playCall();
      break;
    case 'stop-call':
      stopCall();
      renderLiveEnquiryModal(true);
      break;
    case 'approve-targets': {
      const detail = liveState.activeDetail;
      if (!detail) break;
      const proposed = (detail.targets || []).filter(t => t.status === 'proposed');
      const targetIds = proposed.filter(t => !liveState.deselectedTargetIds.has(t.id)).map(t => t.id);
      if (targetIds.length === 0) break;
      try {
        liveState.actionError = '';
        await apiCall('POST', `/api/v1/enquiries/${encodeURIComponent(key)}/targets/approve`, { target_ids: targetIds });
        liveState.actionNotice = `Approved ${targetIds.length} target pharmacy(s). Wassit Agent notified to send enquiries.`;
        await fetchAndRenderLiveEnquiry(key, true);
        await refreshLivePharmacyData();
      } catch (err) {
        liveState.actionError = err.message;
        renderLiveEnquiryModal(true);
      }
      break;
    }
    case 'send-enquiry': {
      try {
        liveState.actionError = '';
        await apiCall('POST', `/api/v1/enquiries/${encodeURIComponent(key)}/send`);
        liveState.actionNotice = 'Queued outbound enquiry messages in outbox.';
        await fetchAndRenderLiveEnquiry(key, true);
        await refreshLivePharmacyData();
      } catch (err) {
        liveState.actionError = err.message;
        renderLiveEnquiryModal(true);
      }
      break;
    }
    case 'cancel-message': {
      try {
        liveState.actionError = '';
        await apiCall('POST', `/api/v1/messages/${encodeURIComponent(key)}/cancel`);
        liveState.actionNotice = 'Outbound message cancelled inside the undo window. The target pharmacy will never see it.';
        if (liveState.activeEnquiryId) await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
        await refreshLivePharmacyData();
      } catch (err) {
        liveState.actionError = err.message;
        if (liveState.activeEnquiryId) renderLiveEnquiryModal(true);
      }
      break;
    }
    case 'undo-event': {
      try {
        liveState.actionError = '';
        await apiCall('POST', `/api/v1/events/${encodeURIComponent(key)}/undo`);
        liveState.actionNotice = 'Event undone via stored inverse instruction.';
        if (liveState.activeEnquiryId) await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
        await refreshLivePharmacyData();
      } catch (err) {
        liveState.actionError = err.message;
        if (liveState.activeEnquiryId) renderLiveEnquiryModal(true);
      }
      break;
    }
    case 'followup-target': {
      try {
        liveState.actionError = '';
        await apiCall('POST', `/api/v1/targets/${encodeURIComponent(key)}/follow-up`);
        liveState.actionNotice = 'Follow-up reminder queued for target pharmacy.';
        if (liveState.activeEnquiryId) await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
      } catch (err) {
        liveState.actionError = err.message;
        if (liveState.activeEnquiryId) renderLiveEnquiryModal(true);
      }
      break;
    }
    case 'noresponse-target': {
      try {
        liveState.actionError = '';
        await apiCall('POST', `/api/v1/targets/${encodeURIComponent(key)}/no-response`);
        liveState.actionNotice = 'Target pharmacy marked as no_response.';
        if (liveState.activeEnquiryId) await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
      } catch (err) {
        liveState.actionError = err.message;
        if (liveState.activeEnquiryId) renderLiveEnquiryModal(true);
      }
      break;
    }
    case 'close-enquiry': {
      const enquiryId = key;
      const outcome = value;
      const branchId = extra || undefined;
      try {
        liveState.actionError = '';
        const body = { outcome };
        if (branchId) body.branch_id = branchId;
        await apiCall('POST', `/api/v1/enquiries/${encodeURIComponent(enquiryId)}/close`, body);
        liveState.actionNotice = `Enquiry closed with outcome: ${outcome}.`;
        await fetchAndRenderLiveEnquiry(enquiryId, true);
        await refreshLivePharmacyData();
      } catch (err) {
        liveState.actionError = err.message;
        renderLiveEnquiryModal(true);
      }
      break;
    }
    case 'quick-reply': {
      const targetId = key;
      const kind = value;
      const qty = Number(extra ?? 0);
      try {
        liveState.actionError = '';
        await apiCall('POST', `/api/v1/targets/${encodeURIComponent(targetId)}/reply`, {
          kind,
          quantity: qty,
          text: kind === 'available' ? `Confirmed ${qty} units in stock at ${liveSession?.branch?.name || 'branch'}` : 'Out of stock'
        });
        liveState.actionNotice = `Reply (${kind}) submitted from ${liveSession?.branch?.name || 'branch'}.`;
        if (liveState.activeEnquiryId && dialog.open) await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
        await refreshLivePharmacyData();
      } catch (err) {
        liveState.actionError = err.message;
        if (dialog.open) renderLiveEnquiryModal(true);
        else updateDeskIfSafe();
      }
      break;
    }
    case 'quick-reply-alt': {
      const targetId = key;
      try {
        liveState.actionError = '';
        await apiCall('POST', `/api/v1/targets/${encodeURIComponent(targetId)}/reply`, {
          kind: 'alternative',
          quantity: 2,
          text: '625mg tablets only in stock (not 1000mg)'
        });
        liveState.actionNotice = 'Alternative offer ("625mg tablets only") sent. Note: Contract Rule 1 treats alternatives as options, never a match.';
        if (liveState.activeEnquiryId && dialog.open) await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
        await refreshLivePharmacyData();
      } catch (err) {
        liveState.actionError = err.message;
        if (dialog.open) renderLiveEnquiryModal(true);
        else updateDeskIfSafe();
      }
      break;
    }

    // Simulated Client Actions
    case 'pin':
      mapOpen = document.getElementById('directory-map')?.open || false;
      selectedPharmacy = Number(key);
      render();
      app.querySelectorAll('[data-action="pin:'+key+'"]')[button.classList.contains('map-pin')?1:0]?.focus();
      break;
    case 'queue':
      selectedQueue = key;
      document.querySelector('.desk').innerHTML = desk();
      app.querySelector('[data-action="queue:'+key+'"]')?.focus();
      break;
    case 'sample-profile':
      sessions.client = { key: 'demo-client', name: 'Maya', entered: true };
      try { sessionStorage.setItem('findose-profiles', JSON.stringify(sessions)); } catch {}
      location.hash = 'client/home';
      status('Sample profile restored');
      break;
    case 'enquire':
      if (!isClient() || reviewedPrescription(Number(key))) enquiry(Number(key));
      break;
    case 'branch-enquiry':
      if (isClient() && reviewedPrescription(Number(key)) && pharmacies[Number(value)]) {
        targetPreference = Number(value);
        enquiry(Number(key));
      }
      break;
    case 'product':
      if (isClient() && reviewedPrescription(Number(key))) showProduct(Number(key));
      break;
    case 'choose':
      targetPreference = Number(key);
      location.hash = 'client/search';
      break;
    case 'view': {
      const request = db.requests.find(r => r.id === key);
      if (request) showRequest(request);
      break;
    }
    case 'decide':
      decide(db.requests.find(r => r.id === key), value);
      break;
    case 'newfolder':
      modal('Create case folder','<form class="form" id="folder-form"><label class="field">Folder name<input name="name" required maxlength="60" placeholder="e.g. Sample records"></label><button class="btn">Create folder</button></form>', true);
      break;
    case 'folder':
      showFolder(folders().find(f => f.id === key));
      break;
    case 'document': {
      const file = documents.get(key)[Number(value)];
      modal(escapeHTML(file.name),(file.type==='application/pdf'?'<iframe class="doc pdf" title="Sample PDF preview" src="'+file.url+'"></iframe>':'<img class="doc" src="'+file.url+'" alt="Sample document preview">')+'<div class="actions">'+btn('Back to folder','folder:'+key,'secondary')+'</div>', true);
      break;
    }
    case 'remove': {
      const [file] = documents.get(key).splice(Number(value), 1);
      URL.revokeObjectURL(file.url);
      showFolder(folders().find(f => f.id === key));
      status('Sample document removed');
      break;
    }
  }
});

// ---- Form Submissions (Live & Simulated) ------------------------------------
document.addEventListener('toggle', e => {
  const details = e.target;
  if (!(details instanceof HTMLDetailsElement) || !details.dataset.offerForm) return;
  if (!liveState.openOfferForms) liveState.openOfferForms = new Set();
  if (details.open) liveState.openOfferForms.add(details.dataset.offerForm);
  else liveState.openOfferForms.delete(details.dataset.offerForm);
}, true);

document.addEventListener('submit', async e => {
  const form = e.target;
  e.preventDefault();
  const data = new FormData(form);

  if (form.classList.contains('offer-read-form')) {
    const targetId = form.dataset.targetId;
    const str = name => String(data.get(name) ?? '').trim();
    const tri = name => str(name) === 'true' ? true : str(name) === 'false' ? false : undefined;
    const payload = { confidence: Number(str('confidence') || 0), evidence: {} };
    if (tri('in_stock') !== undefined) payload.in_stock = tri('in_stock');
    if (tri('needs_prescription') !== undefined) payload.needs_prescription = tri('needs_prescription');
    if (str('strength')) payload.strength = str('strength');
    if (str('quantity') !== '') payload.quantity = Number(str('quantity'));
    if (str('price') !== '') payload.price_cents = Math.round(Number(str('price')) * 100);
    if (str('currency')) payload.currency = str('currency');
    if (str('pickup_by')) payload.pickup_by = str('pickup_by');
    if (str('payment')) payload.payment = str('payment');
    for (const [field] of OFFER_FIELDS) {
      if (str('ev_' + field)) payload.evidence[field] = str('ev_' + field);
    }
    try {
      liveState.actionError = '';
      const offer = await apiCall('POST', `/api/v1/targets/${encodeURIComponent(targetId)}/offers`, payload);
      liveState.openOfferForms?.delete(targetId);
      liveState.offersNotice = `Offer recorded. Rules decided: ${offerDecisionLabel(offer.decision)}.`;
      if (document.activeElement && form.contains(document.activeElement)) document.activeElement.blur();
      if (liveState.activeEnquiryId && dialog.open) await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
    } catch (err) {
      liveState.offersNotice = '';
      liveState.actionError = 'Offer not recorded: ' + err.message;
      if (document.activeElement && form.contains(document.activeElement)) document.activeElement.blur();
      if (dialog.open) renderLiveEnquiryModal(true);
    }
    return;
  }

  if (form.classList.contains('inbox-reply-form')) {
    const targetId = form.dataset.targetId;
    const kind = String(data.get('kind') || 'available');
    const rawQty = String(data.get('quantity') ?? '').trim();
    const text = String(data.get('text') || '').trim();
    const payload = { kind, text };
    if (rawQty !== '') payload.quantity = Number(rawQty);
    try {
      liveState.actionError = '';
      await apiCall('POST', `/api/v1/targets/${encodeURIComponent(targetId)}/reply`, payload);
      liveState.actionNotice = `Reply (${kind}) recorded for target pharmacy.`;
      if (liveState.activeEnquiryId && dialog.open) await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
      await refreshLivePharmacyData();
    } catch (err) {
      liveState.actionError = err.message;
      if (dialog.open) renderLiveEnquiryModal(true);
      else updateDeskIfSafe();
    }
    return;
  }

  switch (form.id) {
    case 'staff-login-form': {
      const email = String(data.get('email') || '').trim();
      const pin = String(data.get('pin') || '').trim();
      await loginAsStaff(email, pin, true);
      break;
    }
    case 'live-enquiry-form': {
      const rawRequest = String(data.get('raw_request') || '').trim();
      const quantity = Number(data.get('quantity') || 1);
      const medId = String(data.get('medication_id') || '').trim();
      const body = {
        raw_request: rawRequest,
        quantity,
        handled_by: 'agent'
      };
      if (medId) body.medication_id = medId;
      try {
        liveState.actionError = '';
        const created = await apiCall('POST', '/api/v1/enquiries', body);
        liveState.actionNotice = 'Agent-handled enquiry created. Watching live SSE stream…';
        await refreshLivePharmacyData();
        await openLiveEnquiry(created.id);
      } catch (err) {
        liveState.actionError = err.message;
        updateDeskIfSafe();
      }
      break;
    }
    case 'live-clarify-form': {
      const enquiryId = form.dataset.enquiryId;
      const answer = String(data.get('answer') || '').trim();
      if (!answer) return;
      try {
        liveState.actionError = '';
        await apiCall('POST', `/api/v1/enquiries/${encodeURIComponent(enquiryId)}/clarify`, { answer });
        liveState.actionNotice = `Clarification answered ("${answer}"). Wassit Agent woken up.`;
        await fetchAndRenderLiveEnquiry(enquiryId, true);
        await refreshLivePharmacyData();
      } catch (err) {
        liveState.actionError = err.message;
        renderLiveEnquiryModal(true);
      }
      break;
    }
    case 'pharmacy-application': {
      const name = String(data.get('pharmacyName') || '').trim();
      const town = String(data.get('town') || '').trim();
      if (!name || !town) return;
      application = { name, town };
      render(true);
      break;
    }
    case 'auth-form': {
      if (page[0] === 'register') {
        const name = String(data.get('name') || '').trim();
        if (!name) { form.elements.name.focus(); return; }
        sessions.client = { key: id(), name, entered: true };
      } else {
        sessions.client.entered = true;
      }
      try { sessionStorage.setItem('findose-profiles', JSON.stringify(sessions)); } catch {}
      location.hash = 'client/home';
      break;
    }
    case 'search-form':
      searchTerm = String(data.get('query') || '');
      render();
      document.querySelector('#search-form input')?.focus();
      break;
    case 'enquiry-form': {
      const role = page[0];
      const med = Number(form.dataset.med);
      const record = isClient() ? reviewedPrescription(med) : null;
      if (isClient() && !record) { enquiry(med); break; }
      const request = newRequest({
        med,
        target: Number(data.get('target')),
        quantity: record?.quantity ?? Number(data.get('quantity')),
        origin: role,
        account: sessions[role].key,
        sender: 0
      });
      if (record) {
        request.unit = record.unit;
        event(request, 'Reviewed demo prescription', 'Quantity copied from the prescription: ' + request.quantity + ' ' + request.unit);
      }
      db.requests.push(request);
      save();
      showRequest(request);
      status('Simulated enquiry sent');
      break;
    }
    case 'folder-form': {
      const name = String(data.get('name') || '').trim();
      if (!name) { form.elements.name.focus(); return; }
      folders().push({ name, id: id() });
      save();
      dialog.close();
      render(true);
      status('Case folder created');
      break;
    }
    case 'document-form': {
      const selected = data.getAll('file');
      const folder = folders().find(f => f.id === form.dataset.folder);
      if (selected.some(file => !file.size || file.size > 10 * 1024 * 1024 || !['application/pdf','image/jpeg','image/png'].includes(file.type))) {
        document.querySelector('#file-error').textContent = 'Each file must be a PDF, JPEG or PNG between 1 byte and 10 MB.';
        return;
      }
      const files = documents.get(folder.id) || [];
      files.push(...selected.map(file => ({ name: file.name, type: file.type, url: URL.createObjectURL(file) })));
      documents.set(folder.id, files);
      showFolder(folder);
      status(selected.length + ' sample documents added');
      break;
    }
  }
});

dialog.addEventListener('close', () => {
  stopCall();
  requestInDialog = undefined;
  closeLiveSSE();
  liveState.activeEnquiryId = null;
  if (navs[page[0]]) {
    const main = document.querySelector('.desk');
    if (main) main.innerHTML = desk();
    (opener?.isConnected && opener ? opener : main)?.focus?.({ preventScroll: true });
  }
});

window.addEventListener('hashchange', () => {
  selectedQueue = 'all';
  if (dialog.open) dialog.close();
  if (location.hash !== '#client/search' || page[1] !== 'home') searchTerm = '';
  if (page[0] === 'client' && !location.hash.startsWith('#client/search')) targetPreference = undefined;
  render(true);
});

document.querySelector('.skip').addEventListener('click', e => {
  e.preventDefault();
  document.getElementById('main')?.focus();
});

document.addEventListener('change', e => {
  if (e.target.id !== 'language') return;
  language = e.target.value;
  try { localStorage.setItem('findose-language', language); } catch {}
  render();
  document.getElementById('language')?.focus();
});

// ---- Periodic Timers (Simulated Client Advance + Live Polling Fallback) -----
setInterval(advance, 350);

// 1s ticker to update queued outbox countdown labels inside open modal
setInterval(() => {
  if (!dialog.open) return;
  const tags = dialog.querySelectorAll('[data-send-after]');
  tags.forEach(el => {
    const sendAfter = el.getAttribute('data-send-after');
    if (!sendAfter) return;
    const rem = Math.max(0, Math.ceil((Date.parse(sendAfter) - Date.now()) / 1000));
    el.textContent = rem > 0 ? `Queued · sends in ${rem}s` : 'Dispatching…';
  });
}, 1000);

// 2.5s polling fallback while a live enquiry modal is open
setInterval(async () => {
  if (!liveSession?.token || !liveState.activeEnquiryId || !dialog.open) return;
  try {
    await fetchAndRenderLiveEnquiry(liveState.activeEnquiryId, true);
  } catch {}
}, 2500);

// 4s background refresh when in the Pharmacy Office
setInterval(async () => {
  if (!liveSession?.token || page[0] !== 'pharmacist') return;
  try {
    await refreshLivePharmacyData();
  } catch {}
}, 4000);

// ---- Boot -------------------------------------------------------------------
(async function boot() {
  render();
  if (liveSession?.token) {
    await verifyRestoredSession();
    render();
  }
})();
