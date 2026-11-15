// DEMO ADAPTER — simulated client tickets, schedules, government reference, photo checks and stock. No backend routes yet; Phase 3 replaces each method with an apiCall. Data lives in localStorage key `dosyn-demo-v2`.
// Pure logic: no DOM, no imports, no references to other page scripts. Everything is private to the IIFE below.

const DemoAdapter = (() => {
  const KEY = 'dosyn-demo-v2';
  const SOURCE = 'Demo seed — not real MoPH data';
  const DAY = 24 * 60 * 60 * 1000;
  const BEST_DELAY = 500;
  const HAMRA = 'branch-hamra';
  const FALLBACK_BARCODE = '6280000000000';
  const DEFAULT_PICKUP = 'Today before 18:00';
  const NOTE_VERIFIED = 'Barcode matches the government reference. A photo alone does not prove a box is genuine.';
  const NOTE_BLUR = "Photo is blurry; the expiry date can't be read. A pharmacist on our team checks it. A photo alone does not prove a box is genuine.";
  const NOTE_UNCLEAR = 'Photo unclear; a pharmacist on our team checks it. A photo alone does not prove a box is genuine.';
  const NO_ANSWER_EN = 'No reply after one follow-up. Recorded as no response, never as unavailable.';
  const NO_ANSWER_AR = 'لا رد بعد متابعة واحدة. يُسجَّل كـ «لا رد»، وليس «غير متوفر».';
  const NEG_RE = /\b(la2|la|no|not available|ma\s?3a?nd[ei]|ma\s?fi|pas dispo)\b|مافي|ما في|ما عندي|لا يوجد/i;
  const PRICE_RE = /(\d+(?:\.\d+)?)\s*(\$|usd|dollars?)/i;
  const QTY_RE = /(\d+)\s*(boxes|box|3elab|3elbe|packs?|علب|علبة)/i;
  const EXP_RE = /(\d{1,2})\s*[\/-]\s*(\d{4})/;
  const IN_STOCK_RE = /\b(eh\s+)?3and[ei]\b/i;
  const PAY_RE = /\b(cash|card)\b/i;
  const TIME_RE = /\b\d{1,2}:\d{2}\b/;
  const AR_LABEL = { matches: 'مطابق للطلب', rejected: 'مرفوض حسب القواعد', escalated: 'محوّل لشخص لمراجعته' };

  // Demo government reference: [code, brandName, ingredient, strength, form, officialPriceCents]
  const GOV_SEED = [
    ['6280000000011', 'Augmentin', 'Amoxicillin/Clavulanate', '1000mg', 'tablet', 1200],
    ['6280000000028', 'Augmentin', 'Amoxicillin/Clavulanate', '625mg', 'tablet', 1450],
    ['6280000000035', 'Augmentin', 'Amoxicillin/Clavulanate', '500mg', 'tablet', 700],
    ['6280000000042', 'Brufen', 'Ibuprofen', '400mg', 'tablet', 600],
    ['6280000000059', 'Brufen', 'Ibuprofen', '200mg', 'tablet', 350],
    ['6280000000066', 'Concor', 'Bisoprolol', '5mg', 'tablet', 900],
    ['6280000000073', 'Concor', 'Bisoprolol', '2.5mg', 'tablet', 520],
    ['6280000000080', 'Glucophage', 'Metformin', '1000mg', 'tablet', 650],
    ['6280000000097', 'Glucophage', 'Metformin', '500mg', 'tablet', 380],
    ['6280000000103', 'Lexotanil', 'Bromazepam', '3mg', 'tablet', 500],
    ['6280000000110', 'Lipitor', 'Atorvastatin', '20mg', 'tablet', 2400],
    ['6280000000127', 'Lipitor', 'Atorvastatin', '10mg', 'tablet', 1300],
    ['6280000000134', 'Nexium', 'Esomeprazole', '20mg', 'tablet', 1450],
    ['6280000000141', 'Panadol Advance', 'Paracetamol', '500mg', 'tablet', 450],
    ['6280000000158', 'Panadol', 'Paracetamol', '250mg', 'tablet', 260],
    ['6280000000165', 'Ventolin', 'Salbutamol', '100mcg', 'inhaler', 750],
    ['6280000000172', 'Zithromax', 'Azithromycin', '500mg', 'tablet', 1100],
    ['6280000000189', 'Zithromax', 'Azithromycin', '250mg', 'tablet', 600],
  ];
  const GOV = GOV_SEED.map((r, i) => ({
    code: r[0],
    regNo: 'DEMO-' + String(i + 1).padStart(3, '0'),
    brandName: r[1],
    ingredient: r[2],
    strength: r[3],
    form: r[4],
    officialPriceCents: r[5],
    source: SOURCE,
  }));

  // ---------- small helpers ----------

  function uid() {
    const hasUuid = typeof crypto !== 'undefined' && crypto && typeof crypto.randomUUID === 'function';
    return (hasUuid ? crypto.randomUUID() : null) ?? Math.random().toString(36).slice(2);
  }

  function norm(s) { return String(s == null ? '' : s).trim().toLowerCase(); }
  function pad2(n) { return String(n).padStart(2, '0'); }
  function money(cents) { return (Math.round(Number(cents) || 0) / 100) + '$'; }
  function escapeRe(s) { return String(s).replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); }
  function pick(re, text) { const m = re.exec(text); return m ? m[0] : null; }

  function parseStrength(s) {
    const m = /^\s*(\d+(?:\.\d+)?)\s*([a-zA-Z%]*)\s*$/.exec(String(s == null ? '' : s));
    return m ? { value: parseFloat(m[1]), unit: m[2] || '' } : { value: NaN, unit: '' };
  }

  function fmtStrength(value, unit) {
    return String(Math.round(Number(value) * 100) / 100) + (unit || '');
  }

  function normMed(m) {
    const src = m || {};
    const strength = typeof src.strength === 'number' ? fmtStrength(src.strength, 'mg') : String(src.strength || '');
    return {
      id: src.id || uid(),
      brandName: String(src.brandName || ''),
      ingredient: String(src.ingredient || ''),
      strength,
      form: String(src.form || 'tablet'),
      unitPriceCents: Number(src.unitPriceCents) || 0,
    };
  }

  function medFromApi(m) {
    const src = m || {};
    return normMed({
      id: src.id,
      brandName: src.brand_name,
      ingredient: src.active_ingredient,
      strength: src.strength,
      form: src.dosage_form,
      unitPriceCents: src.unit_price_cents,
    });
  }

  function refOf(row) {
    return row ? { code: row.code, brandName: row.brandName, strength: row.strength } : null;
  }

  // ---------- government reference ----------

  function govReference() { return GOV.slice(); }

  function govLookup(ingredient, strength, form) {
    const ing = norm(ingredient);
    const f = norm(form);
    let want;
    let wantUnit = '';
    if (typeof strength === 'number') {
      want = strength;
    } else {
      const p = parseStrength(strength);
      want = p.value;
      wantUnit = norm(p.unit);
    }
    return GOV.find((r) => {
      const rs = parseStrength(r.strength);
      return norm(r.ingredient) === ing
        && norm(r.form) === f
        && rs.value === want
        && (!wantUnit || norm(rs.unit) === wantUnit);
    }) || null;
  }

  // Same ingredient + form, strength does not divide the requested strength evenly.
  function findOther(med, sval) {
    if (Number.isNaN(sval.value)) return null;
    const ing = norm(med.ingredient);
    const f = norm(med.form);
    return GOV.find((r) => {
      const rs = parseStrength(r.strength);
      return norm(r.ingredient) === ing
        && norm(r.form) === f
        && norm(rs.unit) === norm(sval.unit)
        && rs.value !== sval.value
        && sval.value % rs.value !== 0;
    }) || null;
  }

  // ---------- storage (write-through, memory is the source of truth in this session) ----------

  function emptyState() { return { tickets: [], schedules: [], docs: [], stock: {} }; }

  function loadState() {
    try {
      const raw = typeof localStorage !== 'undefined' ? localStorage.getItem(KEY) : null;
      const p = raw ? JSON.parse(raw) : null;
      if (p && typeof p === 'object') {
        return {
          tickets: Array.isArray(p.tickets) ? p.tickets : [],
          schedules: Array.isArray(p.schedules) ? p.schedules : [],
          docs: Array.isArray(p.docs) ? p.docs : [],
          stock: p.stock && typeof p.stock === 'object' ? p.stock : {},
        };
      }
    } catch (e) {
      // storage blocked or corrupt: start empty and work in memory
    }
    return emptyState();
  }

  let state = loadState();

  function save() {
    try {
      if (typeof localStorage !== 'undefined') localStorage.setItem(KEY, JSON.stringify(state));
    } catch (e) {
      // quota or blocked: keep working in memory
    }
  }

  // ---------- event helpers ----------

  function ev(kind, decider, branchId, text, textAr, data) {
    return { id: uid(), at: 0, kind, decider, branchId: branchId || null, text: text || '', textAr: textAr || '', data: data || {} };
  }

  function nextManualAt(ticket) {
    const last = ticket.manual.length ? ticket.manual[ticket.manual.length - 1].at : 0;
    return Math.max(Date.now(), last + 1);
  }

  function addManual(ticket, e) {
    e.at = nextManualAt(ticket);
    ticket.manual.push(e);
    return e;
  }

  function branchName(ticket, branchId) {
    const b = ticket.branches.find((x) => x.id === branchId);
    return b ? b.name : String(branchId || '');
  }

  function lastOffset(ticket) {
    return ticket.script.reduce((m, e) => Math.max(m, e.at), 0);
  }

  function evidenceFor(text, strength) {
    const found = {};
    const add = (key, v) => { if (v) found[key] = v; };
    add('in_stock', pick(IN_STOCK_RE, text));
    add('quantity', pick(QTY_RE, text));
    add('price', pick(PRICE_RE, text));
    add('strength', strength ? pick(new RegExp(escapeRe(strength), 'i'), text) : null);
    add('pickup_by', pick(TIME_RE, text));
    add('payment', pick(PAY_RE, text));
    return found;
  }

  function offerLine(o) {
    const label = o.decision === 'matches' ? 'Matches the request'
      : o.decision === 'rejected' ? 'Rejected by rules' : 'Escalated to a human';
    const priceTxt = o.price_cents == null
      ? 'no price found'
      : `${o.quantity} × ${money(o.price_cents)} = ${money(o.totalCents)}`;
    return `${label}: ${priceTxt}${o.expiry ? ', expiry ' + o.expiry : ''}.`;
  }

  function offerLineAr(o) {
    const label = AR_LABEL[o.decision] || '';
    return o.price_cents == null ? label : `${label}: ${o.quantity} × ${money(o.price_cents)} = ${money(o.totalCents)}`;
  }

  // Builds a snake_case offer object (rendered by the backend-shaped offer card).
  function makeOffer(o) {
    const text = o.text || '';
    return {
      id: uid(),
      branch_id: o.branchId,
      source_text: text,
      evidence: evidenceFor(text, o.strength),
      in_stock: true,
      strength: o.strength,
      quantity: o.quantity,
      price_cents: o.priceCents == null ? null : o.priceCents,
      currency: 'USD',
      pickup_by: pick(TIME_RE, text),
      payment: pick(PAY_RE, text),
      confidence: 0.9,
      decision: o.decision,
      decided_by: 'rules',
      reasons: o.reasons,
      extracted_by: 'agent',
      created_at: new Date(o.at || Date.now()).toISOString(),
      expiry: o.expiry || null,
      totalCents: o.totalCents == null ? null : o.totalCents,
      equivalent: !!o.equivalent,
      unitStrength: o.unitStrength || o.strength,
    };
  }

  function offerEvent(branchId, offer) {
    return ev('offer', 'rules', branchId, offerLine(offer), offerLineAr(offer), { offer });
  }

  function photoEvent(o) {
    const flagged = o.status !== 'verified';
    return ev('photo_check', flagged ? 'human' : 'rules', o.branchId, o.text, o.textAr, {
      status: o.status,
      barcode: o.barcode,
      govMatch: !flagged,
      expiry: flagged ? null : o.expiry,
      note: o.note,
    });
  }

  function doseEvent(branchId, accepted, requested, offered, checks, govRef, text, textAr) {
    return ev('dose_check', 'rules', branchId, text, textAr, { accepted, requested, offered, checks, govRef });
  }

  // Parses a raw pharmacy reply into one offer event (rules decide, not AI).
  function parseReply(ticket, branchId, raw) {
    const priceM = PRICE_RE.exec(raw);
    const qtyM = QTY_RE.exec(raw);
    const expM = EXP_RE.exec(raw);
    const month = expM ? Number(expM[1]) : 0;
    const expiry = expM && month >= 1 && month <= 12 ? expM[2] + '-' + pad2(month) : null;
    const priceCents = priceM ? Math.round(parseFloat(priceM[1]) * 100) : null;
    const quantity = qtyM ? Math.max(1, parseInt(qtyM[1], 10)) : ticket.quantity;
    const totalCents = priceCents == null ? null : priceCents * quantity;
    let decision;
    let reasons;
    if (priceCents == null) {
      decision = 'escalated';
      reasons = ['No price given; a human checks it.'];
    } else if (!expiry) {
      decision = 'escalated';
      reasons = ['No expiry date given; a human checks it.'];
    } else {
      decision = 'matches';
      reasons = ['Same medicine, strength and form as requested', 'Quantity covers the request', 'Every field is quoted from the reply'];
    }
    const offer = makeOffer({
      branchId,
      text: raw,
      strength: ticket.med.strength,
      quantity,
      priceCents,
      decision,
      reasons,
      expiry,
      totalCents,
      equivalent: false,
      unitStrength: ticket.med.strength,
    });
    return offerEvent(branchId, offer);
  }

  // ---------- tickets ----------

  function listTickets() {
    return state.tickets.slice().sort((a, b) => b.createdAt - a.createdAt);
  }

  function getTicket(id) {
    return state.tickets.find((t) => t.id === id) || null;
  }

  function buildRoles(branches) {
    const roles = {};
    const order = ['match', 'declined', 'dose_rejected', 'equivalent'];
    let i = 0;
    branches.forEach((b) => {
      if (b.id === HAMRA) { roles[b.id] = 'silent'; return; }
      roles[b.id] = i < order.length ? order[i] : 'silent';
      i += 1;
    });
    return roles;
  }

  // Script events store `at` as an offset (ms) from createdAt.
  function buildScript(t, base) {
    const med = t.med;
    const S = med.strength;
    const B = med.brandName;
    const F = med.form;
    const Q = t.quantity;
    const n = t.branches.length;
    const sval = parseStrength(S);
    const gov = govLookup(med.ingredient, med.strength, med.form);
    const barcode = gov ? gov.code : FALLBACK_BARCODE;
    const half = govLookup(med.ingredient, sval.value / 2, med.form);
    const halfStr = fmtStrength(sval.value / 2, sval.unit);
    const other = findOther(med, sval);
    const otherStr = other ? other.strength : fmtStrength(sval.value * 0.75, sval.unit);
    const createdAt = t.createdAt;
    const script = [];
    const put = (offset, e) => { e.at = offset; script.push(e); };
    const roleBranch = (role) => Object.keys(t.roles).find((id) => t.roles[id] === role) || null;

    put(0, ev('client_request', 'client', null, `${B} ${S} ${F} × ${Q} · ${t.mode} · pickup ${t.pickup}`, '', {}));
    put(600, ev('wassit_note', 'wassit', null,
      `Got it. I'm asking ${n} pharmacies near you. They only see the medicine, strength, form, quantity and pickup window — never your name or phone number.`,
      `حصلت على الطلب. أسأل ${n} صيدليات قريبة منك. يرون الدواء والتركيز والشكل والكمية ونافذة الاستلام فقط، ولا يرون اسمك ولا رقم هاتفك أبداً.`,
      {}));
    put(1800, ev('outreach', 'wassit', null, `Asked ${n} pharmacies.`, `أرسلت الطلب إلى ${n} صيدليات.`, {
      branchIds: t.branches.map((b) => b.id),
      message: `Hello, do you have ${B} ${S} ${F} × ${Q} for pickup ${t.pickup}? Please reply with price and expiry date.`,
    }));

    const declinedId = roleBranch('declined');
    if (declinedId) {
      put(4500, ev('declined', 'pharmacy', declinedId, 'la2 ma 3ande, sorry', '', {}));
    }

    const matchId = roleBranch('match');
    if (matchId) {
      const priceTxt = `${base / 100}$`;
      const matchReply = `eh 3ande ${Q} 3elab, ${priceTxt} el 3elbe, expiry 08/2027, cash aw card`;
      put(6500, ev('pharmacy_reply', 'pharmacy', matchId, matchReply, '', {}));
      const offer = makeOffer({
        branchId: matchId,
        text: matchReply,
        strength: S,
        quantity: Q,
        priceCents: base,
        decision: 'matches',
        reasons: ['Same medicine, strength and form as requested', 'Quantity covers the request', 'Every field is quoted from the reply'],
        expiry: '2027-08',
        totalCents: base * Q,
        equivalent: false,
        unitStrength: S,
        at: createdAt + 7300,
      });
      put(7300, offerEvent(matchId, offer));
      put(9600, photoEvent({
        branchId: matchId,
        status: 'verified',
        expiry: '2027-08',
        barcode,
        note: NOTE_VERIFIED,
        text: 'Box photo checked: barcode matches the government reference.',
        textAr: 'فُحصت صورة العلبة: الباركود مطابق للمرجع الحكومي.',
      }));
    }

    const rejectId = roleBranch('dose_rejected');
    if (rejectId) {
      put(8500, ev('pharmacy_reply', 'pharmacy', rejectId, `ma fi ${S}, fi ${otherStr} bas`, '', {}));
      const listed = !!other;
      put(9200, doseEvent(rejectId, false,
        `${S} × ${Q}`,
        `${otherStr} × ${Q}`,
        [
          { label: 'Same active ingredient', ok: true },
          { label: 'Same form', ok: true },
          { label: 'Total strength matches', ok: false },
          { label: 'Listed in government reference', ok: listed },
        ],
        refOf(other),
        `Dose equivalent rejected: ${otherStr} tablets cannot add up to exactly ${S} per dose. Not counted as a match.`,
        `رُفضت معادلة الجرعة: ${otherStr} لا يمكن أن تُجمع لتعطي ${S} بالضبط في الجرعة. لا تُحتسب كتطابق.`));
    }

    const eqId = roleBranch('equivalent');
    if (eqId) {
      if (half) {
        const eqPrice = Math.round(base * 0.6);
        const eqReply = `3ande ${halfStr} bas, ${2 * Q} 3elab, ${eqPrice / 100}$ kel wehde, exp 03/2027`;
        put(11000, ev('pharmacy_reply', 'pharmacy', eqId, eqReply, '', {}));
        put(11800, doseEvent(eqId, true,
          `${S} × ${Q}`,
          `${halfStr} × ${2 * Q}`,
          [
            { label: 'Same active ingredient', ok: true },
            { label: 'Same form', ok: true },
            { label: 'Total strength matches', ok: true },
            { label: 'Listed in government reference', ok: true },
          ],
          refOf(half),
          `Dose equivalent accepted by rules: 2 × ${halfStr} = 1 × ${S}, same ingredient and form, listed in the government reference.`,
          `قبلت القواعد معادلة الجرعة: 2 × ${halfStr} = 1 × ${S}، نفس المادة الفعالة والشكل، ومدرجة في المرجع الحكومي.`));
        const offer = makeOffer({
          branchId: eqId,
          text: eqReply,
          strength: S,
          quantity: 2 * Q,
          priceCents: eqPrice,
          decision: 'matches',
          reasons: [`Rules-checked dose equivalent: 2 × ${halfStr} = ${S}`, 'Quantity covers the request', 'Every field is quoted from the reply'],
          expiry: '2027-03',
          totalCents: eqPrice * 2 * Q,
          equivalent: true,
          unitStrength: halfStr,
          at: createdAt + 12400,
        });
        put(12400, offerEvent(eqId, offer));
        put(14000, photoEvent({
          branchId: eqId,
          status: 'flagged',
          expiry: null,
          barcode,
          note: NOTE_BLUR,
          text: "Photo is blurry; the expiry date can't be read. A pharmacist on our team checks it.",
          textAr: 'الصورة غير واضحة وتاريخ الانتهاء غير مقروء. يراجعها صيدلي من فريقنا.',
        }));
      } else {
        // Half-strength product is not in the reference: behaves like a plain match.
        const fbReply = `eh 3ande ${Q} 3elab, ${base / 100}$ el 3elbe, exp 03/2027`;
        put(11000, ev('pharmacy_reply', 'pharmacy', eqId, fbReply, '', {}));
        const offer = makeOffer({
          branchId: eqId,
          text: fbReply,
          strength: S,
          quantity: Q,
          priceCents: base,
          decision: 'matches',
          reasons: ['Same medicine, strength and form as requested', 'Quantity covers the request', 'Every field is quoted from the reply'],
          expiry: '2027-03',
          totalCents: base * Q,
          equivalent: false,
          unitStrength: S,
          at: createdAt + 12400,
        });
        put(12400, offerEvent(eqId, offer));
        put(14000, photoEvent({
          branchId: eqId,
          status: 'verified',
          expiry: '2027-03',
          barcode,
          note: NOTE_VERIFIED,
          text: 'Box photo checked: barcode matches the government reference.',
          textAr: 'فُحصت صورة العلبة: الباركود مطابق للمرجع الحكومي.',
        }));
      }
    }

    t.branches.forEach((b) => {
      if (t.roles[b.id] === 'silent') {
        put(15000, ev('no_answer', 'rules', b.id, NO_ANSWER_EN, NO_ANSWER_AR, {}));
      }
    });

    return script;
  }

  function createTicket(opts) {
    const o = opts || {};
    const med = normMed(o.med);
    const gov = govLookup(med.ingredient, med.strength, med.form);
    const base = med.unitPriceCents || (gov ? gov.officialPriceCents : 1000);
    const branches = (Array.isArray(o.branches) ? o.branches : []).map((b) => ({ id: b.id, name: b.name }));
    const ticket = {
      id: uid(),
      createdAt: Date.now(),
      med,
      quantity: Math.max(1, Math.floor(Number(o.quantity)) || 1),
      mode: o.mode === 'scheduled' ? 'scheduled' : 'once',
      scheduleId: o.scheduleId || null,
      pickup: o.pickup || DEFAULT_PICKUP,
      status: 'open',
      reservedBranchId: null,
      branches,
      roles: buildRoles(branches),
      script: [],
      manual: [],
    };
    ticket.script = buildScript(ticket, base);
    state.tickets.push(ticket);
    save();
    return ticket;
  }

  // Script + manual events, filtered by time. Manual events are always visible.
  function baseEvents(ticket, now) {
    const replied = new Set(ticket.manual.filter((e) => e.kind === 'pharmacy_reply').map((e) => e.branchId));
    const scripted = ticket.script
      .filter((e) => ticket.createdAt + e.at <= now && !(e.kind === 'no_answer' && replied.has(e.branchId)))
      .map((e) => Object.assign({}, e, { at: ticket.createdAt + e.at }));
    return scripted.concat(ticket.manual.map((e) => Object.assign({}, e)));
  }

  function reasonFor(vis, branchId, photo) {
    const mine = vis.filter((e) => e.branchId === branchId);
    if (mine.some((e) => e.kind === 'declined')) return 'Declined';
    if (mine.some((e) => (e.kind === 'dose_check' && e.data.accepted === false)
      || (e.kind === 'offer' && e.data.offer.decision === 'rejected'))) return 'Dose equivalent rejected by rules';
    if (mine.some((e) => e.kind === 'no_answer')) return 'No response';
    if (photo[branchId] === 'verified' && mine.some((e) => e.kind === 'offer' && e.data.offer.decision === 'matches')) return 'Higher total price';
    if (photo[branchId] === 'flagged') return 'Photo awaiting human check';
    return 'No verified photo yet';
  }

  function computeBest(ticket, now) {
    const vis = baseEvents(ticket, now).sort((a, b) => a.at - b.at);
    const photo = {};
    vis.forEach((e) => { if (e.kind === 'photo_check') photo[e.branchId] = e.data.status; });
    const cands = vis.filter((e) => e.kind === 'offer'
      && e.data.offer.decision === 'matches'
      && photo[e.branchId] === 'verified');
    cands.sort((a, b) => (a.data.offer.totalCents - b.data.offer.totalCents)
      || String(b.data.offer.expiry || '').localeCompare(String(a.data.offer.expiry || '')));
    const top = cands[0] || null;
    const best = top ? {
      branchId: top.branchId,
      totalCents: top.data.offer.totalCents,
      expiry: top.data.offer.expiry,
      offer: top.data.offer,
    } : null;
    const excluded = ticket.branches
      .filter((b) => !best || b.id !== best.branchId)
      .map((b) => ({ branchId: b.id, reason: reasonFor(vis, b.id, photo) }));
    return { best, excluded };
  }

  function bestEvent(ticket, now) {
    const res = computeBest(ticket, now);
    const best = res.best;
    const data = best
      ? { branchId: best.branchId, totalCents: best.totalCents, expiry: best.expiry, excluded: res.excluded }
      : { branchId: null, totalCents: null, expiry: null, excluded: res.excluded };
    const name = best ? branchName(ticket, best.branchId) : '';
    const text = best
      ? `Best verified offer: ${name} at ${money(best.totalCents)} total, expiry ${best.expiry}.`
      : 'No verified offer yet. An offer needs a matching reply and a verified photo.';
    const textAr = best
      ? `أفضل عرض موثّق: ${name} بإجمالي ${money(best.totalCents)}، تاريخ الانتهاء ${best.expiry}.`
      : 'لا يوجد عرض موثّق بعد. العرض يحتاج رداً مطابقاً وصورة موثّقة.';
    return {
      id: 'best-' + ticket.id,
      at: ticket.createdAt + lastOffset(ticket) + BEST_DELAY,
      kind: 'best_offer',
      decider: 'rules',
      branchId: data.branchId,
      text,
      textAr,
      data,
    };
  }

  function events(ticket, now = Date.now()) {
    if (!ticket) return [];
    const list = baseEvents(ticket, now);
    if (now >= ticket.createdAt + lastOffset(ticket) + BEST_DELAY) list.push(bestEvent(ticket, now));
    return list.sort((a, b) => a.at - b.at);
  }

  function isWorking(ticket, now = Date.now()) {
    if (!ticket) return false;
    return ticket.script.some((e) => ticket.createdAt + e.at > now);
  }

  function bestOffer(ticket, now = Date.now()) {
    if (!ticket) return null;
    return computeBest(ticket, now).best;
  }

  function reserve(ticketId, branchId) {
    const ticket = getTicket(ticketId);
    if (!ticket) return null;
    const name = branchName(ticket, branchId);
    addManual(ticket, ev('reserved', 'client', branchId,
      `Reserved at ${name}`, `تم الحجز في ${name}`, {}));
    addManual(ticket, ev('wassit_note', 'wassit', null,
      `I asked ${name} to hold it until ${ticket.pickup}. Pay at the counter.`,
      `طلبت من ${name} الاحتفاظ به حتى ${ticket.pickup}. ادفع عند الكاونتر.`, {}));
    ticket.status = 'reserved';
    ticket.reservedBranchId = branchId;
    save();
    return ticket;
  }

  function ticketsForBranch(branchId) {
    return listTickets().filter((t) => t.branches.some((b) => b.id === branchId));
  }

  function branchThread(ticket, branchId, now = Date.now()) {
    return events(ticket, now).filter((e) => e.kind === 'outreach' || e.branchId === branchId);
  }

  function pharmacyReply(ticketId, branchId, text) {
    const ticket = getTicket(ticketId);
    if (!ticket) return [];
    const raw = String(text == null ? '' : text).trim();
    const out = [];
    out.push(addManual(ticket, ev('pharmacy_reply', 'pharmacy', branchId, raw, '', {})));
    if (NEG_RE.test(raw) && !PRICE_RE.test(raw)) {
      out.push(addManual(ticket, ev('declined', 'pharmacy', branchId, raw, '', {})));
    } else {
      out.push(addManual(ticket, parseReply(ticket, branchId, raw)));
    }
    save();
    return out;
  }

  function latestExpiry(ticket, branchId) {
    const offers = baseEvents(ticket, Date.now())
      .filter((e) => e.kind === 'offer' && e.branchId === branchId && e.data.offer.expiry)
      .sort((a, b) => a.at - b.at);
    return offers.length ? offers[offers.length - 1].data.offer.expiry : null;
  }

  function monthsFromNow(n) {
    const d = new Date();
    d.setDate(1);
    d.setMonth(d.getMonth() + n);
    return d.getFullYear() + '-' + pad2(d.getMonth() + 1);
  }

  function pharmacyPhoto(ticketId, branchId, fileName) {
    const ticket = getTicket(ticketId);
    if (!ticket) return null;
    const gov = govLookup(ticket.med.ingredient, ticket.med.strength, ticket.med.form);
    const barcode = gov ? gov.code : FALLBACK_BARCODE;
    let e;
    if (/blur|unclear/i.test(String(fileName || ''))) {
      e = photoEvent({
        branchId,
        status: 'flagged',
        expiry: null,
        barcode,
        note: NOTE_UNCLEAR,
        text: 'Photo unclear: a pharmacist on our team checks it.',
        textAr: 'الصورة غير واضحة: يراجعها صيدلي من فريقنا.',
      });
    } else {
      e = photoEvent({
        branchId,
        status: 'verified',
        expiry: latestExpiry(ticket, branchId) || monthsFromNow(18),
        barcode,
        note: NOTE_VERIFIED,
        text: 'Box photo checked: barcode matches the government reference.',
        textAr: 'فُحصت صورة العلبة: الباركود مطابق للمرجع الحكومي.',
      });
    }
    addManual(ticket, e);
    save();
    return e;
  }

  // ---------- schedules ----------

  function listSchedules() {
    return state.schedules.slice().sort((a, b) => b.createdAt - a.createdAt);
  }

  function getSchedule(id) {
    return state.schedules.find((s) => s.id === id) || null;
  }

  function createSchedule(opts) {
    const o = opts || {};
    const every = ['week', '2weeks', 'month'].indexOf(o.every) >= 0 ? o.every : 'week';
    const sched = {
      id: uid(),
      med: normMed(o.med),
      quantity: Math.max(1, Math.floor(Number(o.quantity)) || 1),
      every,
      day: (((Math.floor(Number(o.day)) || 0) % 7) + 7) % 7,
      pickup: o.pickup || DEFAULT_PICKUP,
      noBulk: true,
      paused: false,
      createdAt: Date.now(),
      ticketIds: [],
    };
    state.schedules.push(sched);
    save();
    return sched;
  }

  // Next `day` (0=Sunday) at 09:00 local time, strictly after `now`.
  function nextDayAt9(day, now) {
    const d = new Date(now);
    d.setHours(9, 0, 0, 0);
    d.setDate(d.getDate() + ((day - d.getDay() + 7) % 7));
    if (d.getTime() <= now) d.setDate(d.getDate() + 7);
    return d.getTime();
  }

  function addDaysAt9(ms, days) {
    const d = new Date(ms);
    d.setDate(d.getDate() + days);
    d.setHours(9, 0, 0, 0);
    return d.getTime();
  }

  function lastRunAt(s) {
    let last = null;
    (s.ticketIds || []).forEach((id) => {
      const t = getTicket(id);
      if (t && (last === null || t.createdAt > last)) last = t.createdAt;
    });
    return last;
  }

  function nextRun(s, now = Date.now()) {
    let t = nextDayAt9(s.day, now);
    const last = lastRunAt(s);
    if (s.every === '2weeks' && last !== null && t - last < 13 * DAY) {
      t = nextDayAt9(s.day, Math.max(now, last + 13 * DAY));
    }
    if (s.every === 'month' && last !== null && now - last <= 21 * DAY) {
      t = addDaysAt9(t, 21);
    }
    return t;
  }

  function runSchedule(id, branches) {
    const s = getSchedule(id);
    if (!s) return null;
    const ticket = createTicket({
      med: s.med,
      quantity: s.quantity,
      mode: 'scheduled',
      pickup: s.pickup,
      scheduleId: s.id,
      branches,
    });
    s.ticketIds.push(ticket.id);
    save();
    return ticket;
  }

  function togglePause(id) {
    const s = getSchedule(id);
    if (!s) return null;
    s.paused = !s.paused;
    save();
    return s;
  }

  // ---------- reference docs ----------

  function listReferenceDocs() {
    return state.docs.slice();
  }

  function addReferenceDoc(opts) {
    const o = opts || {};
    const doc = {
      id: uid(),
      name: String(o.name || ''),
      size: Number(o.size) || 0,
      type: String(o.type || ''),
      addedAt: Date.now(),
      status: 'parsed (demo)',
    };
    state.docs.push(doc);
    save();
    return doc;
  }

  // ---------- stock (pharmacist demo) ----------

  function stockKey(branchId, medId) { return String(branchId) + '::' + String(medId); }

  function getStock(branchId, medId) {
    const key = stockKey(branchId, medId);
    return Object.prototype.hasOwnProperty.call(state.stock, key) ? state.stock[key] : null;
  }

  function setStock(branchId, medId, qty) {
    const value = Math.max(0, Math.floor(Number(qty)) || 0);
    state.stock[stockKey(branchId, medId)] = value;
    save();
    return value;
  }

  // ---------- reset ----------

  function reset() {
    state = emptyState();
    try {
      if (typeof localStorage !== 'undefined') localStorage.removeItem(KEY);
    } catch (e) {
      // storage blocked: memory is already cleared
    }
  }

  return {
    medFromApi,
    govReference,
    govLookup,
    parseStrength,
    fmtStrength,
    listTickets,
    getTicket,
    createTicket,
    events,
    isWorking,
    bestOffer,
    reserve,
    ticketsForBranch,
    branchThread,
    pharmacyReply,
    pharmacyPhoto,
    listSchedules,
    getSchedule,
    createSchedule,
    nextRun,
    runSchedule,
    togglePause,
    listReferenceDocs,
    addReferenceDoc,
    getStock,
    setStock,
    reset,
  };
})();
