/* ── Predictions Page ── */

let predFilter = '';

async function loadPredictions() {
  const el = document.getElementById('page-predictions');
  el.innerHTML = `<div class="p-6 space-y-4"><div class="flex items-center justify-between"><h2 class="text-base font-semibold text-gray-100">${t('pred.title')}</h2><button class="btn btn-primary" onclick="showPredictionCreate()">${t('pred.new')}</button></div><div id="pred-create-form" class="hidden"></div><div class="flex gap-1.5" id="pred-tabs"><button class="tab-btn active" onclick="filterPredictions('',this)">${t('pred.all')}</button><button class="tab-btn" onclick="filterPredictions('open',this)">${t('pred.open')}</button><button class="tab-btn" onclick="filterPredictions('resolved',this)">${t('pred.resolved')}</button></div><div id="pred-list">${loading()}</div><div id="pred-detail" class="hidden"></div></div>`;
  await refreshPredictionList('');
}

async function refreshPredictionList(status) {
  const el = document.getElementById('pred-list');
  try {
    const preds = await get(`/api/predictions?status=${status}&limit=30`);
    if (!preds.length) { el.innerHTML = `<p class="text-sm text-gray-600 text-center py-8">${t('pred.none')}</p>`; return; }
    el.innerHTML = `<div class="grid gap-3">${preds.map(p => {
      let opts = []; try { opts = JSON.parse(p.options); } catch { opts = [p.options]; }
      return `<div class="card cursor-pointer hover:!border-accent-500/30 transition-colors" onclick="showPredictionDetail('${p.id}')"><div class="flex items-center justify-between mb-1"><h3 class="font-medium text-sm text-gray-200">${escHtml(p.question)}</h3>${statusBadge(p.status)}</div><div class="flex flex-wrap gap-1 mb-2">${opts.map(o => `<span class="badge bg-accent-500/10 text-accent-400">${escHtml(o)}</span>`).join('')}</div><div class="flex gap-3 text-[11px] text-gray-500"><span>${t('pred.stake')}: ${p.total_stake || 0}</span><span>${escHtml(p.creator_name || short(p.creator_id))}</span><span>${timeAgo(p.created_at)}</span>${p.category ? `<span class="badge bg-surface-3 text-gray-400">${p.category}</span>` : ''}</div>${p.result ? `<div class="mt-2 text-sm text-emerald-400/80">${t('pred.result')}: ${escHtml(p.result)}</div>` : ''}</div>`;
    }).join('')}</div>`;
  } catch { el.innerHTML = '<p class="text-sm text-red-400/70 text-center py-4">Failed to load</p>'; }
}

function filterPredictions(s, btn) {
  predFilter = s;
  document.querySelectorAll('#pred-tabs .tab-btn').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  refreshPredictionList(s);
}

async function showPredictionDetail(id) {
  const el = document.getElementById('pred-detail'); el.classList.remove('hidden'); el.innerHTML = loading();
  try {
    const p = await get(`/api/predictions/${id}`);
    let opts = []; try { opts = JSON.parse(p.options); } catch { opts = [p.options]; }
    const isMine = p.creator_id === MY_PEER_ID;
    el.innerHTML = `<div class="card space-y-4"><div class="flex items-center justify-between"><h3 class="text-base font-semibold text-gray-100">${escHtml(p.question)}</h3><button class="text-gray-600 hover:text-gray-400 text-lg" onclick="document.getElementById('pred-detail').classList.add('hidden')">&times;</button></div>
      <div class="flex gap-2">${statusBadge(p.status)}${p.category ? `<span class="badge bg-surface-3 text-gray-400">${p.category}</span>` : ''}</div>
      <div class="grid grid-cols-2 gap-2 text-[11px] text-gray-500"><div>${t('pred.total_stake')}: <strong class="text-gray-300">${p.total_stake || 0} Shell</strong></div><div>${t('pred.resolution')}: ${p.resolution_date || '--'}</div><div>${t('pred.creator')}: ${escHtml(p.creator_name || short(p.creator_id))}</div>${p.resolution_source ? `<div>${t('pred.source')}: ${escHtml(p.resolution_source)}</div>` : ''}</div>
      ${p.result ? `<div class="bg-emerald-500/10 rounded-lg p-3 text-emerald-400">${t('pred.result')}: ${escHtml(p.result)}</div>` : ''}
      ${p.status === 'open' ? `<div class="border-t border-surface-3 pt-3 space-y-2"><h4 class="text-xs font-medium text-gray-400">${t('pred.bet_title')}</h4><div class="flex gap-2 flex-wrap"><select id="bet-option" class="input w-auto">${opts.map(o => `<option value="${escHtml(o)}">${escHtml(o)}</option>`).join('')}</select><input id="bet-stake" class="input w-28" type="number" min="1" placeholder="${t('pred.stake')}"><button class="btn btn-primary" onclick="placeBet('${id}')">${t('pred.bet')}</button></div></div>` : ''}
      ${isMine && p.status === 'open' ? `<div class="border-t border-surface-3 pt-3 space-y-2"><h4 class="text-xs font-medium text-gray-400">${t('pred.resolve_title')}</h4><div class="flex gap-2"><select id="resolve-option" class="input w-auto">${opts.map(o => `<option value="${escHtml(o)}">${escHtml(o)}</option>`).join('')}</select><button class="btn btn-success" onclick="resolvePrediction('${id}')">${t('pred.resolve')}</button></div></div>` : ''}
    </div>`;
  } catch (e) { el.innerHTML = `<div class="card text-red-400/70 text-sm">Failed: ${e.message}</div>`; }
}

async function placeBet(id) {
  const o = document.getElementById('bet-option').value, s = parseInt(document.getElementById('bet-stake').value);
  if (!s || s < 1) { toast(t('pred.stake_req'), 'warning'); return; }
  try { await post(`/api/predictions/${id}/bet`, { option: o, stake: s }); toast(t('pred.bet_ok'), 'success'); showPredictionDetail(id); } catch {}
}

async function resolvePrediction(id) {
  const r = document.getElementById('resolve-option').value;
  if (!confirm(`${t('pred.resolve_confirm')} "${r}"?`)) return;
  try { await post(`/api/predictions/${id}/resolve`, { result: r }); toast(t('pred.resolved_ok'), 'success'); showPredictionDetail(id); } catch {}
}

function showPredictionCreate() {
  const el = document.getElementById('pred-create-form'); el.classList.toggle('hidden');
  el.innerHTML = `<div class="card space-y-3"><h3 class="text-sm font-medium text-gray-200">${t('pred.create_title')}</h3><input id="pc-question" class="input" placeholder="${t('pred.question')}"><input id="pc-options" class="input" placeholder="${t('pred.options')}"><div class="grid grid-cols-2 gap-3"><input id="pc-category" class="input" placeholder="${t('pred.category')}"><input id="pc-date" class="input" type="date"></div><div class="flex gap-2"><button class="btn btn-primary" onclick="createPrediction()">${t('pred.create')}</button><button class="btn btn-secondary" onclick="document.getElementById('pred-create-form').classList.add('hidden')">${t('pred.cancel')}</button></div></div>`;
}

async function createPrediction() {
  const q = document.getElementById('pc-question').value.trim(), o = document.getElementById('pc-options').value.split(',').map(x => x.trim()).filter(Boolean);
  if (!q || o.length < 2) { toast(t('pred.q_opts_req'), 'warning'); return; }
  try { await post('/api/predictions', { question: q, options: o, category: document.getElementById('pc-category').value.trim(), resolution_date: document.getElementById('pc-date').value || new Date(Date.now() + 7 * 86400000).toISOString() }); toast(t('pred.created'), 'success'); document.getElementById('pred-create-form').classList.add('hidden'); refreshPredictionList(predFilter); } catch {}
}
