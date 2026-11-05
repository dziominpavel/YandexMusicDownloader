// Стенд FLAC-флоу: бейдж на настройках, тихая автопроверка и проверка
// без закрытия формы. Исполняет app.js в VM с минимальным DOM.
const fs = require('fs');
const path = require('path');
const vm = require('vm');

const script = fs.readFileSync(path.join(__dirname, '..', 'src', 'frontend', 'dist', 'app.js'), 'utf8');

function makeEl(tag) {
  const el = {
    tag,
    children: [],
    style: {},
    dataset: {},
    className: '',
    textContent: '',
    title: '',
    value: '',
    checked: false,
    hidden: false,
    disabled: false,
    type: '',
    id: '',
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
    focus() { document.activeElement = (el.id && registry[el.id]) || el; },
    blur() { if (document.activeElement === el) document.activeElement = null; },
    querySelectorAll() { return []; },
    get innerHTML() { return ''; },
    set innerHTML(v) { el.children = []; },
  };
  return new Proxy(el, {
    get(t, p) {
      if (p in t) return t[p];
      if (typeof p === 'string' && /^\d+$/.test(p)) return t.children[Number(p)];
      return undefined;
    },
    set(t, p, v) { t[p] = v; return true; },
  });
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

const listeners = {};
const emit = (name, ...args) => {
  for (const fn of listeners[name] || []) fn(...args);
};

const window = {
  runtime: {
    EventsOn(name, fn) { (listeners[name] = listeners[name] || []).push(fn); },
  },
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

const pending = [
  { key: 'C:/Music/A.mp3', path: 'C:/Music/A.mp3', fileName: 'A.mp3', label: 'A', foundFormat: 'FLAC', selected: true, matchSource: 'id' },
  { key: 'C:/Music/B.mp3', path: 'C:/Music/B.mp3', fileName: 'B.mp3', label: 'B', foundFormat: 'FLAC', selected: true, matchSource: 'id' },
  { key: 'C:/Music/C.mp3', path: 'C:/Music/C.mp3', fileName: 'C.mp3', label: 'C', foundFormat: 'FLAC', selected: true, matchSource: 'id' },
];
const more = pending.concat([
  { key: 'C:/Music/D.mp3', path: 'C:/Music/D.mp3', fileName: 'D.mp3', label: 'D', foundFormat: 'FLAC', selected: true, matchSource: 'search' },
  { key: 'C:/Music/E.mp3', path: 'C:/Music/E.mp3', fileName: 'E.mp3', label: 'E', foundFormat: 'FLAC', selected: false, matchSource: 'search' },
]);

window.go.ui.App.QualityStatus = async () => ({
  text: 'FLAC: 04.11.2026 10:00 — найдено 3',
  checking: false,
  pending: 3,
  outputDir: 'C:/downloads',
});
window.go.ui.App.PendingFound = async () => pending;
window.go.ui.App.GetSettings = async () => ({
  Token: 'token',
  OutputDir: 'C:/downloads',
  LibraryDir: 'C:/Music',
  Workers: 3,
  ConvertM4A: true,
  EmbedCover: true,
  EmbedLyrics: true,
  SaveLRC: false,
  Subdirs: false,
  SubdirTemplate: '%artist%/%album%',
  ExportM3U: false,
  FlacMonitor: true,
  QualityText: 'FLAC: 04.11.2026 10:00 — найдено 5',
  QualityChecking: false,
  QualityPending: 5,
});

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

(async () => {
  await tick();
  await tick();

  console.log('scenario: startup badge and quiet auto-check');
  check('бейдж показывает 3', registry['settingsBadge'].textContent === '3' && !registry['settingsBadge'].hidden);
  check('имя настроек содержит количество', String(registry['settingsBtn'].getAttribute('aria-label')).includes('3'));
  check('кнопка «Открыть найденное» видна',
    !registry['sQualityOpen'].hidden && registry['sQualityOpen'].textContent === 'Открыть найденное (3)');
  check('окно найденного само не открылось', !registry['qualityOverlay'].classList.contains('open'));

  console.log('scenario: quality-found updates badge without opening');
  emit('quality-found', more);
  check('бейдж обновился до 5', registry['settingsBadge'].textContent === '5' && !registry['settingsBadge'].hidden);
  check('окно не открылось', !registry['qualityOverlay'].classList.contains('open'));

  console.log('scenario: open found list from settings');
  await api.openSettings();
  await tick();
  check('настройки открыты', registry['settingsOverlay'].classList.contains('open'));
  registry['sQualityOpen'].onclick();
  check('окно найденного открылось', registry['qualityOverlay'].classList.contains('open'));
  check('в сводке есть папка назначения', String(registry['qualitySummary'].textContent).includes('C:/downloads'));
  check('в сводке есть правило _old', String(registry['qualitySummary'].textContent).includes('_old'));
  api.closeQuality();
  check('окно закрылось', !registry['qualityOverlay'].classList.contains('open'));
  check('настройки остались открыты', registry['settingsOverlay'].classList.contains('open'));
  check('фокус вернулся на «Открыть найденное»', document.activeElement === registry['sQualityOpen'],
    document.activeElement && document.activeElement.id);

  console.log('scenario: check does not close settings');
  window.go.ui.App.CheckQuality = async () => {
    emit('quality-check-started');
    await new Promise((r) => setTimeout(r, 0));
    emit('quality-pending', []);
    emit('quality-check-finished', 'FLAC: 04.11.2026 10:05 — ничего не найдено', 0, '', 0);
    return {};
  };
  const wasOpen = registry['settingsOverlay'].classList.contains('open');
  registry['sCheckNow'].onclick();
  check('форма не закрылась сразу', wasOpen && registry['settingsOverlay'].classList.contains('open'));
  check('кнопка проверки заблокирована на время', registry['sCheckNow'].disabled === true);
  await new Promise((r) => setTimeout(r, 10));
  await tick();
  check('форма не закрылась после проверки', registry['settingsOverlay'].classList.contains('open'));
  check('статус показывает результат', String(registry['sQualityStatus'].textContent).includes('ничего не найдено'));
  check('статус не содержит «идёт»', !String(registry['sQualityStatus'].textContent).includes('идёт'));
  check('кнопка проверки снова доступна', registry['sCheckNow'].disabled === false);
  check('бейдж скрыт при нуле', registry['settingsBadge'].hidden === true);
  check('«Открыть найденное» скрыто при нуле', registry['sQualityOpen'].hidden === true);

  process.exit(failures ? 1 : 0);
})();
