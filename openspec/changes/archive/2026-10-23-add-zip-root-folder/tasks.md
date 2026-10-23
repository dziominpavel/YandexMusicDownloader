# Tasks

## 1. Упаковка архива

- [x] 1.1 В `scripts/pack-zip.ps1` заменить `Compress-Archive` на `System.IO.Compression.ZipArchive`: подключить `System.IO.Compression` и `System.IO.Compression.FileSystem`, создать `dist/YandexMusicDownloader-<version>-win-x64.zip` с записями `YandexMusicDownloader/app.exe` и `YandexMusicDownloader/ffmpeg.exe` (`CompressionLevel::Optimal`), архив открывать в `try/finally`. Проверка: `powershell.exe -NoProfile -File scripts\pack-zip.ps1` создаёт zip; при чтении через `[System.IO.Compression.ZipFile]::OpenRead` в нём ровно две записи с прямыми слэшами.
- [x] 1.2 Добавить пост-проверку: после закрытия архива открыть его на чтение и сверить отсортированный список `FullName` с ожидаемым; при расхождении — сообщение `[ERROR] ...`, удалить zip и `exit 1`. Проверка: временно изменить ожидаемое имя записи — скрипт возвращает 1, zip удалён; вернуть ожидаемый список — снова 0.
- [x] 1.3 Проверить обработку ошибок: при отсутствии `app.exe`/`ffmpeg.exe` и при ошибке записи zip не остаётся частичный файл в `dist/`; `try/finally` освобождает архив. Проверка: запуск без `ffmpeg.exe` (временно переименовать) даёт понятную ошибку и не создаёт zip; вернуть файл.
- [x] 1.4 Собрать реальный архив и распаковать во временную папку: внутри ровно `YandexMusicDownloader/app.exe` и `YandexMusicDownloader/ffmpeg.exe`, в корне распаковки нет `app.exe`/`ffmpeg.exe`, размер сопоставим с прежним (~40 МБ). Проверка: `Expand-Archive` + `Get-ChildItem -Recurse`.

## 2. Документация и changelog

- [x] 2.1 В `docs/friends-setup.md` обновить вступление и шаг 2: после распаковки архива открыть `YandexMusicDownloader\app.exe`; `.env` и `downloads/` создаются рядом с exe. Проверка: инструкция проходится по свежему zip без обращения к репозиторию; упоминаний «два файла в одной папке» не осталось.
- [x] 2.2 В `CHANGELOG.md` в `## [Unreleased]` добавить пользовательский блок `**Изменено**` (одна строка ≤140 символов, без backticks: архив распаковывается в папку `YandexMusicDownloader`) и инженерные детали под `### Изменено`: change-id, `pack-zip.ps1`, проверка структуры, тесты. Проверка: `py -3 scripts/check-version.py` — exit 0.

## 3. Интеграционная проверка

- [x] 3.1 `openspec validate --all` — зелёный; `go test ./...` в `src/` — для уверенности, что упаковка не затронула сборку (Go-код не менялся). Проверка: обе команды без ошибок.
- [x] 3.2 Полный релизный путь без публикации: `scripts/build-windows.ps1` → `scripts/pack-zip.ps1` → прочитать zip, распаковать, запустить `YandexMusicDownloader\app.exe`; окно открывается, после сохранения настроек `.env` появляется рядом с exe, `downloads/` — там же. Проверка: визуально и через `Get-ChildItem`.
- [x] 3.3 Проверить `dist/` после сборки: старый zip перезаписан, лишних файлов нет; `release.ps1` видит ровно один ассет `YandexMusicDownloader-<version>-win-x64.zip`. Проверка: `Get-ChildItem dist -File` и `git status --short`.

## Workflow follow-up

- Свести правки `docs/friends-setup.md` и `CHANGELOG.md` с change'ем `add-token-help` (оба меняют эти файлы), затем архивировать оба change'а.
