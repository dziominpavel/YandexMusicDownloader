// Стенд для dirty-guard настроек: исполняет app.js в VM с минимальным DOM.
const fs = require('fs');
const path = require('path');
const vm = require('vm');

const script = fs.readFileSync(path.join(__dirname, '..', 'src', 'frontend', 'dist', 'app.js'), 'utf8');

function makeEl(tag) {
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

const settings = {
  Token: 'old-token',
  LibraryDir: 'C:/Music',
  OutputDir: 'C:/Music',
  Workers: 3,
  ConvertM4A: true,
  EmbedCover: true,
  EmbedLyrics: true,
  SaveLRC: false,
  Subdirs: true,
  SubdirTemplate: '%artist%/%album%',
  ExportM3U: false,
  FlacMonitor: true,
  QualityText: 'FLAC: проверок не было',
};

(async () => {
  console.log('scenario: open settings and snapshot');
  window.go.ui.App.GetSettings = async () => settings;
  await api.openSettings();
  await tick();
  check('настройки открыты', registry['settingsOverlay'].classList.contains('open'));
  check('фокус в первом поле', document.activeElement === registry['sToken'],
    document.activeElement && document.activeElement.id);
  check('снимок чистый', api.isDirty() === false);

  console.log('scenario: dirty guard on backdrop/Escape');
  registry['sToken'].value = 'new-token';
  check('изменение замечено', api.isDirty() === true);
  api.closeSettings(false);
  check('появился confirm', registry['confirmOverlay'].classList.contains('open'));
  check('настройки не закрылись', registry['settingsOverlay'].classList.contains('open'));
  api.closeConfirm();
  check('после отказа настройки открыты', registry['settingsOverlay'].classList.contains('open'));
  check('после отказа confirm закрыт', !registry['confirmOverlay'].classList.contains('open'));

  console.log('scenario: clean close');
  registry['sToken'].value = settings.Token;
  check('изменения отменены', api.isDirty() === false);
  api.closeSettings(false);
  check('настройки закрылись без вопроса', !registry['settingsOverlay'].classList.contains('open'));
  check('confirm не показывался', !registry['confirmOverlay'].classList.contains('open'));

  console.log('scenario: explicit cancel');
  await api.openSettings();
  await tick();
  registry['sToken'].value = 'another-token';
  check('есть изменения', api.isDirty() === true);
  api.closeSettings(true);
  check('Отмена закрыла без вопроса', !registry['settingsOverlay'].classList.contains('open'));
  check('confirm не показывался', !registry['confirmOverlay'].classList.contains('open'));

  process.exit(failures ? 1 : 0);
})();
