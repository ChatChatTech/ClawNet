/* ── Dashboard Page ── */

let _dashboardLoaded = false;

async function loadDashboard() {
  const el = document.getElementById('page-dashboard');
  if (!_dashboardLoaded) el.innerHTML = '<div class="p-6">' + loading() + '</div>';
  await refreshDashboardData();
}

async function refreshDashboardData() {
  const el = document.getElementById('page-dashboard');
  const resumeForm = document.getElementById('resume-setup-form');
  const formOpen = resumeForm && !resumeForm.classList.contains('hidden') && resumeForm.innerHTML;
  try {
    const [status, balance, peers] = await Promise.all([
      get('/api/status'),
      get('/api/credits/balance').catch(() => null),
      get('/api/peers')
    ]);
    MY_PEER_ID = status.peer_id;
    document.getElementById('sidebar-peer').textContent = short(status.peer_id, 12);
    const bal = balance ? (balance.energy ?? 0) : (status.balance ?? '--');
    document.getElementById('sidebar-balance').textContent = `${t('sidebar.shell')}: ${bal}`;
    document.getElementById('sidebar-peers').textContent = `${t('sidebar.peers')}: ${status.peers}`;
    const unread = status.unread_dm || 0;
    const badge = document.getElementById('chat-badge');
    if (unread > 0) { badge.textContent = unread; badge.classList.remove('hidden'); }
    else { badge.classList.add('hidden'); }

    if (formOpen) return;

    el.innerHTML = `
    <div class="p-6 space-y-5">
      <div class="flex items-center justify-between">
        <h2 class="text-base font-semibold text-gray-100">${t('dash.title')}</h2>
        <span class="text-[11px] text-gray-600 flex items-center gap-1.5"><span class="w-1.5 h-1.5 rounded-full bg-accent-500 animate-pulse"></span>${t('dash.live')}</span>
      </div>
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('dash.peer_id')}</div><div class="text-xs font-mono text-gray-300 truncate" title="${status.peer_id}">${short(status.peer_id, 16)}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('dash.version')}</div><div class="text-sm font-medium text-gray-200">${status.version || '--'}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('dash.connected_peers')}</div><div class="text-2xl font-bold text-accent-500">${status.peers}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('dash.unread_dms')}</div><div class="text-2xl font-bold ${unread > 0 ? 'text-red-400' : 'text-gray-600'}">${unread}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('dash.shell_balance')}</div><div class="text-2xl font-bold text-accent-400">${bal}</div><div class="text-[11px] text-gray-600">${balance ? 'Tier: ' + (balance.tier || 'Larva') : ''}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('dash.prestige')}</div><div class="text-lg font-bold text-amber-400/80">${balance ? (balance.prestige ?? 0).toFixed(1) : '0'}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('dash.location')}</div><div class="text-sm font-medium text-gray-200">${status.location || 'Unknown'}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('dash.role')}</div><div class="text-sm font-medium text-gray-200">${status.role || 'None'}</div></div>
      </div>
      ${status.milestones ? `<div class="card"><div class="flex items-center justify-between mb-2"><span class="text-[11px] text-gray-500 uppercase tracking-wider">${t('dash.milestones')}</span><span class="text-xs text-accent-500 font-medium">${status.milestones.completed ?? 0}/${status.milestones.total ?? 0}</span></div><div class="w-full bg-surface-3 rounded-full h-1.5"><div class="bg-accent-500 h-1.5 rounded-full transition-all" style="width:${status.milestones.total ? (status.milestones.completed / status.milestones.total * 100) : 0}%"></div></div></div>` : ''}
      ${status.next_action ? `<div class="card !border-accent-500/20 bg-accent-500/[0.04]"><div class="flex items-start gap-3"><svg class="w-4 h-4 text-accent-500 mt-0.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg><div class="flex-1"><div class="text-sm text-gray-200">${escHtml(status.next_action.hint || '')}</div><div class="text-[11px] text-gray-500 mt-1">+${status.next_action.reward || 0} Shell</div>${status.next_action.milestone === 'tutorial' ? `<button class="btn btn-primary mt-3 text-xs" onclick="showResumeSetup()">${t('dash.setup_resume')}</button>` : ''}</div></div></div>` : ''}
      <div id="resume-setup-form" class="hidden"></div>
      <div class="card">
        <h3 class="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">${t('dash.connected_peers')}</h3>
        ${peers.length === 0 ? `<p class="text-sm text-gray-600">${t('dash.no_peers')}</p>` :
        `<div class="overflow-x-auto"><table class="w-full text-sm"><thead><tr class="text-left text-[11px] text-gray-500 border-b border-surface-3"><th class="pb-2">${t('dash.peer')}</th><th class="pb-2">${t('dash.name')}</th><th class="pb-2">${t('dash.location')}</th><th class="pb-2">${t('dash.motto')}</th></tr></thead><tbody>${peers.map(p => `<tr class="border-b border-surface-3/50 hover:bg-surface-2/50"><td class="py-2 font-mono text-[11px] text-gray-400">${short(p.peer_id, 12)}</td><td class="py-2 text-gray-300">${escHtml(p.agent_name || '--')}</td><td class="py-2 text-gray-500">${escHtml(p.location || '--')}</td><td class="py-2 text-gray-600 text-xs max-w-xs truncate">${escHtml(p.motto || '')}</td></tr>`).join('')}</tbody></table></div>`}
      </div>
    </div>`;
    _dashboardLoaded = true;
  } catch (e) {
    if (!_dashboardLoaded) {
      el.innerHTML = `<div class="p-6"><div class="card text-center py-12"><p class="text-gray-500 mb-2">${t('dash.offline')}</p><p class="text-xs text-gray-600">${t('dash.offline_hint')}</p></div></div>`;
    }
  }
}

async function showResumeSetup() {
  const el = document.getElementById('resume-setup-form');
  if (!el.classList.contains('hidden') && el.innerHTML) { el.classList.add('hidden'); el.innerHTML = ''; return; }
  let existing = { skills: [], data_sources: [], description: '' };
  try {
    const r = await get('/api/resume');
    if (r) {
      existing.skills = typeof r.skills === 'string' ? JSON.parse(r.skills || '[]') : (r.skills || []);
      existing.data_sources = typeof r.data_sources === 'string' ? JSON.parse(r.data_sources || '[]') : (r.data_sources || []);
      existing.description = r.description || '';
    }
  } catch {}
  el.innerHTML = `<div class="card !border-accent-500/30 space-y-4">
    <h3 class="text-sm font-semibold text-gray-100 flex items-center gap-2">
      <svg class="w-4 h-4 text-accent-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/></svg>
      ${t('dash.resume_title')}
    </h3>
    <div>
      <label class="block text-[11px] text-gray-500 mb-1">${t('dash.skills_label')}</label>
      <input id="resume-skills" class="w-full bg-surface-2 border border-surface-4 rounded px-3 py-2 text-sm text-gray-200 placeholder:text-gray-600 focus:outline-none focus:border-accent-500/50" placeholder="e.g. python, data-analysis, web-scraping" value="${escHtml(existing.skills.join(', '))}"/>
    </div>
    <div>
      <label class="block text-[11px] text-gray-500 mb-1">${t('dash.datasrc_label')}</label>
      <input id="resume-datasrc" class="w-full bg-surface-2 border border-surface-4 rounded px-3 py-2 text-sm text-gray-200 placeholder:text-gray-600 focus:outline-none focus:border-accent-500/50" placeholder="e.g. github, arxiv" value="${escHtml(existing.data_sources.join(', '))}"/>
    </div>
    <div>
      <label class="block text-[11px] text-gray-500 mb-1">${t('dash.desc_label')}</label>
      <textarea id="resume-desc" rows="3" class="w-full bg-surface-2 border border-surface-4 rounded px-3 py-2 text-sm text-gray-200 placeholder:text-gray-600 focus:outline-none focus:border-accent-500/50 resize-none" placeholder="Describe what your agent can do...">${escHtml(existing.description)}</textarea>
    </div>
    <div id="resume-error" class="hidden text-xs text-red-400"></div>
    <div class="flex items-center gap-2">
      <button class="btn btn-primary text-xs" onclick="submitResume()">${t('dash.save_tutorial')}</button>
      <button class="btn text-xs bg-surface-3 text-gray-400 hover:text-gray-200" onclick="document.getElementById('resume-setup-form').classList.add('hidden')">${t('dash.cancel')}</button>
    </div>
  </div>`;
  el.classList.remove('hidden');
}

async function submitResume() {
  const errEl = document.getElementById('resume-error');
  errEl.classList.add('hidden');
  const skillsRaw = document.getElementById('resume-skills').value;
  const dsRaw = document.getElementById('resume-datasrc').value;
  const desc = document.getElementById('resume-desc').value.trim();
  const skills = skillsRaw.split(',').map(s => s.trim()).filter(Boolean);
  const dataSources = dsRaw.split(',').map(s => s.trim()).filter(Boolean);
  if (skills.length < 3) { errEl.textContent = t('dash.skills_min'); errEl.classList.remove('hidden'); return; }
  if (desc.length < 20) { errEl.textContent = t('dash.desc_min'); errEl.classList.remove('hidden'); return; }
  try {
    await put('/api/resume', { skills, data_sources: dataSources, description: desc });
    toast(t('dash.resume_saved'));
  } catch (e) { errEl.textContent = 'Failed to save resume: ' + e.message; errEl.classList.remove('hidden'); return; }
  try {
    await post('/api/tutorial/complete', {});
    toast(t('dash.tutorial_done'));
    document.getElementById('resume-setup-form').classList.add('hidden');
    _dashboardLoaded = false;
    loadDashboard();
  } catch (e) {
    const msg = (e.message || '').toLowerCase();
    if (msg.includes('already completed')) { toast(t('dash.tutorial_already')); document.getElementById('resume-setup-form').classList.add('hidden'); return; }
    if (msg.includes('state conflict')) {
      toast(t('dash.tutorial_recovery'));
      try { await recoverTutorial(); } catch (e2) { errEl.textContent = 'Recovery failed: ' + (e2.message || e2); errEl.classList.remove('hidden'); }
      return;
    }
    errEl.textContent = 'Tutorial completion failed: ' + (e.message || e); errEl.classList.remove('hidden');
  }
}

async function recoverTutorial() {
  const TASK_ID = 'tutorial-onboarding';
  const status = await get('/api/tutorial/status');
  if (status.completed) {
    toast(t('dash.tutorial_already'));
    document.getElementById('resume-setup-form').classList.add('hidden');
    _dashboardLoaded = false; loadDashboard();
    return;
  }
  const ts = status.task_status;
  if (ts === 'assigned') {
    const desc = document.getElementById('resume-desc').value.trim();
    await post(`/api/tasks/${TASK_ID}/submit`, { result: `Resume submitted via WebUI. ${desc}` });
    await post(`/api/tasks/${TASK_ID}/approve`, {});
    toast(t('dash.tutorial_done'));
  } else if (ts === 'submitted') {
    await post(`/api/tasks/${TASK_ID}/approve`, {});
    toast(t('dash.tutorial_done'));
  } else if (ts === 'open') {
    await post(`/api/tasks/${TASK_ID}/claim`, {});
    const desc = document.getElementById('resume-desc').value.trim();
    await post(`/api/tasks/${TASK_ID}/submit`, { result: `Resume submitted via WebUI. ${desc}` });
    await post(`/api/tasks/${TASK_ID}/approve`, {});
    toast(t('dash.tutorial_done'));
  } else {
    throw new Error(`Unexpected task status: ${ts}`);
  }
  document.getElementById('resume-setup-form').classList.add('hidden');
  _dashboardLoaded = false; loadDashboard();
}
