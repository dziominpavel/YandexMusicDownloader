# Changelog

Формат: Keep a Changelog, уровни — семантическое версионирование.
Правила ведения: `docs/versioning.md`.

## [Unreleased]

**Исправлено**
- Поиск дублей теперь замечает радиоверсию: «Not Afraid» и «Not Afraid Radio Edit» предупреждают о возможном дубле.

### Исправлено
- `add-version-duplicate-matching`: `norm.RadioEditBase` снимает только хвостовой Radio Edit (без скобок, в `(…)`/`[…]`, с именем ремиксера), `library.FindMatches` даёт fallback 95% при схожести исполнителей ≥0.90; remix/live/remastered/version и служебные пометки не трогаются. Тесты: `TestRadioEditBase`, `TestFindMatchesRadioEdit*`.

## [1.0.0] — 2026-11-04 (MAJOR)

**Добавлено**
- Приложение следит за MP3 в коллекции и предлагает заменить их на FLAC, если Яндекс добавил lossless; старые файлы сохраняются в _old.
- В окне найденного FLAC можно отметить «Не тот трек» и вернуть ошибочно отклонённое совпадение в настройках.
- Файловый лог рядом с приложением: сбои и ошибки теперь можно разобрать после закрытия окна.
- Enter в поле ссылки запускает скачивание, а поле само получает фокус при старте и после загрузки.
- Анимация запуска: приложение открывается с короткой заставкой, а проверка токена больше не задерживает окно.

**Исправлено**
- В логе видно, почему вместо FLAC скачался MP3: показывается качество, кодек и битрейт, которые вернул Яндекс.
- Ссылка на источник в тегах содержит обычный номер альбома, без записи вида 6.73558e+06.
- Окно возможного дубля показывает полный путь и имя найденного файла целиком, даже если путь длинный.
- FLAC-обновление теперь попадает в папку загрузок, а не в папку коллекции; старый MP3 уходит в _old.
- Иконка приложения снова отображается в панели задач и на закреплённом ярлыке.
- Статус проверки FLAC больше не зависает на «проверка идёт…» после завершения.

**Изменено**
- **BREAKING:** история последних ссылок убрана: поле ввода больше не перекрывает лог выпадающим списком.
- Новый тёмный интерфейс Warm Vinyl: заметная кнопка скачивания, понятные состояния загрузки и компактный список треков.
- Настройки разбиты на секции в две колонки, сохранение всегда на виду, а случайное закрытие с изменениями просит подтверждение.
- Ошибки теперь видны рядом с полем ввода, лог убран в «Подробности», а exe получил иконку и версию.
- Проверка FLAC перенесена в настройки, найденное показывается бейджем на кнопке настроек.
- Проверка из настроек больше не закрывает форму и не теряет несохранённые изменения.

### Добавлено
- `add-quality-reject-undo`: явное отклонение поискового совпадения кнопкой «Не тот трек» без скачивания; журнал `flac-quality.json` v2 хранит список отказов на локальный файл с метаданными и без автоистечения, миграция v1 сохраняет активные отказы. `Journal.Forget("file:"+path)` сбрасывает lossless-запись, поэтому MP3 остаётся кандидатом; `Checker` перепроверяет отклонённую запись, а при «все кандидаты отклонены» ставит `no_match` на 7 дней. В настройках появился список «Отклонённые совпадения» с кнопкой «Вернуть»; `UndoReject` восстанавливает предложение без сети. Тесты: `TestJournalDeclined`, `TestJournalDeclinedPersistence`, `TestJournalMigratesV1`, `TestCheckerRechecksDeclinedLossless`, `TestCheckerDeclinedCandidateFallsThrough`, `TestRejectFoundKeepsLocalFileAndClearsLossless`, `TestUndoRejectRestoresImmediateOffer`, `tests/quality_reject_check.js`, обновлён `tests/settings_check.js`.
- `add-flac-quality-monitor`: новая capability `quality-upgrade` и параметр `FLAC_MONITOR` (по умолчанию false): проверка MP3 из `LIBRARY_DIR` и `OUTPUT_DIR` по Yandex ID или поиску `artist + title`, журнал `flac-quality.json` с TTL и сбросом по аккаунту/Плюсу, окно сводки/списка с чекбоксами, статус последней проверки в главном окне и настройках. Обновление идёт через `downloader.Upgrade`: lossless → ALAC→FLAC при `flac-mp4` → теги → атомарная публикация рядом со старым MP3 → старый файл в `<корень>\_old\<относительный путь>`; при fallback/ошибке старый файл не перемещается. Индекс хранит `TrackID`/`SourceURL`, поднят `cacheVersion` до 2, каталоги `_old` исключены. Новые пакеты `internal/tagread` и `internal/quality`; тесты `internal/{config,library,downloader,quality,ui}`. Документация: `.env.example`, `docs/15-flac-monitor.md`.
- `add-file-log`: пакет `internal/applog` пишет все строки окна в `app.log` рядом с exe (fallback `%LOCALAPPDATA%\YandexMusicDownloader\app.log`), формат RFC 3339 + `[уровень]`, ротация 5 МиБ в `app.log.1` с одной резервной копией, редакция `OAuth ...` и зарегистрированного токена до записи в файл и окно, шапка с версией/VCS-ревизией, ОС, архитектурой и путями, строка `[log] файл: ...` при старте. Тесты: `TestWriteFormat`, `TestWriteMultiline`, `TestRedactOAuth`, `TestRedactSecretValue`, `TestRotate`, `TestRotateReplacesBackup`, `TestOpenFallback`, `TestOpenAllFail`, `TestConcurrentWrites`, `TestLogWritesFile`, `TestStartupLogsFilePath`, `TestLogRedactsToken`.
- `enter-to-download`: в `src/frontend/dist/index.html` старт вынесен в `startDownload()`, Enter на `#link` вызывает его через `keyup`, кнопка остаётся тумблером старт/стоп; при `busy` Enter игнорируется и сохраняет текст в поле; фокус возвращается при старте окна, после `job-end`, закрытия окна дубля и настроек. Стенд `tests/statusbar_check.js` получил `dispatch`/`activeElement` и сценарии Enter; README обновлён. Проверки: `node tests/statusbar_check.js`, `go test ./...`, `openspec validate --all`, `python scripts/check-version.py`.
- `add-startup-animation`: слой `#boot` в `src/frontend/dist/index.html` с CSS/SVG-анимацией Warm Vinyl ~1 с (диск, wordmark, fade 160 мс), `boot--run`/`boot--done`, fail-open через CSS-таймаут 3 с и немедленное снятие в тестовой VM без `setTimeout`, `prefers-reduced-motion` — статичный знак 400 мс. `StartHidden: true` + `OnDomReady -> WindowShow` показывают окно, когда страница готова. `NewApp` больше не вызывает `config.Validate`: клиент создаётся сразу, `Startup` запускает `verifyStartupToken` в фоне с `tokenGen`/`authErr`, `Download`/`PreviewLink` возвращают русскую ошибку при невалидном токене. Тесты: `tests/splash_check.js`, `node tests/statusbar_check.js`, `TestApplyStartupDoesNotValidateToken`, `TestVerifyStartupTokenSuccess`, `TestVerifyStartupTokenFailure`, `TestVerifyStartupTokenIgnoresStaleResult`, `TestDownloadRejectsInvalidToken`, `TestPreviewLinkRejectsInvalidToken`, `TestStartupChecksTokenInBackground`. Проверки: `go test ./...`, `go vet ./...`, `go build ./...`, `openspec validate --all`, `python scripts/check-version.py`.

### Исправлено
- `fallback-reason-and-album-ids`: `DownloadTrack` сохраняет причину отказа lossless (`quality`, `codec`, `bitrate` из `/get-file-info`) в `Result.FallbackReason`, пишет её в строку `[fallback MP3: ...]` и отдельной строкой `[fallback]` для плейлистов и альбомов; ID из JSON (`any`) форматируются helper'ом `idString` (`ids.go`), поэтому `float64` больше не попадает в теги в экспоненциальной записи (`track.go`, `playlist.go`). Тесты: `TestIDString`, `TestLosslessFallbackReason`, `TestJobCallbacksLogFallbackReason`, обновлён `TestFormatEvent`.
- `duplicate-dialog-full-path`: подтверждение дубля переведено с нативного `MessageBox` (обрезал длинные пути) на окно в `src/frontend/dist/index.html`: `duplicatePrompt` (процент, трек, имя и полный путь найденного файла, формат) уходит событием `duplicate-prompt`, ответ приходит через `AnswerDuplicate`; полный путь переносится по любому символу, имя файла выделено. Тесты: `TestDuplicatePromptWaitsForFrontendAnswer`, обновлены `TestCheckDuplicateBranching` и `TestDuplicatePromptMentionsFormat`; удалён `TestConfirmAnswerAcrossPlatforms` (нативного диалога больше нет).
- `fix-ui-and-flac-upgrade` (назначение): `downloader.Upgrade` принимает `destDir` и использует обычное имя/`SUBDIRS`; `quality.Upgrader` передаёт `OUTPUT_DIR`; старый MP3 и его `.lrc` уходят в `_old` в корне старого файла. Иконка окна: группа ресурсов `#3` вместо `APP`, чтобы Wails нашёл её по `AppIconID`. Статус: `checking` сбрасывается до `qualityStatusView`, поэтому `quality-check-finished` несёт фактический результат. Тесты: `TestUpgradeDownloadsToDestDir`, `TestUpgraderLibraryFileGoesToOutputDir`, `TestQualityFinishedEventTextError`, `TestQualityFinishedEventTextCanceled`, `TestDownloadFoundKeepsFinalStatusAfterPartialFailure`.

### Изменено
- `remove-link-history`: из `src/frontend/dist/index.html` удалены `datalist`, `list="linkHistory"` и JS-блок истории (`HISTORY_KEY`, `loadHistory`, `renderHistory`, `rememberLink`, вызов при старте скачивания); требование `track-download` обновлено, README и `tests/statusbar_check.js` поправлены. Проверки: `node tests/statusbar_check.js`, `go test ./...`, `go vet ./...`, `go build ./...`, `openspec validate remove-link-history --type change`, `openspec validate --specs`.
- `redesign-ui`: фронтенд разделён на `src/frontend/dist/{index.html,app.css,app.js}`; добавлена визуальная система Warm Vinyl, state machine `idle/preview/active/done/error`, список треков с человекочитаемыми статусами и форматами, итоговая карточка, inline-ошибка/повтор, свёрнутый лог «Подробности», SVG-иконки, ARIA/focus trap/reduced motion, двухколоночные настройки с dirty guard. Wails получил `MinWidth/MinHeight 860×600`, тёмный `BackgroundColour` и `Windows.Theme`; `scripts/build-windows.ps1` генерирует иконку/версию/PerMonitorV2-манифест через `go-winres`, добавлены `assets/icon.ico` и `assets/winres/`. Тесты: `node tests/statusbar_check.js`, `node tests/settings_check.js`, `go test ./...`, `go vet ./...`, `go build ./...`, `openspec validate --all`.
- `fix-ui-and-flac-upgrade` (UI): из шапки удалены кнопка/бейдж/статус FLAC; найденное показывается бейджем на `#settingsBtn`, окно открывается действием «Открыть найденное (N)» в настройках. `quality-found`/`quality-pending` больше не открывают модалку сами. «Проверить сейчас» не закрывает форму и блокируется на время проверки; окно найденного показывает `OUTPUT_DIR` и правило `_old`. Шестерёнка заменена на cog. Стенд `tests/quality_check.js`; проверки: `node tests/quality_check.js`, `node tests/settings_check.js`, `node tests/statusbar_check.js`, `go test ./...`.

## [0.3.0] — 2026-10-28 (MINOR)

**Добавлено**

- Текст песни встраивается в файл при скачивании, если он есть на Яндекс Музыке.
- Можно включить подпапки по исполнителю и альбому, сохранять текст отдельным .lrc и экспортировать плейлист в .m3u.
- Ссылку можно перетащить в окно, последние ссылки доступны в истории, перед скачиванием видно обложку и название.
- Рядом с настройками появилась кнопка открытия папки загрузок.
- В окне настроек появилась справка «Как получить токен?» с пошаговой инструкцией и кнопкой открытия страницы OAuth.

**Изменено**

- Архив релиза теперь распаковывается в папку YandexMusicDownloader: внутри app.exe и ffmpeg.exe.
- Основное окно при первом запуске стало на 10% больше — настройки помещаются без прокрутки.

### Добавлено

- `lyrics-folders-and-ux-improvements`: лирика из `GET /tracks/{id}/lyrics` (TEXT/LRC) встраивается в теги (`LYRICS`/`USLT`/`©lyr`) при `EMBED_LYRICS=true` (по умолчанию), опционально пишется `.lrc` рядом (`SAVE_LRC`); при `SUBDIRS=true` файлы раскладываются по `SUBDIR_TEMPLATE` (`%artist%`, `%album%`, `%year%`), при `EXPORT_M3U=true` альбом/плейлист получает `.m3u`. UX без настроек: drag&drop, история ссылок в localStorage, предпросмотр обложки/названия, кнопка открытия папки загрузок. Тесты: `TestLoadBehaviorFlags`, `TestDestPathSubdirs`, `TestLRCToText`, `TestSplitLinks`, `TestExportM3U`, проверки лирики в roundtrip-тестах теггера.
- `add-token-help`: встроенная справка у поля токена в `src/frontend/dist/index.html`; подсказка про 30-секундное превью при пустом токене; OAuth-страница открывается в системном браузере. Сообщения без токена в `src/internal/ui/app.go` теперь ведут в «⚙ Настройки → Как получить токен?». Тесты: `TestDownloadWithoutToken`, `TestReadyWithoutTokenMentionsHelp`; обновлены `docs/11-get-token.md` и `docs/friends-setup.md`.

### Изменено

- `add-zip-root-folder`: `scripts/pack-zip.ps1` собирает zip через `System.IO.Compression.ZipArchive` с единственным корневым каталогом `YandexMusicDownloader/`; после сборки структура проверяется, при расхождении zip удаляется и скрипт завершается с ошибкой. Проверены упаковка, распаковка, `go test ./...` и запуск из папки.
- Окно первого запуска: дефолт 950×634 вместо 864×576, чтобы форма настроек помещалась без прокрутки; обновлены `openspec/specs/desktop-app/spec.md` и `.env.example`.

## [0.2.0] — 2026-10-07 (MINOR)

Загрузку можно остановить прямо посреди передачи — недокачанные треки
помечаются `[stopped]`, а не ошибкой. Статусбар показывает проценты и байты
для любого скачивания, настройки собраны в одном окне, дубли проверяются
по своей коллекции; на Windows подтверждение скачивания снова работает.

### Добавлено

- Кнопка остановки активной загрузки: пока идёт скачивание, кнопка
  «Скачать» превращается в красную «■ Остановить» (и «Останавливаю…»
  на время обработки команды) вместо блокировки. Остановка обрывает уже
  идущую передачу, не даёт плейлисту запустить очередные треки —
  недокачанные отмечаются `[stopped]`, а не ошибкой, итог пишется как
  `[stopped] прервано: ...`, а статусбар замораживается на фактическом
  проценте (не доводится до 100%). Следующая загрузка работает без
  перезапуска.
- Агрегированный статусбар прогресса: полоса на всю ширину между строкой
  ввода и логами — процент, счётчик скачанных треков (`N/M`) и байты
  обновляются в реальном времени для любого скачивания, будь то одиночный
  трек или весь плейлист. Одиночный трек теперь тоже имеет проценты
  (раньше прогресс был только у плейлистов). После завершения статусбар
  замораживается на итоге, ошибки подсвечиваются.
- Окно настроек (кнопка «⚙ Настройки»): токен с проверкой, папки загрузок
  и коллекции с выбором через системный диалог, число параллельных
  скачиваний, переключатели `CONVERT_M4A` и `EMBED_COVER` (чекбокс
  «Встраивать обложку (текстовые теги пишутся)», по умолчанию включён).
  Настройки сохраняются в `.env` (комментарии и чужие ключи не
  теряются) и применяются без перезапуска; на время скачивания окно
  заблокировано. Прежний инвертированный ключ заменён на `EMBED_COVER`
  с противоположным смыслом (дефолт — встраивать), мёртвый `FORMAT`
  убран из `.env.example`.
- Запоминание размера окна: при первом запуске окно открывается
  864×576 (было 720×480), а последний обычный размер сохраняется в
  `.env` (`WINDOW_WIDTH`, `WINDOW_HEIGHT`) при закрытии и
  восстанавливается при следующем запуске. Развёрнутое окно размером
  не запоминается.
- Проверка на дубли по своей коллекции: параметр `LIBRARY_DIR` в `.env`
  (папка с подкаталогами) — перед скачиванием приложение ищет похожие файлы
  и честно спрашивает подтверждение (процент совпадения, путь найденного
  файла, если формат отличается). Индексация идёт в фоне с прогрессом
  в логе, повторный запуск использует кэш, свежескачанные файлы
  попадают в индекс сразу. Без `LIBRARY_DIR` фича выключена полностью.

### Исправлено

- Фикс подтверждения дублей на Windows: системный диалог возвращает
  `Yes`/`No` (Wails игнорирует наши надписи кнопок), а ответ сравнивался
  со строкой «Скачать» — поэтому любой клик давал
  `[skip] ... (дубль, отменено пользователем)` и скачать было нельзя.
  Теперь `Yes`/«Скачать» подтверждает скачивание, `No`/«Отмена» — нет.

## [0.1.0] — 2026-10-02

Первый опубликованный тег: содержание релиза — в сводной секции ниже.

## История до введения changelog

- Десктопное приложение на Go и Wails: вставьте ссылку на трек Яндекс
  Музыки — файл появится в `downloads/`.
- Скачивание в FLAC с автоматическим фолбэком на MP3, конвертация
  ALAC → FLAC.
- Теги и обложка записываются в файл, имена остаются плоскими и
  читаемыми.
- Скачивание целых плейлистов по uuid-ссылкам: пул воркеров, общий
  прогресс, одна строка на трек.
- Упаковка релиза: `scripts/pack-zip.ps1` собирает `app.exe` и
  `ffmpeg.exe` в `dist/YandexMusicDownloader-<версия>-win-x64.zip`.
- Подключение витрины: `store.yaml`, иконка 512×512, скриншоты.
