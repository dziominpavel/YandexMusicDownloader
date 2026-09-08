# 03. nnikitochka / YandexMusicDownloader — донор тегов

Репо: https://github.com/nnikitochka/YandexMusicDownloader
Статус: 5 звезд, 112 коммитов. Проверено на Linux + Termux/Android.

## Стек

- Kotlin/JVM, `jvmToolchain(21)`, Gradle Kotlin DSL, версия 2.1.
- Модули: `ymd-api` (библиотека, публикуется в Maven) + `ymd-console-client` (fat-jar через Shadow, `Main-Class=Launcher`).
- `ymd-api`: `okhttp:4.12.0`, `gson:2.10.1`, `jaudiotagger:3.0.1`, `slf4j + YetAnotherLogger`.
- `console`: `toml4j`, `oshi-core` (детект Linux для ffmpeg), `jline:3.26.1`.

## Структура

```
ymd-api/.../api/
  YandexMusicClient.kt       # весь HTTP
  download/YandexMusicDownloader.kt / AbstractMusicDownloader.kt
  objects/Track.kt, Quality.kt, DownloadInfo.kt
  link/LinkParser.kt         # split " && " для батча, старый/новый формат ссылок
  ffmpeg/FfmpegProvider.kt
  utils/AudioTagWriter.kt, GenreTranslator.kt
  lrclib/                    # парсер LRC (пока не используется в загрузке)
ymd-console-client/.../
  Launcher.kt                # main, validateToken, ffmpeg bootstrap
  MusicDownloader.kt         # очередь + фоновый поток
  config/TomlConfig.kt       # config.toml
  terminal/...               # JLine контексты: Running, GenreSelect, Confirm...
```

Модели: `Track(id,title,version,artists,albums,coverUri,genre)`, `fullTitle=title+(version)`, `Quality(LOW/NORMAL/HIGH/LOSSLESS)`, `DownloadInfo(quality,codec,bitrate,key,url)`.

## Как качает

Токен из `config.toml`, проверка `GET /account/status`. User-Agent `YandexMusicDesktopAppWindows/5.54.0`.

`getFileInfo`: `sign=HMAC_SHA256(SECRET=kzqU4XhfCaY6B6JTHODeq5, ts+trackId+quality+"flacaache-aacmp3...encraw")` → `GET /get-file-info?...&codecs=flac,aac,...&transports=encraw`.

FLAC есть (`Quality.LOSSLESS` по умолчанию). `codec` вида `flac`, `flac-mp4`, `aac-mp4` → `shouldMux = contains("mp4")` → `ffmpeg -i input -c:a copy output`. Честный FLAC при `codec==flac`.

Расшифровка: `AES/CTR/NoPadding, key=hex, IV=16 нулей`. Скачивание через `HttpURLConnection`, `readAllBytes()` → decrypt → файл. Путь по шаблону `music/%author_name%/%year% - %album_title%`, имя `%author_name% — %track_title%`, `%track_num%` с нулем. Санитизация `\ / " → _`. `exists → skip`. Ошибка → delete + 3 ретрая (в консоли). Порядок: download → mux → tag → `.lrc` + `cover.jpg` рядом.

## Теги — лучшее в подборке

Библиотека `jaudiotagger`, фасад `AudioTagWriter.write()`:

- `TITLE`, `VERSION?`, `ALBUM=title+(version)`, `YEAR=releaseDate[..T]`, `ARTIST=мульти (track.artists + decomposed фиты!)`, `ALBUM_ARTIST`, `TRACK + TRACK_TOTAL`, `GENRE` (с переводом!), `Artwork 300x300`, кастомный `YM_ID=track.id`.
- Обложка двухуровневая: в тег `300x300`, на диск `orig` как `cover.jpg`.
- Жанры: `genres.json` (39 записей en→ru) копируется в рабочую папку, неизвестный — интерактивный вопрос с персистом. Нет жанра — ручной ввод.
- Лирика: `.lrc` sidecar рядом.

Это чинит главную боль Stmol: фиты не теряются, есть totals, есть жанр.

## CLI UX

JLine с контекстами-промптами, команды `вставка ссылок / pause / status / stop`. `config.toml` автосоздается. Пауза — busy-wait (жрет CPU), качание строго по одному (один поток).

## Сильные

- Реальный lossless + AES, разделение api/console, универсальный TagWriter, кэш обложек, шаблоны путей, батч `&&`, skip-if-exists.

## Слабые

- Весь файл в RAM (OOM на FLAC), `Album.tracks=volumes[0]` (теряет диски 2..N), `YEAR=YYYY-MM-DD` (для ID3 надо YYYY), `aac→flac` апконверт в api-модуле — бессмысленно, нет плейлистов, токен в плейнтексте, нет тестов, Cipher-синглтон не потокобезопасен.

## Что берем

1. Карту полей TagWriter 1:1 + `enableLogs=false`.
2. Мультиартист через `decomposed`.
3. Кастомный `YM_ID` для идемпотентности.
4. `GenreTranslator` + `genres.json`.
5. Два уровня обложек + кэш.
6. Шаблонизатор `%author%/%album%/%year%/%track%/%num%`.
7. При переносе чиним: `volumes.flatten()`, `DISCNUM`, стриминг вместо `readAllBytes`, параллелизм.
