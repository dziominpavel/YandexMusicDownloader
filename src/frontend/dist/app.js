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
  const setClass = (el, name, on) => {
    if (!el) return;
    if (on) el.classList.add(name);
    else el.classList.remove(name);
  };
  const hide = (el, value) => { if (el) el.hidden = value; };

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
  const composerTitle = $('composerTitle');
  const composerSub = $('composerSub');
  const composerHint = $('composerHint');
  const multiHint = $('multiHint');

  const preview = $('preview');
  const previewSkeleton = $('previewSkeleton');
  const previewContent = $('previewContent');
  const previewCover = $('previewCover');
  const previewCoverFallback = $('previewCoverFallback');
  const previewKind = $('previewKind');
  const previewTitle = $('previewTitle');
  const previewSub = $('previewSub');

  const jobHead = $('jobHead');
  const jobTitle = $('jobTitle');
  const jobSubtitle = $('jobSubtitle');
  const bgBanner = $('bgBanner');
  const bgBannerText = $('bgBannerText');
  const errorBanner = $('errorBanner');
  const errorText = $('errorText');
  const retryBtn = $('retryBtn');
  const errorClose = $('errorClose');
  const statusbar = $('statusbar');
  const sbfillwrap = $('sbfillwrap');
  const sbfill = $('sbfill');
  const sbpct = $('sbpct');
  const sbcount = $('sbcount');
  const sbbytes = $('sbbytes');
  const tracksSection = $('tracksSection');
  const tracks = $('tracks');
  const trackSummary = $('trackSummary');
  const summary = $('summary');
  const summaryTitle = $('summaryTitle');
  const summaryText = $('summaryText');
  const summaryDetails = $('summaryDetails');
  const summaryIconUse = $('summaryIconUse');
  const summaryOpen = $('summaryOpen');
  const summaryAgain = $('summaryAgain');
  const details = $('details');
  const log = $('log');
  const logBadge = $('logBadge');
  const logCopy = $('logCopy');
  const logClear = $('logClear');
  const progressAnnounce = $('progressAnnounce');
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

  const dupOverlay = $('dupOverlay');
  const dupCard = $('dupCard');
  const dupPercent = $('dupPercent');
  const dupTrack = $('dupTrack');
  const dupMatchName = $('dupMatchName');
  const dupMatchPath = $('dupMatchPath');
  const dupFormat = $('dupFormat');
  const dupYes = $('dupYes');
  const dupNo = $('dupNo');

  const confirmOverlay = $('confirmOverlay');
  const confirmCard = $('confirmCard');
  const confirmTitle = $('confirmTitle');
  const confirmText = $('confirmText');
  const confirmCancel = $('confirmCancel');
  const confirmOk = $('confirmOk');

  // --- State ---
  let busy = false;
  let lastLink = '';
  let uiState = 'idle';
  let jobIsQuality = false;
  let previewSeq = 0;
  let previewTimer = 0;
  const clearPreviewTimer = () => {
    if (typeof clearTimeout === 'function') clearTimeout(previewTimer);
  };
  const schedulePreview = () => {
    if (typeof setTimeout === 'function') previewTimer = setTimeout(updatePreview, 500);
  };
  let logErrorCount = 0;
  let qualityChecking = false;
  let qualityOutputDir = '';
  let qualityItems = [];
  let qualityListOpen = false;
  let rejectedItems = [];
  let dupOpen = false;
  let settingsSnapshot = null;
  let confirmAction = null;
  let confirmReturn = null;
  let confirmPreviousFocus = null;
  let activeModal = null;

  const job = {
    total: 0,
    states: new Map(),
    ended: false,
    result: null,
    lastAnnounce: -1,
    lastDone: -1,
  };
  const trows = new Map();

  function plural(n, one, few, many) {
    const mod10 = n % 10;
    const mod100 = n % 100;
    if (mod10 === 1 && mod100 !== 11) return one;
    if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return few;
    return many;
  }

  function fmtMB(b) {
    return (Number(b) / 1048576).toFixed(1);
  }

  function announce(text) {
    if (!progressAnnounce || !text) return;
    progressAnnounce.textContent = '';
    progressAnnounce.textContent = text;
  }

  function updateLogBadge() {
    if (!logBadge) return;
    logBadge.textContent = String(logErrorCount);
    logBadge.hidden = logErrorCount === 0;
  }

  function line(text, cls) {
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
    if (details && details.open && typeof log.scrollHeight === 'number') {
      log.scrollTop = log.scrollHeight;
    }
  }

  function parseTrackStatus(raw) {
    const s = String(raw || '');
    if (s.startsWith('[done]')) {
      const inside = /\(([^)]+)\)/.exec(s);
      let format = '';
      if (inside && /flac/i.test(inside[1])) format = 'FLAC';
      else if (inside && /mp3/i.test(inside[1])) format = 'MP3';
      else if (/flac/i.test(s)) format = 'FLAC';
      else if (/mp3/i.test(s)) format = 'MP3';
      let label = 'Готово';
      if (format) label += ' - ' + format;
      if (s.includes('[ALAC→FLAC]') || s.includes('[ALAC->FLAC]')) label = 'Готово - FLAC';
      return { label, format, kind: 'done', terminal: true };
    }
    if (s.startsWith('[skip]')) {
      let label = 'Пропущено';
      if (s.includes('already exists')) label += ' - уже есть';
      else if (s.includes('duplicate')) label += ' - дубль';
      else if (s.includes('unavailable')) label += ' - недоступно';
      return { label, format: '', kind: 'info', terminal: true };
    }
    if (s.startsWith('[error]')) {
      const reason = s.replace(/^\[error\]\s*/, '').trim();
      return { label: reason ? 'Ошибка: ' + reason : 'Ошибка', format: '', kind: 'err', terminal: true };
    }
    if (s.startsWith('[stopped]')) return { label: 'Остановлено', format: '', kind: 'stopped', terminal: false };
    if (s.startsWith('[downloading]')) return { label: 'Скачивается', format: '', kind: 'info', terminal: false };
    return { label: 'Ожидание', format: '', kind: '', terminal: false };
  }

  function setState(next) {
    uiState = next;
    if (document.body && document.body.setAttribute) {
      document.body.setAttribute('data-state', next);
    }
    hide(composerHint, next !== 'idle' && next !== 'preview');
    hide(preview, next !== 'preview');
    hide(jobHead, !(next === 'active' || next === 'done' || next === 'error'));
    hide(tracksSection, !(next === 'active' || next === 'done' || next === 'error'));
    hide(statusbar, next !== 'active');
    hide(summary, !(next === 'done' || next === 'error'));
    hide(errorBanner, next !== 'error');
    hide(bgBanner, !(jobIsQuality && (next === 'active' || next === 'done')));

    if (composerTitle) {
      if (next === 'active') composerTitle.textContent = 'Скачивание';
      else if (next === 'done') composerTitle.textContent = 'Готово';
      else if (next === 'error') composerTitle.textContent = 'Попробовать снова';
      else if (next === 'preview') composerTitle.textContent = 'Готово к скачиванию';
      else composerTitle.textContent = 'Что скачать?';
    }
    if (composerSub) {
      if (next === 'idle') composerSub.textContent = 'Вставьте ссылку на трек, альбом или плейлист';
      else if (next === 'preview') composerSub.textContent = 'Проверьте ссылку и нажмите «Скачать»';
      else if (next === 'active') composerSub.textContent = '';
      else if (next === 'done') composerSub.textContent = 'Можно вставить следующую ссылку';
      else composerSub.textContent = 'Ссылка сохранена — можно повторить';
    }
  }

  function setIcon(name) {
    if (dlIconUse && dlIconUse.setAttribute) {
      dlIconUse.setAttribute('href', '#icon-' + name);
    }
  }

  function setBusyUi(active) {
    busy = active;
    if (!btn) return;
    if (active) {
      setClass(btn, 'stop', true);
      btn.disabled = false;
      if (dlLabel) dlLabel.textContent = jobIsQuality ? 'Остановить обновление' : 'Остановить';
      setIcon('stop');
      if (settingsBtn) settingsBtn.disabled = true;
      if (sCheckNow) sCheckNow.disabled = true;
    } else {
      setClass(btn, 'stop', false);
      btn.disabled = false;
      if (dlLabel) dlLabel.textContent = 'Скачать';
      setIcon('download');
      if (settingsBtn) settingsBtn.disabled = false;
      if (sCheckNow) sCheckNow.disabled = qualityChecking;
      if (input) input.focus();
    }
  }

  function updateTrackSummary() {
    if (!trackSummary) return;
    trackSummary.textContent = job.total > 0 ? trows.size + ' из ' + job.total : '';
    if (jobSubtitle && job.total > 0) {
      jobSubtitle.textContent = trows.size + ' из ' + job.total + ' ' + plural(job.total, 'трека', 'треков', 'треков');
    }
  }

  // --- Statusbar ---
  let sbDirty = false;

  function setTrackState(id, share, done, total) {
    let s = job.states.get(id);
    if (!s) {
      s = { share: 0, done: 0, total: 0 };
      job.states.set(id, s);
    }
    if (share !== null) s.share = share;
    if (done !== null) s.done = done;
    if (total !== null) s.total = total;
    scheduleSB();
  }

  function scheduleSB() {
    if (sbDirty) return;
    sbDirty = true;
    requestAnimationFrame(() => {
      sbDirty = false;
      drawSB();
    });
  }

  function drawSB() {
    if (job.total <= 0) return;
    let done = 0;
    let sum = 0;
    let bytesDone = 0;
    let bytesTotal = 0;
    let anyTotal = false;
    for (const s of job.states.values()) {
      sum += s.share;
      if (s.share >= 1) done++;
      if (s.total > 0) {
        bytesDone += s.done;
        bytesTotal += s.total;
        anyTotal = true;
      }
    }
    const pct = Math.min(100, Math.round((sum / job.total) * 100));
    if (sbfill) sbfill.style.width = pct + '%';
    if (sbpct) sbpct.textContent = pct + '%';
    if (sbcount) sbcount.textContent = done + '/' + job.total + ' треков';
    if (sbbytes) sbbytes.textContent = anyTotal ? fmtMB(bytesDone) + '/' + fmtMB(bytesTotal) + ' МБ' : '';
    if (sbfillwrap && sbfillwrap.setAttribute) {
      sbfillwrap.setAttribute('aria-valuenow', String(pct));
    }

    if (pct >= 100 && job.lastAnnounce < 100) {
      announce('Загрузка завершена');
      job.lastAnnounce = 100;
    } else if (pct >= 75 && job.lastAnnounce < 75) {
      announce('Загрузка 75 процентов');
      job.lastAnnounce = 75;
    } else if (pct >= 50 && job.lastAnnounce < 50) {
      announce('Загрузка 50 процентов');
      job.lastAnnounce = 50;
    } else if (pct >= 25 && job.lastAnnounce < 25) {
      announce('Загрузка 25 процентов');
      job.lastAnnounce = 25;
    }
    if (done !== job.lastDone && done > 0) {
      announce('Готово ' + done + ' из ' + job.total + ' треков');
      job.lastDone = done;
    }

    if (job.ended && job.result) {
      if (job.result.stopped) {
        if (sbfill) sbfill.className = 'stopped';
        if (sbcount) sbcount.textContent = done + '/' + job.total + ' треков, остановлено';
      } else if (job.result.failed > 0) {
        if (sbfill) sbfill.className = 'err';
        if (sbcount) sbcount.textContent = done + '/' + job.total + ' треков, ошибок: ' + job.result.failed;
      } else if (sbfill) {
        sbfill.className = 'done';
      }
    } else if (sbfill) {
      sbfill.className = '';
    }
  }

  // --- Tracks ---
  function trackList(items) {
    if (!tracks) return;
    items.forEach((pair) => {
      const id = pair[0];
      const label = pair[1] || 'Трек';
      if (trows.has(id)) return;

      const row = document.createElement('div');
      row.className = 'track-row';

      const index = document.createElement('span');
      index.className = 'track-index';
      index.textContent = String(trows.size + 1).padStart(2, '0');

      const main = document.createElement('div');
      main.className = 'track-main';
      const title = document.createElement('div');
      title.className = 'track-title';
      title.textContent = label;
      title.title = label;
      const meta = document.createElement('div');
      meta.className = 'track-meta';
      const format = document.createElement('span');
      format.className = 'format-pill';
      format.hidden = true;
      const size = document.createElement('span');
      size.className = 'track-size';
      meta.appendChild(format);
      meta.appendChild(size);
      main.appendChild(title);
      main.appendChild(meta);

      const bar = document.createElement('div');
      bar.className = 'track-bar';
      const fill = document.createElement('div');
      fill.className = 'track-fill';
      bar.appendChild(fill);

      const stat = document.createElement('span');
      stat.className = 'track-status';
      stat.textContent = 'Ожидание';

      row.appendChild(index);
      row.appendChild(main);
      row.appendChild(bar);
      row.appendChild(stat);
      tracks.appendChild(row);

      trows.set(id, { row, stat, bar, fill, size, format, title });
      if (!job.states.has(id)) setTrackState(id, 0, null, null);
    });
    updateTrackSummary();
  }

  function trackStatus(id, status, kind) {
    let t = trows.get(id);
    if (!t) {
      trackList([[id, '']]);
      t = trows.get(id);
    }
    if (!t) return;
    const parsed = parseTrackStatus(status);
    t.stat.textContent = parsed.label;
    t.stat.className = 'track-status' + (parsed.kind ? ' status-' + parsed.kind : '');
    if (parsed.format) {
      t.format.textContent = parsed.format;
      t.format.hidden = false;
      t.format.className = 'format-pill ' + parsed.format.toLowerCase();
    } else {
      t.format.hidden = true;
    }
    if (parsed.terminal) {
      setTrackState(id, 1, null, null);
      if (t.fill) {
        t.fill.style.width = '100%';
        setClass(t.fill, 'done', parsed.kind === 'done');
        setClass(t.fill, 'err', parsed.kind === 'err');
        setClass(t.fill, 'skipped', parsed.kind === 'info');
      }
      if (t.title) announce(parsed.label + ': ' + t.title.textContent);
    } else if (status.startsWith('[downloading]') && !job.states.has(id)) {
      setTrackState(id, 0, null, null);
    }
  }

  function progress(id, label, pct, done, total) {
    const pctNum = Number(pct);
    setTrackState(id, pctNum >= 0 ? pctNum / 100 : 0, Number(done), Number(total));
    let t = trows.get(id);
    if (!t) {
      trackList([[id, label]]);
      t = trows.get(id);
    }
    if (!t) return;
    const hasTotal = Number(total) > 0 && pctNum >= 0;
    t.fill.style.width = hasTotal ? pctNum + '%' : '100%';
    setClass(t.fill, 'indeterminate', !hasTotal);
    t.bar.title = hasTotal
      ? pctNum + '% ' + fmtMB(done) + '/' + fmtMB(total) + ' МБ'
      : fmtMB(done) + ' МБ';
    t.size.textContent = hasTotal
      ? fmtMB(done) + ' / ' + fmtMB(total) + ' МБ'
      : fmtMB(done) + ' МБ';
  }

  // --- Job lifecycle ---
  function jobReset(title, count) {
    job.total = count;
    job.states = new Map();
    job.ended = false;
    job.result = null;
    job.lastAnnounce = -1;
    job.lastDone = -1;
    if (tracks) tracks.innerHTML = '';
    trows.clear();
    if (log) log.innerHTML = '';
    logErrorCount = 0;
    updateLogBadge();
    setClass(summary, 'error', false);
    hide(summary, true);
    hide(errorBanner, true);
    hide(bgBanner, !jobIsQuality);
    if (jobTitle) jobTitle.textContent = title || 'Скачивание';
    if (jobSubtitle) {
      jobSubtitle.textContent = count + ' ' + plural(count, 'трек', 'трека', 'треков');
    }
    if (trackSummary) trackSummary.textContent = '0 из ' + count;
    if (sbfill) {
      sbfill.className = '';
      sbfill.style.width = '0%';
    }
    if (sbpct) sbpct.textContent = '0%';
    if (sbcount) sbcount.textContent = '0/' + count + ' треков';
    if (sbbytes) sbbytes.textContent = '';
    setState('active');
    scheduleSB();
  }

  function showSummary(ok, skip, failed, stopped) {
    if (!summary) return;
    const title = stopped
      ? 'Остановлено'
      : failed > 0
        ? (jobIsQuality ? 'Обновление FLAC завершено с ошибками' : 'Завершено с ошибками')
        : (jobIsQuality ? 'Обновление FLAC завершено' : 'Готово');
    if (summaryTitle) summaryTitle.textContent = title;
    if (summaryText) {
      summaryText.textContent = 'Готово: ' + ok + ' · Пропущено: ' + skip + ' · Ошибки: ' + failed;
    }
    let flac = 0;
    let mp3 = 0;
    for (const t of trows.values()) {
      if (!t.format || t.format.hidden) continue;
      if (t.format.textContent === 'FLAC') flac++;
      else if (t.format.textContent === 'MP3') mp3++;
    }
    if (summaryDetails) {
      const bits = [];
      if (flac > 0) bits.push('FLAC: ' + flac);
      if (mp3 > 0) bits.push('MP3: ' + mp3);
      summaryDetails.textContent = bits.join(' · ');
      summaryDetails.hidden = bits.length === 0;
    }
    setClass(summary, 'error', stopped || failed > 0);
    if (summaryIconUse) {
      summaryIconUse.setAttribute('href', stopped || failed > 0 ? '#icon-alert' : '#icon-check');
    }
    hide(summary, false);
    const errorState = !stopped && failed > 0 && ok === 0 && skip === 0;
    setState(errorState ? 'error' : 'done');
    if (errorState) {
      showError(
        'Не удалось скачать. Подробности — в «Подробностях».',
        jobIsQuality ? '' : lastLink
      );
    }
    announce(title + '. Готово ' + ok + ', пропущено ' + skip + ', ошибок ' + failed + '.');
  }

  function handleJobStart(title, count) {
    jobIsQuality = /обновление\s+flac/i.test(title || '');
    jobReset(title, count);
    setBusyUi(true);
    if (jobIsQuality) {
      if (bgBannerText) bgBannerText.textContent = 'Обновление FLAC: можно остановить в любой момент';
      hide(bgBanner, false);
    }
  }

  function handleJobEnd(ok, skip, error, stopped) {
    job.ended = true;
    job.result = { ok, skip, failed: error, stopped: !!stopped };
    if (!stopped) {
      for (const s of job.states.values()) s.share = 1;
    }
    drawSB();
    if (job.total > 0) showSummary(ok, skip, error, !!stopped);
    else setState('idle');
    setBusyUi(false);
    if (jobIsQuality) {
      hide(bgBanner, true);
      jobIsQuality = false;
    }
  }

  events('job-start', handleJobStart);
  events('job-end', handleJobEnd);

  events('tracks', (items) => trackList(items));
  events('track-status', (id, status, kind) => trackStatus(id, status, kind));
  events('progress', (id, label, pct, done, total) => progress(id, label, pct, done, total));
  events('log', (text, kind) => {
    line(text, kind);
    if (kind === 'err') {
      showError(String(text).replace(/^\[error\]\s*/, ''), lastLink);
    }
  });

  // --- Preview ---
  function hidePreview() {
    previewSeq++;
    hide(preview, true);
    hide(previewSkeleton, true);
    hide(previewContent, false);
    if (previewCover) previewCover.removeAttribute('src');
    if (previewTitle) previewTitle.textContent = '';
    if (previewSub) previewSub.textContent = '';
    if (previewKind) previewKind.textContent = '';
    if (uiState === 'preview') setState('idle');
  }

  function showPreviewSkeleton() {
    hide(preview, false);
    hide(previewSkeleton, false);
    hide(previewContent, true);
    setState('preview');
  }

  function showPreview(p) {
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
    hide(previewSkeleton, true);
    hide(previewContent, false);
    hide(preview, false);
    setState('preview');
  }

  async function updatePreview() {
    const link = input ? input.value.trim() : '';
    if (!link || /\s/.test(link)) {
      hidePreview();
      return;
    }
    const seq = ++previewSeq;
    showPreviewSkeleton();
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

  function updateInputChrome() {
    const value = input ? input.value : '';
    const links = value.trim() ? value.trim().split(/\s+/).filter(Boolean) : [];
    if (clearBtn) clearBtn.hidden = value.length === 0;
    if (multiHint) {
      multiHint.textContent = links.length > 1
        ? links.length + ' ' + plural(links.length, 'ссылка', 'ссылки', 'ссылок')
        : '';
      multiHint.hidden = links.length <= 1;
    }
    return links;
  }

  function clearError() {
    hide(errorBanner, true);
    if (errorText) errorText.textContent = '';
  }

  function showError(message, link) {
    if (!errorBanner) return;
    const text = message || 'Неизвестная ошибка';
    if (errorText) errorText.textContent = text;
    if (retryBtn) retryBtn.hidden = !link;
    if (summaryTitle) summaryTitle.textContent = 'Ошибка';
    if (summaryText) summaryText.textContent = text;
    if (summaryDetails) {
      summaryDetails.textContent = '';
      summaryDetails.hidden = true;
    }
    if (summaryIconUse) summaryIconUse.setAttribute('href', '#icon-alert');
    setClass(summary, 'error', true);
    hide(errorBanner, false);
    setState('error');
  }

  // --- Download button ---
  async function startDownload() {
    if (busy) return;
    const link = input ? input.value.trim() : '';
    if (!link) return;
    lastLink = link;
    clearPreviewTimer();
    hidePreview();
    clearError();
    if (input) input.value = '';
    if (clearBtn) clearBtn.hidden = true;
    if (multiHint) multiHint.hidden = true;
    if (input) input.focus();
    setBusyUi(true);
    try {
      const res = await callApp('Download', link);
      if (res) {
        const kind = res.startsWith('[done]') ? 'done'
          : res.startsWith('[error]') ? 'err'
            : res.startsWith('[stopped]') ? 'stopped' : 'info';
        line(res, kind);
        if (res.startsWith('[error]')) {
          showError(res.replace(/^\[error\]\s*/, ''), lastLink);
        }
      }
    } catch (e) {
      const message = String(e);
      line('[error] ' + message, 'err');
      showError(message, lastLink);
    }
    setBusyUi(false);
  }

  async function stopDownload() {
    if (!busy) return;
    if (btn) btn.disabled = true;
    if (dlLabel) dlLabel.textContent = 'Останавливаю…';
    try {
      await callApp('Stop');
    } catch (e) {
      line('[error] ' + e, 'err');
      if (btn) btn.disabled = false;
      if (dlLabel) dlLabel.textContent = 'Остановить';
    }
  }

  if (btn) {
    btn.onclick = async () => {
      if (busy) await stopDownload();
      else await startDownload();
    };
  }

  if (input) {
    input.addEventListener('keyup', (e) => {
      if (e.key !== 'Enter' || e.isComposing) return;
      startDownload();
    });
    input.addEventListener('input', () => {
      const links = updateInputChrome();
      clearError();
      clearPreviewTimer();
      if (links.length === 1) {
        schedulePreview();
      } else {
        hidePreview();
      }
    });
  }

  if (clearBtn) {
    clearBtn.onclick = () => {
      if (!input) return;
      input.value = '';
      input.focus();
      updateInputChrome();
      hidePreview();
      clearError();
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
        input.focus();
        await updatePreview();
      } catch (e) {
        line('[error] не удалось прочитать буфер обмена: ' + e, 'err');
      }
    };
  }

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

  // --- Folder / summary ---
  async function openFolder() {
    try {
      await callApp('OpenOutputDir');
    } catch (e) {
      line('[error] ' + e, 'err');
      showError(String(e), '');
    }
  }
  if (openFolderBtn) openFolderBtn.onclick = openFolder;
  if (summaryOpen) summaryOpen.onclick = openFolder;
  if (summaryAgain) {
    summaryAgain.onclick = () => {
      if (input) input.focus();
      setState('idle');
      clearError();
    };
  }
  if (retryBtn) {
    retryBtn.onclick = () => {
      if (!lastLink || !input) return;
      input.value = lastLink;
      input.focus();
      updateInputChrome();
      clearError();
      startDownload();
    };
  }
  if (errorClose) errorClose.onclick = clearError;

  if (details) {
    details.addEventListener('toggle', () => {
      if (details.open && log && typeof log.scrollHeight === 'number') {
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
        line('[error] не удалось скопировать лог: ' + e, 'err');
      }
    };
  }

  // --- Quality monitor ---
  function setQualityStatus(text) {
    const value = text || 'FLAC: проверок не было';
    if (sQualityStatus) sQualityStatus.textContent = value;
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
      undo.disabled = qualityChecking || busy;
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
      plural(qualityItems.length, 'трек', 'трека', 'треков') + ' в лучшем качестве.';
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
      } catch (e) {
        line('[error] ' + e, 'err');
        showError(String(e), '');
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
        .catch((e) => line('[error] ' + e, 'err'));
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
    if (sCheckNow) sCheckNow.disabled = busy;
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

  // --- Duplicate dialog ---
  function openDup(p) {
    if (!dupOverlay) return;
    if (dupPercent) dupPercent.textContent = p.percent + '%';
    if (dupTrack) dupTrack.textContent = p.track || '';
    if (dupMatchName) dupMatchName.textContent = p.fileName || '';
    if (dupMatchPath) {
      dupMatchPath.textContent = p.filePath || '';
      dupMatchPath.title = p.filePath || '';
    }
    if (dupFormat) {
      dupFormat.hidden = !p.differentFormat;
      if (p.differentFormat) {
        dupFormat.textContent = 'Найденный файл в формате ' +
          String(p.fileExt || '').toUpperCase() +
          ' — может получиться один трек в двух форматах.';
      }
    }
    dupOpen = true;
    dupOverlay.classList.add('open');
    dupOverlay.setAttribute('aria-hidden', 'false');
    activeModal = 'dup';
    if (dupNo) dupNo.focus();
  }

  function closeDup() {
    if (!dupOverlay || !dupOpen) return;
    dupOpen = false;
    dupOverlay.classList.remove('open');
    dupOverlay.setAttribute('aria-hidden', 'true');
    activeModal = null;
    if (input) input.focus();
  }

  async function answerDup(accept) {
    if (!dupOpen) return;
    closeDup();
    try {
      await callApp('AnswerDuplicate', accept);
    } catch (e) {
      line('[error] ' + e, 'err');
    }
  }

  if (dupYes) dupYes.onclick = () => answerDup(true);
  if (dupNo) dupNo.onclick = () => answerDup(false);
  if (dupOverlay) {
    dupOverlay.onclick = (e) => { if (e.target === dupOverlay) answerDup(false); };
  }
  events('duplicate-prompt', (p) => openDup(p));

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
    settingsSnapshot = settingsSnapshot === null ? null : settingsSnapshot;
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
    if (!settingsOverlay || busy) return;
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
      if (sCheckNow) sCheckNow.disabled = qualityChecking || busy;
      renderRejectedList(s.QualityRejected || []);
      updateQualityBadge(s.QualityPending || 0);
      if (s.QualityPending > 0 && qualityItems.length === 0) {
        callApp('PendingFound')
          .then((items) => renderQuality(items))
          .catch((e) => line('[error] ' + e, 'err'));
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
      line('[error] ' + e, 'err');
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
      if (sCheckNow) sCheckNow.disabled = true;
      try {
        await checkQualityNow();
      } catch (e) {
        line('[error] ' + e, 'err');
        if (settingsError) settingsError.textContent = String(e);
      } finally {
        if (sCheckNow) sCheckNow.disabled = qualityChecking || busy;
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
        line('[done] настройки сохранены', 'done');
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
      case 'dup': return dupCard;
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
    } else if (activeModal === 'dup') {
      e.preventDefault();
      answerDup(false);
    } else if (activeModal === 'confirm') {
      e.preventDefault();
      closeConfirm();
    }
  }
  document.addEventListener('keydown', onKeyDown);
  events('window-shown', () => { startBoot(); if (input) input.focus(); });

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
  setState('idle');
  if (input) input.focus();
  loadQualityStatus();

  // Небольшой тестовый/диагностический API: стенд tests/statusbar_check.js
  // исполняет этот файл в VM и проверяет состояние без глобального мусора.
  window.__yamdl = {
    getState: () => uiState,
    isBusy: () => busy,
    isBootDismissed: () => bootDismissed,
    parseTrackStatus,
    startDownload,
    stopDownload,
    closeSettings,
    isDirty,
    answerDup,
    openSettings,
    closeQuality,
    openQuality,
    openConfirm,
    closeConfirm,
    renderQuality,
    renderRejectedList,
    rejectQualityItem,
    undoRejected,
    getLastLink: () => lastLink,
    getQualityItems: () => qualityItems,
    getRejectedItems: () => rejectedItems,
    jobReset,
    handleJobStart,
    handleJobEnd,
    trackList,
    trackStatus,
    progress,
    showSummary,
    setState,
    setBusyUi,
    showError,
  };
})();
