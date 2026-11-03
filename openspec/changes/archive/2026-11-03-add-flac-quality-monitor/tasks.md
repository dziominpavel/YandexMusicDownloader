# Tasks

Все команды `go test` выполняются из каталога `src/`.

## 1. Конфигурация и индекс

- [x] 1.1 Добавить `FLAC_MONITOR` в `config.Config`/`config.Load` (default false, `1/true/yes/on`), проверить `config_test.go`: значение задано, отсутствует, мусор — проверка: `go test ./internal/config`
- [x] 1.2 Расширить `library.Entry` полями `TrackID`/`SourceURL` и научить `readTags` извлекать ID из FLAC (`YANDEX_TRACK_ID`/`COMMENT`), MP3 (UFID/COMM source-url) и M4A (source-url), с regex-fallback по URL; тесты на фикстурах с ID и без — проверка: `go test ./internal/library`
- [x] 1.3 Поднять `cacheVersion` до 2, добавить `Snapshot()`, `RemoveFile(path)` и исключение каталогов `_old` в `collect()`; тесты: старый кэш игнорируется, `_old` не индексируется, снимок не даёт гонок — проверка: `go test ./internal/library`
- [x] 1.4 Разделить наличие индекса и проверку дублей: `setupLibrary` строит индекс по `OUTPUT_DIR` при включённом `FLAC_MONITOR` даже без `LIBRARY_DIR`, а `checkDuplicate`/`filterDuplicates` работают только при `duplicatesEnabled`; тесты в `ui` на пустой `LIBRARY_DIR` с мониторингом и без — проверка: `go test ./internal/ui`
- [x] 1.5 Описать `FLAC_MONITOR` и `_old` в `.env.example` и `docs/` (новый раздел или дополнение к `docs/12-file-naming.md`/`docs/08-requirements.md`); проверка: документация содержит параметр, значение по умолчанию и ограничение «только при открытом окне»

## 2. Пробник lossless и поиск

- [x] 2.1 Добавить `downloader.ProbeLossless(ctx, trackID)` с классификацией по `codec` (`flac`/`flac-mp4` — доступен, `mp3`/`aac-mp4` — нет), ошибками `ErrPreview`/`ErrAuth`/`ErrForbidden`/`ErrUnavailable`/`ErrCheckFailed` и кэшем `accountUID`; тесты на `httptest`: flac, flac-mp4, mp3, aac-mp4, smart_preview, 401, 403, 404, 429, сеть — проверка: `go test ./internal/downloader`
- [x] 2.2 Добавить `downloader.SearchTracks(ctx, query)` для `GET /search?text=...&type=track&page=0`, парсинг `result.tracks.results`, фильтр `available`, маппинг в `Track` (ID, title, artists, album, year, cover); тесты на фикстурах поиска, включая пустой ответ — проверка: `go test ./internal/downloader`
- [x] 2.3 Экспортировать/добавить функцию скоринга на базе `library/match.go` (`ScoreCandidate`) с порогами title >= 0.90 и artist >= 0.90, нормализацией `norm.Comparable`/`ArtistsKey`; тесты: точное совпадение, 93%, ё/е, порядок исполнителей, разные версии — проверка: `go test ./internal/library`
- [x] 2.4 Добавить троттлинг и ограниченные ретраи для probe/search (200 мс между треками, без ретраев на 401/403/404); тест с фейковым clock/счётчиком запросов — проверка: `go test ./internal/downloader ./internal/quality`

## 3. Пакет quality: журнал и проверка

- [x] 3.1 Реализовать `quality.Journal`: JSON `flac-quality.json` рядом с `library-index.json`, версия схемы, атомарная запись, TTL (`no_lossless` 30 дней, `error` 1 день, `unavailable` 90 дней), `lastCompleted`/`lastAttempt`/`lastResult` (найдено N / ошибка), сброс по `accountUID`/`hasPlus`, `declined`; тесты на load/save, TTL, смену аккаунта, битый файл, сохранение результата — проверка: `go test ./internal/quality`
- [x] 3.2 Реализовать построение кандидатов из `Index.Snapshot()`: только `.mp3`, `_old` исключён, ID из индекса, поиск по `Artists`/`Title`, очистка запроса от `[320]`/`[m4a]`; тесты на временном дереве — проверка: `go test ./internal/quality`
- [x] 3.3 Реализовать `Checker.Check(ctx)`: ID первым, поиск только без ID, пропуск по TTL, probe, классификация ошибок (preview/auth → abort, 404/одиночный 403 → unavailable, серия 403 → abort, сеть → error), запись `lastCompleted`/`lastResult` даже при нуле находок и ошибке, прогресс/лог-колбэки; тесты с фейковыми `Prober`/`Searcher`/`Clock` — проверка: `go test ./internal/quality`
- [x] 3.4 Реализовать модель `FoundItem` и правила предвыбора: ID — все, поиск — только 100% нормализованное совпадение; до 3 кандидатов; тесты на смешанный список — проверка: `go test ./internal/quality`
- [x] 3.5 Реализовать проверку расписания (`due(lastCompleted, now, 7 дней)`) и защиту от параллельной проверки/занятости job'ой; тесты на границы 7 дней и busy — проверка: `go test ./internal/quality ./internal/ui`

## 4. Обновление файлов

- [x] 4.1 Вынести общий lossless-путь в `downloader.Upgrade(ctx, track, oldPath)`: probe, вычисление расширения (`.flac`/`.m4a`), конвертация ALAC→FLAC при ffmpeg и `CONVERT_M4A`, явный dest рядом со старым MP3, `ErrAlreadyExists`/`ErrDestConflict`; тесты с фейковым `httptest` и временным деревом — проверка: `go test ./internal/downloader`
- [x] 4.2 Реализовать `quality.Upgrader`: 2 воркера, `StartJob`/`Stop`, вызов `Upgrade`, запрет перемещения при `Fallback`/ошибке, перемещение старого файла в `<корень>\_old\<относительный путь>`, метка времени при коллизии, обновление индекса `RemoveFile`/`AddFile`; тесты на успех, fallback, stop, коллизию — проверка: `go test ./internal/quality`
- [x] 4.3 Обработать существующий `.flac` рядом: проверить TrackID, не скачивать второй, предложить/выполнить только перемещение старого MP3 в `_old`; тест на пару MP3+FLAC — проверка: `go test ./internal/quality`
- [x] 4.4 Обновлять журнал после обновления/ошибки и писать строки `[upgrade]`/`[skip]`/`[error]` с причиной; тесты на формат строк и статусы — проверка: `go test ./internal/quality`

## 5. UI: настройки, кнопка, окно

- [x] 5.1 Добавить `FlacMonitor` в `ui.Settings`, `GetSettings`, `SaveSettings` и `config.Write`; отдавать в `GetSettings` дату/время последней проверки и её результат; проверить, что сохранение не ломает комментарии/порядок `.env`, а выключение не отменяет идущее обновление — проверка: `go test ./internal/ui`
- [x] 5.2 Добавить биндинги `App.CheckQuality()` и `App.DownloadFound(items)`, события `quality-check-*`/`quality-found` с `lastChecked`/`found`/`error`; тесты с `eventSink`/`logSink`: ручная проверка, ошибка busy, авто-триггер после индекса, событие с нулём находок — проверка: `go test ./internal/ui`
- [x] 5.3 Добавить в `index.html` кнопку `FLAC` с бейджем, компактную строку `FLAC: <дата> — <результат>` рядом с ней, модалку сводки/списка (чекбоксы, `MP3 → FLAC`, поисковые детали, «Скачать все»/«Показать список»/«Позже», Escape/клик по фону) и раздел настроек с той же индикацией; проверка: `node --check` для извлечённого script и `go build ./...`; визуальный прогон — в 6.2
- [x] 5.4 Интегрировать обновление с существующей job'ой: `job-start` включает `busy` и «■ Остановить» даже при старте не по кнопке «Скачать», `job-end` сбрасывает, настройки заблокированы, фокус возвращается в поле ссылки; тесты/ручная проверка — проверка: `go test ./internal/ui` и `TestDownloadFoundArchivesExistingFlac`
- [x] 5.5 Проверить тексты ошибок и логов на русском: нет Плюса, preview, 403, сеть, нет `ffmpeg`, занятый старый файл; проверка: `TestHumanError` и `TestFormatEvent`

## 6. Интеграция и релиз

- [x] 6.1 Прогнать `go test ./...` в `src/` и `openspec validate --all`; проверка: обе команды завершаются успешно
- [x] 6.2 Ручной прогон на текущей коллекции: проверка находит 67 ID-совпадений в `downloads`, после неё обновляются дата/время и результат в главном окне и настройках (в том числе после перезапуска), обновление 1–2 треков даёт `.flac` с тегами, старый MP3 лежит в `_old`, повторная проверка не предлагает обновлённое — проверка: файлы, окно и строки лога (на момент прогона в `downloads` осталось 2 MP3; проверка нашла 3 поисковых совпадения, реальный апгрейд одного во временной копии дал FLAC с тегами и `_old`)
- [x] 6.3 Ручной прогон поиска по названию на 2–3 внешних MP3 из `F:`; проверить, что неверное совпадение можно снять, отклонённое не предлагается снова, а при отсутствии совпадения окно не открывается — проверка: окно/логи (3 точных совпадения, одно 403 обработано как недоступный трек; decline покрыт `TestPendingDeclinedIsSkipped`)
- [x] 6.4 Проверить stop/resume: остановить обновление после 1–2 треков, убедиться, что обновлённые остались, старые не перемещены для незавершённых, следующий запуск продолжает — проверка: временное дерево/логи (`TestUpgraderStopKeepsPendingForResume`)
- [x] 6.5 Добавить пользовательский блок и инженерные детали в `CHANGELOG.md` `## [Unreleased]`: мониторинг FLAC, окно предложения, `_old`, ограничение «только при открытом окне»; проверка: `python scripts/check-version.py` и формат блока

## Workflow follow-up

- После ревью и приёмки — архивировать change и обновить основные спеки.
