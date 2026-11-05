// Стенд очереди: исполняет app.js в VM с минимальным DOM и проигрывает
// события task-added/task-updated/queue/sources, проверяя разметку строк,
// агрегат, источники, журнал и действия.
const fs = require('fs');
const path = require('path');
const vm = require('vm');

const script = fs.readFileSync(path.join(__dirname, '..', 'src', 'frontend', 'dist', 'app.js'), 'utf8');

// --- Минимальный DOM-стуб ---
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
    removeChild(c) {
      el.children = el.children.filter((x) => x !== c);
      if (c) c.parent = null;
    },
    remove() {
      if (el.parent) {
        el.parent.children = el.parent.children.filter((x) => x !== el && x !== el._proxy);
      }
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
  const proxy = new Proxy(el, {
    get(t, p) {
      if (p in t) return t[p];
      if (typeof p === 'string' && /^\d+$/.test(p)) return t.children[Number(p)];
      return undefined;
    },
    set(t, p, v) { t[p] = v; return true; },
  });
  el._proxy = proxy;
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

const listeners = {};
const emit = (name, ...args) => {
  for (const fn of listeners[name] || []) fn(...args);
};

const calls = [];
let submitResult = '';
let queueSnapshot = emptySnapshot();

function emptySnapshot() {
  return {
    tasks: [], sources: [], total: 0, done: 0, skipped: 0, failed: 0,
    canceled: 0, active: 0, waiting: 0, review: 0, percent: 0,
    bytesDone: 0, bytesTotal: 0, workers: 3,
  };
}

const App = {
  BootReady: () => Promise.resolve(),
  Submit: (raw) => { calls.push(['Submit', raw]); return Promise.resolve(submitResult); },
  GetQueue: () => Promise.resolve(queueSnapshot),
  PreviewLink: () => Promise.reject(new Error('no preview')),
  CancelTask: (k) => { calls.push(['CancelTask', k]); return Promise.resolve(); },
  RetryTask: (k) => { calls.push(['RetryTask', k]); return Promise.resolve(); },
  RemoveTask: (k) => { calls.push(['RemoveTask', k]); return Promise.resolve(); },
  CancelAll: () => { calls.push(['CancelAll']); return Promise.resolve(); },
  ClearFinished: () => { calls.push(['ClearFinished']); return Promise.resolve(); },
  CancelSource: (k) => { calls.push(['CancelSource', k]); return Promise.resolve(); },
  ResolveDuplicate: (k, a) => { calls.push(['ResolveDuplicate', k, a]); return Promise.resolve(); },
  OpenOutputDir: () => { calls.push(['OpenOutputDir']); return Promise.resolve(); },
  QualityStatus: () => Promise.reject(new Error('no quality')),
  GetSettings: () => Promise.resolve({}),
};

const window = {
  runtime: {
    EventsOn(name, fn) { (listeners[name] = listeners[name] || []).push(fn); },
  },
  go: { ui: { App } },
  addEventListener() {},
};
const ctx = {
  document,
  window,
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
  Promise,
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
const dispatch = (el, type, event) => {
  const ev = Object.assign({ preventDefault() {}, stopPropagation() {}, isComposing: false }, event || {});
  for (const fn of (el._listeners && el._listeners[type]) || []) fn(ev);
};
const task = (over) => Object.assign({
  key: 't1', trackId: '1', label: 'A — T1', kind: 'download',
  sourceKey: 's1', sourceLabel: 'Альбом · Микс', state: 'queued',
  percent: 0, done: 0, total: 0, format: '', fallbackReason: '',
  reason: '', path: '', position: 1, candidate: undefined,
}, over || {});

(async () => {
  await tick();
  await tick();

  console.log('scenario: init');
  check('заставка снята', api.isBootDismissed() === true);
  check('поле ссылки в фокусе', document.activeElement === registry['link'],
    document.activeElement && document.activeElement.id);
  check('состояние empty', api.getState() === 'empty' && registry['queueSection'].hidden === true);

  console.log('scenario: empty Enter does nothing');
  const before = calls.length;
  registry['link'].value = '';
  dispatch(registry['link'], 'keydown', { key: 'Enter' });
  await tick();
  check('Submit не вызван', calls.length === before);

  console.log('scenario: порядок строк из снапшота');
  registry['tasks'].children = [];
  api.handleTaskAdded(task({ key: 'ord1', state: 'done', percent: 100, format: 'FLAC' }));
  api.handleTaskAdded(task({ key: 'ord2', state: 'downloading', percent: 10, total: 100, done: 10 }));
  api.handleTaskAdded(task({ key: 'ord3', state: 'queued', position: 1 }));
  api.handleQueue({
    tasks: [
      task({ key: 'ord2', state: 'downloading', percent: 10, total: 100, done: 10 }),
      task({ key: 'ord3', state: 'queued', position: 1 }),
      task({ key: 'ord1', state: 'done', percent: 100, format: 'FLAC' }),
    ],
    sources: [], total: 3, done: 1, skipped: 0, failed: 0, canceled: 0,
    active: 1, waiting: 1, review: 0, percent: 40, bytesDone: 10, bytesTotal: 100, workers: 3,
  });
    const rowOrder = registry['tasks'].children.map((c) => c.dataset.key).join(',');
    check('активные, ожидание, завершённые', rowOrder === 'ord2,ord3,ord1', rowOrder);

  console.log('scenario: Enter ставит ссылку в очередь');
  registry['link'].value = 'https://music.yandex.ru/album/1/track/2';
  dispatch(registry['link'], 'keydown', { key: 'Enter' });
  await tick();
  const submitCall = calls.find((c) => c[0] === 'Submit');
  check('Submit получил ссылку', !!submitCall && submitCall[1] === 'https://music.yandex.ru/album/1/track/2');
  check('поле очищено', registry['link'].value === '');
  check('фокус остался в поле', document.activeElement === registry['link']);

  console.log('scenario: вторая ссылка во время работы');
  api.handleQueue({
    tasks: [task({ key: 'a1', state: 'downloading', percent: 30, total: 100, done: 30 })],
    sources: [{ key: 's1', label: 'Альбом · Микс', kind: 'album', title: 'Микс' }],
    total: 1, done: 0, skipped: 0, failed: 0, canceled: 0,
    active: 1, waiting: 0, review: 0, percent: 30, bytesDone: 30, bytesTotal: 100, workers: 3,
  });
  check('состояние working', api.getState() === 'working' && registry['queueSection'].hidden === false);
  check('настройки доступны', registry['settingsBtn'].disabled === false);
  calls.length = 0;
  registry['link'].value = 'https://music.yandex.ru/album/2/track/3';
  dispatch(registry['link'], 'keydown', { key: 'Enter' });
  await tick();
  check('вторая ссылка принята', calls.some((c) => c[0] === 'Submit'));
  check('первая загрузка не остановлена', !calls.some((c) => c[0] === 'CancelAll'));

  console.log('scenario: строка задачи проходит состояния');
  registry['tasks'].children = [];
  api.handleTaskAdded(task({ key: 't1', state: 'resolving', label: 'Определяется…' }));
  let row = registry['tasks'].children[0];
  check('строка создана', !!row);
  api.handleTaskUpdated(task({ key: 't1', state: 'queued', position: 2 }));
  check('в очереди с позицией', row._refs.state.textContent === 'в очереди #2', row._refs.state.textContent);
  api.handleTaskUpdated(task({ key: 't1', state: 'downloading', percent: 42, done: 44040192, total: 104857600 }));
  check('скачивается 42%', row._refs.state.textContent === 'скачивается 42%', row._refs.state.textContent);
  check('полоса 42%', row._refs.fill.style.width === '42%', row._refs.fill.style.width);
  check('полоса показана', row._refs.bar.hidden === false);
  check('размер 42.0 / 100.0 МБ', row._refs.size.textContent === '42.0 / 100.0 МБ', row._refs.size.textContent);
  api.handleTaskUpdated(task({ key: 't1', state: 'done', percent: 100, total: 1048576, format: 'FLAC' }));
  check('готово', row._refs.state.textContent === 'готово');
  check('формат FLAC', row._refs.pill.textContent === 'FLAC' && row._refs.pill.hidden === false);
  check('у готовой строки нет полосы', row._refs.bar.hidden === true);
  const actions = row._refs.actions.children;
  check('действия готовой строки', actions.length === 2);
  check('кнопка папки доступна', String(actions[0].getAttribute('aria-label')).includes('папку'));
  actions[0].onclick();
  await tick();
  check('папка открывается', calls.some((c) => c[0] === 'OpenOutputDir'));

  console.log('scenario: ошибка и повтор');
  api.handleTaskUpdated(task({ key: 't1', state: 'error', reason: 'сеть упала' }));
  check('ошибка в строке', row._refs.state.textContent === 'ошибка');
  check('причина видна', row._refs.reason.textContent === 'сеть упала');
  const errActions = row._refs.actions.children;
  errActions[0].onclick();
  await tick();
  check('повтор вызван', calls.some((c) => c[0] === 'RetryTask' && c[1] === 't1'));

  console.log('scenario: дубль решается в строке');
  api.handleTaskUpdated(task({
    key: 't1', state: 'needs-review',
    candidate: { percent: 93, fileName: 'Artist - Song.mp3', filePath: 'C:/Music/Artist - Song.mp3', differentFormat: true },
  }));
  check('состояние дубля', row._refs.state.textContent === 'возможен дубль 93%');
  check('блок дубля виден', row._refs.dup.hidden === false);
  check('путь виден целиком', String(row._refs.dupPath.textContent).includes('Artist - Song.mp3'));
  const dupButtons = row._refs.dupActions.children;
  check('две кнопки решения', dupButtons.length === 2);
  dupButtons[1].onclick();
  await tick();
  check('«Скачать всё равно» вызвано', calls.some((c) => c[0] === 'ResolveDuplicate' && c[1] === 't1' && c[2] === true));
  check('фокус не перехвачен', document.activeElement === registry['link']);

  console.log('scenario: агрегат очереди');
  const list = [];
  for (let i = 0; i < 10; i++) list.push(task({ key: 'd' + i, state: 'done', percent: 100, format: 'FLAC' }));
  for (let i = 0; i < 2; i++) list.push(task({ key: 'a' + i, state: 'downloading', percent: 50, total: 100, done: 50 }));
  for (let i = 0; i < 8; i++) list.push(task({ key: 'q' + i, state: 'queued', position: i + 1 }));
  api.handleQueue({
    tasks: list, sources: [{ key: 's1', label: 'Альбом · Микс', kind: 'album' }],
    total: 20, done: 10, skipped: 0, failed: 0, canceled: 0,
    active: 2, waiting: 8, review: 0, percent: 55, bytesDone: 100, bytesTotal: 200, workers: 3,
  });
  check('процент 55', registry['qPct'].textContent === '55%', registry['qPct'].textContent);
  check('полоса 55%', registry['qfill'].style.width === '55%');
  check('aria-valuenow 55', registry['qbar'].getAttribute('aria-valuenow') === '55');
  check('счётчик потоков', registry['qWorkers'].textContent === 'потоков: 3', registry['qWorkers'].textContent);
  const counts = registry['qCounts'].children.map((c) => c.textContent);
  check('счётчики состояний', counts.includes('2 качаются') && counts.includes('8 в очереди') && counts.includes('10 готово'),
    JSON.stringify(counts));
  check('остановить всё видна', registry['qStopAll'].hidden === false);
  check('очистить завершённые видна', registry['qClear'].hidden === false);

  console.log('scenario: смена лимита потоков приходит из настроек');
  api.handleQueue({
    tasks: list, sources: [{ key: 's1', label: 'Альбом · Микс', kind: 'album' }],
    total: 20, done: 10, skipped: 0, failed: 0, canceled: 0,
    active: 2, waiting: 8, review: 0, percent: 55, bytesDone: 100, bytesTotal: 200, workers: 5,
  });
  check('лимит обновился на 5', registry['qWorkers'].textContent === 'потоков: 5', registry['qWorkers'].textContent);
  check('подпись объясняет смысл', String(registry['qWorkers'].title).includes('Настройках'));

  console.log('scenario: завершённая очередь');
  const doneList = list.map((t) => Object.assign({}, t, { state: 'done', percent: 100 }));
  api.handleQueue({
    tasks: doneList, sources: [{ key: 's1', label: 'Альбом · Микс', kind: 'album' }],
    total: 20, done: 20, skipped: 0, failed: 0, canceled: 0,
    active: 0, waiting: 0, review: 0, percent: 100, bytesDone: 0, bytesTotal: 0, workers: 3,
  });
  check('состояние settled', api.getState() === 'settled');
  check('остановить всё скрыта', registry['qStopAll'].hidden === true);
  check('очистить завершённые видна', registry['qClear'].hidden === false);

  console.log('scenario: источники и фильтр');
  registry['tasks'].children = [];
  api.handleTaskAdded(task({ key: 'sx1t', sourceKey: 'sx1', sourceLabel: 'Альбом · Микс', state: 'done' }));
  api.handleTaskAdded(task({ key: 'sx2t', sourceKey: 'sx2', sourceLabel: 'Трек · Solo', state: 'done' }));
  api.handleSources([
    { key: 'sx1', label: 'Альбом · Микс', kind: 'album' },
    { key: 'sx2', label: 'Трек · Solo', kind: 'track' },
  ]);
  check('два чипа источников', registry['sources'].children.length === 2);
  let chip1 = registry['sources'].children[0];
  check('счётчик источника', chip1.children[1].textContent === '1/1', chip1.children[1].textContent);
  chip1.children[0].onclick();
  check('фильтр применён', registry['tasks'].children[1].hidden === true, String(registry['tasks'].children[1].hidden));
  chip1 = registry['sources'].children[0];
  check('активный чип подсвечен', chip1._cls.has('active'));
  const chip2 = registry['sources'].children[1];
  calls.length = 0;
  chip2.children[2].onclick();
  await tick();
  check('отмена источника вызвана', calls.some((c) => c[0] === 'CancelSource' && c[1] === 'sx2'));

  console.log('scenario: журнал переживает новую задачу');
  emit('log', '[downloading] A — T1', 'info');
  emit('log', '[error] A — T2: boom', 'err');
  const logCount = registry['log'].children.length;
  check('две строки журнала', logCount === 2, String(logCount));
  check('бейдж ошибок', registry['logBadge'].textContent === '1');
  api.handleTaskAdded(task({ key: 'new', state: 'queued' }));
  check('журнал не очищен', registry['log'].children.length === logCount);
  registry['logClear'].onclick();
  check('журнал очищен', registry['log'].children.length === 0);

  console.log('scenario: кнопки очереди');
  calls.length = 0;
  registry['qStopAll'].onclick();
  registry['qClear'].onclick();
  await tick();
  check('Остановить всё вызвано', calls.some((c) => c[0] === 'CancelAll'));
  check('Очистить завершённые вызвано', calls.some((c) => c[0] === 'ClearFinished'));

  console.log('scenario: multiple links hint');
  registry['link'].value = 'https://a/1 https://b/2';
  dispatch(registry['link'], 'input', {});
  check('подсказка о двух ссылках', registry['multiHint'].hidden === false && registry['multiHint'].textContent === '2 ссылки',
    registry['multiHint'].textContent);
  check('предпросмотр скрыт', registry['previewHint'].hidden === true);

  process.exit(failures ? 1 : 0);
})();
