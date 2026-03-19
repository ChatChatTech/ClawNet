/* ── Swarm Page ── */

async function loadSwarm() {
  const el = document.getElementById('page-swarm');
  el.innerHTML = `<div class="p-6 space-y-4"><div class="flex items-center justify-between"><h2 class="text-base font-semibold text-gray-100">${t('swarm.title')}</h2><button class="btn btn-primary" onclick="showSwarmCreate()">${t('swarm.new')}</button></div><div id="swarm-create-form" class="hidden"></div><div id="swarm-list">${loading()}</div><div id="swarm-detail" class="hidden"></div></div>`;
  await refreshSwarmList();
}

async function refreshSwarmList() {
  const el = document.getElementById('swarm-list');
  try {
    const sw = await get('/api/swarm?limit=30');
    if (!sw.length) { el.innerHTML = `<p class="text-sm text-gray-600 text-center py-8">${t('swarm.none')}</p>`; return; }
    el.innerHTML = `<div class="grid gap-3">${sw.map(s => `<div class="card cursor-pointer hover:!border-accent-500/30 transition-colors" onclick="showSwarmDetail('${s.id}')"><div class="flex items-center justify-between mb-1"><h3 class="font-medium text-sm text-gray-200">${escHtml(s.title)}</h3>${statusBadge(s.status)}</div><p class="text-xs text-gray-500 mb-2">${escHtml(s.question)}</p><div class="flex gap-3 text-[11px] text-gray-500"><span>${s.contrib_count || 0} ${t('swarm.contributions')}</span><span>${escHtml(s.creator_name || short(s.creator_id))}</span><span>${timeAgo(s.created_at)}</span><span class="badge bg-surface-3 text-gray-400">${s.template_type || 'freeform'}</span></div></div>`).join('')}</div>`;
  } catch { el.innerHTML = '<p class="text-sm text-red-400/70 text-center py-4">Failed to load</p>'; }
}

async function showSwarmDetail(id) {
  const el = document.getElementById('swarm-detail'); el.classList.remove('hidden'); el.innerHTML = loading();
  try {
    const [sw, contribs] = await Promise.all([get(`/api/swarm/${id}`), get(`/api/swarm/${id}/contributions`)]);
    el.innerHTML = `<div class="card space-y-4"><div class="flex items-center justify-between"><h3 class="text-base font-semibold text-gray-100">${escHtml(sw.title)}</h3><button class="text-gray-600 hover:text-gray-400 text-lg" onclick="document.getElementById('swarm-detail').classList.add('hidden')">&times;</button></div>
      <p class="text-sm text-gray-400">${escHtml(sw.question)}</p><div class="flex gap-2">${statusBadge(sw.status)}<span class="badge bg-surface-3 text-gray-400">${sw.template_type || 'freeform'}</span></div>
      ${sw.synthesis ? `<div class="bg-accent-500/[0.05] rounded-lg p-4 border border-accent-500/15"><h4 class="text-xs font-medium text-accent-400 mb-2 uppercase tracking-wider">${t('swarm.synthesis')}</h4><p class="text-sm text-gray-300 whitespace-pre-wrap">${escHtml(sw.synthesis)}</p></div>` : ''}
      <div><h4 class="text-xs font-medium text-gray-400 mb-2">${t('swarm.contributions')} (${contribs.length})</h4><div class="space-y-2">${contribs.map(c => `<div class="bg-surface-2 rounded-lg p-3 border ${c.perspective === 'bull' ? 'perspective-bull' : c.perspective === 'bear' ? 'perspective-bear' : 'perspective-neutral'}"><div class="flex items-center gap-2 mb-1"><span class="text-xs text-gray-300">${escHtml(c.author_name || short(c.author_id))}</span>${c.perspective ? `<span class="badge ${c.perspective === 'bull' ? 'status-approved' : c.perspective === 'bear' ? 'status-rejected' : 'bg-surface-3 text-gray-400'}">${c.perspective}</span>` : ''}${c.confidence ? `<span class="text-[11px] text-gray-500">${(c.confidence * 100).toFixed(0)}%</span>` : ''}</div><p class="text-sm text-gray-400">${escHtml(c.body)}</p></div>`).join('')}</div></div>
      ${sw.status === 'open' ? `<div class="border-t border-surface-3 pt-3 space-y-2"><h4 class="text-xs font-medium text-gray-400">${t('swarm.contribute')}</h4><textarea id="swarm-contrib-body" class="input" rows="3" placeholder="${t('swarm.reasoning_ph')}"></textarea><div class="flex gap-2"><select id="swarm-contrib-perspective" class="input w-auto"><option value="neutral">${t('swarm.neutral')}</option><option value="bull">${t('swarm.bull')}</option><option value="bear">${t('swarm.bear')}</option><option value="devil-advocate">${t('swarm.devil')}</option></select><input id="swarm-contrib-confidence" class="input w-24" type="number" min="0" max="1" step="0.1" value="0.7"><button class="btn btn-primary" onclick="contributeSwarm('${id}')">${t('swarm.submit')}</button></div></div>` : ''}
      ${sw.status === 'open' && sw.creator_id === MY_PEER_ID ? `<button class="btn btn-secondary" onclick="synthesizeSwarm('${id}')">${t('swarm.gen_synthesis')}</button>` : ''}
    </div>`;
  } catch (e) { el.innerHTML = `<div class="card text-red-400/70 text-sm">Failed: ${e.message}</div>`; }
}

async function contributeSwarm(id) {
  const b = document.getElementById('swarm-contrib-body').value.trim();
  if (!b) { toast(t('swarm.body_req'), 'warning'); return; }
  try {
    await post(`/api/swarm/${id}/contribute`, {
      body: b,
      perspective: document.getElementById('swarm-contrib-perspective').value,
      confidence: parseFloat(document.getElementById('swarm-contrib-confidence').value) || 0.7
    });
    toast(t('swarm.contributed'), 'success'); showSwarmDetail(id);
  } catch {}
}

async function synthesizeSwarm(id) {
  const s = prompt(t('swarm.enter_synthesis'));
  if (!s) return;
  try { await post(`/api/swarm/${id}/synthesize`, { synthesis: s }); toast(t('swarm.synthesized'), 'success'); showSwarmDetail(id); } catch {}
}

function showSwarmCreate() {
  const el = document.getElementById('swarm-create-form'); el.classList.toggle('hidden');
  el.innerHTML = `<div class="card space-y-3"><h3 class="text-sm font-medium text-gray-200">${t('swarm.create_title')}</h3><input id="sc-title" class="input" placeholder="${t('swarm.field_title')}"><textarea id="sc-question" class="input" rows="2" placeholder="${t('swarm.field_question')}"></textarea><select id="sc-template" class="input"><option value="freeform">${t('swarm.freeform')}</option><option value="investment-analysis">${t('swarm.investment')}</option><option value="tech-selection">${t('swarm.tech')}</option></select><div class="flex gap-2"><button class="btn btn-primary" onclick="createSwarm()">${t('swarm.create')}</button><button class="btn btn-secondary" onclick="document.getElementById('swarm-create-form').classList.add('hidden')">${t('swarm.cancel')}</button></div></div>`;
}

async function createSwarm() {
  const ti = document.getElementById('sc-title').value.trim(), q = document.getElementById('sc-question').value.trim();
  if (!ti || !q) { toast(t('swarm.title_q_req'), 'warning'); return; }
  try { await post('/api/swarm', { title: ti, question: q, template_type: document.getElementById('sc-template').value }); toast(t('swarm.created'), 'success'); document.getElementById('swarm-create-form').classList.add('hidden'); refreshSwarmList(); } catch {}
}
