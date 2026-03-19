/* ── Knowledge Page ── */

async function loadKnowledge() {
  const el = document.getElementById('page-knowledge');
  el.innerHTML = `<div class="p-6 space-y-4">
    <div class="flex items-center justify-between"><h2 class="text-base font-semibold text-gray-100">${t('know.title')}</h2><button class="btn btn-primary" onclick="showKnowledgePublish()">${t('know.publish')}</button></div>
    <div class="flex gap-2"><input id="k-search" class="input flex-1" placeholder="${t('know.search_ph')}" onkeydown="if(event.key==='Enter')searchKnowledge()"><button class="btn btn-secondary" onclick="searchKnowledge()">${t('know.search')}</button><button class="btn btn-secondary" onclick="loadKnowledgeFeed()">${t('know.feed')}</button></div>
    <div id="k-publish-form" class="hidden"></div>
    <div id="k-feed">${loading()}</div>
  </div>`;
  await loadKnowledgeFeed();
}

async function loadKnowledgeFeed() {
  const el = document.getElementById('k-feed');
  try {
    const e = await get('/api/knowledge/feed?limit=30');
    if (!e.length) { el.innerHTML = `<p class="text-sm text-gray-600 text-center py-8">${t('know.none')}</p>`; return; }
    el.innerHTML = e.map(x => knowledgeCard(x)).join('');
  } catch { el.innerHTML = '<p class="text-sm text-red-400/70 text-center py-4">Failed to load</p>'; }
}

async function searchKnowledge() {
  const q = document.getElementById('k-search').value.trim();
  if (!q) { loadKnowledgeFeed(); return; }
  const el = document.getElementById('k-feed'); el.innerHTML = loading();
  try {
    const e = await get(`/api/knowledge/search?q=${encodeURIComponent(q)}`);
    if (!e.length) { el.innerHTML = `<p class="text-sm text-gray-600 text-center py-8">${t('know.no_results')}</p>`; return; }
    el.innerHTML = e.map(x => knowledgeCard(x)).join('');
  } catch {}
}

function knowledgeCard(e) {
  return `<div class="card mb-3">
    <div class="flex items-start justify-between mb-2">
      <div><h3 class="font-medium text-sm text-gray-200">${escHtml(e.title)}</h3><p class="text-[11px] text-gray-500">${escHtml(e.author_name || short(e.author_id))} &middot; ${timeAgo(e.created_at)}</p></div>
      <div class="flex gap-1 shrink-0"><button class="btn btn-secondary text-xs !px-2 !py-1" onclick="reactKnowledge('${e.id}','upvote')">&#9650; ${e.upvotes || 0}</button><button class="btn btn-secondary text-xs !px-2 !py-1" onclick="toggleReplies('${e.id}')">${t('know.reply')}</button></div>
    </div>
    <p class="text-sm text-gray-400 whitespace-pre-wrap">${escHtml(e.body)}</p>
    ${e.domains && e.domains.length ? `<div class="flex gap-1 mt-2">${e.domains.map(d => `<span class="badge bg-accent-500/10 text-accent-400">${escHtml(d)}</span>`).join('')}</div>` : ''}
    <div id="replies-${e.id}" class="hidden mt-3 border-t border-surface-3 pt-3 space-y-2"></div>
  </div>`;
}

async function reactKnowledge(id, reaction) {
  try { await post(`/api/knowledge/${id}/react`, { reaction }); toast(t('know.reacted'), 'success'); loadKnowledgeFeed(); } catch {}
}

async function toggleReplies(id) {
  const el = document.getElementById('replies-' + id);
  if (!el.classList.contains('hidden')) { el.classList.add('hidden'); return; }
  el.classList.remove('hidden'); el.innerHTML = loading();
  try {
    const r = await get(`/api/knowledge/${id}/replies`);
    el.innerHTML = r.map(x => `<div class="bg-surface-2 rounded-lg px-3 py-2"><div class="text-[11px] text-gray-500 mb-1">${escHtml(x.author_name || short(x.author_id))} &middot; ${timeAgo(x.created_at)}</div><p class="text-sm text-gray-300">${escHtml(x.body)}</p></div>`).join('') +
      `<div class="flex gap-2"><input id="reply-input-${id}" class="input flex-1 text-xs" placeholder="${t('know.reply_ph')}"><button class="btn btn-primary text-xs" onclick="replyKnowledge('${id}')">${t('know.send')}</button></div>`;
  } catch { el.innerHTML = '<p class="text-xs text-red-400/70">Failed to load replies</p>'; }
}

async function replyKnowledge(id) {
  const i = document.getElementById('reply-input-' + id);
  const b = i.value.trim();
  if (!b) return;
  try { await post(`/api/knowledge/${id}/reply`, { body: b }); toast(t('know.reply_sent'), 'success'); toggleReplies(id); toggleReplies(id); } catch {}
}

function showKnowledgePublish() {
  const el = document.getElementById('k-publish-form'); el.classList.toggle('hidden');
  el.innerHTML = `<div class="card space-y-3"><h3 class="text-sm font-medium text-gray-200">${t('know.publish_title')}</h3><input id="kp-title" class="input" placeholder="${t('know.field_title')}"><textarea id="kp-body" class="input" rows="4" placeholder="${t('know.field_body')}"></textarea><input id="kp-domain" class="input" placeholder="${t('know.field_domain')}"><div class="flex gap-2"><button class="btn btn-primary" onclick="publishKnowledge()">${t('know.publish').replace('+ ','')}</button><button class="btn btn-secondary" onclick="document.getElementById('k-publish-form').classList.add('hidden')">${t('know.cancel')}</button></div></div>`;
}

async function publishKnowledge() {
  const ti = document.getElementById('kp-title').value.trim(), b = document.getElementById('kp-body').value.trim();
  if (!ti || !b) { toast(t('know.title_body_req'), 'warning'); return; }
  const p = { title: ti, body: b };
  const d = document.getElementById('kp-domain').value.trim();
  if (d) p.domains = [d];
  try { await post('/api/knowledge', p); toast(t('know.published'), 'success'); document.getElementById('k-publish-form').classList.add('hidden'); loadKnowledgeFeed(); } catch {}
}
