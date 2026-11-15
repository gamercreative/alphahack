// ---- Wassit UI: AI pharmacist character, client search + chat, tickets, schedules ----------
// Plain script loaded before app.js. Uses app.js globals (escapeHTML, icon, chip, btn, t,
// language, liveState, liveSession, page, DEMO_BRANCH_PRESETS, branchNameById, buildOfferCardHTML,
// render, status, loadPublicCatalogue) only at call time, never at load time.
// All client data comes from LiveClient (web/app.js), which reads and writes the Go backend.

const wzState = { query: '', draft: null, lastSig: '' };
const wzT = (en, ar) => (language === 'ar' ? ar : en);
const wzHasDemo = () => typeof LiveClient !== 'undefined';
const wzMoney = cents => '$' + (Number(cents || 0) / 100).toFixed(2);
const wzDays = () => language === 'ar'
  ? ['الأحد', 'الاثنين', 'الثلاثاء', 'الأربعاء', 'الخميس', 'الجمعة', 'السبت']
  : ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const wzDaysEn = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const wzExpiry = ym => {
  if (!ym) return '—';
  const [y, m] = String(ym).split('-');
  return (m || '') + '/' + (y || '');
};
const wzWhen = ms => new Date(ms).toLocaleString(language === 'ar' ? 'ar' : 'en', { dateStyle: 'medium', timeStyle: 'short' });
const wzTime = ms => new Date(ms).toLocaleTimeString(language === 'ar' ? 'ar' : 'en', { hour: '2-digit', minute: '2-digit' });

// ---- Character ---------------------------------------------------------------------------
function wzAvatar(size = 'md', working = false) {
  return '<span class="wz-avatar ' + size + (working ? ' working' : '') + '" aria-hidden="true">' +
    (working ? '<span class="wz-rings"><i></i><i></i><i></i></span>' : '') +
    '<svg viewBox="0 0 120 120" role="img">' +
      '<path d="M14 120c2-28 20-40 46-40s44 12 46 40Z" fill="var(--wz-coat,#fff)" stroke="#c9d8d3" stroke-width="1.5"/>' +
      '<path d="M48 82 60 104 72 82Z" fill="#2f7d68"/>' +
      '<path d="M48 82 60 104 52 120 38 89Z" fill="var(--wz-coat,#fff)" stroke="#c9d8d3" stroke-width="1.5"/>' +
      '<path d="M72 82 60 104 68 120 82 89Z" fill="var(--wz-coat,#fff)" stroke="#c9d8d3" stroke-width="1.5"/>' +
      '<rect x="53" y="64" width="14" height="19" rx="6" fill="#e0ad86"/>' +
      '<circle cx="60" cy="49" r="22" fill="#efc4a0"/>' +
      '<path d="M37 50c-3-22 11-29 24-29 15 0 25 9 22 29-3-10-11-15-23-15-11 0-19 5-23 15Z" fill="#3a2a21"/>' +
      '<circle class="wz-eye" cx="52" cy="52" r="2.3" fill="#22302c"/>' +
      '<circle class="wz-eye" cx="68" cy="52" r="2.3" fill="#22302c"/>' +
      '<circle cx="46" cy="59" r="3.4" fill="#f08f86" opacity=".35"/>' +
      '<circle cx="74" cy="59" r="3.4" fill="#f08f86" opacity=".35"/>' +
      '<path d="M53 60q7 6 14 0" fill="none" stroke="#8a4b3a" stroke-width="2.2" stroke-linecap="round"/>' +
      '<rect x="69" y="93" width="24" height="10" rx="2.5" fill="#2f7d68"/>' +
      '<rect x="72" y="97" width="14" height="2.2" rx="1" fill="#e9fff6"/>' +
      '<rect x="27" y="96" width="16" height="7" rx="3.5" fill="#f2b84b"/>' +
      '<rect x="35" y="96" width="8" height="7" rx="3.5" fill="#e76f51"/>' +
    '</svg></span>';
}

const wzNametag = () => '<span class="wz-nametag">' + wzT('Wassit · AI pharmacist assistant', 'وسيط · مساعد صيدلي بالذكاء الاصطناعي') + '</span>';

function wzDecider(decider) {
  const names = {
    rules: wzT('Rules', 'القواعد'), human: wzT('Human', 'إنسان'), pharmacy: wzT('Pharmacy', 'الصيدلية'),
    client: wzT('You', 'أنت'), wassit: wzT('Wassit · relayed', 'وسيط · نقل فقط')
  };
  return '<span class="wz-decider ' + escapeHTML(decider) + '">' + escapeHTML(names[decider] || decider) + '</span>';
}

function wzMsg(kind, inner, { who = '', meta = '', card = false, badge = '' } = {}) {
  const side = kind === 'wassit' ? wzAvatar('sm') : kind === 'pharmacy' ? '<span class="wz-pharm-badge" aria-hidden="true">' + escapeHTML(badge || '?') + '</span>' : '';
  return '<div class="wz-msg ' + kind + '">' + side +
    '<div class="wz-bubble' + (card ? ' card' : '') + '">' +
      (who ? '<div class="wz-who">' + who + '</div>' : '') +
      inner +
      (meta ? '<div class="wz-meta">' + meta + '</div>' : '') +
    '</div></div>';
}

const wzTyping = text => '<div class="wz-msg wassit">' + wzAvatar('sm', true) +
  '<div class="wz-bubble wz-typing"><i></i><i></i><i></i><span>' + escapeHTML(text) + '</span></div></div>';

const wzPrivacy = () => '<p class="wz-privacy">' + icon('shield') + ' ' +
  wzT('Pharmacies only see the medicine, strength, form, quantity and pickup window. Never your name or phone. Wassit never suggests or swaps a medicine.',
      'الصيدليات ترى فقط الدواء والتركيز والشكل والكمية ووقت الاستلام. لا ترى اسمك أو هاتفك أبداً. وسيط لا يقترح ولا يستبدل أي دواء.') + '</p>';

// ---- Catalogue + branches ------------------------------------------------------------------
function wzMeds() { return liveState.medications || []; }
function wzBranches() {
  const live = (liveState.branches || []).map(b => ({ id: b.id, name: b.name }));
  return live.length ? live.slice(0, 5) : DEMO_BRANCH_PRESETS.map(p => ({ id: p.branchId, name: p.name }));
}
const wzShortName = name => String(name || '').replace(/^Cornilia Pharmacy\s*/i, '') || name;
const wzInitial = name => wzShortName(name).trim().charAt(0).toUpperCase();
const wzHue = id => [...String(id)].reduce((a, ch) => (a * 31 + ch.charCodeAt(0)) % 360, 7);

function wzMedCard(m) {
  return '<button type="button" class="wz-med" data-action="wz-pick:' + escapeHTML(m.id) + '">' +
    '<span class="wz-pack" style="--hue:' + wzHue(m.id) + '">' + icon('pill') + '</span>' +
    '<span class="wz-med-name">' + escapeHTML(m.brand_name) + ' ' + escapeHTML(m.strength) + '</span>' +
    '<span class="wz-med-sub">' + escapeHTML(m.active_ingredient) + ' · ' + escapeHTML(m.dosage_form) + '</span>' +
    '<span class="wz-med-tags">' +
      (m.rx_required ? chip(wzT('Prescription', 'بوصفة'), 'warm') : chip(wzT('No prescription', 'بدون وصفة'))) +
      (m.chronic_eligible ? chip(wzT('Chronic', 'مزمن'), 'info') : '') +
    '</span></button>';
}

function wzResultsHTML() {
  const q = wzState.query.trim().toLowerCase();
  const list = wzMeds().filter(m => !q || (m.brand_name + ' ' + m.active_ingredient + ' ' + m.strength + ' ' + m.dosage_form + ' ' + m.therapeutic_class).toLowerCase().includes(q));
  if (!wzMeds().length) return '<div class="wz-empty">' + wzT('Loading the catalogue…', 'جارٍ تحميل قائمة الأدوية…') + '</div>';
  if (!list.length) return '<div class="wz-empty">' + wzT('No medicine matches. Try the brand or the active ingredient.', 'لا يوجد دواء مطابق. جرّب الاسم التجاري أو المادة الفعالة.') + '</div>';
  return list.map(wzMedCard).join('');
}

// ---- Client home: search -------------------------------------------------------------------
function wzClientHome() {
  if (!wzMeds().length && typeof loadPublicCatalogue === 'function') loadPublicCatalogue();
  const name = escapeHTML(clientSession?.client?.display_name || '');
  const tickets = wzHasDemo() ? LiveClient.listTickets().slice(0, 3) : [];
  return '<div class="wz-page">' +
    '<section class="wz-hero">' + wzAvatar('lg') +
      '<div>' + wzNametag() +
        '<h1>' + wzT('What medicine do you need' + (name ? ', ' + name : '') + '?', 'ما الدواء الذي تحتاجه' + (name ? ' يا ' + name : '') + '؟') + '</h1>' +
        '<p>' + wzT('Pick it from the catalogue. I ask several pharmacies at once, read their answers, check doses and box photos against the official reference, and bring you the best verified price and expiry.',
                    'اختره من القائمة. أسأل عدة صيدليات في وقت واحد، أقرأ ردودهم، أتحقق من الجرعات وصور العلب مقابل المرجع الرسمي، وأعطيك أفضل سعر وتاريخ صلاحية موثّقين.') + '</p>' +
      '</div>' +
    '</section>' +
    '<form class="wz-search wz-form" id="wz-search-form" role="search">' +
      '<label class="sr" for="wz-q">' + wzT('Search medicines', 'ابحث عن دواء') + '</label>' +
      '<span class="wz-search-icon">' + icon('search') + '</span>' +
      '<input id="wz-q" type="search" autocomplete="off" value="' + escapeHTML(wzState.query) + '" placeholder="' + wzT('Search brand or ingredient, e.g. Concor, metformin', 'ابحث بالاسم أو المادة، مثلاً كونكور') + '">' +
      '<button class="btn" type="submit">' + wzT('Search', 'بحث') + '</button>' +
    '</form>' +
    '<div class="wz-results" id="wz-results">' + wzResultsHTML() + '</div>' +
    (tickets.length ? '<h2 class="wz-section-title">' + wzT('Your latest tickets', 'آخر طلباتك') + '</h2><div class="wz-tickets">' + tickets.map(wzTicketRow).join('') + '</div>' : '') +
  '</div>';
}

// ---- Client chat: draft a request ----------------------------------------------------------
function wzDraftFor(medId) {
  if (!wzState.draft || wzState.draft.medId !== medId) {
    wzState.draft = { medId, step: 'qty', quantity: null, mode: null, every: null, day: 5, pickup: null };
  }
  return wzState.draft;
}

function wzChips(items, prefix) {
  return '<div class="wz-chips">' + items.map(([value, text, pressed]) =>
    '<button type="button" class="wz-chip" data-action="' + prefix + ':' + escapeHTML(value) + '" aria-pressed="' + Boolean(pressed) + '">' + escapeHTML(text) + '</button>').join('') + '</div>';
}

function wzEveryText(every, day) {
  const d = wzDays()[day];
  if (every === 'week') return wzT('every ' + d, 'كل ' + d);
  if (every === '2weeks') return wzT('every other ' + d, d + ' كل أسبوعين');
  return wzT('one ' + d + ' a month', d + ' مرة في الشهر');
}
function wzEveryTextEn(every, day) {
  const d = wzDaysEn[day];
  return every === 'week' ? 'every ' + d : every === '2weeks' ? 'every other ' + d : 'one ' + d + ' a month';
}

function wzChatPage(medId) {
  const m = wzMeds().find(x => x.id === medId);
  if (!m) {
    if (typeof loadPublicCatalogue === 'function') loadPublicCatalogue();
    return '<div class="wz-page narrow"><div class="wz-empty">' + wzT('Loading this medicine…', 'جارٍ تحميل الدواء…') + '</div></div>';
  }
  const d = wzDraftFor(medId);
  const medText = m.brand_name + ' ' + m.strength + ' ' + m.dosage_form;
  let html = wzMsg('wassit', '<p>' + escapeHTML(wzT('You picked ' + medText + ' (' + m.active_ingredient + '). How many boxes do you need?',
    'اخترت ' + medText + ' (' + m.active_ingredient + '). كم علبة تحتاج؟')) + '</p>');
  if (d.step === 'qty') {
    html += wzChips([['1', '1'], ['2', '2'], ['3', '3'], ['4', '4']], 'wz-qty');
  } else {
    html += wzMsg('client', '<p>' + d.quantity + ' ' + wzT(d.quantity === 1 ? 'box' : 'boxes', 'علب') + '</p>');
    html += wzMsg('wassit', '<p>' + wzT('Is this a one-time request, or do you need it on a schedule?', 'هل هذا طلب لمرة واحدة أم تحتاجه بشكل دوري؟') + '</p>');
    if (d.step === 'mode') {
      html += wzChips([['once', wzT('Just once', 'مرة واحدة')], ['week', wzT('Every week', 'كل أسبوع')], ['2weeks', wzT('Every 2 weeks', 'كل أسبوعين')], ['month', wzT('Every month', 'كل شهر')]], 'wz-mode');
    } else {
      html += wzMsg('client', '<p>' + (d.mode === 'once' ? wzT('Just once', 'مرة واحدة') : wzT('On a schedule', 'بشكل دوري')) + '</p>');
      if (d.mode === 'scheduled') {
        html += wzMsg('wassit', '<p>' + wzT('Which day should I search for you?', 'في أي يوم أبحث لك؟') + '</p>');
        if (d.step === 'day') {
          html += wzChips(wzDays().map((name, i) => [String(i), name, i === d.day]), 'wz-day');
        } else {
          html += wzMsg('client', '<p>' + escapeHTML(wzEveryText(d.every, d.day)) + '</p>');
          html += wzMsg('wassit', '<p>' + wzT('Noted: no bulk buying. I open a fresh ticket ' + wzEveryText(d.every, d.day) + ' and search again each time.',
            'تم: بدون شراء بالجملة. أفتح طلباً جديداً ' + wzEveryText(d.every, d.day) + ' وأبحث من جديد في كل مرة.') + '</p>');
        }
      }
      if (['pickup', 'confirm'].includes(d.step)) {
        html += wzMsg('wassit', '<p>' + wzT('When can you pick it up?', 'متى يمكنك الاستلام؟') + '</p>');
        if (d.step === 'pickup') {
          const opts = d.mode === 'scheduled'
            ? [['Same day before 18:00', wzT('Same day before 18:00', 'نفس اليوم قبل 18:00')], ['Same day after 18:00', wzT('Same day after 18:00', 'نفس اليوم بعد 18:00')], ['Next morning', wzT('Next morning', 'صباح اليوم التالي')]]
            : [['Today before 18:00', wzT('Today before 18:00', 'اليوم قبل 18:00')], ['Tomorrow morning', wzT('Tomorrow morning', 'غداً صباحاً')], ['Within 2 days', wzT('Within 2 days', 'خلال يومين')]];
          html += wzChips(opts, 'wz-pickup');
        } else {
          html += wzMsg('client', '<p>' + escapeHTML(d.pickup) + '</p>');
          html += wzMsg('wassit',
            '<p>' + wzT('Here is exactly what pharmacies will see. Shall I send it?', 'هذا بالضبط ما ستراه الصيدليات. هل أرسله؟') + '</p>' +
            '<dl class="wz-summary">' +
              '<div><dt>' + wzT('Medicine', 'الدواء') + '</dt><dd>' + escapeHTML(m.brand_name + ' (' + m.active_ingredient + ')') + '</dd></div>' +
              '<div><dt>' + wzT('Strength · form', 'التركيز · الشكل') + '</dt><dd>' + escapeHTML(m.strength + ' · ' + m.dosage_form) + '</dd></div>' +
              '<div><dt>' + wzT('Quantity', 'الكمية') + '</dt><dd>' + d.quantity + '</dd></div>' +
              '<div><dt>' + wzT('Pickup', 'الاستلام') + '</dt><dd>' + escapeHTML(d.pickup) + '</dd></div>' +
              (d.mode === 'scheduled' ? '<div><dt>' + wzT('Schedule', 'الجدول') + '</dt><dd>' + escapeHTML(wzEveryText(d.every, d.day)) + ' · ' + wzT('no bulk', 'بدون جملة') + '</dd></div>' : '') +
            '</dl>' + wzPrivacy() +
            '<div class="actions">' + btn(wzT('Send to pharmacies', 'أرسل إلى الصيدليات'), 'wz-send', '') + btn(wzT('Start over', 'ابدأ من جديد'), 'wz-restart', 'secondary small') + '</div>',
            { card: true });
        }
      }
    }
  }
  return '<div class="wz-page narrow">' +
    '<div class="wz-ticket-head card"><div class="row">' + wzAvatar('md') + '<div><strong>' + wzT('New request', 'طلب جديد') + '</strong><p>' + escapeHTML(medText) + '</p></div></div>' +
      '<a class="btn secondary small" href="#client/home">' + wzT('← Back to search', '→ العودة للبحث') + '</a></div>' +
    '<div class="wz-chat">' + html + '</div></div>';
}

// ---- Ticket thread ---------------------------------------------------------------------------
function wzBranchName(id, ticket) {
  const fromTicket = ticket?.branches?.find(b => b.id === id);
  return wzShortName(fromTicket?.name || branchNameById(id));
}

function wzBranchState(ticket, evs, branchId) {
  const mine = evs.filter(e => e.branchId === branchId);
  const has = (kind, test = () => true) => mine.some(e => e.kind === kind && test(e));
  if (ticket.reservedBranchId === branchId) return ['match', wzT('Reserved', 'محجوز')];
  if (has('photo_check', e => e.data?.status === 'flagged')) return ['flagged', wzT('Photo: human check', 'الصورة: فحص بشري')];
  if (has('photo_check', e => e.data?.status === 'verified') && has('offer', e => e.data?.offer?.decision === 'matches')) return ['match', wzT('Verified offer', 'عرض موثّق')];
  if (has('dose_check', e => e.data && !e.data.accepted)) return ['rejected', wzT('Dose rejected', 'جرعة مرفوضة')];
  if (has('declined')) return ['declined', wzT('Declined', 'رفض')];
  if (has('no_answer')) return ['silent', wzT('No response', 'لا رد')];
  if (has('offer')) return ['sent', wzT('Offer read', 'تمت قراءة العرض')];
  if (has('pharmacy_reply')) return ['sent', wzT('Replied', 'رد')];
  return ['sent', wzT('Waiting', 'بانتظار الرد')];
}

const wzText = ev => (language === 'ar' && ev.textAr) ? ev.textAr : (ev.text || '');

function wzEventHTML(ev, ticket, evs, viewer) {
  const meta = wzDecider(ev.decider) + '<span>' + wzTime(ev.at) + '</span>';
  const bname = ev.branchId ? wzBranchName(ev.branchId, ticket) : '';
  switch (ev.kind) {
    case 'client_request':
      return viewer === 'client'
        ? wzMsg('client', '<p>' + escapeHTML(ev.text) + '</p>', { meta })
        : '';
    case 'wassit_note':
      return wzMsg('wassit', '<p>' + escapeHTML(wzText(ev)) + '</p>', { meta });
    case 'outreach': {
      const ids = ev.data?.branchIds || [];
      const pills = ids.filter(id => viewer === 'client' || id === viewer).map(id => {
        const [cls, label] = wzBranchState(ticket, evs, id);
        return '<span class="wz-pharm ' + cls + '"><i></i>' + escapeHTML(wzBranchName(id, ticket)) + ' · ' + escapeHTML(label) + '</span>';
      }).join('');
      return wzMsg('wassit',
        '<p><strong>' + (viewer === 'client'
          ? wzT('I messaged ' + ids.length + ' pharmacies at once.', 'راسلت ' + ids.length + ' صيدليات في وقت واحد.')
          : wzT('Wassit sent you this request for a client (identity hidden).', 'أرسل لك وسيط هذا الطلب لعميل (الهوية مخفية).')) + '</strong></p>' +
        '<div class="wz-outreach"><div class="wz-ring-row">' + pills + '</div>' +
        '<span class="eyebrow">' + wzT('What pharmacies saw', 'ما رأته الصيدليات') + '</span>' +
        '<blockquote class="wz-sent-msg" dir="auto">' + escapeHTML(ev.data?.message || '') + '</blockquote></div>',
        { meta, card: true });
    }
    case 'pharmacy_reply':
    case 'declined':
      return wzMsg('pharmacy', '<p dir="auto">' + escapeHTML(ev.text) + '</p>', {
        who: escapeHTML(bname) + (ev.kind === 'declined' ? ' · ' + wzT('declined', 'رفض') : ''),
        meta, badge: wzInitial(bname)
      });
    case 'no_answer':
      return wzMsg('system', '<p>' + escapeHTML(bname) + ': ' + escapeHTML(wzText(ev)) + '</p>', { meta });
    case 'offer': {
      const o = ev.data?.offer || {};
      return wzMsg('wassit',
        '<p>' + wzT('I read ' + bname + '’s reply into an offer. The rules decided:', 'قرأت رد ' + bname + ' كعرض. قرار القواعد:') + '</p>' +
        buildOfferCardHTML(o) +
        '<div class="wz-best-meta"><span>' + wzT('Total', 'المجموع') + ': <strong>' + wzMoney(o.totalCents) + '</strong></span>' +
          '<span>' + wzT('Expiry', 'الصلاحية') + ': <strong>' + escapeHTML(wzExpiry(o.expiry)) + '</strong></span>' +
          (o.equivalent ? chip(wzT('Rules-checked dose equivalent', 'جرعة مكافئة تحققت منها القواعد'), 'info') : '') + '</div>',
        { meta, card: true });
    }
    case 'dose_check': {
      const dd = ev.data || {};
      return wzMsg('wassit',
        '<div class="wz-check ' + (dd.accepted ? 'accepted' : 'rejected') + '">' +
          '<strong>' + (dd.accepted ? wzT('Dose equivalent accepted', 'جرعة مكافئة مقبولة') : wzT('Dose equivalent rejected', 'جرعة مكافئة مرفوضة')) + ' · ' + escapeHTML(bname) + '</strong>' +
          '<p>' + escapeHTML(wzText(ev)) + '</p>' +
          '<p class="caption">' + wzT('Requested', 'المطلوب') + ': ' + escapeHTML(dd.requested || '') + ' · ' + wzT('Offered', 'المعروض') + ': ' + escapeHTML(dd.offered || '') + '</p>' +
          '<ul class="wz-checks">' + (dd.checks || []).map(c => '<li class="' + (c.ok ? 'ok' : 'fail') + '">' + escapeHTML(c.label) + '</li>').join('') + '</ul>' +
          (dd.govRef ? '<p class="caption">' + wzT('Government reference', 'المرجع الحكومي') + ': ' + escapeHTML(dd.govRef.brandName + ' ' + dd.govRef.strength + ' · ' + dd.govRef.code) + '</p>' : '') +
        '</div>', { meta, card: true });
    }
    case 'photo_check': {
      const pd = ev.data || {};
      const flagged = pd.status !== 'verified';
      return wzMsg('wassit',
        '<div class="wz-photo ' + (flagged ? 'flagged' : 'verified') + '">' +
          '<span class="wz-photo-thumb' + (flagged ? ' blur' : '') + '" aria-hidden="true"></span>' +
          '<div><strong>' + (flagged ? wzT('Package photo flagged for a human', 'صورة العلبة محوّلة لإنسان') : wzT('Package photo checked', 'تم فحص صورة العلبة')) + ' · ' + escapeHTML(bname) + '</strong>' +
            '<p>' + escapeHTML(wzText(ev)) + '</p>' +
            '<p class="caption">' + wzT('Barcode', 'الباركود') + ': ' + escapeHTML(pd.barcode || '—') + ' · ' +
              wzT('Government reference', 'المرجع الحكومي') + ': ' + (pd.govMatch ? wzT('match', 'مطابق') : wzT('not confirmed', 'غير مؤكد')) + ' · ' +
              wzT('Expiry', 'الصلاحية') + ': ' + escapeHTML(wzExpiry(pd.expiry)) + '</p>' +
            '<p class="caption">' + escapeHTML(pd.note || '') + '</p>' +
          '</div></div>', { meta, card: true });
    }
    case 'best_offer': {
      const bd = ev.data || {};
      if (viewer !== 'client') return '';
      const reserved = ticket.status === 'reserved';
      return wzMsg('wassit',
        '<div class="wz-best">' +
          '<span class="eyebrow">' + wzT('Best verified offer', 'أفضل عرض موثّق') + '</span>' +
          (bd.branchId
            ? '<div class="wz-best-price">' + wzMoney(bd.totalCents) + '</div>' +
              '<div class="wz-best-meta"><strong>' + escapeHTML(wzBranchName(bd.branchId, ticket)) + '</strong><span>' + wzT('Expiry', 'الصلاحية') + ' ' + escapeHTML(wzExpiry(bd.expiry)) + '</span><span>' + escapeHTML(ticket.pickup) + '</span></div>'
            : '') +
          '<p>' + escapeHTML(wzText(ev)) + '</p>' +
          ((bd.excluded || []).length ? '<ul class="wz-best-excluded">' + bd.excluded.map(x => '<li>' + escapeHTML(wzBranchName(x.branchId, ticket)) + ': ' + escapeHTML(x.reason) + '</li>').join('') + '</ul>' : '') +
          (bd.branchId && !reserved ? '<div class="actions">' + btn(wzT('Reserve for pickup', 'احجز للاستلام'), 'wz-reserve:' + ticket.id + ':' + bd.branchId, '') + '</div>' : '') +
        '</div>', { meta, card: true });
    }
    case 'reserved':
      return wzMsg('client', '<p>' + escapeHTML(wzText(ev)) + '</p>', { meta });
    default:
      return '';
  }
}

function wzTicketStatus(ticket) {
  if (!wzHasDemo()) return ['waiting', ''];
  if (ticket.status === 'reserved') return ['reserved', wzT('Reserved', 'محجوز')];
  if (LiveClient.isWorking(ticket)) return ['working', wzT('Wassit is working', 'وسيط يعمل')];
  return LiveClient.bestOffer(ticket) ? ['done', wzT('Best offer ready', 'أفضل عرض جاهز')] : ['waiting', wzT('Needs a human', 'يحتاج إنساناً')];
}

function wzTicketRow(ticket) {
  const [cls, label] = wzTicketStatus(ticket);
  const route = page[0] === 'pharmacist' ? 'pharmacist/cticket/' : 'client/ticket/';
  return '<a class="wz-ticket card" href="#' + route + escapeHTML(ticket.id) + '">' +
    '<span class="wz-pack" style="--hue:' + wzHue(ticket.med.id) + '">' + icon('pill') + '</span>' +
    '<span><strong>' + escapeHTML(ticket.med.brandName + ' ' + ticket.med.strength) + ' × ' + ticket.quantity + '</strong>' +
      '<small style="display:block">' + escapeHTML(wzWhen(ticket.createdAt)) + ' · ' + (ticket.mode === 'scheduled' ? wzT('Scheduled', 'مجدول') : wzT('Once', 'مرة واحدة')) + ' · ' + escapeHTML(ticket.pickup) + '</small></span>' +
    '<span class="wz-status ' + cls + '">' + escapeHTML(label) + '</span></a>';
}

function wzThreadHTML(ticket, viewer) {
  const evs = LiveClient.events(ticket);
  const all = evs;
  let html = evs.map(ev => wzEventHTML(ev, ticket, all, viewer)).join('');
  if (viewer === 'client' && LiveClient.isWorking(ticket)) {
    const replied = all.filter(e => ['pharmacy_reply', 'declined'].includes(e.kind)).length;
    html += wzTyping(replied ? wzT('Reading replies and checking them against the rules…', 'أقرأ الردود وأتحقق منها بالقواعد…') : wzT('Contacting pharmacies…', 'أتواصل مع الصيدليات…'));
  }
  return html;
}

function wzTicketPage(ticketId) {
  if (!wzHasDemo()) return '<div class="wz-empty">' + wzT('Loading…', 'جارٍ التحميل…') + '</div>';
  const ticket = LiveClient.getTicket(ticketId);
  if (!ticket || !ticket.thread) {
    LiveClient.refreshTicket(ticketId).then(() => { if (page[2] === ticketId) wzRefreshDesk(); }).catch(err => status(err.message));
    if (!ticket) return '<div class="wz-page narrow"><div class="wz-empty">' + wzT('Loading ticket…', 'جارٍ تحميل الطلب…') + '</div></div>';
  }
  const [cls, label] = wzTicketStatus(ticket);
  const working = LiveClient.isWorking(ticket);
  return '<div class="wz-page narrow">' +
    '<div class="wz-ticket-head card"><div class="row">' + wzAvatar('md', working) +
      '<div><strong>' + escapeHTML(ticket.med.brandName + ' ' + ticket.med.strength + ' ' + ticket.med.form) + ' × ' + ticket.quantity + '</strong>' +
      '<p>' + wzT('Ticket', 'طلب') + ' ' + escapeHTML(ticket.id.slice(0, 8)) + ' · ' + escapeHTML(ticket.pickup) + (ticket.mode === 'scheduled' ? ' · ' + wzT('scheduled, no bulk', 'مجدول، بدون جملة') : '') + '</p></div></div>' +
      '<span class="wz-status ' + cls + '" id="wz-ticket-status">' + escapeHTML(label) + '</span></div>' +
    wzPrivacy() +
    '<div class="wz-chat" id="wz-thread" data-ticket="' + escapeHTML(ticket.id) + '" data-viewer="client" aria-live="polite">' + wzThreadHTML(ticket, 'client') + '</div>' +
  '</div>';
}

function wzTicketsPage() {
  const list = wzHasDemo() ? LiveClient.listTickets() : [];
  return '<div class="wz-page">' +
    '<section class="wz-greet card">' + wzAvatar('md') + '<div class="wz-bubble"><p>' +
      wzT('Every request gets a ticket. Open one to see each pharmacy’s answer and who decided what: the rules, a human, or the pharmacy.',
          'كل طلب له ملف. افتحه لترى رد كل صيدلية ومن قرر ماذا: القواعد أو إنسان أو الصيدلية.') + '</p></div></section>' +
    '<h2 class="wz-section-title">' + wzT('My tickets', 'طلباتي') + ' (' + list.length + ')</h2>' +
    (list.length ? '<div class="wz-tickets">' + list.map(wzTicketRow).join('') + '</div>'
      : '<div class="wz-empty">' + wzT('No tickets yet. Search a medicine to start.', 'لا توجد طلبات بعد. ابحث عن دواء للبدء.') + ' <a href="#client/home">' + wzT('Search', 'بحث') + '</a></div>') +
  '</div>';
}

// ---- Scheduled medicines -------------------------------------------------------------------
function wzSchedulesPage() {
  const list = wzHasDemo() ? LiveClient.listSchedules() : [];
  const short = language === 'ar' ? wzDays() : ['SUN', 'MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT'];
  return '<div class="wz-page">' +
    '<section class="wz-greet card">' + wzAvatar('md') + '<div class="wz-bubble"><p>' +
      wzT('For medicines you need on a fixed day and can’t buy in bulk. I open a new ticket every cycle and search again, so you never chase pharmacies yourself.',
          'للأدوية التي تحتاجها في يوم ثابت ولا يمكنك شراؤها بالجملة. أفتح طلباً جديداً في كل دورة وأبحث من جديد.') + '</p></div></section>' +
    '<h2 class="wz-section-title">' + wzT('Scheduled medicines', 'الأدوية المجدولة') + ' (' + list.length + ')</h2>' +
    (list.length ? '<div class="wz-schedules">' + list.map(s => {
      const next = LiveClient.nextRun(s);
      const tickets = s.ticketIds.map(id => LiveClient.getTicket(id)).filter(Boolean).reverse();
      return '<article class="wz-schedule card">' +
        '<div class="row"><span class="wz-cal"><span>' + escapeHTML(short[s.day]) + '</span></span>' +
          '<div><strong>' + escapeHTML(s.med.brandName + ' ' + s.med.strength) + '</strong>' +
          '<p>' + escapeHTML(s.med.ingredient + ' · ' + s.med.form) + ' · ' + s.quantity + ' ' + wzT('boxes', 'علب') + '</p></div></div>' +
        '<div class="row" style="margin-top:10px">' + chip(escapeHTML(wzEveryText(s.every, s.day))) +
          (s.noBulk ? '<span class="tag wz-nobulk">' + wzT('No bulk', 'بدون جملة') + '</span>' : '') +
          (s.paused ? chip(wzT('Paused', 'متوقف'), 'neutral') : '') + '</div>' +
        '<p class="caption" style="margin-top:8px">' + wzT('Pickup', 'الاستلام') + ': ' + escapeHTML(s.pickup) + ' · ' +
          (s.paused ? wzT('Paused', 'متوقف') : wzT('Next search', 'البحث التالي') + ': ' + escapeHTML(wzWhen(next))) + '</p>' +
        '<div class="actions">' + btn(wzT('Run this cycle now (demo)', 'شغّل هذه الدورة الآن (تجريبي)'), 'wz-run:' + s.id, 'small') +
          btn(s.paused ? wzT('Resume', 'استئناف') : wzT('Pause', 'إيقاف'), 'wz-pause:' + s.id, 'secondary small') + '</div>' +
        '<ul class="wz-history">' + (tickets.length ? tickets.map(tk => '<li><a href="#client/ticket/' + escapeHTML(tk.id) + '">' + escapeHTML(wzWhen(tk.createdAt)) + '</a> · ' + escapeHTML(wzTicketStatus(tk)[1]) + '</li>').join('') : '<li>' + wzT('No cycles yet.', 'لا توجد دورات بعد.') + '</li>') + '</ul>' +
      '</article>';
    }).join('') + '</div>'
      : '<div class="wz-empty">' + wzT('Nothing scheduled. Pick a medicine and choose “Every week”.', 'لا شيء مجدول. اختر دواءً ثم «كل أسبوع».') + ' <a href="#client/home">' + wzT('Search', 'بحث') + '</a></div>') +
  '</div>';
}

// ---- Pharmacist side -----------------------------------------------------------------------
function wzPharmGreet() {
  const name = liveSession?.staff?.full_name || wzT('pharmacist', 'الصيدلي');
  const branchId = liveSession?.branch?.id;
  const waiting = (liveState.inboxItems || []).filter(i => i.target_status === 'sent' || i.target_status === 'no_response').length;
  const hitl = (liveState.outgoingEnquiries || []).filter(e => e.status === 'awaiting_clarification' || e.status === 'awaiting_approval').length;
  const clientWaiting = wzHasDemo() && branchId ? LiveClient.clientInbox().filter(item => item.target_status === 'sent' || item.target_status === 'no_response').length : 0;
  const hour = new Date().getHours();
  const hello = hour < 12 ? wzT('Good morning', 'صباح الخير') : hour < 18 ? wzT('Good afternoon', 'مساء الخير') : wzT('Good evening', 'مساء الخير');
  return '<section class="wz-greet card">' + wzAvatar('md', waiting + clientWaiting > 0) +
    '<div><div class="wz-bubble"><p><strong>' + escapeHTML(hello + ', ' + name) + '.</strong> ' +
      wzT(waiting + ' pharmacy enquiries and ' + clientWaiting + ' client tickets wait for your reply' + (hitl ? ', and ' + hitl + ' of your own enquiries need your OK.' : '.') + ' I relay and read; you and the rules decide.',
          'هناك ' + waiting + ' استفسارات من صيدليات و' + clientWaiting + ' طلبات عملاء بانتظار ردك' + (hitl ? '، و' + hitl + ' من استفساراتك تحتاج موافقتك.' : '.') + ' أنا أنقل وأقرأ فقط؛ أنت والقواعد تقررون.') +
    '</p></div>' +
    '<div class="wz-chips">' +
      '<a class="wz-chip" href="#pharmacist/requests">' + wzT('Client tickets', 'طلبات العملاء') + ' (' + clientWaiting + ')</a>' +
      '<a class="wz-chip" href="#pharmacist/approvals">' + wzT('Peer inbox', 'صندوق الصيدليات') + ' (' + waiting + ')</a>' +
      '<a class="wz-chip" href="#pharmacist/inventory">' + wzT('Update stock', 'تحديث المخزون') + '</a>' +
      '<a class="wz-chip" href="#pharmacist/reference">' + wzT('Government reference', 'المرجع الحكومي') + '</a>' +
    '</div></div></section>';
}

function wzPharmTicketsPage() {
  const list = wzHasDemo() ? LiveClient.clientInbox() : [];
  return '<div class="wz-page">' + wzPharmGreet() +
    '<h2 class="wz-section-title">' + wzT('Client tickets for', 'طلبات العملاء لـ') + ' ' + escapeHTML(wzShortName(liveSession?.branch?.name || '')) + ' (' + list.length + ')</h2>' +
    '<p class="caption">' + wzT('Requests Wassit sent you on behalf of a client (identity hidden). Reply in your own words; Wassit reads it and the rules decide. Then add a package photo with the code and expiry you see on the box.',
      'طلبات أرسلها وسيط نيابة عن عميل (الهوية مخفية). رد بكلماتك؛ وسيط يقرأ والقواعد تقرر. ثم أضف صورة العلبة مع الرمز وتاريخ الصلاحية.') + '</p>' +
    (list.length ? '<div class="stack">' + list.map(item =>
      renderInboxItemCard(item) +
      '<form class="form wz-form wz-photo-form card" data-target-id="' + escapeHTML(item.target_id) + '" style="margin-top:8px">' +
        '<div class="row split"><strong>' + wzT('Package photo', 'صورة العلبة') + '</strong>' +
          (item.pickup_window ? chip(wzT('Pickup', 'الاستلام') + ': ' + escapeHTML(item.pickup_window), 'info') : '') + '</div>' +
        '<div class="inbox-reply-grid">' +
          '<label class="field">' + wzT('Photo', 'الصورة') + '<input type="file" name="photo" accept="image/png,image/jpeg" required></label>' +
          '<label class="field">' + wzT('Product code on the box', 'رمز المنتج على العلبة') + '<input name="code" inputmode="numeric" maxlength="20" required placeholder="6280000000001"></label>' +
          '<label class="field">' + wzT('Expiry (YYYY-MM)', 'الصلاحية (YYYY-MM)') + '<input name="expiry_month" pattern="\\d{4}-\\d{2}" maxlength="7" required placeholder="2027-08"></label>' +
          '<button class="btn secondary small" type="submit">' + wzT('Upload', 'رفع') + '</button>' +
        '</div>' +
        '<p class="caption">' + wzT('Go checks the code against the government reference and the expiry against the pickup window. A photo alone does not prove a box is genuine; unclear photos go to a human reviewer.',
          'يتحقق النظام من الرمز مقابل المرجع الحكومي والصلاحية مقابل وقت الاستلام. الصورة وحدها لا تثبت أن العلبة أصلية؛ الصور غير الواضحة تذهب لمراجع بشري.') + '</p>' +
      '</form>').join('') + '</div>'
      : '<div class="wz-empty">' + wzT('No client tickets for this branch yet.', 'لا توجد طلبات عملاء لهذا الفرع بعد.') + '</div>') +
  '</div>';
}

function wzPharmTicketPage() { return wzPharmTicketsPage(); }

function wzReferencePage() {
  if (liveState.govReference === undefined && typeof loadGovReference === 'function') {
    liveState.govReference = null;
    loadGovReference().then(rows => { if (rows && page[1] === 'reference') wzRefreshDesk(); });
  }
  const live = Array.isArray(liveState.govReference) && liveState.govReference.length;
  const rows = live ? liveState.govReference : [];
  if (!LiveClient.refDocsLoaded && liveSession?.token) { LiveClient.refDocsLoaded = true; LiveClient.refreshReferenceDocs().then(() => { if (page[1] === 'reference') wzRefreshDesk(); }).catch(() => {}); }
  const docs = LiveClient.listReferenceDocs();
  return '<div class="wz-page">' +
    '<section class="wz-greet card">' + wzAvatar('md') + '<div class="wz-bubble"><p>' +
      wzT('The rules compare every dose equivalent and box photo against this reference. Upload an official price list or leaflet to add to it.',
          'تقارن القواعد كل جرعة مكافئة وصورة علبة بهذا المرجع. ارفع لائحة أسعار رسمية أو نشرة لإضافتها.') + '</p></div></section>' +
    '<div class="demo-sim-banner"><div>' + chip(live ? 'Live · demo seed' : 'Demo seed', 'demo-badge') + ' ' + wzT((live ? 'From the backend (GET /api/v1/reference/products). ' : '') + 'Seeded demo data, not real Ministry of Public Health records.', 'بيانات تجريبية، ليست سجلات وزارة الصحة الحقيقية.') + '</div></div>' +
    '<form class="row wz-form" id="wz-doc-form" style="margin:14px 0">' +
      '<label class="field" style="flex:1">' + wzT('Official document (PDF or image)', 'مستند رسمي (PDF أو صورة)') + '<input type="file" name="doc" accept="application/pdf,image/*" required></label>' +
      '<button class="btn secondary small" type="submit">' + wzT('Upload', 'رفع') + '</button></form>' +
    (docs.length ? '<ul class="wz-history">' + docs.map(dc => '<li>' + escapeHTML(dc.name) + ' · ' + wzWhen(dc.addedAt) + '</li>').join('') + '</ul>' : '') +
    '<div class="wz-table-wrap"><table class="wz-table"><thead><tr><th>' + wzT('Code', 'الرمز') + '</th><th>' + wzT('Product', 'المنتج') + '</th><th>' + wzT('Ingredient', 'المادة') + '</th><th>' + wzT('Form', 'الشكل') + '</th><th>' + wzT('Official price', 'السعر الرسمي') + '</th></tr></thead><tbody>' +
      rows.map(rw => '<tr><td><code>' + escapeHTML(rw.code) + '</code></td><td>' + escapeHTML(rw.brandName + ' ' + rw.strength) + '</td><td>' + escapeHTML(rw.ingredient) + '</td><td>' + escapeHTML(rw.form) + '</td><td>' + wzMoney(rw.officialPriceCents) + '</td></tr>').join('') +
    '</tbody></table></div></div>';
}

// ---- Events ----------------------------------------------------------------------------------
function wzRefreshDesk() {
  const deskEl = document.querySelector('.desk');
  if (deskEl && typeof desk === 'function') deskEl.innerHTML = desk();
}
function wzScrollThread() {
  const last = document.querySelector('#wz-thread > :last-child, .wz-chat > :last-child');
  last?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
}

document.addEventListener('click', async e => {
  const el = e.target.closest('[data-action^="wz-"]');
  if (!el) return;
  const [action, key, value] = el.dataset.action.split(':');
  const d = wzState.draft;
  try {
    switch (action) {
      case 'wz-pick':
        wzState.draft = null;
        location.hash = 'client/chat/' + key;
        return;
      case 'wz-qty': d.quantity = Number(key); d.step = 'mode'; break;
      case 'wz-mode':
        if (key === 'once') { d.mode = 'once'; d.step = 'pickup'; } else { d.mode = 'scheduled'; d.every = key; d.step = 'day'; }
        break;
      case 'wz-day': d.day = Number(key); d.step = 'pickup'; break;
      case 'wz-pickup': d.pickup = el.dataset.action.split(':').slice(1).join(':'); d.step = 'confirm'; break;
      case 'wz-restart': wzState.draft = null; break;
      case 'wz-send': {
        const m = wzMeds().find(x => x.id === d.medId);
        if (!m) return;
        el.disabled = true;
        const med = LiveClient.medFromApi(m);
        let ticket;
        if (d.mode === 'scheduled') {
          const sched = await LiveClient.createSchedule({ med, quantity: d.quantity, every: d.every, day: d.day, pickup: d.pickup });
          ticket = await LiveClient.runSchedule(sched.id);
        } else {
          ticket = await LiveClient.createTicket({ med, quantity: d.quantity, pickup: d.pickup });
        }
        wzState.draft = null;
        status(wzT('Request sent to pharmacies', 'تم إرسال الطلب إلى الصيدليات'));
        location.hash = 'client/ticket/' + ticket.id;
        return;
      }
      case 'wz-reserve':
        await LiveClient.reserve(key, value);
        status(wzT('Reserved', 'تم الحجز'));
        break;
      case 'wz-run': {
        const ticket = await LiveClient.runSchedule(key);
        location.hash = 'client/ticket/' + ticket.id;
        return;
      }
      case 'wz-pause': await LiveClient.togglePause(key); break;
      default: return;
    }
  } catch (err) {
    el.disabled = false;
    status(err.message);
    alert(err.message);
    return;
  }
  wzRefreshDesk();
  wzScrollThread();
});

document.addEventListener('input', e => {
  if (e.target.id !== 'wz-q') return;
  wzState.query = e.target.value;
  const box = document.getElementById('wz-results');
  if (box) box.innerHTML = wzResultsHTML();
});

document.addEventListener('submit', async e => {
  const form = e.target;
  if (!form.classList.contains('wz-form')) return;
  e.preventDefault();
  const fd = new FormData(form);
  try {
    if (form.id === 'wz-search-form') {
      const box = document.getElementById('wz-results');
      if (box) box.innerHTML = wzResultsHTML();
      return;
    }
    if (form.classList.contains('wz-photo-form')) {
      const res = await LiveClient.pharmacyPhoto(form.dataset.targetId, fd.get('photo'), String(fd.get('code') || '').trim(), String(fd.get('expiry_month') || '').trim());
      status(wzT('Photo checked: ', 'تم فحص الصورة: ') + (res?.status || ''));
      alert(wzT('Photo result: ', 'نتيجة الصورة: ') + (res?.status || '') + (res?.note ? ' · ' + res.note : ''));
    } else if (form.id === 'wz-stock-form') {
      await LiveClient.setStock(form.dataset.med, Number(fd.get('qty')));
      status(wzT('Stock saved on the backend', 'تم حفظ المخزون في الخادم'));
    } else if (form.id === 'wz-doc-form') {
      await LiveClient.addReferenceDoc(fd.get('doc'));
      status(wzT('Document uploaded', 'تم رفع المستند'));
    } else {
      return;
    }
  } catch (err) {
    status(err.message);
    alert(err.message);
    return;
  }
  wzRefreshDesk();
});

// Live ticket updates: re-render the client thread only when its visible events change.
setInterval(() => {
  const thread = document.getElementById('wz-thread');
  if (!thread || !wzHasDemo()) return;
  const ticket = LiveClient.getTicket(thread.dataset.ticket);
  if (!ticket) return;
  const evs = LiveClient.events(ticket);
  const sig = ticket.id + '|' + evs.length + '|' + LiveClient.isWorking(ticket) + '|' + ticket.status + '|' + language;
  if (sig === wzState.lastSig) return;
  const first = !wzState.lastSig.startsWith(ticket.id + '|');
  wzState.lastSig = sig;
  if (first) return;
  wzRefreshDesk();
  wzScrollThread();
}, 700);
