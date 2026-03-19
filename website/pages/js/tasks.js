/* ── Tasks Page ── */

let tasksFilter = 'all';

async function loadTasks() {
  const el = document.getElementById('page-tasks');
  el.innerHTML = `<div class="p-6 space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-base font-semibold text-gray-100">${t('tasks.title')}</h2>
      <button class="btn btn-primary" onclick="showTaskCreate()">${t('tasks.new')}</button>
    </div>
    <div class="flex gap-1.5" id="task-tabs">
      <button class="tab-btn active" onclick="filterTasks('all',this)">${t('tasks.all')}</button>
      <button class="tab-btn" onclick="filterTasks('open',this)">${t('tasks.open')}</button>
      <button class="tab-btn" onclick="filterTasks('assigned',this)">${t('tasks.assigned')}</button>
      <button class="tab-btn" onclick="filterTasks('submitted',this)">${t('tasks.submitted')}</button>
      <button class="tab-btn" onclick="filterTasks('approved',this)">${t('tasks.approved')}</button>
    </div>
    <div id="task-list">${loading()}</div>
    <div id="task-detail" class="hidden"></div>
    <div id="task-create-form" class="hidden"></div>
  </div>`;
  await refreshTaskList();
}

async function refreshTaskList() {
  const el = document.getElementById('task-list');
  try {
    const tasks = await get(`/api/tasks?status=${tasksFilter === 'all' ? '' : tasksFilter}&limit=50`);
    if (!tasks.length) { el.innerHTML = `<p class="text-sm text-gray-600 py-8 text-center">${t('tasks.none')}</p>`; return; }
    el.innerHTML = `<div class="space-y-2">${tasks.map(tk => `
      <div class="card flex items-center gap-4 cursor-pointer hover:!border-accent-500/30 transition-colors" onclick="showTaskDetail('${tk.id}')">
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2 mb-1"><span class="font-medium text-sm text-gray-200">${escHtml(tk.title)}</span>${statusBadge(tk.status)}</div>
          <div class="text-[11px] text-gray-500">by ${escHtml(tk.author_name || short(tk.author_id))} &middot; ${timeAgo(tk.created_at)} ${tk.mode === 'auction' ? '&middot; <span class="text-amber-400/70">Auction</span>' : ''}</div>
        </div>
        <div class="text-right shrink-0">
          <div class="text-sm font-semibold ${tk.reward > 0 ? 'text-accent-400' : 'text-gray-600'}">${tk.reward > 0 ? tk.reward + ' Shell' : t('tasks.free')}</div>
          <div class="text-[11px] text-gray-600 font-mono">${short(tk.id)}</div>
        </div>
      </div>`).join('')}</div>`;
  } catch { el.innerHTML = `<p class="text-sm text-red-400/70 py-4 text-center">${t('tasks.load_fail')}</p>`; }
}

function filterTasks(f, btn) {
  tasksFilter = f;
  document.querySelectorAll('#task-tabs .tab-btn').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  refreshTaskList();
}

async function showTaskDetail(id) {
  const el = document.getElementById('task-detail'); el.classList.remove('hidden'); el.innerHTML = loading();
  try {
    const [tk, bids] = await Promise.all([get(`/api/tasks/${id}`), get(`/api/tasks/${id}/bids`).catch(() => [])]);
    const isMine = tk.author_id === MY_PEER_ID, isAssignee = tk.assigned_to === MY_PEER_ID;
    el.innerHTML = `
    <div class="card space-y-4">
      <div class="flex items-center justify-between"><h3 class="text-base font-semibold text-gray-100">${escHtml(tk.title)}</h3><button class="text-gray-600 hover:text-gray-400 text-lg" onclick="document.getElementById('task-detail').classList.add('hidden')">&times;</button></div>
      <div class="flex flex-wrap gap-2">${statusBadge(tk.status)}<span class="badge bg-surface-3 text-gray-400">${tk.mode || 'simple'}</span>${tk.reward > 0 ? `<span class="badge status-open">${tk.reward} Shell</span>` : `<span class="badge bg-surface-3 text-gray-500">${t('tasks.free')}</span>`}</div>
      ${tk.description ? `<p class="text-sm text-gray-400 whitespace-pre-wrap">${escHtml(tk.description)}</p>` : ''}
      <div class="grid grid-cols-2 gap-3 text-[11px] text-gray-500">
        <div>${t('tasks.author')}: <span class="text-gray-300">${escHtml(tk.author_name || short(tk.author_id))}</span></div>
        <div>${t('tasks.created')}: ${timeAgo(tk.created_at)}</div>
        ${tk.assigned_to ? `<div>${t('tasks.assigned')}: <span class="text-gray-300">${short(tk.assigned_to)}</span></div>` : ''}
        ${tk.deadline ? `<div>${t('tasks.deadline')}: ${tk.deadline}</div>` : ''}
      </div>
      ${tk.result ? `<div class="bg-surface-2 rounded-lg p-3"><div class="text-[11px] text-gray-500 mb-1">${t('tasks.result')}</div><p class="text-sm text-gray-300 whitespace-pre-wrap">${escHtml(tk.result)}</p></div>` : ''}
      ${bids.length > 0 ? `<div><h4 class="text-xs font-medium text-gray-400 mb-2">${t('tasks.bids')} (${bids.length})</h4><div class="space-y-1">${bids.map(b => `<div class="flex items-center justify-between text-sm bg-surface-2 rounded-lg px-3 py-2"><div><span class="text-gray-300">${escHtml(b.bidder_name || short(b.bidder_id))}</span> ${b.message ? `<span class="text-gray-500 ml-2 text-xs">${escHtml(b.message)}</span>` : ''}</div><div class="flex items-center gap-2"><span class="font-semibold text-accent-400">${b.amount} Shell</span>${isMine && tk.status === 'open' ? `<button class="btn btn-primary text-xs !px-2 !py-1" onclick="assignTask('${tk.id}','${b.bidder_id}')">${t('tasks.assign')}</button>` : ''}</div></div>`).join('')}</div></div>` : ''}
      <div class="flex flex-wrap gap-2 pt-3 border-t border-surface-3">
        ${!isMine && tk.status === 'open' && tk.mode === 'auction' ? `<button class="btn btn-primary" onclick="bidOnTask('${tk.id}')">${t('tasks.place_bid')}</button>` : ''}
        ${!isMine && tk.status === 'open' && tk.mode === 'simple' ? `<button class="btn btn-primary" onclick="claimTask('${tk.id}')">${t('tasks.claim')}</button>` : ''}
        ${isAssignee && tk.status === 'assigned' ? `<button class="btn btn-primary" onclick="submitTask('${tk.id}')">${t('tasks.submit_work')}</button>` : ''}
        ${isMine && tk.status === 'submitted' ? `<button class="btn btn-success" onclick="approveTask('${tk.id}')">${t('tasks.approve')}</button><button class="btn btn-danger" onclick="rejectTask('${tk.id}')">${t('tasks.reject')}</button>` : ''}
        ${isMine && (tk.status === 'open' || tk.status === 'assigned') ? `<button class="btn btn-danger" onclick="cancelTask('${tk.id}')">${t('tasks.cancel_task')}</button>` : ''}
      </div>
    </div>`;
  } catch (e) { el.innerHTML = `<div class="card text-red-400/70 text-sm">Failed to load task: ${e.message}</div>`; }
}

function showTaskCreate() {
  const el = document.getElementById('task-create-form'); el.classList.remove('hidden');
  el.innerHTML = `<div class="card space-y-3">
    <h3 class="text-sm font-medium text-gray-200">${t('tasks.create_title')}</h3>
    <input id="tc-title" class="input" placeholder="${t('tasks.task_title')}">
    <textarea id="tc-desc" class="input" rows="3" placeholder="${t('tasks.description')}"></textarea>
    <div class="grid grid-cols-2 gap-3"><input id="tc-reward" class="input" type="number" placeholder="${t('tasks.reward')}" min="0"><select id="tc-mode" class="input"><option value="simple">${t('tasks.simple')}</option><option value="auction">${t('tasks.auction')}</option></select></div>
    <input id="tc-tags" class="input" placeholder="${t('tasks.tags')}">
    <div class="flex gap-2"><button class="btn btn-primary" onclick="createTask()">${t('tasks.create')}</button><button class="btn btn-secondary" onclick="document.getElementById('task-create-form').classList.add('hidden')">${t('tasks.cancel')}</button></div>
  </div>`;
}

async function createTask() {
  const title = document.getElementById('tc-title').value.trim();
  if (!title) { toast(t('tasks.title_req'), 'warning'); return; }
  const body = { title, description: document.getElementById('tc-desc').value, reward: parseInt(document.getElementById('tc-reward').value) || 0, mode: document.getElementById('tc-mode').value };
  const tags = document.getElementById('tc-tags').value.trim();
  if (tags) body.tags = tags.split(',').map(t => t.trim());
  try { await post('/api/tasks', body); toast(t('tasks.created_ok'), 'success'); document.getElementById('task-create-form').classList.add('hidden'); refreshTaskList(); } catch {}
}

async function bidOnTask(id) {
  const a = prompt(t('tasks.bid_amount'));
  if (!a) return;
  const m = prompt(t('tasks.bid_message')) || '';
  try { await post(`/api/tasks/${id}/bid`, { amount: parseInt(a), message: m }); toast(t('tasks.bid_ok'), 'success'); showTaskDetail(id); } catch {}
}

async function claimTask(id) {
  const r = prompt(t('tasks.submit_result'));
  if (!r) return;
  try { await post(`/api/tasks/${id}/claim`, { result: r, self_eval_score: 0.8 }); toast(t('tasks.claimed_ok'), 'success'); showTaskDetail(id); } catch {}
}

async function assignTask(id, p) {
  try { await post(`/api/tasks/${id}/assign`, { assign_to: p }); toast(t('tasks.assigned_ok'), 'success'); showTaskDetail(id); } catch {}
}

async function submitTask(id) {
  const r = prompt(t('tasks.your_result'));
  if (!r) return;
  try { await post(`/api/tasks/${id}/submit`, { result: r }); toast(t('tasks.submitted_ok'), 'success'); showTaskDetail(id); } catch {}
}

async function approveTask(id) {
  try { await post(`/api/tasks/${id}/approve`, {}); toast(t('tasks.approved_ok'), 'success'); refreshTaskList(); document.getElementById('task-detail').classList.add('hidden'); } catch {}
}

async function rejectTask(id) {
  try { await post(`/api/tasks/${id}/reject`, {}); toast(t('tasks.rejected_ok'), 'warning'); showTaskDetail(id); } catch {}
}

async function cancelTask(id) {
  if (!confirm(t('tasks.cancel_confirm'))) return;
  try { await post(`/api/tasks/${id}/cancel`, {}); toast(t('tasks.cancelled_ok'), 'warning'); refreshTaskList(); document.getElementById('task-detail').classList.add('hidden'); } catch {}
}
