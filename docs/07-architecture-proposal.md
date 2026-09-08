# 07. Архитектура нашего приложения (предложение)

Статус: черновик до выбора стека. Стек-независимое описание.

## Принцип

Берем движок Stmol, теги nnikitochka, транспорт MarshalX. UX — максимально простой.

```
┌─────────┐   ссылка    ┌──────────┐   Track[]   ┌───────────┐   файл   ┌────────┐
│   UI    │ ──────────→ │ Resolver │ ──────────→ │ Downloader│ ───────→ │ Tagger │ ─→ downloads/
│ (1 поле │             │ (парсинг │             │ (MP3 vs    │          │ (теги+  │
│ +кнопка)│ ←────────── │  URL)    │ ←────────── │  lossless)│ ←──────── │ обложка)│
└─────────┘   статусы   └──────────┘   мета      └───────────┘  байты   └────────┘
     ↑                       ↑                ↑
     └────── .env (токен) ───┴────────────────┘
```

## Модули

1. **Config** — чтение `.env` (`YANDEX_TOKEN`, `OUTPUT_DIR=./downloads`, `FORMAT=flac`, `SKIP_COVER=false`). Один раз при старте, валидация через `account/status`.
2. **Resolver** — `Parse(url)` (5 типов как у Stmol: track/album/playlist/uuid/chart) → `Resolve()` в `Track{id,title,version,artists[],album,year,genre,coverUri,trackNum,totalTracks}`. Дедуп по ID, флаг `available`.
3. **Downloader** — два пути:
   - `downloadMP3(track)`: `download-info → max bitrate → XML → md5 → GET`.
   - `downloadLossless(track)`: `get-file-info?quality=lossless → HMAC → AES-CTR → flac|flac-mp4`.
   - `download(track)`: пробуем lossless, при ошибке fallback MP3 + пометка. `direct_link` получаем прямо перед скачиванием (TTL 60с). Конкурентность 3, стриминг чанками (не `readAllBytes`!).
4. **Tagger** — после скачивания, до публикации (как `publishAudioArtifact`):
   - MP3/FLAC: ошибка тега = нет файла (retry как MP3 при FLAC-фейле).
   - M4A: best-effort + warn.
   - Поля: TITLE, ARTIST(мульти), ALBUMARTIST, ALBUM, DATE/YEAR, TRACK/TRACKTOTAL, DISCNUM, GENRE(с переводом), YM_ID, source-url, обложка (в тег 1000px опционально, на диск orig).
   - Имена: `%author%/%year% - %album%/%num% - %title%.flac` (шаблонизируемо).
5. **UI** — v1 минималка: одно поле + кнопка + лог строк `[downloading]/[done]/[fallback MP3]/[skip]/[error]`. Позже: альбомы/плейлисты списком.
6. **Storage** — `downloads/` + опционально `cover.jpg/.lrc` рядом.

## Флоу FLAC подробно

```
токен из .env → AccountStatus (uid)
→ Parse(link) → Resolve → Track
→ GetFileInfo(ts, HMAC) → codec==flac?
  ├─ да → Download bytes → AES decrypt → check "fLaC" → Tag → Rename .flac
  └─ нет → DownloadMP3(320) → Tag → Rename .mp3 + пометка fallback
```

## Что унифицировать (ошибки Stmol)

- Один движок очереди для CLI и UI (у Stmol два: `batch` + `download_session`).
- Расширение по факту (`flac/m4a/mp3`), не по запросу.
- Merge тегов вместо wipe чужих картинок.
- Обложка 1000px опцией, не 400 фикс.
- `volumes.flatten()` для многодисковых.
- `YEAR=YYYY`, полную дату в отдельное поле.

## Будущие папки `src/` (после стека)

- `config/`, `resolver/`, `downloader/`, `tagger/`, `ui/`, `storage/` — имена условные, уточним под язык.
- Пока `src/` пустой — это нормально.
