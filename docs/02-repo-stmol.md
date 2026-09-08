# 02. Stmol / yandex-music-downloader — базовый проект

Репо: https://github.com/Stmol/yandex-music-downloader
Статус: ~82 звезды, 40 коммитов. Единственный проверенный автором (есть рабочий exe).

## Стек

- Go, `module ya-music`, `go 1.25.0`.
- TUI: `bubbletea/v2` + `bubbles/v2` (list, progress, spinner) + `lipgloss/v2`.
- Теги: `bogem/id3v2` (MP3), `go-flac/go-flac + flacvorbis + flacpicture` (FLAC), `tommyo123/mtag` (M4A) + свой достройщик `moov/udta/meta/ilst`.
- Свое HTTP: `utils.HttpClient` (токен, таймаут, отмена), `slog`.
- Запуск: `go run ./cmd/yamdl`, `go test ./...`. Флаги `--timeout`, `--skip-cover`.

## Структура

```
cmd/yamdl/main.go        -> cli.Run()
internal/cli/            -> run.go, flags.go, download.go, tui.go, signals.go
internal/batch/          -> headless-движок для CLI (канал событий, семафор 3)
ya/                      -> клиент + скачивание + теги
ya/model/                -> track, album, playlist, download_info, track_download_info...
ya/lossless/             -> get-file-info + AES
source/                  -> url.go (5 regex), resolve.go
ui/                      -> tui.go, token.go, source.go, download.go (~1200 строк), download_session.go
utils/                   -> http.go, token.go (token.txt 0600), file.go, logger.go
```

## Как качает

Токен: `Authorization: OAuth <token>`. TUI умеет читать `token.txt`, пустой ввод = без токена (только 30 сек). CLI: `--token` обязателен. Для lossless еще нужен `userUID` из `AccountStatus()` → заголовок `x-yandex-music-multi-auth-user-id`.

Парсинг ссылок (`source/url.go`): трек `.../album/{albumId}/track/{trackId}`, альбом, плейлист legacy/uuid, чарт `.../chart[/region]`. Хосты `music.yandex.(ru|com|kz|by|uz)`.

MP3 (`ya/client.go:downloadTrackMP3`):
1. Имя `Artist - Title.mp3` (санитизация).
2. `FileExists` → skip.
3. `GET /tracks/{id}/download-info` → выбор max bitrate (обычно 320).
4. `GET DownloadInfoURL` → XML → `host,path,ts,s` → подпись `md5(XGRlBW9FXlekgbPrRHuSiA + path[1:] + s)` → `https://{host}/get-mp3/{sign}/{ts}{path}`.
5. Запись через `publishAudioArtifact` (см. ниже).

Lossless (`ya/lossless/lossless.go`):
1. `GET /get-file-info?ts&trackId&quality=lossless&codecs=flac,aac,...&transports=raw&sign=HMAC-SHA256("7tvSmFbyf5hJnIHhCimDDD", ts+trackId+lossless+codecs+raw) base64 без =`.
2. Берут только `flac|flac-mp4`, иначе ошибка.
3. `flac-mp4 → .m4a`, иначе `.flac`.
4. Перебор URL, `AES-CTR IV=0` если есть `key`, проверка префикса `fLaC`.
5. Fallback: любая ошибка lossless → качаем MP3. Статус `✅ MP3` в lossless-режиме = fallback.

Атомарность (`audio_artifact.go`): `CreateTemp(.artifact-*)` → запись аудио (обложка качается параллельно) → теги → `Rename`. Недокачанное не светится в `downloads/`.

CLI: `yamdl download --token --link --format mp3|flac --output ./downloads`. Append-only строки `[downloading]/[done]/[already exists]/...`, финал `Finished: X downloaded...`. Двойной `Ctrl+C` (стоп планирования → разрыв HTTP).

TUI: 3 экрана (токен → ссылка → список). Формат один на очередь (`1/2`), `D` — скачать все, `q` — отмена, `?` — помощь. `outputDir=./downloads` захардкожен.

## Теги (где боль)

- MP3: `TIT2, TPE1(одной строкой), TALB, TCON, TRCK(без total), TYER, UFID, COMM(source-url), APIC 400x400`. Политика: ошибка тега = нет файла.
- FLAC: Vorbis `TITLE, ARTIST, ALBUM, ALBUMARTIST, GENRE, TRACKNUMBER, DISCNUMBER, DATE, YANDEX_TRACK_ID, COMMENT`. Старые Vorbis/Picture сносятся.
- M4A: `©nam, ©ART, aART, ©alb, ©gen, ©day, trkn, disk, source-url, covr`. Политика best-effort (битый файл публикуется с warn).
- Чего нет: текстов, композитора, `TRACKTOTAL` (кроме M4A), `ALBUMARTIST` для MP3, мультиартиста, иерархии `Album/01 - Title`.

Обложка всегда `400x400`, `--skip-cover` оставляет текстовые теги.

## Сильные стороны

- Два режима из одного бинарника, `token.txt`, скриптуемый CLI.
- Оба протокола реверснуты + тесты.
- Дедуп, `AlreadyExists`, суффикс `[id]` при коллизии, конкурентные закачки (3).

## Слабые (подтверждают жалобы)

- Бедные/несимметричные теги, плоские имена, обложка 400px, M4A-тихий фейл тегов, формат один на очередь, нет смены папки в TUI.

## Что берем

1. `buildDirectLink + pickBestBitrate` и `BuildFileInfoURL/SignRequest/DecryptData` как есть.
2. `publishAudioArtifact (temp→tag→rename)`.
3. `source/url.go + resolve.go` парсер.
4. `batch.Run` движок + CLI-протокол `[downloading]/[done]`.
5. `cover.go` конкурентную докачку.
6. `signals.go` двойной Ctrl+C.
