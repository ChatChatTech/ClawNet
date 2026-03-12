package daemon

const topologyHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>LetChat — Network Topology</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: #0d1117; color: #c9d1d9; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; overflow: hidden; }
  #header { position: fixed; top: 0; left: 0; right: 0; z-index: 10; padding: 12px 20px; background: rgba(13,17,23,0.9); border-bottom: 1px solid #30363d; display: flex; align-items: center; gap: 16px; }
  #header h1 { font-size: 18px; color: #58a6ff; }
  #stats { font-size: 13px; color: #8b949e; }
  #stats span { color: #58a6ff; font-weight: 600; }
  svg { width: 100vw; height: 100vh; }
  .node-label { font-size: 11px; fill: #8b949e; pointer-events: none; }
  .node-self { fill: #58a6ff; }
  .node-peer { fill: #3fb950; }
  .link { stroke: #30363d; stroke-opacity: 0.8; }
  #panel { position: fixed; right: 20px; top: 60px; width: 280px; background: #161b22; border: 1px solid #30363d; border-radius: 8px; padding: 16px; display: none; z-index: 10; }
  #panel h3 { font-size: 14px; color: #58a6ff; margin-bottom: 8px; }
  #panel p { font-size: 12px; color: #8b949e; word-break: break-all; margin-bottom: 4px; }
  #panel .close { position: absolute; top: 8px; right: 12px; cursor: pointer; color: #8b949e; font-size: 16px; }
</style>
</head>
<body>
<div id="header">
  <h1>🌐 LetChat Topology</h1>
  <div id="stats">Peers: <span id="peer-count">0</span> &nbsp;|&nbsp; You: <span id="self-id">{{PEER_ID}}</span></div>
</div>
<div id="panel">
  <span class="close" onclick="document.getElementById('panel').style.display='none'">&times;</span>
  <h3 id="panel-title">Node</h3>
  <p id="panel-id"></p>
</div>
<svg id="graph"></svg>
<script src="https://d3js.org/d3.v7.min.js"></script>
<script>
const svg = d3.select('#graph');
const width = window.innerWidth;
const height = window.innerHeight;
const g = svg.append('g');

svg.call(d3.zoom().on('zoom', (e) => g.attr('transform', e.transform)));

const simulation = d3.forceSimulation()
  .force('link', d3.forceLink().id(d => d.id).distance(120))
  .force('charge', d3.forceManyBody().strength(-300))
  .force('center', d3.forceCenter(width / 2, height / 2))
  .force('collision', d3.forceCollide().radius(30));

let linkGroup = g.append('g');
let nodeGroup = g.append('g');
let labelGroup = g.append('g');

function update(data) {
  document.getElementById('peer-count').textContent = data.nodes.length - 1;

  // Links
  const link = linkGroup.selectAll('line').data(data.links, d => d.source + '-' + d.target);
  link.exit().remove();
  link.enter().append('line').attr('class', 'link').attr('stroke-width', 1.5);

  // Nodes
  const node = nodeGroup.selectAll('circle').data(data.nodes, d => d.id);
  node.exit().remove();
  const enter = node.enter().append('circle')
    .attr('r', d => d.self ? 12 : 8)
    .attr('class', d => d.self ? 'node-self' : 'node-peer')
    .style('cursor', 'pointer')
    .call(d3.drag()
      .on('start', (e, d) => { if (!e.active) simulation.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y; })
      .on('drag', (e, d) => { d.fx = e.x; d.fy = e.y; })
      .on('end', (e, d) => { if (!e.active) simulation.alphaTarget(0); d.fx = null; d.fy = null; })
    )
    .on('click', (e, d) => {
      document.getElementById('panel').style.display = 'block';
      document.getElementById('panel-title').textContent = d.self ? d.name + ' (You)' : d.name;
      document.getElementById('panel-id').textContent = 'Peer ID: ' + d.id;
    });

  // Labels
  const label = labelGroup.selectAll('text').data(data.nodes, d => d.id);
  label.exit().remove();
  label.enter().append('text')
    .attr('class', 'node-label')
    .attr('dx', 14)
    .attr('dy', 4)
    .text(d => d.self ? d.name : d.name);

  // Restart simulation
  simulation.nodes(data.nodes).on('tick', () => {
    linkGroup.selectAll('line')
      .attr('x1', d => d.source.x).attr('y1', d => d.source.y)
      .attr('x2', d => d.target.x).attr('y2', d => d.target.y);
    nodeGroup.selectAll('circle').attr('cx', d => d.x).attr('cy', d => d.y);
    labelGroup.selectAll('text').attr('x', d => d.x).attr('y', d => d.y);
  });
  simulation.force('link').links(data.links);
  simulation.alpha(0.5).restart();
}

// Connect to SSE endpoint
const evtSource = new EventSource('/api/topology');
evtSource.onmessage = (e) => {
  try { update(JSON.parse(e.data)); } catch(err) { console.error(err); }
};
evtSource.onerror = () => {
  console.log('SSE connection lost, reconnecting...');
};
</script>
</body>
</html>`
