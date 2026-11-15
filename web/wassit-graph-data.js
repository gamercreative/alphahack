/* Wassit knowledge graph data: nodes, links and A-to-Z workflows, derived from
   agent/ (Python tools + runner), agent/prompts/system.md and backend/internal (Go rules).
   Plain script: sets window.WZ_GRAPH_DATA. */
(function () {
  var nodes = [];
  var links = [];
  function n(id, label, group, desc, file, tags) {
    nodes.push({ id: id, label: label, group: group, desc: desc, file: file, tags: tags || [] });
  }
  function l(source, target, kind) { links.push({ source: source, target: target, kind: kind }); }

  /* ---------- core ---------- */
  n('wassit', 'Wassit', 'core',
    'The admin coordinator agent for pharmacists and the DOSYN client desk: finds which nearby participating pharmacy has a medicine in stock and relays replies. It is not a clinician and never decides matches; Go rules do.',
    'agent/prompts/system.md', ['agent', 'coordinator', 'llm']);

  /* ---------- runtime ---------- */
  n('runner', 'Agent runner', 'workflow',
    'Polls GET /api/v1/agent/wakeups, loads enquiry state with next_step, builds the prompt and runs the model (or the scripted policy) with the tool set, tracing every tool call to the backend.',
    'agent/runner.py', ['runtime', 'wakeups', 'tracing']);
  n('scripted-policy', 'Scripted policy', 'workflow',
    'Deterministic fallback policy (decide(state)) that walks the same tools without an LLM: chooses the medication, proposes targets, sends, follows up, records offers and closes.',
    'agent/scripted.py', ['runtime', 'deterministic', 'fallback']);
  n('go-ai-loop', 'Go AI tool loop', 'workflow',
    'Backend tool-use loop: validates the request, maps tool names to handlers, executes each tool call and emits events back to the stream.',
    'backend/internal/ai/loop.go', ['runtime', 'go', 'tool-loop']);
  n('system-prompt', 'System prompt', 'data',
    'Wassit\'s hard rules, inventory shield flags, 6-step workflow and client-ticket rules, with $date and $weekday filled in at run time.',
    'agent/prompts/system.md', ['prompt', 'policy']);
  n('llm-provider', 'Model provider', 'actor',
    'The language model behind Wassit (Anthropic or Ollama, chosen by env) that reads the state and picks tools.',
    'backend/internal/ai/anthropic.go', ['llm', 'anthropic', 'ollama']);

  /* ---------- skills ---------- */
  n('skill-next-step', 'Follow next_step', 'skill',
    'Reads the system-computed next_step in the enquiry state (from _next_step/_after_replies) and does several steps per run when allowed.',
    'agent/tools/enquiry.py', ['planning', 'state-machine']);
  n('skill-intake', 'Medicine identification', 'skill',
    'Normalises the raw request (Arabic, English, French, Arabizi), searches the catalogue and picks the product whose strength and form match the request.',
    'agent/tools/catalogue.py', ['intake', 'search', 'strength']);
  n('skill-clarify', 'Clarification', 'skill',
    'When several products match or the request is unclear, asks the requester one question (strength, form or quantity only) and stops.',
    'agent/tools/enquiry.py', ['clarify', 'ask']);
  n('skill-inventory-shield', 'Inventory shield', 'skill',
    'Computes deterministic policy flags per branch (RARE_PROTECT, CHRONIC_BUFFER, EXCHANGE_ONLY, NEAR_EXPIRY_PUSH) from stock, chronic reservations and expiry.',
    'agent/tools/catalogue.py', ['policy-flags', 'stock', 'barter']);
  n('skill-target-selection', 'Target selection', 'skill',
    'Picks the 3 nearest candidates, preferring has_reported_stock and NEAR_EXPIRY_PUSH surplus while respecting RARE_PROTECT, CHRONIC_BUFFER and EXCHANGE_ONLY.',
    'agent/tools/enquiry.py', ['candidates', 'ranking', 'distance']);
  n('skill-outreach', 'Pharmacy outreach', 'skill',
    'Sends the approved enquiries to target pharmacies through the outbox, then waits for replies.',
    'agent/tools/enquiry.py', ['outreach', 'outbox']);
  n('skill-follow-up', 'Follow-up and silence handling', 'skill',
    'A pharmacy silent after a wait gets one follow-up; still silent after another wait, it is marked no_response. Silence is never "unavailable".',
    'agent/tools/enquiry.py', ['follow-up', 'no-response']);
  n('skill-reply-reading', 'Reply reading and offer extraction', 'skill',
    'Turns a pharmacy\'s free-text reply into a structured offer (in stock, strength, quantity, price_cents, pickup_by, payment, prescription) with exact evidence quotes.',
    'agent/tools/offers.py', ['extraction', 'evidence', 'structured-offer']);
  n('skill-dose-check', 'Dose-equivalence relay', 'skill',
    'Sends an alternative strength to the Go dose rules via record_offer and relays the verdict; Wassit never decides equivalence itself.',
    'backend/internal/services/enq_dose.go', ['dose', 'equivalence', 'relay']);
  n('skill-photo-reading', 'Package photo reading', 'skill',
    'Reads the product code and expiry from a package photo with a vision model and posts the reading; Go compares it to the gov reference.',
    'agent/tools/photos.py', ['vision', 'barcode', 'expiry']);
  n('skill-reference-lookup', 'Gov reference lookup', 'skill',
    'Looks up government reference products by ingredient, strength and form to see what code and listing exist.',
    'agent/tools/reference.py', ['reference', 'gov']);
  n('skill-relay-notify', 'Plain relay to requester', 'skill',
    'Writes one or two plain sentences to the requester (pharmacist or client ticket) quoting the reply or the rules\' reasons; no advice.',
    'agent/tools/enquiry.py', ['notify', 'relay', 'plain-language']);
  n('skill-bilingual', 'Bilingual EN/AR replies', 'skill',
    'Detects the requester\'s language (Arabic, English, French, Arabizi) and replies in it; the client UI renders EN/AR labels.',
    'agent/prompts/system.md', ['arabic', 'english', 'i18n']);
  n('skill-wait-scheduling', 'Wait and wake scheduling', 'skill',
    'Ends a run with wait(minutes) so the backend schedules a wakeup when replies, approvals, photos or the timer arrive.',
    'agent/tools/enquiry.py', ['wait', 'wakeup', 'timer']);
  n('skill-hitl', 'Human-in-the-loop pause', 'skill',
    'Stops after propose_targets without wait(): the requester pharmacist approves the targets before any peer enquiry is sent.',
    'agent/prompts/system.md', ['hitl', 'approval']);
  n('skill-privacy', 'Client privacy', 'skill',
    'On client tickets never asks who the client is and never shares anything about them with a pharmacy; the desk view is redacted.',
    'backend/internal/services/client_tickets.go', ['privacy', 'redaction']);
  n('skill-closing', 'Closing the enquiry', 'skill',
    'Closes with available (pharmacist enquiries only), unavailable (every target replied unavailable) or unable_to_confirm.',
    'agent/tools/enquiry.py', ['close', 'outcome']);

  /* ---------- tools ---------- */
  n('tool-get-enquiry', 'get_enquiry', 'tool',
    'Fetches the enquiry and returns a compact state with targets, offers decisions, photos to read and next_step.',
    'agent/tools/enquiry.py', ['read', 'state']);
  n('tool-search-medications', 'search_medications', 'tool',
    'Searches the catalogue (GET /api/v1/medications, limit 20, retrying per word) and marks matches by strength in mg.',
    'agent/tools/catalogue.py', ['catalogue', 'search']);
  n('tool-evaluate-shield', 'evaluate_inventory_shield', 'tool',
    'Pure helper: RARE_PROTECT when available <= 3 boxes or surplus below the requested qty; NEAR_EXPIRY_PUSH within 180 days of expiry.',
    'agent/tools/catalogue.py', ['policy-flags', 'helper']);
  n('tool-set-medication', 'set_medication', 'tool',
    'Sets the chosen catalogue medication on the enquiry (POST /medication).',
    'agent/tools/enquiry.py', ['write', 'medication']);
  n('tool-ask-requester', 'ask_requester', 'tool',
    'Posts a clarification question to the requester (POST /clarification-request) and ends the run until they answer.',
    'agent/tools/enquiry.py', ['clarify', 'write']);
  n('tool-find-candidates', 'find_candidates', 'tool',
    'Lists nearby participating branches for the medication with distance, has_reported_stock, reported qty and policy flags.',
    'agent/tools/enquiry.py', ['candidates', 'read']);
  n('tool-propose-targets', 'propose_targets', 'tool',
    'Proposes branch ids as targets (POST /targets); they stay pending until the pharmacist approves.',
    'agent/tools/enquiry.py', ['targets', 'hitl']);
  n('tool-send-enquiries', 'send_enquiries', 'tool',
    'Sends the approved targets their enquiry message (POST /send) through the outbox.',
    'agent/tools/enquiry.py', ['send', 'outbox']);
  n('tool-follow-up', 'follow_up', 'tool',
    'Sends one follow-up to a target that has not replied (POST /targets/{id}/follow-up).',
    'agent/tools/enquiry.py', ['follow-up']);
  n('tool-mark-no-response', 'mark_no_response', 'tool',
    'Marks a target that stayed silent after a follow-up as no_response (POST /targets/{id}/no-response).',
    'agent/tools/enquiry.py', ['no-response']);
  n('tool-notify-requester', 'notify_requester', 'tool',
    'Writes a message on the requester\'s enquiry or client ticket (POST /notify); passes the content filter or gets 422 content_filtered.',
    'agent/tools/enquiry.py', ['notify', 'filtered']);
  n('tool-wait', 'wait', 'tool',
    'Schedules the next wake after N minutes (POST /wait); refuses when the state says there is nothing to wait for.',
    'agent/tools/enquiry.py', ['wait', 'schedule']);
  n('tool-close-enquiry', 'close_enquiry', 'tool',
    'Closes the enquiry with an outcome and optional branch_id (POST /close) after re-reading state.',
    'agent/tools/enquiry.py', ['close']);
  n('tool-record-offer', 'record_offer', 'tool',
    'Records a structured offer for a target with evidence quotes (POST /targets/{id}/offers); checks each number appears in its quote and returns the rules\' decision.',
    'agent/tools/offers.py', ['offer', 'evidence', 'decision']);
  n('tool-read-package-photo', 'read_package_photo', 'tool',
    'Fetches the photo file, asks the vision model for product code and expiry, parses the reading and posts it (POST /photos/{id}/agent-reading).',
    'agent/tools/photos.py', ['vision', 'photo']);
  n('tool-check-reference', 'check_reference', 'tool',
    'Queries GET /api/v1/reference/products by ingredient, strength and form.',
    'agent/tools/reference.py', ['reference', 'read']);
  n('tool-http', 'Agent HTTP client', 'tool',
    'Shared call(method, path, body, query) with the agent bearer token; tool_error turns backend errors into tool results.',
    'agent/tools/_http.py', ['http', 'auth']);

  /* ---------- endpoints ---------- */
  n('ep-wakeups', 'GET /agent/wakeups', 'endpoint', 'Lists due wakeups (trigger + enquiry id) for the runner.', 'backend/internal/handlers/enq_agentapi.go', ['agent-api']);
  n('ep-agent-runs', 'POST /agent/runs', 'endpoint', 'Opens an agent run; tool calls are traced via /agent/runs/{id}/tool-calls and finished with PATCH.', 'backend/internal/handlers/enq_agentapi.go', ['agent-api', 'tracing']);
  n('ep-medications', 'GET /medications', 'endpoint', 'Catalogue search by q with limit.', 'backend/internal/handlers/pharmacy.go', ['catalogue']);
  n('ep-enquiry-get', 'GET /enquiries/{id}', 'endpoint', 'Enquiry detail with targets, messages and photos.', 'backend/internal/handlers/enq_enquiries.go', ['enquiry']);
  n('ep-medication-set', 'POST /enquiries/{id}/medication', 'endpoint', 'Sets the medication_id on the enquiry.', 'backend/internal/handlers/enq_enquiries.go', ['enquiry']);
  n('ep-clarification', 'POST /enquiries/{id}/clarification-request', 'endpoint', 'Stores Wassit\'s question; the requester answers via POST /clarify, which wakes the agent.', 'backend/internal/handlers/enq_enquiries.go', ['clarify']);
  n('ep-candidates', 'GET /enquiries/{id}/candidates', 'endpoint', 'Nearby participating branches for the enquiry\'s medication.', 'backend/internal/handlers/enq_enquiries.go', ['candidates']);
  n('ep-targets', 'POST /enquiries/{id}/targets', 'endpoint', 'Stores proposed target branches.', 'backend/internal/handlers/enq_enquiries.go', ['targets']);
  n('ep-targets-approve', 'POST /enquiries/{id}/targets/approve', 'endpoint', 'Pharmacist approves the proposed targets (HITL); this wakes Wassit.', 'backend/internal/handlers/enq_enquiries.go', ['hitl', 'approval']);
  n('ep-send', 'POST /enquiries/{id}/send', 'endpoint', 'Queues outbound enquiry messages to approved targets.', 'backend/internal/handlers/enq_outbox.go', ['outbox']);
  n('ep-follow-up', 'POST /targets/{id}/follow-up', 'endpoint', 'Queues one follow-up message to a silent target.', 'backend/internal/handlers/enq_outbox.go', ['follow-up']);
  n('ep-no-response', 'POST /targets/{id}/no-response', 'endpoint', 'Sets the target status to no_response.', 'backend/internal/handlers/enq_replies.go', ['no-response']);
  n('ep-reply', 'POST /targets/{id}/reply', 'endpoint', 'A target pharmacist replies (available, alternative, unavailable or text); wakes Wassit.', 'backend/internal/handlers/enq_replies.go', ['reply']);
  n('ep-notify', 'POST /enquiries/{id}/notify', 'endpoint', 'Writes Wassit\'s message to the requester after the content filter.', 'backend/internal/handlers/enq_enquiries.go', ['notify', 'filter']);
  n('ep-wait', 'POST /enquiries/{id}/wait', 'endpoint', 'Schedules a timer wakeup after N minutes.', 'backend/internal/handlers/enq_enquiries.go', ['wakeup']);
  n('ep-close', 'POST /enquiries/{id}/close', 'endpoint', 'Closes the enquiry with an outcome (available / unavailable / unable_to_confirm).', 'backend/internal/handlers/enq_enquiries.go', ['close']);
  n('ep-offers-post', 'POST /targets/{id}/offers', 'endpoint', 'Records a structured offer; Go EvaluateOffer returns matches / escalated / rejected with reasons and a dose check.', 'backend/internal/handlers/enq_offers.go', ['offer', 'rules']);
  n('ep-offers-list', 'GET /enquiries/{id}/offers', 'endpoint', 'Lists recorded offers and their decisions.', 'backend/internal/handlers/enq_offers.go', ['offer']);
  n('ep-photos-upload', 'POST /targets/{id}/photos', 'endpoint', 'Target pharmacist uploads a package photo with typed product code and expiry; wakes Wassit with a photo trigger.', 'backend/internal/handlers/photos.go', ['photo', 'upload']);
  n('ep-photo-reading', 'POST /photos/{id}/agent-reading', 'endpoint', 'Stores Wassit\'s vision reading of the photo; Go re-decides the photo status.', 'backend/internal/handlers/photos.go', ['photo', 'agent']);
  n('ep-photo-review', 'POST /photos/{id}/review', 'endpoint', 'Human reviewer verifies or rejects a flagged photo.', 'backend/internal/handlers/photos.go', ['photo', 'human']);
  n('ep-reference', 'GET /reference/products', 'endpoint', 'Government reference products filtered by ingredient, strength and form.', 'backend/internal/handlers/reference.go', ['reference']);
  n('ep-client-tickets', 'POST /client/tickets', 'endpoint', 'A DOSYN client opens a one-time ticket for a medicine; creates a client_desk enquiry.', 'backend/internal/handlers/client.go', ['client', 'ticket']);
  n('ep-ticket-stream', 'GET /client/tickets/{id}/stream', 'endpoint', 'Streams the redacted ticket events (outreach, pharmacy_reply, offer, dose_check, best_offer...) to the client UI.', 'backend/internal/handlers/client.go', ['client', 'sse']);
  n('ep-reserve', 'POST /client/tickets/{id}/reserve', 'endpoint', 'The client reserves the best verified offer.', 'backend/internal/handlers/client.go', ['client', 'reserve']);
  n('ep-schedules', 'POST /client/schedules', 'endpoint', 'Client creates a refill schedule (every week, 2weeks or month on a weekday).', 'backend/internal/handlers/client.go', ['schedule']);
  n('ep-schedule-run', 'POST /client/schedules/{id}/run', 'endpoint', 'Runs a schedule now; normally fires the next weekday 09:00 Beirut, creating a ticket with a scheduled trigger.', 'backend/internal/handlers/client.go', ['schedule', 'trigger']);

  /* ---------- rules ---------- */
  n('rule-no-substitute', 'Never suggest or swap medicine', 'rule', 'Wassit never diagnoses, prescribes, recommends or suggests a substitute; an alternative is only a match if Go accepts it.', 'agent/prompts/system.md', ['safety']);
  n('rule-content-filter', 'Content filter', 'rule', 'Blocks "I recommend", "instead", "switch to" and any medicine name other than the requested or offered one; returns 422 content_filtered.', 'backend/internal/services/content_filter.go', ['filter', '422']);
  n('rule-tool-data-only', 'Tool data only', 'rule', 'Never invent medicines, pharmacies or stock; use only data from the tools.', 'agent/prompts/system.md', ['grounding']);
  n('rule-silence', 'Silence is not unavailable', 'rule', 'No reply is never "unavailable"; silence becomes no_response.', 'agent/prompts/system.md', ['no-response']);
  n('rule-hitl', 'HITL before outbound pings', 'rule', 'Always pause for the pharmacist\'s confirmation before committing stock or sending peer enquiries.', 'agent/prompts/system.md', ['hitl']);
  n('rule-identity-hidden', 'Client identity hidden', 'rule', 'Client events are redacted: actor_id becomes "client" and client payload keys are dropped for pharmacies.', 'backend/internal/services/client_tickets.go', ['privacy', 'redact']);
  n('rule-dose-equivalence', 'Dose equivalence (4 checks)', 'rule', 'Same active ingredient, same form, total strength matches exactly (whole units per dose), listed in the gov reference. All must pass.', 'backend/internal/services/enq_dose.go', ['contract-rule-1', 'dose']);
  n('rule-offer-evaluation', 'Offer evaluation', 'rule', 'EvaluateOffer validates fields, checks numbers are shown in the evidence quotes and decides matches, escalated or rejected with reasons; decided_by "rules".', 'backend/internal/services/enq_offers.go', ['offer', 'decision']);
  n('rule-photo-evidence', 'Photo is evidence, not proof', 'rule', 'Product code must equal the gov reference code of the offered product and expiry must not be before the pickup month; anything unclear is flagged for a human.', 'backend/internal/services/photos.go', ['contract-rule-10', 'photo']);
  n('rule-best-offer', 'Best offer ranking', 'rule', 'Only verified offers win (rules matched, photo verified, has a total, no decline/silence/dose rejection); lowest total, then latest expiry, then lower branch id.', 'backend/internal/services/client_best.go', ['contract-rule-11', 'ranking']);
  n('rule-no-close-available-client', 'Client reserves, agent never closes available', 'rule', 'On a client ticket Wassit never closes "available"; it notifies in plain words and waits for the client to reserve.', 'agent/prompts/system.md', ['client']);
  n('rule-end-run', 'Every run ends cleanly', 'rule', 'Each run must end with wait(), close_enquiry(), or ask_requester() when an answer is needed.', 'agent/prompts/system.md', ['run-contract']);
  n('rule-language', 'Reply in requester\'s language', 'rule', 'Requests may be Arabic, English, French or Arabizi; replies match the requester.', 'agent/prompts/system.md', ['i18n']);
  n('rule-rare-protect', 'RARE_PROTECT', 'rule', 'Available stock <= 3 boxes (or surplus below the requested qty): protect the last boxes.', 'agent/tools/catalogue.py', ['policy-flag']);
  n('rule-chronic-buffer', 'CHRONIC_BUFFER', 'rule', 'Boxes reserved for active chronic refill commitments come before any surplus shown to peers.', 'agent/tools/catalogue.py', ['policy-flag']);
  n('rule-exchange-only', 'EXCHANGE_ONLY', 'rule', 'Scarce items are bartered with peers rather than sold outright.', 'agent/tools/catalogue.py', ['policy-flag']);
  n('rule-near-expiry', 'NEAR_EXPIRY_PUSH', 'rule', 'Batches within 180 days of expiry or beyond the ~4-package importer return cap; prefer pinging branches with this surplus.', 'agent/tools/catalogue.py', ['policy-flag']);

  /* ---------- actors ---------- */
  n('actor-client', 'Client', 'actor', 'A DOSYN client-desk user who opens one-time tickets or refill schedules and reserves the best offer. Never identified to pharmacies.', 'backend/internal/handlers/client.go', ['client']);
  n('actor-pharmacist', 'Requesting pharmacist', 'actor', 'The pharmacist who opens an enquiry, answers clarifications and approves targets (HITL).', 'backend/internal/handlers/enq_enquiries.go', ['pharmacist', 'hitl']);
  n('actor-peer-pharmacy', 'Peer pharmacy', 'actor', 'A participating target branch that replies (available, alternative, unavailable), uploads package photos, or stays silent.', 'backend/internal/handlers/enq_replies.go', ['pharmacy', 'target']);
  n('actor-human-reviewer', 'Human reviewer', 'actor', 'Reviews flagged package photos and verifies or rejects them.', 'backend/internal/handlers/photos.go', ['human', 'review']);
  n('actor-vision-model', 'Vision model', 'actor', 'Reads product code and expiry text from a package photo for read_package_photo.', 'agent/tools/photos.py', ['vision']);

  /* ---------- data ---------- */
  n('data-gov-reference', 'Gov reference list', 'data', 'Government reference products (ingredient, strength, form, code) used by dose and photo checks.', 'backend/internal/handlers/reference.go', ['reference']);
  n('data-catalogue', 'Medication catalogue', 'data', 'Catalogue of medications with name, strength and form.', 'backend/internal/handlers/pharmacy.go', ['catalogue']);
  n('data-enquiries', 'Enquiries', 'data', 'Enquiry rows: raw request, medication, status, outcome, client_desk flag.', 'backend/internal/services/enq_enquiries.go', ['enquiry']);
  n('data-targets', 'Targets and messages', 'data', 'Target branches per enquiry with status (pending, sent, replied, no_response) and outbound/inbound messages.', 'backend/internal/services/enq_outbox.go', ['targets', 'outbox']);
  n('data-offers', 'Target offers', 'data', 'Structured offers with evidence, decision, reasons and dose check.', 'backend/internal/services/enq_offers.go', ['offers']);
  n('data-photos', 'Package photos', 'data', 'Uploaded photos with typed code/expiry, agent reading, status (verified, flagged, mismatch) and decided_by.', 'backend/internal/services/photos.go', ['photos']);
  n('data-tickets', 'Client tickets', 'data', 'Client-desk enquiries and their events (client_request, outreach, pharmacy_reply, declined, no_answer, offer, dose_check, photo_check, best_offer, reserved).', 'backend/internal/services/client_tickets.go', ['tickets', 'events']);
  n('data-schedules', 'Refill schedules', 'data', 'Client schedules: medicine, every (week/2weeks/month), weekday, next run at 09:00 Beirut.', 'backend/internal/services/client_schedules.go', ['schedules']);
  n('data-inventory', 'Branch inventory', 'data', 'Per-branch stock quantities, chronic reservations and batch expiry.', 'backend/internal/services/stock.go', ['stock']);
  n('data-wakeups', 'Wakeups and agent runs', 'data', 'Pending wakeups (trigger: new, clarified, approved, reply, timer, photo, scheduled) and traced agent runs.', 'backend/internal/services/enq_agentapi.go', ['wakeups', 'runs']);

  /* ---------- links ---------- */
  // core <-> runtime
  l('runner', 'wassit', 'calls'); l('runner', 'ep-wakeups', 'calls'); l('runner', 'ep-agent-runs', 'writes');
  l('runner', 'scripted-policy', 'uses'); l('runner', 'system-prompt', 'reads'); l('runner', 'llm-provider', 'calls');
  l('wassit', 'system-prompt', 'reads'); l('wassit', 'llm-provider', 'uses'); l('go-ai-loop', 'llm-provider', 'calls');
  l('go-ai-loop', 'wassit', 'flows'); l('scripted-policy', 'skill-next-step', 'uses');
  l('ep-wakeups', 'data-wakeups', 'reads'); l('ep-agent-runs', 'data-wakeups', 'writes');
  // wassit -> skills
  ['skill-next-step', 'skill-intake', 'skill-clarify', 'skill-inventory-shield', 'skill-target-selection', 'skill-outreach',
   'skill-follow-up', 'skill-reply-reading', 'skill-dose-check', 'skill-photo-reading', 'skill-reference-lookup',
   'skill-relay-notify', 'skill-bilingual', 'skill-wait-scheduling', 'skill-hitl', 'skill-privacy', 'skill-closing']
    .forEach(function (s) { l('wassit', s, 'uses'); });
  // skills -> tools
  l('skill-next-step', 'tool-get-enquiry', 'uses');
  l('skill-intake', 'tool-search-medications', 'uses'); l('skill-intake', 'tool-set-medication', 'uses');
  l('skill-clarify', 'tool-ask-requester', 'uses');
  l('skill-inventory-shield', 'tool-evaluate-shield', 'uses');
  l('skill-target-selection', 'tool-find-candidates', 'uses'); l('skill-target-selection', 'tool-propose-targets', 'uses');
  l('skill-outreach', 'tool-send-enquiries', 'uses');
  l('skill-follow-up', 'tool-follow-up', 'uses'); l('skill-follow-up', 'tool-mark-no-response', 'uses');
  l('skill-reply-reading', 'tool-record-offer', 'uses');
  l('skill-dose-check', 'tool-record-offer', 'uses'); l('skill-dose-check', 'tool-check-reference', 'uses');
  l('skill-photo-reading', 'tool-read-package-photo', 'uses');
  l('skill-reference-lookup', 'tool-check-reference', 'uses');
  l('skill-relay-notify', 'tool-notify-requester', 'uses');
  l('skill-wait-scheduling', 'tool-wait', 'uses');
  l('skill-hitl', 'tool-propose-targets', 'uses');
  l('skill-closing', 'tool-close-enquiry', 'uses');
  l('skill-bilingual', 'tool-notify-requester', 'uses'); l('skill-bilingual', 'tool-ask-requester', 'uses');
  l('skill-privacy', 'tool-notify-requester', 'uses');
  l('tool-find-candidates', 'tool-evaluate-shield', 'uses');
  // tools -> http -> endpoints
  ['tool-get-enquiry', 'tool-search-medications', 'tool-set-medication', 'tool-ask-requester', 'tool-find-candidates',
   'tool-propose-targets', 'tool-send-enquiries', 'tool-follow-up', 'tool-mark-no-response', 'tool-notify-requester',
   'tool-wait', 'tool-close-enquiry', 'tool-record-offer', 'tool-read-package-photo', 'tool-check-reference']
    .forEach(function (t) { l(t, 'tool-http', 'uses'); });
  l('tool-get-enquiry', 'ep-enquiry-get', 'calls'); l('tool-get-enquiry', 'ep-offers-list', 'calls');
  l('tool-search-medications', 'ep-medications', 'calls');
  l('tool-set-medication', 'ep-medication-set', 'calls');
  l('tool-ask-requester', 'ep-clarification', 'calls');
  l('tool-find-candidates', 'ep-candidates', 'calls');
  l('tool-propose-targets', 'ep-targets', 'calls');
  l('tool-send-enquiries', 'ep-send', 'calls');
  l('tool-follow-up', 'ep-follow-up', 'calls');
  l('tool-mark-no-response', 'ep-no-response', 'calls');
  l('tool-notify-requester', 'ep-notify', 'calls');
  l('tool-wait', 'ep-wait', 'calls'); l('tool-wait', 'ep-enquiry-get', 'reads');
  l('tool-close-enquiry', 'ep-close', 'calls'); l('tool-close-enquiry', 'ep-enquiry-get', 'reads');
  l('tool-record-offer', 'ep-offers-post', 'calls');
  l('tool-read-package-photo', 'ep-photo-reading', 'calls'); l('tool-read-package-photo', 'actor-vision-model', 'calls');
  l('tool-check-reference', 'ep-reference', 'calls');
  l('tool-http', 'ep-agent-runs', 'writes');
  // endpoints -> data
  l('ep-medications', 'data-catalogue', 'reads');
  l('ep-enquiry-get', 'data-enquiries', 'reads'); l('ep-enquiry-get', 'data-targets', 'reads'); l('ep-enquiry-get', 'data-photos', 'reads');
  l('ep-medication-set', 'data-enquiries', 'writes');
  l('ep-clarification', 'data-enquiries', 'writes'); l('ep-clarification', 'data-wakeups', 'writes');
  l('ep-candidates', 'data-inventory', 'reads');
  l('ep-targets', 'data-targets', 'writes');
  l('ep-targets-approve', 'data-targets', 'writes'); l('ep-targets-approve', 'data-wakeups', 'writes');
  l('ep-send', 'data-targets', 'writes'); l('ep-follow-up', 'data-targets', 'writes'); l('ep-no-response', 'data-targets', 'writes');
  l('ep-reply', 'data-targets', 'writes'); l('ep-reply', 'data-wakeups', 'writes');
  l('ep-notify', 'data-enquiries', 'writes'); l('ep-notify', 'data-tickets', 'writes');
  l('ep-wait', 'data-wakeups', 'writes'); l('ep-close', 'data-enquiries', 'writes');
  l('ep-offers-post', 'data-offers', 'writes'); l('ep-offers-list', 'data-offers', 'reads');
  l('ep-offers-post', 'data-gov-reference', 'reads');
  l('ep-photos-upload', 'data-photos', 'writes'); l('ep-photos-upload', 'data-wakeups', 'writes');
  l('ep-photo-reading', 'data-photos', 'writes'); l('ep-photo-review', 'data-photos', 'writes');
  l('ep-reference', 'data-gov-reference', 'reads');
  l('ep-client-tickets', 'data-tickets', 'writes'); l('ep-client-tickets', 'data-enquiries', 'writes'); l('ep-client-tickets', 'data-wakeups', 'writes');
  l('ep-ticket-stream', 'data-tickets', 'reads');
  l('ep-reserve', 'data-tickets', 'writes'); l('ep-reserve', 'data-offers', 'reads');
  l('ep-schedules', 'data-schedules', 'writes'); l('ep-schedule-run', 'data-schedules', 'reads'); l('ep-schedule-run', 'data-tickets', 'writes');
  // rules -> where they decide
  l('rule-no-substitute', 'wassit', 'decides'); l('rule-no-substitute', 'rule-content-filter', 'uses');
  l('rule-content-filter', 'ep-notify', 'decides'); l('rule-content-filter', 'ep-clarification', 'decides');
  l('rule-tool-data-only', 'wassit', 'decides'); l('rule-silence', 'skill-follow-up', 'decides');
  l('rule-hitl', 'skill-hitl', 'decides'); l('rule-hitl', 'ep-targets-approve', 'decides');
  l('rule-identity-hidden', 'skill-privacy', 'decides'); l('rule-identity-hidden', 'ep-ticket-stream', 'decides'); l('rule-identity-hidden', 'ep-send', 'decides');
  l('rule-dose-equivalence', 'ep-offers-post', 'decides'); l('rule-dose-equivalence', 'data-gov-reference', 'reads');
  l('rule-offer-evaluation', 'ep-offers-post', 'decides'); l('rule-offer-evaluation', 'rule-dose-equivalence', 'uses');
  l('rule-photo-evidence', 'ep-photos-upload', 'decides'); l('rule-photo-evidence', 'ep-photo-reading', 'decides'); l('rule-photo-evidence', 'data-gov-reference', 'reads');
  l('rule-best-offer', 'data-offers', 'reads'); l('rule-best-offer', 'data-photos', 'reads'); l('rule-best-offer', 'data-tickets', 'writes'); l('rule-best-offer', 'ep-reserve', 'decides');
  l('rule-no-close-available-client', 'skill-closing', 'decides'); l('rule-end-run', 'wassit', 'decides');
  l('rule-language', 'skill-bilingual', 'decides');
  ['rule-rare-protect', 'rule-chronic-buffer', 'rule-exchange-only', 'rule-near-expiry'].forEach(function (r) {
    l('tool-evaluate-shield', r, 'decides'); l(r, 'skill-target-selection', 'decides');
  });
  l('tool-evaluate-shield', 'data-inventory', 'reads');
  // actors
  l('actor-client', 'ep-client-tickets', 'calls'); l('actor-client', 'ep-schedules', 'calls'); l('actor-client', 'ep-reserve', 'calls'); l('actor-client', 'ep-ticket-stream', 'reads');
  l('actor-pharmacist', 'ep-targets-approve', 'calls'); l('actor-pharmacist', 'data-enquiries', 'writes');
  l('actor-peer-pharmacy', 'ep-reply', 'calls'); l('actor-peer-pharmacy', 'ep-photos-upload', 'calls');
  l('actor-human-reviewer', 'ep-photo-review', 'calls');
  l('ep-send', 'actor-peer-pharmacy', 'flows'); l('ep-follow-up', 'actor-peer-pharmacy', 'flows');
  l('ep-notify', 'actor-pharmacist', 'flows'); l('ep-notify', 'actor-client', 'flows');
  l('ep-clarification', 'actor-pharmacist', 'flows'); l('ep-clarification', 'actor-client', 'flows');
  l('rule-photo-evidence', 'actor-human-reviewer', 'flows');
  l('data-wakeups', 'runner', 'flows');

  /* ---------- workflows ---------- */
  var workflows = [
    { id: 'client-one-time-ticket', name: 'Client one-time ticket, end to end',
      summary: 'A client asks for Concor 5mg; Wassit finds stock, the rules verify offers and the client reserves the best one.',
      steps: [
        { node: 'actor-client', label: 'Client opens a ticket', detail: 'Client types "بدي كونكور 5" (I need Concor 5) for 1 box on the DOSYN desk. Event: client_request.', decider: 'client' },
        { node: 'ep-client-tickets', label: 'Ticket created', detail: 'POST /api/v1/client/tickets creates a client_desk enquiry and a "new" wakeup.', decider: 'rules' },
        { node: 'runner', label: 'Runner wakes Wassit', detail: 'GET /agent/wakeups returns the ticket; state is loaded with next_step "identify medicine".', decider: 'rules' },
        { node: 'tool-search-medications', label: 'Find the product', detail: 'search_medications("concor 5") returns Concor 5mg tablet x30; strength 5mg matches the request.', decider: 'wassit' },
        { node: 'tool-set-medication', label: 'Set medication', detail: 'set_medication(med_concor5) stores the exact strength and form.', decider: 'wassit' },
        { node: 'tool-find-candidates', label: 'Rank nearby branches', detail: 'find_candidates returns Hamra (0.8 km, reported 6), Achrafieh (1.4 km, NEAR_EXPIRY_PUSH), Verdun (2.1 km, RARE_PROTECT: 2 boxes).', decider: 'wassit' },
        { node: 'tool-propose-targets', label: 'Propose 3 targets', detail: 'propose_targets([hamra, achrafieh, verdun]); on a client ticket the desk side approves before sending.', decider: 'human' },
        { node: 'tool-send-enquiries', label: 'Outreach', detail: 'send_enquiries sends "Do you have Concor 5mg, 1 box?" with no client details. Event: outreach.', decider: 'wassit' },
        { node: 'actor-peer-pharmacy', label: 'Pharmacies reply', detail: 'Hamra: "yes 3 boxes, 450,000 LBP, pickup today". Achrafieh: "available, 420000, exp 03/2027". Event: pharmacy_reply.', decider: 'pharmacy' },
        { node: 'tool-record-offer', label: 'Record structured offers', detail: 'record_offer(hamra, in_stock=true, quantity=3, price_cents=45000000, evidence {price:"450,000 LBP"}). Rules: matches. Event: offer.', decider: 'rules' },
        { node: 'rule-best-offer', label: 'Best offer computed', detail: 'Verified offers only: Achrafieh (420,000 LBP) beats Hamra on lowest total. Event: best_offer.', decider: 'rules' },
        { node: 'tool-notify-requester', label: 'Plain notice to client', detail: '"A pharmacy near you has Concor 5mg for 420,000 LBP. You can reserve it on this ticket." Then wait().', decider: 'wassit' },
        { node: 'ep-reserve', label: 'Client reserves', detail: 'POST /client/tickets/{id}/reserve on the best offer. Event: reserved. Wassit never closes "available" itself.', decider: 'client' }
      ] },
    { id: 'scheduled-refill-cycle', name: 'Scheduled refill cycle',
      summary: 'A monthly schedule fires on its weekday at 09:00 Beirut and runs a fresh ticket through Wassit.',
      steps: [
        { node: 'ep-schedules', label: 'Client sets a schedule', detail: 'POST /client/schedules {medicine: Glucophage 850mg, every: "month", weekday: 1}.', decider: 'client' },
        { node: 'data-schedules', label: 'Next run computed', detail: 'nextWeekdayAt returns the first Monday 09:00 Beirut strictly after now.', decider: 'rules' },
        { node: 'ep-schedule-run', label: 'Schedule fires', detail: 'At Monday 09:00 (or POST /run now) a ticket is created with trigger "scheduled".', decider: 'rules' },
        { node: 'data-wakeups', label: 'Wakeup queued', detail: 'A scheduled wakeup points at the new client_desk enquiry.', decider: 'rules' },
        { node: 'skill-next-step', label: 'Read state', detail: 'get_enquiry shows the medication already set from the schedule; next_step "find candidates".', decider: 'wassit' },
        { node: 'skill-target-selection', label: 'Choose targets', detail: 'Skips a branch flagged CHRONIC_BUFFER (boxes reserved for chronic refills); picks 3 nearest with reported stock.', decider: 'wassit' },
        { node: 'skill-outreach', label: 'Send and wait', detail: 'send_enquiries, then wait(30).', decider: 'wassit' },
        { node: 'skill-reply-reading', label: 'Read replies', detail: '"Glucophage 850 available, 2 boxes, 180,000 each" becomes price_cents 18000000, quantity 2.', decider: 'wassit' },
        { node: 'rule-best-offer', label: 'Rank verified offers', detail: 'Lowest total, then latest expiry, then lower branch id.', decider: 'rules' },
        { node: 'tool-notify-requester', label: 'Notify client', detail: '"Your monthly Glucophage 850mg is available nearby. Reserve it on your ticket."', decider: 'wassit' },
        { node: 'tool-wait', label: 'Wait for reservation', detail: 'wait(120) until the client reserves; the schedule rolls to next month.', decider: 'wassit' }
      ] },
    { id: 'dose-equivalent-accepted', name: 'Dose-equivalent offer accepted',
      summary: 'A pharmacy offers 2 x Concor 2.5mg for a Concor 5mg request; all four Go checks pass.',
      steps: [
        { node: 'actor-peer-pharmacy', label: 'Alternative strength offered', detail: 'Reply: "No 5mg, but we have Concor 2.5mg, 2 boxes, 230,000 LBP each."', decider: 'pharmacy' },
        { node: 'tool-get-enquiry', label: 'State shows a reply needing an offer', detail: 'next_step: record_offer for target T-2 before deciding.', decider: 'rules' },
        { node: 'skill-reply-reading', label: 'Extract fields', detail: 'strength "2.5mg", quantity 2, price_cents 23000000, evidence quoted exactly from the reply.', decider: 'wassit' },
        { node: 'tool-record-offer', label: 'Submit to rules', detail: 'POST /targets/T-2/offers; record_offer checks every number appears in its quote.', decider: 'wassit' },
        { node: 'rule-dose-equivalence', label: 'Four checks', detail: 'Same active ingredient (bisoprolol) PASS; same form (tablet) PASS; total strength 2 x 2.5mg = 5mg PASS; listed in gov reference PASS.', decider: 'rules' },
        { node: 'data-gov-reference', label: 'Gov lookup', detail: 'Bisoprolol 2.5mg tablet found in the government list.', decider: 'rules' },
        { node: 'rule-offer-evaluation', label: 'Decision: matches', detail: 'decided_by "rules"; dose_check attached to the offer. Event: dose_check.', decider: 'rules' },
        { node: 'tool-notify-requester', label: 'Relay the verdict', detail: '"A pharmacy has Concor 2.5mg; the rules confirm 2 tablets equal your 5mg dose." No advice added.', decider: 'wassit' },
        { node: 'rule-content-filter', label: 'Filter passes', detail: 'Only the requested and offered names appear; no "instead" or "switch to".', decider: 'rules' }
      ] },
    { id: 'dose-equivalent-rejected', name: 'Dose-equivalent offer rejected',
      summary: 'A different form is offered; the rules reject it and Wassit only relays.',
      steps: [
        { node: 'actor-peer-pharmacy', label: 'Different form offered', detail: 'Reply: "We have Augmentin 1g sachets, not tablets, 1 box 310,000."', decider: 'pharmacy' },
        { node: 'skill-reply-reading', label: 'Extract fields', detail: 'strength "1g" (1000mg), form sachet, price_cents 31000000.', decider: 'wassit' },
        { node: 'tool-record-offer', label: 'Submit to rules', detail: 'record_offer with evidence {strength: "1g sachets"}.', decider: 'wassit' },
        { node: 'rule-dose-equivalence', label: 'Form check fails', detail: 'Same active ingredient PASS; Same form FAIL (sachet vs tablet); total strength n/a; gov listed checked.', decider: 'rules' },
        { node: 'rule-offer-evaluation', label: 'Decision: rejected', detail: 'Reasons: ["offered form differs from requested"]. It is not a match.', decider: 'rules' },
        { node: 'rule-no-substitute', label: 'No swap suggested', detail: 'Wassit does not suggest taking the sachets; it never decides equivalence.', decider: 'rules' },
        { node: 'tool-notify-requester', label: 'Plain relay', detail: '"One pharmacy has Augmentin 1g sachets, which the rules did not accept as the same as your tablets."', decider: 'wassit' },
        { node: 'rule-content-filter', label: 'Filter check', detail: 'A draft with "I recommend" would return 422 content_filtered and be rewritten as a relay.', decider: 'rules' },
        { node: 'tool-wait', label: 'Keep waiting for others', detail: 'wait(30) for the remaining targets.', decider: 'wassit' }
      ] },
    { id: 'photo-flagged-to-human', name: 'Package photo flagged to a human',
      summary: 'A pharmacy uploads a package photo; the code does not match and a human reviewer decides.',
      steps: [
        { node: 'actor-peer-pharmacy', label: 'Photo uploaded', detail: 'Achrafieh uploads a box photo and types code 6251234000017, expiry 2026-12.', decider: 'pharmacy' },
        { node: 'ep-photos-upload', label: 'Photo stored, wakeup', detail: 'POST /targets/{id}/photos stores it and wakes Wassit with trigger "photo".', decider: 'rules' },
        { node: 'tool-read-package-photo', label: 'Read the photo first', detail: 'read_package_photo(photo_id) fetches the file.', decider: 'wassit' },
        { node: 'actor-vision-model', label: 'Vision reading', detail: 'Model reads code 6251234000024 and expiry 11/2026 from the box.', decider: 'wassit' },
        { node: 'ep-photo-reading', label: 'Post agent reading', detail: 'POST /photos/{id}/agent-reading {product_code, expiry}.', decider: 'wassit' },
        { node: 'rule-photo-evidence', label: 'Go compares', detail: 'Code differs from the gov reference code of the offered product; expiry 11/2026 is before the pickup month 12/2026. Status: flagged. Event: photo_check.', decider: 'rules' },
        { node: 'data-gov-reference', label: 'Reference code', detail: 'Gov list says Concor 5mg x30 is 6251234000017.', decider: 'rules' },
        { node: 'actor-human-reviewer', label: 'Human reviews', detail: 'Reviewer opens the flagged photo.', decider: 'human' },
        { node: 'ep-photo-review', label: 'Reject or verify', detail: 'POST /photos/{id}/review {status: "mismatch"}; decided_by human.', decider: 'human' },
        { node: 'rule-best-offer', label: 'Excluded from ranking', detail: 'The Achrafieh offer cannot win without a verified photo; Hamra becomes best_offer.', decider: 'rules' },
        { node: 'tool-notify-requester', label: 'Notify', detail: '"One pharmacy\'s box could not be confirmed, so the other offer is shown."', decider: 'wassit' }
      ] },
    { id: 'pharmacist-peer-enquiry-hitl', name: 'Pharmacist peer enquiry with HITL approval',
      summary: 'A pharmacist asks peers for stock; Wassit proposes targets and waits for human approval before any ping.',
      steps: [
        { node: 'actor-pharmacist', label: 'Pharmacist asks', detail: '"Need Euthyrox 50 mcg x2 for a patient today."', decider: 'human' },
        { node: 'tool-search-medications', label: 'Search', detail: 'Two results: Euthyrox 50mcg x30 and x100.', decider: 'wassit' },
        { node: 'tool-ask-requester', label: 'Clarify pack', detail: '"Which pack: 30 or 100 tablets?" Run stops.', decider: 'wassit' },
        { node: 'actor-pharmacist', label: 'Answer', detail: 'Pharmacist replies "30" via POST /clarify; wakes Wassit.', decider: 'human' },
        { node: 'tool-set-medication', label: 'Set medication', detail: 'set_medication(euthyrox_50_30).', decider: 'wassit' },
        { node: 'tool-find-candidates', label: 'Candidates with flags', detail: 'Branch B12 EXCHANGE_ONLY, B07 NEAR_EXPIRY_PUSH (exp in 120 days), B03 RARE_PROTECT.', decider: 'rules' },
        { node: 'skill-target-selection', label: 'Pick 3, cite flags', detail: 'Prefers B07 (NEAR_EXPIRY_PUSH surplus), avoids depleting B03\'s last boxes.', decider: 'wassit' },
        { node: 'tool-propose-targets', label: 'Propose and stop', detail: 'propose_targets([B07, B12, B05]); no wait(): HITL pause.', decider: 'wassit' },
        { node: 'ep-targets-approve', label: 'Pharmacist approves', detail: 'POST /targets/approve; this wakes Wassit with trigger "approved".', decider: 'human' },
        { node: 'tool-send-enquiries', label: 'Send', detail: 'send_enquiries, then wait(20).', decider: 'wassit' },
        { node: 'tool-record-offer', label: 'Available reply', detail: 'B07: "yes 2 boxes, 95,000 each" recorded; rules: matches.', decider: 'rules' },
        { node: 'tool-close-enquiry', label: 'Close available', detail: 'close_enquiry("available", branch_id B07) (pharmacist enquiry, not a client ticket).', decider: 'wassit' }
      ] },
    { id: 'no-answer-and-decline', name: 'No-answer and decline handling',
      summary: 'One pharmacy declines, one stays silent through a follow-up; Wassit closes unable_to_confirm.',
      steps: [
        { node: 'tool-send-enquiries', label: 'Enquiries sent', detail: '3 targets for Ventolin inhaler 100mcg.', decider: 'wassit' },
        { node: 'tool-wait', label: 'First wait', detail: 'wait(30).', decider: 'wassit' },
        { node: 'actor-peer-pharmacy', label: 'One declines', detail: 'Verdun replies "unavailable". Event: declined.', decider: 'pharmacy' },
        { node: 'skill-follow-up', label: 'Silent target', detail: 'Hamra has not replied after the wait: next_step follow_up.', decider: 'rules' },
        { node: 'tool-follow-up', label: 'One follow-up', detail: 'follow_up(T-hamra) sends a single reminder.', decider: 'wassit' },
        { node: 'tool-wait', label: 'Second wait', detail: 'wait(30) again.', decider: 'wassit' },
        { node: 'rule-silence', label: 'Silence is not unavailable', detail: 'Still silent: it must be no_response, never "unavailable".', decider: 'rules' },
        { node: 'tool-mark-no-response', label: 'Mark no_response', detail: 'mark_no_response(T-hamra). Event: no_answer.', decider: 'wassit' },
        { node: 'actor-peer-pharmacy', label: 'Third offers alternative', detail: 'Achrafieh: "only the generic Asthalin". Escalated, not a match; relayed with notify_requester.', decider: 'pharmacy' },
        { node: 'tool-close-enquiry', label: 'Close unable_to_confirm', detail: 'Every target answered or is no_response, none available, not all unavailable: close_enquiry("unable_to_confirm").', decider: 'wassit' }
      ] },
    { id: 'reservation', name: 'Reservation of the best offer',
      summary: 'Client sees the ranked best verified offer on the ticket stream and reserves it.',
      steps: [
        { node: 'data-offers', label: 'Verified offers exist', detail: 'Hamra 450,000 LBP exp 2027-01 (photo verified) and Achrafieh 450,000 LBP exp 2027-06 (verified).', decider: 'rules' },
        { node: 'rule-best-offer', label: 'Tie-break', detail: 'Equal totals, so the later expiry wins: Achrafieh. Hamra excluded with reason "higher total or earlier expiry".', decider: 'rules' },
        { node: 'data-tickets', label: 'best_offer event', detail: 'A best_offer event is written to the ticket.', decider: 'rules' },
        { node: 'rule-identity-hidden', label: 'Redacted view', detail: 'Pharmacies only see actor "client"; client payload keys are dropped.', decider: 'rules' },
        { node: 'ep-ticket-stream', label: 'Client sees it live', detail: 'The ticket stream shows outreach, pharmacy_reply, offer, photo_check and best_offer in EN/AR.', decider: 'rules' },
        { node: 'tool-notify-requester', label: 'Plain notice', detail: '"Concor 5mg is ready at a pharmacy near you for 450,000 LBP. Tap reserve to hold it."', decider: 'wassit' },
        { node: 'rule-no-close-available-client', label: 'Agent does not close', detail: 'Wassit waits; the client reserves.', decider: 'rules' },
        { node: 'actor-client', label: 'Client reserves', detail: 'Client taps Reserve.', decider: 'client' },
        { node: 'ep-reserve', label: 'Reservation stored', detail: 'POST /client/tickets/{id}/reserve. Event: reserved.', decider: 'client' }
      ] }
  ];

  window.WZ_GRAPH_DATA = { nodes: nodes, links: links, workflows: workflows };
})();
