/* ── Chat Page ── */

let chatActivePeer = null;

async function loadChat() {
  const el = document.getElementById('page-chat');
  el.innerHTML = `<div class="flex h-screen">
    <div class="w-64 border-r border-surface-3 bg-surface-1 flex flex-col">
      <div class="p-4 border-b border-surface-3"><h2 class="text-sm font-semibold text-gray-200">${t('chat.title')}</h2></div>
      <div id="chat-inbox" class="flex-1 overflow-y-auto">${loading()}</div>
      <div class="p-3 border-t border-surface-3"><button class="btn btn-primary w-full text-xs" onclick="newChat()">${t('chat.new')}</button></div>
    </div>
    <div id="chat-thread" class="flex-1 flex flex-col bg-surface-0">
      <div class="flex-1 flex items-center justify-center text-gray-600 text-sm">${t('chat.select')}</div>
    </div>
  </div>`;
  await loadChatInbox();
}

async function loadChatInbox() {
  const el = document.getElementById('chat-inbox');
  try {
    const msgs = await get('/api/dm/inbox');
    const threads = {};
    msgs.forEach(m => {
      if (!threads[m.peer_id]) threads[m.peer_id] = { peer_id: m.peer_id, last: m, unread: 0 };
      if (!m.read && m.direction === 'received') threads[m.peer_id].unread++;
      if (new Date(m.created_at) > new Date(threads[m.peer_id].last.created_at)) threads[m.peer_id].last = m;
    });
    const sorted = Object.values(threads).sort((a, b) => new Date(b.last.created_at) - new Date(a.last.created_at));
    if (!sorted.length) { el.innerHTML = `<p class="text-sm text-gray-600 text-center py-8">${t('chat.none')}</p>`; return; }
    el.innerHTML = sorted.map(th => `<div class="px-4 py-3 border-b border-surface-3/50 cursor-pointer hover:bg-surface-2/50 ${chatActivePeer === th.peer_id ? 'bg-accent-500/5 border-l-2 !border-l-accent-500' : ''}" onclick="openThread('${th.peer_id}')"><div class="flex justify-between items-center"><span class="text-xs font-medium text-gray-300 truncate font-mono">${short(th.peer_id, 12)}</span>${th.unread > 0 ? `<span class="badge bg-red-500/80 text-white text-[10px]">${th.unread}</span>` : ''}</div><p class="text-[11px] text-gray-600 truncate mt-0.5">${escHtml(th.last.body).slice(0, 50)}</p></div>`).join('');
  } catch { el.innerHTML = '<p class="text-sm text-red-400/70 text-center py-4">Failed to load</p>'; }
}

async function openThread(peerId) {
  chatActivePeer = peerId;
  loadChatInbox();
  const el = document.getElementById('chat-thread'); el.innerHTML = loading();
  try {
    const msgs = await get(`/api/dm/thread/${peerId}?limit=100`);
    el.innerHTML = `<div class="p-3 border-b border-surface-3 bg-surface-1"><h3 class="text-xs font-medium text-gray-300 font-mono">${short(peerId, 20)}</h3></div>
      <div class="flex-1 overflow-y-auto p-4 space-y-2" id="chat-messages">${msgs.map(m => `<div class="flex ${m.direction === 'sent' ? 'justify-end' : 'justify-start'}"><div class="max-w-[70%] rounded-xl px-3 py-2 text-sm ${m.direction === 'sent' ? 'bg-accent-500/20 text-accent-700' : 'bg-surface-2 text-gray-300 border border-surface-3'}">${escHtml(m.body)}<div class="text-[10px] mt-1 ${m.direction === 'sent' ? 'text-accent-500/40' : 'text-gray-600'}">${timeAgo(m.created_at)}</div></div></div>`).join('')}</div>
      <div class="p-3 bg-surface-1 border-t border-surface-3 flex gap-2"><input id="chat-input" class="input flex-1" placeholder="${t('chat.input_ph')}" onkeydown="if(event.key==='Enter')sendChatMsg()"><button class="btn btn-primary" onclick="sendChatMsg()">${t('chat.send')}</button></div>`;
    const m = document.getElementById('chat-messages'); m.scrollTop = m.scrollHeight;
  } catch { el.innerHTML = '<div class="p-4 text-red-400/70 text-sm">Failed to load thread</div>'; }
}

async function sendChatMsg() {
  if (!chatActivePeer) return;
  const i = document.getElementById('chat-input');
  const b = i.value.trim();
  if (!b) return;
  i.value = '';
  try { await post('/api/dm/send', { peer_id: chatActivePeer, body: b }); openThread(chatActivePeer); } catch {}
}

function newChat() {
  const p = prompt(t('chat.peer_prompt'));
  if (!p) return;
  openThread(p.trim());
}
