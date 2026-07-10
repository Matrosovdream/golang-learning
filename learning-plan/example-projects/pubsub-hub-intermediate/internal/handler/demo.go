package handler

// demoPage is a single self-contained HTML page (no external assets). It uses
// the browser's built-in EventSource to open an SSE subscription and a small
// form to POST publishes, so the fan-out broadcast is visible live. A raw
// string literal (backtick-quoted) keeps the HTML readable without escaping.
const demoPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>PubSub Hub — demo</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 640px; margin: 2rem auto; padding: 0 1rem; }
  h1 { font-size: 1.3rem; }
  input, button { font-size: 1rem; padding: .4rem .6rem; }
  #log { background: #111; color: #0f0; padding: .8rem; height: 260px; overflow-y: auto;
         font-family: ui-monospace, monospace; font-size: .85rem; border-radius: 6px; }
  .row { display: flex; gap: .5rem; margin: .6rem 0; }
  .muted { color: #888; }
</style>
</head>
<body>
<h1>PubSub Hub</h1>
<p class="muted">Subscribe to a topic, then publish to it. Open this page in two
tabs (or add subscribers with <code>curl -N</code>) to watch one publish fan out.</p>

<div class="row">
  <input id="topic" value="demo" placeholder="topic">
  <button id="sub">Subscribe</button>
</div>
<div class="row">
  <input id="msg" value="hello" placeholder="message" style="flex:1">
  <button id="pub">Publish</button>
</div>

<div id="log"></div>

<script>
  var es = null;
  var log = document.getElementById('log');
  function line(t) { log.innerHTML += t + '\n'; log.scrollTop = log.scrollHeight; }

  document.getElementById('sub').onclick = function () {
    var topic = document.getElementById('topic').value;
    if (es) { es.close(); }
    // EventSource is the browser's native SSE client; it reconnects on its own.
    es = new EventSource('/topics/' + encodeURIComponent(topic) + '/subscribe');
    es.onopen = function () { line('[subscribed to ' + topic + ']'); };
    es.onmessage = function (e) { line('recv: ' + e.data); };
    es.onerror = function () { line('[connection error]'); };
  };

  document.getElementById('pub').onclick = function () {
    var topic = document.getElementById('topic').value;
    var body = document.getElementById('msg').value;
    fetch('/topics/' + encodeURIComponent(topic) + '/publish', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: body })
    }).then(function (r) { line('[published -> ' + r.status + ']'); });
  };
</script>
</body>
</html>`
