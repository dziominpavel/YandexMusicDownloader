## Context

Кодовой базы нет (`src/` пуст). Есть проверенный протокол на Go в `references/01-stmol-yandex-music-downloader`
(`ya/client.go`, `ya/lossless/`, `source/`), теговые библиотеки уже подобраны там же
(`id3v2`, `go-flac`, `mtag`). Мотивация — см. `proposal.md` (Why). Требования — см. `specs/desktop-app/spec.md`,
`openspec/specs/track-download/spec.md`, `openspec/specs/audio-tagging/spec.md`.

## Goals / Non-Goals

**Goals:**
- Сквозной путь «окно → ссылка → `.flac` с тегами» одним exe
- Максимум переиспользования протокола Stmol (порт, не реверс заново)

**Non-Goals:**
- Альбомы/плейлисты пачкой, `device_auth`, автообновления, инсталлятор — всё это v0.2+

## Decisions

### 1. GUI — Wails v2 (Fyne — запасной)

Окно «поле + кнопка + лог» на Wails делается за вечер обычным HTML/JS, exe ~10 МБ,
бэкенд — чистый Go, который напрямую вызывает `downloader`. WebView2 встроен в Win10 (1809+) и Win11,
у друзей проблем не ожидается.
Альтернатива Fyne (чистый Go без WebView2) — проще в сборке, но беднее UI и толще бинарник;
держим как план Б, если WebView2 станет проблемой.

### 2. Каркас `src/` — по мотивам Stmol, но один движок очереди

```
src/
├── cmd/app/main.go
└── internal/
    ├── config/      # .env рядом с exe, валидация через account/status
    ├── resolver/    # парсинг ссылки (порт source/url.go, пока только трек)
    ├── downloader/  # порт ya/client.go + ya/lossless (MP3 + lossless + fallback)
    └── tagger/      # порт ya/id3.go + flac.go + m4a.go, схема полей из docs/03
```

У Stmol два движка очереди (`internal/batch` для CLI и `download_session` для TUI) — у нас будет один,
окно подписывается на события (`[downloading]/[done]/...`) через колбэк/канал.

### 3. Порт, а не копия

Stmol без явной лицензии — переносим протокол и структуру своими словами,
дословно тащим только формулы подписи/шифра (они — факты API, плюс сверка с Kud1nov/MarshalX).

### 4. Сборка без консоли

`go build -ldflags -H=windowsgui` — иначе у друзей будет висеть чёрное окно.
`.env` ищется рядом с exe, `downloads/` создаётся рядом при первом скачивании.

## Risks / Trade-offs

- [Risk] WebView2 отсутствует на старой Win10 → Mitigation: проверка при старте с понятным сообщением; план Б — Fyne
- [Risk] Яндекс сменит ключи подписи (`SIGN_SALT`/`SignKey`) → Mitigation: ключи — константы в одном месте (`internal/downloader`), сверка с `references/03` и `04` при поломке
- [Risk] Теги M4A тихо не запишутся (у Stmol best-effort) → Mitigation: в MVP логируем warning в окно, файл помечаем; строгость решим в v0.2
- [Trade-off] Wails тянет Node.js в toolchain для сборки фронта → приемлемо, фронт — один статический экран

## Migration Plan

Не применимо (первый код). Откат — предыдущий git-коммит.

## Open Questions

- Точный размер обложки в теге (400px как у Stmol vs 1000px) — решить при реализации tagger, на спеки не влияет.
