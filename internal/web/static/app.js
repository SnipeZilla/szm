/* SnipeZilla Server Manager — web interface. No framework, no build step: this file is served as it is. */
'use strict';
(function () {

/* =================================================================================================================
 * Helpers
 * =============================================================================================================== */

const SVGNS = 'http://www.w3.org/2000/svg';
const ICONS = {
  play: 'M7 4.5v15l12.5-7.5z',
  stop: 'M6.5 6.5h11v11h-11z',
  restart: 'M3 12a9 9 0 1 0 2.6-6.4L3 8M3 3v5h5',
  update: 'M12 3v12M7 10l5 5 5-5M5 21h14',
  shield: 'M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6zM9 12l2 2 4-4',
  terminal: 'M4 17l6-5-6-5M12 19h8',
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z',
  folderplus: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zM12 10.5v5M9.5 13h5',
  file: 'M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5',
  fileplus: 'M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5M12 11v6M9 14h6',
  filetext: 'M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5M9 13h6M9 17h6',
  map: 'M9 4L3 6v14l6-2 6 2 6-2V4l-6 2zM9 4v14M15 6v14',
  sliders: 'M4 6h9M17 6h3M15 4v4M4 12h3M11 12h9M9 10v4M4 18h11M19 18h1M17 16v4',
  list: 'M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01',
  users: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM22 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8',
  user: 'M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
  plus: 'M12 5v14M5 12h14',
  trash: 'M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14M10 11v5M14 11v5',
  x: 'M18 6L6 18M6 6l12 12',
  check: 'M5 12l5 5L20 7',
  search: 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM21 21l-4.3-4.3',
  cpu: 'M9 3v2M15 3v2M9 19v2M15 19v2M3 9h2M3 15h2M19 9h2M19 15h2M7 5h10a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2zM9.5 9.5h5v5h-5z',
  memory: 'M4 7h16a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1zM7 10v4M11 10v4M15 10v4M6 17v3M18 17v3',
  disk: 'M22 12H2M5.5 5h13l3.5 7v6a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-6zM6 16h.01M10 16h.01',
  upload: 'M12 21V9M7 14l5-5 5 5M5 3h14',
  download: 'M12 3v12M7 10l5 5 5-5M5 21h14',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z',
  save: 'M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2zM17 21v-8H7v8M7 3v5h8',
  refresh: 'M21 12a9 9 0 1 1-2.6-6.4L21 8M21 3v5h-5',
  back: 'M19 12H5M12 19l-7-7 7-7',
  up: 'M12 19V5M5 12l7-7 7 7',
  chevron: 'M9 18l6-6-6-6',
  server: 'M4 4h16v6H4zM4 14h16v6H4zM8 7h.01M8 17h.01',
  logout: 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9',
  key: 'M15.5 7.5l3 3L22 7l-3-3M11.4 12.6a5.5 5.5 0 1 1-7.8 7.8 5.5 5.5 0 0 1 7.8-7.8zM11.4 12.6L19 5M21 3l-2 2',
  copy: 'M9 9h11v11H9zM5 15H4V4h11v1',
  scissors: 'M6 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM20 4L8.1 15.9M14.5 14.5L20 20M8.1 8.1L12 12',
  eye: 'M1 12s4-8 11-8 11 8 11 8-4 8-11 8S1 12 1 12zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
  bolt: 'M13 2L3 14h9l-1 8 10-12h-9z',
  alert: 'M12 9v4M12 17h.01M10.3 3.9L1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z',
  info: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 16v-4M12 8h.01',
  clock: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 6v6l4 2',
  grip: 'M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01',
  chat: 'M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z',
  cloud: 'M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z',
  layers: 'M12 2L2 7l10 5 10-5zM2 17l10 5 10-5M2 12l10 5 10-5',
  globe: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM2 12h20M12 2a15 15 0 0 1 0 20 15 15 0 0 1 0-20',
  external: 'M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6M15 3h6v6M10 14L21 3',
  dice: 'M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM8 8h.01M16 16h.01M12 12h.01',
  power: 'M18.4 6.6a9 9 0 1 1-12.8 0M12 2v10',
  gauge: 'M12 14l4-4M3.3 17a10 10 0 1 1 17.4 0',
  arrowdown: 'M12 5v14M19 12l-7 7-7-7',
  arrowright: 'M5 12h14M12 5l7 7-7 7',
  arrowleft: 'M19 12H5M12 19l-7-7 7-7',
  columns: 'M4 4h16v16H4zM12 4v16',
  pause: 'M7 5h3v14H7zM14 5h3v14h-3z',
  package: 'M21 8l-9-5-9 5 9 5 9-5zM3 8v8l9 5 9-5V8M12 13v8M7.5 5.5l9 5',
  wrench: 'M14.7 6.3a4 4 0 0 0 5 5L22 14l-8 8-2.3-2.3a4 4 0 0 0-5-5L4.4 12.4a4 4 0 0 1 5-5zM2 22l5.5-5.5',
};

function icon(name, cls) {
  const s = document.createElementNS(SVGNS, 'svg');
  s.setAttribute('viewBox', '0 0 24 24');
  s.setAttribute('fill', name === 'play' ? 'currentColor' : 'none');
  s.setAttribute('stroke', 'currentColor');
  s.setAttribute('stroke-width', '2');
  s.setAttribute('stroke-linecap', 'round');
  s.setAttribute('stroke-linejoin', 'round');
  s.setAttribute('aria-hidden', 'true');
  if (cls) s.setAttribute('class', cls);
  const p = document.createElementNS(SVGNS, 'path');
  p.setAttribute('d', ICONS[name] || ICONS.info);
  s.append(p);
  return s;
}

const PROPS = new Set(['value', 'checked', 'disabled', 'selected', 'indeterminate', 'multiple', 'readOnly', 'spellcheck', 'draggable']);

function h(tag, props, ...kids) {
  const el = document.createElement(tag);
  if (props) {
    for (const k in props) {
      const v = props[k];
      if (v == null || v === false && !PROPS.has(k)) continue;
      if (k === 'class') el.className = v;
      else if (k === 'title') el.setAttribute('data-tip', v); // shown by the tooltip below, not the browser's
      else if (k === 'style') el.style.cssText = v;
      else if (k.length > 2 && k[0] === 'o' && k[1] === 'n') el.addEventListener(k.slice(2).toLowerCase(), v);
      else if (PROPS.has(k)) el[k] = v;
      else el.setAttribute(k, v === true ? '' : v);
    }
  }
  append(el, kids);
  return el;
}

function append(el, ...kids) {
  for (const k of kids.flat(Infinity)) {
    if (k == null || k === false) continue;
    el.append(k instanceof Node ? k : String(k));
  }
  return el;
}

function clear(el) { while (el.firstChild) el.removeChild(el.firstChild); return el; }

// morph makes the element a look like b, changing only what differs, so what is under the mouse (a tooltip, a
// hover) stays. Elements with a different data-key are replaced, never reused (a Stop button is not a Start one).
function morph(a, b) {
  if (a.nodeType !== b.nodeType || a.nodeName !== b.nodeName ||
      (a.nodeType === 1 && a.getAttribute('data-key') !== b.getAttribute('data-key'))) {
    a.replaceWith(b);
    return b;
  }
  if (a.nodeType === 3) {
    if (a.nodeValue !== b.nodeValue) a.nodeValue = b.nodeValue;
    return a;
  }
  if (a.nodeType !== 1) { a.replaceWith(b); return b; }
  for (const at of [...a.attributes]) if (!b.hasAttribute(at.name)) a.removeAttribute(at.name);
  for (const at of [...b.attributes]) if (a.getAttribute(at.name) !== at.value) a.setAttribute(at.name, at.value);
  patchChildren(a, [...b.childNodes]);
  return a;
}

function patchChildren(el, kids) {
  const old = [...el.childNodes];
  kids.forEach((k, i) => { if (i < old.length) morph(old[i], k); else el.appendChild(k); });
  for (let i = kids.length; i < old.length; i++) old[i].remove();
}

// patch is swap that keeps what did not change.
function patch(el, ...kids) {
  const tmp = document.createElement('div');
  append(tmp, kids);
  patchChildren(el, [...tmp.childNodes]);
  return el;
}
function swap(el, ...kids) { clear(el); return append(el, kids); }
const $ = (s, el) => (el || document).querySelector(s);

function fmtBytes(n) {
  if (!n) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1)) + ' ' + u[i];
}

// fmtRAM: a machine's memory as sold (32 GB), not as the system counts it (31.0 GiB: firmware and the kernel keep
// a little before the system sees it)
function fmtRAM(n) {
  const gb = n / 1073741824;
  return gb >= 2 ? Math.ceil(gb - 0.02) + ' GB' : fmtBytes(n);
}

function fmtDur(sec) {
  sec = Math.max(0, Math.round(sec));
  if (sec < 60) return sec + 's';
  const m = Math.floor(sec / 60), hh = Math.floor(m / 60), d = Math.floor(hh / 24);
  if (m < 60) return m + 'm ' + String(sec % 60).padStart(2, '0') + 's';
  if (hh < 48) return hh + 'h ' + String(m % 60).padStart(2, '0') + 'm';
  return d + 'd ' + (hh % 24) + 'h';
}

const now = () => Date.now() / 1000;

// diskOf: the disk of the page's machine a folder is on: E:\\ for E:\\Servers\\cs2, /srv/games for /srv/games/cs2
function diskOf(dir) {
  const win = /^[A-Za-z]:|^\\\\/.test(dir || '');
  const norm = p => win ? p.replace(/\//g, '\\').toLowerCase() : p;
  const path = norm(dir || '');
  let best = null;
  for (const d of MI().disks || []) {
    const root = norm(d.path);
    const inside = path === root.replace(/[\\/]$/, '') || path.startsWith(root.endsWith('/') || root.endsWith('\\') ? root : root + (win ? '\\' : '/'));
    if (inside && (!best || root.length > norm(best.path).length)) best = d;
  }
  return best;
}

// Versions of the managers: one older than the newest among the machines (hub and nodes) shows in red, to update.
// cmpVer: -1, 0, 1, by numbers (2.0.10 after 2.0.9); "dev" or a missing number counts as older.
function cmpVer(a, b) {
  const pa = String(a || '').split(/[.-]/), pb = String(b || '').split(/[.-]/);
  for (let k = 0; k < Math.max(pa.length, pb.length); k++) {
    const x = parseInt(pa[k], 10), y = parseInt(pb[k], 10);
    const nx = isNaN(x) ? -1 : x, ny = isNaN(y) ? -1 : y;
    if (nx !== ny) return nx < ny ? -1 : 1;
  }
  return 0;
}
// newestVersion: the highest version of the managers on the page, and a machine that has it
function newestVersion() {
  let best = null, where = '';
  const see = (v, name) => { if (v && (!best || cmpVer(v, best) > 0)) { best = v; where = name; } };
  const local = (S.machines || []).find(mc => mc.local);
  see((S.machine || {}).version, local ? local.name : 'this manager');
  for (const mc of S.machines || []) if (!mc.local) see(mc.version, mc.name);
  return { version: best, where };
}
// verTag: "v2.0.0", red with why when another machine runs a newer one
function verTag(v) {
  if (!v) return null;
  const n = newestVersion();
  const old = S.machines && n.version && cmpVer(v, n.version) < 0;
  return h('span', { class: 'small ' + (old ? 'verold' : 'muted'), title: old ? `Out of date: v${v}, while ${n.where} runs v${n.version}. Update its szm.` : 'The version of its manager (szm)' }, 'v' + v);
}

// fmtBits: a network rate (bytes per second) in bits per second, as bandwidth is sold: 850 kb/s, 12.4 Mb/s.
function fmtBits(bytes) {
  const b = (bytes || 0) * 8;
  if (b < 1e6) return (b < 1e4 ? (b / 1e3).toFixed(1) : Math.round(b / 1e3)) + ' kb/s';
  if (b < 1e9) return (b < 1e8 ? (b / 1e6).toFixed(1) : Math.round(b / 1e6)) + ' Mb/s';
  return (b / 1e9).toFixed(2) + ' Gb/s';
}

// fmtPct: 0.4 %, 12 %: a decimal only when small.
function fmtPct(p) { return (p < 10 ? p.toFixed(1) : Math.round(p)) + '%'; }

function timeAgo(t) {
  const s = now() - t;
  if (s < 45) return 'just now';
  if (s < 3600) return Math.round(s / 60) + ' min ago';
  if (s < 86400) return Math.round(s / 3600) + ' h ago';
  return Math.round(s / 86400) + ' d ago';
}

function fmtTime(iso, withDate) {
  const d = new Date(iso);
  const today = new Date().toDateString() === d.toDateString();
  const t = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  if (today && !withDate) return t;
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' }) + ' ' + t;
}

function debounce(fn, ms) {
  let t;
  return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); };
}

const store = {
  get(k, d) { try { const v = localStorage.getItem('szm.' + k); return v == null ? d : JSON.parse(v); } catch (e) { return d; } },
  set(k, v) { try { localStorage.setItem('szm.' + k, JSON.stringify(v)); } catch (e) { /* private window */ } },
};

function copyText(t) {
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(t).then(() => toast('Copied.', 'ok'), () => toast('Could not copy.', 'bad'));
    return;
  }
  const ta = h('textarea', { style: 'position:fixed;opacity:0' }, t);
  document.body.append(ta);
  ta.select();
  try { document.execCommand('copy'); toast('Copied.', 'ok'); } catch (e) { toast('Could not copy.', 'bad'); }
  ta.remove();
}

/* =================================================================================================================
 * API
 * =============================================================================================================== */

async function api(method, url, body) {
  const init = { method, headers: { 'X-SZM': '1' }, credentials: 'same-origin' };
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  url = machineURL(url);
  let r;
  try {
    r = await fetch(url, init);
  } catch (e) {
    throw new Error('The manager does not answer. Is it running?');
  }
  let data = null;
  try { data = await r.json(); } catch (e) { /* empty */ }
  if (r.status === 401 && url !== 'api/login') {
    showLogin();
    throw new Error('Please sign in.');
  }
  if (!r.ok) {
    let msg = data && data.error;
    if (!msg && r.status === 405) msg = 'The web server in front of the manager refused this ' + method + ' (405 Method Not Allowed). Behind IIS: the WebDAV feature takes the saves; remove WebDAV Publishing (see INSTALL.md, IIS step 5).';
    if (!msg) msg = 'Error ' + r.status + (r.statusText ? ' ' + r.statusText : '') + ' from ' + method + ' ' + url;
    const err = new Error(msg);
    err.status = r.status;
    err.data = data;
    throw err;
  }
  return data;
}

// Shows the error of a failed call
async function run(fn, okText) {
  try {
    const r = await fn();
    if (okText) toast(okText, 'ok');
    return r;
  } catch (e) {
    if (e.message !== 'Please sign in.') toast(e.message, 'bad');
    return undefined;
  }
}

/* =================================================================================================================
 * State
 * =============================================================================================================== */

const S = {
  polling: false, user: null, servers: [], machine: null, games: [], gamesBy: {}, events: [], rights: {}, live: false,
  machines: null, // with linked machines: this one first (id ''), then the others
  ctxMachine: '', // the linked machine the page works on ('' this one)
};
const can = r => !!S.user && S.user.rights.includes(r);
const serverBy = id => S.servers.find(s => s.id === id);
// Machines: the linked ones' servers have IDs "<machine>~<server>"
const machineBy = id => (S.machines || []).find(m => m.id === (id || ''));
const machineName = id => { const m = machineBy(id); return m ? m.name : id; };
const machineOnline = id => { const m = machineBy(id); return !id || !m || m.online; };
// MI: the machine (RAM, IPs, steamcmd…) of the page's machine
const MI = () => S.ctxMachine ? ((machineBy(S.ctxMachine) || {}).machine || {}) : (S.machine || {});
// sid: a server ID of the page's machine, as this manager knows it ("us~css-1")
const sid = id => S.ctxMachine ? S.ctxMachine + '~' + id : id;
// machineServers: the servers of the page's machine, with its own IDs
const machineServers = () => S.servers.filter(v => (v.machine || '') === S.ctxMachine)
  .map(v => S.ctxMachine ? Object.assign({}, v, { id: v.id.slice(S.ctxMachine.length + 1) }) : v);
const MACHINE_PAGES = ['settings', 'games', 'addons', 'backups', 'cache', 'new'];
// machineBar: on a machine's page (settings, addons…), which machine
// A hub with no servers of its own (on a web server, only for the page): left out of the machine lists, but for
// its Settings (its users, Discord, forums, machines) and the New server page (to give it one after all).
const hubOnly = mc => !!(mc && mc.local && !mc.servers);
const machinesFor = page => (S.machines || []).filter(mc => !hubOnly(mc) || page === 'settings' || page === 'new');
// the machine of a machine page: the one picked last, a hub with no servers only on the page it was picked on
// (Settings by default)
function pickedMachine(page) {
  const list = machinesFor(page);
  if (store.get('pickHubOn', 'settings') === page && list.some(hubOnly)) return '';
  const pm = store.get('pickMachine', '');
  const m = list.find(mc => mc.id === pm && !hubOnly(mc)) || list.find(mc => !hubOnly(mc));
  return m && !m.local ? m.id : '';
}
function machineBar() {
  if (!S.machines) return null;
  const cur = S.ctxMachine;
  const m = machineBy(cur);
  const page = S.page;
  const pick = mc => {
    if (hubOnly(mc)) store.set('pickHubOn', page);
    else { store.set('pickMachine', mc.id); store.set('pickHubOn', ''); }
    route();
  };
  return h('div', { class: 'machinebar' },
    h('span', { class: 'muted small' }, 'Machine'),
    h('div', { class: 'seg' }, machinesFor(page).map(mc => h('button', { type: 'button', class: (mc.id === cur ? 'on' : '') + (mc.online ? '' : ' off'),
      title: mc.online ? (hubOnly(mc) ? 'This hub: it has no servers of its own' : '') : mc.name + ' is not connected', onclick: () => pick(mc) },
    h('span', { class: 'dot ' + (mc.online ? 'ok' : 'bad') }), mc.name, hubOnly(mc) ? h('span', { class: 'muted small' }, ' (hub)') : null))),
    m && !m.local && !m.online ? h('span', { class: 'note warn small' }, icon('alert'), ' ' + m.name + ' is not connected: its pages can not be reached now.') : null);
}

// machineURL: a call of the page goes to its machine (through this manager), except what is this manager's own
function machineURL(url) {
  if (!S.ctxMachine || !url.startsWith('api/') || url.includes('~')) return url;
  const rest = url.slice(4);
  if (/^(state|stream|logs|users|machines|login|logout|password|hello|setup|paste$|paste\/jobs\/|m\/)/.test(rest)) return url;
  return 'api/m/' + encodeURIComponent(S.ctxMachine) + '/' + rest;
}
const humans = v => v.info ? Math.max(0, v.info.players - v.info.bots) : 0;

const GAME_COLORS = {
  cs2: ['#ffb02e', '#e8590c'], css: ['#5b8cff', '#3048c9'], cs16: ['#c49a52', '#7a5520'], czero: ['#a77b4f', '#5d3b1c'],
  tf2: ['#e0603d', '#a82424'], l4d: ['#b33', '#5e1010'], l4d2: ['#d9443f', '#7a1717'], gmod: ['#3fa2ff', '#1557b0'],
  hl2dm: ['#ff9c3a', '#c25200'], hldms: ['#ffa94d', '#b34700'], hldm: ['#f08c00', '#a34a00'], dods: ['#8c9a4b', '#4b5520'],
  dod: ['#7d8c45', '#3f4719'], tfc: ['#6c8ea3', '#34495a'], dmc: ['#8a7f9e', '#463c5c'], ricochet: ['#2bb3c0', '#106a73'],
  insurgency: ['#a08a5c', '#5a4a26'], nmrih: ['#9b4dca', '#4a1d6e'],
  rust: ['#cd412b', '#6e1c10'], sdtd: ['#b8442c', '#4d1b10'], valheim: ['#5f8fb4', '#26445e'], ark: ['#3c9a8f', '#18493f'],
  palworld: ['#4fb4e8', '#1f5f8c'], pz: ['#8f2f2f', '#3d1212'], sandstorm: ['#c9a25b', '#6b5126'], squad: ['#7b8a52', '#3a4222'],
  unturned: ['#7ac24a', '#3a6a1c'], conan: ['#b3863c', '#5c3f12'], vrising: ['#b02646', '#4e0f1e'], enshrouded: ['#5ab08f', '#235743'],
  satisfactory: ['#f59f2b', '#a85a00'], kf2: ['#9e1f1f', '#3f0b0b'], dayz: ['#6f7b5a', '#2f3524'], bo3: ['#e4752a', '#7a3108'],
  arma2oa: ['#8a7d4e', '#3b351f'], arma3: ['#5d7a3c', '#25321a'],
};
const BADGE_TEXT = { cs16: '1.6', css: 'CSS', czero: 'CZ', dods: 'DoDS', hldms: 'HLS', ricochet: 'RIC', nmrih: 'NMR', insurgency: 'INS', gmod: 'GM', hl2dm: 'HL2', hldm: 'HL',
  rust: 'RUST', sdtd: '7DTD', valheim: 'VH', ark: 'ARK', palworld: 'PAL', pz: 'PZ', sandstorm: 'INS', squad: 'SQD', unturned: 'UNT',
  conan: 'CE', vrising: 'VR', enshrouded: 'ENS', satisfactory: 'SF', kf2: 'KF2', dayz: 'DZ', bo3: 'BO3', arma2oa: 'A2', arma3: 'A3' };

function badge(gameKey, size) {
  const g = S.gamesBy[gameKey];
  let c = GAME_COLORS[gameKey];
  if (!c) {
    let hsh = 0;
    for (const ch of gameKey || '?') hsh = (hsh * 31 + ch.charCodeAt(0)) % 360;
    c = [`hsl(${hsh} 70% 58%)`, `hsl(${(hsh + 30) % 360} 65% 35%)`];
  }
  if (g && g.icon) {
    return h('div', { class: 'badge icon' + (size ? ' ' + size : ''), title: g.name }, h('img', { src: 'games/' + g.icon + '.png', alt: g.short }));
  }
  const text = BADGE_TEXT[gameKey] || (g ? g.short.replace(/[^A-Za-z0-9.]/g, '').slice(0, 4) : '?');
  return h('div', { class: 'badge' + (size ? ' ' + size : ''), style: `background:linear-gradient(135deg,${c[0]},${c[1]})`, title: g ? g.name : gameKey }, text);
}

const BUSY = { install: 'Installing', update: 'Updating', restart: 'Restarting', stop: 'Stopping', start: 'Starting', addons: 'Updating addons', restore: 'Restoring a backup' };

function statusInfo(v) {
  if (v.machine && !machineOnline(v.machine)) return { cls: 'offline', text: machineName(v.machine) + ' offline', tip: 'Its machine is not connected to this hub: this is what it said last. Its manager goes on by itself.' };
  if (v.busy && v.busy !== 'start') return { cls: 'busy', text: BUSY[v.busy] || v.busy, spin: true };
  switch (v.status) {
    case 'running': return v.update ? { cls: 'running update', text: 'Running', tip: 'Running. Steam has an update for it' + (v.autoupdate ? ': it installs by itself, warning the players first.' : ': click Update.') } : { cls: 'running', text: 'Running' };
    case 'starting': return { cls: 'starting', text: 'Starting', spin: true };
    case 'stopping': return { cls: 'stopping', text: 'Stopping', spin: true };
    case 'crashed': return { cls: 'crashed', text: 'Crashed' };
    case 'failed': return v.start_map_gone ? { cls: 'failed', text: 'Can not start', tip: 'Its start map ' + v.start_map_gone + ' is not on this server' } : v.cant_start ? { cls: 'failed', text: 'Can not start', tip: v.cant_start } : { cls: 'failed', text: 'Gave up' };
    case 'not_installed': return { cls: 'not_installed', text: 'Not installed' };
    default: return { cls: 'stopped', text: 'Stopped' };
  }
}

function pill(v) {
  const st = statusInfo(v);
  return h('span', { class: 'pill ' + st.cls, title: st.tip }, st.spin ? h('i', { class: 'spin' }) : h('i'), st.text);
}

// up: it runs (as far as known: a server of a machine that is not connected counts as not up)
const isUp = v => v.pid > 0 && machineOnline(v.machine);

/* =================================================================================================================
 * Toasts, modals
 * =============================================================================================================== */

function toast(text, kind) {
  kind = kind || 'info';
  const el = h('div', { class: 'toast ' + kind, role: kind === 'bad' ? 'alert' : 'status' },
    icon(kind === 'ok' ? 'check' : kind === 'bad' ? 'alert' : 'info'), h('div', { class: 'grow' }, text));
  $('#toasts').append(el);
  setTimeout(() => { el.style.transition = 'opacity .3s'; el.style.opacity = '0'; setTimeout(() => el.remove(), 300); }, kind === 'bad' ? 7000 : 3500);
}

// modal opens a dialog; close() closes it. Escape and a click outside close it too (unless locked).
function modal(opts) {
  const back = h('div', { class: 'modal-back' });
  const box = h('div', { class: 'modal ' + (opts.size || ''), role: 'dialog', 'aria-modal': 'true' });
  const close = () => {
    document.removeEventListener('keydown', onKey, true);
    back.remove();
    if (opts.onClose) opts.onClose();
  };
  const onKey = e => {
    if (e.key === 'Escape' && !opts.locked) { e.stopPropagation(); close(); }
  };
  const hd = h('div', { class: 'hd' }, h('h2', { class: 'grow' }, opts.title),
    h('button', { class: 'btn ghost icon sm', title: 'Close', onclick: close }, icon('x')));
  box.append(hd);
  const bd = h('div', { class: 'bd' });
  box.append(bd);
  let ft = null;
  if (opts.buttons !== false) {
    ft = h('div', { class: 'ft' });
    box.append(ft);
  }
  back.append(box);
  back.addEventListener('mousedown', e => { if (e.target === back && !opts.locked) close(); });
  document.addEventListener('keydown', onKey, true);
  document.body.append(back);
  return { back, box, bd, ft, hd, close };
}

function confirmBox(title, text, okLabel, danger) {
  return new Promise(resolve => {
    let done = false;
    const m = modal({ title, onClose: () => { if (!done) resolve(false); } });
    append(m.bd, typeof text === 'string' ? h('p', { style: 'margin:0;color:var(--text-2)' }, text) : text);
    const ok = h('button', { class: 'btn ' + (danger ? 'danger' : 'primary'), onclick: () => { done = true; m.close(); resolve(true); } }, okLabel || 'OK');
    append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'), ok);
    setTimeout(() => ok.focus(), 30);
  });
}

function promptBox(title, label, value, okLabel) {
  return new Promise(resolve => {
    let done = false;
    const m = modal({ title, onClose: () => { if (!done) resolve(null); } });
    const inp = h('input', { type: 'text', value: value || '', spellcheck: false });
    const submit = e => {
      if (e) e.preventDefault();
      if (!inp.value.trim()) return;
      done = true;
      m.close();
      resolve(inp.value.trim());
    };
    append(m.bd, h('form', { onsubmit: submit }, h('label', { class: 'field' }, h('span', null, label), inp)));
    append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'),
      h('button', { class: 'btn primary', onclick: submit }, okLabel || 'OK'));
    setTimeout(() => { inp.focus(); inp.select(); }, 30);
  });
}

/* =================================================================================================================
 * Sign in
 * =============================================================================================================== */

let stream = null;

function logoImg(cls) { return h('img', { src: 'logo.svg', class: cls || 'logo', alt: '' }); }

function showLogin() {
  if (stream) { stream.close(); stream = null; }
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null; S.polling = false; }
  S.user = null;
  api('GET', 'api/hello').then(hello => {
    if (hello.user) { S.user = hello.user; boot(); return; }
    renderLogin(hello);
  }).catch(e => renderLogin({ error: e.message }));
}

function renderLogin(hello) {
  const app = $('#app');
  const err = h('div', { class: 'err', role: 'alert' }, hello.error || '');
  const name = h('input', { type: 'text', autocomplete: 'username', spellcheck: false, required: true });
  const pw = h('input', { type: 'password', autocomplete: hello.setup ? 'new-password' : 'current-password', required: true });
  const btn = h('button', { class: 'btn primary', type: 'submit', style: 'height:40px' });
  let form;
  if (hello.setup) {
    const pw2 = h('input', { type: 'password', autocomplete: 'new-password', required: true });
    const code = h('input', { type: 'text', spellcheck: false, placeholder: 'from setup-code.txt', style: 'text-transform:uppercase' });
    btn.textContent = 'Create the admin';
    form = h('form', {
      onsubmit: async e => {
        e.preventDefault();
        err.textContent = '';
        if (pw.value !== pw2.value) { err.textContent = 'The passwords are not the same.'; return; }
        btn.disabled = true;
        try {
          const r = await api('POST', 'api/setup', { name: name.value, password: pw.value, code: code.value });
          S.user = r.user;
          boot();
        } catch (ex) { err.textContent = ex.message; btn.disabled = false; }
      },
    },
    h('label', { class: 'field' }, h('span', null, 'Admin name'), name),
    h('label', { class: 'field' }, h('span', null, 'Password'), pw, h('small', null, '8 characters or more.')),
    h('label', { class: 'field' }, h('span', null, 'Password again'), pw2),
    hello.local ? null : h('label', { class: 'field' }, h('span', null, 'Setup code'), code,
      h('small', null, 'You are not on the server itself: the code is in setup-code.txt, next to szm.exe.')),
    err, btn);
  } else {
    const remember = h('input', { type: 'checkbox', checked: true });
    btn.textContent = 'Sign in';
    form = h('form', {
      onsubmit: async e => {
        e.preventDefault();
        err.textContent = '';
        btn.disabled = true;
        try {
          const r = await api('POST', 'api/login', { name: name.value, password: pw.value, remember: remember.checked });
          S.user = r.user;
          boot();
        } catch (ex) { err.textContent = ex.message; btn.disabled = false; pw.select(); }
      },
    },
    h('label', { class: 'field' }, h('span', null, 'Name'), name),
    h('label', { class: 'field' }, h('span', null, 'Password'), pw),
    h('label', { class: 'check' }, remember, h('span', null, 'Keep me signed in on this device')),
    err, btn);
  }
  swap(app, h('div', { class: 'login' }, h('div', { class: 'panel box' },
    logoImg(),
    h('h1', null, hello.setup ? 'Welcome' : [brandName(), ' Server Manager']),
    h('div', { class: 'muted' }, hello.setup ? 'First run: create the admin of this manager.' : 'Sign in to manage your game servers.'),
    form,
    h('div', { class: 'muted small', style: 'margin-top:18px;text-align:center' }, 'v' + (hello.version || '')))));
  setTimeout(() => name.focus(), 30);
}

/* =================================================================================================================
 * Shell: top bar, navigation, live updates
 * =============================================================================================================== */

let view = null; // the page shown: { el, update(), destroy() }
const shell = {};

function boot() {
  api('GET', 'api/state').then(st => {
    applyState(st);
    S.games = st.games;
    S.gamesBy = Object.fromEntries(st.games.map(g => [g.key, g]));
    S.rights = st.rights;
    S.events = st.events || [];
    S.warn = st.warn || {};
    buildShell();
    connect();
    route();
  }).catch(e => { if (e.message !== 'Please sign in.') toast(e.message, 'bad'); });
}

// staleUI: the manager was updated while this page was open: its code is the old one
function staleUI(ui) {
  if (S.staleShown) return;
  S.staleShown = true;
  const el = h('div', { class: 'stalebar' }, icon('refresh'),
    h('span', { class: 'grow' }, h('b', null, 'The manager was updated.'), ' Reload the page to use the new version (this one is the old one).'),
    // a new address for the page: no cache on the way gives the old one back
    h('button', { class: 'btn sm primary', onclick: () => location.replace(location.pathname + '?v=' + encodeURIComponent(ui || Date.now()) + location.hash) }, 'Reload'));
  document.body.append(el);
}

function applyState(st) {
  if (st.ui) { if (!S.ui) S.ui = st.ui; else if (st.ui !== S.ui) staleUI(st.ui); }
  if (st.games) { S.games = st.games; S.gamesBy = Object.fromEntries(st.games.map(g => [g.key, g])); }
  if (st.servers) S.servers = st.servers;
  if (st.machine) S.machine = st.machine;
  if ('machines' in st) S.machines = st.machines && st.machines.length > 1 ? st.machines : null;
  if (st.user) S.user = st.user;
  if ('crashes' in st) S.crashes = st.crashes || 0;
}

function onState(st) {
  applyState(st);
  updateTop();
  if (view && view.update) view.update();
}

function onLog(ev) {
  S.events.unshift(ev);
  if (S.events.length > 300) S.events.length = 300;
  if (/^games\.json (changed|is right)/.test(ev.text)) api('GET', 'api/state').then(applyState).catch(() => {}); // the games, everywhere
  if (view && view.onEvent) view.onEvent(ev);
  if (['crash', 'failed', 'error'].includes(ev.kind)) toast((ev.name ? ev.name + ': ' : '') + ev.text, 'bad');
}

// When a proxy in front (IIS with response buffering on) holds the live stream back, nothing arrives: the page then
// asks every few seconds instead. Slower, but everything works.
let pollTimer = null;
function startPolling() {
  if (pollTimer) return;
  if (stream) { stream.close(); stream = null; }
  S.polling = true;
  const tick = async () => {
    try {
      const st = await api('GET', 'api/state');
      S.live = true;
      const newest = S.events.length ? new Date(S.events[0].time).getTime() : 0;
      const fresh = (st.events || []).filter(e => new Date(e.time).getTime() > newest).reverse();
      onState({ servers: st.servers, machine: st.machine });
      fresh.forEach(onLog);
    } catch (e) {
      S.live = false;
      updateTop();
    }
  };
  pollTimer = setInterval(tick, 3000);
  tick();
}

function connect() {
  if (pollTimer) return;
  if (stream) stream.close();
  stream = new EventSource('api/stream');
  let gotState = false;
  const watchdog = setTimeout(() => { if (!gotState && stream) startPolling(); }, 9000);
  stream.addEventListener('open', () => { S.live = true; updateTop(); });
  stream.addEventListener('state', e => {
    gotState = true;
    clearTimeout(watchdog);
    onState(JSON.parse(e.data));
  });
  stream.addEventListener('log', e => onLog(JSON.parse(e.data)));
  stream.addEventListener('error', () => {
    S.live = false;
    updateTop();
    if (stream.readyState === EventSource.CLOSED) {
      // Session gone, or the manager restarted: check, then reconnect
      setTimeout(() => api('GET', 'api/hello').then(hl => {
        if (hl.ui && S.ui && hl.ui !== S.ui) staleUI(hl.ui); // it came back updated
        if (hl.user) connect(); else showLogin();
      }).catch(() => setTimeout(connect, 3000)), 2000);
    }
  });
}

const NAV = [
  { href: '#/', label: 'Servers', icon: 'server', match: r => r[0] === '' || r[0] === 'server' || r[0] === 'new' },
  { href: '#/events', label: 'Activity', icon: 'list', match: r => r[0] === 'events' },
  { href: '#/addons', label: 'Addons', icon: 'package', match: r => r[0] === 'addons', right: 'h' },
  { href: '#/backups', label: 'Backups', icon: 'save', match: r => r[0] === 'backups', right: 'j' },
  { href: '#/games', label: 'Games', icon: 'layers', match: r => r[0] === 'games', right: 'b' },
  { href: '#/settings', label: 'Settings', icon: 'sliders', match: r => r[0] === 'settings', right: 'b' },
  { href: '#/users', label: 'Users', icon: 'users', match: r => r[0] === 'users', right: 'd' },
];

function buildShell() {
  const app = $('#app');
  const navItems = NAV.filter(n => !n.right || can(n.right));
  shell.nav = h('nav', { class: 'nav' }, navItems.map(n => h('a', { href: n.href, title: n.label }, icon(n.icon), h('span', { class: 'navlbl' }, n.label))));
  shell.bottom = h('nav', { class: 'bottomnav' }, navItems.map(n => h('a', { href: n.href }, icon(n.icon), n.label)));
  shell.live = h('span', { class: 'live' }, h('i'), 'Live');
  shell.menuWrap = h('div', { class: 'usermenu' });
  const avatar = h('button', { class: 'avatar', title: S.user.name, 'aria-haspopup': 'menu', onclick: e => { e.stopPropagation(); toggleMenu(); } },
    S.user.name.slice(0, 1).toUpperCase());
  shell.menuWrap.append(avatar);
  shell.view = h('main', { class: 'main', id: 'view' });
  swap(app,
    h('header', { class: 'topbar' },
      h('a', { class: 'brand', href: '#/' }, logoImg(), h('div', null, h('small', null, brandName()), h('b', null, 'Server Manager'))),
      shell.nav,
      h('div', { class: 'topstats' }, shell.live, shell.menuWrap)),
    shell.view, shell.bottom);
  updateTop();
}

function toggleMenu() {
  const old = $('.menu', shell.menuWrap);
  if (old) { old.remove(); return; }
  const menu = h('div', { class: 'menu', role: 'menu' },
    h('div', { class: 'who' }, h('b', null, S.user.name), h('div', { class: 'muted small' }, can('a') ? 'Admin' : 'Rights: ' + S.user.rights)),
    h('a', { href: '#/account', role: 'menuitem' }, icon('key'), 'Change password'),
    can('b') ? h('a', { href: '#/settings', role: 'menuitem' }, icon('sliders'), 'Settings') : null,
    h('button', { role: 'menuitem', onclick: async () => { await run(() => api('POST', 'api/logout')); showLogin(); } }, icon('logout'), 'Sign out'),
    h('div', { class: 'muted small', style: 'padding:8px 10px 4px' }, 'Version ' + (S.machine ? S.machine.version : '')));
  shell.menuWrap.append(menu);
  const off = e => { if (!menu.contains(e.target)) { menu.remove(); document.removeEventListener('click', off); } };
  setTimeout(() => document.addEventListener('click', off), 0);
  menu.addEventListener('click', e => { if (e.target.closest('a')) menu.remove(); });
}

function updateTop() {
  if (!shell.live) return;
  shell.live.className = 'live' + (S.live ? '' : ' off');
  shell.live.lastChild.textContent = S.live ? 'Live' : 'Reconnecting…';
}

function setActiveNav(parts) {
  for (const nav of [shell.nav, shell.bottom]) {
    if (!nav) continue;
    [...nav.children].forEach((a, i) => {
      const n = NAV.filter(x => !x.right || can(x.right))[i];
      a.classList.toggle('active', n.match(parts));
    });
  }
}

/* =================================================================================================================
 * Router
 * =============================================================================================================== */

function route() {
  if (!S.user || !shell.view) return;
  const parts = location.hash.replace(/^#\/?/, '').split('/').map(decodeURIComponent);
  if (view && view.destroy) view.destroy();
  view = null;
  setActiveNav(parts);
  S.ctxMachine = '';
  if (parts[0] === 'server' && (parts[1] || '').includes('~')) S.ctxMachine = parts[1].split('~')[0];
  S.page = parts[0];
  const machinePage = S.machines && MACHINE_PAGES.includes(parts[0]);
  if (machinePage) S.ctxMachine = pickedMachine(parts[0]);
  let page;
  switch (parts[0]) {
    case '': page = pageDashboard(); break;
    case 'server': page = serverBy(parts[1]) ? pageServer(parts[1], parts[2] || 'overview', parts[3]) : pageMissing(); break;
    case 'new': page = can('c') ? pageNew() : pageMissing(); break;
    case 'events': page = pageEvents(); break;
    case 'settings': page = can('b') ? pageSettings(parts[1]) : pageMissing(); break;
    case 'games': page = can('b') ? pageGames(parts[1] === 'new') : pageMissing(); break;
    case 'addons': page = can('h') ? pageAddons() : pageMissing(); break;
    case 'backups': page = can('j') ? pageBackups() : pageMissing(); break;
    case 'cache': page = can('b') ? pageCache() : pageMissing(); break;
    case 'users': page = can('d') ? pageUsers() : pageMissing(); break;
    case 'account': page = pageAccount(); break;
    default: page = pageMissing();
  }
  view = page;
  swap(shell.view, machinePage ? h('div', null, machineBar(), page.el) : page.el);
  window.scrollTo(0, 0);
}

window.addEventListener('hashchange', route);

function pageMissing() {
  return { el: h('div', { class: 'empty' }, icon('alert'), h('h2', null, 'Nothing here'), h('p', null, h('a', { href: '#/' }, 'Back to the servers'))) };
}

/* =================================================================================================================
 * Server actions
 * =============================================================================================================== */

async function serverOp(v, op, extra) {
  const n = humans(v);
  const who = n === 1 ? '1 player is' : n + ' players are';
  const verbs = { stop: 'Stop', restart: 'Restart', update: 'Update', validate: 'Verify files of', addons: 'Update the addons of' };
  let warn = false;
  if (verbs[op] && isUp(v) && n > 0) {
    // Players on it: warn them first (as the automatic updates and restarts do), or go now
    const what = { stop: 'stops', restart: 'restarts', update: 'updates', validate: 'updates', addons: 'restarts' }[op];
    const secs = (S.warn || {})[op === 'update' || op === 'validate' ? 'update' : 'restart'] || 60;
    const span = secs >= 90 ? Math.round(secs / 60) + ' minutes' : secs >= 60 ? '1 minute' : secs + ' seconds';
    const more = (op === 'update' || op === 'validate') && v.update_mode === 'cache' ? ' The update downloads first, while they play; the warning starts when it is ready.' : '';
    const pick = await choiceBox(verbs[op] + ' ' + v.name + '?',
      `${who} on it. They can be warned in the chat first ("Server ${what} in ${span}", repeated), or disconnected now.${more}`,
      [{ value: 'warn', label: 'Warn players first', cls: 'primary' }, { value: 'now', label: (op === 'addons' ? 'Update' : verbs[op].split(' ')[0]) + ' now', cls: 'danger' }]);
    if (!pick) return;
    warn = pick === 'warn';
  } else if (op === 'stop' && isUp(v)) {
    if (!await confirmBox('Stop ' + v.name + '?', 'It stays stopped until you start it again: no crash restart, no scheduled restart.', 'Stop', true)) return;
  } else if (op === 'addons') {
    if (!await confirmBox('Update the addons of ' + v.name + '?', 'Its addon packages (' + (v.packages || []).join(', ') + ') are copied into it: what changed only, its plugin configs kept.' + (isUp(v) ? ' Nobody is playing: it stops for the copy and starts again.' : ''), 'Update addons')) return;
  } else if ((op === 'update' || op === 'validate') && isUp(v)) {
    const mode = v.update_mode === 'cache' ? 'The shared copy downloads first while it keeps running; it stops only for the copy.' : 'It stops during the download.';
    if (!await confirmBox((op === 'validate' ? 'Verify files of ' : 'Update ') + v.name + '?', `Nobody is playing. ${mode} It starts again after.`, op === 'validate' ? 'Verify files' : 'Update')) return;
  }
  const labels = { start: 'Starting', stop: 'Stopping', restart: 'Restarting', update: 'Update started', validate: 'File check started', install: 'Install started', cancel: 'Cancelled', addons: 'Updating addons' };
  const q = extra || (warn ? '?warn=1' : '');
  await run(() => api('POST', `api/servers/${encodeURIComponent(v.id)}/${op}${q}`), (warn ? 'Warning the players: ' : labels[op] + ': ') + v.name);
}

// choiceBox asks to pick one of several buttons: the value picked, or null.
function choiceBox(title, text, buttons) {
  return new Promise(resolve => {
    let done = false;
    const m = modal({ title, onClose: () => { if (!done) resolve(null); } });
    append(m.bd, h('p', { style: 'margin:0;color:var(--text-2)' }, text));
    append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'),
      buttons.map(b => h('button', { class: 'btn ' + (b.cls || ''), onclick: () => { done = true; m.close(); resolve(b.value); } }, b.label)));
    setTimeout(() => { const f = m.ft.querySelector('.btn.primary'); if (f) f.focus(); }, 30);
  });
}

// SnipeZilla, with ZILLA in red
const brandName = () => ['Snipe', h('span', { class: 'zilla' }, 'Zilla')];

// The buttons of a server, as fits its state and the user's rights.
function actionButtons(v, big) {
  if (v.machine && !machineOnline(v.machine)) return null; // nothing to ask a machine that is not connected
  const sm = big ? '' : ' sm';
  const stop = e => e.stopPropagation();
  const b = (cls, ic, label, fn, title) => h('button', {
    class: 'btn' + sm + ' ' + cls + (label ? '' : ' icon'), title: title || label, 'aria-label': title || label,
    'data-key': ic + ':' + (title || label), onclick: e => { stop(e); fn(); },
  }, icon(ic), label || null);
  const cur = () => serverBy(v.id) || v; // the state when clicked, not when drawn
  const out = [];
  const busy = v.busy && v.busy !== 'start';
  if (busy) {
    if (can('i') || can('h')) out.push(b('', 'x', big ? 'Cancel' : '', () => serverOp(cur(), 'cancel'), 'Cancel ' + (BUSY[v.busy] || v.busy).toLowerCase()));
    return out;
  }
  if (v.status === 'not_installed') {
    if (can('c') || can('h')) out.push(b('primary', 'download', 'Install', async () => {
      await serverOp(cur(), 'install');
      if (can('f')) location.hash = '#/server/' + encodeURIComponent(v.id) + '/setup';
    }, 'Download it, then set it up'));
    if (big && can('c')) out.push(b('danger', 'trash', 'Delete', () => deleteServerFiles(cur()), 'Delete this server and its folder'));
    return out;
  }
  if (can('i')) {
    if (isUp(v)) {
      out.push(b('danger', 'stop', big ? 'Stop' : '', () => serverOp(cur(), 'stop'), 'Stop'));
      out.push(b('', 'restart', big ? 'Restart' : '', () => serverOp(cur(), 'restart'), 'Restart'));
    } else {
      out.push(b('ok', 'play', 'Start', () => serverOp(cur(), 'start')));
      if (v.desired && (v.status === 'crashed')) out.push(b('', 'pause', big ? 'Stop retrying' : '', () => serverOp(cur(), 'stop'), 'Stop retrying'));
    }
  }
  // Stopped (or given up): it can be deleted, with its whole folder
  if (big && can('c') && !v.pid && (v.status === 'stopped' || v.status === 'failed')) {
    out.push(b('danger', 'trash', 'Delete', () => deleteServerFiles(cur()), 'Delete this server and its whole folder'));
  }
  if (can('h')) {
    out.push(b(v.update ? 'primary' : '', 'update', big || v.update ? 'Update' : '', () => serverOp(cur(), 'update'), v.update ? 'An update is waiting: install it' : 'Update'));
    if (big) out.push(b('', 'shield', 'Verify files', () => serverOp(cur(), 'validate'), 'Check every file with steamcmd and repair'));
    if (big && v.packages && v.packages.length) out.push(b('', 'package', 'Update addons', () => serverOp(cur(), 'addons'), 'Copy its addon packages into it: ' + v.packages.join(', ')));
  }
  return out;
}

/* =================================================================================================================
 * Dashboard
 * =============================================================================================================== */

// notAdminNote: the warning when the manager does not run as an administrator (Windows).
function notAdminNote(m) {
  if (!m || !(m.os || '').startsWith('windows') || m.admin !== false) return null;
  return h('div', { class: 'note warn', style: 'margin-bottom:18px' }, icon('alert'), ' ',
    h('b', null, 'The manager is not running as an administrator.'), ' Its crash reports have no crash dump (the module and the plugins at fault), the servers\' network is not counted, and a server it did not start can only be stopped through RCON. ',
    'Install it as the Windows service (szm-service.bat, or ', h('span', { class: 'mono' }, 'szm install'), ' in an administrator command prompt), or start szm.exe with Run as administrator.');
}

function pageDashboard() {
  const filter = store.get('filter', { q: '', status: 'all', game: '' });
  const tiles = h('div', { class: 'tiles' });
  const machinesEl = h('div', { class: 'machines' });
  const banner = h('div');
  const adminNote = h('div');
  const cards = h('div', { class: 'cards' });
  const chipsStatus = h('div', { class: 'chips' });
  const chipsGame = h('div', { class: 'chips' });
  const activity = h('div', { class: 'activity' });
  const sub = h('div', { class: 'sub' });
  const q = h('input', {
    type: 'search', placeholder: 'Search servers, maps, folders…', value: filter.q, 'aria-label': 'Search',
    oninput: () => { filter.q = q.value; store.set('filter', filter); renderCards(); },
  });

  const STATUS = [
    ['all', 'All', () => true],
    ['running', 'Running', v => isUp(v)],
    ['stopped', 'Stopped', v => !isUp(v) && !['crashed', 'failed'].includes(v.status)],
    ['problems', 'Problems', v => ['crashed', 'failed'].includes(v.status) || !!v.error && !isUp(v) || v.update],
  ];

  function matches(v) {
    const st = STATUS.find(s => s[0] === filter.status) || STATUS[0];
    if (!st[2](v)) return false;
    if (S.machines && filter.machine != null && (v.machine || '') !== filter.machine) return false;
    if (filter.game && v.game !== filter.game) return false;
    if (filter.q) {
      const hay = [v.name, v.hostname, v.game_name, v.address, v.info && v.info.map, v.install_dir, v.notes].join(' ').toLowerCase();
      if (!filter.q.toLowerCase().split(/\s+/).every(w => hay.includes(w))) return false;
    }
    return true;
  }

  function renderTiles() {
    const list = S.servers;
    const up = list.filter(isUp).length;
    const pl = list.reduce((a, v) => a + humans(v), 0);
    const slots = list.filter(isUp).reduce((a, v) => a + (v.info ? v.info.maxplayers : 0), 0);
    const bots = list.reduce((a, v) => a + (v.info ? v.info.bots : 0), 0);
    const m = S.machine || {};
    swap(adminNote, notAdminNote(m));
    const problems = list.filter(v => ['crashed', 'failed'].includes(v.status)).length;
    const updates = list.filter(v => v.update).length;
    swap(tiles,
      h('div', { class: 'panel tile' }, h('div', { class: 'ic green' }, icon('server')), h('div', null,
        h('div', { class: 'v' }, up, h('small', null, ' / ' + list.length)), h('div', { class: 'l' }, 'servers online'))),
      h('div', { class: 'panel tile' }, h('div', { class: 'ic purple' }, icon('users')), h('div', null,
        h('div', { class: 'v' }, pl, h('small', null, ' / ' + slots)), h('div', { class: 'l' }, 'players' + (bots ? ` (+${bots} bots)` : '')))),
      h('div', { class: 'panel tile' }, h('div', { class: 'ic ' + (problems ? 'amber' : 'blue') }, icon(problems ? 'alert' : 'bolt')), h('div', null,
        h('div', { class: 'v' }, problems ? problems : updates, h('small', null, problems ? ' crashed' : updates === 1 ? ' update' : ' updates')),
        h('div', { class: 'l' }, problems ? 'need a look' : updates ? 'waiting to install' : 'everything up to date'))),
      crashTile());
    const steam = m.steam;
    if (steam) {
      const pct = Math.round(steam.percent || 0);
      swap(banner, h('div', { class: 'panel', style: 'padding:12px 16px;margin-bottom:18px' },
        h('div', { class: 'row', style: 'margin-bottom:8px' }, h('span', { class: 'tag blue' }, icon('download'), 'steamcmd'),
          h('b', { class: 'grow ellipsis' }, steam.label), h('span', { class: 'muted small' }, (steam.state || 'working') + (pct ? ' · ' + pct + '%' : ''))),
        h('div', { class: 'bar striped' }, h('i', { style: `width:${Math.max(pct, 3)}%` })),
        steam.prompt === 'approve'
          ? h('div', { class: 'note warn', style: 'margin-top:8px' }, h('b', null, 'Approve the login in the Steam app now'), ': the shield (Steam Guard) at the bottom of the app, then Approve. steamcmd waits about a minute.')
          : h('div', { class: 'row wrap small', style: 'gap:6px 18px;margin-top:8px;color:var(--text-2)' }, steamFacts(steam))));
    } else clear(banner);
    sub.textContent = list.length ? `${list.length} server${list.length > 1 ? 's' : ''} · ${up} running · ${pl} player${pl === 1 ? '' : 's'} online` : '';
  }

  // the crashes of the last 24 hours, of every machine (those this user sees); a click opens the log
  function crashTile() {
    const n = S.crashes || 0;
    return h('a', { class: 'panel tile link', href: '#/events', title: n ? 'See them in the log' : 'The log' },
      h('div', { class: 'ic ' + (n ? 'red' : 'green') }, icon(n ? 'alert' : 'shield')), h('div', null,
        h('div', { class: 'v' }, n, h('small', null, n === 1 ? ' crash' : ' crashes')),
        h('div', { class: 'l' }, 'in the last 24 hours')));
  }

  // a manager (szm): its memory, CPU, network, how long it has been running
  function selfLine(mi, whose) {
    const net = mi.self_net, g = mi.self_go;
    // where its memory goes: what Go holds (a leak of the manager grows it), the rest is the system's
    const inside = g ? `\n\nInside: Go holds ${fmtBytes(g.mem)} (${fmtBytes(g.heap)} in use)` +
      `\n${g.goroutines} goroutines` + (g.threads ? ` · ${g.threads} threads` : '') + (g.handles ? ` · ${g.handles} ${mi.os && mi.os.startsWith('windows') ? 'handles' : 'open files'}` : '') : '';
    return h('div', { class: 'mself', title: whose + ' manager (szm): memory, CPU (of the whole machine)' + (net ? ', network in ↓ and out ↑ in bits per second (the hub, the nodes, the web pages, RCON, Steam; Resource Monitor shows bytes: ×8)' : '') + ', running for' + inside },
      icon('gauge'), h('span', null, [h('b', { class: 'nw' }, fmtBytes(mi.self_mem)), h('span', { class: 'nw' }, fmtPct(mi.self_cpu || 0) + ' CPU'),
        net ? h('span', { class: 'nw' }, '↓ ' + fmtBits(net.in) + ' ↑ ' + fmtBits(net.out)) : null, h('span', { class: 'nw' }, 'up ' + fmtDur(mi.uptime || 0))]
        .filter(Boolean).flatMap((x, k) => k ? [' · ', x] : [x])));
  }

  // the machines: this one and the linked ones, each a tile; a click shows only its servers
  function renderMachines() {
    if (!S.machines) { clear(machinesEl); if (filter.machine != null) { filter.machine = null; store.set('filter', filter); } return; }
    if (filter.machine != null && (!machineBy(filter.machine) || hubOnly(machineBy(filter.machine)))) filter.machine = null;
    swap(machinesEl, S.machines.filter(mc => !hubOnly(mc)).map(mc => {
      const mi = mc.local ? S.machine || {} : mc.machine || {};
      const list = S.servers.filter(v => (v.machine || '') === mc.id);
      const up = list.filter(isUp).length;
      const used = mi.mem_total ? (mi.mem_total - mi.mem_free) / mi.mem_total : 0;
      const on = filter.machine === mc.id;
      return h('button', { type: 'button', class: 'panel mtile' + (on ? ' on' : '') + (mc.online ? '' : ' off'), 'aria-pressed': String(on),
        title: on ? 'Show every machine\'s servers' : 'Show only the servers of ' + mc.name,
        onclick: () => { filter.machine = on ? null : mc.id; store.set('filter', filter); renderMachines(); renderHere(); renderCards(); } },
      h('div', { class: 'row', style: 'gap:8px' }, h('span', { class: 'dot ' + (mc.online ? 'ok' : 'bad') }), h('b', { class: 'grow ellipsis' }, mc.name),
        mc.local ? h('span', { class: 'muted small' }, 'this hub') : mc.online ? (verTag(mc.version) || h('span', { class: 'muted small' }, 'v?'))
          : h('span', { class: 'muted small' }, mc.linked ? 'offline' + (mc.since ? ' · ' + timeAgo(mc.since) : '') : 'waiting to join')),
      h('div', { class: 'small', style: 'color:var(--text-2)' }, `${up} / ${list.length} server${list.length === 1 ? '' : 's'} running`,
        mi.os ? h('span', { class: 'muted' }, ' · ' + (mi.os.startsWith('windows') ? 'Windows' : 'Linux')) : null),
      // its manager: what it uses, how long it has been running (what it said last: not when offline)
      mc.online && mi.self_mem && !(S.machine || {}).hide_manager ? selfLine(mi, 'Its') : null,
      // the machine's health: CPU, memory, network (what it said last: not when offline)
      mc.online && mi.load ? loadRow('CPU', mi.load.cpu / 100, fmtPct(mi.load.cpu), 'CPU in use on ' + mc.name + ', all its cores') : null,
      mi.mem_total ? loadRow('RAM', used, fmtBytes(mi.mem_total - mi.mem_free) + ' / ' + fmtRAM(mi.mem_total), 'Memory (RAM) in use on ' + mc.name + ': ' + Math.round(used * 100) + '%') : null,
      mc.online && mi.load && mi.load.net ? h('div', { class: 'mram', title: 'Network of ' + mc.name + ', every program: in ↓ and out ↑, in bits per second (as hosting plans count it)' },
        h('span', { class: 'muted' }, 'NET'), h('span', { class: 'mono grow' }, '↓ ' + fmtBits(mi.load.in) + '  ↑ ' + fmtBits(mi.load.out))) : null);
    }));
  }

  function loadRow(label, frac, value, tip) {
    return h('div', { class: 'mram', title: tip },
      h('span', { class: 'muted' }, label),
      h('div', { class: 'bar grow' + (frac > 0.9 ? ' bad' : frac > 0.75 ? ' warn' : '') }, h('i', { style: `width:${Math.min(100, Math.round(frac * 100))}%` })),
      h('span', { class: 'mono' }, value));
  }

  function renderChips() {
    const list = S.servers;
    swap(chipsStatus, STATUS.map(([k, label, fn]) => {
      const n = list.filter(fn).length;
      if (k === 'problems' && !n && filter.status !== 'problems') return null;
      return h('button', { class: 'chip' + (filter.status === k ? ' on' : ''), onclick: () => { filter.status = k; store.set('filter', filter); renderChips(); renderCards(); } },
        label, h('span', { class: 'n' }, n));
    }));
    const games = [...new Set(list.map(v => v.game))];
    if (games.length < 2) { clear(chipsGame); if (filter.game) { filter.game = ''; } return; }
    swap(chipsGame, h('button', { class: 'chip' + (!filter.game ? ' on' : ''), onclick: () => { filter.game = ''; store.set('filter', filter); renderChips(); renderCards(); } }, 'All games'),
      games.map(g => h('button', { class: 'chip' + (filter.game === g ? ' on' : ''), onclick: () => { filter.game = g; store.set('filter', filter); renderChips(); renderCards(); } },
        (S.gamesBy[g] || { short: g }).short, h('span', { class: 'n' }, list.filter(v => v.game === g).length))));
  }

  // The cards on screen are updated each second; the others when they come into view (with many servers, building
  // every card each second would keep the browser busy for nothing).
  const cardCache = new Map();
  const cardData = new Map(); // id -> the server's latest data
  const onScreen = new Set();
  const io = 'IntersectionObserver' in window ? new IntersectionObserver(entries => {
    for (const e of entries) {
      const id = e.target.dataset.sid;
      if (!e.isIntersecting) { onScreen.delete(id); continue; }
      onScreen.add(id);
      const v = cardData.get(id);
      if (v && e.target.isConnected && e.target.dataset.stale) setCard(v, e.target);
    }
  }, { rootMargin: '300px 0px' }) : null;
  function setCard(v, old) {
    const fresh = serverCard(v);
    const el = old && old.isConnected ? morph(old, fresh) : fresh;
    el.dataset.sid = v.id;
    delete el.dataset.stale;
    if (io && el !== old) { if (old) io.unobserve(old); io.observe(el); }
    cardCache.set(v.id, el);
    return el;
  }
  function renderCards() {
    const list = S.servers.filter(matches);
    if (!S.servers.length) {
      swap(cards, h('div', { class: 'panel empty', style: 'grid-column:1/-1' }, icon('server'),
        h('h2', null, 'No server yet'),
        h('p', null, 'Add your first game server: the manager installs it with steamcmd, starts it and keeps it running.'),
        can('c') ? h('a', { class: 'btn primary', href: '#/new' }, icon('plus'), 'Add a server') : null,
        can('a') ? h('p', { class: 'small' }, 'Coming from ', brandName(), ' Srcds Manager 1.x? ', h('a', { href: '#/settings' }, 'Import its config.xml'), '.') : null));
      cardCache.clear();
      return;
    }
    if (!list.length) {
      swap(cards, h('div', { class: 'empty', style: 'grid-column:1/-1' }, h('p', null, 'No server matches.')));
      cardCache.clear();
      return;
    }
    // Update the cards in place: only what changed is touched, and only the cards on screen
    const els = list.map(v => {
      cardData.set(v.id, v);
      const old = cardCache.get(v.id);
      if (io && old && old.isConnected && !onScreen.has(v.id)) {
        old.dataset.stale = '1';
        return old;
      }
      return setCard(v, old);
    });
    const same = els.length === cards.children.length && els.every((e, i) => cards.children[i] === e);
    if (!same) swap(cards, els);
  }

  // this machine (the hub, or the only one), drawn as the machines' tiles: its manager, CPU, memory, network and
  // every disk; above Activity, first on a phone
  function renderHere() {
    const m = S.machine;
    if (!m || !m.mem_total) { clear(here); return; }
    const mc = machineBy('');
    const used = (m.mem_total - m.mem_free) / m.mem_total;
    const os = m.os ? (m.os.startsWith('windows') ? 'Windows' : 'Linux') : '';
    const disks = (m.disks || []).slice().sort((a, b) => a.path.localeCompare(b.path));
    // with linked machines, the hub's panel shows every machine's servers (their tiles show only theirs)
    const all = !!S.machines;
    here.classList.toggle('pick', all);
    here.classList.toggle('on', all && filter.machine == null);
    if (all) here.setAttribute('data-tip', filter.machine == null ? 'Every machine\'s servers are shown' : 'Show every machine\'s servers');
    else here.removeAttribute('data-tip');
    here.setAttribute('role', all ? 'button' : 'region');
    here.tabIndex = all ? 0 : -1;
    const mine = S.servers.filter(v => !v.machine);
    const servers = list => `${list.filter(isUp).length} / ${list.length} server${list.length === 1 ? '' : 's'} running`;
    let status;
    if (all) {
      const nodes = S.machines.filter(x => !x.local);
      const on = nodes.filter(x => x.online).length;
      status = [h('span', { title: on < nodes.length ? 'Offline: ' + nodes.filter(x => !x.online).map(x => x.name).join(', ') : 'Every node is connected' },
        `${on} / ${nodes.length} node${nodes.length === 1 ? '' : 's'} online`), mine.length ? ' · ' + servers(mine) : ''];
    } else status = [servers(mine)];
    swap(here,
      h('div', { class: 'row', style: 'gap:8px' }, h('span', { class: 'dot ok' }), h('b', { class: 'grow ellipsis' }, mc ? mc.name : 'This machine'),
        all ? [h('span', { class: 'muted small' }, 'hub'), verTag(m.version)] : h('span', { class: 'muted small ellipsis', style: 'min-width:0' }, m.hostname || '')),
      h('div', { class: 'small', style: 'color:var(--text-2)' }, status, os ? h('span', { class: 'muted' }, ' · ' + os) : null),
      m.self_mem ? selfLine(m, 'This machine\'s') : null,
      m.load ? loadRow('CPU', m.load.cpu / 100, fmtPct(m.load.cpu), 'CPU in use on this machine, all its ' + (m.cpus || '') + ' cores') : null,
      loadRow('RAM', used, fmtBytes(m.mem_total - m.mem_free) + ' / ' + fmtRAM(m.mem_total), 'Memory (RAM) in use: ' + Math.round(used * 100) + '%'),
      m.load && m.load.net ? h('div', { class: 'mram', title: 'Network of this machine, every program: in ↓ and out ↑, in bits per second (as hosting plans count it)' },
        h('span', { class: 'muted' }, 'NET'), h('span', { class: 'mono grow' }, '↓ ' + fmtBits(m.load.in) + '  ↑ ' + fmtBits(m.load.out))) : null,
      disks.map(d => loadRow(d.path, 1 - d.free / d.total, fmtBytes(d.free) + ' free',
        `Disk ${d.path}: ${fmtBytes(d.total - d.free)} used of ${fmtBytes(d.total)}, ${fmtBytes(d.free)} free`)));
  }

  function renderActivity() {
    const evs = S.events.slice(0, 14);
    if (!evs.length) { swap(activity, h('div', { class: 'muted small', style: 'padding:16px' }, 'Nothing happened yet.')); return; }
    swap(activity, evs.map(eventRow));
  }

  const here = h('div', { class: 'panel mpanel' });
  const showAll = () => {
    if (!S.machines || filter.machine == null) return;
    filter.machine = null; store.set('filter', filter); renderMachines(); renderHere(); renderCards();
  };
  here.addEventListener('click', showAll);
  here.addEventListener('keydown', e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); showAll(); } });
  const el = h('div', null,
    h('div', { class: 'pagehead' },
      h('div', { class: 'grow' }, h('h1', null, 'Servers'), sub),
      can('h') && S.servers.length ? h('button', { class: 'btn', onclick: () => run(() => api('POST', 'api/updates/check'), 'Asking Steam for updates…') }, icon('refresh'), 'Check updates') : null,
      can('c') ? h('a', { class: 'btn primary', href: '#/new' }, icon('plus'), 'New server') : null),
    adminNote, tiles, banner,
    h('div', { class: 'grid2' },
      h('div', null,
        S.servers.length > 1 ? h('div', { class: 'toolbar' }, h('div', { class: 'search' }, icon('search'), q), chipsStatus, chipsGame) : null,
        cards),
      h('div', { class: 'dside' }, h('div', { class: 'mgroup' }, here, machinesEl),
        h('div', { class: 'panel dact' }, h('div', { class: 'hd' }, h('h2', { class: 'grow' }, 'Activity'), h('a', { href: '#/events', class: 'small' }, 'All')), activity))));

  const update = () => { renderTiles(); renderMachines(); renderHere(); renderChips(); renderCards(); };
  update();
  renderActivity();
  const timer = setInterval(() => { renderCards(); }, 1000); // countdowns, uptime
  return { el, update, onEvent: renderActivity, destroy: () => { clearInterval(timer); if (io) io.disconnect(); } };
}

function serverCard(v) {
  const st = statusInfo(v);
  const n = humans(v);
  const max = v.info ? v.info.maxplayers : 0;
  const notes = [];
  if (v.warn_until) {
    const left = Math.max(0, v.warn_until - now());
    notes.push(h('div', { class: 'note warn' }, `Players warned: the server ${v.warn_what} in ${fmtDur(left)}`));
  }
  if (v.progress && v.busy) {
    const pct = Math.round(v.progress.percent || 0);
    notes.push(h('div', null,
      h('div', { class: 'row small', style: 'margin-bottom:6px', title: v.progress.total ? fmtBytes(v.progress.done || 0) + ' of ' + fmtBytes(v.progress.total) + (v.progress.left ? ' · about ' + fmtDur(v.progress.left) + ' left' : '') : null },
        h('span', { class: 'grow ellipsis muted' }, v.progress.state || 'steamcmd'), h('b', null, pct + '%')),
      h('div', { class: 'bar striped' }, h('i', { style: `width:${Math.max(3, pct)}%` }))));
  } else if ((v.busy === 'install' || v.busy === 'update') && !v.warn_until) {
    notes.push(h('div', { class: 'note info' }, v.stage || (v.busy === 'install' ? 'Installing…' : 'Updating…')));
  }
  if (v.status === 'crashed' && !v.busy) {
    const left = v.next_start ? Math.max(0, v.next_start - now()) : 0;
    notes.push(h('div', { class: 'note bad' }, `Crashed ${v.crashes}× in a row` + (left ? ` · restarting in ${fmtDur(left)}` : ' · restarting…')));
  } else if (v.status === 'failed' && v.start_map_gone) {
    notes.push(h('div', { class: 'note bad' }, `Can not start: its start map ${v.start_map_gone} is not on this server. Change it in Settings.`));
  } else if (v.status === 'failed' && v.cant_start) {
    notes.push(h('div', { class: 'note bad' }, 'Can not start: ' + v.cant_start));
  } else if (v.status === 'failed') {
    notes.push(h('div', { class: 'note bad' }, `Crashed ${v.crashes} times: the manager stopped trying. Check the console, then start it.`));
  } else if (v.error && !isUp(v) && !v.busy) {
    notes.push(v.login_need ? h('div', { class: 'note warn' }, v.login_need === 'approve' ? 'The Steam login was not approved: open the server'
      : 'Steam asks for a ' + (v.login_need === 'guard' || v.login_need === '2fa' ? 'Steam Guard code' : 'password') + ': open the server')
      : h('div', { class: 'note bad ellipsis', title: v.error }, v.error));
  }
  if (v.vanilla && isUp(v)) notes.push(h('div', { class: 'note warn', title: 'The manager gave up on it after crashes, and started it without its addons (Metamod off) while it waits for a fix of ' + v.vanilla + '. Its addons come back with the fix.' }, 'Without its addons: waiting for a fix of ' + v.vanilla));
  if (v.update && !v.busy) notes.push(h('div', { class: 'note orange' }, v.autoupdate ? 'Update waiting: installs when ready' : 'An update is available'));
  if (v.foreign) notes.push(h('div', { class: 'note info', title: 'It was started as another Windows user (by the old manager?). The manager watches it and stops it through RCON; Windows does not let it force it to end. Run the manager as administrator or as the service for full control.' }, 'Started outside the manager: watched'));

  const up = isUp(v);
  return h('div', {
    class: 'card s-' + (st.cls === 'running update' ? 'update' : st.cls), tabindex: '0', role: 'link', 'aria-label': v.name, 'data-key': v.id,
    onclick: () => { location.hash = '#/server/' + encodeURIComponent(v.id); },
    onkeydown: e => { if (e.key === 'Enter' && e.target === e.currentTarget) location.hash = '#/server/' + encodeURIComponent(v.id); },
  },
  h('div', { class: 'top' }, badge(v.game),
    h('div', { class: 'grow' }, h('div', { class: 'name' }, v.name), hostLine(v, 'host'),
      h('div', { class: 'addr' }, S.machines ? h('span', { class: 'mtag' + (machineOnline(v.machine) ? '' : ' off'), title: 'On the machine ' + machineName(v.machine || '') }, icon('server'), machineName(v.machine || '')) : null, joinLink(v))),
    pill(v)),
  notes,
  h('div', { class: 'meta' },
    h('div', null, h('div', { class: 'k' }, 'Map'), h('div', { class: 'v ellipsis' }, up && v.info ? v.info.map : '—')),
    h('div', null, h('div', { class: 'k' }, 'Players'),
      h('div', { class: 'v' }, up && v.info ? `${n} / ${max}` : '—', up && v.info && v.info.bots ? h('span', { class: 'muted small' }, ` +${v.info.bots} bot${v.info.bots > 1 ? 's' : ''}`) : null),
      up && max ? h('div', { class: 'bar ok', style: 'margin-top:4px' }, h('i', { style: `width:${Math.min(100, n / max * 100)}%` })) : null),
    h('div', null, h('div', { class: 'k' }, 'CPU · RAM'), h('div', { class: 'v' }, up && v.memory ? `${v.cpu.toFixed(1)}% · ${fmtBytes(v.memory)}` : '—')),
    up ? cardNet(v) : h('div', null, h('div', { class: 'k' }, 'Version'), h('div', { class: 'v ellipsis' }, v.version || '—'))),
  h('div', { class: 'actions' }, actionButtons(v), h('span', { class: 'spacer' }),
    up && v.started ? h('span', { class: 'uptag', title: 'Running for ' + fmtDur(now() - v.started) + ', since ' + new Date(v.started * 1000).toLocaleString() }, icon('clock'), fmtDur(now() - v.started)) : null,
    v.pid ? h('span', { class: 'pidtag', title: 'Its process ID, as Task Manager shows it (Details tab)' + (v.foreign ? '\nStarted outside the manager, as another Windows user.' : '') }, 'PID ', h('b', null, String(v.pid))) : null,
    can('g') && up ? h('a', { class: 'btn sm ghost icon', href: '#/server/' + encodeURIComponent(v.id) + '/console', title: 'Console', 'aria-label': 'Console', onclick: e => e.stopPropagation() }, icon('terminal')) : null,
    h('span', { class: 'muted', style: 'display:flex' }, icon('chevron'))));
}

// privateIP: an address of a local network (players outside it can not reach it)
const privateIP = ip => /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\.|169\.254\.)/.test(ip);

// bestIP: the address of a machine players reach: its first public one, else its first
function bestIP(mi) {
  const ips = ((mi || {}).ips || []).filter(x => x && !x.startsWith('127.'));
  return ips.find(x => !privateIP(x)) || ips[0] || '';
}

// joinAddr: the address players join a server at. A server listening on every address (shown *:port) is joined at
// its machine's address (its first public one).
function joinAddr(v) {
  const a = v.address || '';
  const i = a.lastIndexOf(':');
  if (i < 0) return a;
  const host = a.slice(0, i).replace(/^\[|\]$/g, ''), port = a.slice(i + 1);
  if (!['*', '0.0.0.0', '127.0.0.1', '::', ''].includes(host)) return a;
  const mc = machineBy(v.machine || '');
  const ip = bestIP((mc && !mc.local ? mc.machine : S.machine) || {});
  return ip ? ip + ':' + port : a;
}

// joinLink: the server's address, a link that joins it in the game (Steam opens it: steam://connect/ip:port)
function joinLink(v) {
  const a = joinAddr(v);
  if (!a) return null;
  // the games of other engines: their address (Steam's connect link is not theirs to all)
  if ((S.gamesBy[v.game] || {}).engine === 'other') return h('span', { title: 'Its game port' }, a);
  if (a.startsWith('*')) return h('span', { title: 'It listens on every address, and its machine\'s addresses are not known' }, a);
  return h('a', { class: 'join', href: 'steam://connect/' + a, title: 'Join', onclick: e => e.stopPropagation() }, a);
}

// cardNet: a server's network on its card, in ↓ and out ↑ in bits per second, as its machine counts it (on Windows,
// only when the manager runs as administrator)
function cardNet(v) {
  const mc = machineBy(v.machine || '');
  const mi = (mc && !mc.local ? mc.machine : S.machine) || {};
  const ns = mi.net || {};
  const k = h('div', { class: 'k' }, 'Network');
  if (!v.net) return h('div', { title: ns.on ? 'In ↓ and out ↑ of its process: measuring…' : ns.error || 'Not counted on this machine' }, k, h('div', { class: 'v' }, ns.on && v.pid ? '…' : '—'));
  return h('div', { title: 'In ↓ and out ↑: the data its process receives and sends, in bits per second' }, k,
    h('div', { class: 'v cnet' }, h('span', null, h('i', null, '↓'), fmtBits(v.net.in)), h('span', null, h('i', null, '↑'), fmtBits(v.net.out))));
}

const KIND_ICON = { crash: 'alert', failed: 'alert', error: 'alert', update: 'download', updated: 'check', restart: 'restart', start: 'play', stop: 'stop', steamcmd: 'download', manager: 'server', info: 'info' };

function eventRow(ev) {
  const t = new Date(ev.time).getTime() / 1000;
  return h('div', { class: 'act k-' + ev.kind },
    h('div', { class: 'dot' }, icon(KIND_ICON[ev.kind] || 'info')),
    h('div', { class: 'grow' },
      h('div', { class: 'row', style: 'gap:6px' },
        ev.machine && S.machines ? h('span', { class: 'mtag' }, icon('server'), machineName(ev.machine)) : null,
        ev.server && serverBy(ev.server) ? h('a', { class: 'who ellipsis', href: '#/server/' + encodeURIComponent(ev.server) }, ev.name) : h('span', { class: 'who' }, ev.name || 'Manager'),
        h('span', { class: 'spacer' }), h('span', { class: 't nowrap', title: new Date(ev.time).toLocaleString() }, timeAgo(t))),
      h('div', { class: 'txt' }, ev.text, reportLink(ev))));
}

// reportLink: the crash report an event comes with
function reportLink(ev) {
  if (!ev.report || !ev.server || !serverBy(ev.server) || !can('g')) return null;
  return h('a', { class: 'replink', href: '#/server/' + encodeURIComponent(ev.server) + '/crashes/' + ev.report }, icon('alert'), 'Crash report');
}

/* =================================================================================================================
 * Server page
 * =============================================================================================================== */

const TABS = [
  { k: 'overview', label: 'Overview', icon: 'gauge' },
  { k: 'console', label: 'Console', icon: 'terminal', right: 'g' },
  { k: 'files', label: 'Files', icon: 'folder', right: 'j' },
  { k: 'backups', label: 'Backups', icon: 'save', right: 'j' },
  { k: 'maps', label: 'Maps', icon: 'map' },
  { k: 'settings', label: 'Settings', icon: 'sliders', right: 'f' },
  { k: 'activity', label: 'Activity', icon: 'list' },
  { k: 'crashes', label: 'Crashes', icon: 'alert', right: 'g' },
];

function pageServer(id, tab, arg) {
  const g0 = S.gamesBy[(serverBy(id) || {}).game] || {};
  // the games of other engines have no maps the manager knows of
  const tabs = TABS.filter(t => (!t.right || can(t.right)) && !(t.k === 'maps' && g0.engine === 'other'));
  const setup = tab === 'setup' && can('f');
  if (!setup && !tabs.find(t => t.k === tab)) tab = 'overview';
  const head = h('div');
  const body = h('div');
  const href = k => '#/server/' + encodeURIComponent(id) + (k === 'overview' ? '' : '/' + k);

  function renderHead() {
    const v = serverBy(id);
    if (!v) return;
    const g = S.gamesBy[v.game] || {};
    patch(head,
      h('a', { class: 'back', href: '#/' }, icon('back'), 'Servers'),
      h('div', { class: 'serverhead' },
        badge(v.game, 'lg'),
        h('div', { class: 'grow' },
          h('h1', null, v.name, pill(v), v.update ? h('span', { class: 'tag orange' }, 'Update waiting') : null),
          hostLine(v, 'hosthead'),
          h('div', { class: 'muted', style: 'margin-top:4px' }, v.machine ? h('span', { class: 'mtag' + (machineOnline(v.machine) ? '' : ' off') }, icon('server'), machineName(v.machine)) : null,
            g.name, g.name && v.address ? ' · ' : null, joinLink(v))),
        h('div', { class: 'btngroup' }, actionButtons(v, true))));
  }

  let sub = null;
  const tabBar = h('nav', { class: 'tabs' }, tabs.map(t => h('a', { href: href(t.k), class: t.k === tab ? 'active' : '' }, icon(t.icon), t.label)));
  switch (setup ? 'setup' : tab) {
    case 'setup': sub = tabSetup(id); break;
    case 'console': sub = tabConsole(id); break;
    case 'files': sub = tabFiles(id); break;
    case 'maps': sub = tabMaps(id); break;
    case 'settings': sub = tabSettings(id); break;
    case 'activity': sub = tabActivity(id); break;
    case 'crashes': sub = tabCrashes(id, arg); break;
    case 'backups': sub = tabBackups(id); break;
    default: sub = tabOverview(id);
  }
  append(body, sub.el);
  renderHead();
  return {
    el: h('div', null, head, setup ? null : tabBar, body),
    update: () => {
      if (!serverBy(id)) { location.hash = '#/'; return; }
      renderHead();
      if (sub.update) sub.update();
    },
    onEvent: ev => { if (sub.onEvent && ev.server === id) sub.onEvent(ev); },
    destroy: () => { if (sub.destroy) sub.destroy(); },
  };
}

/* ---------- Set up (step 2 of a new server) ---------- */

function tabSetup(id) {
  // Step by step: only the download until it is over; then the server's files and its few essential settings.
  const base = 'api/servers/' + encodeURIComponent(id);
  const stepsEl = h('div', { class: 'steps' });
  const dl = h('div', { class: 'bd col' });
  const after = h('div'); // shown once downloaded
  const finish = h('div', { class: 'row wrap', style: 'margin-top:18px' });
  const con = can('g') ? tabConsole(id) : null;
  const conWrap = h('div', { class: 'hidden', style: 'margin-top:4px' }, con ? con.el : null);
  const installing = v => ['install', 'update', 'validate'].includes(v.busy);
  let wasReady = null;
  let f = null; // the server's settings, once downloaded
  let dirty = false;
  let cv = null; // what its server.cfg says: hostname, join password
  let cvDirty = false;

  async function loadFiles(files) {
    try {
      const r = await api('GET', base + '/setupfiles');
      swap(files, r.files.map(fl => h('div', { class: 'mapitem', style: 'font-family:var(--sans);align-items:center' },
        icon(fl.exists ? 'filetext' : 'fileplus'),
        h('div', { class: 'grow', style: 'min-width:0' }, h('b', null, fl.label), h('div', { class: 'small muted' }, h('span', { class: 'mono' }, fl.path), ' · ' + fl.hint)),
        h('button', { class: 'btn sm', onclick: async () => {
          if (!fl.exists) {
            try { await api('PUT', base + '/file', { path: fl.path, content: '', crlf: true, mtime: 0 }); } catch (e) { toast(e.message, 'bad'); return; }
          }
          openEditor(base, fl.path, () => loadFiles(files));
        } }, icon('edit'), fl.exists ? 'Edit' : 'Create'))),
        h('div', { class: 'row', style: 'padding:10px 6px 4px' }, h('a', { class: 'btn sm ghost', href: '#/server/' + encodeURIComponent(id) + '/files' }, icon('folder'), 'Browse all its files')));
    } catch (e) { swap(files, h('div', { class: 'note bad' }, e.message)); }
  }

  // Step 2: its files, then its essential settings
  async function showSetup() {
    const files = h('div', { class: 'maplist cfgfiles', style: 'max-height:none' }, h('div', { class: 'muted small', style: 'padding:12px' }, 'Loading…'));
    const form = h('div', { class: 'formgrid' }, h('div', { class: 'muted' }, 'Loading…'));
    swap(after,
      h('div', { class: 'panel', style: 'margin-top:18px' }, h('div', { class: 'hd' }, icon('filetext'), h('h2', { class: 'grow' }, 'Its files'), h('span', { class: 'muted small' }, 'Edit them here, or browse all of them')),
        h('div', { style: 'padding:8px' }, files)),
      h('div', { class: 'panel', style: 'margin-top:18px' }, h('div', { class: 'hd' }, icon('sliders'), h('h2', { class: 'grow' }, 'Its settings'),
        h('a', { class: 'btn sm ghost', href: '#/server/' + encodeURIComponent(id) + '/settings' }, 'All settings')),
        h('div', { class: 'bd' }, form)));
    loadFiles(files);
    try {
      f = await api('GET', base + '/config');
      const g = S.gamesBy[f.game] || {};
      const other = g.engine === 'other';
      cv = other ? null : (await api('GET', base + '/cvars')).cvars;
      bindText.onChange = () => { dirty = true; };
      const gslt = secret(f, 'gslt', { placeholder: '32 characters from Steam' });
      const rcon = secret(f, 'rcon_password');
      const cvText = (key, attrs) => {
        const inp = h('input', Object.assign({ type: 'text', value: cv[key] || '', spellcheck: false }, attrs || {}));
        inp.addEventListener('input', () => { cv[key] = inp.value; cvDirty = true; });
        return inp;
      };
      if (other) {
        // its name, password, ports are its command line's: the server's settings
        swap(form,
          g.notes ? h('div', { class: 'note info small wide', style: 'padding:8px 10px;border-radius:8px' }, g.notes) : null,
          field('Name players see', bindText(f, 'name', { maxlength: 80 }), 'Its name in the manager, and in the server browser.', true),
          g.password ? field('Password to join', h('div', { class: 'inputgroup' }, ...secret(f, 'password')), 'Empty: anyone can join.') : null,
          field('IP', ipPicker(f), 'The address players join it at.'),
          field('Port', bindText(f, 'port', { type: 'number', min: 1, max: 65535 })),
          field('Max players', bindText(f, 'maxplayers', { type: 'number', min: 1, max: 255 })),
          g.map ? field('Map or world', bindText(f, 'map', { placeholder: g.default_map })) : null,
          g.gslt ? field('Game server login token', h('div', { class: 'inputgroup' }, gslt[0], gslt[1]), null, true) : null,
          g.rcon === 'none' ? null : field('RCON password', h('div', { class: 'inputgroup' }, rcon[0], rcon[1]), g.rcon === 'battleye'
            ? 'Set in its BattlEye config (BEServer_x64.cfg) as it starts, letters and numbers only: the manager uses it to stop the server cleanly. Takes effect at its next start.'
            : 'Given on its command line: the manager uses it to stop the server cleanly.'));
        return;
      }
      swap(form,
        field('Name players see', cvText('hostname', { maxlength: 120, placeholder: 'SnipeZilla | ' + (g.short || '') }), 'In the server browser and the scoreboard (hostname in server.cfg).', true),
        field('Password to join', cvText('sv_password', { placeholder: 'Empty: anyone can join', autocomplete: 'off' }), 'Only for a private server (sv_password).'),
        g.key === 'cs2' ? field('Game mode', cs2ModePicker(f, () => { dirty = true; })) : null,
        field('IP', ipPicker(f), 'The address players join it at. 0.0.0.0 (every address) only for a machine behind a router, whose public address is not one of its own.'),
        field('Port', bindText(f, 'port', { type: 'number', min: 1, max: 65535 })),
        field('Max players', bindText(f, 'maxplayers', { type: 'number', min: 1, max: 255 })),
        field('First map', mapPicker(f, g, false), 'The maps it has show as you type.'),
        g.engine === 'goldsrc' ? null : field('Game server login token', h('div', { class: 'inputgroup' }, gslt[0], gslt[1]),
          h('span', null, 'Needed to be seen on the internet. Get one at ', h('a', { href: 'https://steamcommunity.com/dev/managegameservers', target: '_blank', rel: 'noopener' }, 'steamcommunity.com/dev/managegameservers'), '.'), true),
        field('RCON password', h('div', { class: 'inputgroup' }, rcon[0], rcon[1]), 'Already set: the manager uses it to warn players and stop the server cleanly.'));
    } catch (e) { swap(form, h('div', { class: 'note bad' }, e.message)); }
  }

  async function save() {
    try {
      if (f && dirty) { await api('PUT', base, f); dirty = false; }
      if (cv && cvDirty) { await api('PUT', base + '/cvars', cv); cvDirty = false; }
      return true;
    } catch (e) {
      const list = e.data && e.data.errors ? e.data.errors : [e.message];
      toast('Not saved: ' + list.join(' '), 'bad');
      return false;
    }
  }

  function update() {
    const v = serverBy(id);
    if (!v) return;
    const busy = installing(v);
    const ready = v.status !== 'not_installed' && !busy;
    swap(stepsEl, [['Download', ready], ['Set up', false]].map(([n, done], i) =>
      h('div', { class: 'step' + (done ? ' done' : (i === 0) !== ready ? ' on' : '') }, h('i', null, done ? '✓' : i + 1), n)));
    if (!v.login_need) delete dl.dataset.need;
    if (busy) {
      // steamcmd's progress, when the job it runs is this server's
      const st = MI().steam;
      const mine = st && st.label && st.label.includes(v.name);
      const pct = mine ? Math.round(st.percent || 0) : 0;
      const gname = (S.gamesBy[v.game] || {}).name || 'the game';
      if (mine && st.prompt === 'approve') patch(dl, approveNow(v));
      else patch(dl, h('div', { class: 'row', style: 'gap:10px' }, h('span', { class: 'spin' }),
          h('b', { class: 'grow' }, mine ? steamStage(st.state) + ' ' + gname : 'Waiting to download ' + gname),
          h('b', { style: 'font-size:18px' }, mine && pct ? pct + '%' : '')),
        h('div', { class: 'bar striped' }, h('i', { style: `width:${Math.max(pct, 2)}%` })),
        h('div', { class: 'row wrap small', style: 'gap:6px 18px;color:var(--text-2)' }, mine ? steamFacts(st) : 'steamcmd does one download at a time: this one starts when the other is over.'),
        h('div', { class: 'muted small' }, 'You can leave this page: the download goes on, and the setup waits for you here.'),
        con ? h('div', null, h('button', { class: 'btn sm ghost', onclick: () => conWrap.classList.toggle('hidden') }, icon('terminal'), 'Show steamcmd')) : null);
    } else if (!ready && v.login_need) {
      if (dl.dataset.need !== v.login_need) { dl.dataset.need = v.login_need; swap(dl, steamLoginBox(v)); }
    } else if (!ready) {
      patch(dl, h('div', { class: 'note bad' }, 'The download did not finish' + (v.error ? ': ' + v.error : '.')),
        h('div', { class: 'row wrap' }, h('button', { class: 'btn primary', onclick: () => serverOp(serverBy(id), 'install') }, icon('download'), 'Download again'),
          can('c') ? h('button', { class: 'btn danger', onclick: () => deleteServerFiles(serverBy(id)) }, icon('trash'), 'Delete it') : null));
    } else {
      patch(dl, h('div', { class: 'row', style: 'gap:8px' }, h('span', { class: 'tag green' }, 'Installed'), v.version ? h('span', { class: 'muted' }, 'version ' + v.version) : null));
      conWrap.classList.add('hidden');
    }
    if (ready !== wasReady) {
      wasReady = ready;
      if (ready) showSetup();
      else clear(after);
    }
    if (!ready) { clear(finish); }
    else {
      patch(finish, h('a', { class: 'btn ghost', href: '#/server/' + encodeURIComponent(id) }, 'Later'), h('span', { class: 'spacer' }),
        isUp(v) ? h('span', { class: 'tag green' }, 'Running')
          : h('button', { class: 'btn ok', disabled: !can('i'), onclick: async () => {
            if (!await save()) return;
            await serverOp(serverBy(id), 'start');
            location.hash = '#/server/' + encodeURIComponent(id) + (can('g') ? '/console' : '');
          } }, icon('play'), 'Save and start'));
    }
    if (con && con.update) con.update();
  }

  update();
  const el = h('div', { style: 'max-width:1000px' }, stepsEl,
    h('div', { class: 'panel' }, h('div', { class: 'hd' }, icon('download'), h('h2', null, 'Download')), dl, conWrap),
    after, finish);
  function warnUnsaved(e) { if (dirty || cvDirty) { e.preventDefault(); e.returnValue = ''; } }
  window.addEventListener('beforeunload', warnUnsaved);
  return {
    el, update,
    destroy: () => { window.removeEventListener('beforeunload', warnUnsaved); if (con && con.destroy) con.destroy(); },
  };
}

/* ---------- Overview ---------- */

function tabOverview(id) {
  const stats = h('div', { class: 'stats even' });
  const notes = h('div', { class: 'col' });
  const loginWrap = h('div'); // Steam's question: its own place, so redrawing the notes does not take the cursor away
  const players = h('div');
  const cmd = h('div', { class: 'cmdline' }, '…');
  const recent = h('div', { class: 'activity' });

  if (can('f')) {
    api('POST', 'api/preview', { id }).then(p => { cmd.textContent = p.command; cmd.setAttribute('data-tip', 'Runs in ' + p.dir); }).catch(() => { cmd.textContent = '—'; });
  }

  function stat(k, v, small, tip) {
    return h('div', { class: 'stat', 'data-tip': tip || null }, h('div', { class: 'k' }, k), h('div', { class: 'v' }, v, small ? h('small', null, ' ' + small) : null));
  }

  // The size of its folder: asked while this page is open (measured again every 10 minutes)
  let use = null;
  const loadUsage = () => api('GET', 'api/servers/' + encodeURIComponent(id) + '/usage').then(u => { use = u; render(); }).catch(() => {});
  function pidStat(v, up, why) {
    const tip = why ? 'The network is not shown: ' + why + '.' : null;
    if (!up || !v.pid) return stat('PID', '—', '', tip);
    return stat('PID', h('span', { class: 'mono', style: 'font-size:17px' }, String(v.pid)), v.foreign ? 'other user' : '',
      'Its process ID, as Task Manager shows it (Details tab).' + (v.foreign ? '\nStarted outside the manager, as another Windows user.' : '') + (tip ? '\n' + tip : ''));
  }
  // Network: counted by Windows for its process, as Task Manager does. When it cannot be (the manager does not run as
  // administrator), its PID instead, with the reason.
  const rate = fmtBits; // bits per second, as the machines and the cards show it
  function netStat(v, up) {
    const ns = MI().net || {};
    if (!ns.on) return pidStat(v, up, ns.error);
    const tip = (v.pid ? 'PID ' + v.pid + (v.foreign ? ' (another Windows user)' : '') + '\n' : '') + 'In ↓ and out ↑: the data its process receives and sends (what Task Manager shows on Windows), in bits per second.';
    if (!up || !v.pid) return stat('Network', '—');
    if (!v.net) return stat('Network', '…', '', tip);
    return stat('Network', h('span', { class: 'net' }, h('span', null, h('i', null, '↓'), rate(v.net.in)), h('span', null, h('i', null, '↑'), rate(v.net.out))), '', tip);
  }
  function sizeStat() {
    const dir = serverBy(id).install_dir;
    const d = diskOf(dir);
    // under the size: the disk it is on, and how full that disk is
    const drive = d ? h('div', { class: 'sdisk' }, h('span', { class: 'muted' }, d.path),
      h('div', { class: 'bar grow' + (1 - d.free / d.total > 0.9 ? ' bad' : 1 - d.free / d.total > 0.8 ? ' warn' : '') }, h('i', { style: `width:${Math.round((1 - d.free / d.total) * 100)}%` })),
      h('span', null, fmtBytes(d.free) + ' free')) : null;
    const tip = 'Everything in ' + dir + (use && use.size_at ? '\nMeasured ' + timeAgo(use.size_at) : '') +
      (d ? `\nDisk ${d.path}: ${fmtBytes(d.total - d.free)} used of ${fmtBytes(d.total)}, ${fmtBytes(d.free)} free` : '');
    if (!use || (!use.size && use.sizing)) return stat('Folder', [use ? 'Measuring…' : '…', drive], '', tip);
    return stat('Folder', [use.size ? fmtBytes(use.size) : '—', drive], '', tip);
  }

  function render() {
    const v = serverBy(id);
    if (!v) return;
    const up = isUp(v);
    const n = humans(v);
    swap(stats,
      stat('Map', up && v.info ? v.info.map : '—'),
      stat('Players', up && v.info ? `${n} / ${v.info.maxplayers}` : '—', up && v.info && v.info.bots ? `+${v.info.bots} bot${v.info.bots > 1 ? 's' : ''}` : ''),
      stat('Uptime', up && v.started ? fmtDur(now() - v.started) : '—'),
      stat('CPU', up && v.memory ? v.cpu.toFixed(1) + '%' : '—', up && v.pid && (MI().net || {}).on ? 'PID ' + v.pid : '',
        v.pid ? 'Its process ID: ' + v.pid + ' (Task Manager, Details tab)' : null),
      stat('Memory', up && v.memory ? fmtBytes(v.memory) : '—'),
      netStat(v, up),
      sizeStat(),
      stat('Version', v.version || '—'),
      stat('Restart', v.restart === 'never' ? 'Never' : v.restart === 'auto' ? 'After 24 h, when empty' : 'Daily ' + v.restart.replace('daily ', '')),
      stat('Updates', (v.autoupdate ? 'Automatic' : 'Manual') + (v.update_mode === 'cache' ? ' · shared copy' : '')));

    const list = [];
    if (v.status === 'not_installed') list.push(h('div', { class: 'note info' }, 'This server is not installed yet. ', can('c') || can('h') ? 'Click Install: steamcmd downloads it into ' + v.install_dir + '.' : ''));
    if (v.status === 'crashed' && !v.busy) {
      const left = v.next_start ? Math.max(0, v.next_start - now()) : 0;
      list.push(h('div', { class: 'note bad' }, `It crashed ${v.crashes} time${v.crashes > 1 ? 's' : ''} in a row. ${left ? 'Next start in ' + fmtDur(left) + '.' : 'Starting…'} After 3 crashes the manager checks its files with steamcmd; it stops trying after too many. `,
        can('g') ? h('a', { href: '#/server/' + encodeURIComponent(id) + '/crashes' }, 'See what happened') : null));
    }
    if (v.status === 'failed' && v.start_map_gone) list.push(h('div', { class: 'note bad' }, `It can not start: its start map ${v.start_map_gone} is not on this server (its console said so, and it waited with no map). Change its start map in Settings, or add the map's file, then click Start. `,
      can('g') ? h('a', { href: '#/server/' + encodeURIComponent(id) + '/crashes' }, 'See the report') : null));
    else if (v.status === 'failed' && v.cant_start) list.push(h('div', { class: 'note bad' }, 'It can not start: ' + v.cant_start + ' The manager does not start it again by itself until then. ',
      can('g') ? h('a', { href: '#/server/' + encodeURIComponent(id) + '/crashes' }, 'See the report') : null));
    else if (v.status === 'failed') list.push(h('div', { class: 'note bad' }, `The manager stopped restarting it after ${v.crashes} crashes in a row. Look at the crash reports, fix what is wrong, then click Start. `,
      can('g') ? h('a', { href: '#/server/' + encodeURIComponent(id) + '/crashes' }, 'See the crash reports') : null));
    if (v.vanilla) list.push(h('div', { class: 'note warn' }, v.status === 'failed'
      ? `It crashed without its addons too (Metamod off), so the manager stopped trying. It waits for a fix of ${v.vanilla}: the fix, when it comes out, is installed and the server started again, with its addons. `
      : `It runs without its addons (Metamod off): the manager gave up on it after crashes, and started it like this while it waits for a fix of ${v.vanilla}. The fix, when it comes out, is installed and brings its addons back (the Addons page). `,
      can('g') ? h('a', { href: '#/server/' + encodeURIComponent(id) + '/crashes' }, 'See the crash reports') : null));
    // Steam wants a password or a code: the box stays as it is while the question is the same (typing is not lost)
    const st = MI().steam;
    const approving = v.busy && st && st.prompt === 'approve' && (st.label || '').includes(v.name);
    const need = approving ? 'approving' : v.login_need && !v.busy ? v.login_need : '';
    if (loginWrap.dataset.need !== need) { loginWrap.dataset.need = need; swap(loginWrap, approving ? approveNow(v) : need ? steamLoginBox(v) : null); }
    if (v.error && !up && !v.login_need) list.push(h('div', { class: 'note bad' }, v.error));
    if (v.warn_until) list.push(h('div', { class: 'note warn' }, `The players are being warned in the chat: the server ${v.warn_what} in ${fmtDur(Math.max(0, v.warn_until - now()))}. Cancel stops it.`));
    if (v.update) list.push(h('div', { class: 'note orange' }, v.autoupdate ? 'Steam has an update. The manager installs it now, warning the players first.' : 'Steam has an update. Automatic updates are off for this server: click Update.'));
    if (up && !v.info && v.status === 'starting') list.push(h('div', { class: 'note info' }, 'Waiting for the server to answer queries on ' + v.address + '…'));
    if (v.foreign) list.push(h('div', { class: 'note info' }, 'This server was started outside the manager, as another Windows user (the old manager ran as SYSTEM). The manager watches it, restarts it if it crashes and stops it through RCON, but Windows does not let it force the server to end if RCON fails. Run the manager as administrator or as the service for full control; the next time it starts the server itself, this goes away.'));
    if (v.notes) list.push(h('div', { class: 'note', style: 'background:var(--bg-2);white-space:pre-wrap' }, v.notes));
    swap(notes, list);

    if (up && v.players && v.players.length) {
      swap(players, h('table', { class: 'list' },
        h('thead', null, h('tr', null, h('th', null, 'Player'), h('th', { class: 'num' }, 'Score'), h('th', { class: 'num' }, 'Time'))),
        h('tbody', null, v.players.slice().sort((a, b) => (a.bot - b.bot) || (b.score - a.score) || (b.duration - a.duration)).map(p =>
          h('tr', null,
            h('td', null, p.name || h('span', { class: 'muted' }, p.bot ? '(bot)' : '(connecting)'), p.bot && p.name ? h('span', { class: 'muted' }, ' (bot)') : null),
            h('td', { class: 'num' }, p.no_score ? h('span', { class: 'muted' }, '—') : p.score),
            h('td', { class: 'num' }, p.bot && !p.duration ? h('span', { class: 'muted' }, '—') : fmtDur(p.duration)))))));
    } else {
      swap(players, h('div', { class: 'muted small', style: 'padding:16px' }, up ? 'Nobody is playing.' : 'The server is not running.'));
    }
  }

  function renderRecent() {
    const evs = S.events.filter(e => e.server === id).slice(0, 8);
    swap(recent, evs.length ? evs.map(eventRow) : h('div', { class: 'muted small', style: 'padding:16px' }, 'Nothing yet.'));
  }

  render();
  renderRecent();
  const v0 = serverBy(id);
  const el = h('div', { class: 'grid2' },
    h('div', { class: 'col', style: 'gap:18px' }, loginWrap, notes, stats,
      h('div', { class: 'panel' }, h('div', { class: 'hd' }, icon('users'), h('h2', null, 'Players')), players),
      can('f') ? h('div', { class: 'panel' }, h('div', { class: 'hd' }, icon('terminal'), h('h2', { class: 'grow' }, 'Command line'),
        h('button', { class: 'btn sm ghost', onclick: () => copyText(cmd.textContent) }, icon('copy'), 'Copy')),
      h('div', { class: 'bd' }, cmd, h('div', { class: 'hint', style: 'margin-top:8px' }, 'Folder: ', h('span', { class: 'mono' }, v0.install_dir)))) : null),
    h('div', { class: 'panel' }, h('div', { class: 'hd' }, h('h2', { class: 'grow' }, 'Recent activity'),
      h('a', { class: 'small', href: '#/server/' + encodeURIComponent(id) + '/activity' }, 'All')), recent));
  const timer = setInterval(render, 1000);
  loadUsage();
  const useTimer = setInterval(() => { if (!document.hidden) loadUsage(); }, 30000);
  return { el, update: render, onEvent: renderRecent, destroy: () => { clearInterval(timer); clearInterval(useTimer); } };
}

/* ---------- Console ---------- */

function lineClass(l) {
  if (l.startsWith('] ')) return 'c-cmd';
  if (l.startsWith('──')) return 'c-sys';
  if (l.startsWith('[steamcmd]') || l.startsWith('[copy]') || l.startsWith('[cache]')) return /error|fail/i.test(l) ? 'c-err' : 'c-steam';
  if (l.startsWith('[rcon]')) return 'c-err';
  if (/\b(error|failed|fatal|crash|exception|could not|couldn't)\b|invalid map name|not found on server/i.test(l)) return 'c-err';
  if (/\bwarn(ing)?\b/i.test(l)) return 'c-warn';
  if (/"[^"]*<\d+><[^>]*><[^>]*>" say(_team)? "/.test(l)) return 'c-chat';
  if (/^L \d\d\/\d\d\/\d{4} - /.test(l)) return 'c-dim';
  return '';
}

function tabConsole(id) {
  const v0 = serverBy(id);
  const pre = h('pre', { tabindex: '0', 'aria-label': 'Console', 'aria-live': 'off' });
  const filterIn = h('input', { type: 'search', placeholder: 'Filter…', style: 'width:180px;height:30px' });
  let follow = true;
  const followBtn = h('button', { class: 'btn sm', title: 'Follow new lines', onclick: () => { follow = !follow; followBtn.classList.toggle('primary', follow); if (follow) pre.scrollTop = pre.scrollHeight; } }, icon('arrowdown'), 'Follow');
  followBtn.classList.add('primary');
  const MAX = 3000;
  let filterText = '';

  function addLines(lines) {
    if (!Array.isArray(lines)) return; // a new server has no console yet
    const atBottom = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 40;
    const frag = document.createDocumentFragment();
    for (const l of lines) {
      const sp = document.createElement('span');
      const c = lineClass(l);
      if (c) sp.className = c;
      sp.textContent = l + '\n';
      if (filterText && !l.toLowerCase().includes(filterText)) sp.hidden = true;
      frag.append(sp);
    }
    pre.append(frag);
    while (pre.childNodes.length > MAX) pre.removeChild(pre.firstChild);
    if (follow && (atBottom || lines.length > 200)) pre.scrollTop = pre.scrollHeight;
  }

  filterIn.addEventListener('input', debounce(() => {
    filterText = filterIn.value.toLowerCase();
    for (const sp of pre.children) sp.hidden = !!filterText && !sp.textContent.toLowerCase().includes(filterText);
  }, 150));
  pre.addEventListener('scroll', () => {
    const atBottom = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 40;
    if (!atBottom && follow) { follow = false; followBtn.classList.remove('primary'); }
    if (atBottom && !follow) { follow = true; followBtn.classList.add('primary'); }
  });

  const consoleURL = 'api/servers/' + encodeURIComponent(id) + '/console';
  let es = null;
  let poll = null;
  let next = 0;
  const pollConsole = () => {
    if (poll) return;
    if (es) { es.close(); es = null; }
    const tick = async () => {
      try {
        const r = await api('GET', consoleURL + '?since=' + next);
        if (next === 0) { clear(pre); }
        if (r.lines.length) addLines(r.lines);
        if (next === 0) pre.scrollTop = pre.scrollHeight;
        next = r.next;
      } catch (e) { /* the state polling shows the connection */ }
    };
    poll = setInterval(tick, 1500);
    tick();
  };
  if (S.polling) {
    pollConsole();
  } else {
    es = new EventSource(consoleURL);
    let got = false;
    const watchdog = setTimeout(() => { if (!got) pollConsole(); }, 6000);
    es.addEventListener('init', e => { got = true; clearTimeout(watchdog); clear(pre); addLines(JSON.parse(e.data)); pre.scrollTop = pre.scrollHeight; });
    es.addEventListener('lines', e => addLines(JSON.parse(e.data)));
  }

  // Command input with history (↑ ↓)
  const histKey = 'hist.' + id;
  let hist = store.get(histKey, []);
  let hi = hist.length;
  const gc = S.gamesBy[(v0 || {}).game] || {};
  const noRcon = gc.engine === 'other' && gc.rcon === 'none';
  const inp = h('input', { type: 'text', placeholder: noRcon ? 'This game has no RCON the manager speaks: type its commands in its own window' : gc.engine === 'other' ? 'RCON command' : 'RCON command (status, changelevel de_dust2, say Hello…)', spellcheck: false, autocomplete: 'off', 'aria-label': 'RCON command', disabled: noRcon });
  const send = h('button', { class: 'btn primary', type: 'submit' }, icon('chevron'), 'Send');
  inp.addEventListener('keydown', e => {
    if (e.key === 'ArrowUp' && hi > 0) { hi--; inp.value = hist[hi]; e.preventDefault(); }
    if (e.key === 'ArrowDown') { if (hi < hist.length - 1) { hi++; inp.value = hist[hi]; } else { hi = hist.length; inp.value = ''; } e.preventDefault(); }
  });
  const form = h('form', {
    onsubmit: async e => {
      e.preventDefault();
      const cmd = inp.value.trim();
      if (!cmd) return;
      hist = hist.filter(x => x !== cmd).concat(cmd).slice(-50);
      store.set(histKey, hist);
      hi = hist.length;
      inp.value = '';
      send.disabled = true;
      try { await api('POST', 'api/servers/' + encodeURIComponent(id) + '/rcon', { command: cmd }); } catch (ex) { toast(ex.message, 'bad'); }
      send.disabled = false;
      inp.focus();
    },
  }, inp, send);

  const g = S.gamesBy[v0.game] || {};
  // the usual commands of its engine (CS2: no 'maps *', it prints nothing there and crashes it with CounterStrikeSharp)
  const quick = {
    goldsrc: ['status', 'users', 'amxx plugins', 'meta list', 'say '],
    source2: ['status', 'css_plugins list', 'meta list', 'say '],
    other: [],
  }[g.engine] || ['status', 'maps *', 'sm plugins list', 'meta list', 'say '];
  const quickEl = h('div', { class: 'quick' }, quick.map(q => h('button', {
    class: 'chip', type: 'button',
    onclick: () => { if (q.endsWith(' ')) { inp.value = q; inp.focus(); } else { inp.value = q; form.requestSubmit(); } },
  }, q.trim() + (q.endsWith(' ') ? '…' : ''))));

  const note = h('div');
  function update() {
    const v = serverBy(id);
    if (!v) return;
    const list = [];
    if (!v.console_log) list.push(h('div', { class: 'note info', style: 'margin-bottom:12px' }, 'The console log (-condebug) is off for this server: only RCON answers and the manager\'s messages show here. ', can('f') ? h('a', { href: '#/server/' + encodeURIComponent(id) + '/settings' }, 'Turn it on in Settings') : null, '.'));
    if (!isUp(v)) list.push(h('div', { class: 'note', style: 'margin-bottom:12px;background:var(--bg-2)' }, 'The server is not running: commands can not be sent.'));
    swap(note, list);
    inp.disabled = send.disabled = !isUp(v);
  }
  update();

  const el = h('div', null, note, h('div', { class: 'console' },
    h('div', { class: 'bar2' }, quickEl, h('span', { class: 'spacer' }), filterIn, followBtn,
      h('button', { class: 'btn sm ghost', title: 'Clear the view', onclick: () => clear(pre) }, icon('trash'))),
    pre, form));
  setTimeout(() => inp.focus(), 50);
  return { el, update, destroy: () => { if (es) es.close(); if (poll) clearInterval(poll); } };
}

/* ---------- Files ---------- */

// tabFiles: the server's files; with two panes (as Total Commander), another server's next to them (of any
// machine), and what is ticked on one side is copied or moved straight into the folder open on the other.
function tabFiles(id) {
  const wrap = h('div');
  let dual = store.get('filesDual', false);
  // two panes only where they fit side by side (the choice is kept for a wider screen)
  const wide = window.matchMedia('(min-width: 1301px)');
  const onWidth = () => { if (!document.body.contains(wrap)) { wide.removeEventListener('change', onWidth); return; } draw(); };
  wide.addEventListener('change', onWidth);
  const toggle = () => !wide.matches ? null : h('button', { class: 'btn sm' + (dual ? ' on' : ''), 'aria-pressed': String(dual), onclick: () => { dual = !dual; store.set('filesDual', dual); draw(); },
    title: dual ? 'One pane' : 'Two panes: another server\'s files next to these, to copy or move between them' }, icon('columns'), dual ? 'One pane' : 'Two panes');
  let live = []; // the file managers shown: their keys let go when redrawn
  function draw() {
    live.forEach(b => b.destroy());
    const two = dual && wide.matches;
    const left = fileBrowser('api/servers/' + encodeURIComponent(id), 'path.' + id, null, { tools: toggle(), side: two ? 'left' : '' });
    live = [left];
    if (!two) { swap(wrap, left.el); return; }
    // the other side: the server picked last, else another of the same game, else this one
    const others = S.servers.filter(v => v.id !== id);
    const me = serverBy(id) || {};
    let other = store.get('filesDualWith.' + id, '');
    if (!serverBy(other)) other = (others.find(v => v.game === me.game) || others[0] || me).id;
    const rightBox = h('div', { class: 'fpane' });
    const pick = h('select', { 'aria-label': 'Server of the right pane', style: 'height:32px;max-width:260px' },
      groupByMachine(S.servers).map(([mname, list]) => {
        const opts = list.map(v => h('option', { value: v.id, selected: v.id === other }, v.name + (v.id === id ? ' (this one)' : '')));
        return mname ? h('optgroup', { label: mname }, opts) : opts;
      }));
    let right;
    const drawRight = () => {
      if (right) right.destroy();
      right = fileBrowser('api/servers/' + encodeURIComponent(other), 'path2.' + id + '.' + other, null, { head: pick, side: 'right' });
      live = [left, right];
      right.setPeer(left);
      left.setPeer(right);
      swap(rightBox, right.el);
    };
    pick.addEventListener('change', () => { other = pick.value; store.set('filesDualWith.' + id, other); drawRight(); });
    drawRight();
    swap(wrap, h('div', { class: 'fdual' }, h('div', { class: 'fpane' }, left.el), rightBox));
    left.activate();
  }
  draw();
  return { el: wrap };
}

// groupByMachine: servers by machine ([machine name, servers]); one group without a name when there is one machine
function groupByMachine(list) {
  if (!S.machines) return [['', list]];
  const out = new Map();
  for (const v of list) {
    const n = machineName(v.machine || '');
    if (!out.has(n)) out.set(n, []);
    out.get(n).push(v);
  }
  return [...out];
}

// the file manager the keys act on (Ctrl+C, Ctrl+V…): the last one clicked, when two are on the page
let FB_ACTIVE = null;

// The file manager's clipboard: what was copied (or cut), from which place and folder. Kept while the tab is open.
let FILECLIP = (() => { try { return JSON.parse(sessionStorage.getItem('szm.clip') || 'null'); } catch (e) { return null; } })();
function setFileClip(c) {
  FILECLIP = c;
  try { c ? sessionStorage.setItem('szm.clip', JSON.stringify(c)) : sessionStorage.removeItem('szm.clip'); } catch (e) { /* */ }
  window.dispatchEvent(new Event('szmclip'));
}
// trackPasteJob: a copy between machines, shown with its progress (bottom of the page, whatever page) until done.
function trackPasteJob(id, cut, onDone) {
  let box = document.querySelector('.jobs');
  if (!box) { box = h('div', { class: 'jobs' }); document.body.append(box); }
  const label = h('div', { class: 'small ellipsis' }, (cut ? 'Moving' : 'Copying') + '…');
  const bar = h('i', { style: 'width:2%' });
  const amount = h('span', { class: 'muted small nowrap' });
  const el = h('div', { class: 'job' }, h('div', { class: 'row', style: 'gap:10px' }, icon(cut ? 'scissors' : 'copy'), h('div', { class: 'grow', style: 'min-width:0' }, label), amount),
    h('div', { class: 'bar striped' }, bar));
  box.append(el);
  const tick = async () => {
    let j;
    try { j = await api('GET', 'api/paste/jobs/' + encodeURIComponent(id)); } catch (e) { el.remove(); toast(e.message, 'bad'); return; }
    label.textContent = (cut ? 'Moving' : 'Copying') + ' from ' + j.from + ' to ' + j.to;
    const pct = j.total ? Math.min(100, j.done / j.total * 100) : 0;
    bar.style.width = Math.max(2, pct).toFixed(1) + '%';
    amount.textContent = fmtBytes(j.done) + (j.total ? ' / ' + fmtBytes(j.total) : '');
    if (!j.finished) { setTimeout(tick, 1000); return; }
    el.remove();
    if (!box.children.length) box.remove();
    const what = (cut ? 'Moved ' : 'Pasted ') + j.items + ' item' + (j.items === 1 ? '' : 's') + ' to ' + j.to + ' (' + j.files + ' file' + (j.files === 1 ? '' : 's') + ', ' + fmtBytes(j.done) + ').';
    if (j.error) toast('Copy to ' + j.to + ' failed: ' + j.error, 'bad');
    else if (j.failed && j.failed.length) toast(what + ' Not: ' + j.failed.join('; '), 'bad');
    else toast(what, 'ok');
    if (onDone) onDone(j);
  };
  tick();
}

// placeOfBase: the place a file manager works in, from its API address.
function placeOfBase(base) {
  // on a hub: the machine it is on, named in the label
  const on = (machine, label) => S.machines ? machineName(machine) + ': ' + label : label;
  let m = base.match(/^api\/servers\/(.+)$/);
  if (m) {
    const id = decodeURIComponent(m[1]); const v = serverBy(id);
    const machine = id.includes('~') ? id.split('~')[0] : '';
    return { kind: 'server', id, machine, label: on(machine, v ? v.name : id) };
  }
  m = base.match(/^api\/addons\/pkg\/(.+)$/);
  if (m) { const id = decodeURIComponent(m[1]); return { kind: 'pkg', id, machine: S.ctxMachine, label: on(S.ctxMachine, 'package ' + id) }; }
  return { kind: 'cache', id: '', machine: S.ctxMachine, label: on(S.ctxMachine, 'shared copies') };
}

// fileBrowser is the file manager of a folder the API serves under base (a server's, an addon package's). Columns
// sort both ways; a "logs" folder shows the newest first. opts: head and tools (toolbar additions), side (one of two
// panes).
function fileBrowser(base, key, rootLabel, opts) {
  opts = opts || {};
  const id = base;
  let peer = null; // the other pane (two panes): ticked entries are copied or moved into its folder
  let path = store.get(key, '');
  let entries = [];
  let isLogs = false;
  let logsDays = 0;
  let canClean = false;
  let sort = { by: 'name', dir: 1 };
  const picked = new Set();
  const place = placeOfBase(base);
  const clipBar = h('div');
  const crumbs = h('div', { class: 'crumbs' });
  const tbody = h('tbody');
  const thead = h('thead');
  const info = h('div', { class: 'muted small', style: 'overflow-wrap:anywhere' });
  const upl = h('div');
  const selBar = h('div');
  const logsNote = h('div');
  const fileInput = h('input', { type: 'file', multiple: true, class: 'hidden', onchange: () => { uploadFiles([...fileInput.files]); fileInput.value = ''; } });
  const join = (a, b) => (a ? a + '/' : '') + b;
  const sortKey = () => 'fsort' + (isLogs ? '.logs' : '');

  async function load(p) {
    try {
      const r = await api('GET', base + '/files?path=' + encodeURIComponent(p));
      const moved = r.path !== path;
      if (moved) picked.clear();
      path = r.path;
      store.set(key, path);
      isLogs = 'logs_days' in r;
      logsDays = r.logs_days || 0;
      canClean = !!r.logs;
      sort = store.get(sortKey(), isLogs ? { by: 'mtime', dir: -1 } : { by: 'name', dir: 1 });
      entries = r.entries;
      for (const n of [...picked]) if (!entries.some(e => e.name === n)) picked.delete(n);
      info.textContent = r.root + (path ? '/' + path : '');
      renderCrumbs();
      render();
      // another folder: its top in view (the same one, refreshed: the page stays)
      if (moved && zone.getBoundingClientRect().top < 70) window.scrollTo(0, Math.max(0, window.scrollY + zone.getBoundingClientRect().top - 140));
    } catch (e) {
      if (p) { path = ''; load(''); return; }
      swap(tbody, h('tr', null, h('td', { colspan: '5' }, h('div', { class: 'empty' }, icon('folder'), h('p', null, e.message)))));
    }
  }

  // the path, as Windows shows it: when it is too long for its room, its first folders go into a … menu
  function renderCrumbs() {
    const parts = path ? path.split('/') : [];
    const items = [{ label: rootLabel || 'Server folder', to: '', root: true }].concat(parts.map((p, i) => ({ label: p, to: parts.slice(0, i + 1).join('/') })));
    const link = it => h('a', { href: '#', title: it.root ? info.textContent : it.label, onclick: e => { e.preventDefault(); load(it.to); } }, it.root ? icon('folder') : null, h('span', { class: 'ellipsis' }, it.label));
    const draw = hidden => {
      const kids = [];
      if (hidden) kids.push(h('button', { type: 'button', class: 'more', title: 'More folders', 'aria-label': 'More folders', onclick: e => crumbMenu(e.currentTarget, items.slice(0, hidden)) }, '…'), h('span', null, '/'));
      items.slice(hidden).forEach((it, k) => { if (k) kids.push(h('span', null, '/')); kids.push(link(it)); });
      swap(crumbs, kids);
    };
    let hidden = 0;
    draw(0);
    // as many first folders hidden as it takes to fit; the last one always shows
    while (crumbs.scrollWidth > crumbs.clientWidth + 1 && hidden < items.length - 1) draw(++hidden);
  }
  function crumbMenu(btn, list) {
    const old = document.querySelector('.menu.crumbmenu');
    if (old) { old.remove(); return; }
    const r = btn.getBoundingClientRect();
    const menu = h('div', { class: 'menu crumbmenu', role: 'menu', style: `position:fixed;left:${Math.round(r.left)}px;top:${Math.round(r.bottom + 6)}px;right:auto` },
      list.map(it => h('button', { type: 'button', role: 'menuitem', title: it.label, onclick: () => { close(); load(it.to); } }, icon('folder'), h('span', { class: 'ellipsis' }, it.label))));
    const off = e => { if (!menu.contains(e.target) && e.target !== btn) close(); };
    const close = () => { menu.remove(); document.removeEventListener('click', off, true); window.removeEventListener('scroll', close, true); };
    document.body.append(menu);
    setTimeout(() => { document.addEventListener('click', off, true); window.addEventListener('scroll', close, true); }, 0);
  }

  function sorted() {
    const cmpName = (a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' });
    const by = { name: cmpName, size: (a, b) => (a.size - b.size) || cmpName(a, b), mtime: (a, b) => (a.mtime - b.mtime) || cmpName(a, b) }[sort.by] || cmpName;
    // folders first, whatever the order
    return entries.slice().sort((a, b) => (a.dir !== b.dir ? (a.dir ? -1 : 1) : by(a, b) * sort.dir));
  }

  function setSort(by) {
    sort = sort.by === by ? { by, dir: -sort.dir } : { by, dir: by === 'name' ? 1 : -1 };
    store.set(sortKey(), sort);
    render();
  }

  function render() {
    const th = (by, label, cls) => h('th', { class: (cls || '') + ' sortable' + (sort.by === by ? ' on' : ''), 'aria-sort': sort.by === by ? (sort.dir > 0 ? 'ascending' : 'descending') : 'none' },
      h('button', { type: 'button', onclick: () => setSort(by), title: 'Sort by ' + label.toLowerCase() }, label, h('span', { class: 'arrow' }, sort.by === by ? (sort.dir > 0 ? '▲' : '▼') : '')));
    const all = h('input', { type: 'checkbox', 'aria-label': 'Select all', checked: entries.length > 0 && picked.size === entries.length,
      onchange: e => { picked.clear(); if (e.target.checked) entries.forEach(x => picked.add(x.name)); render(); } });
    all.indeterminate = picked.size > 0 && picked.size < entries.length;
    swap(thead, h('tr', null, h('th', { class: 'pick' }, all), th('name', 'Name'), th('size', 'Size', 'num'), th('mtime', 'Modified', 'hide-m'), h('th')));
    const rows = [];
    if (path) rows.push(h('tr', null, h('td', { class: 'pick' }), h('td', { class: 'name', onclick: () => load(path.split('/').slice(0, -1).join('/')) }, h('div', null, icon('up', 'd'), '..')), h('td'), h('td', { class: 'hide-m' }), h('td')));
    for (const e of sorted()) {
      const full = join(path, e.name);
      const open = () => e.dir ? load(full) : e.text ? openEditor(id, full) : download(full);
      rows.push(h('tr', { class: picked.has(e.name) ? 'picked' : '' },
        h('td', { class: 'pick' }, h('input', { type: 'checkbox', checked: picked.has(e.name), 'aria-label': 'Select ' + e.name,
          onchange: ev => { ev.target.checked ? picked.add(e.name) : picked.delete(e.name); render(); } })),
        h('td', { class: 'name', onclick: open, title: e.name }, h('div', null,
          icon(e.dir ? 'folder' : e.text ? 'filetext' : 'file', e.dir ? 'd' : 'f'), h('span', { class: 'ellipsis' }, e.name))),
        h('td', { class: 'num muted nowrap' }, e.dir ? '' : fmtBytes(e.size)),
        h('td', { class: 'muted nowrap hide-m' }, new Date(e.mtime * 1000).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })),
        h('td', { class: 'acts' },
          e.text ? h('button', { class: 'btn sm ghost icon', title: 'Edit', 'aria-label': 'Edit ' + e.name, onclick: () => openEditor(id, full) }, icon('edit')) : null,
          !e.dir ? h('button', { class: 'btn sm ghost icon', title: 'Download', 'aria-label': 'Download ' + e.name, onclick: () => download(full) }, icon('download')) : null,
          h('button', { class: 'btn sm ghost icon', title: 'Copy (then Paste where it goes)', 'aria-label': 'Copy ' + e.name, onclick: () => copyNames([e.name], false) }, icon('copy')),
          h('button', { class: 'btn sm ghost icon', title: 'Rename', 'aria-label': 'Rename ' + e.name, onclick: () => renameEntry(e) }, icon('wrench')),
          h('button', { class: 'btn sm ghost icon', title: 'Delete', 'aria-label': 'Delete ' + e.name, onclick: () => deleteEntry(e) }, icon('trash')))));
    }
    if (!entries.length) rows.push(h('tr', null, h('td', { colspan: '5', class: 'muted', style: 'text-align:center;padding:30px' }, 'Empty folder. Drop files here to upload them.')));
    // the page stays where it is (the list rebuilt would bring it up), and the box just ticked keeps the focus
    const y = window.scrollY;
    const focused = document.activeElement && zone.contains(document.activeElement) ? document.activeElement.getAttribute('aria-label') : null;
    swap(tbody, rows);
    renderSelBar();
    renderClip();
    if (focused) { const el = [...zone.querySelectorAll('[aria-label]')].find(x => x.getAttribute('aria-label') === focused); if (el) el.focus({ preventScroll: true }); }
    window.scrollTo(0, y);
  }

  function renderSelBar() {
    const files = entries.filter(e => !e.dir);
    const olderBtn = isLogs && files.length ? h('button', { class: 'btn sm', type: 'button', onclick: selectOlder, title: 'Tick the files older than a number of days' }, icon('clock'), 'Select older than…') : null;
    swap(logsNote, isLogs ? h('div', { class: 'note info logsnote' }, icon('clock'),
      h('span', { class: 'grow' }, logsDays > 0 ? 'Logs folder: newest first. Files older than ' + logsDays + ' days are deleted by themselves.' : 'Logs folder: newest first. Old logs can be deleted by themselves: Settings, Logs.',
        can('b') && canClean ? h('span', null, ' ', h('a', { href: '#/settings/logs' }, logsDays > 0 ? 'Change' : 'Set it')) : null),
      olderBtn) : null);
    swap(selBar, picked.size ? h('div', { class: 'selbar' },
      h('b', null, picked.size + ' selected'),
      h('span', { class: 'muted small' }, fmtBytes(entries.filter(e => picked.has(e.name) && !e.dir).reduce((t, e) => t + e.size, 0))),
      h('span', { class: 'spacer' }),
      h('button', { class: 'btn sm ghost', type: 'button', onclick: () => { picked.clear(); render(); } }, 'Clear'),
      peer ? null : h('button', { class: 'btn sm', type: 'button', title: 'Ctrl+C', onclick: () => copyNames([...picked], false) }, icon('copy'), 'Copy'),
      peer ? null : h('button', { class: 'btn sm', type: 'button', title: 'Ctrl+X: moved when pasted', onclick: () => copyNames([...picked], true) }, icon('scissors'), 'Cut'),
      peer ? [
        h('button', { class: 'btn sm primary', type: 'button', title: 'Copy into the folder open on the other side: ' + peer.where(), onclick: () => sendToPeer(false) },
          opts.side === 'right' ? [icon('arrowleft'), 'Copy'] : ['Copy', icon('arrowright')]),
        h('button', { class: 'btn sm', type: 'button', title: 'Move into the folder open on the other side: ' + peer.where(), onclick: () => sendToPeer(true) },
          opts.side === 'right' ? [icon('arrowleft'), 'Move'] : ['Move', icon('arrowright')])] : null,
      h('button', { class: 'btn sm danger', type: 'button', onclick: deletePicked }, icon('trash'), 'Delete ' + picked.size)) : null);
  }

  function copyNames(names, cut) {
    if (!names.length) return;
    setFileClip({ from: { kind: place.kind, id: place.id, machine: place.machine || '' }, label: place.label, dir: path, names, cut });
    picked.clear();
    render();
    toast((cut ? 'Cut ' : 'Copied ') + (names.length === 1 ? names[0] : names.length + ' items') + '. Go to the folder it goes in, then Paste.', 'ok');
  }

  function renderClip() {
    const c = FILECLIP;
    if (!c) { clear(clipBar); return; }
    const from = c.label + (c.dir ? ' / ' + c.dir : '');
    const here = c.from.kind === place.kind && c.from.id === place.id && (c.from.machine || '') === (place.machine || '') && c.dir === path;
    swap(clipBar, h('div', { class: 'clipbar' }, icon(c.cut ? 'scissors' : 'copy'),
      h('span', { class: 'grow ellipsis' }, h('b', null, c.names.length === 1 ? c.names[0] : c.names.length + ' items'), ' ' + (c.cut ? 'cut' : 'copied') + ' from ', h('span', { class: 'mono small' }, from)),
      h('button', { class: 'btn sm primary', type: 'button', title: 'Ctrl+V', disabled: here && c.cut, onclick: () => paste('') }, icon('download'), here ? 'Paste a copy here' : 'Paste here'),
      h('button', { class: 'btn sm ghost icon', type: 'button', title: 'Forget it', 'aria-label': 'Forget what was copied', onclick: () => setFileClip(null) }, icon('x'))));
  }

  function paste(conflict) {
    if (FILECLIP) pasteInto(FILECLIP, place, path, conflict, () => load(path), true);
  }

  // sendToPeer: the ticked entries, copied (or moved) into the folder open on the other side
  function sendToPeer(cut) {
    const names = [...picked];
    if (!names.length || !peer) return;
    const c = { from: { kind: place.kind, id: place.id, machine: place.machine || '' }, label: place.label, dir: path, names, cut };
    const to = peer.target();
    if (to.place.kind === place.kind && to.place.id === place.id && (to.place.machine || '') === (place.machine || '') && to.path === path && cut) {
      toast('Both sides show the same folder.', 'bad');
      return;
    }
    pasteInto(c, to.place, to.path, '', () => { picked.clear(); if (document.body.contains(zone)) load(path); peer.reload(); }, false);
  }

  // pasteInto: copies (or moves) what c names into a folder of a place: asks what to do with the names already there
  async function pasteInto(c, toPlace, toPath, conflict, done, fromClip) {
    let r;
    const slow = setTimeout(() => toast((c.cut ? 'Moving' : 'Copying') + '…'), 700); // said only when it takes time
    try {
      r = await api('POST', 'api/paste', { from: c.from, dir: c.dir, names: c.names, to: { kind: toPlace.kind, id: toPlace.id, machine: toPlace.machine || '' }, dest: toPath, cut: c.cut, conflict });
    } catch (e) { clearTimeout(slow); toast(e.message, 'bad'); return; }
    clearTimeout(slow);
    if (r.conflicts) {
      const m = modal({ title: r.conflicts.length === 1 ? r.conflicts[0] + ' is already there' : r.conflicts.length + ' are already there' });
      append(m.bd, h('p', { style: 'margin-top:0' }, 'In ', h('span', { class: 'mono' }, toPlace.label + (toPath ? ' / ' + toPath : '')), ':'),
        h('pre', { class: 'crashpre', style: 'max-height:160px' }, r.conflicts.join('\n')),
        h('p', { class: 'small muted' }, 'Replace: a folder gets the files pasted over its own (the others stay). Keep both: the pasted ones get a new name (2).'));
      const pick = v => { m.close(); pasteInto(c, toPlace, toPath, v, done, fromClip); };
      append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'),
        h('button', { class: 'btn', onclick: () => pick('skip') }, 'Skip them'),
        h('button', { class: 'btn', onclick: () => pick('rename') }, 'Keep both'),
        h('button', { class: 'btn danger', onclick: () => pick('overwrite') }, 'Replace'));
      return;
    }
    if (c.cut && fromClip) setFileClip(null);
    if (r.job) { // between machines: in the background, followed here
      trackPasteJob(r.job, c.cut, () => { if (document.body.contains(zone)) done(); });
      return;
    }
    const said = (c.cut ? 'Moved ' : fromClip ? 'Pasted ' : 'Copied ') + r.items + ' item' + (r.items === 1 ? '' : 's') + (r.files ? ' (' + r.files + ' file' + (r.files === 1 ? '' : 's') + (r.bytes ? ', ' + fmtBytes(r.bytes) : '') + ')' : '') + '.';
    if (r.failed && r.failed.length) toast(said + ' Not: ' + r.failed.join('; '), 'bad'); else toast(said, 'ok');
    done();
  }

  // Ctrl+C, Ctrl+X, Ctrl+V while it is on the page (not in a text field)
  function onKey(e) {
    if (!document.body.contains(zone)) { document.removeEventListener('keydown', onKey); window.removeEventListener('szmclip', renderClip); return; }
    if (!(e.ctrlKey || e.metaKey) || e.altKey || document.querySelector('.modal-back')) return;
    if (FB_ACTIVE && FB_ACTIVE !== el && document.body.contains(FB_ACTIVE)) return; // the other pane's
    const t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName) && t.type !== 'checkbox')) return;
    if (String(window.getSelection() || '')) return;
    const k = e.key.toLowerCase();
    if ((k === 'c' || k === 'x') && picked.size) { e.preventDefault(); copyNames([...picked], k === 'x'); }
    else if (k === 'v' && FILECLIP) { e.preventDefault(); paste(''); }
  }

  async function selectOlder() {
    const v = await promptBox('Select older than', 'Days', String(logsDays || 30), 'Select');
    if (v == null) return;
    const days = Number(v);
    if (!(days >= 0)) { toast('A number of days.', 'bad'); return; }
    const cut = Date.now() / 1000 - days * 86400;
    picked.clear();
    entries.forEach(e => { if (!e.dir && e.mtime < cut) picked.add(e.name); });
    render();
    toast(picked.size ? picked.size + ' file' + (picked.size > 1 ? 's' : '') + ' older than ' + days + ' days selected.' : 'No file is older than ' + days + ' days.', picked.size ? 'ok' : '');
  }

  async function deletePicked() {
    const names = [...picked];
    const dirs = names.filter(n => (entries.find(e => e.name === n) || {}).dir).length;
    const ok = await confirmBox('Delete ' + names.length + ' item' + (names.length > 1 ? 's' : '') + '?',
      (dirs ? dirs + ' folder' + (dirs > 1 ? 's' : '') + ' and everything in ' + (dirs > 1 ? 'them' : 'it') + ', ' : '') + 'This can not be undone.', 'Delete', true);
    if (!ok) return;
    const r = await run(() => api('POST', base + '/delete', { paths: names.map(n => join(path, n)) }));
    if (!r) return;
    picked.clear();
    if (r.failed && r.failed.length) toast(r.deleted + ' deleted; not: ' + r.failed.join('; '), 'bad'); else toast(r.deleted + ' deleted.', 'ok');
    load(path);
  }

  function download(p) {
    const a = h('a', { href: machineURL(base + '/file?download=1&path=' + encodeURIComponent(p)), download: '' });
    document.body.append(a);
    a.click();
    a.remove();
  }

  async function renameEntry(e) {
    const name = await promptBox('Rename', 'New name', e.name, 'Rename');
    if (!name || name === e.name) return;
    if (await run(() => api('POST', base + '/rename', { from: join(path, e.name), to: join(path, name) }), 'Renamed.')) load(path);
  }

  async function deleteEntry(e) {
    const ok = await confirmBox('Delete ' + e.name + '?', e.dir ? 'The folder and everything in it are deleted. This can not be undone.' : 'The file is deleted. This can not be undone.', 'Delete', true);
    if (ok && await run(() => api('DELETE', base + '/file?path=' + encodeURIComponent(join(path, e.name))), 'Deleted.')) load(path);
  }

  async function newFolder() {
    const name = await promptBox('New folder', 'Name', '', 'Create');
    if (name && await run(() => api('POST', base + '/mkdir', { path: join(path, name) }))) load(join(path, name));
  }

  async function newFile() {
    const name = await promptBox('New file', 'Name', 'new.cfg', 'Create');
    if (!name) return;
    if (await run(() => api('PUT', base + '/file', { path: join(path, name), content: '', crlf: true }))) { load(path); openEditor(id, join(path, name)); }
  }

  function uploadFiles(files) {
    if (!files.length) return;
    const fd = new FormData();
    let total = 0;
    for (const f of files) { fd.append('file', f, f.name); total += f.size; }
    const bar = h('i', { style: 'width:0%' });
    const label = h('span', { class: 'grow ellipsis' }, `Uploading ${files.length} file${files.length > 1 ? 's' : ''} (${fmtBytes(total)})…`);
    swap(upl, h('div', { class: 'panel', style: 'padding:10px 14px;margin-bottom:12px' }, h('div', { class: 'row small', style: 'margin-bottom:6px' }, label), h('div', { class: 'bar' }, bar)));
    const xhr = new XMLHttpRequest();
    xhr.open('POST', machineURL(base + '/upload?path=' + encodeURIComponent(path)));
    xhr.setRequestHeader('X-SZM', '1');
    xhr.upload.onprogress = e => { if (e.lengthComputable) bar.style.width = (e.loaded / e.total * 100).toFixed(1) + '%'; };
    xhr.onload = () => {
      clear(upl);
      let d = {};
      try { d = JSON.parse(xhr.responseText); } catch (e) { /* */ }
      if (xhr.status === 200) { toast(`Uploaded ${d.files ? d.files.length : files.length} file(s).`, 'ok'); load(path); } else toast(d.error || 'Upload failed.', 'bad');
    };
    xhr.onerror = () => { clear(upl); toast('Upload failed.', 'bad'); };
    xhr.send(fd);
  }

  const table = h('table', { class: 'list files' }, thead, tbody);
  const zone = h('div', { class: 'panel dropzone', style: 'overflow:auto' }, table);
  let depth = 0;
  zone.addEventListener('dragenter', e => { if (e.dataTransfer.types.includes('Files')) { e.preventDefault(); depth++; zone.classList.add('over'); } });
  zone.addEventListener('dragover', e => { if (e.dataTransfer.types.includes('Files')) e.preventDefault(); });
  zone.addEventListener('dragleave', () => { depth = Math.max(0, depth - 1); if (!depth) zone.classList.remove('over'); });
  zone.addEventListener('drop', e => { e.preventDefault(); depth = 0; zone.classList.remove('over'); uploadFiles([...e.dataTransfer.files]); });

  load(path);
  // the path fitted again when its room changes (the window, the other pane)
  if (window.ResizeObserver) {
    let w = 0;
    new ResizeObserver(() => { if (crumbs.clientWidth && crumbs.clientWidth !== w) { w = crumbs.clientWidth; renderCrumbs(); } }).observe(crumbs);
  }
  // the toolbar's buttons: only their icon when two panes share the width
  const tbtn = (cls, ic, label, fn, tip) => h('button', { class: 'btn sm' + cls + (opts.side ? ' icon' : ''), onclick: fn, title: tip || label, 'aria-label': tip || label },
    icon(ic), opts.side ? null : label);
  const el = h('div', null,
    h('div', { class: 'toolbar ftool' }, opts.head || null, crumbs, opts.tools || null,
      tbtn('', 'refresh', 'Refresh', () => load(path)),
      tbtn('', 'folderplus', 'Folder', newFolder, 'New folder'),
      tbtn('', 'fileplus', 'File', newFile, 'New file'),
      tbtn(' primary', 'upload', 'Upload', () => fileInput.click(), 'Upload files (or drop them on the list)'), fileInput),
    logsNote, upl, clipBar, selBar, zone, h('div', { style: 'margin-top:8px' }, info));
  document.addEventListener('keydown', onKey);
  window.addEventListener('szmclip', renderClip);
  const activate = () => { FB_ACTIVE = el; };
  el.addEventListener('pointerdown', activate);
  el.addEventListener('focusin', activate);
  return {
    el, go: p => { picked.clear(); load(p); }, activate,
    setPeer: p => { peer = p; renderSelBar(); },
    target: () => ({ place, path }),
    where: () => place.label + (path ? ' / ' + path : ''),
    reload: () => { if (document.body.contains(zone)) load(path); },
    destroy: () => { document.removeEventListener('keydown', onKey); window.removeEventListener('szmclip', renderClip); },
  };
}

async function openEditor(base, path, onSaved) {
  let file;
  try { file = await api('GET', base + '/file?path=' + encodeURIComponent(path)); } catch (e) { toast(e.message, 'bad'); return; }
  let dirty = false;
  const ta = h('textarea', { spellcheck: false, value: file.content, 'aria-label': path });
  const state = h('span', { class: 'muted small' }, file.crlf ? 'Windows line ends' : 'Unix line ends');
  const saveBtn = h('button', { class: 'btn primary sm' }, icon('save'), 'Save');
  const m = modal({
    title: h('span', { class: 'mono', style: 'font-size:14px' }, path), size: 'full', buttons: false, locked: true,
  });
  // Closing with changes asks first
  const close = m.close;
  const tryClose = async () => { if (!dirty || await confirmBox('Close without saving?', 'Your changes to ' + path + ' are lost.', 'Close', true)) close(); };
  m.hd.lastChild.replaceWith(h('button', { class: 'btn ghost icon sm', title: 'Close', onclick: tryClose }, icon('x')));
  m.hd.insertBefore(h('div', { class: 'row' }, state, saveBtn), m.hd.lastChild);
  append(m.bd, h('div', { class: 'editor' }, ta));
  const save = async () => {
    saveBtn.disabled = true;
    try {
      const r = await api('PUT', base + '/file', { path, content: ta.value, crlf: file.crlf, mtime: file.mtime });
      file.mtime = r.mtime;
      dirty = false;
      state.textContent = 'Saved ' + new Date().toLocaleTimeString();
      toast('Saved ' + path.split('/').pop(), 'ok');
      if (onSaved) onSaved();
    } catch (e) { toast(e.message, 'bad'); }
    saveBtn.disabled = false;
  };
  saveBtn.onclick = save;
  ta.addEventListener('input', () => { if (!dirty) { dirty = true; state.textContent = 'Not saved'; } });
  ta.addEventListener('keydown', e => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') { e.preventDefault(); save(); }
    if (e.key === 'Tab' && !e.shiftKey) {
      e.preventDefault();
      const s = ta.selectionStart;
      ta.setRangeText('\t', s, ta.selectionEnd, 'end');
      ta.dispatchEvent(new Event('input'));
    }
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); tryClose(); }
  });
  setTimeout(() => ta.focus(), 30);
}

/* ---------- Maps ---------- */

function tabMaps(id) {
  const base = 'api/servers/' + encodeURIComponent(id);
  const left = h('div', { class: 'maplist' });
  const right = h('div', { class: 'maplist' });
  const countL = h('span', { class: 'muted small' });
  const countR = h('span', { class: 'muted small' });
  const search = h('input', { type: 'search', placeholder: 'Search maps…', style: 'height:32px' });
  const saveBtn = h('button', { class: 'btn primary sm', disabled: true }, icon('save'), 'Save');
  let data = { maps: [], cycle: [], cycle_supported: false, current: '' };
  let cycle = [];
  let dirty = false;
  let group = ''; // CS2: the map group edited (and played once saved)
  const markDirty = () => { dirty = true; saveBtn.disabled = !can('j'); renderRight(); };
  const canChange = () => can('g') && isUp(serverBy(id) || {});

  async function changeMap(name) {
    const v = serverBy(id);
    const n = humans(v);
    if (n > 0 && !await confirmBox('Change map to ' + name + '?', `${n} player${n > 1 ? 's are' : ' is'} on the server: the map changes now.`, 'Change map')) return;
    if (await run(() => api('POST', base + '/rcon', { command: 'changelevel ' + name }), 'Changing to ' + name + '…')) setTimeout(load, 4000);
  }

  function renderLeft() {
    const q = search.value.toLowerCase();
    const list = data.maps.filter(m => !q || m.toLowerCase().includes(q));
    countL.textContent = list.length + ' of ' + data.maps.length;
    if (!data.maps.length) { swap(left, h('div', { class: 'muted small', style: 'padding:14px' }, 'No map found in the maps folder (is the server installed?).')); return; }
    swap(left, list.map(m => h('div', { class: 'mapitem' + (m === data.current ? ' current' : '') },
      h('span', { class: 'n', title: m }, m), m === data.current ? h('span', { class: 'tag green' }, 'now') : null,
      data.cycle_supported && can('j') ? h('button', { class: 'btn sm ghost icon', title: 'Add to the cycle', 'aria-label': 'Add ' + m + ' to the cycle', onclick: () => { cycle.push(m); markDirty(); } }, icon('plus')) : null,
      canChange() ? h('button', { class: 'btn sm ghost icon', title: 'Change to this map now', 'aria-label': 'Change to ' + m, onclick: () => changeMap(m) }, icon('play')) : null)));
  }

  let dragFrom = -1;
  function renderRight() {
    countR.textContent = cycle.length + ' maps' + (dirty ? ' · not saved' : '');
    if (!cycle.length) { swap(right, h('div', { class: 'muted small', style: 'padding:14px' }, 'The cycle is empty: add maps from the list.')); return; }
    const gone = data.not_found || {};
    const nGone = cycle.filter(m => gone[m.toLowerCase()]).length;
    swap(right, nGone ? h('div', { class: 'note warn small', style: 'margin:8px' }, nGone === 1 ? '1 map of it is not on this server' : nGone + ' maps of it are not on this server',
      ' (its console said so when it tried to load ' + (nGone === 1 ? 'it' : 'them') + '). Remove ' + (nGone === 1 ? 'it' : 'them') + ', or add ' + (nGone === 1 ? 'its file' : 'their files') + '.') : null, cycle.map((m, i) => {
      const missing = gone[m.toLowerCase()];
      const row = h('div', { class: 'mapitem' + (m === data.current ? ' current' : ''), draggable: can('j') },
        can('j') ? h('span', { class: 'grip', title: 'Drag to move' }, icon('grip')) : null,
        h('span', { class: 'idx' }, i + 1), h('span', { class: 'n', title: m }, m),
        missing ? h('span', { class: 'tag red', title: 'Its console said it does not have this map, ' + new Date(missing * 1000).toLocaleString() }, 'not on this server') : null,
        canChange() ? h('button', { class: 'btn sm ghost icon', title: 'Change to this map now', 'aria-label': 'Change to ' + m, onclick: () => changeMap(m) }, icon('play')) : null,
        can('j') ? h('button', { class: 'btn sm ghost icon', title: 'Remove', 'aria-label': 'Remove ' + m, onclick: () => { cycle.splice(i, 1); markDirty(); } }, icon('x')) : null);
      row.addEventListener('dragstart', e => { dragFrom = i; row.classList.add('dragging'); e.dataTransfer.effectAllowed = 'move'; });
      row.addEventListener('dragend', () => row.classList.remove('dragging'));
      row.addEventListener('dragover', e => { e.preventDefault(); });
      row.addEventListener('drop', e => {
        e.preventDefault();
        if (dragFrom < 0 || dragFrom === i) return;
        const [x] = cycle.splice(dragFrom, 1);
        cycle.splice(i, 0, x);
        dragFrom = -1;
        markDirty();
      });
      return row;
    }));
  }

  async function load() {
    try {
      data = await api('GET', base + '/maps');
      if (!dirty) { cycle = (data.cycle || []).slice(); group = data.map_group || ''; }
      renderLeft();
      renderRight();
      if (cycleHead.isConnected) drawCycleHead();
      if (data.workshop) renderWorkshop();
    } catch (e) { toast(e.message, 'bad'); }
  }

  saveBtn.onclick = async () => {
    const r = await run(() => api('PUT', base + '/mapcycle', { maps: cycle, group }));
    if (!r) return;
    toast(r.group ? 'Map group ' + r.group + ' saved' + (isUp(serverBy(id) || {}) ? ': the server plays it from its next start.' : '.') : 'Map cycle saved.', 'ok');
    dirty = false; saveBtn.disabled = true;
    if (r.group) load(); else renderRight();
  };
  search.addEventListener('input', renderLeft);

  const v0 = serverBy(id);
  const g = S.gamesBy[v0.game] || {};
  const cyclePanel = h('div', { class: 'panel' });
  // CS2: which map group of gamemodes_server.txt; another one, or a new one, is played once saved
  function groupPicker() {
    const names = (data.map_groups || []).slice();
    if (group && !names.some(n => n.toLowerCase() === group.toLowerCase())) names.push(group);
    const sel = h('select', { style: 'height:30px;width:auto;max-width:150px', disabled: !can('j'), 'aria-label': 'Map group',
      title: 'The map group it plays (+mapgroup): the maps it rotates, and that the vote at the end of a match offers' },
      names.map(n => h('option', { value: n, selected: n === group }, n)), h('option', { value: '+' }, 'New group…'));
    sel.addEventListener('change', async () => {
      let n = sel.value;
      if (n === '+') {
        n = await promptBox('New map group', 'Its name: letters, digits and _ (mg_custom)', 'mg_', 'Create');
        n = n && n.trim();
        if (!n || !/^[A-Za-z0-9_.-]{1,64}$/.test(n)) { if (n) toast('Letters, digits and _ only.', 'bad'); sel.value = group; return; }
      }
      group = n;
      cycle = ((data.group_maps || {})[n] || []).slice();
      markDirty();
      drawCycleHead();
    });
    return sel;
  }
  const cycleHead = h('div', { class: 'hd' });
  const cycleNote = h('div', { class: 'bd col', style: 'border-top:1px solid var(--line)' });
  function drawCycleHead() {
    const cs2 = g.engine === 'source2';
    const played = data.map_group_set && (data.map_group || '').toLowerCase() === group.toLowerCase();
    if (cs2) swap(cycleNote, h('p', { style: 'margin:0', class: 'muted small' },
      'The maps it rotates, and that the vote at the end of a match offers. Saved into its gamemodes_server.txt (its other map groups stay as they are). ',
      played ? ['It plays it: ', h('b', { class: 'mono' }, '+mapgroup ' + group), ' in its launch options.']
        : ['Once saved, it plays it from its next start (', h('b', { class: 'mono' }, '+mapgroup ' + group), ' in its launch options).'],
      ' A Workshop collection (below) replaces it.'));
    swap(cycleHead, icon('restart'),
      h('div', { class: 'grow', style: 'min-width:0' }, h('h2', null, cs2 ? 'Map group' : 'Map cycle'),
        h('div', { class: 'muted small mono ellipsis', title: data.cycle_path }, cs2 ? data.cycle_path.split('/').pop() : data.cycle_path)),
      cs2 ? groupPicker() : null, countR,
          can('j') ? h('button', { class: 'btn sm ghost', onclick: () => { if (data.maps.length) { cycle = data.maps.slice(); markDirty(); } } }, 'All') : null,
          can('j') ? h('button', { class: 'btn sm ghost', onclick: () => { cycle = []; markDirty(); } }, 'Clear') : null, saveBtn);
  }
  load().then(() => {
    if (data.cycle_supported) {
      drawCycleHead();
      swap(cyclePanel, cycleHead,
        h('div', { style: 'padding:8px' }, right),
        g.engine === 'source2' ? cycleNote : null);
    } else {
      swap(cyclePanel, h('div', { class: 'hd' }, icon('info'), h('h2', null, 'Map rotation')),
        h('div', { class: 'bd col' },
          h('p', { style: 'margin:0', class: 'muted' }, 'This game has no map cycle file.')));
    }
  });

  // CS2 Workshop: maps added by ID or address (or a whole collection), played now, started on; a collection played
  const wsPanel = h('div', { class: 'panel', style: 'margin-top:16px' });
  const wsInput = h('input', { type: 'text', placeholder: 'Workshop map or collection: ID or address', spellcheck: false });
  const wsSearch = h('input', { type: 'search', placeholder: 'Search…', style: 'height:32px;max-width:220px' });
  const wsGrid = h('div', { class: 'wsgrid' });
  wsSearch.addEventListener('input', () => drawWsGrid());
  async function addWorkshop(e) {
    e.preventDefault();
    const v = wsInput.value.trim();
    if (!v) return;
    try {
      const r = await api('POST', base + '/workshop', { input: v });
      r.added = r.added || [];
      wsInput.value = '';
      toast(r.collection ? 'Collection ' + r.collection.title + ': ' + r.added.length + ' maps added' + (r.already ? ', ' + r.already + ' already there' : '') + '.'
        : r.added.length ? r.added[0].title + ' added.' : 'Already in the list.', 'ok');
      await load();
    } catch (err) { toast(err.message, 'bad'); }
  }
  async function playWorkshop(w) {
    const v = serverBy(id);
    const n = humans(v);
    if (n > 0 && !await confirmBox('Change map to ' + w.title + '?', `${n} player${n > 1 ? 's are' : ' is'} on the server: the map changes now. The first time, the server downloads it first (${fmtBytes(w.size || 0)}).`, 'Change map')) return;
    if (await run(() => api('POST', base + '/rcon', { command: 'host_workshop_map ' + w.id }), 'Loading ' + w.title + '…')) setTimeout(load, 6000);
  }
  function drawWsGrid() {
    const list = data.workshop || [];
    const q = wsSearch.value.toLowerCase();
    const shown = list.filter(w => !q || w.title.toLowerCase().includes(q) || w.id.includes(q));
    if (!list.length) { swap(wsGrid, h('div', { class: 'muted small', style: 'padding:6px 2px' }, 'No Workshop map yet. Paste one above: its ID, or its address on the Steam Workshop.')); return; }
    swap(wsGrid, shown.map(w => {
      const isStart = data.workshop_map === w.id;
      return h('div', { class: 'wscard' + (isStart ? ' start' : '') },
        h('a', { class: 'wsimg', href: 'https://steamcommunity.com/sharedfiles/filedetails/?id=' + w.id, target: '_blank', rel: 'noopener', title: 'On the Steam Workshop' },
          w.preview ? h('img', { src: w.preview, alt: '', loading: 'lazy' }) : icon('map')),
        h('div', { class: 'wsbody' },
          h('div', { class: 'wstitle', title: w.title }, w.title),
          h('div', { class: 'muted small mono' }, w.id + (w.size ? ' · ' + fmtBytes(w.size) : '')),
          isStart ? h('span', { class: 'tag blue', style: 'margin-top:4px' }, 'starts on it') : null),
        h('div', { class: 'wsacts' },
          canChange() ? h('button', { class: 'btn sm', title: 'Play it now (host_workshop_map)', onclick: () => playWorkshop(w) }, icon('play'), 'Play') : null,
          can('f') ? h('button', { class: 'btn sm ghost', title: isStart ? 'Start on its usual map again' : 'Start on this map (from the next start)',
            onclick: async () => { if (await run(() => api('PUT', base + '/workshop/start', { id: isStart ? '' : w.id }), isStart ? 'It starts on ' + (serverBy(id).map || 'its map') + ' again.' : 'It starts on ' + w.title + ' from its next start.')) load(); } },
            icon(isStart ? 'x' : 'power'), isStart ? 'Usual start' : 'Start on it') : null,
          can('j') ? h('button', { class: 'btn sm ghost icon', title: 'Remove from the list', 'aria-label': 'Remove ' + w.title,
            onclick: async () => { if (await run(() => api('DELETE', base + '/workshop/' + w.id), w.title + ' removed.')) load(); } }, icon('trash')) : null));
    }));
  }
  function renderWorkshop() {
    const coll = data.workshop_collection;
    const collInput = h('input', { type: 'text', placeholder: 'Collection ID or address', spellcheck: false });
    swap(wsPanel,
      h('div', { class: 'hd' }, icon('globe'), h('div', { class: 'grow' }, h('h2', null, 'Workshop maps'),
        h('div', { class: 'muted small' }, 'Paste a map or a collection (a collection adds all its maps). CS2 downloads a map itself the first time it plays it.')),
        (data.workshop || []).length > 6 ? wsSearch : null),
      h('div', { class: 'bd col', style: 'gap:14px' },
        can('j') ? h('form', { class: 'inputgroup', onsubmit: addWorkshop }, wsInput, h('button', { class: 'btn primary', type: 'submit' }, icon('plus'), 'Add')) : null,
        h('div', { class: 'wscoll' },
          h('div', { class: 'grow' }, h('b', null, 'Collection played: '), coll ? h('span', null, h('a', { href: 'https://steamcommunity.com/sharedfiles/filedetails/?id=' + coll, target: '_blank', rel: 'noopener' }, data.workshop_collection_title || coll)) : h('span', { class: 'muted' }, 'none'),
            h('div', { class: 'muted small' }, 'The server downloads the collection and plays its maps (+host_workshop_collection), from its next start.')),
          can('f') ? (coll
            ? h('button', { class: 'btn sm ghost', onclick: async () => { if (await run(() => api('PUT', base + '/workshop/collection', { input: '' }), 'No collection from the next start.')) load(); } }, icon('x'), 'Stop using it')
            : h('form', { class: 'inputgroup', style: 'max-width:420px', onsubmit: async e => {
              e.preventDefault();
              if (!collInput.value.trim()) return;
              const r = await run(() => api('PUT', base + '/workshop/collection', { input: collInput.value.trim() }), 'Collection set: from the next start.');
              if (r) load();
            } }, collInput, h('button', { class: 'btn', type: 'submit' }, 'Use'))) : null),
        wsGrid));
    drawWsGrid();
  }

  const el = h('div', null, h('div', { class: 'grid2 even' },
    h('div', { class: 'panel' },
      h('div', { class: 'hd' }, icon('map'), h('h2', { class: 'grow' }, 'Installed maps'), countL),
      h('div', { style: 'padding:10px 10px 0' }, search),
      h('div', { style: 'padding:8px' }, left)),
    cyclePanel), g.engine === 'source2' ? wsPanel : null);
  window.addEventListener('beforeunload', warnUnsaved);
  function warnUnsaved(e) { if (dirty) { e.preventDefault(); e.returnValue = ''; } }
  let lastMap = '';
  return {
    el,
    update: () => {
      const v = serverBy(id);
      const m = v && v.info ? v.info.map : '';
      if (m !== lastMap) { lastMap = m; data.current = m; renderLeft(); if (data.cycle_supported) renderRight(); }
    },
    destroy: () => window.removeEventListener('beforeunload', warnUnsaved),
  };
}

/* ---------- Settings of a server ---------- */

// Form fields bound to an object
function bindText(obj, key, attrs) {
  const inp = h('input', Object.assign({ type: 'text', value: obj[key] == null ? '' : obj[key], spellcheck: false }, attrs || {}));
  inp.addEventListener('input', () => { obj[key] = inp.type === 'number' ? (inp.value === '' ? 0 : Number(inp.value)) : inp.value; if (bindText.onChange) bindText.onChange(); });
  return inp;
}
function bindSelect(obj, key, options, onChange) {
  const enc = v => JSON.stringify(v === undefined ? null : v);
  const sel = h('select', null, options.map(([v, label]) => h('option', { value: enc(v), selected: enc(obj[key]) === enc(v) }, label)));
  sel.addEventListener('change', () => { obj[key] = JSON.parse(sel.value); if (onChange) onChange(); if (bindText.onChange) bindText.onChange(); });
  return sel;
}
function bindCheck(obj, key, title, desc, onChange) {
  const inp = h('input', { type: 'checkbox', checked: !!obj[key] });
  inp.addEventListener('change', () => { obj[key] = inp.checked; if (onChange) onChange(); if (bindText.onChange) bindText.onChange(); });
  return h('label', { class: 'check' }, inp, h('span', null, h('b', null, title), desc ? h('small', null, desc) : null));
}
function field(label, input, hint, wide) {
  return h('label', { class: 'field' + (wide ? ' wide' : '') }, h('span', null, label), input, hint ? h('small', null, hint) : null);
}
function secret(obj, key, attrs) {
  const inp = bindText(obj, key, Object.assign({ type: 'password', autocomplete: 'off' }, attrs || {}));
  const eye = h('button', { class: 'btn icon', type: 'button', title: 'Show', 'aria-label': 'Show', onclick: () => { inp.type = inp.type === 'password' ? 'text' : 'password'; } }, icon('eye'));
  return [inp, eye];
}

// packagePicker: the addon packages a server gets (its addon_packages), as chips to tick.
function packagePicker(f) {
  const wrap = h('div', null, h('div', { class: 'small', style: 'font-weight:600;color:var(--text-2);margin-bottom:8px' }, 'Addon packages it gets'));
  const chips = h('div', { class: 'chips' }, h('span', { class: 'muted small' }, 'Loading…'));
  const hint = h('div', { class: 'hint', style: 'margin-top:6px' }, 'Copied into the server by Update addons (or the Addons page), keeping its plugin configs.');
  append(wrap, chips, hint);
  if (!f.addon_packages) f.addon_packages = [];
  api('GET', 'api/addons').then(r => {
    const names = r.packages.map(p => p.name);
    f.addon_packages.forEach(n => { if (!names.includes(n)) names.push(n); });
    if (!names.length) {
      swap(chips, h('span', { class: 'muted small' }, 'No package yet. '), can('h') ? h('a', { href: '#/addons' }, 'Create one on the Addons page') : null);
      return;
    }
    swap(chips, names.map(n => {
      const on = f.addon_packages.includes(n);
      return h('button', { type: 'button', class: 'chip' + (on ? ' on' : ''), onclick: e => {
        f.addon_packages = f.addon_packages.includes(n) ? f.addon_packages.filter(x => x !== n) : f.addon_packages.concat(n).sort();
        e.currentTarget.classList.toggle('on');
        if (bindText.onChange) bindText.onChange();
      } }, icon('package'), n);
    }));
  }).catch(() => swap(chips, h('span', { class: 'muted small' }, 'Packages can not be listed.')));
  return wrap;
}

const PRIORITIES = ['real time', 'high', 'above normal', 'normal', 'below normal', 'idle'];

// hostLine: the server's hostname (what players see), under its name when it says something else
function hostLine(v, cls) {
  if (!v.hostname || v.hostname === v.name) return null;
  return h('div', { class: cls, title: v.hostname }, v.hostname);
}

// steamLoginBox: Steam wants something before the download goes on (a password, a Steam Guard code): ask for it,
// then download again with it. Nothing is saved but the account.
function steamLoginBox(v) {
  const need = v.login_need;
  if (!need) return null;
  const acct = v.steam_user || '';
  const account = h('input', { type: 'text', value: acct, spellcheck: false, autocomplete: 'off', placeholder: 'Steam account' });
  const pass = h('input', { type: 'password', autocomplete: 'new-password', placeholder: 'Password' });
  const code = h('input', { type: 'text', autocomplete: 'one-time-code', placeholder: 'Code', maxlength: 8, style: 'text-transform:uppercase;letter-spacing:.15em;font-family:var(--mono)' });
  const msg = {
    password: ['Steam wants the password of ' + (acct || 'the account'), 'It is used for this download only, not saved. steamcmd remembers the login after it works.'],
    bad_password: ['The password of ' + (acct || 'the account') + ' was wrong', 'Type it again.'],
    guard: ['Steam sent a Steam Guard code to the email of ' + (acct || 'the account'), 'Look in that mailbox (and its spam folder), then type the code here. The password is still known: only the code is needed.'],
    '2fa': ['Steam wants the code of the Steam app of ' + (acct || 'the account'), 'Open the Steam app on the phone, Steam Guard: type the code it shows now (it changes every 30 seconds).'],
    approve: ['The login was not approved in the Steam app in time', 'Click Try again, then approve it in the Steam app (the shield at the bottom, then Approve) within a minute. Or type the code the Steam app shows: no approval is needed then.'],
  }[need] || ['Steam wants something before the download goes on', ''];
  const askPass = need === 'password' || need === 'bad_password';
  const err = h('div');
  const go = h('button', { class: 'btn primary', onclick: async () => {
    go.disabled = true;
    clear(err);
    const body = { account: account.value.trim() };
    if (askPass) body.password = pass.value;
    if (need === 'guard') body.guard = code.value.trim().toUpperCase();
    if (need === '2fa' || (need === 'approve' && code.value.trim())) body.two_factor = code.value.trim().toUpperCase();
    try { await api('POST', 'api/servers/' + encodeURIComponent(v.id) + '/steamlogin', body); toast('Downloading again…', 'ok'); }
    catch (e) { swap(err, h('div', { class: 'note bad' }, e.message)); go.disabled = false; }
  } }, icon('download'), askPass ? 'Log in and download' : need === 'approve' ? 'Try again' : 'Send the code');
  const anon = h('button', { class: 'btn ghost sm', onclick: async () => {
    if (await run(() => api('POST', 'api/servers/' + encodeURIComponent(v.id) + '/steamlogin', { account: 'anonymous' }), 'Downloading again, anonymously…')) clear(err);
  } }, 'Use anonymous instead');
  [pass, code].forEach(x => x.addEventListener('keydown', e => { if (e.key === 'Enter') go.click(); }));
  if (need !== 'approve') setTimeout(() => (askPass ? (acct ? pass : account) : code).focus(), 50);
  return h('div', { class: 'note warn col', style: 'gap:10px' },
    h('div', null, h('b', null, msg[0]), msg[1] ? h('div', { class: 'small', style: 'margin-top:2px' }, msg[1]) : null),
    askPass ? h('div', { class: 'row wrap', style: 'gap:8px' }, h('div', { style: 'flex:1;min-width:180px' }, account), h('div', { style: 'flex:1;min-width:180px' }, pass))
      : h('div', { style: 'max-width:220px' }, need === 'approve' ? (code.placeholder = 'Code (optional)', code) : code),
    err, h('div', { class: 'row wrap', style: 'gap:8px' }, go, anon));
}

// approveNow: steamcmd waits (about a minute) for the login to be approved in the Steam app
function approveNow(v) {
  return h('div', { class: 'note warn col', style: 'gap:8px' },
    h('div', { class: 'row', style: 'gap:10px' }, h('span', { class: 'spin' }), h('b', null, 'Approve the login in the Steam app now')),
    h('div', { class: 'small' }, 'Steam asks to confirm this login' + (v.steam_user ? ' of ' + v.steam_user : '') + ': open the Steam app on the phone, tap the shield (Steam Guard) at the bottom, then Approve. steamcmd waits about a minute, then the download goes on by itself.'));
}

// steamStage: what steamcmd is doing, in plain words
function steamStage(state) {
  const s = (state || '').toLowerCase();
  if (s.includes('download')) return 'Downloading';
  if (s.includes('verif')) return 'Checking the files of';
  if (s.includes('commit')) return 'Finishing';
  if (s.includes('prealloc')) return 'Making room on the disk for';
  if (s.includes('reconfig') || s.includes('stag')) return 'Preparing';
  return 'Connecting to Steam for';
}

// steamFacts: how much is done, how fast, how long is left, how long it has run
function steamFacts(st) {
  const out = [];
  if (st.total) out.push(h('span', null, h('b', null, fmtBytes(st.done)), ' of ' + fmtBytes(st.total)));
  if (st.speed) out.push(h('span', null, h('b', null, fmtBytes(st.speed) + '/s')));
  if (st.left) out.push(h('span', null, 'about ', h('b', null, fmtDur(st.left)), ' left'));
  if (st.started) out.push(h('span', { class: 'muted' }, 'running for ' + fmtDur(Date.now() / 1000 - st.started)));
  if (!out.length) out.push(h('span', { class: 'muted' }, 'Starting…'));
  return out;
}

// cs2ModePicker: CS2's game mode, as the +game_type and +game_mode launch options say it
const CS2_MODES = [['0 0', 'Casual'], ['0 1', 'Competitive'], ['0 2', 'Wingman'], ['1 2', 'Deathmatch'], ['1 0', 'Arms Race']];
function cs2ModePicker(f, onChange) {
  const tok = (f.args || '').split(/\s+/).filter(Boolean);
  const val = k => { const i = tok.indexOf(k); return i >= 0 && i + 1 < tok.length ? tok[i + 1] : null; };
  const cur = (val('+game_type') || '0') + ' ' + (val('+game_mode') || '0');
  const known = CS2_MODES.some(m => m[0] === cur);
  const sel = h('select', null, CS2_MODES.map(([v, l]) => h('option', { value: v, selected: v === cur }, l)),
    known ? null : h('option', { value: cur, selected: true }, 'Other (' + cur + ', from the launch options)'));
  sel.addEventListener('change', () => {
    const [t, m] = sel.value.split(' ');
    const out = [];
    for (let i = 0; i < tok.length; i++) {
      if (tok[i] === '+game_type' || tok[i] === '+game_mode') { i++; continue; }
      out.push(tok[i]);
    }
    out.push('+game_type', t, '+game_mode', m);
    tok.splice(0, tok.length, ...out);
    f.args = out.join(' ');
    onChange();
  });
  return sel;
}

// ipPicker: the server's IP, one of the machine's addresses (a new server: its first public one), or 0.0.0.0 for all
// of them (a machine behind a router, whose public address is not its own)
function ipPicker(f, isNew) {
  const ips = (MI().ips || []).slice();
  if (isNew && !f.ip && bestIP(MI())) f.ip = bestIP(MI());
  const cur = f.ip && f.ip !== '0.0.0.0' ? f.ip : '';
  if (cur && !ips.includes(cur)) ips.push(cur); // an address this machine no longer has: kept, flagged
  const known = MI().ips || [];
  const sel = h('select', null,
    ips.map(ip => h('option', { value: ip, selected: ip === cur }, ip + (known.includes(ip) ? (privateIP(ip) ? ' (local network)' : '') : ' (not on this machine)'))),
    h('option', { value: '', selected: cur === '' }, '0.0.0.0 (every address: behind a router)'));
  sel.addEventListener('change', () => { f.ip = sel.value; if (bindText.onChange) bindText.onChange(); });
  if (f.ip === '0.0.0.0') f.ip = '';
  return sel;
}

// mapPicker: the first map, with the maps of the server once it is installed
function mapPicker(f, g, isNew) {
  const id = 'maps-' + Math.random().toString(36).slice(2);
  const list = h('datalist', { id });
  const inp = bindText(f, 'map', { placeholder: g.default_map, list: id });
  if (!isNew && f.id) {
    api('GET', 'api/servers/' + encodeURIComponent(f.id) + '/maps').then(r => swap(list, (r.maps || []).map(m => h('option', { value: m })))).catch(() => {});
  }
  return h('div', null, inp, list);
}

function folderPicker(obj, key, placeholder, opts) {
  const inp = bindText(obj, key, { placeholder: placeholder || 'E:\\Servers\\CS2 - Server 1' });
  const btn = h('button', { class: 'btn', type: 'button', title: 'Browse', 'aria-label': 'Browse', onclick: async () => {
    const p = await browseFolder(obj[key] || placeholder, opts);
    if (p) { inp.value = p; obj[key] = p; inp.dispatchEvent(new Event('input')); }
  } }, icon('folder'), opts && opts.forNew ? 'Browse' : null);
  return h('div', { class: 'inputgroup' }, inp, btn);
}

// exePicker: the program a server starts with. Empty setting: the manager finds it (the game's usual one); else
// the one picked. refresh(preview) shows what the preview found.
function exePicker(f, onChange) {
  const pathEl = h('span', { class: 'mono grow' }, '…');
  const tagEl = h('span');
  const reset = h('button', { class: 'btn sm ghost', type: 'button', title: 'Let the manager find it', onclick: () => { f.exe = ''; onChange(); } }, icon('restart'), 'Automatic');
  const pick = h('button', { class: 'btn sm', type: 'button', onclick: async () => {
    const p = await pickProgram(f);
    if (p) { f.exe = p; onChange(); }
  } }, icon('terminal'), 'Pick…');
  const el = h('div', { class: 'col', style: 'gap:8px' },
    h('div', { class: 'exeline', style: 'padding:8px 10px;background:var(--bg-2);border:1px solid var(--line-2);border-radius:8px;min-height:36px' }, icon('terminal'), pathEl, tagEl),
    h('div', { class: 'row wrap', style: 'gap:6px' }, pick, reset));
  el.refresh = p => {
    pathEl.textContent = p.exe || '—';
    reset.classList.toggle('hidden', !f.exe);
    swap(tagEl, !p.exe_found ? h('span', { class: 'tag red', title: 'This file is not there' }, 'not found')
      : p.exe_auto ? h('span', { class: 'tag', title: 'The usual program of the game, found in its folder' }, 'automatic')
      : h('span', { class: 'tag blue', title: 'Picked for this server' }, 'picked'));
  };
  return el;
}

// pickProgram: the programs of a server's folder in one click, or any file (for whoever may browse the disk)
function pickProgram(f) {
  return new Promise(resolve => {
    let done = false;
    const m = modal({ title: 'The program that starts the server', size: 'wide', onClose: () => { if (!done) resolve(null); } });
    const list = h('div', { class: 'maplist', style: 'max-height:45vh' }, h('div', { class: 'muted small', style: 'padding:12px' }, 'Loading…'));
    const finish = p => { done = true; m.close(); resolve(p); };
    api('GET', 'api/servers/' + encodeURIComponent(f.id) + '/programs').then(r => {
      const cur = (r.found || '').toLowerCase();
      swap(list, r.programs.length ? r.programs.map(p => h('div', { class: 'mapitem' + (p.toLowerCase() === cur ? ' current' : ''), style: 'cursor:pointer', onclick: () => finish(p) },
        icon('terminal'), h('span', { class: 'n', title: p }, p.slice(r.dir.length).replace(/^[\\/]/, '')), p.toLowerCase() === cur ? h('span', { class: 'tag green' }, 'now') : null))
        : h('div', { class: 'muted small', style: 'padding:12px' }, 'No program at the top of ' + r.dir + '.'));
    }).catch(e => swap(list, h('div', { class: 'note bad' }, e.message)));
    append(m.bd, h('div', { class: 'col' },
      h('div', { class: 'hint' }, 'Some servers start with srcds.exe, others with srcds_win64.exe or a program of their own. Pick the one this server uses: it is used from its next start.'),
      h('div', { class: 'panel', style: 'padding:6px' }, list)));
    append(m.ft, can('c') ? h('button', { class: 'btn ghost', style: 'margin-right:auto', onclick: async () => {
      done = true; m.close();
      resolve(await browseFolder(f.install_dir, { files: 'program', title: 'The program that starts the server' }));
    } }, icon('folder'), 'Somewhere else…') : null, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'));
  });
}

function serverForm(f, g, isNew, onChange) {
  bindText.onChange = onChange;
  const rconFields = secret(f, 'rcon_password');
  const rcon = h('div', { class: 'inputgroup' }, rconFields[0], rconFields[1],
    h('button', { class: 'btn', type: 'button', title: 'New random password', 'aria-label': 'New random password', onclick: () => {
      const a = new Uint8Array(9);
      crypto.getRandomValues(a);
      f.rcon_password = [...a].map(x => x.toString(16).padStart(2, '0')).join('');
      rconFields[0].value = f.rcon_password;
      rconFields[0].type = 'text';
      if (onChange) onChange();
    } }, icon('dice')));
  const gslt = secret(f, 'gslt', { placeholder: '32 characters from Steam' });
  const restartTime = bindText(f, 'restart_time', { placeholder: 'HH:MM' });
  const timeField = field('Restart time', restartTime, 'Empty: the manager\'s time.');
  const showTime = () => { timeField.style.display = (f.restart === 'daily' || (f.restart === '' && false)) ? '' : 'none'; };
  const restartSel = bindSelect(f, 'restart', [['', 'Manager default'], ['daily', 'Every day at a time'], ['auto', 'After 24 h, when empty'], ['never', 'Never']], showTime);
  showTime();
  const goldsrc = g.engine === 'goldsrc';
  const other = g.engine === 'other'; // Rust, 7 Days to Die…: their command line says it all

  return h('div', null,
    h('div', { class: 'section' }, h('h3', null, 'Server'), h('div', { class: 'hint' }, 'How it shows in the manager, and where it lives.'),
      h('div', { class: 'formgrid' },
        field('Name', bindText(f, 'name', { required: true, maxlength: 80 })),
        isNew
          ? field('Folder', folderPicker(f, 'install_dir', 'Browse to create its folder', { forNew: true, root: f._root, name: f._folder }),
            'An empty folder: steamcmd installs the game there. Browse opens ' + (f._root || 'the folder for new servers') + ', where you create it in one click.', true)
          : field('Folder', folderPicker(f, 'install_dir'), 'Where the game server is installed.', true),
        isNew ? null : h('div', { class: 'field wide' }, h('span', { class: 'flabel' }, 'Program'), (serverForm.exe = exePicker(f, onChange)),
          h('small', { class: 'hint' }, 'What starts the server. The manager finds the usual one by itself' + ((g.exe || []).length ? ' (' + g.exe.join(' or ') + ')' : '') + '; pick another when this server uses one of its own.')),
        field('Notes', bindText(f, 'notes', { placeholder: 'Anything to remember: owner, plugins, purpose…' }), null, true))),
    h('div', { class: 'section' }, h('h3', null, 'Game'), h('div', { class: 'hint' }, other ? 'Address, players and passwords: given on its command line.' : 'Address, players and first map.'),
      other && g.notes ? h('div', { class: 'note info small', style: 'margin-bottom:12px;padding:8px 10px;border-radius:8px' }, g.notes) : null,
      h('div', { class: 'formgrid' },
        field('IP', ipPicker(f, isNew), 'The address players join it at. 0.0.0.0 (every address) only for a machine behind a router, whose public address is not one of its own.'),
        field('Port', bindText(f, 'port', { type: 'number', min: 1, max: 65535 })),
        field('Max players', bindText(f, 'maxplayers', { type: 'number', min: 1, max: 255 })),
        other ? (g.map ? field('Map or world', bindText(f, 'map', { placeholder: g.default_map }), 'The map, or the world, it plays (' + g.default_map + ' unless you give another).') : null)
          : field('Map', mapPicker(f, g, isNew), isNew ? 'The first map. ' + g.name + ' starts on ' + g.default_map + ' unless you pick another; the Maps tab lists them once it is installed.' : 'The map it starts on.'),
        other && g.password ? field('Join password', h('div', { class: 'inputgroup' }, ...secret(f, 'password')), 'What players type to join. Empty: none.') : null,
        goldsrc || (other && !g.gslt) ? null : field('Game server login token', h('div', { class: 'inputgroup' }, gslt[0], gslt[1]),
          h('span', null, 'Needed for players outside your LAN to find the server. Get one at ', h('a', { href: 'https://steamcommunity.com/dev/managegameservers', target: '_blank', rel: 'noopener' }, 'steamcommunity.com/dev/managegameservers'), ' (app ID ' + (g.key === 'cs2' ? '730' : 'of the game') + ').'), true),
        other && g.rcon === 'none' ? null : field('RCON password', rcon, other ? (g.rcon === 'battleye' ? 'Set in its BattlEye config (BEServer_x64.cfg) as it starts, letters and numbers only' : 'Given to it on its command line') + ': the manager uses it to ' + (g.say ? 'warn players and ' : '') + 'stop it cleanly, and the Console tab to send commands.' : 'The manager uses it to warn players, change maps and stop the server cleanly. Written into server.cfg for a new server.'),
        other ? null : field('Say command', bindText(f, 'say_command', { placeholder: 'say' }), 'How players are warned: say, sm_csay, hlx_sm_csay…'))),
    h('div', { class: 'section' }, h('h3', null, 'Launch options'), h('div', { class: 'hint' }, other ? 'More options for the command line, after the game\'s own (these games take the last of an option given twice).' : 'More options for the command line. One you give replaces the default one of the same name (for example +maxplayers).'),
      h('div', { class: 'formgrid' },
        field('Options', h('textarea', {
          rows: 3, spellcheck: false, value: f.args || '', placeholder: other ? '' : '+sv_lan 0',
          oninput: e => { f.args = e.target.value; if (onChange) onChange(); },
        }), null, true),
        field('Priority', bindSelect(f, 'priority', [['', 'Manager default']].concat(PRIORITIES.map(p => [p, p[0].toUpperCase() + p.slice(1)])))),
        other ? (g.log ? h('div', { class: 'wide' }, bindCheck(f, 'console_log', 'Show the console here', 'Its console log, which the Console tab reads live. Recommended.')) : null)
          : h('div', { class: 'wide' }, bindCheck(f, 'console_log', 'Show the console here (-condebug)', 'The server always runs with -console (its text console, in its own window). -condebug also copies everything that console prints into console.log, which the Console tab reads live. Recommended.')))),
    h('div', { class: 'section' }, h('h3', null, 'Running and updates'), h('div', { class: 'hint' }, 'What the manager does by itself.'),
      h('div', { class: 'formgrid' },
        h('div', { class: 'wide' }, bindCheck(f, 'autostart', 'Start with the manager', 'Started when the manager starts, unless you stopped it yourself.')),
        h('div', { class: 'wide' }, packagePicker(f)),
        g.metamod || g.engine === 'source' ? h('div', { class: 'wide' }, bindCheck(f, 'vanilla_while_waiting', 'Without its addons while it waits for a fix',
          'When the manager gives up on it after crashes an addon caused (or crashes in a row after a game update), it starts it without its addons (Metamod off) until the fix is installed (the package\'s crash watch, on the Addons page). Players can play meanwhile; the plugins do not run.')) : null,
        field('Automatic updates', bindSelect(f, 'autoupdate', [[null, 'Manager default'], [true, 'On'], [false, 'Off']])),
        field('Update method', bindSelect(f, 'update_mode', [['direct', 'steamcmd into the server'], ['cache', 'Shared copy, then copy the files']]), 'The shared copy downloads once for all the servers of the game, while they keep running.'),
        field('Scheduled restart', restartSel), timeField,
        g.metamod ? field('Metamod', bindSelect(f, 'metamod', [[null, 'When addons/metamod is there'], [true, 'Always'], [false, 'Never']]), 'Each CS2 update removes Metamod from gameinfo.gi: the manager puts it back.') : null,
        field('Steam login', bindText(f, 'steam_login', { placeholder: 'anonymous' }), 'anonymous for every free dedicated server: CS 1.6, CS:S, CS2, TF2… An account (user password) only for the rare game that needs one, without Steam Guard (the manager cannot type its code). Used from the next download or update.'))));
}

function tabSettings(id) {
  const base = 'api/servers/' + encodeURIComponent(id);
  const v0 = serverBy(id);
  const g = S.gamesBy[v0.game] || {};
  const wrap = h('div', null, h('div', { class: 'muted' }, 'Loading…'));
  const cmd = h('div', { class: 'cmdline' }, '…');
  const errs = h('div');
  let f = null;
  let dirty = false;
  const saveBtn = h('button', { class: 'btn primary', disabled: true }, icon('save'), 'Save');
  const preview = debounce(() => {
    api('POST', 'api/preview', f).then(p => {
      cmd.textContent = p.command;
      cmd.setAttribute('data-tip', 'Runs in ' + p.dir);
      if (serverForm.exe) serverForm.exe.refresh(p);
    }).catch(e => { cmd.textContent = e.message; });
  }, 350);
  const changed = () => { dirty = true; saveBtn.disabled = false; preview(); };

  api('GET', base + '/config').then(cfg => {
    f = cfg;
    if (f.autoupdate === undefined) f.autoupdate = null;
    if (f.metamod === undefined) f.metamod = null;
    if (!f.priority) f.priority = '';
    if (!f.restart) f.restart = '';
    swap(wrap,
      h('div', { class: 'panel' }, h('div', { class: 'bd' }, serverForm(f, g, false, changed))),
      h('div', { class: 'panel', style: 'margin-top:18px' }, h('div', { class: 'hd' }, icon('terminal'), h('h2', { class: 'grow' }, 'Command line'), h('span', { class: 'muted small' }, 'Used at the next start')),
        h('div', { class: 'bd' }, cmd)),
      can('c') ? h('div', { class: 'panel', style: 'margin-top:18px;border-color:rgba(255,92,108,.35)' },
        h('div', { class: 'hd' }, icon('alert'), h('h2', { class: 'grow' }, 'Remove this server')),
        h('div', { class: 'bd row wrap' }, h('div', { class: 'grow muted' }, 'Removes it from the manager. Its files can stay on the disk, or go with it.'),
          h('button', { class: 'btn danger', onclick: () => removeServer(serverBy(id)) }, icon('trash'), 'Remove…'))) : null);
    preview();
  }).catch(e => swap(wrap, h('div', { class: 'note bad' }, e.message)));

  saveBtn.onclick = async () => {
    saveBtn.disabled = true;
    clear(errs);
    try {
      await api('PUT', base, f);
      dirty = false;
      const v = serverBy(id);
      toast('Saved.' + (v && isUp(v) ? ' The server uses the new settings at its next start.' : ''), 'ok');
    } catch (e) {
      const list = e.data && e.data.errors ? e.data.errors : [e.message];
      swap(errs, h('div', { class: 'note bad', style: 'margin-bottom:14px' }, list.map(x => h('div', null, x))));
      toast('Not saved: ' + list.join(' '), 'bad');
      saveBtn.disabled = false;
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
  };
  const bar = h('div', { class: 'row', style: 'position:sticky;bottom:0;padding:14px 0;background:linear-gradient(180deg,transparent,var(--bg) 40%);margin-top:6px;z-index:5' },
    h('span', { class: 'spacer' }), saveBtn);
  function warnUnsaved(e) { if (dirty) { e.preventDefault(); e.returnValue = ''; } }
  window.addEventListener('beforeunload', warnUnsaved);
  return { el: h('div', { style: 'max-width:1000px' }, errs, wrap, bar), destroy: () => window.removeEventListener('beforeunload', warnUnsaved) };
}

// deleteServerFiles: the server out of the manager and its whole folder deleted, once its name is typed.
function deleteServerFiles(v) {
  if (isUp(v) || v.pid || (v.busy && v.busy !== 'start')) { toast('Stop the server first.', 'bad'); return; }
  const m = modal({ title: 'Delete ' + v.name + '?' });
  const name = h('input', { type: 'text', placeholder: 'delete', spellcheck: false, autocomplete: 'off' });
  const ok = h('button', { class: 'btn danger', disabled: true, onclick: async () => {
    ok.disabled = true;
    if (await run(() => api('DELETE', 'api/servers/' + encodeURIComponent(v.id) + '?files=1'), v.name + ' removed: its folder is being deleted.')) {
      m.close();
      location.hash = '#/';
    } else ok.disabled = false;
  } }, icon('trash'), 'Delete everything');
  name.addEventListener('input', () => { ok.disabled = name.value.trim().toLowerCase() !== 'delete'; });
  name.addEventListener('keydown', e => { if (e.key === 'Enter' && !ok.disabled) ok.click(); });
  append(m.bd, h('div', { class: 'col' },
    h('div', { class: 'note bad' }, 'The server is removed from the manager and its whole folder is deleted, with its maps, configs, plugins and logs. This cannot be undone.'),
    h('div', null, h('div', { class: 'small muted' }, 'Folder'), h('div', { class: 'mono', style: 'word-break:break-all' }, v.install_dir)),
    h('label', { class: 'field' }, h('span', null, 'Type ', h('b', null, 'delete'), ' to confirm'), name)));
  append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'), ok);
  setTimeout(() => name.focus(), 30);
}

async function removeServer(v) {
  if (isUp(v) || (v.busy && v.busy !== 'start')) { toast('Stop the server first.', 'bad'); return; }
  const delFiles = h('input', { type: 'checkbox' });
  const confirmName = h('input', { type: 'text', placeholder: v.name, spellcheck: false });
  const confirmWrap = h('label', { class: 'field hidden', style: 'margin-top:14px' }, h('span', null, 'Type the server\'s name to confirm'), confirmName);
  delFiles.addEventListener('change', () => confirmWrap.classList.toggle('hidden', !delFiles.checked));
  const body = h('div', null,
    h('p', { style: 'margin-top:0;color:var(--text-2)' }, v.name + ' is removed from the manager.'),
    h('label', { class: 'check' }, delFiles, h('span', null, h('b', null, 'Also delete its files'), h('small', { class: 'mono' }, v.install_dir))),
    confirmWrap);
  if (!await confirmBox('Remove ' + v.name + '?', body, 'Remove', true)) return;
  if (delFiles.checked && confirmName.value.trim() !== v.name) { toast('The name did not match: nothing was removed.', 'bad'); return; }
  if (await run(() => api('DELETE', 'api/servers/' + encodeURIComponent(v.id) + (delFiles.checked ? '?files=1' : '')), v.name + ' removed.')) location.hash = '#/';
}

/* ---------- Activity of a server ---------- */

function tabActivity(id) {
  const list = h('div', { class: 'activity' });
  let evs = [];
  api('GET', 'api/logs?server=' + encodeURIComponent(id)).then(r => { evs = r.events; render(); });
  function render() { swap(list, evs.length ? evs.slice(0, 300).map(eventRow) : h('div', { class: 'muted small', style: 'padding:16px' }, 'Nothing yet.')); }
  return {
    el: h('div', { class: 'panel', style: 'max-width:900px' }, list),
    onEvent: ev => { evs.unshift(ev); render(); },
  };
}

/* ---------- Backups: its settings and plugins, as zip files ---------- */

const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const ordinal = n => n + (n % 100 >= 11 && n % 100 <= 13 ? 'th' : ['th', 'st', 'nd', 'rd'][n % 10] || 'th');
// backupWhen says when the backups are made by themselves
function backupWhen(b) {
  const at = ' at ' + (b.time || '05:30');
  switch (b.schedule) {
    case 'daily': return 'Every day' + at;
    case 'weekly': return 'Every ' + WEEKDAYS[b.weekday || 0] + at;
    case 'monthly': {
      const d = Number(b.day) || 1;
      return (d >= 31 ? 'On the last day of each month' : 'On the ' + ordinal(d) + ' of each month' + (d > 28 ? ' (the last day of shorter months)' : '')) + at;
    }
    default: return 'By hand only' + (b.before ? ', and before a change' : '');
  }
}
const BACKUP_WHY = { daily: 'Daily', weekly: 'Weekly', monthly: 'Monthly', manual: 'By hand', 'before-update': 'Before an update', 'before-addons': 'Before an addons push', 'before-restore': 'Before a restore' };

function tabBackups(id) {
  const base = 'api/servers/' + encodeURIComponent(id) + '/backups';
  const tbody = h('tbody');
  const info = h('div', { class: 'muted small', style: 'overflow-wrap:anywhere' });
  let data = { backups: [] };
  let waiting = false;

  async function load() {
    try { data = await api('GET', base); } catch (e) { toast(e.message, 'bad'); return; }
    const when = backupWhen(data);
    swap(info, when + ', the newest ' + data.keep + ' kept' + (data.before && data.schedule !== 'manual' ? '; also before an update, an addons push and a restore' : '') + '. In ',
      h('span', { class: 'mono' }, data.dir), '.', (data.offsite || []).length ? ' Copied to ' + data.offsite.map(o => o.name).join(', ') + (data.offsite.some(o => o.error) ? ' (failing: see the Backups page)' : '') + '.' : '',
      can('b') ? h('span', null, ' ', h('a', { href: '#/settings/backups' }, 'Change')) : null);
    render();
  }

  function render() {
    const list = data.backups || [];
    if (!list.length) {
      swap(tbody, h('tr', null, h('td', { colspan: '4', class: 'muted', style: 'text-align:center;padding:30px' }, waiting ? 'Backing up…' : data.schedule === 'manual' ? 'No backup yet. Click Back up now.' : 'No backup yet. Click Back up now, or wait for the scheduled one.')));
      return;
    }
    swap(tbody, list.map(b => h('tr', null,
      h('td', { class: 'nowrap', title: new Date(b.time).toLocaleString() }, fmtTime(b.time, true), h('div', { class: 'muted small' }, timeAgo(new Date(b.time).getTime() / 1000))),
      h('td', null, h('span', { class: 'tag' + (['daily', 'weekly', 'monthly'].includes(b.reason) ? '' : b.reason === 'manual' ? ' blue' : ' amber') }, BACKUP_WHY[b.reason] || b.reason)),
      h('td', { class: 'num nowrap' }, fmtBytes(b.size)),
      h('td', { class: 'num nowrap' },
        h('button', { class: 'btn sm ghost', title: 'What it holds', onclick: () => backupFilesBox(base, b) }, icon('list'), h('span', { class: 'hide-m' }, 'Files')),
        h('a', { class: 'btn sm ghost', href: machineURL(base + '/' + encodeURIComponent(b.name)), download: '', title: 'Download the zip' }, icon('download'), h('span', { class: 'hide-m' }, 'Download')),
        h('button', { class: 'btn sm', onclick: () => backupRestoreBox(id, b, () => setTimeout(load, 1500)) }, icon('restart'), 'Restore'),
        h('button', { class: 'btn sm ghost icon', title: 'Remove', 'aria-label': 'Remove ' + b.name, onclick: async () => {
          if (!await confirmBox('Remove this backup?', fmtTime(b.time, true) + ' (' + (BACKUP_WHY[b.reason] || b.reason) + ') is deleted.', 'Remove', true)) return;
          if (await run(() => api('DELETE', base + '/' + encodeURIComponent(b.name)), 'Backup removed.')) load();
        } }, icon('trash'))))));
  }

  const nowBtn = h('button', { class: 'btn primary', onclick: async () => {
    waiting = true; nowBtn.disabled = true;
    if (await run(() => api('POST', base), 'Backing up…')) setTimeout(load, 1500);
    setTimeout(() => { waiting = false; nowBtn.disabled = false; }, 2000);
  } }, icon('save'), 'Back up now');
  load();
  return {
    el: h('div', { style: 'max-width:1000px' },
      h('div', { class: 'panel', style: 'overflow:auto' },
        h('div', { class: 'hd' }, h('div', { class: 'grow' }, h('h2', null, 'Backups'), info), h('a', { class: 'btn ghost', href: '#/backups', title: 'The backups of every server' }, icon('list'), h('span', { class: 'hide-m' }, 'All servers')), nowBtn),
        h('table', { class: 'list' },
          h('thead', null, h('tr', null, h('th', null, 'When'), h('th', null, 'Why'), h('th', { class: 'num' }, 'Size'), h('th', { class: 'num' }, ''))), tbody))),
    onEvent: ev => { if (/Backed up|Backup|Restored/.test(ev.text)) load(); },
  };
}

// What a backup holds (base: the API address of its server's backups).
async function backupFilesBox(base, b) {
  const m = modal({ title: 'In the backup of ' + fmtTime(b.time, true), size: 'wide' });
  append(m.bd, h('div', { class: 'muted small' }, 'Loading…'));
  try {
    const r = await api('GET', base + '/' + encodeURIComponent(b.name) + '?files=1');
    swap(m.bd, h('div', { class: 'hint' }, r.files.length + ' files, in its game folder:'), h('pre', { class: 'crashpre tall' }, r.files.join('\n')));
  } catch (e) { swap(m.bd, h('div', { class: 'note bad' }, e.message)); }
  append(m.ft, h('button', { class: 'btn', onclick: () => m.close() }, 'Close'));
}

// backupRestoreBox asks, then restores a backup of a server; done is called once it started.
function backupRestoreBox(id, b, done) {
  const base = 'api/servers/' + encodeURIComponent(id) + '/backups';
  const v = serverBy(id);
  const up = v ? isUp(v) : false;
  const n = v ? humans(v) : 0;
  const warn = { on: up && n > 0 };
  const m = modal({ title: 'Restore the backup of ' + fmtTime(b.time, true) + '?' });
  append(m.bd,
    v ? h('p', { style: 'margin-top:0' }, h('b', null, v.name), ': its files are put back into the game folder: configs, plugins and their data as they were then. Files it does not have stay as they are.') : null,
    h('ul', { class: 'small', style: 'padding-left:18px;color:var(--text-2)' },
      up ? h('li', null, 'The server is stopped first, then started again.') : null,
      h('li', null, 'What it has now is backed up first: you can come back to it.')),
    up ? bindCheck(warn, 'on', 'Warn the players first', n ? n + ' player' + (n > 1 ? 's are' : ' is') + ' on: they get the usual warning before the stop.' : 'Nobody is on.') : null);
  append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'),
    h('button', { class: 'btn primary', onclick: async () => {
      m.close();
      if (await run(() => api('POST', base + '/' + encodeURIComponent(b.name) + '/restore', { warn: !!warn.on }), 'Restoring…') && done) done();
    } }, icon('restart'), 'Restore'));
}

function downloadURL(url) {
  const a = h('a', { href: machineURL(url), download: '' });
  document.body.append(a);
  a.click();
  a.remove();
}

/* =================================================================================================================
 * Backups: every server's, downloaded one by one or together
 * =============================================================================================================== */

function pageBackups() {
  const tiles = h('div', { class: 'tiles two' });
  const info = h('div', { class: 'sub' });
  const selBar = h('div', { class: 'bkselbar' });
  const offEl = h('div');
  const list = h('div', { class: 'col', style: 'gap:16px' }, h('div', { class: 'muted' }, 'Loading…'));
  const picked = new Set(); // "<id>/<name>"
  const open = new Set(store.get('bk.open', [])); // the servers showing all their backups
  let data = { servers: [] };
  const SHOWN = 5;
  const key = (g, b) => g.id + '/' + b.name;
  const total = l => l.reduce((t, b) => t + b.size, 0);
  const allItems = () => data.servers.flatMap(g => g.backups.map(b => ({ g, b })));

  async function load() {
    try { data = await api('GET', 'api/backups'); } catch (e) { swap(list, h('div', { class: 'note bad' }, e.message)); return; }
    const have = new Set(allItems().map(x => key(x.g, x.b)));
    for (const k of [...picked]) if (!have.has(k)) picked.delete(k);
    render();
  }

  function render() {
    const items = allItems();
    const latest = items.reduce((t, x) => Math.max(t, new Date(x.b.time).getTime()), 0);
    const servers = data.servers.filter(g => !g.removed);
    const without = servers.filter(g => !g.backups.length).length;
    swap(info, backupWhen(data) + ', the newest ' + data.keep + ' of each server kept. In ', h('span', { class: 'mono' }, data.dir), '.',
      can('b') ? h('span', null, ' ', h('a', { href: '#/settings/backups' }, 'Change')) : null);
    swap(tiles,
      h('div', { class: 'panel tile' }, h('div', { class: 'ic purple' }, icon('save')), h('div', null,
        h('div', { class: 'v' }, items.length), h('div', { class: 'l' }, 'backups'))),
      h('div', { class: 'panel tile' }, h('div', { class: 'ic blue' }, icon('disk')), h('div', null,
        h('div', { class: 'v' }, fmtBytes(total(items.map(x => x.b)))), h('div', { class: 'l' }, 'in all'))),
      h('div', { class: 'panel tile' }, h('div', { class: 'ic ' + (without ? 'amber' : 'green') }, icon(without ? 'alert' : 'server')), h('div', null,
        h('div', { class: 'v' }, servers.length - without, h('small', null, ' / ' + servers.length)), h('div', { class: 'l' }, without ? without + ' server' + (without > 1 ? 's' : '') + ' without a backup' : 'servers backed up'))),
      h('div', { class: 'panel tile' }, h('div', { class: 'ic blue' }, icon('clock')), h('div', null,
        h('div', { class: 'v' }, latest ? timeAgo(latest / 1000) : '—'), h('div', { class: 'l' }, 'latest backup'))));
    swap(list, data.servers.length ? data.servers.map(group) : h('div', { class: 'panel' }, h('div', { class: 'empty' }, icon('save'), h('h2', null, 'No server yet'))));
    renderOffsite();
    renderSel();
  }

  // the off-site copies: where, and how the last copy went
  let offTimer = 0;
  function renderOffsite() {
    const off = data.offsite || [];
    clearTimeout(offTimer);
    if (!off.length) {
      swap(offEl, h('div', { class: 'muted small offnote' }, icon('cloud'), 'The backups are only on this machine.',
        can('b') ? h('span', null, ' ', h('a', { href: '#/settings/offsite' }, 'Copy them to Google Drive, OneDrive or another disk')) : null));
      return;
    }
    if (off.some(o => o.busy)) offTimer = setTimeout(() => { if (offEl.isConnected) load(); }, 3000);
    swap(offEl, h('section', { class: 'panel offsite' },
      h('div', { class: 'hd' }, h('h2', { class: 'grow' }, 'Off-site copies'),
        can('b') ? h('a', { class: 'btn sm ghost', href: '#/settings/offsite' }, icon('sliders'), h('span', { class: 'hide-m' }, 'Settings')) : null,
        h('button', { class: 'btn sm', title: 'Copy the backups not there yet', disabled: off.some(o => o.busy), onclick: async () => {
          if (await run(() => api('POST', 'api/backups/offsite'), 'Copying…')) setTimeout(load, 800);
        } }, icon('upload'), 'Copy now')),
      h('div', { class: 'offlist' }, off.map(o => {
        const state = o.busy ? h('span', { class: 'tag blue' }, 'Copying…')
          : o.error ? h('span', { class: 'tag red', title: o.error }, 'Failed')
          : o.ok ? h('span', { class: 'tag green' }, 'Up to date')
          : h('span', { class: 'tag' }, 'Waiting');
        return h('div', { class: 'offrow' },
          h('span', { class: 'offic' }, icon(o.kind === 'rclone' ? 'cloud' : 'folder')),
          h('div', { class: 'grow', style: 'min-width:0' },
            h('div', { class: 'row', style: 'gap:8px' }, h('b', null, o.name || o.target), state),
            h('div', { class: 'mono muted small offtarget' }, o.target),
            o.error && !o.busy ? h('div', { class: 'small', style: 'color:var(--bad);overflow-wrap:anywhere;margin-top:2px' }, o.error) : null),
          h('div', { class: 'muted small nowrap', style: 'text-align:right' },
            o.ok ? ['last copy', h('br'), timeAgo(new Date(o.ok).getTime() / 1000)] : o.at ? ['tried', h('br'), timeAgo(new Date(o.at).getTime() / 1000)] : null));
      }))));
  }

  function group(g) {
    const base = g.removed ? 'api/backups/' + encodeURIComponent(g.id) : 'api/servers/' + encodeURIComponent(g.id) + '/backups';
    const showAll = open.has(g.id) || g.backups.length <= SHOWN + 1;
    const shown = showAll ? g.backups : g.backups.slice(0, SHOWN);
    const keys = g.backups.map(b => key(g, b));
    const allPicked = keys.length > 0 && keys.every(k => picked.has(k));
    const somePicked = keys.some(k => picked.has(k));
    const head = h('input', { type: 'checkbox', 'aria-label': 'Select every backup of ' + g.name, checked: allPicked, disabled: !keys.length,
      onchange: e => { keys.forEach(k => e.target.checked ? picked.add(k) : picked.delete(k)); render(); } });
    head.indeterminate = somePicked && !allPicked;
    const last = g.backups[0];
    const rows = shown.map(b => h('tr', { class: picked.has(key(g, b)) ? 'picked' : '' },
      h('td', { class: 'pick' }, h('input', { type: 'checkbox', checked: picked.has(key(g, b)), 'aria-label': 'Select ' + b.name,
        onchange: e => { e.target.checked ? picked.add(key(g, b)) : picked.delete(key(g, b)); render(); } })),
      h('td', { class: 'nowrap', title: new Date(b.time).toLocaleString() }, fmtTime(b.time, true), h('div', { class: 'muted small' }, timeAgo(new Date(b.time).getTime() / 1000), h('span', { class: 'show-m' }, ' · ', BACKUP_WHY[b.reason] || b.reason))),
      h('td', { class: 'hide-m' }, h('span', { class: 'tag' + (['daily', 'weekly', 'monthly'].includes(b.reason) ? '' : b.reason === 'manual' ? ' blue' : ' amber') }, BACKUP_WHY[b.reason] || b.reason)),
      h('td', { class: 'num nowrap' }, fmtBytes(b.size)),
      h('td', { class: 'num nowrap' },
        h('button', { class: 'btn sm ghost', title: 'What it holds', onclick: () => backupFilesBox(base, b) }, icon('list'), h('span', { class: 'hide-m' }, 'Files')),
        h('a', { class: 'btn sm ghost', href: machineURL(base + '/' + encodeURIComponent(b.name)), download: '', title: 'Download the zip' }, icon('download'), h('span', { class: 'hide-m' }, 'Download')),
        !g.removed ? h('button', { class: 'btn sm', onclick: () => backupRestoreBox(g.id, b, () => setTimeout(load, 1500)) }, icon('restart'), h('span', { class: 'hide-m' }, 'Restore')) : null,
        h('button', { class: 'btn sm ghost icon', title: 'Remove', 'aria-label': 'Remove ' + b.name, onclick: async () => {
          if (!await confirmBox('Remove this backup?', g.name + ': ' + fmtTime(b.time, true) + ' (' + (BACKUP_WHY[b.reason] || b.reason) + ') is deleted.', 'Remove', true)) return;
          if (await run(() => api('DELETE', 'api/backups/' + encodeURIComponent(g.id) + '/' + encodeURIComponent(b.name)), 'Backup removed.')) load();
        } }, icon('trash')))));
    if (!g.backups.length) rows.push(h('tr', null, h('td', { colspan: '5', class: 'muted', style: 'text-align:center;padding:18px' }, 'No backup yet.')));
    const more = g.backups.length - shown.length;
    return h('section', { class: 'panel bkgroup', style: 'overflow:auto' },
      h('div', { class: 'hd' },
        g.removed ? h('div', { class: 'badge sm', style: 'background:var(--panel-3)' }, '?') : badge(g.game, 'sm'),
        h('div', { class: 'grow', style: 'min-width:0' },
          h('div', { class: 'row', style: 'gap:8px' }, g.removed ? h('b', null, g.id) : h('a', { href: '#/server/' + encodeURIComponent(sid(g.id)) + '/backups', class: 'bkname' }, g.name),
            g.removed ? h('span', { class: 'tag amber' }, 'Removed server') : null),
          h('div', { class: 'muted small' }, g.backups.length ? g.backups.length + ' backup' + (g.backups.length > 1 ? 's' : '') + ' · ' + fmtBytes(total(g.backups)) + ' · latest ' + timeAgo(new Date(last.time).getTime() / 1000) : 'No backup')),
        !g.removed ? h('button', { class: 'btn sm', onclick: () => backupNow([g.id], g.name) }, icon('save'), h('span', { class: 'hide-m' }, 'Back up now')) : null,
        g.backups.length ? h('button', { class: 'btn sm ghost', title: 'Every backup of ' + g.name + ', in one zip', onclick: () => downloadURL('api/backups/zip?server=' + encodeURIComponent(g.id)) }, icon('download'), h('span', { class: 'hide-m' }, 'All')) : null),
      h('table', { class: 'list files bktable' },
        h('thead', null, h('tr', null, h('th', { class: 'pick' }, head), h('th', null, 'When'), h('th', { class: 'hide-m' }, 'Why'), h('th', { class: 'num' }, 'Size'), h('th', { class: 'num' }, ''))),
        h('tbody', null, rows)),
      more > 0 || (showAll && g.backups.length > SHOWN + 1) ? h('div', { class: 'bkmore' }, h('button', { class: 'btn sm ghost', onclick: () => {
        showAll ? open.delete(g.id) : open.add(g.id);
        store.set('bk.open', [...open]);
        render();
      } }, more > 0 ? 'Show ' + more + ' more' : 'Show fewer')) : null);
  }

  function renderSel() {
    if (!picked.size) { clear(selBar); selBar.classList.remove('on'); return; }
    const sel = allItems().filter(x => picked.has(key(x.g, x.b)));
    selBar.classList.add('on');
    swap(selBar, h('div', { class: 'selbar' },
      h('b', null, picked.size + ' selected'), h('span', { class: 'muted small' }, fmtBytes(total(sel.map(x => x.b)))),
      h('span', { class: 'spacer' }),
      h('button', { class: 'btn sm ghost', onclick: () => { picked.clear(); render(); } }, 'Clear'),
      h('button', { class: 'btn sm', onclick: () => downloadURL('api/backups/zip?' + sel.map(x => 'b=' + encodeURIComponent(key(x.g, x.b))).join('&')) }, icon('download'), 'Download'),
      h('button', { class: 'btn sm danger', onclick: async () => {
        if (!await confirmBox('Remove ' + sel.length + ' backup' + (sel.length > 1 ? 's' : '') + '?', 'They are deleted. This can not be undone.', 'Remove', true)) return;
        let n = 0;
        const bad = [];
        for (const x of sel) {
          try { await api('DELETE', 'api/backups/' + encodeURIComponent(x.g.id) + '/' + encodeURIComponent(x.b.name)); n++; } catch (e) { bad.push(x.g.name + ': ' + e.message); }
        }
        picked.clear();
        toast(bad.length ? n + ' removed; not: ' + bad.join('; ') : n + ' backup' + (n > 1 ? 's' : '') + ' removed.', bad.length ? 'bad' : 'ok');
        load();
      } }, icon('trash'), 'Remove')));
  }

  async function backupNow(ids, label) {
    const r = await run(() => api('POST', 'api/backups/now', { ids }));
    if (!r) return;
    if (r.skipped && r.skipped.length) toast((r.started ? 'Backing up ' + r.started + '. ' : '') + 'Not: ' + r.skipped.join('; '), r.started ? '' : 'bad');
    else toast(label ? 'Backing up ' + label + '…' : 'Backing up ' + r.started + ' server' + (r.started > 1 ? 's' : '') + '…', 'ok');
  }

  load();
  return {
    el: h('div', { style: 'max-width:1100px' },
      h('div', { class: 'pagehead' }, h('div', { class: 'grow' }, h('h1', null, 'Backups'), info),
        h('button', { class: 'btn', onclick: () => downloadURL('api/backups/zip?all=1'), title: 'Every backup, a folder per server, in one zip' }, icon('download'), 'Download all'),
        h('button', { class: 'btn primary', onclick: () => backupNow([]) }, icon('save'), 'Back up all now')),
      tiles, offEl, selBar, list),
    onEvent: ev => { if (/Backed up|Backup|Restored|Off-site/.test(ev.text)) load(); },
  };
}

/* ---------- Crashes: what was known when it crashed ---------- */

function tabCrashes(id, want) {
  const base = 'api/servers/' + encodeURIComponent(id) + '/crashes';
  const listEl = h('div', null, h('div', { class: 'muted small', style: 'padding:16px' }, 'Loading…'));
  const repEl = h('div');
  let list = [];
  let cur = want || '';

  async function load() {
    try {
      list = (await api('GET', base)).crashes || [];
    } catch (e) { swap(listEl, h('div', { class: 'note bad' }, e.message)); return; }
    if (!cur && list.length) cur = list[0].id;
    renderList();
    if (cur) show(cur); else clear(repEl);
  }

  function renderList() {
    if (!list.length) {
      swap(listEl, h('div', { class: 'panel' }, h('div', { class: 'bd muted', style: 'text-align:center;padding:30px' },
        'No crash so far. When it crashes, or is stuck and ended, what was known then is kept here: the plugin or module most likely at fault (from the crash dump), the last lines of its console and logs, its error code, what Windows wrote down, and what changed before.')));
      return;
    }
    swap(listEl, patterns(), h('div', { class: 'panel', style: 'overflow:auto' },
      h('div', { class: 'hd' }, h('h2', null, 'Crashes'), h('span', { class: 'muted small' }, list.length + ' kept (the last 30)'), h('span', { class: 'spacer' }),
        can('f') ? h('button', { class: 'btn sm ghost', onclick: async () => {
          if (!await confirmBox('Remove its crash reports?', 'The ' + list.length + ' reports of this server are removed. New crashes make new ones.', 'Remove', true)) return;
          if (await run(() => api('DELETE', base), 'Crash reports removed.')) { cur = ''; load(); }
        } }, icon('trash'), 'Remove all') : null),
      h('table', { class: 'list crashlist' },
        h('thead', null, h('tr', null, h('th', null, 'When'), h('th', null, 'What happened'), h('th', { class: 'hide-m' }, 'Map'), h('th', { class: 'num hide-m' }, 'Ran for'))),
        h('tbody', null, list.map(c => h('tr', { class: 'click' + (c.id === cur ? ' sel' : ''), onclick: () => { location.hash = '#/server/' + encodeURIComponent(id) + '/crashes/' + c.id; } },
          h('td', { class: 'nowrap', title: new Date(c.time).toLocaleString() }, fmtTime(c.time, true)),
          h('td', null, c.killed ? h('span', { class: 'tag orange' }, 'stuck') : h('span', { class: 'tag red' }, 'crash'), ' ', c.cause),
          h('td', { class: 'hide-m mono' }, c.map || '—'),
          h('td', { class: 'num hide-m nowrap' }, c.uptime ? fmtDur(c.uptime) : '—')))))));
  }

  // patterns: what the crashes kept have in common
  function patterns() {
    if (list.length < 2) return null;
    const n = list.length;
    const now = Date.now();
    const count = f => list.filter(f).length;
    const top = key => {
      const m = {};
      list.forEach(c => { const k = key(c); if (k) m[k] = (m[k] || 0) + 1; });
      return Object.entries(m).sort((x, y) => y[1] - x[1])[0] || null;
    };
    const of = k => h('b', null, k + ' of ' + n);
    const items = [];
    const sig = top(c => c.signature && c.signature !== 'unknown' && c.signature !== 'stuck' ? c.signature : '');
    if (sig && sig[1] >= 2) items.push([200 + sig[1], h('span', null, 'The same crash in ', of(sig[1]), ': ', h('span', { class: 'mono' }, sig[0]), '.')]);
    const sus = top(c => c.suspect || '');
    if (sus && sus[1] >= 2) items.push([300 + sus[1], h('span', null, h('b', null, sus[0]), ' is named as the cause (sure or likely) in ', of(sus[1]), ' crashes.')]);
    const mp = top(c => c.map || '');
    const withMap = count(c => c.map);
    if (mp && mp[1] >= 2 && mp[1] / withMap >= 0.4) items.push([mp[1], h('span', null, of(mp[1]), ' on ', h('span', { class: 'mono' }, mp[0]), '.')]);
    const nm = top(c => c.next_map || '');
    if (nm && nm[1] >= 2) items.push([250 + nm[1], h('span', null, of(nm[1]), ' while changing to ', h('span', { class: 'mono' }, nm[0]), '.')]);
    const mc = count(c => c.map_change);
    if (mc >= 2) items.push([mc, h('span', null, of(mc), ' while changing map.')]);
    const early = count(c => c.uptime && c.uptime < 300);
    if (early >= 2) items.push([early, h('span', null, of(early), ' within 5 minutes of starting: something at load (a plugin, a config, the map).')]);
    const empty = count(c => !c.killed && c.players === 0);
    if (empty >= 2 && empty / n >= 0.6) items.push([empty, h('span', null, of(empty), ' with nobody on: not caused by players (a timer, a plugin, the map, hibernation).')]);
    const busy = count(c => c.players > 0);
    if (busy === n) items.push([n - 0.5, h('span', null, 'Every one with players on: something they do, or a plugin reacting to them.')]);
    const stuck = count(c => c.killed);
    if (stuck >= 2) items.push([stuck, h('span', null, of(stuck), ' were stuck (ended by the manager), not crashes.')]);
    const day = count(c => now - new Date(c.time).getTime() < 86400000);
    const week = count(c => now - new Date(c.time).getTime() < 7 * 86400000);
    items.sort((x, y) => y[0] - x[0]);
    return h('div', { class: 'panel patterns', style: 'margin-bottom:16px' },
      h('div', { class: 'hd' }, icon('search'), h('h2', { class: 'grow' }, 'Patterns'),
        h('span', { class: 'muted small' }, day + ' in the last 24 h · ' + week + ' in 7 days · ' + n + ' kept')),
      h('div', { class: 'bd' }, items.length ? h('ul', { class: 'plain patlist' }, items.slice(0, 6).map(x => h('li', null, x[1])))
        : h('div', { class: 'muted small' }, 'No pattern yet: the crashes kept do not look alike.')));
  }

  async function show(rid) {
    let r;
    try { r = await api('GET', base + '/' + rid); } catch (e) { swap(repEl, h('div', { class: 'note bad', style: 'margin-top:14px' }, e.message)); return; }
    const kv = (k, v, tip) => v === null || v === undefined || v === '' ? null : h('div', { class: 'stat', 'data-tip': tip || null }, h('div', { class: 'k' }, k), h('div', { class: 'v small' }, v));
    const pre = (lines, cls) => h('pre', { class: 'crashpre' + (cls ? ' ' + cls : '') }, lines.map(l => h('span', { class: lineClass(l) }, l + '\n')));
    const sect = (title, ...body) => h('div', { class: 'section' }, h('h3', null, title), ...body);
    const t = new Date(r.time);
    const p = r.plain || {};
    const d = r.dump && r.dump.code ? r.dump : null;
    const inDir = x => { const dir = serverBy(id).install_dir || ''; return dir && x.toLowerCase().startsWith(dir.toLowerCase()) ? x.slice(dir.length).replace(/^[\\/]+/, '') : null; };
    const consoleEl = r.console_short && r.console_short.length ? pre(r.console_short, 'tall') : null;
    const SURE = { sure: ['red', 'Sure', 'The crash dump proves it: it crashed inside it.'], likely: ['amber', 'Likely', 'The call chain points at it, but does not prove it.'],
      unsure: ['', 'Not sure', 'What is known does not show what is at fault: only what is said here is sure.'] };
    const sure = SURE[p.sure];
    const frameRow = (f, first) => h('tr', { class: f.class === 'addon' ? 'addon' : '' },
      h('td', { class: 'muted small nowrap' }, first),
      h('td', { class: 'mono' }, f.module, h('span', { class: 'muted' }, '+' + f.offset), f.function ? h('div', { class: 'small muted' }, f.function) : null),
      h('td', null, h('span', { class: 'tag' + (f.class === 'addon' ? ' amber' : f.class === 'game' ? ' blue' : '') }, f.class)),
      h('td', { class: 'small hide-m' + (f.label ? '' : ' muted') }, f.label || f.role || ''));
    const cmds = r.commands || [];
    const chained = d && d.chain && (d.chain.length > 1 || d.chain.length === 1 && !d.chain_end);

    swap(repEl, h('div', { class: 'panel', style: 'margin-top:16px' }, h('div', { class: 'bd' },
      // in plain words
      h('div', { class: 'row', style: 'gap:10px;align-items:flex-start;flex-wrap:wrap' },
        h('div', { class: 'grow' },
          h('h2', { style: 'margin:0 0 4px' }, r.killed ? 'Stuck, ended by the manager' : 'Crashed', h('span', { class: 'muted', style: 'font-weight:400;font-size:14px' }, ' · ' + t.toLocaleString())),
          h('div', { style: 'margin-top:6px;font-size:15px' }, p.headline || r.cause)),
        h('div', { class: 'row', style: 'gap:6px;flex-wrap:wrap' },
          r.summary ? h('button', { class: 'btn sm', onclick: () => copyText(r.summary), 'data-tip': 'The few lines a bug report needs, to paste in a GitHub issue or a forum post' }, icon('copy'), 'Copy summary') : null,
          h('a', { class: 'btn sm', href: base + '/' + r.id + '?format=text', download: '' }, icon('download'), 'Save as text'),
          h('a', { class: 'btn sm', href: base + '/' + r.id + '/zip', download: '', 'data-tip': 'The report (text and JSON) and its crash dumps, to attach to a bug report' }, icon('download'), 'Download all (zip)'))),
      p.cause ? h('div', { class: 'verdict' + (sure ? ' v-' + p.sure : '') },
        h('div', { class: 'row', style: 'gap:8px;align-items:center' }, h('b', null, 'Cause'), sure ? h('span', { class: 'tag ' + sure[0], 'data-tip': sure[2] }, sure[1]) : null),
        h('div', { style: 'margin-top:4px' }, p.cause),
        p.error ? h('div', { class: 'small muted', style: 'margin-top:4px' }, p.error) : null,
        r.next ? h('div', { class: 'small muted', style: 'margin-top:4px' }, 'Then: ' + r.next + '.') : null) : null,
      p.before && p.before.length ? sect('What was going on', h('ul', { class: 'plain patlist' }, p.before.map(x => h('li', null, x)))) : null,
      p.todo && p.todo.length ? sect('What to do', h('ol', { class: 'todo' }, p.todo.map(x => h('li', null, x)))) : null,
      p.report_to ? sect('Report it',
        h('div', null, 'To ' + p.report_to + (p.report_at ? ': ' : '.'), p.report_at ? h('a', { href: p.report_at, target: '_blank', rel: 'noopener noreferrer' }, p.report_at) : null),
        h('div', { class: 'hint' }, 'Paste the summary (Copy summary, above) and attach the zip (Download all): it has the report and the crash dump they need.')) : null,
      h('div', { class: 'stats', style: 'margin-top:14px' },
        kv('Ran for', r.uptime ? fmtDur(r.uptime) : '—', r.started ? 'Started ' + new Date(r.started).toLocaleString() : null),
        kv('Map', r.map ? h('span', { class: 'mono' }, r.map) : null),
        kv('Changing to', r.next_map ? h('span', { class: 'mono' }, r.next_map) : null, 'The map it was loading when it crashed'),
        kv('Players', r.map ? (r.players - r.bots) + (r.bots ? ' + ' + r.bots + ' bots' : '') + (r.max_players ? ' / ' + r.max_players : '') : null, (r.player_list || []).join(', ') || null),
        kv('Exit code', r.exit_code ? h('span', { class: 'mono' }, r.exit_code) : r.killed ? 'Ended by the manager' : 'Not known', r.exit_text || null),
        kv('In a row', r.in_a_row > 1 ? 'Crash ' + r.in_a_row : null),
        kv('Version', r.version || null),
        kv('System', r.os || null),
        kv('Last use', r.memory ? (r.cpu || 0).toFixed(1) + ' % · ' + fmtBytes(r.memory) : null, 'CPU and memory at the last measure before it ended')),

      // for the developers
      h('h2', { class: 'devhead' }, 'For the developers'),
      h('div', { class: 'hint' }, 'What a plugin\'s author or Valve needs to find the bug. The summary is what to paste; the zip has the crash dump.'),
      r.summary ? h('pre', { class: 'crashpre' }, r.summary) : null,
      d ? sect('In the crash dump',
        h('div', { class: 'stats' },
          kv('Exception', h('span', { class: 'mono' }, d.code), d.exception || null),
          kv('Crashed in', d.module ? h('span', { class: 'mono' }, d.module + '+' + d.offset) : 'Outside any module', d.path || null),
          kv('Function', d.chain && d.chain[0] && d.chain[0].function ? h('span', { class: 'mono' }, d.chain[0].function) : null, 'The exported function it is in'),
          kv('Its version', d.version || null),
          kv('Program', d.arch + ' · ' + d.threads + ' threads')),
        d.access ? h('div', { class: 'hint', style: 'margin-top:8px' }, 'It was ' + d.access + (d.null ? ': something it used was not there (a player who left, an entity removed, a plugin\'s object freed).' : '.')) : null,
        d.why ? h('div', { class: 'small', style: 'margin-top:8px' }, h('b', null, 'Verdict' + (SURE[d.sure || 'unsure'] ? ' (' + SURE[d.sure || 'unsure'][1].toLowerCase() + ')' : '') + ': '), (d.suspect ? d.suspect + ': ' : '') + d.why + '.') : null,
        chained ? [
          h('h4', null, 'Call chain of the crashed thread'),
          h('div', { class: 'hint' }, 'Followed with the modules\' own unwind data: each line called the one above it. The crash first.'),
          h('table', { class: 'list frames' }, h('tbody', null, d.chain.map((f, k) => frameRow(f, '#' + k)))),
          d.chain_end ? h('div', { class: 'note info small', style: 'margin-top:8px' }, 'The chain stops there: ' + d.chain_end + '.') : null] : null,
        d.frames && d.frames.length ? (chained
          ? h('details', { style: 'margin-top:10px' }, h('summary', { class: 'small muted' }, 'Modules found on its stack (a scan: some were not calling)'),
            h('table', { class: 'list frames' }, h('tbody', null, d.frames.map((f, k) => frameRow(f, k === 0 ? 'crash' : '')))))
          : [h('h4', null, 'Modules found on the crashed thread\'s stack'),
            h('div', { class: 'hint' }, 'A scan, as the call chain could not be followed' + (d.chain_end ? ' (' + d.chain_end + ')' : '') + ': some of these were not calling, and the order is not sure.'),
            h('table', { class: 'list frames' }, h('tbody', null, d.frames.map((f, k) => frameRow(f, k === 0 ? 'crash' : ''))))]) : null,
        r.addons_loaded && r.addons_loaded.length ? [h('h4', null, 'Addons loaded'),
          h('table', { class: 'list' }, h('tbody', null, r.addons_loaded.map(f => h('tr', null,
            h('td', null, f.name, f.parts && f.parts.length ? h('div', { class: 'small muted' }, f.parts.join(', ')) : null),
            h('td', { class: 'mono small' }, f.version || '—'),
            h('td', { class: 'small muted hide-m' }, (f.runtime ? f.runtime + ', ' : '') + f.modules + ' module' + (f.modules === 1 ? '' : 's')))))),
          r.plugins_logged && r.plugins_logged.length ? h('div', { class: 'hint' }, 'Plugins loaded this run (CounterStrikeSharp\'s log): ', h('b', null, r.plugins_logged.join(', '))) : null] : null,
        d.note ? h('div', { class: 'note info small', style: 'margin-top:8px' }, d.note) : null) : null,
      cmds.length ? sect('Commands sent (RCON, the 5 minutes before)',
        h('table', { class: 'list' }, h('tbody', null, cmds.map(c => h('tr', { class: c.routine ? 'muted' : '' },
          h('td', { class: 'nowrap small' }, new Date(c.time).toLocaleTimeString()),
          h('td', { class: 'nowrap small muted' }, t - new Date(c.time) < 1000 ? '<1s' : '−' + fmtDur(Math.round((t - new Date(c.time)) / 1000))),
          h('td', { class: 'small' }, c.by || 'the manager'),
          h('td', { class: 'mono small' }, c.command, c.routine ? h('span', { class: 'muted' }, '  (routine)') : null, c.error ? h('span', { style: 'color:var(--bad)' }, '  ' + c.error) : null)))))) : null,
      r.changes && r.changes.length ? sect('Changed in the week before this run',
        h('div', { class: 'hint' }, 'Plugins, configs and game files modified, and what the manager did, before it was started. A crash series often starts right after one of these.'),
        h('ul', { class: 'plain changes' }, r.changes.map(c => h('li', null,
          h('span', { class: 'muted small nowrap' }, fmtTime(c.time, true)), ' ',
          c.kind === 'file' ? h('span', { class: 'mono small' }, c.text) : h('span', { class: 'small' }, c.text),
          c.size ? h('span', { class: 'muted small' }, ' ' + fmtBytes(c.size)) : null)))) : null,
      r.windows && r.windows.length ? sect('What it showed on screen', h('div', { class: 'hint' }, 'The window it was waiting on when the manager ended it.'), pre(r.windows)) : null,
      r.system && r.system.length ? sect(r.system[0].source.startsWith('Linux') ? 'What Linux wrote down' : 'What Windows wrote down',
        r.system.map(x => h('div', { class: 'sysrec' },
          h('div', null, h('b', null, x.text)),
          x.meaning ? h('div', { class: 'muted small' }, x.exception + ': ' + x.meaning) : null,
          x.module_path ? h('div', { class: 'mono small muted' }, x.module_path) : null,
          h('div', { class: 'muted small' }, x.source + ', ' + new Date(x.time).toLocaleTimeString())))) : null,
      r.dumps && r.dumps.length ? sect('Crash dumps', h('div', { class: 'hint' }, 'Open with Visual Studio or WinDbg.'),
        h('ul', { class: 'plain' }, r.dumps.map((x, k) => h('li', null, h('span', { class: 'mono small' }, x.path), ' ', h('span', { class: 'muted small' }, fmtBytes(x.size)), ' ',
          h('a', { class: 'small', href: base + '/' + r.id + '/dump/' + k, download: '' }, 'Download'))))) : null,
      (r.logs || []).map(l => sect('End of ' + (inDir(l.path) || l.path.split(/[\\/]/).slice(-3).join('/')), h('div', { class: 'hint' }, 'This run only. Last written ' + new Date(l.time).toLocaleString() + (Math.abs(new Date(l.time) - t) < 120000 ? ', as it crashed.' : '.')), pre(l.lines))),
      sect('Last lines of its console', h('div', { class: 'hint' }, 'Lines that repeat (but for their numbers) are folded.'), r.console_note ? h('div', { class: 'note warn small' }, r.console_note) : null,
        consoleEl || h('div', { class: 'muted small' }, 'Nothing.')),
      (r.notes || []).map(n => h('div', { class: 'note info small', style: 'margin-top:10px' }, n)))));
    if (consoleEl) consoleEl.scrollTop = consoleEl.scrollHeight;
  }

  load();
  return {
    el: h('div', { style: 'max-width:1100px' }, listEl, repEl),
    onEvent: ev => { if (ev.report) { cur = ev.report; load(); } },
  };
}

/* =================================================================================================================
 * Folder browser
 * =============================================================================================================== */

// browseFolder: pick a folder. opts.forNew: for a new server, it starts in opts.root (the folder for new servers),
// offers to create a folder named opts.name there, and takes an empty folder (to install into) or the folder of a
// game server already there (added as it is); not the folder of a server of the manager.
// opts.files: pick a file instead ('program': what can start a server; or extensions: '.xml'), from start.
function browseFolder(start, opts) {
  opts = opts || {};
  const pickFile = !!opts.files;
  return new Promise(resolve => {
    let done = false;
    let cur = '';
    let curEmpty = false;
    let missing = ''; // the folder for new servers, when it does not exist yet: created with the server's folder
    let inServer = ''; // the folder shown is a game server's: nothing new goes inside it
    let managed = false; // ... and that server is one of the manager's
    const m = modal({ title: opts.title || (pickFile ? 'Pick a file' : opts.forNew ? 'Folder for the new server' : 'Pick a folder'), size: 'wide', onClose: () => { if (!done) resolve(null); } });
    const pathEl = h('input', { type: 'text', spellcheck: false, placeholder: pickFile ? 'Type a path and press Enter, or pick below' : 'Type a path and press Enter, or pick below' });
    const note = h('div');
    const list = h('div', { class: 'maplist', style: 'max-height:45vh' });
    const finish = p => { done = true; m.close(); resolve(p); };
    const row = (ic, label, fn, extra) => h('div', { class: 'mapitem', style: 'cursor:pointer;font-family:var(--sans)', onclick: fn }, icon(ic), h('span', { class: 'n' }, label), extra || null);
    const go = async (p, tries) => {
      tries = tries || 0;
      if (!tries) missing = '';
      try {
        const r = await api('GET', 'api/browse?path=' + encodeURIComponent(p || '') + (pickFile ? '&files=' + encodeURIComponent(opts.files) : ''));
        cur = r.path;
        curEmpty = !!r.empty;
        inServer = r.server || '';
        managed = !!r.managed;
        pathEl.value = missing || r.path;
        clear(note);
        if (opts.forNew && managed) swap(note, h('div', { class: 'note info' }, 'This is the folder of ' + inServer + ', already in the manager: the new server goes next to it. Go up (..) one level.'));
        else if (opts.forNew && r.installed) swap(note, h('div', { class: 'note ok' }, icon('check'), ' A game server is installed here: click ', h('b', null, 'Use this folder'), ' to add it as it is, without downloading it again.'));
        if (createBtn) createBtn.disabled = !!(opts.forNew && (inServer || r.installed));
        if (missing) {
          // Not there yet: nothing to list, only the way up
          swap(list, row('up', '..', () => go(r.path)),
            h('div', { class: 'muted small', style: 'padding:12px' }, 'This folder does not exist yet: it is created with the server\'s folder.'));
          return;
        }
        const files = r.files || [];
        swap(list,
          r.path ? row('up', '..', () => go(r.parent)) : null,
          r.dirs.map(d => row('folder', d.name, () => go(d.path))),
          files.map(f => row(opts.files === 'program' ? 'terminal' : 'file', f.name, () => finish(f.path), h('span', { class: 'muted small' }, fmtBytes(f.size)))),
          !r.dirs.length && !files.length ? h('div', { class: 'muted small', style: 'padding:12px' }, r.path ? (r.empty ? 'Empty folder.' : pickFile ? 'Nothing to pick here.' : 'No folder inside.') : 'No drive found.') : null);
      } catch (e) {
        // The folder for new servers may not exist yet: show its nearest parent (once per level, never in a loop)
        const parent = (p || '').replace(/[\\/]+$/, '').replace(/[\\/][^\\/]*$/, '');
        if (e.status === 404 && parent && parent !== p && tries < 8) {
          if (!tries && !pickFile) missing = p;
          go(parent, tries + 1);
          return;
        }
        swap(note, h('div', { class: 'note bad' }, e.message));
      }
    };
    pathEl.addEventListener('keydown', e => {
      if (e.key !== 'Enter') return;
      e.preventDefault();
      const v = pathEl.value.trim();
      // A file typed in full: taken as it is
      if (pickFile && /\.[a-z0-9]{1,5}$/i.test(v)) { finish(v); return; }
      go(v);
    });

    let createBtn = null;
    if (pickFile) {
      append(m.bd, h('div', { class: 'col' }, pathEl, note, opts.hint ? h('div', { class: 'hint' }, opts.hint) : null, h('div', { class: 'panel', style: 'padding:6px' }, list)));
      append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'));
      go(start || '');
      return;
    }

    const newName = h('input', { type: 'text', value: opts.name || '', placeholder: 'Name of the new folder', spellcheck: false });
    createBtn = h('button', { class: 'btn primary', type: 'button', onclick: () => create() }, icon('plus'), opts.forNew ? 'Create and use' : 'Create');
    const create = async () => {
      const name = newName.value.trim();
      if (!name) { newName.focus(); return; }
      let base = missing || cur;
      if (!base) { swap(note, h('div', { class: 'note bad' }, 'Open a drive or a folder first.')); return; }
      try {
        const r = await api('POST', 'api/browse/mkdir', { path: base, name });
        finish(r.path);
      } catch (e) { swap(note, h('div', { class: 'note bad' }, e.message)); }
    };
    newName.addEventListener('keydown', e => { if (e.key === 'Enter' && !createBtn.disabled) { e.preventDefault(); create(); } });
    const useThis = () => {
      const p = pathEl.value.trim() || cur;
      if (!p) { swap(note, h('div', { class: 'note bad' }, 'Open a folder first.')); return; }
      if (opts.forNew && p === missing) {
        swap(note, h('div', { class: 'note bad' }, 'Give the server its own folder: type a name below and click Create and use.'));
        return;
      }
      if (opts.forNew && p === cur && managed) {
        swap(note, h('div', { class: 'note bad' }, 'This is the folder of ' + inServer + ', already in the manager: pick another one.'));
        return;
      }
      if (opts.forNew && p === cur && !cur.replace(/[\\/]+$/, '').match(/[\\/]/) && !curEmpty) {
        swap(note, h('div', { class: 'note bad' }, 'Not a whole drive: open the server\'s own folder, or create one.'));
        return;
      }
      finish(p);
    };

    append(m.bd, h('div', { class: 'col' },
      pathEl, note,
      h('div', { class: 'panel', style: 'padding:6px' }, list),
      h('div', { class: 'panel', style: 'padding:12px' },
        h('div', { class: 'small', style: 'font-weight:600;color:var(--text-2);margin-bottom:8px' }, opts.forNew ? 'Or create the server\'s folder here' : 'New folder here'),
        h('div', { class: 'inputgroup' }, newName, createBtn)),
      opts.forNew ? h('div', { class: 'hint' }, 'An empty folder: the game is downloaded into it. A folder that already has the game\'s server: it is added as it is.') : null));
    append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'),
      h('button', { class: 'btn' + (opts.forNew ? '' : ' primary'), onclick: useThis }, icon('check'), 'Use this folder'));
    go(opts.forNew ? (opts.root || '') : (start ? start.replace(/[\\/][^\\/]*$/, '') : ''));
    if (opts.forNew) setTimeout(() => { newName.focus(); newName.select(); }, 30);
  });
}

/* =================================================================================================================
 * New server
 * =============================================================================================================== */

// New server, step 1: the game, a name and a new folder, then the download. Step 2 (the server's Set up page):
// its settings and config files, while it downloads.
function pageNew() {
  let step = 1;
  let game = null;
  const f = { name: '', install_dir: '', ip: '', port: 27015, map: '', maxplayers: 0, args: '', gslt: '', rcon_password: '', say_command: 'say', priority: '', autostart: true, autoupdate: null, update_mode: 'direct', console_log: true, restart: '', restart_time: '', notes: '', steam_login: 'anonymous', metamod: null };
  const stepsEl = h('div', { class: 'steps' });
  const body = h('div');

  function renderSteps() {
    // Two steps: the game, its name and folder, then the download (here); the set up comes after it
    swap(stepsEl, h('div', { class: 'step on' }, h('i', null, 1), step === 1 ? 'Install: pick the game' : 'Install'),
      h('div', { class: 'step' }, h('i', null, 2), 'Set up'));
  }

  function render() {
    renderSteps();
    if (step === 1) {
      const groups = ENGINES;
      swap(body, groups.map(([eng, label]) => {
        const list = S.games.filter(x => x.engine === eng);
        if (!list.length) return null;
        return h('div', { class: 'section' }, h('h3', { style: 'margin-bottom:12px' }, label),
          h('div', { class: 'gamegrid' }, list.map(x => h('button', {
            class: 'gamecard' + (game && game.key === x.key ? ' on' : ''),
            disabled: !x.exe.length, title: x.exe.length ? (x.notes || null) : 'Its server does not run on this system (' + (MI().os || 'this machine') + ')',
            onclick: async () => {
              const sug = await run(() => api('GET', 'api/suggest?game=' + encodeURIComponent(x.key)));
              if (!sug) return;
              if (!game || game.key !== x.key) {
                Object.assign(f, { name: sug.name, ip: sug.ip || '', install_dir: sug.install_dir, _root: sug.root, _folder: sug.folder_name, port: sug.port, map: sug.map, maxplayers: sug.maxplayers, rcon_password: sug.rcon_password, gslt: '' });
              }
              game = x;
              step = 2;
              render();
            },
          }, badge(x.key), h('div', null, h('b', null, x.name), h('small', null, !x.exe.length ? 'not on this system' : x.engine === 'goldsrc' ? 'hlds' : x.engine === 'other' ? (x.login ? 'steamcmd · Steam account' : 'steamcmd') : 'srcds'))))));
      }), can('b') ? h('div', { class: 'row', style: 'margin-top:8px' }, h('a', { class: 'btn ghost', href: '#/games/new' }, icon('plus'), 'A game is missing? Add it')) : null);
      return;
    }
    // Step 2: one button. The name and the folder are proposed; both can be changed, neither has to be. The folder
    // decides what the button does: an empty (or new) folder is installed; one with the game's server is added as it is.
    const errs = h('div');
    const dlBtn = h('button', { class: 'btn primary', style: 'font-size:15px;padding:10px 22px' }, icon('download'), 'Install');
    const size = game.key === 'cs2' ? 'Counter-Strike 2 is about 35 GB: the download takes a while.' : game.engine === 'goldsrc' ? 'A small download: a few minutes.' : game.engine === 'other' ? 'The download can be large: it takes a while.' : 'The download takes a few minutes.';
    const folderText = h('span', { class: 'mono', style: 'word-break:break-all' });
    const found = h('div'); // what the folder holds
    const sizeLine = h('div', { class: 'muted small' });
    let insp = null; // the folder, as the manager sees it
    let mode = 'install'; // install | existing | blocked
    let updateToo = false;
    const loginField = h('div');
    const setMode = md => {
      mode = md;
      swap(dlBtn, md === 'existing' ? [icon('check'), updateToo ? 'Add and update it' : 'Add this server'] : [icon('download'), 'Install']);
      dlBtn.disabled = md === 'blocked';
      loginField.classList.toggle('hidden', md === 'existing' && !updateToo);
      sizeLine.textContent = md === 'existing' ? 'Nothing to download: its files stay as they are. The settings come after, in plain words.' : size + ' The settings come after, in plain words.';
    };
    const inspect = async () => {
      folderText.textContent = f.install_dir || '(pick one)';
      clear(found);
      insp = null;
      if (!f.install_dir) { setMode('install'); return; }
      try {
        insp = await api('GET', 'api/inspect?' + new URLSearchParams({ path: f.install_dir, game: game.key }));
      } catch (e) { swap(found, h('div', { class: 'note bad' }, e.message)); setMode('blocked'); return; }
      const exeNames = (insp.expected || []).map(x => x.split('/').pop()).join(' or ') || 'its program';
      const pickProgram = async () => {
        const p = await browseFolder(insp.path, { files: 'program', title: 'The program that starts the server', hint: 'Usually ' + exeNames + '. Pick the one this server uses.' });
        if (p) { f.exe = p; showCustom(); }
      };
      const showCustom = () => {
        swap(found, h('div', { class: 'note ok found' }, icon('check'),
          h('div', { class: 'grow' }, h('b', null, 'Added as it is, with its own program'),
            h('div', { class: 'small mono', style: 'word-break:break-all;margin-top:2px' }, f.exe),
            h('div', { class: 'small', style: 'margin-top:4px' }, 'Nothing is downloaded. ', h('a', { href: '', onclick: e => { e.preventDefault(); delete f.exe; inspect(); } }, 'Undo')))));
        setMode('existing');
      };
      if (f.exe) { showCustom(); return; }
      if (insp.server) {
        swap(found, h('div', { class: 'note bad' }, insp.server + ' already uses this folder' + (insp.path === f.install_dir ? '' : ', or one inside it') + ': pick another one.'));
        setMode('blocked');
      } else if (!insp.exists || insp.empty) {
        swap(found, h('div', { class: 'muted small' }, insp.exists ? 'An empty folder: ' + game.name + ' is downloaded into it.' : 'A new folder: it is created, and ' + game.name + ' is downloaded into it.'));
        setMode('install');
      } else if (insp.exe) {
        const upd = h('input', { type: 'checkbox', checked: updateToo });
        upd.addEventListener('change', () => { updateToo = upd.checked; setMode('existing'); });
        swap(found, h('div', { class: 'note ok found' }, icon('check'),
          h('div', { class: 'grow col', style: 'gap:6px' },
            h('div', null, h('b', null, 'A ' + game.name + ' server is already here'),
              h('div', { class: 'small mono', style: 'word-break:break-all;margin-top:2px' }, insp.exe)),
            h('div', { class: 'small' }, [
              insp.version ? 'Version ' + insp.version + '. ' : '',
              insp.hostname ? 'It calls itself "' + insp.hostname + '". ' : '',
              'It is added as it is: nothing is downloaded, its files and configs stay. ',
              insp.has_rcon ? 'Its RCON password (server.cfg) is kept. ' : '',
              'If it is running, the manager takes it over.'].join('')),
            h('label', { class: 'check', style: 'color:var(--text-2)' }, upd, h('span', null, h('b', null, 'Update it with steamcmd now'), h('small', null, 'Otherwise it is updated by itself when Steam has a new version.'))))));
        setMode('existing');
      } else if ((insp.detected || []).length) {
        const other = insp.detected[0];
        swap(found, h('div', { class: 'note warn col', style: 'gap:8px' },
          h('div', null, 'This folder has a ', h('b', null, other.name), ' server, not ' + game.name + '.'),
          h('div', { class: 'row wrap', style: 'gap:8px' },
            S.gamesBy[other.key] ? h('button', { class: 'btn sm', onclick: async () => {
              const sug = await run(() => api('GET', 'api/suggest?game=' + encodeURIComponent(other.key)));
              if (!sug) return;
              Object.assign(f, { name: sug.name, ip: sug.ip || '', port: sug.port, map: sug.map, maxplayers: sug.maxplayers, _root: sug.root, _folder: sug.folder_name });
              game = S.gamesBy[other.key];
              render();
            } }, 'Make it a ' + other.name + ' server') : null,
            h('button', { class: 'btn sm ghost', onclick: pickProgram }, icon('terminal'), 'Pick its program'))));
        setMode('blocked');
      } else if (insp.partial) {
        swap(found, h('div', { class: 'note info' }, 'A download that did not finish is in this folder: Install finishes it.'));
        setMode('install');
      } else {
        swap(found, h('div', { class: 'note warn col', style: 'gap:8px' },
          h('div', null, 'This folder is not empty, and has no ' + exeNames + '. Pick an empty folder, or the folder of a ' + game.name + ' server.'),
          h('div', { class: 'small' }, 'If this server starts with another program, pick it: it is added as it is.'),
          h('div', { class: 'row wrap', style: 'gap:8px' },
            h('button', { class: 'btn sm', onclick: pickProgram }, icon('terminal'), 'Pick its program'),
            h('button', { class: 'btn sm ghost', onclick: () => change.click() }, icon('folder'), 'Another folder'))));
        setMode('blocked');
      }
    };
    const change = h('button', { class: 'btn sm ghost', type: 'button', onclick: async () => {
      const p = await browseFolder('', { forNew: true, root: f._root, name: f._folder });
      if (p) { f.install_dir = p; delete f.exe; updateToo = false; inspect(); }
    } }, icon('folder'), 'Change');
    // Steam login: anonymous for the free dedicated servers; an account, with its password for this download only
    let steamPass = '';
    const passIn = h('input', { type: 'password', autocomplete: 'new-password', placeholder: 'Its password (not saved)', oninput: e => { steamPass = e.target.value; } });
    const passWrap = h('div', { class: 'hidden' }, passIn, h('small', { class: 'muted' }, 'Used for this download only. If Steam asks for a Steam Guard code, the next page asks you for it.'));
    const loginIn = bindText(f, 'steam_login', { placeholder: 'anonymous', autocomplete: 'off' });
    const showPass = () => passWrap.classList.toggle('hidden', !f.steam_login || f.steam_login.trim().toLowerCase() === 'anonymous');
    loginIn.addEventListener('input', showPass);
    showPass();
    append(loginField, field('Steam login', h('div', { class: 'col', style: 'gap:8px' }, loginIn, passWrap), 'anonymous works for every game here. A Steam account only if a game needs one.'));
    swap(body,
      h('div', { class: 'panel', style: 'max-width:760px' },
        h('div', { class: 'hd' }, badge(game.key, 'sm'), h('h2', { class: 'grow' }, game.name), h('button', { class: 'btn sm ghost', onclick: () => { step = 1; render(); } }, 'Change game')),
        h('div', { class: 'bd col' },
          field('Name', bindText(f, 'name', { required: true, maxlength: 80 }), 'How it shows in the manager. Change it if you like.'),
          h('div', null, h('div', { class: 'small', style: 'font-weight:600;color:var(--text-2);margin-bottom:6px' }, 'Folder'),
            h('div', { class: 'row wrap', style: 'gap:10px' }, folderText, change),
            h('div', { class: 'hint', style: 'margin-top:4px' }, 'A new or empty folder to install into, or the folder of a ' + game.name + ' server you already have.')),
          found,
          loginField,
          sizeLine,
          errs)),
      h('div', { class: 'row', style: 'max-width:760px;margin-top:18px' },
        h('button', { class: 'btn ghost', onclick: () => { step = 1; render(); } }, icon('back'), 'Back'), h('span', { class: 'spacer' }), dlBtn));
    setMode('install');
    inspect();
    dlBtn.onclick = async () => {
      if (!f.name.trim()) { swap(errs, h('div', { class: 'note bad' }, 'Give the server a name.')); return; }
      if (!f.install_dir.trim()) { swap(errs, h('div', { class: 'note bad' }, 'Pick its folder: Change.')); return; }
      if (mode === 'blocked') return;
      dlBtn.disabled = true;
      clear(errs);
      try {
        const server = Object.assign({}, f, { game: game.key });
        delete server._root;
        delete server._folder;
        const existing = mode === 'existing';
        const r = await api('POST', 'api/servers', { server, install: !existing || updateToo, update: existing && updateToo, start: false, password: steamPass });
        applyState(await api('GET', 'api/state'));
        // on a linked machine, the hub hears of it within seconds
        for (let k = 0; k < 12 && !serverBy(sid(r.id)); k++) {
          await new Promise(res => setTimeout(res, 500));
          applyState(await api('GET', 'api/state'));
        }
        if (existing) toast(f.name + ' added' + (updateToo ? ': it is being updated.' : '.'), 'ok');
        location.hash = '#/server/' + encodeURIComponent(sid(r.id)) + '/setup';
      } catch (e) {
        const list = e.data && e.data.errors ? e.data.errors : [e.message];
        swap(errs, h('div', { class: 'note bad' }, list.map(x => h('div', null, x))));
        dlBtn.disabled = false;
      }
    };
  }
  render();
  return {
    el: h('div', null, h('a', { class: 'back', href: '#/' }, icon('back'), 'Servers'),
      h('div', { class: 'pagehead' }, h('div', null, h('h1', null, 'New server'), h('div', { class: 'sub' }, 'Pick a game and click Install: the manager downloads it, then asks a few simple questions. Already have the server in a folder? Pick that folder: it is added as it is.'))),
      stepsEl, body),
  };
}

/* =================================================================================================================
 * Addons: a master folder per game, pushed into the servers
 * =============================================================================================================== */

function pageAddons() {
  const wrap = h('div', null, h('div', { class: 'muted' }, 'Loading…'));
  const rootEl = h('span', { class: 'mono' });
  let data = null;
  const SUGGEST = ['metamod2', 'metamod1', 'counterstrikesharp', 'sourcemod', 'amxmodx', 'metamod-r', 'reapi'];

  async function load() {
    const r = await run(() => api('GET', 'api/addons'));
    if (!r) return;
    data = r;
    rootEl.textContent = r.root;
    render();
  }

  function render() {
    if (!data.packages.length) {
      swap(wrap, h('div', { class: 'panel empty' }, icon('package'), h('h2', null, 'No addon package yet'),
        h('p', null, 'A package is a plugin, by brand and version: metamod2, counterstrikesharp, sourcemod, amxmodx… Upload its release archive (.zip), tick the servers that get it, push.'),
        h('button', { class: 'btn primary', onclick: newPackage }, icon('plus'), 'New package')));
      return;
    }
    swap(wrap, data.packages.map(card));
    // while one is being checked or installed: its progress
    clearTimeout(render.t);
    if (data.packages.some(p => p.update && p.update.busy)) render.t = setTimeout(() => { if (wrap.isConnected) load(); }, 2000);
  }

  function card(p) {
    const users = machineServers().filter(v => p.servers.includes(v.id)); // its IDs, as the page's machine knows them
    const status = p.files ? `${p.files} files · ${fmtBytes(p.bytes)} · changed ${timeAgo(p.modified)}` : 'Empty: upload its archive';
    const archive = h('input', { type: 'file', accept: '.zip,.tar.gz,.tgz', class: 'hidden', onchange: () => { const f = archive.files[0]; archive.value = ''; if (f) uploadArchive(p, f); } });
    return h('div', { class: 'panel', style: 'margin-bottom:16px' },
      h('div', { class: 'hd' }, icon('package'),
        h('div', { class: 'grow' }, h('h2', null, p.name), h('div', { class: 'muted small' }, status)),
        h('div', { class: 'btngroup' },
          h('button', { class: 'btn sm', title: 'Upload the release archive (.zip, .tar.gz) of this package', onclick: () => archive.click() }, icon('upload'), 'Upload archive'), archive,
          h('button', { class: 'btn sm', title: 'Browse, edit and upload single files', onclick: () => browse(p) }, icon('folder'), 'Files'),
          p.files && users.length ? h('button', { class: 'btn sm primary', onclick: () => pushPackage(p, users) }, icon('upload'), `Push to ${users.length} server${users.length > 1 ? 's' : ''}`) : null,
          h('button', { class: 'btn sm ghost icon', title: 'Delete this package', 'aria-label': 'Delete ' + p.name, onclick: () => del(p) }, icon('trash')))),
      h('div', { class: 'bd col', style: 'gap:14px' },
        p.top.length ? h('div', { class: 'chips' }, p.top.map(t => h('span', { class: 'tag' }, t))) :
          h('div', { class: 'muted small' }, 'Nothing in addons/ yet.'),
        updatesBox(p),
        h('div', null,
          h('div', { class: 'small', style: 'font-weight:600;color:var(--text-2);margin-bottom:8px' }, 'Servers that get it'),
          machineServers().length ? h('div', { class: 'chips' }, machineServers().map(v => {
            const on = p.servers.includes(v.id);
            return h('button', {
              class: 'chip' + (on ? ' on' : ''), disabled: !can('f') && !can('h'),
              title: on ? 'Click: no longer gets it (its files stay on the server)' : 'Click: gets it at the next push',
              onclick: async () => { if (await run(() => api('POST', `api/servers/${encodeURIComponent(v.id)}/package?name=${encodeURIComponent(p.name)}&on=${on ? 0 : 1}`))) load(); },
            }, (S.gamesBy[v.game] || { short: v.game }).short + ' · ' + v.name);
          })) : h('div', { class: 'muted small' }, 'No server yet.'))));
  }

  /* ---------- Updates of a package: where its new versions come out, and what is done with them ---------- */

  const KINDS = [['none', 'Not checked'], ['sourcemod', 'SourceMod'], ['metamod', 'Metamod:Source'], ['counterstrikesharp', 'CounterStrikeSharp'],
    ['amxmodx', 'AMX Mod X'], ['github', 'A GitHub project\'s releases'], ['link', 'A link']];
  const EVERY = [['', 'Daily'], ['6h', 'Every 6 hours'], ['1h', 'Every hour'], ['off', 'Never by itself']];
  const WATCH = [['', 'Every hour'], ['30m', 'Every 30 minutes'], ['15m', 'Every 15 minutes'], ['3h', 'Every 3 hours'], ['off', 'Off']];
  const clock = t => {
    const d = new Date(t * 1000);
    const hm = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    return new Date().toDateString() === d.toDateString() ? hm : d.toLocaleDateString([], { month: 'short', day: 'numeric' }) + ' ' + hm;
  };
  const srcText = src => {
    switch (src.kind) {
      case 'sourcemod': return 'SourceMod ' + src.branch;
      case 'metamod': return 'Metamod:Source ' + src.branch;
      case 'amxmodx': return 'AMX Mod X ' + src.branch;
      case 'counterstrikesharp': return 'CounterStrikeSharp' + (src.no_runtime ? ' (without the .NET runtime)' : '');
      case 'github': return 'GitHub ' + src.repo;
      case 'link': return src.url;
    }
    return src.kind;
  };
  const sname = id => { const v = machineServers().find(x => x.id === id); return v ? v.name : id; };

  function updatesBox(p) {
    const u = p.update || {};
    const src = u.source;
    const label = h('div', { class: 'small', style: 'font-weight:600;color:var(--text-2)' }, 'Updates');
    const btns = h('div', { class: 'btngroup' },
      src ? h('button', { class: 'btn sm', disabled: !!u.busy, title: 'Ask ' + srcText(src) + ' for its newest version now', onclick: () => checkNow(p) }, icon('refresh'), 'Check now') : null,
      h('button', { class: 'btn sm ghost', title: 'Where its new versions come out, how often they are checked, what is done with them', onclick: () => updateSettings(p) }, icon('sliders'), 'Settings'));
    if (!src) {
      return h('div', { class: 'col pkgupd', style: 'gap:8px' }, h('div', { class: 'row' }, label, h('div', { class: 'grow' }), btns),
        h('div', { class: 'muted small' }, 'Not checked. Settings picks where its new versions come out: SourceMod, Metamod:Source, CounterStrikeSharp, AMX Mod X, a GitHub project, or a link.'));
    }
    const every = (EVERY.find(e => e[0] === (src.every || '')) || EVERY[0])[1].toLowerCase();
    const how = (src.every === 'off' ? 'checked by hand' : 'checked ' + every) + ', ' + (src.auto ? 'a new version installed by itself' : 'a new version told') + (u.set ? '' : ' (the default for its name and its servers\' game: Settings changes it)');
    const rows = [h('div', { class: 'row' }, label, h('span', { class: 'muted small grow ellipsis', title: how }, srcText(src) + ' · ' + how), btns)];
    const l = u.latest;
    const has = u.installed ? h('span', { title: u.installed_file ? 'From ' + u.installed_file : null }, 'Installed: ', h('b', null, u.installed))
      : h('span', { class: 'muted', title: 'Filled from a server, or from an archive whose name does not say its version. Its program files are compared with the newest version: the same, that is its version.' }, 'Installed: not known');
    let out = null;
    if (l) {
      const notes = l.page ? h('a', { href: l.page, target: '_blank', rel: 'noopener noreferrer', title: 'Its release notes or downloads page' }, icon('external')) : null;
      out = u.new ? h('span', { class: 'tag amber', title: l.file }, 'New: ' + l.version)
        : u.current ? h('span', { class: 'tag green', title: l.file }, 'Up to date')
        : u.installed ? h('span', { class: 'muted', title: l.file + ': not newer than the one it has' }, 'Newest out: ' + l.version)
        : h('span', { title: l.file }, 'Newest: ', h('b', null, l.version));
      out = h('span', { class: 'row', style: 'gap:6px' }, out, l.published ? h('span', { class: 'muted small' }, timeAgo(l.published)) : null, notes);
    }
    const acts = h('div', { class: 'btngroup' },
      l && (u.new || !u.installed) && !u.busy ? h('button', { class: 'btn sm primary', onclick: () => install(p) }, icon('download'), 'Install ' + l.version) : null,
      l && !u.installed && !u.busy ? h('button', { class: 'btn sm ghost', title: 'The package has ' + l.version + ' already: from now on, the next version is told (or installed)', onclick: async () => { if (await run(() => api('POST', `api/addons/pkg/${encodeURIComponent(p.name)}/have`))) load(); } }, 'Mark as installed') : null);
    rows.push(h('div', { class: 'row small', style: 'gap:12px;flex-wrap:wrap' }, has, out, h('div', { class: 'grow' }), acts));
    const when = [];
    if (u.checked) when.push('checked ' + timeAgo(u.checked));
    if (u.next && src.every !== 'off' || u.next && u.watch) when.push('next check ' + clock(u.next));
    if (when.length) rows.push(h('div', { class: 'muted small' }, when.join(' · ')));
    if (u.error) rows.push(h('div', { class: 'note bad small' }, u.error));
    if (u.busy) rows.push(h('div', { class: 'note info small' }, h('i', { class: 'spin', style: 'margin-right:6px' }), u.busy));
    const w = u.watch;
    if (w) {
      const wevery = (WATCH.find(e => e[0] === (src.crash_watch || '')) || WATCH[0])[1].toLowerCase();
      rows.push(h('div', { class: 'note warn small row', style: 'gap:10px' },
        h('div', { class: 'grow' }, h('b', null, 'Crash watch'), ` since ${clock(w.since)}: ${w.why}. Checked ${wevery}` + (u.installed || w.base ? `; a version newer than ${u.installed || w.base} is installed when it comes out` : '; the fix is installed when it comes out') +
          ', and the servers the manager gave up on are started again.' + (w.vanilla && w.vanilla.length ? ' Without their addons meanwhile: ' + w.vanilla.map(sname).join(', ') + '.' : '')),
        h('button', { class: 'btn sm ghost', title: 'Stop watching: back to the usual checks', onclick: async () => { if (await run(() => api('POST', `api/addons/pkg/${encodeURIComponent(p.name)}/unwatch`))) load(); } }, 'Stop')));
    }
    const r = u.rollout;
    if (r && r.held) rows.push(h('div', { class: 'note bad small' }, `${r.version} held back: ${r.held}.` + (r.rest && r.rest.length ? ` ${r.rest.map(sname).join(', ')} ${r.rest.length > 1 ? 'keep the version they have: Push gives it to them' : 'keeps the version it has: Push gives it the new one'}.` : '')));
    else if (r && r.rest && r.rest.length && !u.busy) rows.push(h('div', { class: 'note info small' }, `${r.version}: ${r.rest.length} server${r.rest.length > 1 ? 's' : ''} still to get it (it goes on).`));
    return h('div', { class: 'col pkgupd', style: 'gap:8px' }, rows);
  }

  async function checkNow(p) {
    const r = await run(() => api('POST', `api/addons/pkg/${encodeURIComponent(p.name)}/check`));
    if (!r) return;
    const u = r.update;
    if (r.error) toast(p.name + ': ' + r.error, 'bad');
    else if (u.new) toast(`${p.name}: ${u.latest.version} is out.`, 'ok');
    else if (u.latest) toast(`${p.name}: ${u.installed ? 'up to date' : 'the newest is ' + u.latest.version}.`, 'ok');
    load();
  }

  async function install(p) {
    const u = p.update, l = u.latest;
    const users = machineServers().filter(v => p.servers.includes(v.id));
    const one = users.length > 1 && !u.source.all_at_once;
    const text = [`${l.file} is downloaded and put into the package: its files over the package's, the files a push never overwrites kept (Settings, Addons).`,
      users.length ? (one ? ` Then it goes to one server first, watched ${'10 minutes'}, then to the others one at a time` : ` Then it goes to ${users.map(v => v.name).join(', ')}, one at a time`) + ': each running server warns its players, stops for the copy and starts again.' : ' No server gets this package.'];
    if (!await confirmBox(`Install ${p.name} ${l.version}?`, text.join(''), 'Install')) return;
    if (await run(() => api('POST', `api/addons/pkg/${encodeURIComponent(p.name)}/install`), 'Installing ' + l.version + ': Activity says how it goes.')) load();
  }

  function updateSettings(p) {
    const u = p.update || {};
    const o = Object.assign({ kind: 'none', branch: '', repo: '', asset: '', prereleases: false, no_runtime: false, url: '', every: '', auto: false, crash_watch: '', all_at_once: false }, u.source || {});
    // the branch when none is set: the manager's (Metamod: from the name, else the games of its servers)
    const defBranch = Object.assign({ sourcemod: '1.12', metamod: '2.0', amxmodx: '1.10' }, u.default_branch || {});
    if (u.source && !u.set) o.branch = ''; // the default
    const m = modal({ title: h('span', null, 'Updates of ', h('b', null, p.name)), size: 'wide' });
    const inp = (key, attrs) => { const i = h('input', Object.assign({ type: 'text', spellcheck: false, value: o[key] || '' }, attrs || {})); i.addEventListener('input', () => { o[key] = i.value.trim(); }); return i; };
    const chk = (key, title, desc) => { const i = h('input', { type: 'checkbox', checked: !!o[key] }); i.addEventListener('change', () => { o[key] = i.checked; }); return h('label', { class: 'check' }, i, h('span', null, h('b', null, title), desc ? h('small', null, desc) : null)); };
    const sel = (key, opts, onchange) => { const sl = h('select', null, opts.map(([v, l]) => h('option', { value: v, selected: (o[key] || '') === v }, l))); sl.addEventListener('change', () => { o[key] = sl.value; if (onchange) onchange(); }); return sl; };
    const segOf = (key, opts) => {
      const el = h('div', { class: 'seg', style: 'display:inline-flex;width:auto;align-self:flex-start' });
      const draw = () => swap(el, opts.map(([v, l]) => h('button', { type: 'button', class: o[key] === v ? 'on' : '', style: 'white-space:nowrap', onclick: () => { o[key] = v; draw(); } }, l)));
      draw();
      return el;
    };
    const pane = h('div', { class: 'formgrid' });
    const rest = h('div', { class: 'col', style: 'gap:14px' });
    const drawPane = () => {
      const k = o.kind;
      // the branches AlliedModders builds (a list: a typed one may not exist)
      const BRANCHES = {
        metamod: [['2.0', '2.0 (CS2)'], ['1.12', '1.12 (Source games: CS:S, TF2…)'], ['1.11', '1.11 (older)']],
        sourcemod: [['1.12', '1.12 (stable)'], ['1.13', '1.13 (in the works)'], ['1.11', '1.11 (older)']],
        amxmodx: [['1.10', '1.10'], ['1.9', '1.9 (older)']],
      };
      const branch = () => {
        const opts = [['', 'Default: ' + defBranch[k]]].concat(BRANCHES[k] || []);
        if (o.branch && !opts.some(x => x[0] === o.branch)) opts.push([o.branch, o.branch]);
        if (!opts.some(x => x[0] === (o.branch || ''))) o.branch = '';
        return field('Branch', sel('branch', opts), 'The builds of that version of ' + (k === 'metamod' ? 'Metamod:Source' : k === 'sourcemod' ? 'SourceMod' : 'AMX Mod X') + ', the newest first.');
      };
      const parts = {
        none: [h('div', { class: 'muted small wide' }, 'Its new versions are not looked for.')],
        sourcemod: [branch(), h('div', { class: 'muted small' }, 'AlliedModders\' builds (sm.alliedmods.net): the newest of the branch, for this machine\'s system.')],
        metamod: [branch(), h('div', { class: 'muted small' }, 'AlliedModders\' builds (mms.alliedmods.net): the newest of the branch, for this machine\'s system.')],
        amxmodx: [branch(), h('div', { class: 'muted small' }, 'The base package of AlliedModders\' builds (amxmodx.org), for this machine\'s system.')],
        counterstrikesharp: [h('div', { class: 'wide' }, chk('no_runtime', 'Without the .NET runtime', 'Its releases come with the .NET runtime it needs (addons/counterstrikesharp/dotnet), or without it, smaller, when the runtime the servers have is still the right one. With it is the safe choice.')),
          h('div', { class: 'muted small wide' }, 'Its GitHub releases (roflmuffin/CounterStrikeSharp), for this machine\'s system.')],
        github: [field('Project', inp('repo', { placeholder: 'owner/name' }), 'As in github.com/owner/name.'),
          field('Its file', inp('asset', { placeholder: '*{os}*.zip' }), 'A pattern of the release\'s archive: * for anything, {os} for windows or linux. Empty: the archive naming this machine\'s system.'),
          h('div', { class: 'wide' }, chk('prereleases', 'Pre-releases count too', 'Else only the releases the project calls ready.'))],
        link: [field('Link', inp('url', { placeholder: 'https://…/latest.zip' }), 'An archive (.zip, .tar.gz), always at this address: a new one is told by the version in where it leads, its name, or its date. Or a small file that names the newest archive in its folder.', true)],
      };
      swap(pane, field('Where its new versions come out', sel('kind', KINDS, drawPane)), ...(parts[k] || []));
      rest.style.display = k === 'none' ? 'none' : '';
    };
    append(rest,
      h('div', { class: 'formgrid' },
        field('Checked', segOf('every', EVERY), null, true),
        field('A new version', segOf('auto', [[false, 'Tell me'], [true, 'Install it']]), 'Told in Activity, Discord and the forum (Updates). Installed: the package updated (the files a push never overwrites kept), then pushed into its servers, their players warned first.', true),
        field('After crashes it causes', sel('crash_watch', WATCH), 'When a crash report blames this addon (sure or likely), or a server crashes again and again after a game update: checked this often, the fix installed when it comes out (even with Tell me), and the servers the manager gave up on started again.', true),
        h('div', { class: 'wide' }, chk('all_at_once', 'All the servers at once', 'Else a new version goes to one server first; after 10 minutes without a crash, to the others. A crash there holds it back from them.'))));
    drawPane();
    const err = h('div', { class: 'err' });
    append(m.bd, h('div', { class: 'col', style: 'gap:18px' }, pane, rest, err));
    const save = h('button', { class: 'btn primary' }, icon('check'), 'Save');
    const reset = u.set ? h('button', { class: 'btn ghost', title: 'As its name says (a package named sourcemod, metamod2, counterstrikesharp, amxmodx…: daily, told), else not checked', onclick: async () => {
      if (await run(() => api('PUT', `api/addons/pkg/${encodeURIComponent(p.name)}/source`, null))) { m.close(); load(); }
    } }, 'Back to the default') : null;
    append(m.ft, reset, h('div', { class: 'grow' }), h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'), save);
    save.onclick = async () => {
      err.textContent = '';
      const k = o.kind;
      const out = { kind: k, every: o.every, auto: o.auto, crash_watch: o.crash_watch, all_at_once: o.all_at_once };
      if (['sourcemod', 'metamod', 'amxmodx'].includes(k)) out.branch = o.branch;
      if (k === 'counterstrikesharp') out.no_runtime = o.no_runtime;
      if (k === 'github') Object.assign(out, { repo: o.repo, asset: o.asset, prereleases: o.prereleases });
      if (k === 'link') out.url = o.url;
      try {
        await api('PUT', `api/addons/pkg/${encodeURIComponent(p.name)}/source`, out);
        m.close();
        toast(p.name + ': saved; checked in a moment.', 'ok');
        load();
      } catch (e) { err.textContent = e.message; }
    };
  }

  function browse(p) {
    const m = modal({ title: h('span', null, 'Package ', h('b', null, p.name)), size: 'full', buttons: false, onClose: load });
    const fb = fileBrowser('api/addons/pkg/' + encodeURIComponent(p.name), 'pkgpath.' + p.name, p.name);
    m.bd.style.padding = '16px';
    m.bd.style.overflow = 'auto';
    append(m.bd, h('div', { class: 'hint', style: 'margin-bottom:12px' }, 'This folder mirrors the game folder of a server (game/csgo for CS2, cstrike for CS:S and CS 1.6): addons/…, cfg/…'), fb.el);
  }

  function uploadArchive(p, file) {
    choiceBox('Upload ' + file.name + ' into ' + p.name + '?', [
      h('ul', { class: 'plain', style: 'margin:0 0 8px' },
        h('li', null, h('b', null, 'Replace'), ': the package becomes the archive (a new version: files it no longer has are removed from the package).'),
        h('li', null, h('b', null, 'Update'), ': the archive\'s files are written over the package\'s; the package\'s other files stay (your extra plugins, your configs it does not have).'),
        h('li', null, h('b', null, 'Add only new files'), ': only what the package does not have yet; nothing is overwritten.')),
      'The servers keep their own files either way, until the next push. If the archive wraps everything in a folder, it is unwrapped so that addons/ lands at the top.'],
      [{ value: 'replace', label: 'Replace', cls: 'primary' }, { value: 'update', label: 'Update' }, { value: 'add', label: 'Add only new files' }]).then(mode => {
      if (!mode) return;
      const fd = new FormData();
      fd.append('file', file, file.name);
      const t = h('div', { class: 'toast info' }, icon('upload'), h('div', { class: 'grow' }, 'Uploading ' + file.name + '… ', h('b', null, '0%')));
      $('#toasts').append(t);
      const xhr = new XMLHttpRequest();
      xhr.open('POST', machineURL(`api/addons/pkg/${encodeURIComponent(p.name)}/archive?mode=${mode}`));
      xhr.setRequestHeader('X-SZM', '1');
      xhr.upload.onprogress = e => { if (e.lengthComputable) t.querySelector('b').textContent = Math.round(e.loaded / e.total * 100) + '%'; };
      xhr.onload = () => {
        t.remove();
        let d = {};
        try { d = JSON.parse(xhr.responseText); } catch (e) { /* */ }
        if (xhr.status === 200) { toast(`${file.name}: ${d.files} files written in ${p.name}` + (d.kept ? `, ${d.kept} kept as they were.` : '.'), 'ok'); load(); } else toast(d.error || 'Upload failed.', 'bad');
      };
      xhr.onerror = () => { t.remove(); toast('Upload failed.', 'bad'); };
      xhr.send(fd);
    });
  }

  async function pushPackage(p, users) {
    const busy = users.filter(v => isUp(v) && humans(v) > 0);
    let warn = false;
    if (busy.length) {
      const pick = await choiceBox(`Push ${p.name}?`,
        `${busy.map(v => v.name + ' (' + humans(v) + ')').join(', ')} ${busy.length > 1 ? 'have' : 'has'} players. They can be warned in the chat first, or disconnected now. Each running server stops for the copy and starts again.`,
        [{ value: 'warn', label: 'Warn players first', cls: 'primary' }, { value: 'now', label: 'Push now', cls: 'danger' }]);
      if (!pick) return;
      warn = pick === 'warn';
    } else if (!await confirmBox(`Push ${p.name}?`, `Into ${users.map(v => v.name).join(', ')}: what changed only. Running servers stop for the copy and start again; their plugin configs are kept.`, 'Push')) return;
    const r = await run(() => api('POST', `api/addons/pkg/${encodeURIComponent(p.name)}/push${warn ? '?warn=1' : ''}`));
    if (!r) return;
    if (r.started.length) toast((warn ? 'Warning the players, then updating: ' : 'Updating: ') + r.started.join(', '), 'ok');
    (r.errors || []).forEach(e => toast(e, 'bad'));
  }

  async function del(p) {
    if (await confirmBox('Delete the package ' + p.name + '?', 'Its folder is deleted. The servers keep the files they got from it.', 'Delete', true) &&
        await run(() => api('DELETE', 'api/addons/pkg/' + encodeURIComponent(p.name)), p.name + ' deleted.')) load();
  }

  function newPackage() {
    const m = modal({ title: 'New addon package', size: 'wide' });
    const name = h('input', { type: 'text', spellcheck: false, placeholder: 'metamod2', list: 'szm-pkg-names' });
    const names = h('datalist', { id: 'szm-pkg-names' }, SUGGEST.filter(n => !data.packages.some(p => p.name === n)).map(n => h('option', { value: n })));
    const src = { how: 'archive' };
    const archive = h('input', { type: 'file', accept: '.zip,.tar.gz,.tgz' });
    const fromSrv = h('select', null, machineServers().map(v => h('option', { value: v.id }, v.name)));
    const fromDir = h('select', null, h('option', { value: '' }, '…'));
    const loadDirs = async () => {
      swap(fromDir, h('option', { value: '' }, 'Loading…'));
      const r = await run(() => api('GET', `api/servers/${encodeURIComponent(fromSrv.value)}/addon-folders`));
      const list = r ? r.folders : [];
      swap(fromDir, list.length ? list.map(f => h('option', { value: f }, 'addons/' + f)) : h('option', { value: '' }, 'No addons folder'));
      if (list.length && !name.value) name.value = list[0];
    };
    fromSrv.addEventListener('change', loadDirs);
    fromDir.addEventListener('change', () => { if (!name.value || SUGGEST.includes(name.value) || [...fromDir.options].some(o => o.value === name.value)) name.value = fromDir.value; });
    const panes = {
      archive: h('div', { class: 'col' }, field('Release archive (.zip, .tar.gz)', archive, 'As downloaded from the plugin\'s site: it is unwrapped so that addons/ lands at the top.')),
      server: h('div', { class: 'formgrid' }, field('Server', fromSrv), field('Folder', fromDir, 'Its cfg/<folder> comes too, if there is one (SourceMod).')),
      empty: h('div', { class: 'muted small' }, 'An empty package with an addons folder: fill it later with Upload archive or Files.'),
    };
    const seg = h('div', { class: 'seg' });
    const body = h('div');
    const pick = how => {
      src.how = how;
      swap(seg, [['archive', 'Upload an archive'], ['server', 'Copy from a server'], ['empty', 'Empty']].map(([k, l]) => h('button', { type: 'button', class: k === how ? 'on' : '', onclick: () => pick(k) }, l)));
      swap(body, panes[how]);
      if (how === 'server' && machineServers().length && fromDir.options.length <= 1) loadDirs();
    };
    pick('archive');
    const err = h('div', { class: 'err' });
    append(m.bd, h('div', { class: 'col', style: 'gap:16px' },
      field('Name', h('div', null, name, names), 'By brand and version: metamod2 (CS2), metamod1 (Source games), counterstrikesharp, sourcemod, amxmodx…'),
      seg, body, err));
    const create = h('button', { class: 'btn primary' }, icon('check'), 'Create');
    append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'), create);
    create.onclick = async () => {
      err.textContent = '';
      const n = name.value.trim();
      if (src.how === 'archive' && !archive.files[0]) { err.textContent = 'Pick the archive to upload.'; return; }
      if (src.how === 'server' && !fromDir.value) { err.textContent = 'Pick a folder of the server.'; return; }
      create.disabled = true;
      try {
        await api('POST', `api/addons/pkg/${encodeURIComponent(n)}/create`);
        if (src.how === 'server') {
          await api('POST', `api/addons/pkg/${encodeURIComponent(n)}/import?from=${encodeURIComponent(fromSrv.value)}&folder=${encodeURIComponent(fromDir.value)}`);
          toast('Copying addons/' + fromDir.value + ' into ' + n + ': Activity says when it is done.', 'ok');
        }
        m.close();
        await load();
        if (src.how === 'archive') {
          const p = data.packages.find(x => x.name === n);
          if (p) {
            // A new package: its archive is its content
            const fd = new FormData();
            fd.append('file', archive.files[0], archive.files[0].name);
            const r = await fetch(machineURL(`api/addons/pkg/${encodeURIComponent(n)}/archive?mode=replace`), { method: 'POST', body: fd, headers: { 'X-SZM': '1' } });
            const d = await r.json().catch(() => ({}));
            if (r.ok) toast(`${n}: ${d.files} files.`, 'ok'); else toast(d.error || 'Upload failed.', 'bad');
            load();
          }
        }
      } catch (e) { err.textContent = e.message; create.disabled = false; }
    };
    setTimeout(() => name.focus(), 30);
  }

  load();
  return {
    el: h('div', { style: 'max-width:1000px' },
      h('div', { class: 'pagehead' },
        h('div', { class: 'grow' }, h('h1', null, 'Addons'),
          h('div', { class: 'sub' }, 'Plugin packages, one folder per brand and version, each mirroring a server\'s game folder (addons/…, cfg/…). Each server gets the packages you tick; a push copies only what changed and keeps its plugin configs. Folders in ', rootEl, '.')),
        h('button', { class: 'btn primary', onclick: () => data && newPackage() }, icon('plus'), 'New package')),
      wrap),
    onEvent: ev => { if (/addon package|addons/i.test(ev.text)) load(); },
  };
}

/* =================================================================================================================
 * Activity (all events)
 * =============================================================================================================== */

function pageEvents() {
  const kinds = { crash: 'Crashes', failed: 'Gave up', error: 'Errors', update: 'Updates', updated: 'Updated', restart: 'Restarts', start: 'Starts', stop: 'Stops', steamcmd: 'steamcmd', addons: 'Addons', info: 'Info', manager: 'Manager' };
  const sel = new Set();
  const daySel = h('select', { style: 'width:auto' });
  const srvSel = h('select', { style: 'width:auto' }, h('option', { value: '' }, 'All servers'), S.servers.map(v => h('option', { value: v.id }, v.name)));
  const q = h('input', { type: 'search', placeholder: 'Search…' });
  const chips = h('div', { class: 'chips' });
  const tbody = h('tbody');
  let evs = [];
  let live = true;

  async function load() {
    const day = daySel.value;
    live = !day;
    const r = await run(() => api('GET', 'api/logs?' + new URLSearchParams({ day, server: srvSel.value })));
    if (!r) return;
    evs = r.events;
    if (daySel.options.length <= 1) {
      swap(daySel, h('option', { value: '' }, 'Latest'), r.days.map(d => h('option', { value: d }, new Date(d + 'T12:00:00').toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' }))));
    }
    render();
  }

  function render() {
    swap(chips, Object.entries(kinds).map(([k, label]) => {
      const n = evs.filter(e => e.kind === k).length;
      if (!n && !sel.has(k)) return null;
      return h('button', { class: 'chip' + (sel.has(k) ? ' on' : ''), onclick: () => { sel.has(k) ? sel.delete(k) : sel.add(k); render(); } }, label, h('span', { class: 'n' }, n));
    }));
    const words = q.value.toLowerCase().split(/\s+/).filter(Boolean);
    const list = evs.filter(e => (!sel.size || sel.has(e.kind)) && words.every(w => (e.text + ' ' + (e.name || '')).toLowerCase().includes(w)));
    swap(tbody, list.slice(0, 1000).map(e => h('tr', null,
      h('td', { class: 'nowrap muted', title: new Date(e.time).toLocaleString() }, fmtTime(e.time, !!daySel.value)),
      h('td', null, h('span', { class: 'tag ' + ({ crash: 'red', failed: 'red', error: 'red', update: 'amber', updated: 'green', start: 'green', restart: 'blue', steamcmd: 'blue', addons: 'violet' }[e.kind] || '') }, kinds[e.kind] || e.kind)),
      h('td', { class: 'nowrap ev-server', title: e.name || '' }, e.machine && S.machines ? h('span', { class: 'mtag' }, icon('server'), machineName(e.machine)) : null,
        e.server && serverBy(e.server) ? h('a', { href: '#/server/' + encodeURIComponent(e.server) }, e.name) : (e.name || h('span', { class: 'muted' }, 'Manager'))),
      h('td', { class: 'ev-text', style: 'word-break:break-word' }, e.text, reportLink(e)))));
    if (!list.length) swap(tbody, h('tr', null, h('td', { colspan: '4', class: 'muted ev-empty', style: 'text-align:center;padding:30px' }, 'Nothing.')));
  }

  daySel.addEventListener('change', load);
  srvSel.addEventListener('change', load);
  q.addEventListener('input', debounce(render, 150));
  load();
  return {
    el: h('div', null,
      h('div', { class: 'pagehead' }, h('div', { class: 'grow' }, h('h1', null, 'Activity'), h('div', { class: 'sub' }, 'Starts, crashes, updates, restarts and changes: kept ' + 'in a file per day.'))),
      h('div', { class: 'toolbar' }, daySel, srvSel, h('div', { class: 'search' }, icon('search'), q)),
      h('div', { class: 'toolbar' }, chips),
      h('div', { class: 'panel', style: 'overflow:auto' }, h('table', { class: 'list evlist' },
        h('thead', null, h('tr', null, h('th', null, 'Time'), h('th', null, 'What'), h('th', null, 'Server'), h('th', null, 'Details'))), tbody))),
    onEvent: ev => { if (live && (!srvSel.value || ev.server === srvSel.value)) { evs.unshift(ev); render(); } },
  };
}

/* =================================================================================================================
 * Manager settings
 * =============================================================================================================== */

// machinesBox: Settings, Machines. This manager as a hub (the machines linked to it, adding one) or as a node
// (linked to a hub).
function machinesBox() {
  const box = h('div', null, h('div', { class: 'muted small' }, 'Loading…'));
  // the hub's address for the codes: the one used last, else this page's, unless it is this machine (localhost),
  // which the other machine can not reach
  const pageURL = location.origin + location.pathname.replace(/index\.html$/, '');
  const isLoopback = u => { try { const host = new URL(u).hostname.replace(/^\[|\]$/g, ''); return host === 'localhost' || host.endsWith('.localhost') || /^127\./.test(host) || host === '::1' || host === '0.0.0.0'; } catch { return false; } };
  const hubURL = () => store.get('hubAddress', '') || (isLoopback(pageURL) ? '' : pageURL);
  let shownCode = null; // { id, code }
  async function load() {
    let mi;
    try { mi = await api('GET', 'api/machines'); } catch (e) { swap(box, h('div', { class: 'note bad' }, e.message)); return; }
    const nameInp = h('input', { type: 'text', value: mi.name, maxlength: 40, style: 'max-width:260px', 'aria-label': 'This machine\'s name' });
    const nameRow = field('This machine\'s name', h('div', { class: 'inputgroup', style: 'max-width:380px' }, nameInp,
      h('button', { class: 'btn', type: 'button', onclick: async () => { if (await run(() => api('PUT', 'api/machines/name', { name: nameInp.value }), 'Name saved.')) load(); } }, icon('check'), 'Save')),
    'How the other machines and the hub\'s page call it: US, EU 1…');

    if (mi.hub) {
      const st = mi.hub.status || {};
      swap(box, h('div', { class: 'col', style: 'gap:14px' }, nameRow,
        h('div', { class: 'dest' },
          h('div', { class: 'dest-hd' }, icon('server'), h('b', null, 'Linked to the hub ' + mi.hub.name), h('span', { class: 'spacer' }),
            h('button', { class: 'btn sm ghost', type: 'button', onclick: async () => {
              if (!await confirmBox('Unlink from ' + mi.hub.name + '?', 'Its page stops showing this machine\'s servers. This manager and its servers go on as before. To link it again, the hub makes a new code.', 'Unlink', true)) return;
              if (await run(() => api('DELETE', 'api/machines/hub'), 'Unlinked.')) load();
            } }, icon('x'), 'Unlink')),
          h('div', { class: 'mono small muted', style: 'overflow-wrap:anywhere' }, mi.hub.url),
          st.connected
            ? h('div', { class: 'note ok small', style: 'margin-top:8px' }, icon('check'), ' Connected' + (st.since ? ' since ' + new Date(st.since).toLocaleString() : '') + '. Its servers show on the hub\'s page.')
            : h('div', { class: 'note warn small', style: 'margin-top:8px' }, icon('alert'), ' Not connected' + (st.error ? ': ' + st.error : '') + '. It tries again by itself.'))));
      return;
    }

    const urlInp = h('input', { type: 'text', value: hubURL(), placeholder: 'https://your-domain/', spellcheck: false, 'aria-label': 'This hub\'s address' });
    const nodes = (mi.nodes || []).filter(n => !n.local);
    const list = nodes.length ? nodes.map(n => {
      const until = (mi.pairing || {})[n.id];
      const status = !n.linked
        ? h('span', { class: 'tag amber' }, 'Waiting to join' + (until ? (until * 1000 > Date.now() ? ' · code good until ' + new Date(until * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : ' · code expired') : ''))
        : n.online ? h('span', { class: 'tag green' }, 'Connected') : h('span', { class: 'tag red' }, 'Offline' + (n.since ? ' · ' + timeAgo(n.since) : ''));
      const codeBox = shownCode && shownCode.id === n.id ? codeView(shownCode.code) : null;
      return h('div', { class: 'dest' },
        h('div', { class: 'dest-hd' }, h('span', { class: 'dot ' + (n.online ? 'ok' : 'bad') }), h('b', null, n.name), status,
          n.version ? verTag(n.version) : null, h('span', { class: 'spacer' }),
          h('button', { class: 'btn sm ghost', type: 'button', title: n.linked ? 'A new code, to link it again (its current link stops)' : 'A new code (the last one expired, or was lost)', onclick: async () => {
            if (n.linked && !await confirmBox('A new code for ' + n.name + '?', 'Its current link stops working: on ' + n.name + ', link again with the new code (Settings, Machines).', 'New code', true)) return;
            const r = await run(() => api('POST', 'api/machines/' + encodeURIComponent(n.id) + '/code', { url: urlInp.value }));
            if (r) { store.set('hubAddress', urlInp.value.trim()); shownCode = { id: n.id, code: r.code }; load(); }
          } }, icon('key'), 'New code'),
          h('button', { class: 'btn sm ghost', type: 'button', onclick: async () => {
            if (!await confirmBox('Remove ' + n.name + '?', 'This page stops showing its servers, and its link stops working. Its manager and servers go on by themselves.', 'Remove', true)) return;
            if (await run(() => api('DELETE', 'api/machines/' + encodeURIComponent(n.id)), n.name + ' removed.')) load();
          } }, icon('trash'), 'Remove')),
        n.linked ? h('div', { class: 'small muted' }, n.servers + ' server' + (n.servers === 1 ? '' : 's')) : null,
        codeBox);
    }) : h('div', { class: 'muted small' }, 'No machine linked yet.');

    const newName = h('input', { type: 'text', placeholder: 'US East, EU…', maxlength: 40, style: 'max-width:220px', 'aria-label': 'Name of the machine' });
    const addBtn = h('button', { class: 'btn primary', type: 'button', onclick: async () => {
      const r = await run(() => api('POST', 'api/machines', { name: newName.value, url: urlInp.value }));
      if (r) { store.set('hubAddress', urlInp.value.trim()); shownCode = { id: r.id, code: r.code }; load(); }
    } }, icon('plus'), 'Add a machine');
    const joinInp = h('input', { type: 'text', placeholder: 'SZM-…', spellcheck: false, 'aria-label': 'Link code' });
    const urlField = field('This hub\'s address, as the other machines reach it', urlInp,
      isLoopback(pageURL) ? h('span', null, h('b', null, 'Not localhost'), ': this page is opened on the hub itself, the other machines reach it at its address on the internet, https://your-domain/. Codes (Add a machine, New code) carry it.')
        : 'With HTTPS on the internet (your domain, behind IIS: Settings, General). http:// works only on a private network. Codes (Add a machine, New code) carry it.', true);
    swap(box, h('div', { class: 'col', style: 'gap:14px' }, nameRow,
      nodes.length ? h('div', { class: 'formgrid' }, urlField) : null, // New code uses it too
      h('div', { class: 'col', style: 'gap:10px' }, list),
      h('div', { class: 'dest' },
        h('div', { class: 'dest-hd' }, icon('plus'), h('b', null, 'Add a machine')),
        h('div', { class: 'formgrid' },
          field('Its name', newName, 'The other machine: US East, EU…'),
          nodes.length ? null : urlField,
          h('div', { class: 'wide' }, addBtn))),
      nodes.length ? null : h('details', { class: 'dest' },
        h('summary', null, h('b', null, 'Or link this manager to a hub'), h('span', { class: 'muted small' }, ' (the other machine is the hub)')),
        h('div', { class: 'inputgroup', style: 'margin-top:10px' }, joinInp,
          h('button', { class: 'btn primary', type: 'button', onclick: async () => {
            const r = await run(() => api('POST', 'api/machines/join', { code: joinInp.value }));
            if (r) { toast('Linked to ' + r.hub + '.', 'ok'); load(); }
          } }, icon('check'), 'Link')),
        h('div', { class: 'hint', style: 'margin-top:6px' }, 'The code comes from the hub: Settings, Machines, Add a machine.'))));
  }
  function codeView(code) {
    return h('div', { class: 'note info', style: 'margin-top:10px' },
      h('div', null, h('b', null, 'On that machine'), ': open its manager, Settings, Machines, ', h('b', null, 'Or link this manager to a hub'), ', paste this code and click Link. It works once, for 30 minutes.'),
      h('div', { class: 'inputgroup', style: 'margin-top:8px' }, h('input', { type: 'text', readonly: true, value: code, class: 'mono', onfocus: e => e.target.select() }),
        h('button', { class: 'btn', type: 'button', onclick: () => copyText(code) }, icon('copy'), 'Copy')));
  }
  load();
  return box;
}

function pageSettings(start) {
  let spy = null;
  let byHand = null;
  const wrap = h('div', null, h('div', { class: 'muted' }, 'Loading…'));
  const errs = h('div');
  const saveBtn = h('button', { class: 'btn primary', disabled: true }, icon('save'), 'Save settings');
  let st = null;
  let dirty = false;
  const changed = () => { dirty = true; saveBtn.disabled = false; };

  api('GET', 'api/settings').then(r => {
    st = r.settings;
    bindText.onChange = changed;
    const num = (obj, key, attrs) => bindText(obj, key, Object.assign({ type: 'number' }, attrs || {}));
    // minutes shown, seconds kept
    const minutes = (obj, key) => {
      const inp = h('input', { type: 'number', min: 1, value: Math.round(obj[key] / 60) });
      inp.addEventListener('input', () => { obj[key] = Math.max(1, Number(inp.value) || 1) * 60; changed(); });
      return inp;
    };
    // Discord channels and XenForo forums: as many as wanted, each with what it gets and for which servers
    const kindLabels = { crash: 'Crashes', failed: 'Gave up', update: 'Update found', updated: 'Updated', restart: 'Restarts', start: 'Started (ready)', stop: 'Stopped', error: 'Errors', info: 'Info' };
    const PRESETS = [['Public', ['update', 'updated', 'start']], ['Admins', r.kinds.slice()]];
    if (!st.discords) st.discords = [];
    if (!st.forums) st.forums = [];
    // the servers a destination can post for: on a hub, every machine's (it posts their events); else the machine's
    const postServers = () => S.ctxMachine ? machineServers() : S.servers;
    // linked to a hub: it posts this machine's events
    const hubPosts = r.linked_hub ? h('div', { class: 'note info small', style: 'margin-bottom:12px' }, icon('info'), ' ',
      h('b', null, 'Linked to the hub ' + r.linked_hub + ':'), ' it posts this machine\'s events with its own channels and forums. Leave this empty, or its posts come twice.') : null;
    // what a destination gets: its kinds of events, with presets, and its servers
    function pickers(o) {
      const kindEl = h('div', { class: 'chips' });
      const srvEl = h('div', { class: 'chips' });
      const draw = () => {
        const kinds = new Set(o.events || []);
        swap(kindEl, r.kinds.map(k => h('button', {
          type: 'button', class: 'chip' + (kinds.has(k) ? ' on' : ''),
          onclick: () => { kinds.has(k) ? kinds.delete(k) : kinds.add(k); o.events = r.kinds.filter(x => kinds.has(x)); draw(); changed(); },
        }, kindLabels[k] || k)));
        const sel = new Set(o.servers || []);
        swap(srvEl, h('button', { type: 'button', class: 'chip' + (!sel.size ? ' on' : ''), title: 'Every server, and the manager\'s own events',
          onclick: () => { o.servers = []; draw(); changed(); } }, 'All servers'),
        postServers().map(v => h('button', { type: 'button', class: 'chip' + (sel.has(v.id) ? ' on' : ''),
          onclick: () => { sel.has(v.id) ? sel.delete(v.id) : sel.add(v.id); o.servers = postServers().map(x => x.id).filter(x => sel.has(x)); draw(); changed(); } },
        S.machines && !S.ctxMachine ? machineName(v.machine || '') + ': ' + v.name : v.name)));
      };
      draw();
      const presets = PRESETS.map(([label, list]) => h('button', { type: 'button', class: 'btn sm ghost', title: label === 'Public' ? 'Update found, updated, started: what players care about' : 'Everything',
        onclick: () => { o.events = list.slice(); draw(); changed(); } }, label));
      return [h('div', { class: 'wide' }, h('div', { class: 'row small', style: 'font-weight:600;color:var(--text-2);margin-bottom:8px;gap:6px' }, 'What is posted',
        h('span', { class: 'spacer' }), h('span', { class: 'muted', style: 'font-weight:400' }, 'Presets:'), presets), kindEl),
        h('div', { class: 'wide' }, h('div', { class: 'small', style: 'font-weight:600;color:var(--text-2);margin-bottom:8px' }, 'For'), srvEl)];
    }
    const removeBtn = (list, k, what, redraw) => h('button', { class: 'btn sm ghost', type: 'button', onclick: async () => {
      if (!await confirmBox('Remove ' + what + '?', 'Nothing more is posted there once you save.', 'Remove', true)) return;
      list.splice(k, 1); redraw(); changed();
    } }, icon('trash'), 'Remove');
    const discordList = h('div', { class: 'col', style: 'gap:12px' });
    function drawDiscords() {
      swap(discordList, st.discords.length ? st.discords.map((d, k) => {
        const hook = secret(d, 'webhook', { placeholder: 'https://discord.com/api/webhooks/…' });
        return h('div', { class: 'dest' },
          h('div', { class: 'dest-hd' }, icon('chat'), bindText(d, 'name', { placeholder: 'Public, Admins…', class: 'dest-name', 'aria-label': 'Name' }), h('span', { class: 'spacer' }),
            removeBtn(st.discords, k, d.name || 'this channel', drawDiscords)),
          h('div', { class: 'formgrid' },
            field('Webhook URL', h('div', { class: 'inputgroup' }, hook[0], hook[1],
              h('button', { class: 'btn', type: 'button', onclick: () => run(() => api('POST', 'api/discord/test', { webhook: d.webhook, username: d.username, name: d.name }), 'Test message sent to ' + (d.name || 'Discord') + '.') }, icon('chat'), 'Test')), null, true),
            field('Name of the poster', bindText(d, 'username', { placeholder: 'The webhook\'s own name' })),
            pickers(d)));
      }) : h('div', { class: 'muted small' }, 'No channel yet.'));
    }
    drawDiscords();
    const forumList = h('div', { class: 'col', style: 'gap:12px' });
    function drawForums() {
      swap(forumList, st.forums.length ? st.forums.map((xf, k) => {
        const xfKey = secret(xf, 'key', { placeholder: 'From Admin CP > Setup > API keys' });
        const idField = (key, attrs) => bindText(xf, key, Object.assign({ type: 'number', min: 0, placeholder: '0' }, attrs || {}));
        const xfCheck = h('div');
        return h('div', { class: 'dest' },
          h('div', { class: 'dest-hd' }, icon('list'), bindText(xf, 'name', { placeholder: 'Server status, Staff…', class: 'dest-name', 'aria-label': 'Name' }), h('span', { class: 'spacer' }),
            removeBtn(st.forums, k, xf.name || 'this forum', drawForums)),
          h('div', { class: 'formgrid' },
            field('Forum address', bindText(xf, 'url', { placeholder: 'https://snipezilla.com' }), 'Where XenForo is: its API is this address + /api/.'),
            field('API key', h('div', { class: 'inputgroup' }, xfKey[0], xfKey[1])),
            field('Post as user ID', idField('user'), 'Only for a super user key: who the posts are from. 0 for a user key.'),
            field('Thread ID', idField('thread'), 'Posts go as replies of this thread (the number in its address: /threads/server-status.123/ is 123).'),
            field('Or forum ID', idField('node'), 'Without a thread ID: each post a new thread of this forum.'),
            field('Thread prefix ID', idField('prefix'), 'For those new threads. 0 for none.'),
            field('Game icons address', bindText(xf, 'icons', { placeholder: 'Empty: attached to each post' }),
              'Empty: each post carries its game icon as an attachment (the key needs the scope attachment:write). Or the web address of a folder with the icons (cs2.png, css.png… the manager\'s icons): shown from there, nothing attached.', true),
            pickers(xf),
            h('div', { class: 'wide row wrap' }, h('button', { class: 'btn', type: 'button', onclick: async () => {
              swap(xfCheck, h('span', { class: 'muted small' }, 'Checking…'));
              try {
                const res = await api('POST', 'api/xenforo/check', { url: xf.url, key: xf.key, user: Number(xf.user) || 0, thread: Number(xf.thread) || 0, node: Number(xf.node) || 0, icons: xf.icons || '' });
                swap(xfCheck, h('div', { class: 'note ok' }, res.message));
              } catch (e) { swap(xfCheck, h('div', { class: 'note bad' }, e.message)); }
            } }, icon('check'), 'Check (posts nothing)'), xfCheck)));
      }) : h('div', { class: 'muted small' }, 'No forum yet.'));
    }
    drawForums();
    const addDiscord = () => {
      const first = !st.discords.length;
      st.discords.push({ name: first ? 'Public' : 'Admins', webhook: '', username: 'SnipeZilla Server Manager', events: first ? ['update', 'updated', 'start'] : r.kinds.slice(), servers: [] });
      drawDiscords(); changed();
    };
    const addForum = () => {
      st.forums.push({ name: st.forums.length ? 'Forum ' + (st.forums.length + 1) : 'Server status', url: st.forums.length ? st.forums[0].url : '', key: '', user: 0, thread: 0, node: 0, prefix: 0, icons: st.forums.length ? st.forums[0].icons || '' : '', events: ['crash', 'failed', 'updated'], servers: [] });
      drawForums(); changed();
    };
    const exclude = h('textarea', { rows: 5, value: (st.copy_exclude || []).join('\n'), spellcheck: false, oninput: e => { st.copy_exclude = e.target.value.split('\n').map(x => x.trim()).filter(Boolean); changed(); } });
    const restartSeg = h('div', { class: 'seg' });
    const timeInp = bindText(st.restart, 'time', { placeholder: '06:00', style: 'width:120px' });
    const renderSeg = () => swap(restartSeg, [['daily', 'Daily'], ['auto', 'After 24 h'], ['never', 'Never']].map(([k, l]) =>
      h('button', { type: 'button', class: st.restart.mode === k ? 'on' : '', onclick: () => { st.restart.mode = k; renderSeg(); changed(); timeInp.disabled = k !== 'daily'; } }, l)));
    renderSeg();
    timeInp.disabled = st.restart.mode !== 'daily';
    const importPath = h('input', { type: 'text', placeholder: 'C:\\SnipeZilla Srcds Manager\\config.xml', spellcheck: false });
    const m = MI();

    // Backups: by hand, or daily, weekly, monthly at a time
    const bk = st.backups;
    const bkSeg = h('div', { class: 'seg' });
    const bkWhen = h('div', { class: 'row bkwhen' });
    const bkSummary = h('div', { class: 'small muted bksummary', style: 'margin-top:8px' });
    const bkTime = bindText(bk, 'time', { placeholder: '05:30', style: 'width:100px', 'aria-label': 'Time' });
    const bkWeekday = bindSelect(bk, 'weekday', WEEKDAYS.map((d, i) => [i, d]), () => drawBk(true));
    bkWeekday.setAttribute('aria-label', 'Day of the week');
    const bkDay = bindText(bk, 'day', { type: 'number', min: 1, max: 31, style: 'width:80px', 'aria-label': 'Day of the month' });
    bkTime.addEventListener('input', () => drawBk(true));
    bkDay.addEventListener('input', () => drawBk(true));
    const lbl = t => h('span', { class: 'muted small' }, t);
    function drawBk(textOnly) {
      if (!textOnly) {
        swap(bkSeg, [['manual', 'By hand'], ['daily', 'Daily'], ['weekly', 'Weekly'], ['monthly', 'Monthly']].map(([k, l]) =>
          h('button', { type: 'button', class: bk.schedule === k ? 'on' : '', onclick: () => { bk.schedule = k; drawBk(); changed(); } }, l)));
        swap(bkWhen, bk.schedule === 'manual' ? null : [
          bk.schedule === 'weekly' ? [lbl('on'), bkWeekday] : null,
          bk.schedule === 'monthly' ? [lbl('on day'), bkDay] : null,
          lbl('at'), bkTime]);
      }
      swap(bkSummary, backupWhen(bk) + '.');
    }
    drawBk();

    // Off-site copies: each new backup also goes to Google Drive, OneDrive… (with rclone) or to a folder
    if (!bk.offsite) bk.offsite = [];
    let rc = null; // rclone: its program, version, remotes
    const offList = h('div', { class: 'col', style: 'gap:12px' });
    const rcBox = h('div');
    const isWin = (m.os || '').startsWith('windows') || /^[A-Za-z]:\\/.test(r.data_dir || '');
    const q = p => /\s/.test(p) ? '"' + p + '"' : p;
    function drawOffsite() {
      swap(offList, bk.offsite.length ? bk.offsite.map((o, k) => {
        const result = h('div');
        const remotes = o.kind === 'rclone' && rc && rc.remotes && rc.remotes.length
          ? h('div', { class: 'chips', style: 'margin-top:6px' }, h('span', { class: 'muted small', style: 'align-self:center' }, 'Remotes:'),
            rc.remotes.map(x => h('button', { type: 'button', class: 'chip' + ((o.target || '').startsWith(x) ? ' on' : ''),
              title: 'Use ' + x, onclick: () => { o.target = x + 'SZM backups'; drawOffsite(); changed(); } }, x)))
          : null;
        const where = o.kind === 'rclone'
          ? field('Remote and folder', h('div', null, bindText(o, 'target', { placeholder: 'gdrive:SZM backups' }), remotes),
            'An rclone remote (set up below) and a folder in it. Each server gets its own folder inside.', true)
          : field('Folder', folderPicker(o, 'target', isWin ? 'G:\\My Drive\\SZM backups' : '/mnt/backup/szm'),
            'A folder the Google Drive or OneDrive app syncs, another disk, or a network share (\\\\nas\\backups). Each server gets its own folder inside.', true);
        return h('div', { class: 'dest' },
          h('div', { class: 'dest-hd' }, icon(o.kind === 'rclone' ? 'cloud' : 'folder'),
            bindText(o, 'name', { placeholder: 'Google Drive, OneDrive…', class: 'dest-name', 'aria-label': 'Name' }), h('span', { class: 'spacer' }),
            h('button', { class: 'btn sm ghost', type: 'button', onclick: async () => {
              if (!await confirmBox('Remove ' + (o.name || 'this destination') + '?', 'No more backups are copied there once you save. The copies already there stay.', 'Remove', true)) return;
              bk.offsite.splice(k, 1); drawOffsite(); changed();
            } }, icon('trash'), 'Remove')),
          h('div', { class: 'formgrid' },
            field('Kind', bindSelect(o, 'kind', [['rclone', 'Cloud, with rclone'], ['folder', 'A folder']], () => { o.target = ''; drawOffsite(); })),
            where,
            h('div', { class: 'wide row wrap' }, h('button', { class: 'btn', type: 'button', onclick: async () => {
              swap(result, h('span', { class: 'muted small' }, 'Testing…'));
              try {
                const res = await api('POST', 'api/offsite/test', { kind: o.kind, target: o.target || '' });
                swap(result, h('div', { class: 'note ok' }, res.message));
              } catch (e) { swap(result, h('div', { class: 'note bad' }, e.message)); }
            } }, icon('check'), 'Test'), result)));
      }) : h('div', { class: 'muted small' }, 'No off-site copy yet: the backups are only on this machine.'));
      drawRClone();
    }
    function drawRClone() {
      if (!bk.offsite.some(o => o.kind === 'rclone')) { swap(rcBox, null); return; }
      if (!rc) { swap(rcBox, h('div', { class: 'muted small', style: 'margin-top:12px' }, 'Looking for rclone…')); return; }
      const dl = h('button', { class: 'btn' + (rc.path ? '' : ' primary'), type: 'button', onclick: async () => {
        dl.disabled = true; swap(dl, icon('download'), 'Downloading…');
        try { rc = await api('POST', 'api/rclone'); toast('rclone is ready.', 'ok'); } catch (e) { toast(e.message, 'bad'); }
        drawOffsite();
      } }, icon('download'), rc.path ? 'Download again' : 'Download rclone');
      const cmd = q(rc.path || rc.local) + ' config --config ' + q(rc.config);
      const status = rc.path && rc.version
        ? h('div', { class: 'note ok small' }, icon('check'), ' ', h('b', null, rc.version), ' · ', rc.remotes.length ? 'remotes: ' + rc.remotes.join(' ') : 'no remote yet', h('div', { class: 'mono muted', style: 'overflow-wrap:anywhere' }, rc.path))
        : h('div', { class: 'note warn small' }, icon('alert'), ' ', rc.path ? 'rclone does not run: ' + (rc.error || '?') : 'rclone is not here yet. It copies to Google Drive, OneDrive, Dropbox, SFTP, S3 and many more.');
      swap(rcBox, h('div', { class: 'dest', style: 'margin-top:12px' },
        h('div', { class: 'dest-hd' }, icon('cloud'), h('b', null, 'rclone'), h('span', { class: 'spacer' }), dl),
        status,
        h('ol', { class: 'small rcsteps' },
          h('li', null, 'On the server (Remote Desktop), open a command prompt and run:',
            h('div', { class: 'inputgroup', style: 'margin:4px 0' }, h('input', { type: 'text', readonly: true, value: cmd, class: 'mono', onfocus: e => e.target.select() }),
              h('button', { class: 'btn', type: 'button', title: 'Copy', onclick: () => copyText(cmd) }, icon('copy'), 'Copy'))),
          h('li', null, h('b', null, 'n'), ' (new remote), a name: ', h('b', null, 'gdrive'), ' or ', h('b', null, 'onedrive'), ', then the storage: ', h('b', null, 'drive'), ' (Google Drive) or ', h('b', null, 'onedrive'), '.'),
          h('li', null, 'Press Enter for the defaults, ', h('b', null, 'y'), ' to sign in with the browser that opens, then ', h('b', null, 'q'), ' to quit. Done once: rclone keeps the access.'),
          h('li', null, 'Back here, ', h('a', { href: '#', onclick: e => { e.preventDefault(); loadRClone(); } }, 'refresh'), ', click the remote above, Test and save.')),
        h('details', { style: 'margin-top:10px' }, h('summary', { class: 'small muted' }, 'rclone\'s program'),
          field('rclone' + (isWin ? '.exe' : ''), bindText(bk, 'rclone', { placeholder: rc.local }), 'Empty: the one downloaded here, else rclone in the PATH.'))));
    }
    async function loadRClone() {
      try { rc = await api('GET', 'api/rclone'); } catch (e) { rc = { remotes: [], error: e.message, local: '', config: '' }; }
      drawOffsite();
    }
    const addOffsite = kind => {
      bk.offsite.push({ name: kind === 'rclone' ? (bk.offsite.some(o => o.kind === 'rclone') ? 'OneDrive' : 'Google Drive') : 'Folder', kind, target: '' });
      drawOffsite(); changed();
      if (kind === 'rclone' && !rc) loadRClone();
    };
    drawOffsite();
    if (bk.offsite.some(o => o.kind === 'rclone')) loadRClone();

    // a hub with no servers of its own: only what it does (its page, posts, machines); the servers' settings are
    // the machines' own (Machine bar)
    const lean = !S.ctxMachine && hubOnly(machineBy(''));
    const NAV = lean ? [
      ['', [['general', 'General', 'sliders'], ['logs', 'Activity log', 'filetext']]],
      ['Posts', [['discord', 'Discord', 'chat'], ['xenforo', 'XenForo', 'globe']]],
      can('a') ? ['Other', [['machines', 'Machines', 'server']]] : null,
    ].filter(Boolean) : [
      ['', [['general', 'General', 'sliders']]],
      ['Servers', [['crash', 'Crash watch', 'shield'], ['restarts', 'Scheduled restarts', 'restart'], ['updates', 'Updates', 'update'], ['steamcmd', 'steamcmd', 'download']]],
      ['Files', [['backups', 'Backups', 'save'], ['offsite', 'Off-site copies', 'cloud'], ['addons', 'Addons', 'package'], ['logs', 'Logs', 'filetext']]],
      ['Posts', [['discord', 'Discord', 'chat'], ['xenforo', 'XenForo', 'globe']]],
      can('a') && !S.ctxMachine ? ['Other', [['machines', 'Machines', 'server'], ['import', 'Import from 1.x', 'download']]] : null,
    ].filter(Boolean);
    const titles = {};
    NAV.forEach(([, items]) => items.forEach(([k, l, ic]) => { titles[k] = [l, ic]; }));
    const card = (k, hint, ...body) => !titles[k] ? null : h('section', { class: 'panel setcard', id: 'set-' + k, 'data-k': k },
      h('div', { class: 'hd' }, h('span', { class: 'seticon' }, icon(titles[k][1])), h('div', { class: 'grow' }, h('h2', null, titles[k][0]), hint ? h('div', { class: 'hint' }, hint) : null)),
      h('div', { class: 'bd' }, body));
    const lines = (obj, key, rows) => h('textarea', { rows: rows || 5, value: (obj[key] || []).join('\n'), spellcheck: false, oninput: e => { obj[key] = e.target.value.split('\n').map(x => x.trim()).filter(Boolean); changed(); } });

    const cards = [
      card('general', h('span', null, 'Data folder: ', h('span', { class: 'mono' }, r.data_dir)),
        h('div', { class: 'formgrid' },
          lean ? null : h('div', { class: 'span3' }, field('Folder for new servers', folderPicker(st, 'servers_root'), 'Where the New server page proposes to install.')),
          lean ? null : field('Default priority', bindSelect(st, 'priority', PRIORITIES.map(p => [p, p[0].toUpperCase() + p.slice(1)]))),
          field('Web interface address', bindText(st, 'listen', { placeholder: ':8888', style: 'max-width:230px' }), h('span', null,
            h('b', null, ':8888'), ': reachable from other computers, at this machine\'s IP (http://its-ip:8888).', h('br'),
            h('b', null, '127.0.0.1:8888'), ': this machine only. Use it when a website shows the manager at your domain (with HTTPS).', h('br'),
            'Takes effect at the next start of the manager.'), true))),
      card('crash', 'A server is restarted when its program ends, or when it stops answering queries. Restarts wait 5 s, 15 s, 1 min, then longer; the last restart checks its files with steamcmd first. One more crash and the manager gives up (and says so).',
        h('div', { class: 'formgrid' },
          field('Query every (seconds)', num(st.monitor, 'query_interval', { min: 5 })),
          field('Unanswered queries before a restart', num(st.monitor, 'query_failures', { min: 1 })),
          field('Time to start (seconds)', num(st.monitor, 'start_grace', { min: 10 }), 'Not queried before. Big maps and workshop downloads need more.'),
          field('Stable after (seconds)', num(st.monitor, 'stable_after', { min: 10 }), 'Running this long clears the crash count.'),
          field('Restarts after crashes in a row', num(st.monitor, 'give_up_after', { min: 1 }), 'The last one checks the files first. 3: restart, restart, check the files and restart, then give up.'),
          (m.os || '').startsWith('windows') ? h('div', { class: 'wide' }, m.dumps_error
            ? h('div', { class: 'note warn small' }, icon('alert'), ' ', h('b', null, 'No crash dumps: '), 'Windows can not be set to write them, the manager is not an administrator (' + m.dumps_error + '). Run it as the service, or as an administrator: the crash reports then name the module and the plugins at fault.')
            : h('div', { class: 'note ok small' }, icon('check'), ' ', h('b', null, 'Crash dumps on: '), 'Windows saves a small dump of each server crash (crashes\\dumps in the data folder, the last 30 of each program) and shows no "has stopped working" window. The crash reports read them: the module it crashed in, and the plugins and extensions that were running.')) : null)),
      card('restarts', 'A fresh start every day keeps long-running servers healthy. A server can say otherwise in its settings.',
        h('div', { class: 'formgrid' },
          h('div', { class: 'wide row wrap' }, restartSeg, timeInp),
          h('div', { class: 'wide' }, bindCheck(st.restart, 'only_empty', 'Only when nobody plays', 'Daily: waits for the server to be empty, up to 12 hours after the time.')),
          field('Warn players for (seconds)', num(st.restart, 'warn_seconds', { min: 0 })))),
      card('updates', 'The manager asks Steam whether each game is up to date.',
        h('div', { class: 'formgrid' },
          h('div', { class: 'wide' }, bindCheck(st.updates, 'auto', 'Update by itself', 'When Steam has a new version, players are warned, the server is updated and started again. A server can say otherwise in its settings.')),
          field('Check every (minutes)', minutes(st.updates, 'check_interval')),
          field('Warn players for (seconds)', num(st.updates, 'warn_seconds', { min: 0 }), 'Not when the server is empty.'),
          field('Say it every (seconds)', num(st.updates, 'say_every', { min: 5 })))),
      card('steamcmd', 'Installs and updates the servers. Downloaded by itself the first time it is needed.',
        h('div', { class: 'formgrid' },
          field('steamcmd.exe', (() => {
            const inp = bindText(st, 'steamcmd', { placeholder: r.steamcmd });
            return h('div', { class: 'inputgroup' }, inp, h('button', { class: 'btn', type: 'button', title: 'Find steamcmd', 'aria-label': 'Find steamcmd', onclick: async () => {
              const p = await browseFolder(st.steamcmd || r.steamcmd, { files: 'program', title: 'steamcmd' });
              if (p) { inp.value = p; st.steamcmd = p; changed(); }
            } }, icon('folder')));
          })(), m.steamcmd ? 'Found. Empty: the one the manager downloads next to it.' : 'Not there yet. Empty: the manager downloads it next to it.'),
          field('Shared copies of the games', folderPicker(st, 'cache_dir', r.cache_dir), h('span', null, 'For servers that update from a shared copy. ', h('a', { href: '#/cache' }, 'See their files'))),
          h('div', { class: 'wide row' }, h('button', { class: 'btn', type: 'button', onclick: () => run(() => api('POST', 'api/steamcmd'), 'Getting steamcmd ready…') }, icon('download'), m.steamcmd ? 'Check steamcmd' : 'Download steamcmd now')),
          field('Files an update never overwrites', exclude, 'One per line: a name (server.cfg) or a path in the server\'s folder (csgo/cfg/autoexec.cfg). Wildcards work: server_*.cfg, mapcycle_*. A folder protects everything in it (csgo/addons). For shared-copy updates; a file the server does not have yet is still copied.', true))),
      card('backups', 'A zip of each server\'s settings and plugins, in its game folder. Back up or restore from the server\'s Backups tab.',
        h('div', { class: 'formgrid' },
          h('div', { class: 'wide' }, h('div', { class: 'row wrap', style: 'gap:12px' }, bkSeg, bkWhen), bkSummary),
          h('div', { class: 'wide' }, bindCheck(st.backups, 'before', 'Also before a change', 'Before an update, an addons push and a restore: a bad update can be undone.')),
          field('Keep', num(st.backups, 'keep', { min: 1, max: 1000 }), 'The newest of each server; the older ones are removed.'),
          h('div', { class: 'span3' }, field('Folder', folderPicker(st.backups, 'dir', r.backups_dir), 'Empty: the backups folder next to szm.exe. Another disk is safer.')),
          field('What is backed up', lines(st.backups, 'include', 6), 'In the game folder (cstrike, tf, game/csgo…), one per line: folders, files, wildcards (*.txt).'),
          field('Left out', lines(st.backups, 'exclude', 6), 'Big or useless files: logs, demos, maps. Names, paths or wildcards.'))),
      card('offsite', 'Each new backup is also copied elsewhere, and the same number is kept there: the backups survive a dead disk or a lost machine. Copies that fail are tried again every hour; the Backups page shows how they go.',
        offList,
        h('div', { class: 'row wrap', style: 'margin-top:12px;gap:8px' },
          h('button', { class: 'btn', type: 'button', onclick: () => addOffsite('rclone') }, icon('cloud'), 'Add Google Drive, OneDrive…'),
          h('button', { class: 'btn', type: 'button', onclick: () => addOffsite('folder') }, icon('folder'), 'Add a folder')),
        rcBox),
      card('addons', 'Plugin packages (metamod2, counterstrikesharp, sourcemod…), pushed into the servers that get them (Addons page).',
        h('div', { class: 'formgrid' },
          h('div', { class: 'span3' }, field('Packages folder', folderPicker(st, 'addons_dir', r.addons_dir), 'One folder per package inside. Created if it does not exist.')),
          field('Files a push never overwrites', lines(st, 'addons_keep', 6),
            'What a server already has: plugin configs, data, the plugins themselves. One per line, paths inside the game folder, wildcards work (addons/sourcemod/plugins, *.db). A listed folder the server has is left as it is: nothing is overwritten or added in it, so a plugin you removed does not come back (take addons/sourcemod/plugins off the list for a push to bring new plugins). A listed file, or a folder, the server does not have yet is copied.', true))),
      card('logs', lean ? 'The Activity page: the events of every machine.' : 'The manager\'s activity log, and the logs folders of the servers.',
        h('div', { class: 'formgrid' },
          field('Activity log: keep (days)', num(st.logs, 'keep_days', { min: 1 }), 'The Activity page and each server\'s Activity tab.'),
          lean ? null : h('div', { class: 'span3' }, field('Server logs: keep (days)', num(st.logs, 'server_days', { min: 0, max: 3650, style: 'max-width:230px' }), 'The files of every folder named logs in the servers (csgo/logs, addons/sourcemod/logs, addons/counterstrikesharp/logs…) older than this are deleted by themselves, every 6 hours. 0 keeps them all. In a logs folder, the Files tab shows the newest first and can select the old ones to delete them now.')))),
      card('discord', 'Post what happens to Discord channels, each with what it gets: a public channel the updates, a staff channel everything. In Discord: channel settings, Integrations, Webhooks, New webhook, Copy webhook URL.',
        hubPosts, discordList,
        h('div', { class: 'row', style: 'margin-top:12px' }, h('button', { class: 'btn', type: 'button', onclick: addDiscord }, icon('plus'), 'Add a Discord channel'))),
      card('xenforo', 'The same posts on your forums, through their REST API. In the Admin CP: Setup, API keys, Add API key (a user key, or a super user key with the user ID), with the scopes thread:read, thread:write, node:read and attachment:write (for the game icons). Posts go as replies of one thread (a "Server status" thread), or each one as a new thread of a forum.',
        hubPosts ? hubPosts.cloneNode(true) : null, forumList,
        h('div', { class: 'row', style: 'margin-top:12px' }, h('button', { class: 'btn', type: 'button', onclick: addForum }, icon('plus'), 'Add a forum'))),
      can('a') && !S.ctxMachine ? card('machines', 'One page for the servers of several machines: the managers of the other machines link to this one, the hub. Each manager goes on watching its own servers by itself; the hub shows them all, and its users manage them from here.',
        machinesBox()) : null,
      can('a') && !S.ctxMachine ? card('import', 'Brings the servers of ' + 'SnipeZilla Srcds Manager 1.x (with their passwords, tokens and options) and its settings. Stop the old manager first: both would restart the same servers. Servers it had stopped stay stopped; running ones are taken over.',
        h('div', { class: 'inputgroup', style: 'max-width:720px' }, importPath,
          h('button', { class: 'btn', type: 'button', title: 'Find config.xml', onclick: async () => {
            const p = await browseFolder(importPath.value.trim(), { files: '.xml', title: 'The old manager\'s config.xml', hint: 'In the folder of SnipeZilla Srcds Manager 1.x, next to its program.' });
            if (p) importPath.value = p;
          } }, icon('folder'), 'Browse'),
          h('button', { class: 'btn primary', type: 'button', onclick: async () => {
            if (!importPath.value.trim()) return;
            const rep = await run(() => api('POST', 'api/import', { path: importPath.value.trim() }));
            if (!rep) return;
            const body = h('div', { class: 'col' },
              h('div', null, h('b', null, (rep.Added || []).length + ' servers imported')),
              (rep.Added || []).length ? h('div', { class: 'small muted' }, rep.Added.join(', ')) : null,
              (rep.Skipped || []).length ? h('div', { class: 'note warn' }, 'Skipped: ', rep.Skipped.join('; ')) : null,
              (rep.Stopped || []).length ? h('div', { class: 'note info' }, 'Stopped in the old manager, kept stopped: ', rep.Stopped.join(', ')) : null);
            await confirmBox('Import done', body, 'See the servers');
            location.hash = '#/';
          } }, icon('download'), 'Import'))) : null,
    ].filter(Boolean);

    // the side list: jumps to a card, and shows the one being read
    const links = {};
    const nav = h('nav', { class: 'setnav', 'aria-label': 'Settings sections' }, NAV.map(([group, items]) => [
      group ? h('div', { class: 'setnav-group' }, group) : null,
      items.map(([k, l, ic]) => (links[k] = h('a', { href: '#/settings/' + k, onclick: e => { e.preventDefault(); jump(k, true); } }, icon(ic), l)))]));
    const mark = k => Object.entries(links).forEach(([x, a]) => a.classList.toggle('on', x === k));
    // the section clicked stays lit (the last ones can not scroll to the top), until the user scrolls by hand
    let chosen = null;
    function jump(k, smooth) {
      const el = document.getElementById('set-' + k);
      if (!el) return;
      history.replaceState(null, '', '#/settings/' + k);
      chosen = k;
      mark(k);
      const y = el.getBoundingClientRect().top + window.scrollY - 70;
      window.scrollTo({ top: Math.max(0, y), behavior: smooth ? 'smooth' : 'auto' });
    }
    spy = () => {
      if (chosen) return;
      let cur = cards[0].dataset.k;
      for (const c of cards) if (c.getBoundingClientRect().top < 140) cur = c.dataset.k;
      if (window.innerHeight + window.scrollY >= document.body.scrollHeight - 4) cur = cards[cards.length - 1].dataset.k;
      mark(cur);
    };
    window.addEventListener('scroll', spy, { passive: true });
    byHand = e => { if (e.type !== 'keydown' || /^(Arrow|Page|Home|End| )/.test(e.key)) chosen = null; };
    for (const t of ['wheel', 'touchmove', 'keydown']) window.addEventListener(t, byHand, { passive: true });
    swap(wrap, lean ? h('div', { class: 'note info', style: 'margin-bottom:16px' }, icon('info'), ' ',
        h('b', null, 'This hub runs no servers'), ': here are its own settings only (its page, the posts for every machine, the machines).', h('br'), 'Crash watch, restarts, updates, backups… are each machine\'s: pick it in the Machine bar above.') : null,
      h('div', { class: 'setlayout' }, nav, h('div', { class: 'setcards' }, cards)));
    if (start && titles[start]) setTimeout(() => jump(start), 0); else mark('general');
  }).catch(e => swap(wrap, h('div', { class: 'note bad' }, e.message)));

  saveBtn.onclick = async () => {
    saveBtn.disabled = true;
    clear(errs);
    try {
      const r = await api('PUT', 'api/settings', st);
      dirty = false;
      toast(r.restart_needed ? 'Saved. Restart the manager to use the new web address.' : 'Settings saved.', 'ok');
    } catch (e) {
      const list = e.data && e.data.errors ? e.data.errors : [e.message];
      swap(errs, h('div', { class: 'note bad', style: 'margin-bottom:14px' }, list.map(x => h('div', null, x))));
      toast('Not saved: ' + list.join(' '), 'bad');
      saveBtn.disabled = false;
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
  };
  function warnUnsaved(e) { if (dirty) { e.preventDefault(); e.returnValue = ''; } }
  window.addEventListener('beforeunload', warnUnsaved);
  return {
    el: h('div', { style: 'max-width:1240px' },
      h('div', { class: 'pagehead' }, h('div', { class: 'grow' }, h('h1', null, 'Settings'), h('div', { class: 'sub' }, 'How the manager runs every server, unless a server says otherwise.'))),
      errs, wrap,
      h('div', { class: 'row', style: 'position:sticky;bottom:0;padding:14px 0;background:linear-gradient(180deg,transparent,var(--bg) 40%);z-index:5' }, h('span', { class: 'spacer' }), saveBtn)),
    destroy: () => {
      window.removeEventListener('beforeunload', warnUnsaved);
      if (spy) window.removeEventListener('scroll', spy);
      if (byHand) for (const t of ['wheel', 'touchmove', 'keydown']) window.removeEventListener(t, byHand);
    },
  };
}

/* =================================================================================================================
 * Shared copies of the games: their files
 * =============================================================================================================== */

function pageCache() {
  const list = h('div', { class: 'cachelist' }, h('div', { class: 'muted small' }, 'Loading…'));
  const info = h('div', { class: 'sub' });
  const fb = fileBrowser('api/cache', 'cachepath', 'Shared copies');
  api('GET', 'api/cache').then(r => {
    swap(info, 'A copy of each game, downloaded once by steamcmd, copied into the servers that update from it (their Settings, Updates). In ', h('span', { class: 'mono' }, r.dir), '.');
    swap(list, r.copies.length ? r.copies.map(c => h('div', { class: 'panel cachecopy' + (c.exists ? '' : ' missing') },
      c.keys.length ? badge(c.keys[0], 'sm') : h('div', { class: 'badge sm', style: 'background:var(--panel-3)' }, '?'),
      h('div', { class: 'grow', style: 'min-width:0' },
        h('div', { class: 'ellipsis' }, h('b', null, c.games.length ? c.games.join(', ') : 'Not a game the manager knows'), ' ', h('span', { class: 'mono muted small' }, c.folder)),
        h('div', { class: 'muted small ellipsis' }, c.exists ? (c.servers.length ? 'Updates ' + c.servers.join(', ') : 'No server updates from it now') : 'Not downloaded yet: the first update of ' + c.servers.join(', ') + ' makes it')),
      c.exists ? h('button', { class: 'btn sm', onclick: () => { fb.go(c.folder); fb.el.scrollIntoView({ behavior: 'smooth', block: 'start' }); } }, icon('folder'), 'Open') : null))
      : h('div', { class: 'muted small' }, 'No shared copy yet. A server set to update from a shared copy (its Settings, Updates) makes the one of its game at its first update.'));
  }).catch(e => swap(list, h('div', { class: 'note bad' }, e.message)));
  return {
    el: h('div', { style: 'max-width:1100px' },
      h('div', { class: 'pagehead' }, h('div', { class: 'grow' }, h('h1', null, 'Shared copies'), info),
        h('a', { class: 'btn ghost', href: '#/settings/steamcmd' }, icon('sliders'), 'Settings')),
      list, h('div', { style: 'margin-top:18px' }, fb.el)),
  };
}

/* =================================================================================================================
 * Games: the ones the manager knows, and the ones added here (games.json next to szm.exe)
 * =============================================================================================================== */

const ENGINES = [['source2', 'Source 2'], ['source', 'Source'], ['goldsrc', 'GoldSrc (Half-Life engine)'], ['other', 'Other engines (Unity, Unreal…)']];

// gamePage: a game's web page: the one given, else its Steam store page
const gamePage = g => g.url || (g.appid ? 'https://store.steampowered.com/app/' + g.appid + '/' : '');
const pageLabel = u => { try { const host = new URL(u).hostname.replace(/^www\./, ''); return host === 'store.steampowered.com' ? 'Steam' : host; } catch (e) { return u; } };

function pageGames(openNew) {
  const list = h('div', null, h('div', { class: 'muted' }, 'Loading…'));
  const fileNote = h('div');
  let icons = [];

  async function load() {
    try {
      const r = await api('GET', 'api/games');
      icons = r.icons || [];
      swap(fileNote, r.file_error ? h('div', { class: 'note bad', style: 'margin-bottom:16px' }, h('b', null, 'games.json cannot be read: '), r.file_error,
        '. The games stay as they were until it is fixed; the page updates by itself once it is.')
        : h('div', { class: 'hint', style: 'margin:-6px 0 14px' }, 'Also from ', h('span', { class: 'mono' }, r.file || 'games.json'),
          ': edit it by hand (an entry with the key of a built-in game replaces it), the list updates by itself within seconds.'));
      swap(list, ENGINES.map(([eng, label]) => {
        const gs = r.games.filter(g => g.engine === eng);
        if (!gs.length) return null;
        return h('div', { class: 'panel', style: 'margin-bottom:18px' }, h('div', { class: 'hd' }, h('h2', null, label), h('span', { class: 'muted small' }, gs.length + (gs.length === 1 ? ' game' : ' games'))),
          h('table', { class: 'list' },
            h('thead', null, h('tr', null, h('th', null, 'Game'), h('th', { class: 'hide-m' }, 'Server app ID'), h('th', { class: 'hide-m' }, 'Folder'), h('th', { class: 'hide-m' }, ''), h('th', { class: 'hide-m' }, 'Web page'), h('th', { class: 'num' }, ''))),
            h('tbody', null, gs.map(g => h('tr', null,
              h('td', null, h('div', { class: 'row', style: 'gap:10px' }, badge(g.key, 'sm'), h('div', { style: 'min-width:0' }, h('b', null, g.name), h('div', { class: 'small muted' }, g.short + (g.servers ? ' · ' + g.servers + ' server' + (g.servers > 1 ? 's' : '') : ''),
                g.custom ? h('span', { class: 'show-m' }, ' · ' + (g.builtin ? 'changed' : 'added')) : null)))),
              h('td', { class: 'hide-m mono' }, g.server_appid),
              h('td', { class: 'hide-m mono' }, g.game),
              h('td', { class: 'hide-m' }, g.custom && g.builtin ? h('span', { class: 'tag orange' }, 'changed') : g.custom ? h('span', { class: 'tag blue' }, 'added') : h('span', { class: 'tag' }, 'built-in')),
              h('td', { class: 'hide-m nowrap' }, gamePage(g) ? h('a', { href: gamePage(g), target: '_blank', rel: 'noopener', title: gamePage(g), class: 'extlink' }, pageLabel(gamePage(g)), icon('external')) : h('span', { class: 'muted' }, '—')),
              h('td', { class: 'num nowrap' },
                h('button', { class: 'btn sm ghost', title: 'Edit', onclick: () => gameForm(g, icons, load) }, icon('edit'), h('span', { class: 'lbl' }, 'Edit')),
                g.custom ? h('button', { class: 'btn sm ghost', title: g.builtin ? 'Undo your changes' : 'Remove', onclick: async () => {
                  const what = g.builtin ? 'Undo your changes to ' + g.name + '?' : 'Remove ' + g.name + '?';
                  if (!await confirmBox(what, g.builtin ? 'It goes back to how it comes with the manager.' : 'It is removed from the list of games. Servers of it must be removed first.', g.builtin ? 'Undo' : 'Remove', !g.builtin)) return;
                  if (await run(() => api('DELETE', 'api/games/' + encodeURIComponent(g.key)), g.builtin ? 'Back to the built-in ' + g.name + '.' : g.name + ' removed.')) { applyState(await api('GET', 'api/state')); load(); }
                } }, icon(g.builtin ? 'restart' : 'trash'), h('span', { class: 'lbl' }, g.builtin ? 'Undo' : 'Remove')) : null))))));
      }));
      if (openNew) { openNew = false; gameForm(null, icons, load); }
    } catch (e) { swap(list, h('div', { class: 'note bad' }, e.message)); }
  }
  load();
  return {
    el: h('div', { style: 'max-width:1000px' },
      h('div', { class: 'pagehead' }, h('div', { class: 'grow' }, h('h1', null, 'Games'),
        h('div', { class: 'sub' }, 'The games servers can be made of. Add one that is missing: it needs a dedicated server on Steam (steamcmd).')),
        h('button', { class: 'btn primary', onclick: () => gameForm(null, icons, load) }, icon('plus'), 'Add a game')),
      fileNote, list),
    onEvent: ev => { if (ev.kind === 'manager' && /games\.json/.test(ev.text) || ev.kind === 'error' && /games\.json/.test(ev.text)) load(); },
  };
}

// gameForm: add a game, or change one
function gameForm(g0, icons, onSaved) {
  const isNew = !g0;
  const g = g0 ? JSON.parse(JSON.stringify(g0)) : { key: '', name: '', short: '', engine: 'source', server_appid: '', appid: '', game: '', default_map: '', default_maxplayers: 24, icon: '', url: '', login: false, exe: {} };
  const exeWin = (g.exe && g.exe.windows || []).join(', ');
  const exeLin = (g.exe && g.exe.linux || []).join(', ');
  const m = modal({ title: isNew ? 'Add a game' : 'Change ' + g.name, size: 'wide' });
  const err = h('div');
  const keyIn = h('input', { type: 'text', value: g.key, spellcheck: false, disabled: !isNew, placeholder: 'fof' });
  let keyTouched = !isNew;
  keyIn.addEventListener('input', () => { keyTouched = true; });
  const autoKey = () => { if (!keyTouched) keyIn.value = (g.short || g.name).toLowerCase().replace(/[^a-z0-9]/g, '').slice(0, 32); };
  const nameIn = bindText(g, 'name', { placeholder: 'Fistful of Frags' });
  const shortIn = bindText(g, 'short', { placeholder: 'FoF' });
  nameIn.addEventListener('input', autoKey);
  shortIn.addEventListener('input', autoKey);
  const winIn = h('input', { type: 'text', value: exeWin, spellcheck: false });
  const linIn = h('input', { type: 'text', value: exeLin, spellcheck: false });
  const engineSel = bindSelect(g, 'engine', ENGINES, () => { placeholders(); drawOther(); });
  const placeholders = () => {
    winIn.placeholder = { goldsrc: 'hlds.exe', source: 'srcds_win64.exe, srcds.exe', source2: 'game/bin/win64/cs2.exe', other: 'RustDedicated.exe' }[g.engine] + (g.engine === 'source2' || g.engine === 'other' ? ' (needed on Windows)' : ' (the usual one when empty)');
    linIn.placeholder = { goldsrc: 'hlds_run', source: 'srcds_run_64, srcds_run', source2: 'game/cs2.sh', other: 'RustDedicated' }[g.engine];
  };
  placeholders();
  // The games of other engines: their command line, ports, RCON and how they stop
  const quoteArgs = list => (list || []).map(a => /\s/.test(a) ? '"' + a + '"' : a).join(' ');
  const splitArgs = t => (t.match(/"[^"]*"|\S+/g) || []).map(a => a.replace(/^"|"$/g, ''));
  const ox = {
    args: quoteArgs(g.args), win: quoteArgs((g.args_os || {}).windows), lin: quoteArgs((g.args_os || {}).linux),
    stop: (g.stop || []).join(', '), env: Object.entries(g.env || {}).map(([k, v]) => k + '=' + v).join('\n'),
  };
  const oText = (key, attrs) => { const i = h('input', Object.assign({ type: 'text', value: ox[key] || '', spellcheck: false }, attrs || {})); i.addEventListener('input', () => { ox[key] = i.value; }); return i; };
  const oArea = (key, attrs) => { const i = h('textarea', Object.assign({ rows: 3, spellcheck: false }, attrs || {})); i.value = ox[key] || ''; i.addEventListener('input', () => { ox[key] = i.value; }); return i; };
  const otherBox = h('div', { class: 'formgrid wide' });
  const drawOther = () => {
    otherBox.classList.toggle('hidden', g.engine !== 'other');
    if (g.engine !== 'other' || otherBox.childElementCount) return;
    append(otherBox,
      field('Command line', oArea('args', { placeholder: '-batchmode +server.port {port} +server.hostname "{name}" -logfile "{logfile}"' }),
        'Filled in: {port} {queryport} {rconport} {ip} {name} {password} {rcon_password} {gslt} {maxplayers} {map} {logfile} {dir}. An option whose value is left empty is left out.', true),
      field('Command line on Windows', oArea('win', { rows: 2, placeholder: 'Only when it differs on Windows' }), null, true),
      field('Command line on Linux', oArea('lin', { rows: 2, placeholder: 'Only when it differs on Linux' }), null, true),
      field('Steam query', bindSelect(g, 'query', [['try', 'Asked, may not answer'], ['', 'Must answer (else frozen)'], ['none', 'Not asked']]), 'Its players and map, from the Steam query on its query port.'),
      field('Query port', bindText(g, 'query_offset', { type: 'number', min: -60000, max: 60000 }), 'Its port + this ({queryport}): 1 for port+1, 0 for the port itself.'),
      field('RCON', bindSelect(g, 'rcon', [['none', 'None'], ['source', 'Source RCON'], ['webrcon', 'Rust WebRCON'], ['telnet', 'Telnet (7 Days to Die)'], ['battleye', 'BattlEye RCon (DayZ, Arma)']]), 'How the manager sends it commands (to stop it, warn players).'),
      field('RCON port', bindText(g, 'rcon_offset', { type: 'number', min: -60000, max: 60000 }), 'Its port + this ({rconport}).'),
      field('Stop commands', oText('stop', { placeholder: 'saveworld, DoExit' }), 'Sent through RCON to save and end it. None (or no RCON): Ctrl+C, which most of them take as save and quit.', true),
      field('Say command', bindText(g, 'say', { placeholder: 'say {text}' }), 'Through RCON, to warn players before a restart. Empty: not warned.'),
      field('Seconds to save and end', bindText(g, 'stop_wait', { type: 'number', min: 0, max: 3600, placeholder: '120' }), 'Then it is ended by force.'),
      field('Console log', bindText(g, 'console_log', { placeholder: 'logs/server.log' }), 'In the server\'s folder: the Console tab reads it ({logfile} of its command line).'),
      field('Config file', bindText(g, 'config', { placeholder: 'serverconfig.xml' }), 'Shown on its Set up page. {os}: Windows or Linux.'),
      field('Ports it takes', bindText(g, 'ports', { type: 'number', min: 1, max: 100, placeholder: '1' }), 'From its port on (Valheim: 2): a next server gets other ones.'),
      field('First port', bindText(g, 'default_port', { type: 'number', min: 1, max: 65535, placeholder: '27015' })),
      field('Environment', oArea('env', { rows: 2, placeholder: 'LD_LIBRARY_PATH={dir}/linux64' }), 'One per line, NAME=value ({dir}: the server\'s folder).', true),
      field('Notes', bindText(g, 'notes', { placeholder: 'What to know when adding a server of it' }), null, true));
  };
  drawOther();
  const iconPrev = h('img', { class: 'iconprev', alt: '' });
  const showIcon = () => { iconPrev.style.visibility = g.icon ? 'visible' : 'hidden'; if (g.icon) iconPrev.src = 'games/' + g.icon + '.png?' + Date.now(); };
  const iconOpts = () => [['', 'None (letters)']].concat(icons.map(i => [i, i]));
  let iconSel = bindSelect(g, 'icon', iconOpts(), showIcon);
  const iconFile = h('input', { type: 'file', accept: 'image/png,.png', style: 'display:none' });
  iconFile.addEventListener('change', async () => {
    const f = iconFile.files[0];
    iconFile.value = '';
    if (!f) return;
    if (f.size > 1024 * 1024) { toast('This picture is too big (1 MB at most; 64×64 is plenty).', 'bad'); return; }
    const name = (f.name.replace(/\.png$/i, '').toLowerCase().replace(/[^a-z0-9_-]/g, '') || keyIn.value.trim().toLowerCase()).slice(0, 32);
    const data = await new Promise((ok, ko) => { const rd = new FileReader(); rd.onload = () => ok(String(rd.result).split(',')[1] || ''); rd.onerror = ko; rd.readAsDataURL(f); });
    const r = await run(() => api('POST', 'api/icons', { name, png: data }), 'Icon ' + name + ' added.');
    if (!r) return;
    icons = r.icons || icons;
    g.icon = r.name;
    const sel = bindSelect(g, 'icon', iconOpts(), showIcon);
    iconSel.replaceWith(sel);
    iconSel = sel;
    showIcon();
  });
  const iconBox = h('div', { class: 'iconpick' }, iconSel, iconPrev,
    h('button', { type: 'button', class: 'btn sm ghost', onclick: () => iconFile.click() }, icon('plus'), 'Upload a PNG'), iconFile);
  showIcon();
  const save = h('button', { class: 'btn primary', onclick: async () => {
    clear(err);
    const key = keyIn.value.trim().toLowerCase();
    const split = v => v.split(',').map(x => x.trim()).filter(Boolean);
    const body = Object.assign({}, g, {
      server_appid: Number(g.server_appid) || 0, appid: Number(g.appid) || 0, default_maxplayers: Number(g.default_maxplayers) || 0,
      exe: { windows: split(winIn.value), linux: split(linIn.value) },
    });
    if (g.engine === 'other') {
      const env = {};
      ox.env.split('\n').map(l => l.trim()).filter(Boolean).forEach(l => { const k = l.indexOf('='); if (k > 0) env[l.slice(0, k).trim()] = l.slice(k + 1).trim(); });
      const os = {};
      if (ox.win.trim()) os.windows = splitArgs(ox.win);
      if (ox.lin.trim()) os.linux = splitArgs(ox.lin);
      Object.assign(body, {
        args: splitArgs(ox.args), args_os: os, stop: split(ox.stop), env,
        query_offset: Number(g.query_offset) || 0, rcon_offset: Number(g.rcon_offset) || 0, stop_wait: Number(g.stop_wait) || 0,
        ports: Number(g.ports) || 0, default_port: Number(g.default_port) || 0,
      });
    }
    delete body.builtin; delete body.custom; delete body.servers;
    save.disabled = true;
    try {
      await api('PUT', 'api/games/' + encodeURIComponent(key), body);
      applyState(await api('GET', 'api/state'));
      toast((isNew ? 'Added ' : 'Saved ') + (g.name || key) + '.', 'ok');
      m.close();
      onSaved();
    } catch (e) { swap(err, h('div', { class: 'note bad' }, e.message)); save.disabled = false; }
  } }, icon('save'), isNew ? 'Add the game' : 'Save');
  append(m.bd, h('div', { class: 'formgrid' },
    field('Name', nameIn, 'As players know it.'),
    field('Short name', shortIn, 'On the cards and in Discord: TF2, CS:S…'),
    field('Key', keyIn, isNew ? 'Its id in the manager, letters and digits. Cannot change later.' : 'Its id in the manager.'),
    field('Engine', engineSel),
    field('Dedicated server app ID', bindText(g, 'server_appid', { type: 'number', min: 1, placeholder: '295230' }),
      h('span', null, 'What steamcmd downloads. Find it on ', h('a', { href: 'https://developer.valvesoftware.com/wiki/Dedicated_Servers_List', target: '_blank', rel: 'noopener' }, 'Valve\'s list'), ' or ', h('a', { href: 'https://steamdb.info/search/?a=app&q=dedicated+server', target: '_blank', rel: 'noopener' }, 'SteamDB'), '. GoldSrc mods: 90.')),
    field('Game app ID', bindText(g, 'appid', { type: 'number', min: 0, placeholder: 'Optional' }), 'The game players buy: for the update check. Optional.'),
    field('Web page', bindText(g, 'url', { type: 'url', placeholder: 'https://store.steampowered.com/app/…', spellcheck: false }), 'Its page, linked from the list of games. Empty: its Steam store page (from the game app ID).', true),
    field('Game folder', bindText(g, 'game', { placeholder: 'fof' }), 'Its -game folder, inside the server: cstrike, tf, csgo… (none for the other engines)'),
    field('First map', bindText(g, 'default_map', { placeholder: 'fof_fistful' }), 'What a new server starts on.'),
    field('Max players', bindText(g, 'default_maxplayers', { type: 'number', min: 1, max: 255 })),
    field('Program on Windows', winIn, 'Inside the server\'s folder. Several, comma separated: the first found is used.', true),
    field('Program on Linux', linIn, null, true),
    field('Icon', iconBox, 'Pick one, or upload a PNG (square, 64×64 is plenty). Uploaded icons go to the icons folder next to szm.exe.', true),
    h('div', { class: 'wide' }, bindCheck(g, 'login', 'Needs a Steam account', 'Its dedicated server cannot be downloaded anonymously.')),
    otherBox),
    err);
  append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'), save);
  setTimeout(() => (isNew ? nameIn : save).focus(), 30);
}

/* =================================================================================================================
 * Users
 * =============================================================================================================== */

function pageUsers() {
  const tbody = h('tbody');
  let users = [];
  let rights = {};

  async function load() {
    const r = await run(() => api('GET', 'api/users'));
    if (!r) return;
    users = r.users;
    rights = r.rights;
    render();
  }

  function render() {
    swap(tbody, users.map(u => {
      const admin = u.rights.includes('a');
      return h('tr', null,
        h('td', null, h('div', { class: 'row' }, h('div', { class: 'avatar', style: 'width:30px;height:30px;font-size:12px;cursor:default' }, u.name[0].toUpperCase()),
          h('div', null, h('b', null, u.name), u.name === S.user.name ? h('span', { class: 'tag purple', style: 'margin-left:6px' }, 'you') : null))),
        h('td', null, admin ? h('span', { class: 'tag purple' }, 'Admin') : h('div', { class: 'chips' }, u.rights.split('').map(c => h('span', { class: 'tag', title: rights[c] }, rights[c] || c)))),
        h('td', { class: 'muted small' }, admin || u.rights.includes('e') ? 'All' : (u.servers || []).map(id => (serverBy(id) || { name: id }).name).join(', ') || 'None'),
        h('td', { class: 'muted small nowrap' }, u.last_login ? timeAgo(u.last_login) : 'never'),
        h('td', { class: 'nowrap', style: 'text-align:right' },
          h('button', { class: 'btn sm ghost', onclick: () => edit(u) }, icon('edit'), 'Edit'),
          u.name !== S.user.name ? h('button', { class: 'btn sm ghost icon', title: 'Delete', 'aria-label': 'Delete ' + u.name, onclick: async () => {
            if (await confirmBox('Delete ' + u.name + '?', 'They can not sign in any more.', 'Delete', true) && await run(() => api('DELETE', 'api/users/' + encodeURIComponent(u.name)), u.name + ' deleted.')) load();
          } }, icon('trash')) : null));
    }));
  }

  function edit(u) {
    const isNew = !u;
    const st = { name: u ? u.name : '', rights: new Set(u ? u.rights.split('') : ['e', 'g', 'i']), servers: new Set(u ? u.servers || [] : []), password: '' };
    const m = modal({ title: isNew ? 'New user' : 'Edit ' + u.name, size: 'wide' });
    const nameIn = h('input', { type: 'text', value: st.name, readOnly: !isNew, spellcheck: false, autocomplete: 'off' });
    const pwIn = h('input', { type: 'password', autocomplete: 'new-password', placeholder: isNew ? '8 characters or more' : 'Leave empty to keep it' });
    const rightsEl = h('div', { class: 'formgrid', style: 'grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:10px' });
    const serversWrap = h('div');
    const renderRights = () => {
      const admin = st.rights.has('a');
      swap(rightsEl, Object.keys(rights).sort().map(c => {
        const inp = h('input', { type: 'checkbox', checked: admin || st.rights.has(c), disabled: admin && c !== 'a' || (!can('a') && !can(c)) });
        inp.addEventListener('change', () => { inp.checked ? st.rights.add(c) : st.rights.delete(c); renderRights(); });
        return h('label', { class: 'check' }, inp, h('span', null, h('b', null, rights[c]), h('small', null, 'Right ' + c)));
      }));
      const all = admin || st.rights.has('e');
      swap(serversWrap, all ? h('div', { class: 'muted small' }, 'Sees every server.') :
        h('div', { class: 'chips' }, S.servers.map(v => h('button', {
          type: 'button', class: 'chip' + (st.servers.has(v.id) ? ' on' : ''),
          onclick: e => { st.servers.has(v.id) ? st.servers.delete(v.id) : st.servers.add(v.id); e.currentTarget.classList.toggle('on'); },
        }, v.name))));
    };
    renderRights();
    const err = h('div', { class: 'err' });
    append(m.bd, h('div', { class: 'col', style: 'gap:16px' },
      h('div', { class: 'formgrid' }, field('Name', nameIn), field(isNew ? 'Password' : 'New password', pwIn)),
      h('div', null, h('h3', { style: 'margin-bottom:10px' }, 'Rights'), rightsEl),
      h('div', null, h('h3', { style: 'margin-bottom:10px' }, 'Servers they look after'), serversWrap), err));
    append(m.ft, h('button', { class: 'btn ghost', onclick: () => m.close() }, 'Cancel'),
      h('button', { class: 'btn primary', onclick: async () => {
        err.textContent = '';
        try {
          await api('PUT', 'api/users/' + encodeURIComponent(nameIn.value.trim()), { rights: [...st.rights].join(''), servers: [...st.servers], password: pwIn.value });
          m.close();
          toast('Saved.', 'ok');
          load();
        } catch (e) { err.textContent = e.message; }
      } }, icon('save'), isNew ? 'Create' : 'Save'));
    setTimeout(() => (isNew ? nameIn : pwIn).focus(), 30);
  }

  load();
  return {
    el: h('div', { style: 'max-width:1100px' },
      h('div', { class: 'pagehead' }, h('div', { class: 'grow' }, h('h1', null, 'Users'), h('div', { class: 'sub' }, 'Who can sign in, what they can do, and which servers they look after.')),
        h('button', { class: 'btn primary', onclick: () => edit(null) }, icon('plus'), 'New user')),
      h('div', { class: 'panel', style: 'overflow:auto' }, h('table', { class: 'list evlist' },
        h('thead', null, h('tr', null, h('th', null, 'User'), h('th', null, 'Rights'), h('th', null, 'Servers'), h('th', null, 'Last sign-in'), h('th'))), tbody))),
  };
}

/* =================================================================================================================
 * Account
 * =============================================================================================================== */

function pageAccount() {
  const oldIn = h('input', { type: 'password', autocomplete: 'current-password' });
  const newIn = h('input', { type: 'password', autocomplete: 'new-password' });
  const new2 = h('input', { type: 'password', autocomplete: 'new-password' });
  const err = h('div', { class: 'err' });
  return {
    el: h('div', { style: 'max-width:480px' },
      h('div', { class: 'pagehead' }, h('div', null, h('h1', null, 'Your account'), h('div', { class: 'sub' }, 'Signed in as ' + S.user.name + '.'))),
      h('form', {
        class: 'panel bd col', style: 'padding:20px;gap:14px', onsubmit: async e => {
          e.preventDefault();
          err.textContent = '';
          if (newIn.value !== new2.value) { err.textContent = 'The new passwords are not the same.'; return; }
          try {
            await api('POST', 'api/password', { old: oldIn.value, new: newIn.value });
            toast('Password changed. Your other sessions were signed out.', 'ok');
            oldIn.value = newIn.value = new2.value = '';
          } catch (ex) { err.textContent = ex.message; }
        },
      },
      field('Current password', oldIn), field('New password', newIn, '8 characters or more.'), field('New password again', new2), err,
      h('div', null, h('button', { class: 'btn primary', type: 'submit' }, icon('key'), 'Change password')))),
  };
}

/* =================================================================================================================
 * Tooltips: any element with data-tip (set through h()'s title), on hover and on keyboard focus
 * =============================================================================================================== */

(function tooltips() {
  const tip = document.createElement('div');
  tip.className = 'tip';
  tip.setAttribute('role', 'tooltip');
  document.body.append(tip);
  let target = null;
  let timer = null;
  const hide = () => { clearTimeout(timer); target = null; tip.classList.remove('on'); };
  const place = el => {
    tip.textContent = el.getAttribute('data-tip');
    tip.classList.remove('above', 'below');
    tip.style.left = '0px';
    tip.style.top = '0px';
    const r = el.getBoundingClientRect();
    const t = tip.getBoundingClientRect();
    const above = r.top - t.height - 10 >= 6;
    let left = r.left + r.width / 2 - t.width / 2;
    left = Math.max(8, Math.min(left, window.innerWidth - t.width - 8));
    tip.style.left = left + 'px';
    tip.style.top = (above ? r.top - t.height - 9 : r.bottom + 9) + 'px';
    tip.style.setProperty('--ax', Math.max(12, Math.min(t.width - 12, r.left + r.width / 2 - left)) + 'px');
    tip.classList.add(above ? 'above' : 'below', 'on');
  };
  const show = el => {
    if (el === target) return;
    hide();
    target = el;
    timer = setTimeout(() => { if (target === el && el.isConnected) place(el); }, 280);
  };
  let mouseX = -1, mouseY = -1;
  document.addEventListener('mousemove', e => { mouseX = e.clientX; mouseY = e.clientY; }, { passive: true });
  document.addEventListener('mouseover', e => {
    mouseX = e.clientX; mouseY = e.clientY;
    const el = e.target.closest && e.target.closest('[data-tip]');
    if (el && el.getAttribute('data-tip')) show(el); else if (target) hide();
  });
  document.addEventListener('focusin', e => {
    const el = e.target.closest && e.target.closest('[data-tip]');
    if (el && el.getAttribute('data-tip') && e.target.matches(':focus-visible')) show(el); else hide();
  });
  document.addEventListener('focusout', hide);
  document.addEventListener('mousedown', hide);
  document.addEventListener('keydown', e => { if (e.key === 'Escape') hide(); });
  window.addEventListener('scroll', hide, true);
  window.addEventListener('resize', hide);
  // The page redraws parts of itself every second, with new elements: a tooltip whose element was replaced moves to
  // the one now under the mouse, at once and with its new text (live numbers stay readable); it goes when there is none
  const follow = () => {
    if (!target || target.isConnected) return;
    const open = tip.classList.contains('on');
    const under = mouseX >= 0 ? document.elementFromPoint(mouseX, mouseY) : null;
    const el = under && under.closest && under.closest('[data-tip]');
    if (!el || !el.getAttribute('data-tip')) { hide(); return; }
    target = el;
    if (open) place(el);
    else { clearTimeout(timer); timer = setTimeout(() => { if (target === el && el.isConnected) place(el); }, 280); }
  };
  new MutationObserver(() => { if (target && !target.isConnected) follow(); }).observe(document.body, { childList: true, subtree: true });
  setInterval(follow, 500);
})();

/* =================================================================================================================
 * Start
 * =============================================================================================================== */

// the address the Reload of an updated manager used (?v=…): back to the plain one
if (/[?&]v=/.test(location.search)) { try { history.replaceState(null, '', location.pathname + location.hash); } catch (e) { /* kept */ } }

showLogin();

})();
