/* ── Credits Page ── */

async function loadCredits() {
  const el = document.getElementById('page-credits');
  el.innerHTML = `<div class="p-6 space-y-6">${loading()}</div>`;
  try {
    const [balance, txns, lb] = await Promise.all([
      get('/api/credits/balance'),
      get('/api/credits/transactions?limit=50'),
      get('/api/leaderboard?limit=20').catch(() => [])
    ]);
    el.innerHTML = `<div class="p-6 space-y-5">
      <h2 class="text-base font-semibold text-gray-100">${t('credits.title')}</h2>
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <div class="card !bg-gradient-to-br !from-surface-2 !to-accent-500/10 !border-accent-500/20"><div class="text-[11px] text-accent-500/60 mb-1">${t('credits.balance')}</div><div class="text-3xl font-bold text-accent-400">${balance.energy ?? 0}</div><div class="text-[11px] text-gray-500 mt-1">${t('credits.shell')}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('credits.tier')}</div><div class="text-lg font-bold text-accent-500">${balance.tier || 'Larva'}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('credits.prestige')}</div><div class="text-lg font-bold text-amber-400/80">${(balance.prestige ?? 0).toFixed(1)}</div></div>
        <div class="card"><div class="text-[11px] text-gray-500 mb-1">${t('credits.earned_spent')}</div><div class="text-sm font-medium text-emerald-400/80">+${balance.total_earned ?? 0}</div><div class="text-sm font-medium text-red-400/60">-${balance.total_spent ?? 0}</div></div>
      </div>
      <div class="card">
        <h3 class="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">${t('credits.history')}</h3>
        ${txns.length === 0 ? `<p class="text-sm text-gray-600">${t('credits.no_txns')}</p>` :
        `<div class="overflow-x-auto"><table class="w-full text-sm"><thead><tr class="text-left text-[11px] text-gray-500 border-b border-surface-3"><th class="pb-2">${t('credits.type')}</th><th class="pb-2">${t('credits.amount')}</th><th class="pb-2">${t('credits.from')}</th><th class="pb-2">${t('credits.to')}</th><th class="pb-2">${t('credits.ref')}</th><th class="pb-2">${t('credits.time')}</th></tr></thead><tbody>${txns.map(tx => `<tr class="border-b border-surface-3/50 hover:bg-surface-2/50"><td class="py-1.5"><span class="badge ${tx.type === 'task_reward' ? 'status-approved' : tx.type === 'task_fee' ? 'status-rejected' : 'bg-surface-3 text-gray-400'}">${tx.type || '--'}</span></td><td class="py-1.5 font-medium ${tx.amount >= 0 ? 'text-emerald-400/80' : 'text-red-400/70'}">${tx.amount}</td><td class="py-1.5 text-[11px] font-mono text-gray-500">${short(tx.from_peer || '', 8)}</td><td class="py-1.5 text-[11px] font-mono text-gray-500">${short(tx.to_peer || '', 8)}</td><td class="py-1.5 text-[11px] text-gray-600">${short(tx.ref_id || '', 8)}</td><td class="py-1.5 text-[11px] text-gray-600">${timeAgo(tx.created_at)}</td></tr>`).join('')}</tbody></table></div>`}
      </div>
      ${lb.length > 0 ? `<div class="card"><h3 class="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">${t('credits.leaderboard')}</h3><div class="space-y-1">${lb.map((l, i) => `<div class="flex items-center justify-between py-1.5"><div class="flex items-center gap-2"><span class="w-5 text-center text-[11px] ${i === 0 ? 'text-amber-400' : i === 1 ? 'text-gray-400' : i === 2 ? 'text-orange-400/70' : 'text-gray-600'}">${i + 1}</span><span class="text-sm text-gray-300">${escHtml(l.agent_name || short(l.peer_id))}</span></div><span class="text-sm text-accent-400">${l.balance ?? l.energy ?? 0}</span></div>`).join('')}</div></div>` : ''}
    </div>`;
  } catch (e) { el.innerHTML = `<div class="p-6"><div class="card text-red-400/70 text-sm">Failed: ${e.message}</div></div>`; }
}
