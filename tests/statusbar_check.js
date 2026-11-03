// Стенд для проверки агрегации статусбара на реальном коде index.html:
// стубит DOM + window.runtime, проигрывает сценарии событий и сверяет %.
const fs = require('fs');
const path = require('path');

const html = fs.readFileSync(path.join(__dirname, '..', 'src', 'frontend', 'dist', 'index.html'), 'utf8');
const script = /<script>([\s\S]*?)<\/script>/.exec(html)[1];

// --- Минимальный DOM-стуб ---
function makeEl(tag) {
  const el = {
    tag, children: [], style: {}, className: '', textContent: '', title: '',
    value: '', checked: false, hidden: false, disabled: false,
    _cls: new Set(),
    _listeners: {},
    classList: {
      add: (c) => el._cls.add(c),
      remove: (c) => el._cls.delete(c),
      contains: (c) => el._cls.has(c),
    },
    appendChild(c) { el.children.push(c); c.parent = el; return c; },
    remove() {
      if (el.parent) el.parent.children = el.parent.children.filter((x) => x !== el);
      if (el.id && registry[el.id] === el) delete registry[el.id];
    },
    addEventListener(type, fn) {
      (el._listeners[type] = el._listeners[type] || []).push(fn);
    },
    removeEventListener(type, fn) {
      el._listeners[type] = (el._listeners[type] || []).filter((x) => x !== fn);
    },
    removeAttribute() {},
    setAttribute() {},
    focus() { document.activeElement = (el.id && registry[el.id]) || el; },
    blur() { if (document.activeElement === el) document.activeElement = null; },
    get innerHTML() { return ''; },
    set innerHTML(v) { el.children = []; },
    get children_map() { return el.children; },
  };
  // children[index] доступ как массив
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
const docListeners = {};
const document = {
  activeElement: null,
  getElementById(id) {
    if (!registry[id]) { registry[id] = makeEl('div'); registry[id].id = id; }
    return registry[id];
  },
  createElement(tag) { return makeEl(tag); },
  addEventListener(type, fn) {
    (docListeners[type] = docListeners[type] || []).push(fn);
  },
  removeEventListener(type, fn) {
    docListeners[type] = (docListeners[type] || []).filter((x) => x !== fn);
  },
};

// --- Стуб window.runtime ---
const handlers = {};
const listeners = {};
const window = {
  runtime: {
    EventsOn: (name, fn) => { handlers[name] = fn; },
  },
  go: { ui: { App: {} } },
  addEventListener() {},
};

// Запускаем скрипт в песочнице.
const ctx = { document, window, requestAnimationFrame: (fn) => fn(), console, Number, Math, Map, Set, String, Array, Object, JSON };
const vm = require('vm');
vm.createContext(ctx);
vm.runInContext(script, ctx);

function emit(name, ...args) {
  if (!handlers[name]) throw new Error('no handler: ' + name);
  handlers[name](...args);
}

// Вызывает слушателей элемента (например, keyup на #link).
function dispatch(el, type, event) {
  const fns = (el._listeners && el._listeners[type]) || [];
  for (const fn of fns) fn(event || {});
}

// --- Помощники чтения статусбара ---
const sb = () => ({
  pct: registry['sbpct'].textContent,
  count: registry['sbcount'].textContent,
  bytes: registry['sbbytes'].textContent,
  width: registry['sbfill'].style.width,
  cls: registry['sbfill'].className,
});

let failures = 0;
function check(name, cond, extra) {
  if (cond) { console.log('  ok - ' + name); }
  else { failures++; console.log('  FAIL - ' + name + (extra ? ' | ' + extra : '')); }
}

// --- Сценарий 0: поле ссылки получает фокус при загрузке страницы ---
console.log('scenario: autofocus on load');
check('поле ссылки в фокусе', document.activeElement === registry['link'],
  document.activeElement && document.activeElement.id);

// --- Сценарий 1: плейлист 20 треков, 10 done + 2 активных по 50% ---
console.log('scenario: playlist 20 tracks');
emit('job-start', 'Микс', 20);
const items = [];
for (let i = 1; i <= 20; i++) items.push(['t' + i, 'A' + i + ' — T' + i]);
emit('tracks', items);
for (let i = 1; i <= 10; i++) emit('track-status', 't' + i, '[done] (FLAC)', 'done');
// два актива: 50% и 25%, размеры известны (в байтах)
const MB = 1048576;
emit('progress', 't11', 'A — T11', 50, 21 * MB, 42 * MB);
emit('progress', 't12', 'A — T12', 25, 10 * MB, 40 * MB);
// остальные queued (доля 0)
emit('progress', 't13', 'A — T13', -1, 5 * MB, -1); // неизвестный размер
let s = sb();
// (10*1 + 0.5 + 0.25 + 0) / 20 = 10.75/20 = 53.75 → 54
check('pct = 54%', s.pct === '54%', JSON.stringify(s));
check('count = только done (10/20)', s.count === '10/20 треков', s.count);
// байты считаются только у треков с известным размером (t11+t12): 31/82 МБ
check('bytes 31.0/82.0 МБ', s.bytes === '31.0/82.0 МБ', s.bytes);

// --- Сценарий 2: неизвестный размер не ломает полосу ---
console.log('scenario: unknown size only');
emit('job-start', '', 1);
emit('tracks', [['s1', 'A — Solo']]);
emit('progress', 's1', 'A — Solo', -1, 12 * 1048576, -1);
s = sb();
check('без байтов', s.bytes === '', s.bytes);
check('pct 0% у неизвестного', s.pct === '0%', s.pct);
emit('track-status', 's1', '[done] (FLAC)', 'done');
s = sb();
check('после done 100%', s.pct === '100%', s.pct);
check('счётчик 1/1', s.count === '1/1 треков', s.count);

// --- Сценарий 3: job-end замораживает на 100% и красит ошибки ---
console.log('scenario: job-end with errors');
emit('job-start', 'Микс', 3);
emit('tracks', [['a', 'A'], ['b', 'B'], ['c', 'C']]);
emit('track-status', 'a', '[done] (FLAC)', 'done');
emit('track-status', 'b', '[error] boom', 'err');
emit('track-status', 'c', '[skip] (already exists)', 'info');
emit('job-end', 1, 1, 1);
s = sb();
check('pct 100%', s.pct === '100%', s.pct);
check('ошибка в счётчике', s.count.includes('ошибок: 1'), s.count);
check('класс err', s.cls === 'err', s.cls);

// --- Сценарий 4: job-end без ошибок → зелёный ---
console.log('scenario: job-end clean');
emit('job-start', 'Микс', 2);
emit('tracks', [['a', 'A'], ['b', 'B']]);
emit('track-status', 'a', '[done] (FLAC)', 'done');
emit('track-status', 'b', '[done] (FLAC)', 'done');
emit('job-end', 2, 0, 0);
s = sb();
check('класс done', s.cls === 'done', s.cls);
check('pct 100%', s.pct === '100%', s.pct);

// --- Сценарий 5: новая job'а чистит предыдущую ---
console.log('scenario: new job resets');
emit('job-start', 'Другой', 1);
emit('tracks', [['z', 'Z']]);
s = sb();
check('pct сброшен', s.pct === '0%', s.pct);
check('счётчик пуст', s.count === '0/1 треков', s.count);

// --- Сценарий 6: остановка — статусбар замораживается, % не тянется к 100 ---
console.log('scenario: stopped job freezes below 100');
emit('job-start', 'Микс', 4);
emit('tracks', [['a', 'A'], ['b', 'B'], ['c', 'C'], ['d', 'D']]);
emit('track-status', 'a', '[done] (FLAC)', 'done');
emit('progress', 'b', 'B', 50, 21 * MB, 42 * MB);
emit('progress', 'c', 'C', 50, 21 * MB, 42 * MB);
emit('track-status', 'c', '[stopped]', 'stopped');
s = sb();
// (1 + 0.5 + 0.5 + 0) / 4 = 50% — [stopped] долю не закрывает.
check('pct до job-end = 50%', s.pct === '50%', s.pct);
emit('job-end', 1, 0, 0, true);
s = sb();
check('pct заморожен на 50%', s.pct === '50%', s.pct);
check('класс stopped', s.cls === 'stopped', s.cls);
check('счётчик с пометкой', s.count === '1/4 треков, остановлено', s.count);
check('ширина полосы = 50%', s.width === '50%', s.width);

// --- Сценарий 7: «Скачать» на время job'ы становится «■ Остановить» ---
(async () => {
  console.log('scenario: download button becomes stop button');
  let resolveDownload = null;
  let stopCalls = 0;
  window.go.ui.App.Download = () => new Promise((r) => { resolveDownload = r; });
  window.go.ui.App.Stop = () => { stopCalls++; return Promise.resolve(); };

  const dl = registry['dl'];
  const linkInput = registry['link'];
  linkInput.value = 'https://music.yandex.ru/track/1';
  const running = dl.onclick();
  check('кнопка = ■ Остановить', dl.classList.contains('stop') && dl.textContent === '■ Остановить', dl.textContent);
  check('⚙ заблокирован на время job\'ы', registry['settingsBtn'].disabled === true);

  const stopping = dl.onclick();
  check('кнопка = Останавливаю…', dl.textContent === 'Останавливаю…' && dl.disabled === true, dl.textContent);
  check('Stop вызван ровно один раз', stopCalls === 1, String(stopCalls));

  resolveDownload('[stopped] прервано пользователем');
  await running;
  await stopping;
  check('кнопка вернулась в «Скачать»', !dl.classList.contains('stop') && dl.textContent === 'Скачать' && !dl.disabled, dl.textContent);
  check('⚙ разблокирован', registry['settingsBtn'].disabled === false);
  check('фокус вернулся в поле после job-end', document.activeElement === linkInput,
    document.activeElement && document.activeElement.id);
  const last = registry['log'].children[registry['log'].children.length - 1];
  check('лог получил [stopped] янтарём', last && last.textContent.startsWith('[stopped]') && last.className === 'stopped',
    last && last.textContent + '/' + last.className);

  // --- Сценарий 8: Enter в поле запускает скачивание ---
  console.log('scenario: Enter starts download');
  let enterCalls = [];
  let stopCallsBusy = 0;
  let resolveEnterDownload = null;
  window.go.ui.App.Download = (link) => {
    enterCalls.push(link);
    return new Promise((r) => { resolveEnterDownload = r; });
  };
  window.go.ui.App.Stop = () => { stopCallsBusy++; return Promise.resolve(); };

  const enterLink = 'https://music.yandex.ru/track/2';
  linkInput.value = enterLink;
  dispatch(linkInput, 'keyup', { key: 'Enter', isComposing: false });
  check('Download вызван один раз', enterCalls.length === 1, String(enterCalls.length));
  check('Download получил ссылку', enterCalls[0] === enterLink, enterCalls[0]);
  check('поле очищено после старта', linkInput.value === '', linkInput.value);
  check('кнопка = ■ Остановить', dl.classList.contains('stop'), dl.textContent);

  // --- Сценарий 9: Enter при busy ничего не делает ---
  console.log('scenario: Enter while busy is ignored');
  const stagedLink = 'https://music.yandex.ru/track/3';
  linkInput.value = stagedLink;
  dispatch(linkInput, 'keyup', { key: 'Enter', isComposing: false });
  check('вторая загрузка не запущена', enterCalls.length === 1, String(enterCalls.length));
  check('Stop не вызван', stopCallsBusy === 0, String(stopCallsBusy));
  check('текст остался в поле', linkInput.value === stagedLink, linkInput.value);

  resolveEnterDownload('');
  await new Promise((r) => setTimeout(r, 0));
  check('после job-end кнопка = «Скачать»', dl.textContent === 'Скачать' && !dl.disabled, dl.textContent);
  check('после job-end фокус в поле', document.activeElement === linkInput,
    document.activeElement && document.activeElement.id);

  // --- Сценарий 10: Enter в пустом поле ничего не делает ---
  console.log('scenario: Enter in empty field');
  enterCalls = [];
  linkInput.value = '';
  dispatch(linkInput, 'keyup', { key: 'Enter', isComposing: false });
  check('Download не вызван', enterCalls.length === 0, String(enterCalls.length));

  // --- Сценарий 11: Enter во время IME не запускает скачивание ---
  console.log('scenario: Enter during IME is ignored');
  linkInput.value = 'https://music.yandex.ru/track/4';
  dispatch(linkInput, 'keyup', { key: 'Enter', isComposing: true });
  check('Download не вызван при IME', enterCalls.length === 0, String(enterCalls.length));
  linkInput.value = '';

  // --- Сценарий 12: фокус возвращается после окна дубля ---
  console.log('scenario: focus returns after duplicate dialog');
  window.go.ui.App.AnswerDuplicate = () => Promise.resolve();
  emit('duplicate-prompt', {
    percent: 90,
    track: 'Artist — Track',
    fileName: 'Artist - Track.flac',
    filePath: 'C:\\Music\\Artist - Track.flac',
    differentFormat: false,
  });
  check('фокус в окне дубля', document.activeElement === registry['dupYes'],
    document.activeElement && document.activeElement.id);
  await ctx.answerDup(false);
  check('после ответа фокус в поле', document.activeElement === linkInput,
    document.activeElement && document.activeElement.id);

  // --- Сценарий 13: фокус возвращается после закрытия настроек ---
  console.log('scenario: focus returns after settings');
  document.activeElement = registry['dl'];
  ctx.closeSettings();
  check('после настроек фокус в поле', document.activeElement === linkInput,
    document.activeElement && document.activeElement.id);

  process.exit(failures ? 1 : 0);
})();
