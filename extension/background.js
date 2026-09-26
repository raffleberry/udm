// udm — background page.
//
// The extension has exactly two jobs:
//   1. notice that a response is really a file download, cancel it so the
//      browser does not also fetch it, and hand the URL to the udm app over
//      http on loopback;
//   2. fall back to the native messaging host when nothing is listening, which
//      is how the app gets started lazily.
//
// The popup talks to this page over runtime.sendMessage.

'use strict';

// Must match internal/cfg.Port and settings.json.
const API = 'http://127.0.0.1:33210';

// The host exists only to start the app; it is never on the download path once
// the app is already running.
const HOST = 'raffleberry.udm';

// Urls the user has told us to leave alone, for this browser session.
const bypassed = new Set();

// The master switch, toggled from the popup.
let capture = true;

/* ------------------------------------------------------------------ the app */

// hand posts a download to udm. It resolves either way; callers decide whether
// to complain.
async function hand(req) {
  let res;
  try {
    res = await fetch(API + '/api/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
  } catch (e) {
    // Nothing is listening. That is the one case the native host is for.
    return viaHost(req);
  }

  const body = await res.json().catch(() => ({}));
  return res.ok
    ? { ok: true }
    : { ok: false, error: body.error || res.status };
}

// viaHost asks the native host to make sure udm is running, then hands the
// download over. The host starts the binary and waits for the port to answer
// before it replies.
function viaHost(req) {
  return new Promise((resolve) => {
    let port;
    try {
      port = browser.runtime.connectNative(HOST);
    } catch (e) {
      resolve({ ok: false, error: 'udm host not installed — run: just install-host' });
      return;
    }

    let settled = false;
    const finish = (v) => {
      if (settled) return;
      settled = true;
      try { port.disconnect(); } catch (e) { /* already gone */ }
      resolve(v);
    };

    port.onMessage.addListener((msg) => {
      if (!msg || msg.action === 'pong') return; // ignore the ping reply
      finish(msg.ok
        ? { ok: true, launched: !!msg.launched }
        : { ok: false, error: msg.error || 'udm refused the download' });
    });

    port.onDisconnect.addListener(() =>
      finish({ ok: false, error: 'the udm native host went away' })
    );

    port.postMessage({
      action: 'download',
      data: req.url,
      name: req.name || '',
      ref: req.referrer || '',
    });
  });
}

const running = async () => {
  try {
    const res = await fetch(API + '/api/ping');
    return res.ok ? res.json() : null;
  } catch (e) {
    return null;
  }
};

/* ------------------------------------------------------------- interception */

// isDownload decides whether a response is a file rather than a page. This is
// deliberately conservative: guessing wrong means breaking a working website.
function isDownload(headers) {
  const get = (n) => {
    const h = headers.find((x) => x.name.toLowerCase() === n);
    return h ? h.value : '';
  };

  if ((get('content-disposition') || '').toLowerCase().includes('attachment')) return true;

  // Some servers stream a file with no content-disposition at all. Only trust
  // that for types a browser would never render inline.
  const ct = get('content-type').split(';')[0].trim().toLowerCase();
  return /^(application|video|audio)\/(octet-stream|pdf|zip|x-7z-compressed|x-rar-compressed|msdownload|x-bittorrent|mp4|webm|mkv)$/.test(ct);
}

// nameFrom pulls a filename out of Content-Disposition, then out of the URL.
function nameFrom(cd, url) {
  const star = /filename\*\s*=\s*(?:UTF-8|)[^']*'[^']*'([^;]+)/i.exec(cd);
  const plain = /filename\s*=\s*"?([^";]+)"?/i.exec(cd);
  if (star) return safeDecode(star[1].trim());
  if (plain) return safeDecode(plain[1].trim());
  try {
    return safeDecode(new URL(url).pathname.split('/').pop() || '');
  } catch (e) {
    return '';
  }
}

function safeDecode(s) {
  try {
    return decodeURIComponent(s);
  } catch (e) {
    return s;
  }
}

// This is the only interception point. Cancelling in onBeforeRequest instead
// would mean cancelling every GET in the browser, pages and assets included,
// so the decision waits until the response headers say what the body is.
browser.webRequest.onHeadersReceived.addListener(
  (req) => {
    if (!capture || req.statusCode >= 400 || bypassed.has(req.url)) return {};
    if (!isDownload(req.responseHeaders)) return {};

    const cd = (req.responseHeaders.find((h) => h.name.toLowerCase() === 'content-disposition') || {}).value || '';

    // The browser blocks this listener, so it cannot wait on a fetch. The
    // request is cancelled either way; the hand-over is fire-and-forget and
    // the popup reports a failure if it matters.
    hand({
      url: req.url,
      name: nameFrom(cd, req.url),
      referrer: req.originUrl || req.documentUrl || '',
    }).then(complain);

    return { cancel: true };
  },
  { urls: ['<all_urls>'] },
  ['blocking']
);

/* ------------------------------------------------------------- context menu */

const nameOf = (url) => {
  try {
    return safeDecode(new URL(url).pathname.split('/').pop() || '');
  } catch (e) {
    return '';
  }
};

const MENU = { id: 'udm-download', title: 'Download with udm', contexts: ['link', 'image', 'video', 'audio'] };
const BYPASS = { id: 'udm-bypass', title: 'Let the browser handle this one (this page)', contexts: ['link', 'image', 'video', 'audio'] };

browser.contextMenus.create(MENU);
browser.contextMenus.create(BYPASS);

browser.contextMenus.onClicked.addListener(async (info, tab) => {
  const url = info.linkUrl || info.mediaLinkUrl || info.srcUrl;

  if (info.menuItemId === 'udm-download') {
    complain(await hand({ url, name: nameOf(url), referrer: (tab && tab.url) || '' }));
    return;
  }

  if (info.menuItemId === 'udm-bypass') {
    // The in-flight request was already cancelled, so this takes effect from
    // the next attempt onwards. Saying so beats pretending it worked.
    bypassed.add(url);
    await browser.storage.local.set({ bypassed: [...bypassed] });
    browser.notifications.create({
      type: 'basic',
      iconUrl: browser.runtime.getURL('icon/udm-96.png'),
      title: 'udm',
      message: 'udm will leave this one to the browser from now on.',
    });
  }
});

/* ------------------------------------------------------------- popup bridge */

browser.runtime.onMessage.addListener((msg, _sender, respond) => {
  switch (msg && msg.type) {
    case 'status':
      running().then((s) =>
        respond({ running: !!s, version: s && s.version, capture, bypassed: bypassed.size })
      );
      return true;

    case 'capture':
      capture = !!msg.value;
      browser.storage.local.set({ capture });
      respond({ capture });
      return true;

    case 'add':
      hand({ url: msg.url, name: nameOf(msg.url) }).then((res) => {
        respond(res);
        complain(res);
      });
      return true;

    case 'clear-bypass':
      bypassed.clear();
      browser.storage.local.remove('bypassed');
      respond({ bypassed: 0 });
      return true;

    default:
      respond({ error: 'unknown message' });
  }
});

// complain surfaces a failure once. Downloads that work are silent.
function complain(res) {
  if (!res || res.ok) return;
  browser.notifications.create({
    type: 'basic',
    iconUrl: browser.runtime.getURL('icon/udm-96.png'),
    title: 'udm',
    message: res.error || 'the download could not be handed over',
  });
}

/* ------------------------------------------------------------------ startup */

// The background page is persistent, so this runs once per browser start.
browser.storage.local
  .get(['capture', 'bypassed'])
  .then(({ capture: c, bypassed: b }) => {
    if (typeof c === 'boolean') capture = c;
    (b || []).forEach((u) => bypassed.add(u));
  })
  .catch(() => { /* first run */ });
