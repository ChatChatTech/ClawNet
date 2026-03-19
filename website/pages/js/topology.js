/* ── Topology Page ── */

let topoMap = null, topoMarkers = {}, topoLines = [], topoSSE = null;

async function loadTopology() {
  const el = document.getElementById('page-topology');
  el.innerHTML = `<div class="p-6 space-y-4"><div class="flex items-center justify-between"><h2 class="text-base font-semibold text-gray-100">${t('topo.title')}</h2><span id="topo-status" class="text-[11px] text-gray-600">${t('topo.loading')}</span></div><div id="topo-map" class="w-full rounded-xl overflow-hidden border border-surface-3" style="height:520px;"></div><div id="topo-peers" class="card hidden"></div></div>`;
  setTimeout(() => {
    const m = document.getElementById('topo-map');
    if (!m) return;
    if (topoMap) { topoMap.remove(); topoMap = null; }
    topoMap = L.map('topo-map', { zoomControl: false }).setView([20, 0], 2);
    L.control.zoom({ position: 'bottomright' }).addTo(topoMap);
    L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', { attribution: '&copy; CARTO', maxZoom: 18 }).addTo(topoMap);
    loadTopoPeers();
    connectTopoSSE();
  }, 100);
}

async function loadTopoPeers() {
  try {
    const peers = await get('/api/peers/geo');
    Object.values(topoMarkers).forEach(m => topoMap.removeLayer(m));
    topoMarkers = {};
    topoLines.forEach(l => topoMap.removeLayer(l));
    topoLines = [];
    let selfLL = null;
    peers.forEach(p => {
      if (!p.geo || !p.geo.latitude || !p.geo.longitude) return;
      const ll = [p.geo.latitude, p.geo.longitude];
      const c = p.is_self ? themeHex('accent500') : themeHex('accent400');
      const border = p.is_self ? themeHex('accent500') : themeHex('accent100');
      const r = p.is_self ? 7 : 4;
      const mk = L.circleMarker(ll, {
        radius: r, fillColor: c, color: border,
        weight: p.is_self ? 2 : 1, fillOpacity: 0.85
      }).addTo(topoMap);
      const name = p.agent_name || p.short_id || short(p.peer_id);
      mk.bindPopup(`<div style="color:#e2e8f0;font-size:12px"><b>${escHtml(name)}</b><br><span style="color:#94a3b8">${escHtml(p.location || '')}</span><br><span style="color:#64748b;font-size:10px">${short(p.peer_id, 16)}</span>${p.latency_ms ? `<br><span style="color:${themeHex('accent500')}">${p.latency_ms}ms</span>` : ''}</div>`, { className: 'dark-popup' });
      if (p.is_self) { mk.bindTooltip(t('topo.you'), { permanent: true, direction: 'top', className: 'text-xs' }); selfLL = ll; }
      topoMarkers[p.peer_id] = mk;
    });
    if (selfLL) {
      peers.forEach(p => {
        if (p.is_self || !p.geo || !p.geo.latitude) return;
        const l = L.polyline([selfLL, [p.geo.latitude, p.geo.longitude]], {
          color: themeHex('accent500'), weight: 1, opacity: 0.15, dashArray: '4'
        }).addTo(topoMap);
        topoLines.push(l);
      });
    }
    document.getElementById('topo-status').textContent = `${peers.length} ${t('topo.nodes')}`;
    const pe = document.getElementById('topo-peers');
    if (peers.length > 0) {
      pe.classList.remove('hidden');
      pe.innerHTML = `<h3 class="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">${t('topo.peers')} (${peers.length})</h3><div class="grid grid-cols-2 md:grid-cols-3 gap-2">${peers.map(p => `<div class="flex items-center gap-2 text-[11px] p-2 rounded-lg ${p.is_self ? 'bg-accent-500/5' : 'bg-surface-2'}"><span class="w-1.5 h-1.5 rounded-full ${p.is_self ? 'bg-accent-500' : 'bg-accent-300/40'}"></span><div class="min-w-0"><div class="text-gray-300 truncate">${escHtml(p.agent_name || p.short_id || short(p.peer_id))}</div><div class="text-gray-600">${escHtml(p.location || 'Unknown')}</div></div></div>`).join('')}</div>`;
    }
  } catch { document.getElementById('topo-status').textContent = t('topo.fail'); }
}

function connectTopoSSE() {
  if (topoSSE) topoSSE.close();
  topoSSE = new EventSource(API + '/api/topology');
  topoSSE.onmessage = () => loadTopoPeers();
  topoSSE.onerror = () => {
    const s = document.getElementById('topo-status');
    if (s) s.textContent = t('topo.sse_dc');
  };
}
