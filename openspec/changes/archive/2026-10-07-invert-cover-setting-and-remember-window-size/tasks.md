## 1. Конфигурация: `EMBED_COVER` и ключи размера

- [x] 1.1 В `config.Config`: поле `SkipCover` → `EmbedCover` (комментарий: дефолт включён), в `Load` — инициализация `EmbedCover: true`, кейс `EMBED_COVER` с off-списком `0/false/no/off` (зеркало `CONVERT_M4A`), кейс `SKIP_COVER` удалён; добавить `WindowWidth`/`WindowHeight` и кейсы `WINDOW_WIDTH`/`WINDOW_HEIGHT` (положительные, в разумных пределах; мусор = «не задано») — `go test ./internal/config/` зелёный
- [x] 1.2 Тест `TestLoadSkipCover` → `TestLoadEmbedCover`: `""` → true, `EMBED_COVER=false|0|no|off` → false, `EMBED_COVER=true|TRUE|yes|1` → true, `EMBED_COVER=maybe` → true (невалидное игнорируется) — `go test ./internal/config/` зелёный
- [x] 1.3 Тест `TestWritePreservesCommentsAndUnknownKeys`: ключ заменён на `EMBED_COVER=false`, round-trip проверяет `!cfg.EmbedCover`; отдельный тест на round-trip `WINDOW_WIDTH`/`WINDOW_HEIGHT` — `go test ./internal/config/` зелёный

## 2. Тегер и клиент

- [x] 2.1 `tagger.Adapter(skipCover bool)` → `tagger.Adapter(embedCover bool)`, внутри `if !skipCover` → `if embedCover` — `go test ./internal/tagger/` зелёный
- [x] 2.2 В `ui/app.go` вызов `tagger.Adapter(cfg.EmbedCover)`; удалить мёртвое поле `downloader.Client.SkipCover` в `track.go` — `go build ./...` зелёный, `go test ./internal/ui/ ./internal/downloader/` зелёный

## 3. Окно настроек (бэкенд)

- [x] 3.1 Поле `Settings.SkipCover` → `EmbedCover`, ключ updates-map `"EMBED_COVER"`, перенос в `GetSettings`/`SaveSettings` — `go test ./internal/ui/` зелёный
- [x] 3.2 В `app_job_test.go` инвертировать проверки: дефолт `EmbedCover == true`, сохранение `EmbedCover: false` даёт строку `EMBED_COVER=false` в `.env`, восстановление через `GetSettings` — `go test ./internal/ui/` зелёный

## 4. Окно настроек (фронтенд)

- [ ] 4.1 В `src/frontend/dist/index.html`: подпись «Встраивать обложку (текстовые теги пишутся)», id `sSkipCover` → `sEmbedCover` во всех трёх местах (объявление, заполнение, отправка), `checked = !!s.EmbedCover`, в `SaveSettings` — `EmbedCover: sEmbedCover.checked` — открыть окно настроек: галка стоит по умолчанию; снять её, сохранить, проверить в `.env` строку `EMBED_COVER=false`

## 5. Размер окна

- [x] 5.1 В пакете `ui`: константы `DefaultWindowWidth = 864`, `DefaultWindowHeight = 576` и `InitialSize() (int, int)` — возвращает размер из `cfg`, если оба значения корректны, иначе дефолт; тест на три ветки (нет ключей → 864×576, заданы → их, мусор/неполный набор → дефолт)
- [x] 5.2 `BeforeClose(ctx context.Context) bool`: если `runtime.WindowIsNormal(ctx)`, взять `runtime.WindowGetSize(ctx)` и через чистую функцию (размер + последний сохранённый + путь к `.env`) записать `WINDOW_WIDTH`/`WINDOW_HEIGHT` через `config.Write` при отличии; хук всегда возвращает `false`; без `envPath` — тихий пропуск; unit-тест чистой функции
- [x] 5.3 В `src/cmd/app/main.go`: `Width`/`Height` из `app.InitialSize()`, `OnBeforeClose: app.BeforeClose` — `go build ./...` зелёный
- [ ] 5.4 Ручная проверка: удалить из `.env` ключи размера → запуск 864×576; ручной ресайз → закрыть → открыть = тот же размер; максимизировать → закрыть → открыть = прежний обычный размер

## 6. Документация

- [x] 6.1 `.env.example`: строка `SKIP_COVER=false` заменена на `EMBED_COVER=true` с комментарием про дефолт и окно настроек; добавить закомментированные `WINDOW_WIDTH`/`WINDOW_HEIGHT` с пометкой «заполняется автоматически при закрытии окна» — `grep SKIP_COVER .env.example` пуст
- [x] 6.2 `CHANGELOG.md`, секция `## [Unreleased]`: новый пункт про запоминание размера окна (864×576 при первом запуске), в существующем пункте про окно настроек `SKIP_COVER` → `EMBED_COVER` и формулировка чекбокса «Встраивать обложку» — `grep SKIP_COVER CHANGELOG.md` пуст

## 7. Проверка и валидация

- [x] 7.1 `go test ./...` в `src/` зелёный, `gofmt`/`go vet` чистые
- [x] 7.2 `openspec validate --all` зелёный; `python scripts/check-version.py` — версию не трогаем (не релиз)
- [ ] 7.3 Ручная сценарная проверка: `.env` со старым `SKIP_COVER=true` — обложка встраивается (ключ игнорируется); скачать трек с дефолтным `.env` — картинка в теге есть; снять «Встраивать обложку» → сохранить → скачать → картинки нет, текстовые теги на месте
- [x] 7.4 Проверка полноты: `grep -r SkipCover src/` пуст — старого поля в коде и тестах не осталось; `grep -r SKIP_COVER src/` даёт ровно одну строку — тест-кейс, доказывающий, что старый ключ игнорируется (сценарий «Старый ключ не действует» из спеки `audio-tagging`), в рабочем коде старого ключа нет
