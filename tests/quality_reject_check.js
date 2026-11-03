// Стенд для кнопки «Не тот трек» в окне найденного FLAC.
const fs = require('fs');
const path = require('path');
const vm = require('vm');

const script = fs.readFileSync(path.join(__dirname, '..', 'src', 'frontend', 'dist', 'app.js'), 'utf8');

function makeEl(tag) {
  let proxy;
  const el = {
    tag,
    children: [],
    style: {},
    className: '',
    textContent: '',
    title: '',
    value: '',
    checked: false,
    hidden: false,
    disabled: false,
    type: '',
    id: '',
    dataset: {},
    open: false,
    _cls: new Set(),
    _attrs: {},
    _listeners: {},
    classList: {
      add: (c) => el._cls.add(c),
      remove: (c) => el._cls.delete(c),
      contains: (c) => el._cls.has(c),
    },
    appendChild(c) { el.children.push(c); c.parent = el; return c; },
    remove() {
      if (el.parent) el.parent.children = el.parent.children.filter((x) => x !== el);
    },
    addEventListener(type, fn) { (el._listeners[type] = el._listeners[type] || []).push(fn); },
    removeEventListener(type, fn) {
      el._listeners[type] = (el._listeners[type] || []).filter((x) => x !== fn);
    },
    setAttribute(name, value) { el._attrs[name] = String(value); },
    getAttribute(name) { return el._attrs[name]; },
    removeAttribute(name) { delete el._attrs[name]; },
    focus() { document.activeElement = (el.id && registry[el.id]) || proxy || el; },
    blur() { if (document.activeElement === el) document.activeElement = null; },
    querySelectorAll() { return []; },
    get innerHTML() { return ''; },
    set innerHTML(v) { el.children = []; },
  };
  proxy = new Proxy(el, {
    get(t, p) {
      if (p in t) return t[p];
      if (typeof p === 'string' && /^\d+$/.test(p)) return t.children[Number(p)];
      return undefined;
    },
    set(t, p, v) { t[p] = v; return true; },
  });
  return proxy;
}

const registry = {};
const document = {
  activeElement: null,
  body: makeEl('body'),
  getElementById(id) {
    if (!registry[id]) {
      registry[id] = makeEl('div');
      registry[id].id = id;
    }
    return registry[id];
  },
  createElement(tag) { return makeEl(tag); },
  addEventListener() {},
  removeEventListener() {},
};

const window = {
  runtime: { EventsOn() {} },
  go: { ui: { App: {} } },
  addEventListener() {},
};
const ctx = {
  document,
  window,
  requestAnimationFrame: (fn) => fn(),
  setTimeout: (fn) => { fn(); return 0; },
  clearTimeout() {},
  console,
  Number,
  Math,
  Map,
  Set,
  String,
  Array,
  Object,
  JSON,
};
vm.createContext(ctx);
vm.runInContext(script, ctx);

const api = window.__yamdl;
let failures = 0;
function check(name, cond, extra) {
  if (cond) console.log('  ok - ' + name);
  else {
    failures++;
    console.log('  FAIL - ' + name + (extra ? ' | ' + extra : ''));
  }
}
const tick = () => new Promise((r) => setImmediate(r));

const searchItem = {
  key: 'C:/Music/a.mp3', path: 'C:/Music/a.mp3', fileName: 'a.mp3',
  label: 'Artist - Song', matchSource: 'search', trackId: '42',
  matchedArtist: 'Artist', matchedTitle: 'Song (Remix)', selected: false,
};
const idItem = {
  key: 'C:/Music/b.mp3', path: 'C:/Music/b.mp3', fileName: 'b.mp3',
  label: 'Artist - Original', matchSource: 'id', trackId: '7', selected: true,
};
const alreadyFlacItem = {
  key: 'C:/Music/c.mp3', path: 'C:/Music/c.mp3', fileName: 'c.mp3',
  label: 'Artist - Has FLAC', matchSource: 'search', trackId: '8',
  alreadyFlac: 'C:/Music/c.flac', selected: true,
};

(async () => {
  console.log('scenario: reject action visibility');
  api.renderQuality([searchItem, idItem, alreadyFlacItem]);
  api.openQuality();
  const rows = registry['qualityList'].children;
  check('три строки отрисованы', rows.length === 3, String(rows.length));
  check('поисковая строка имеет кнопку', rows[0].children.length === 2 &&
    rows[0].children[1].textContent === 'Не тот трек');
  check('ID-строка без кнопки', rows[1].children.length === 1);
  check('alreadyFlac-строка без кнопки', rows[2].children.length === 1);

  console.log('scenario: reject removes the row');
  const calls = [];
  window.go.ui.App.RejectFound = async (item) => {
    calls.push(item);
    return [idItem];
  };
  window.go.ui.App.GetSettings = async () => ({
    QualityRejected: [{
      path: 'C:/Music/a.mp3', fileName: 'a.mp3', localLabel: 'Artist - Song',
      trackId: '42', matchedArtist: 'Artist', matchedTitle: 'Song (Remix)',
      declinedAtText: '04.11.2026 00:00',
    }],
  });
  await rows[0].children[1].onclick({ preventDefault() {}, stopPropagation() {} });
  await tick();
  check('RejectFound вызван с кандидатом', calls.length === 1 && calls[0].trackId === '42');
  check('список отказов обновился', registry['sRejectedList'].children.length === 1);
  check('в списке остался только ID', api.getQualityItems().length === 1 &&
    api.getQualityItems()[0].trackId === '7');
  check('бейдж обновился', registry['settingsBadge'].textContent === '1' &&
    registry['settingsBadge'].hidden === false);
  check('окно осталось открытым', registry['qualityOverlay'].classList.contains('open'));
  const remainingRow = registry['qualityList'].children[0];
  check('фокус перешёл на оставшуюся строку', document.activeElement === remainingRow.children[0].children[0]);

  console.log('scenario: last rejection closes the modal');
  window.go.ui.App.RejectFound = async () => [];
  api.renderQuality([searchItem]);
  api.openQuality();
  const lastRow = registry['qualityList'].children[0];
  await lastRow.children[1].onclick({ preventDefault() {}, stopPropagation() {} });
  await tick();
  check('список пуст', api.getQualityItems().length === 0);
  check('окно закрыто', !registry['qualityOverlay'].classList.contains('open'));
  check('бейдж скрыт', registry['settingsBadge'].hidden === true);

  console.log('scenario: backend error stays visible');
  window.go.ui.App.RejectFound = async () => { throw new Error('boom'); };
  api.renderQuality([searchItem]);
  api.openQuality();
  await registry['qualityList'].children[0].children[1].onclick({
    preventDefault() {}, stopPropagation() {},
  });
  await tick();
  check('ошибка показана', registry['qualityError'].textContent.includes('boom'));
  check('строка не исчезла', api.getQualityItems().length === 1);

  process.exit(failures ? 1 : 0);
})();
