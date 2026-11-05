(() => {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const events = (name, fn) => {
    if (window.runtime && typeof window.runtime.EventsOn === 'function') {
      window.runtime.EventsOn(name, fn);
    }
  };
  const app = () => (window.go && window.go.ui && window.go.ui.App) || null;
  const callApp = (name, ...args) => {
    const api = app();
    if (!api || typeof api[name] !== 'function') {
      return Promise.reject(new Error('бэкенд ещё не готов'));
    }
    return api[name](...args);
  };
  const hide = (el, value) => { if (el) el.hidden = value; };
  const PLURAL = (n, one, few, many) => {
    const mod10 = n % 10;
    const mod100 = n % 100;
    if (mod10 === 1 && mod100 !== 11) return one;
    if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return few;
    return many;
  };
  const fmtMB = (b) => (Number(b) / 1048576).toFixed(1);
  const fmtBytes = (done, total) => {
    if (Number(total) > 0) return fmtMB(done) + ' / ' + fmtMB(total) + ' МБ';
    return fmtMB(done) + ' МБ';
  };

  // --- Boot splash ---
  const boot = $('boot');
  const reduceMotion = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  const BOOT_MIN_MS = reduceMotion ? 400 : 1000;
  const BOOT_FADE_MS = 160;
  let bootDismissed = false;
  let bootStarted = false;

  function dismissBoot() {
    if (bootDismissed || !boot) return;
    bootDismissed = true;
    if (typeof setTimeout !== 'function') {
      boot.remove();
      return;
    }
    boot.classList.add('boot--done');
    setTimeout(() => boot.remove(), BOOT_FADE_MS);
  }

  function startBoot() {
    if (bootDismissed || bootStarted || !boot) return;
    bootStarted = true;
    if (typeof setTimeout !== 'function') {
      dismissBoot();
      return;
    }
    boot.classList.add('boot--run');
    setTimeout(dismissBoot, BOOT_MIN_MS);
  }

  // --- DOM ---
  const input = $('link');
  const btn = $('dl');
  const dlLabel = $('dlLabel');
  const dlIconUse = $('dlIconUse');
  const pasteBtn = $('pasteBtn');
  const clearBtn = $('clearBtn');
  const multiHint = $('multiHint');
  const previewHint = $('previewHint');
  const previewCover = $('previewCover');
  const previewCoverFallback = $('previewCoverFallback');
  const previewKind = $('previewKind');
  const previewTitle = $('previewTitle');
  const previewSub = $('previewSub');

  const queueSection = $('queueSection');
  const qCounts = $('qCounts');
  const qWorkers = $('qWorkers');
  const qStopAll = $('qStopAll');
  const qClear = $('qClear');
  const qbar = $('qbar');
  const qfill = $('qfill');
  const qPct = $('qPct');
  const qBytes = $('qBytes');
  const sourcesEl = $('sources');
  const tasksEl = $('tasks');
  const progressAnnounce = $('progressAnnounce');
  const toastEl = $('toast');

  const journal = $('journal');
  const log = $('log');
  const logBadge = $('logBadge');
  const logCopy = $('logCopy');
  const logClear = $('logClear');
  const dropOverlay = $('dropOverlay');

  const settingsBtn = $('settingsBtn');
  const settingsBadge = $('settingsBadge');
  const openFolderBtn = $('openFolderBtn');

  const settingsOverlay = $('settingsOverlay');
  const settingsCard = $('settingsCard');
  const settingsClose = $('settingsClose');
  const sToken = $('sToken');
  const sOutput = $('sOutput');
  const sOutputPick = $('sOutputPick');
  const sLibrary = $('sLibrary');
  const sDiffOutput = $('sDiffOutput');
  const sWorkers = $('sWorkers');
  const sMinus = $('sMinus');
  const sPlus = $('sPlus');
  const sConvert = $('sConvert');
  const sEmbedCover = $('sEmbedCover');
  const sEmbedLyrics = $('sEmbedLyrics');
  const sSaveLrc = $('sSaveLrc');
  const sSubdirs = $('sSubdirs');
  const sSubdirTemplate = $('sSubdirTemplate');
  const sExportM3u = $('sExportM3u');
  const sFlacMonitor = $('sFlacMonitor');
  const sQualityStatus = $('sQualityStatus');
  const sCheckNow = $('sCheckNow');
  const sQualityOpen = $('sQualityOpen');
  const sRejectedList = $('sRejectedList');
  const sRejectedEmpty = $('sRejectedEmpty');
  const tokenCheck = $('tokenCheck');
  const settingsError = $('settingsError');
  const tokenHelpLink = $('tokenHelpLink');
  const tokenHint = $('tokenHint');
  const tokenHelpOverlay = $('tokenHelpOverlay');
  const tokenHelpCard = $('tokenHelpCard');
  const tokenHelpClose = $('tokenHelpClose');
  const tokenHelpCloseFooter = $('tokenHelpCloseFooter');
  const tokenHelpOAuth = $('tokenHelpOAuth');
  const sCancel = $('sCancel');
  const sSave = $('sSave');
  const sCheck = $('sCheck');
  const sLibraryPick = $('sLibraryPick');
  const OAUTH_URL = 'https://oauth.yandex.ru/authorize?response_type=token&client_id=23cabbbdc6cd418abb4b39c32c41195d';

  const qualityOverlay = $('qualityOverlay');
  const qualityCard = $('qualityCard');
  const qualitySummary = $('qualitySummary');
  const qualityList = $('qualityList');
  const qualityError = $('qualityError');
  const qualityShowList = $('qualityShowList');
  const qualityLater = $('qualityLater');
  const qualityDownload = $('qualityDownload');
  const qualityClose = $('qualityClose');

  const confirmOverlay = $('confirmOverlay');
  const confirmCard = $('confirmCard');
  const confirmTitle = $('confirmTitle');
  const confirmText = $('confirmText');
  const confirmCancel = $('confirmCancel');
  const confirmOk = $('confirmOk');

  // --- State ---
  const tasks = new Map();       // key -> task
  const rowEls = new Map();      // key -> row element
  let sources = [];
  let snapshot = null;
  let stateName = 'empty';
  let filterSource = '';
  let lastSubmit = '';
  let lastAnnounce = -1;
  let previewSeq = 0;
  let previewTimer = 0;
  let toastTimer = 0;
  let logErrorCount = 0;
  let qualityChecking = false;
  let qualityOutputDir = '';
  let qualityItems = [];
  let qualityListOpen = false;
  let rejectedItems = [];
  let settingsSnapshot = null;
  let confirmAction = null;
  let confirmReturn = null;
  let confirmPreviousFocus = null;
  let activeModal = null;
  let queueReady = false;

  function announce(text) {
    if (!progressAnnounce || !text) return;
    progressAnnounce.textContent = '';
    progressAnnounce.textContent = text;
  }

  function showToast(text, ms) {
    if (!toastEl) return;
    toastEl.textContent = text || '';
    hide(toastEl, false);
    if (typeof clearTimeout === 'function') clearTimeout(toastTimer);
    if (typeof setTimeout === 'function') {
      toastTimer = setTimeout(() => hide(toastEl, true), ms || 4200);
    }
  }

  // --- Journal ---
  function updateLogBadge() {
    if (!logBadge) return;
    logBadge.textContent = String(logErrorCount);
    logBadge.hidden = logErrorCount === 0;
  }

  function appendLog(text, cls) {
    if (!log) return;
    const div = document.createElement('div');
    if (cls) div.className = cls;
    div.textContent = text;
    log.appendChild(div);
    if (log.children.length > 500 && log.removeChild && log.firstChild) {
      log.removeChild(log.firstChild);
    }
    if (cls === 'err') {
      logErrorCount++;
      updateLogBadge();
    }
    if (journal && journal.open && typeof log.scrollHeight === 'number') {
      log.scrollTop = log.scrollHeight;
    }
  }

  // --- Formatting ---
  function shortSource(label) {
    const s = String(label || '');
    const i = s.indexOf(' · ');
    return i >= 0 ? s.slice(i + 3) : s;
  }

  function reasonShort(reason) {
    const r = String(reason || '');
    if (r === 'дубль, отменено пользователем') return 'дубль';
    return r;
  }

  function stateText(t) {
    switch (t.state) {
      case 'resolving': return 'определяется…';
      case 'queued': return 'в очереди' + (t.position > 0 ? ' #' + t.position : '');
      case 'downloading': return t.percent >= 0 ? 'скачивается ' + t.percent + '%' : 'скачивается';
      case 'needs-review': return 'возможен дубль' + (t.candidate ? ' ' + t.candidate.percent + '%' : '');
      case 'done': return 'готово';
      case 'skipped': return 'пропущено' + (reasonShort(t.reason) ? ' · ' + reasonShort(t.reason) : '');
      case 'error': return 'ошибка';
      case 'canceled': return 'отменено';
      default: return 'ожидание';
    }
  }

  function stateColor(t) {
    switch (t.state) {
      case 'done': return 'status-done';
      case 'error': return 'status-err';
      case 'canceled': return 'status-stopped';
      case 'needs-review': return 'status-review';
      case 'downloading': return 'status-info';
      default: return '';
    }
  }

  function iconButton(iconName, label, handler) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'btn btn-ghost btn-sm icon-btn';
    b.title = label;
    b.setAttribute('aria-label', label);
    const svg = document.createElementNS ? document.createElementNS('http://www.w3.org/2000/svg', 'svg') : document.createElement('svg');
    svg.setAttribute('class', 'icon');
    const use = document.createElementNS ? document.createElementNS('http://www.w3.org/2000/svg', 'use') : document.createElement('use');
    use.setAttribute('href', '#icon-' + iconName);
    svg.appendChild(use);
    b.appendChild(svg);
    b.onclick = (e) => {
      if (e && e.stopPropagation) e.stopPropagation();
      handler();
    };
    return b;
  }

  function textButton(label, cls, handler) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'btn ' + cls + ' btn-sm';
    b.textContent = label;
    b.onclick = (e) => {
      if (e && e.stopPropagation) e.stopPropagation();
      handler();
    };
    return b;
  }

  function call(name, ...args) {
    callApp(name, ...args).catch((e) => {
      appendLog('[error] ' + e, 'err');
      showToast(String(e));
    });
  }

  // --- Task rows ---
  function makeRow(t) {
    const row = document.createElement('div');
    row.className = 'task-row';
    if (!row.dataset) row.dataset = {};
    row.dataset.key = t.key;

    const cover = document.createElement('span');
    cover.className = 'task-cover';
    const coverIcon = document.createElement('svg');
    coverIcon.setAttribute('class', 'icon');
    const use = document.createElement('use');
    use.setAttribute('href', '#icon-vinyl');
    coverIcon.appendChild(use);
    cover.appendChild(coverIcon);

    const main = document.createElement('div');
    main.className = 'task-main';
    const line = document.createElement('div');
    line.className = 'task-line';
    const title = document.createElement('span');
    title.className = 'task-title';
    const source = document.createElement('span');
    source.className = 'task-source';
    const pill = document.createElement('span');
    pill.className = 'format-pill';
    pill.hidden = true;
    line.appendChild(title);
    line.appendChild(source);
    line.appendChild(pill);

    const meta = document.createElement('div');
    meta.className = 'task-meta-line';
    const state = document.createElement('span');
    state.className = 'task-state';
    const size = document.createElement('span');
    size.className = 'task-size';
    const reason = document.createElement('span');
    reason.className = 'task-reason';
    meta.appendChild(state);
    meta.appendChild(size);
    meta.appendChild(reason);

    main.appendChild(line);
    main.appendChild(meta);

    const actions = document.createElement('div');
    actions.className = 'task-actions';

    const dup = document.createElement('div');
    dup.className = 'task-dup';
    dup.hidden = true;
    const dupPath = document.createElement('div');
    dupPath.className = 'dup-path';
    const dupActions = document.createElement('div');
    dupActions.className = 'dup-actions';
    dup.appendChild(dupPath);
    dup.appendChild(dupActions);

    const bar = document.createElement('div');
    bar.className = 'task-progress';
    bar.hidden = true;
    const fill = document.createElement('div');
    fill.className = 'task-fill';
    bar.appendChild(fill);

    row.appendChild(cover);
    row.appendChild(main);
    row.appendChild(actions);
    row.appendChild(dup);
    row.appendChild(bar);
    row._refs = { main, line, title, source, pill, meta, state, size, reason, actions, dup, dupPath, dupActions, bar, fill };
    row.dataset.state = '';
    return row;
  }

  function ensureRow(t) {
    let row = rowEls.get(t.key);
    if (!row) {
      row = makeRow(t);
      rowEls.set(t.key, row);
      if (tasksEl) tasksEl.appendChild(row);
    }
    return row;
  }

  function renderTaskActions(row, t) {
    const refs = row._refs;
    const actions = refs.actions;
    actions.innerHTML = '';
    if (t.state === 'downloading' || t.state === 'queued' || t.state === 'resolving') {
      actions.appendChild(iconButton('x', 'Отменить задачу', () => call('CancelTask', t.key)));
      return;
    }
    if (t.state === 'error' || t.state === 'canceled') {
      actions.appendChild(iconButton('refresh', 'Повторить', () => call('RetryTask', t.key)));
      actions.appendChild(iconButton('x', 'Убрать строку', () => call('RemoveTask', t.key)));
      return;
    }
    if (t.state === 'done' || t.state === 'skipped') {
      actions.appendChild(iconButton('folder', 'Открыть папку', () => openFolder()));
      actions.appendChild(iconButton('x', 'Убрать строку', () => call('RemoveTask', t.key)));
    }
  }

  function updateTaskRow(t) {
    const row = ensureRow(t);
    const refs = row._refs;
    row.dataset.state = t.state || '';
    row.dataset.source = t.sourceKey || '';

    refs.title.textContent = t.label || 'Задача';
    refs.title.title = t.label || '';
    refs.source.textContent = shortSource(t.sourceLabel);
    refs.source.hidden = sources.length < 2;

    // Формат-пилюля рядом с названием.
    const pill = refs.pill;
    if (pill) {
      const fmt = t.state === 'done' && t.format ? String(t.format).toUpperCase() : '';
      pill.hidden = fmt === '';
      if (fmt) {
        pill.textContent = fmt;
        pill.className = 'format-pill ' + fmt.toLowerCase();
        if (t.fallbackReason) pill.title = t.fallbackReason;
      }
    }

    refs.state.textContent = stateText(t);
    refs.state.className = 'task-state ' + stateColor(t);

    let sizeText = '';
    if (t.state === 'downloading') sizeText = fmtBytes(t.done, t.total);
    else if (t.state === 'done' && t.total > 0) sizeText = fmtMB(t.total) + ' МБ';
    refs.size.textContent = sizeText;
    refs.size.hidden = sizeText === '';

    let reasonText = '';
    if (t.state === 'error') reasonText = reasonShort(t.reason);
    else if (t.state === 'skipped' && t.reason && t.reason !== 'уже есть') reasonText = '';
    refs.reason.textContent = reasonText;
    refs.reason.title = reasonText;
    refs.reason.hidden = reasonText === '';

    // Прогресс-полоса: только активная задача и отменённая (замороженный прогресс).
    const showBar = t.state === 'downloading' || t.state === 'canceled';
    refs.bar.hidden = !showBar;
    if (showBar) {
      let pct = Number(t.percent);
      const indeterminate = t.state === 'downloading' && !(Number(t.total) > 0);
      refs.fill.style.width = indeterminate ? '100%' : Math.max(0, Math.min(100, pct > 0 ? pct : 0)) + '%';
      refs.fill.className = 'task-fill';
      if (t.state === 'canceled') refs.fill.classList.add('stopped');
      if (indeterminate) refs.fill.classList.add('indeterminate');
    }

    // Дубль: решение прямо в строке.
    const showDup = t.state === 'needs-review' && t.candidate;
    refs.dup.hidden = !showDup;
    if (showDup) {
      const c = t.candidate;
      refs.dupPath.textContent = c.filePath || c.fileName || '';
      refs.dupActions.innerHTML = '';
      refs.dupActions.appendChild(textButton('Пропустить', 'btn-secondary', () => call('ResolveDuplicate', t.key, false)));
      refs.dupActions.appendChild(textButton('Скачать всё равно', 'btn-primary', () => call('ResolveDuplicate', t.key, true)));
    }

    const actionsKey = t.state;
    if (row.dataset.actionsState !== actionsKey) {
      row.dataset.actionsState = actionsKey;
      renderTaskActions(row, t);
    }

    if (filterSource && t.sourceKey !== filterSource) row.hidden = true;
    else row.hidden = false;
  }

  function removeRow(key) {
    const row = rowEls.get(key);
    if (!row) return;
    if (typeof row.remove === 'function') row.remove();
    else if (row.parent && row.parent.removeChild) row.parent.removeChild(row);
    rowEls.delete(key);
    tasks.delete(key);
  }

  function orderRows(list) {
    if (!tasksEl) return;
    const keys = list.map((t) => t.key);
    const current = Array.from(tasksEl.children || []);
    let same = current.length === keys.length;
    if (same) {
      for (let i = 0; i < keys.length; i++) {
        if (!current[i] || !current[i].dataset || current[i].dataset.key !== keys[i]) { same = false; break; }
      }
    }
    if (same) return;
    for (const key of keys) {
      const row = rowEls.get(key);
      if (!row) continue;
      if (typeof row.remove === 'function') row.remove();
      else if (row.parent && row.parent.removeChild) row.parent.removeChild(row);
      tasksEl.appendChild(row);
    }
  }

  // --- Sources ---
  function renderSources() {
    if (!sourcesEl) return;
    sourcesEl.innerHTML = '';
    if (!sources.length || sources.length < 2) {
      sourcesEl.hidden = true;
      return;
    }
    sourcesEl.hidden = false;
    for (const src of sources) {
      let total = 0;
      let finished = 0;
      for (const t of tasks.values()) {
        if (t.sourceKey !== src.key || t.state === 'resolving') continue;
        total++;
        if (t.state === 'done' || t.state === 'skipped' || t.state === 'error' || t.state === 'canceled') finished++;
      }
      const chip = document.createElement('div');
      chip.className = 'source-chip';
      if (filterSource === src.key) chip.classList.add('active');
      const label = document.createElement('button');
      label.type = 'button';
      label.className = 'source-label';
      label.textContent = src.label || src.link || 'Источник';
      label.title = src.link || src.label || '';
      label.onclick = () => {
        filterSource = filterSource === src.key ? '' : src.key;
        renderSources();
        applyFilter();
      };
      const count = document.createElement('span');
      count.className = 'source-count';
      count.textContent = finished + '/' + total;
      const x = document.createElement('button');
      x.type = 'button';
      x.className = 'source-x';
      x.setAttribute('aria-label', 'Отменить все задачи источника');
      x.title = 'Отменить все задачи источника';
      const xi = document.createElement('svg');
      xi.setAttribute('class', 'icon');
      const xu = document.createElement('use');
      xu.setAttribute('href', '#icon-x');
      xi.appendChild(xu);
      x.appendChild(xi);
      x.onclick = (e) => {
        if (e && e.stopPropagation) e.stopPropagation();
        call('CancelSource', src.key);
      };
      chip.appendChild(label);
      chip.appendChild(count);
      chip.appendChild(x);
      sourcesEl.appendChild(chip);
    }
  }

  function applyFilter() {
    for (const [key, row] of rowEls.entries()) {
      const t = tasks.get(key);
      row.hidden = !!(filterSource && t && t.sourceKey !== filterSource);
    }
  }

  // --- Queue bar and screen state ---
  function renderBar(snap) {
    if (!snap) return;
    if (qPct) qPct.textContent = (snap.percent || 0) + '%';
    if (qfill) qfill.style.width = Math.max(0, Math.min(100, snap.percent || 0)) + '%';
    if (qbar) qbar.setAttribute('aria-valuenow', String(snap.percent || 0));
    if (qBytes) {
      qBytes.textContent = snap.bytesTotal > 0 ? fmtBytes(snap.bytesDone, snap.bytesTotal) : '';
    }
    if (qfill) {
      qfill.className = 'qfill';
      const settled = !snap.active && !snap.waiting && !snap.review;
      if (settled && snap.failed > 0) qfill.classList.add('err');
      else if (settled && snap.canceled > 0) qfill.classList.add('stopped');
      else if (settled) qfill.classList.add('done');
    }
    if (qCounts) {
      qCounts.innerHTML = '';
      const parts = [
        ['qc-active', snap.active, 'качается', 'качаются', 'качается'],
        ['qc-wait', snap.waiting, 'в очереди', 'в очереди', 'в очереди'],
        ['qc-review', snap.review, 'ждёт решения', 'ждут решения', 'ждут решения'],
        ['qc-done', snap.done, 'готово', 'готово', 'готово'],
        ['qc-error', snap.failed, 'ошибка', 'ошибки', 'ошибок'],
        ['qc-error', snap.canceled, 'отменено', 'отменено', 'отменено'],
      ];
      for (const [cls, n, one, few, many] of parts) {
        if (!n) continue;
        const span = document.createElement('span');
        span.className = cls;
        const word = n === 1 ? one : PLURAL(n, one, few, many);
        span.textContent = n + ' ' + word;
        qCounts.appendChild(span);
      }
    }
    if (qWorkers) {
      const n = snap.workers || 0;
      qWorkers.textContent = n > 0 ? 'потоков: ' + n : '';
      qWorkers.title = 'Сколько треков качать одновременно; меняется в «Настройках»';
    }
    if (qStopAll) qStopAll.hidden = !(snap.active || snap.waiting || snap.review);
    if (qClear) {
      const finished = (snap.done || 0) + (snap.skipped || 0) + (snap.failed || 0) + (snap.canceled || 0);
      qClear.hidden = finished === 0;
    }
    if (qPct) qPct.textContent = (snap.percent || 0) + '%';
  }

  function computeState(snap) {
    if (!snap || !snap.total) return 'empty';
    if (snap.active || snap.waiting || snap.review || (snap.tasks || []).some((t) => t.state === 'resolving')) return 'working';
    return 'settled';
  }

  function applyState(snap) {
    const next = computeState(snap);
    stateName = next;
    if (document.body && document.body.setAttribute) {
      document.body.setAttribute('data-state', next);
    }
    if (queueSection) queueSection.hidden = next === 'empty';
    if (next === 'empty' && filterSource) filterSource = '';
  }

  function syncSnapshot(snap) {
    if (!snap || typeof snap !== 'object') return;
    snapshot = snap;
    if (Array.isArray(snap.sources)) sources = snap.sources;
    const list = Array.isArray(snap.tasks) ? snap.tasks : [];
    const seen = new Set();
    for (const t of list) {
      tasks.set(t.key, t);
      ensureRow(t);
      seen.add(t.key);
    }
    for (const key of Array.from(rowEls.keys())) {
      if (!seen.has(key)) removeRow(key);
    }
    for (const t of list) updateTaskRow(t);
    orderRows(list);
    renderSources();
    renderBar(snap);
    applyState(snap);
    announceProgress(snap);
  }

  function announceProgress(snap) {
    const pct = snap.percent || 0;
    const milestone = pct >= 100 ? 100 : pct >= 75 ? 75 : pct >= 50 ? 50 : pct >= 25 ? 25 : -1;
    if (milestone > lastAnnounce) {
      lastAnnounce = milestone;
      if (milestone === 100) {
        announce('Очередь завершена: готово ' + snap.done + ', ошибок ' + snap.failed + ', пропущено ' + snap.skipped + '.');
      } else if (milestone > 0) {
        announce('Загрузка ' + milestone + ' процентов.');
      }
    }
  }

  // --- Event handlers (also used by the test harness) ---
  function handleTaskAdded(t) {
    if (!t || !t.key) return;
    tasks.set(t.key, t);
    ensureRow(t);
    updateTaskRow(t);
    renderSources();
  }

  function handleTaskUpdated(t) {
    if (!t || !t.key) return;
    tasks.set(t.key, t);
    updateTaskRow(t);
    renderSources();
  }

  function handleTaskRemoved(key) {
    removeRow(key);
    renderSources();
  }

  function handleSources(list) {
    sources = Array.isArray(list) ? list : [];
    renderSources();
    for (const [key, row] of rowEls.entries()) {
      const t = tasks.get(key);
      if (!t) continue;
      const src = t.sourceLabel ? shortSource(t.sourceLabel) : '';
      if (row._refs && row._refs.source) {
        row._refs.source.textContent = src;
        row._refs.source.hidden = sources.length < 2;
      }
    }
    applyFilter();
  }

  function handleQueue(snap) {
    queueReady = true;
    snapshot = snap;
    if (Array.isArray(snap.sources)) sources = snap.sources;
    const list = Array.isArray(snap.tasks) ? snap.tasks : [];
    for (const t of list) {
      const prev = tasks.get(t.key);
      tasks.set(t.key, t);
      if (!prev) {
        ensureRow(t);
        updateTaskRow(t);
      } else if (prev.state !== t.state) {
        updateTaskRow(t);
      } else if (t.state === 'queued') {
        const row = rowEls.get(t.key);
        if (row && row._refs) row._refs.state.textContent = stateText(t);
      }
    }
    orderRows(list);
    renderSources();
    renderBar(snap);
    applyState(snap);
    announceProgress(snap);
  }

  events('task-added', handleTaskAdded);
  events('task-updated', handleTaskUpdated);
  events('task-removed', handleTaskRemoved);
  events('sources', handleSources);
  events('queue', handleQueue);
  events('existing', (trackId) => {
    showToast('Этот трек уже в очереди — вторая загрузка не создана.');
    for (const [key, t] of tasks.entries()) {
      if (t.trackId !== trackId) continue;
      const row = rowEls.get(key);
      if (!row) continue;
      row.classList.add('flash');
      if (typeof setTimeout === 'function') setTimeout(() => row.classList.remove('flash'), 1200);
    }
  });
  events('log', (text, kind) => {
    appendLog(text, kind);
    if (kind === 'err') showToast(String(text).replace(/^\[error\]\s*/, ''));
  });

  // --- Preview hint ---
  function hidePreview() {
    previewSeq++;
    hide(previewHint, true);
    if (previewCover) previewCover.removeAttribute('src');
    if (previewTitle) previewTitle.textContent = '';
    if (previewSub) previewSub.textContent = '';
    if (previewKind) previewKind.textContent = '';
  }

  function showPreview(p) {
    if (!previewHint) return;
    if (previewKind) {
      previewKind.textContent = p.Kind === 'album' ? 'Альбом' : p.Kind === 'playlist' ? 'Плейлист' : 'Трек';
    }
    if (previewTitle) previewTitle.textContent = p.Title || '';
    if (previewSub) previewSub.textContent = p.Subtitle || '';
    if (previewCover && previewCoverFallback) {
      if (p.CoverURL) {
        previewCover.src = p.CoverURL;
        previewCover.hidden = false;
        previewCoverFallback.hidden = true;
      } else {
        previewCover.removeAttribute('src');
        previewCover.hidden = true;
        previewCoverFallback.hidden = false;
      }
    }
    hide(previewHint, false);
  }

  async function updatePreview() {
    const link = input ? input.value.trim() : '';
    if (!link || /\s/.test(link)) {
      hidePreview();
      return;
    }
    const seq = ++previewSeq;
    try {
      const p = await callApp('PreviewLink', link);
      if (seq !== previewSeq) return;
      if (!p || !p.Title) {
        hidePreview();
        return;
      }
      showPreview(p);
    } catch (e) {
      if (seq === previewSeq) hidePreview();
    }
  }

  function clearPreviewTimer() {
    if (typeof clearTimeout === 'function') clearTimeout(previewTimer);
  }

  function schedulePreview() {
    if (typeof setTimeout !== 'function') return;
    previewTimer = setTimeout(updatePreview, 500);
  }

  function linkList() {
    const value = input ? input.value : '';
    return value.trim() ? value.trim().split(/\s+/).filter(Boolean) : [];
  }

  function updateInputChrome() {
    const value = input ? input.value : '';
    const links = linkList();
    if (clearBtn) clearBtn.hidden = value.length === 0;
    if (multiHint) {
      multiHint.textContent = links.length > 1
        ? links.length + ' ' + PLURAL(links.length, 'ссылка', 'ссылки', 'ссылок')
        : '';
      multiHint.hidden = links.length <= 1;
    }
    if (links.length > 1) hidePreview();
    return links;
  }

  // --- Submit ---
  async function submitQueue() {
    const raw = input ? input.value.trim() : '';
    if (!raw) return;
    clearPreviewTimer();
    try {
      const res = await callApp('Submit', raw);
      if (res) {
        appendLog(res, res.indexOf('[error]') === 0 ? 'err' : 'info');
        showToast(String(res).replace(/^\[error\]\s*/, ''));
        return;
      }
      lastSubmit = raw;
      if (input) input.value = '';
      updateInputChrome();
      hidePreview();
      if (input) input.focus();
    } catch (e) {
      appendLog('[error] ' + e, 'err');
      showToast(String(e));
    }
  }

  if (btn) btn.onclick = () => submitQueue();

  if (input) {
    input.addEventListener('keydown', (e) => {
      if (e.key !== 'Enter' || e.isComposing) return;
      e.preventDefault();
      submitQueue();
    });
    input.addEventListener('input', () => {
      const links = updateInputChrome();
      clearPreviewTimer();
      if (links.length === 1) schedulePreview();
      else hidePreview();
    });
  }

  if (clearBtn) {
    clearBtn.onclick = () => {
      if (!input) return;
      input.value = '';
      input.focus();
      updateInputChrome();
      hidePreview();
    };
  }

  if (pasteBtn) {
    pasteBtn.onclick = async () => {
      try {
        let text = '';
        if (window.runtime && typeof window.runtime.ClipboardGetText === 'function') {
          text = await window.runtime.ClipboardGetText();
        } else if (typeof navigator !== 'undefined' && navigator.clipboard) {
          text = await navigator.clipboard.readText();
        }
        text = String(text || '').trim();
        if (!text) return;
        if (input) input.value = text;
        updateInputChrome();
        if (input) input.focus();
        await updatePreview();
      } catch (e) {
        appendLog('[error] не удалось прочитать буфер обмена: ' + e, 'err');
      }
    };
  }

  async function openFolder() {
    try {
      await callApp('OpenOutputDir');
    } catch (e) {
      appendLog('[error] ' + e, 'err');
      showToast(String(e));
    }
  }
  if (openFolderBtn) openFolderBtn.onclick = openFolder;

  if (qStopAll) qStopAll.onclick = () => call('CancelAll');
  if (qClear) qClear.onclick = () => call('ClearFinished');

  // --- Drag & Drop ---
  function useDroppedLink(e) {
    const dt = e.dataTransfer;
    if (!dt) return false;
    const text = dt.getData('text/uri-list') || dt.getData('text/plain');
    const first = String(text || '').split(/\r?\n/).map((s) => s.trim()).filter(Boolean)[0];
    if (!first) return false;
    if (input) input.value = first;
    if (input) input.focus();
    updateInputChrome();
    updatePreview();
    return true;
  }

  if (input) {
    input.addEventListener('dragover', (e) => { e.preventDefault(); });
    input.addEventListener('drop', (e) => {
      e.preventDefault();
      e.stopPropagation();
      hide(dropOverlay, true);
      useDroppedLink(e);
    });
  }
  if (typeof window.addEventListener === 'function') {
    window.addEventListener('dragover', (e) => {
      e.preventDefault();
      hide(dropOverlay, false);
    });
    window.addEventListener('dragleave', (e) => {
      if (e.relatedTarget === null) hide(dropOverlay, true);
    });
    window.addEventListener('drop', (e) => {
      e.preventDefault();
      hide(dropOverlay, true);
      useDroppedLink(e);
    });
  }

  if (journal) {
    journal.addEventListener('toggle', () => {
      if (journal.open && log && typeof log.scrollHeight === 'number') {
        log.scrollTop = log.scrollHeight;
      }
    });
  }
  if (logClear) {
    logClear.onclick = () => {
      if (log) log.innerHTML = '';
      logErrorCount = 0;
      updateLogBadge();
    };
  }
  if (logCopy) {
    logCopy.onclick = async () => {
      const text = log ? (log.innerText || log.textContent || '') : '';
      try {
        if (typeof navigator !== 'undefined' && navigator.clipboard) {
          await navigator.clipboard.writeText(text);
          logCopy.title = 'Скопировано';
        }
      } catch (e) {
        appendLog('[error] не удалось скопировать лог: ' + e, 'err');
      }
    };
  }

  // --- Quality monitor ---
  function setQualityStatus(text) {
    if (sQualityStatus) sQualityStatus.textContent = text || 'FLAC: проверок не было';
  }

  function syncQualityOpen(n) {
    if (!sQualityOpen) return;
    sQualityOpen.hidden = n <= 0;
    sQualityOpen.textContent = 'Открыть найденное (' + n + ')';
  }

  function updateQualityBadge(n) {
    const count = Number(n) || 0;
    if (settingsBadge) {
      settingsBadge.hidden = count <= 0;
      settingsBadge.textContent = String(count);
    }
    if (settingsBtn) {
      const label = count > 0 ? 'Настройки, найдено FLAC: ' + count : 'Настройки';
      settingsBtn.title = label;
      if (settingsBtn.setAttribute) settingsBtn.setAttribute('aria-label', label);
    }
    syncQualityOpen(count);
  }

  function qualityRow(it) {
    const row = document.createElement('div');
    row.className = 'qrow';
    const label = document.createElement('label');
    label.className = 'qrow-main';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = !!it.selected;
    if (!cb.dataset) cb.dataset = {};
    cb.dataset.key = it.key;
    const main = document.createElement('span');
    main.className = 'qmain';
    const name = document.createElement('div');
    name.className = 'qname';
    name.textContent = it.label || it.fileName || it.path;
    const meta = document.createElement('div');
    meta.className = 'qmeta';
    meta.textContent = it.alreadyFlac
      ? 'MP3 → _old (FLAC уже есть)'
      : 'MP3 → ' + (it.foundFormat || 'FLAC');
    main.appendChild(name);
    main.appendChild(meta);
    if (it.matchSource === 'search') {
      const match = document.createElement('div');
      match.className = 'qmatch';
      const bits = [];
      if (it.matchedArtist) bits.push(it.matchedArtist);
      if (it.matchedTitle) bits.push(it.matchedTitle);
      let text = 'Найдено по названию: ' + bits.join(' — ');
      if (it.album) text += ' · ' + it.album;
      if (it.year) text += ' (' + it.year + ')';
      if (it.confidence) text += ' · ' + it.confidence + '%';
      match.textContent = text;
      main.appendChild(match);
    }
    label.appendChild(cb);
    label.appendChild(main);
    row.appendChild(label);
    if (it.matchSource === 'search' && !it.alreadyFlac) {
      const reject = document.createElement('button');
      reject.type = 'button';
      reject.className = 'btn btn-ghost btn-sm qreject';
      reject.textContent = 'Не тот трек';
      reject.title = 'Запомнить, что это не тот трек';
      reject.disabled = qualityChecking;
      reject.onclick = (e) => {
        e.preventDefault();
        e.stopPropagation();
        return rejectQualityItem(it);
      };
      row.appendChild(reject);
    }
    return row;
  }

  async function rejectQualityItem(it) {
    if (qualityChecking) {
      if (qualityError) qualityError.textContent = 'Идёт проверка — дождитесь завершения';
      return;
    }
    if (qualityError) qualityError.textContent = '';
    const index = qualityItems.findIndex((x) => x.key === it.key);
    try {
      const remaining = await callApp('RejectFound', it);
      qualityItems = Array.isArray(remaining) ? remaining : [];
      updateQualityBadge(qualityItems.length);
      refreshRejectedList();
      if (qualityItems.length === 0) {
        closeQuality();
        return;
      }
      renderQualityList();
      updateQualitySummary();
      focusQualityRow(Math.max(0, Math.min(index, qualityItems.length - 1)));
    } catch (e) {
      if (qualityError) qualityError.textContent = String(e);
    }
  }

  function focusQualityRow(index) {
    if (!qualityList || !qualityList.children || qualityList.children.length === 0) return;
    const row = qualityList.children[Math.max(0, Math.min(index, qualityList.children.length - 1))];
    if (!row || !row.children || row.children.length === 0) return;
    let target = row.children[row.children.length - 1];
    if (target && target.className === 'qrow-main' && target.children && target.children[0]) {
      target = target.children[0];
    }
    if (target && typeof target.focus === 'function') target.focus();
  }

  function renderRejectedList(items) {
    rejectedItems = Array.isArray(items) ? items.slice() : [];
    if (sRejectedEmpty) sRejectedEmpty.hidden = rejectedItems.length > 0;
    if (!sRejectedList) return;
    sRejectedList.innerHTML = '';
    sRejectedList.hidden = rejectedItems.length === 0;
    rejectedItems.forEach((it) => {
      const row = document.createElement('div');
      row.className = 'rejected-row';
      const main = document.createElement('div');
      main.className = 'rejected-main';
      const local = document.createElement('div');
      local.className = 'rejected-local';
      local.textContent = it.localLabel || it.fileName || it.path || '';
      const cand = document.createElement('div');
      cand.className = 'rejected-cand';
      const bits = [];
      if (it.matchedArtist) bits.push(it.matchedArtist);
      if (it.matchedTitle) bits.push(it.matchedTitle);
      let text = 'Найдено: ' + (bits.length ? bits.join(' — ') : ('ID ' + (it.trackId || '')));
      if (it.album) text += ' · ' + it.album;
      if (it.year) text += ' (' + it.year + ')';
      if (it.confidence) text += ' · ' + it.confidence + '%';
      cand.textContent = text;
      main.appendChild(local);
      main.appendChild(cand);
      if (it.declinedAtText) {
        const when = document.createElement('div');
        when.className = 'rejected-date';
        when.textContent = it.declinedAtText;
        main.appendChild(when);
      }
      const undo = document.createElement('button');
      undo.type = 'button';
      undo.className = 'btn btn-secondary btn-sm';
      undo.textContent = 'Вернуть';
      undo.disabled = qualityChecking;
      undo.onclick = () => undoRejected(it);
      row.appendChild(main);
      row.appendChild(undo);
      sRejectedList.appendChild(row);
    });
  }

  function refreshRejectedList() {
    callApp('GetSettings')
      .then((s) => {
        if (s && Array.isArray(s.QualityRejected)) renderRejectedList(s.QualityRejected);
      })
      .catch(() => {
        // The settings modal will refetch the list when it opens.
      });
  }

  async function undoRejected(it) {
    if (qualityChecking) {
      if (settingsError) settingsError.textContent = 'Идёт проверка — дождитесь завершения';
      return;
    }
    if (settingsError) settingsError.textContent = '';
    try {
      const found = await callApp('UndoReject', it.path, it.trackId);
      renderQuality(Array.isArray(found) ? found : []);
      const index = rejectedItems.findIndex((x) => x.path === it.path && x.trackId === it.trackId);
      if (index >= 0) rejectedItems.splice(index, 1);
      renderRejectedList(rejectedItems);
    } catch (e) {
      if (settingsError) settingsError.textContent = String(e);
    }
  }

  function renderQualityList() {
    if (!qualityList) return;
    qualityList.innerHTML = '';
    qualityItems.forEach((it) => qualityList.appendChild(qualityRow(it)));
  }

  function updateQualitySummary() {
    if (!qualitySummary) return;
    const ids = qualityItems.filter((x) => x.matchSource === 'id' || x.alreadyFlac).length;
    const searches = qualityItems.length - ids;
    let text = 'Найдено ' + qualityItems.length + ' ' +
      PLURAL(qualityItems.length, 'трек', 'трека', 'треков') + ' в лучшем качестве.';
    if (searches > 0) {
      text += ' Из них ' + searches + ' — по названию, проверь совпадения.';
    }
    if (qualityOutputDir) {
      text += ' Новые FLAC — в ' + qualityOutputDir + '; старые MP3 — в _old.';
    }
    qualitySummary.textContent = text;
  }

  function renderQuality(items) {
    qualityItems = Array.isArray(items) ? items : [];
    updateQualityBadge(qualityItems.length);
    if (qualityOverlay && qualityOverlay.classList.contains('open')) {
      renderQualityList();
      updateQualitySummary();
    }
  }

  function openQuality() {
    if (!qualityOverlay || qualityItems.length === 0) return;
    if (qualityError) qualityError.textContent = '';
    const needsReview = qualityItems.some((it) => !it.selected);
    qualityListOpen = needsReview;
    if (qualityList) qualityList.hidden = !qualityListOpen;
    if (qualityShowList) qualityShowList.textContent = qualityListOpen ? 'Скрыть список' : 'Показать список';
    if (qualityDownload) qualityDownload.textContent = qualityListOpen ? 'Скачать выбранные' : 'Скачать все';
    renderQualityList();
    updateQualitySummary();
    qualityOverlay.classList.add('open');
    qualityOverlay.setAttribute('aria-hidden', 'false');
    activeModal = 'quality';
    if (qualityDownload) qualityDownload.focus();
  }

  function closeQuality() {
    if (!qualityOverlay) return;
    qualityOverlay.classList.remove('open');
    qualityOverlay.setAttribute('aria-hidden', 'true');
    const settingsOpen = settingsOverlay && settingsOverlay.classList.contains('open');
    activeModal = settingsOpen ? 'settings' : null;
    if (settingsOpen) {
      const target = (sQualityOpen && !sQualityOpen.hidden) ? sQualityOpen : sCheckNow;
      if (target) target.focus();
    } else if (input) {
      input.focus();
    }
  }

  function selectedQualityItems() {
    if (!qualityListOpen) return qualityItems.filter((it) => it.selected);
    if (!qualityList || typeof qualityList.querySelectorAll !== 'function') return qualityItems.filter((it) => it.selected);
    const checked = new Set();
    qualityList.querySelectorAll('input[type=checkbox]').forEach((cb) => {
      if (cb.checked) checked.add(cb.dataset.key);
    });
    return qualityItems.filter((it) => checked.has(it.key));
  }

  if (qualityShowList) {
    qualityShowList.onclick = () => {
      qualityListOpen = !qualityListOpen;
      if (qualityList) qualityList.hidden = !qualityListOpen;
      qualityShowList.textContent = qualityListOpen ? 'Скрыть список' : 'Показать список';
      if (qualityDownload) qualityDownload.textContent = qualityListOpen ? 'Скачать выбранные' : 'Скачать все';
    };
  }
  if (qualityLater) qualityLater.onclick = closeQuality;
  if (qualityClose) qualityClose.onclick = closeQuality;
  if (qualityOverlay) {
    qualityOverlay.onclick = (e) => { if (e.target === qualityOverlay) closeQuality(); };
  }

  if (qualityDownload) {
    qualityDownload.onclick = async () => {
      const selected = selectedQualityItems();
      if (selected.length === 0) {
        if (qualityError) qualityError.textContent = 'Выбери хотя бы один трек';
        return;
      }
      const settingsOpen = settingsOverlay && settingsOverlay.classList.contains('open');
      closeQuality();
      if (settingsOpen) closeSettings(true);
      try {
        await callApp('DownloadFound', selected);
        showToast('Обновление добавлено в очередь.');
      } catch (e) {
        appendLog('[error] ' + e, 'err');
        showToast(String(e));
      }
    };
  }

  async function checkQualityNow() {
    if (qualityChecking) return;
    await callApp('CheckQuality');
  }

  if (sQualityOpen) {
    sQualityOpen.onclick = () => {
      if (qualityItems.length > 0) {
        openQuality();
        return;
      }
      callApp('PendingFound')
        .then((items) => {
          renderQuality(items);
          if (qualityItems.length > 0) openQuality();
        })
        .catch((e) => appendLog('[error] ' + e, 'err'));
    };
  }

  events('quality-check-started', () => {
    qualityChecking = true;
    if (sCheckNow) sCheckNow.disabled = true;
    setQualityStatus('FLAC: проверка идёт…');
    if (qualityOverlay && qualityOverlay.classList.contains('open')) renderQualityList();
    renderRejectedList(rejectedItems);
  });
  events('quality-check-progress', (done, total) => {
    setQualityStatus('FLAC: проверено ' + done + '/' + total + '…');
  });
  events('quality-check-finished', (text, found, err, pending) => {
    qualityChecking = false;
    if (sCheckNow) sCheckNow.disabled = false;
    setQualityStatus(text || 'FLAC: проверка завершена');
    updateQualityBadge(pending || 0);
    if (err && sQualityStatus) sQualityStatus.title = err;
    if (qualityOverlay && qualityOverlay.classList.contains('open')) renderQualityList();
    renderRejectedList(rejectedItems);
  });
  events('quality-found', (items) => renderQuality(items));
  events('quality-pending', (items) => renderQuality(items));

  async function loadQualityStatus() {
    try {
      const s = await callApp('QualityStatus');
      setQualityStatus(s && s.text ? s.text : 'FLAC: проверок не было');
      qualityOutputDir = (s && s.outputDir) || qualityOutputDir;
      qualityChecking = !!(s && s.checking);
      if (sCheckNow) sCheckNow.disabled = qualityChecking;
      updateQualityBadge((s && s.pending) || 0);
      if (s && s.pending > 0) {
        const items = await callApp('PendingFound');
        renderQuality(items);
      }
    } catch (e) {
      // Бэкенд может быть ещё не готов при первом кадре.
    }
  }

  // --- Settings ---
  function settingsSnapshotValue() {
    const fields = [
      sToken, sLibrary, sOutput, sDiffOutput, sWorkers, sConvert, sEmbedCover,
      sEmbedLyrics, sSaveLrc, sSubdirs, sSubdirTemplate, sExportM3u, sFlacMonitor,
    ];
    return fields.map((el) => {
      if (!el) return '';
      return el.type === 'checkbox' ? (el.checked ? '1' : '0') : String(el.value || '');
    }).join('\u0001');
  }

  function isDirty() {
    return settingsSnapshot !== null && settingsSnapshotValue() !== settingsSnapshot;
  }

  function syncOutputField() {
    if (!sDiffOutput || !sOutput || !sOutputPick || !sLibrary) return;
    const diff = sDiffOutput.checked;
    sOutput.disabled = !diff;
    sOutputPick.disabled = !diff;
    if (!diff) sOutput.value = sLibrary.value;
  }

  function syncSubdirTemplate() {
    if (sSubdirTemplate && sSubdirs) sSubdirTemplate.disabled = !sSubdirs.checked;
  }

  function syncTokenHint() {
    if (tokenHint && sToken) tokenHint.hidden = sToken.value.trim() !== '';
  }

  function workersStep(delta) {
    if (!sWorkers) return;
    let v = Number(sWorkers.value);
    if (!Number.isFinite(v)) v = 1;
    v = Math.min(10, Math.max(1, Math.round(v) + delta));
    sWorkers.value = v;
  }

  if (sDiffOutput) sDiffOutput.addEventListener('change', syncOutputField);
  if (sLibrary) sLibrary.addEventListener('input', syncOutputField);
  if (sSubdirs) sSubdirs.addEventListener('change', syncSubdirTemplate);
  if (sMinus) sMinus.onclick = () => workersStep(-1);
  if (sPlus) sPlus.onclick = () => workersStep(1);
  if (sWorkers) {
    sWorkers.addEventListener('blur', () => {
      let v = Math.round(Number(sWorkers.value));
      if (!Number.isFinite(v)) v = 1;
      sWorkers.value = Math.min(10, Math.max(1, v));
    });
  }
  if (sToken) sToken.addEventListener('input', syncTokenHint);

  function openSettings() {
    if (!settingsOverlay) return;
    if (settingsError) settingsError.textContent = '';
    callApp('GetSettings').then((s) => {
      if (sToken) sToken.value = s.Token || '';
      if (sLibrary) sLibrary.value = s.LibraryDir || '';
      if (sDiffOutput) sDiffOutput.checked = (s.OutputDir || '') !== (s.LibraryDir || '');
      if (sOutput) sOutput.value = s.OutputDir || '';
      syncOutputField();
      if (sWorkers) sWorkers.value = s.Workers;
      if (sConvert) sConvert.checked = !!s.ConvertM4A;
      if (sEmbedCover) sEmbedCover.checked = !!s.EmbedCover;
      if (sEmbedLyrics) sEmbedLyrics.checked = s.EmbedLyrics !== false;
      if (sSaveLrc) sSaveLrc.checked = !!s.SaveLRC;
      if (sSubdirs) sSubdirs.checked = !!s.Subdirs;
      if (sSubdirTemplate) sSubdirTemplate.value = s.SubdirTemplate || '%artist%/%album%';
      if (sExportM3u) sExportM3u.checked = !!s.ExportM3U;
      if (sFlacMonitor) sFlacMonitor.checked = !!s.FlacMonitor;
      if (sQualityStatus) sQualityStatus.textContent = s.QualityText || 'FLAC: проверок не было';
      qualityOutputDir = s.OutputDir || qualityOutputDir;
      qualityChecking = !!s.QualityChecking;
      if (sCheckNow) sCheckNow.disabled = qualityChecking;
      renderRejectedList(s.QualityRejected || []);
      updateQualityBadge(s.QualityPending || 0);
      if (s.QualityPending > 0 && qualityItems.length === 0) {
        callApp('PendingFound')
          .then((items) => renderQuality(items))
          .catch((e) => appendLog('[error] ' + e, 'err'));
      }
      syncSubdirTemplate();
      syncTokenHint();
      if (tokenCheck) {
        tokenCheck.textContent = '';
        tokenCheck.className = 'form-hint';
      }
      settingsSnapshot = settingsSnapshotValue();
      settingsOverlay.classList.add('open');
      settingsOverlay.setAttribute('aria-hidden', 'false');
      activeModal = 'settings';
      if (sToken) sToken.focus();
    }).catch((e) => {
      appendLog('[error] ' + e, 'err');
      showToast(String(e));
    });
  }

  function closeTokenHelp(focusLink) {
    if (!tokenHelpOverlay) return;
    const wasOpen = tokenHelpOverlay.classList.contains('open');
    tokenHelpOverlay.classList.remove('open');
    tokenHelpOverlay.setAttribute('aria-hidden', 'true');
    activeModal = settingsOverlay && settingsOverlay.classList.contains('open') ? 'settings' : null;
    if (wasOpen && focusLink !== false && tokenHelpLink) tokenHelpLink.focus();
  }

  function openTokenHelp() {
    if (!tokenHelpOverlay || tokenHelpOverlay.classList.contains('open')) return;
    tokenHelpOverlay.classList.add('open');
    tokenHelpOverlay.setAttribute('aria-hidden', 'false');
    activeModal = 'tokenHelp';
    if (tokenHelpClose) tokenHelpClose.focus();
  }

  function closeSettings(force) {
    if (!settingsOverlay) return;
    if (!force && isDirty()) {
      openConfirm(
        'Закрыть настройки?',
        'В форме есть несохранённые изменения. Если закрыть сейчас, они пропадут.',
        () => closeSettings(true),
        'settings'
      );
      return;
    }
    closeTokenHelp(false);
    settingsOverlay.classList.remove('open');
    settingsOverlay.setAttribute('aria-hidden', 'true');
    activeModal = null;
    settingsSnapshot = null;
    if (tokenCheck) {
      tokenCheck.textContent = '';
      tokenCheck.className = 'form-hint';
    }
    if (tokenHint) tokenHint.hidden = true;
    if (settingsError) settingsError.textContent = '';
    if (input) input.focus();
  }

  if (settingsBtn) settingsBtn.onclick = openSettings;
  if (settingsClose) settingsClose.onclick = () => closeSettings(false);
  if (sCancel) sCancel.onclick = () => closeSettings(true);
  if (settingsOverlay) {
    settingsOverlay.onclick = (e) => { if (e.target === settingsOverlay) closeSettings(false); };
  }
  if (tokenHelpLink) tokenHelpLink.onclick = openTokenHelp;
  if (tokenHelpClose) tokenHelpClose.onclick = () => closeTokenHelp();
  if (tokenHelpCloseFooter) tokenHelpCloseFooter.onclick = () => closeTokenHelp();
  if (tokenHelpOverlay) {
    tokenHelpOverlay.onclick = (e) => { if (e.target === tokenHelpOverlay) closeTokenHelp(); };
  }
  if (tokenHelpOAuth) {
    tokenHelpOAuth.onclick = () => {
      if (window.runtime && typeof window.runtime.BrowserOpenURL === 'function') {
        window.runtime.BrowserOpenURL(OAUTH_URL);
      }
    };
  }

  if (sCheck) {
    sCheck.onclick = async () => {
      if (!tokenCheck) return;
      tokenCheck.textContent = 'проверяю...';
      tokenCheck.className = 'form-hint';
      try {
        const res = await callApp('CheckToken', sToken ? sToken.value : '');
        tokenCheck.className = 'form-hint ok';
        tokenCheck.textContent = res;
      } catch (e) {
        tokenCheck.className = 'form-hint err';
        tokenCheck.textContent = String(e);
      }
    };
  }

  if (sOutputPick) {
    sOutputPick.onclick = async () => {
      try {
        const p = await callApp('PickFolder');
        if (p && sOutput) sOutput.value = p;
      } catch (e) {
        if (settingsError) settingsError.textContent = String(e);
      }
    };
  }
  if (sLibraryPick) {
    sLibraryPick.onclick = async () => {
      try {
        const p = await callApp('PickFolder');
        if (p && sLibrary) {
          sLibrary.value = p;
          syncOutputField();
        }
      } catch (e) {
        if (settingsError) settingsError.textContent = String(e);
      }
    };
  }

  if (sCheckNow) {
    sCheckNow.onclick = async () => {
      if (settingsError) settingsError.textContent = '';
      sCheckNow.disabled = true;
      try {
        await checkQualityNow();
      } catch (e) {
        appendLog('[error] ' + e, 'err');
        if (settingsError) settingsError.textContent = String(e);
      } finally {
        sCheckNow.disabled = qualityChecking;
      }
    };
  }

  if (sSave) {
    sSave.onclick = async () => {
      if (settingsError) settingsError.textContent = '';
      const workers = Number(sWorkers ? sWorkers.value : 0);
      if (!Number.isInteger(workers) || workers < 1 || workers > 10) {
        if (settingsError) settingsError.textContent = 'Число параллельных скачиваний: целое от 1 до 10';
        return;
      }
      const library = sLibrary ? sLibrary.value : '';
      const output = sDiffOutput && sDiffOutput.checked ? (sOutput ? sOutput.value : '') : library;
      if (!output.trim()) {
        if (settingsError) {
          settingsError.textContent = sDiffOutput && sDiffOutput.checked
            ? 'Папка загрузок не может быть пустой'
            : 'Папка коллекции пуста — укажи её или включи «папка загрузок отличается»';
        }
        return;
      }
      try {
        await callApp('SaveSettings', {
          Token: sToken ? sToken.value : '',
          OutputDir: output,
          LibraryDir: library,
          Workers: workers,
          ConvertM4A: sConvert ? sConvert.checked : false,
          EmbedCover: sEmbedCover ? sEmbedCover.checked : false,
          EmbedLyrics: sEmbedLyrics ? sEmbedLyrics.checked : true,
          SaveLRC: sSaveLrc ? sSaveLrc.checked : false,
          Subdirs: sSubdirs ? sSubdirs.checked : false,
          SubdirTemplate: sSubdirTemplate ? sSubdirTemplate.value : '',
          ExportM3U: sExportM3u ? sExportM3u.checked : false,
          FlacMonitor: sFlacMonitor ? sFlacMonitor.checked : false,
        });
        closeSettings(true);
        appendLog('[done] настройки сохранены', 'done');
        showToast('Настройки сохранены. Новые задачи возьмут новые значения.');
      } catch (e) {
        if (settingsError) settingsError.textContent = String(e);
      }
    };
  }

  // --- Confirm dialog ---
  function openConfirm(title, text, action, returnModal) {
    if (!confirmOverlay) return;
    confirmAction = action;
    confirmReturn = returnModal || activeModal;
    confirmPreviousFocus = document.activeElement;
    if (confirmTitle) confirmTitle.textContent = title;
    if (confirmText) confirmText.textContent = text;
    confirmOverlay.classList.add('open');
    confirmOverlay.setAttribute('aria-hidden', 'false');
    activeModal = 'confirm';
    if (confirmCancel) confirmCancel.focus();
  }

  function closeConfirm() {
    if (!confirmOverlay || !confirmOverlay.classList.contains('open')) return;
    confirmOverlay.classList.remove('open');
    confirmOverlay.setAttribute('aria-hidden', 'true');
    activeModal = confirmReturn;
    confirmReturn = null;
    confirmAction = null;
    if (confirmPreviousFocus && typeof confirmPreviousFocus.focus === 'function') {
      confirmPreviousFocus.focus();
    } else if (activeModal === 'settings' && sToken) {
      sToken.focus();
    }
  }

  if (confirmOk) {
    confirmOk.onclick = () => {
      const action = confirmAction;
      closeConfirm();
      if (action) action();
    };
  }
  if (confirmCancel) confirmCancel.onclick = () => closeConfirm();
  if (confirmOverlay) {
    confirmOverlay.onclick = (e) => { if (e.target === confirmOverlay) closeConfirm(); };
  }

  // --- Focus trap and keyboard ---
  function activeModalElement() {
    switch (activeModal) {
      case 'settings': return settingsCard;
      case 'tokenHelp': return tokenHelpCard;
      case 'quality': return qualityCard;
      case 'confirm': return confirmCard;
      default: return null;
    }
  }

  function focusable(container) {
    if (!container || typeof container.querySelectorAll !== 'function') return [];
    const selector = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
    return Array.from(container.querySelectorAll(selector)).filter((el) => {
      if (!el || el.hidden) return false;
      if (typeof el.offsetParent !== 'undefined' && el.offsetParent === null) return false;
      return true;
    });
  }

  function trapFocus(e) {
    const modal = activeModalElement();
    if (!modal) return;
    const items = focusable(modal);
    if (items.length === 0) {
      e.preventDefault();
      if (typeof modal.focus === 'function') modal.focus();
      return;
    }
    const first = items[0];
    const last = items[items.length - 1];
    const active = document.activeElement;
    if (!active || !modal.contains || !modal.contains(active)) {
      e.preventDefault();
      first.focus();
      return;
    }
    if (e.shiftKey && active === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && active === last) {
      e.preventDefault();
      first.focus();
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Tab') {
      trapFocus(e);
      return;
    }
    if (e.key !== 'Escape') return;
    if (activeModal === 'tokenHelp') {
      e.preventDefault();
      closeTokenHelp();
    } else if (activeModal === 'settings') {
      e.preventDefault();
      closeSettings(false);
    } else if (activeModal === 'quality') {
      e.preventDefault();
      closeQuality();
    } else if (activeModal === 'confirm') {
      e.preventDefault();
      closeConfirm();
    }
  }
  document.addEventListener('keydown', onKeyDown);
  events('window-shown', () => {
    startBoot();
    refreshQueue();
    if (input) input.focus();
  });

  // --- Queue bootstrap ---
  function refreshQueue() {
    callApp('GetQueue')
      .then((snap) => {
        queueReady = true;
        syncSnapshot(snap);
      })
      .catch(() => {
        // The backend may still be starting; the first queue event fills the UI.
      });
  }

  // --- Init ---
  if (boot) {
    if (typeof setTimeout !== 'function') {
      dismissBoot();
    } else {
      boot.classList.add('boot--armed');
      const api = app();
      if (api && typeof api.BootReady === 'function') {
        try {
          Promise.resolve(api.BootReady()).then(startBoot, startBoot);
        } catch (e) {
          startBoot();
        }
        setTimeout(startBoot, 1500);
      } else {
        startBoot();
      }
    }
  }
  updateInputChrome();
  updateLogBadge();
  setQualityStatus('FLAC: проверок не было');
  applyState(null);
  if (input) input.focus();
  loadQualityStatus();
  refreshQueue();
  if (typeof setTimeout === 'function') {
    setTimeout(() => { if (!queueReady) refreshQueue(); }, 600);
  }

  // Тестовый/диагностический API: стенд tests/queue_check.js исполняет этот
  // файл в VM и проверяет состояние без глобального мусора.
  window.__yamdl = {
    getState: () => stateName,
    isBootDismissed: () => bootDismissed,
    getTasks: () => Array.from(tasks.values()),
    getSnapshot: () => snapshot,
    getLastSubmit: () => lastSubmit,
    submit: submitQueue,
    stateText,
    handleTaskAdded,
    handleTaskUpdated,
    handleTaskRemoved,
    handleSources,
    handleQueue,
    appendLog,
    setFilter: (key) => { filterSource = key || ''; renderSources(); applyFilter(); },
    getFilter: () => filterSource,
    openSettings,
    closeSettings,
    isDirty,
    openConfirm,
    closeConfirm,
    openQuality,
    closeQuality,
    renderQuality,
    renderRejectedList,
    rejectQualityItem,
    undoRejected,
    getQualityItems: () => qualityItems,
    getRejectedItems: () => rejectedItems,
    showToast,
  };
})();
