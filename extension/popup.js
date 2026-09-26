// udm popup: a status light, the capture switch, and a manual add box.

'use strict';

const $ = (id) => document.getElementById(id);

const ask = (msg) => browser.runtime.sendMessage(msg);

function paint(s) {
  $('dot').className = 'dot ' + (s.running ? 'on' : 'off');
  $('state').textContent = s.running
    ? `udm is running${s.version ? ' — v' + s.version : ''}`
    : 'udm is not running — it starts on the first download';

  $('capture').checked = s.capture;
  $('clear').hidden = !s.bypassed;
  $('clear').textContent = `clear ${s.bypassed} bypassed link(s)`;
}

function refresh() {
  ask({ type: 'status' }).then(paint).catch((e) => {
    $('state').textContent = 'the background page is not responding: ' + e.message;
  });
}

$('capture').addEventListener('change', (e) => {
  ask({ type: 'capture', value: e.target.checked });
  $('msg').textContent = e.target.checked
    ? 'downloads will go to udm'
    : 'udm is out of the way — the browser will handle downloads';
});

$('addform').addEventListener('submit', async (e) => {
  e.preventDefault();
  const url = $('url').value.trim();
  if (!url) return;
  $('msg').textContent = 'sending…';
  const res = await ask({ type: 'add', url });
  $('msg').textContent = res && res.ok ? 'handed over to udm' : (res && res.error) || 'failed';
  if (res && res.ok) $('url').value = '';
  refresh();
});

$('clear').addEventListener('click', async () => {
  await ask({ type: 'clear-bypass' });
  refresh();
});

refresh();
// Keep the light honest while the popup is open.
setInterval(refresh, 2000);
