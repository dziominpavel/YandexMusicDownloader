# YandexMusicDownloader

Личное приложение для скачивания музыки с Яндекс Музыки в **FLAC** (с fallback на MP3).
Сценарий: токен один раз в `.env` → вставил ссылку на трек → нажал «Скачать» → файл с тегами в `downloads/`.

> Статус: MVP работает (09.09.2026). Стек: **Go + десктопное Windows-окно (Wails v2), сборка в один `app.exe`** (см. `docs/09-stack-options.md`).

## Возможности (v0.1)

- Скачивание трека по ссылке, приоритет — lossless (FLAC; ALAC в M4A автоматически перегоняется во FLAC через `ffmpeg.exe` рядом)
- Автоматический fallback на лучший MP3, если lossless недоступен, с пометкой в логе
- Обязательные теги: название, исполнители, альбом, год, номер трека, жанр, обложка
- Плоские имена `Группа - Трек` с правилом сортировки кириллицы (см. `docs/12-file-naming.md`)
- Пропуск уже скачанных файлов, атомарная запись (`temp → rename`), потоковое скачивание

## Структура

```
├── src/           # Go-код: cmd/app + internal/{config,resolver,downloader,tagger,ui}
├── docs/          # аналитика, требования, архитектура, roadmap
├── openspec/      # формальные спеки (track-download, audio-tagging, desktop-app)
├── references/    # снепшоты 4 проектов-доноров для анализа (не в git)
├── scripts/       # сборка app.exe
├── app.exe        # собранный exe (не в git, см. scripts/build-windows.ps1)
├── ffmpeg.exe     # опциональный конвертер ALAC→FLAC рядом с exe (не в git)
└── downloads/     # скачанные треки (не в git)
```

## Документация

- `docs/01-overview.md` — цель и сценарий
- `docs/02..05-*.md` — разбор проектов-доноров (Stmol, nnikitochka, Kud1nov, MarshalX)
- `docs/06-comparison.md` — сравнение и что берём за основу
- `docs/07-architecture-proposal.md` — предлагаемая архитектура
- `docs/08-requirements.md` — требования v0.1
- `docs/09-stack-options.md` — варианты стека (Python vs Go)
- `docs/10-roadmap.md` — план реализации и следующие шаги
- `docs/11-get-token.md` — как получить токен (инструкция для пользователей)
- `docs/12-file-naming.md` — правила именования файлов (плоская раскладка, сортировка кириллицы)
- `docs/friends-setup.md` — инструкция для друзей: токен → запуск
- `AGENTS.md` — правила для AI-агентов

## Дальше

План — в [`docs/10-roadmap.md`](docs/10-roadmap.md).
Коротко: MVP (`mvp-desktop-flac`) готов и заархивирован → дальше альбомы и удобство (v0.2).
