# YandexMusicDownloader — правила для AI-агента

Единый IDE-нейтральный источник always-on правил. Читается любым агентом (Cursor, Devin Desktop, JetBrains AI, OpenCode и др.).
Дополнительные правила: `.cursor/rules/*.mdc`. OpenSpec-скиллы: `.cursor/skills/`, `.devin/skills/`, `.opencode/skills/`; команды: `.cursor/commands/opsx-*.md`, `.devin/workflows/opsx-*.md`, `.opencode/commands/opsx-*.md`.

## Контекст проекта

Личное приложение для скачивания музыки с Яндекс Музыки в **FLAC** (с fallback на MP3).
Сценарий: токен из `.env` → вставил ссылку на трек → нажал скачать → файл с тегами в `downloads/`.

### Стек (выбран 08.09.2026)

- **Язык: Go**, UI — десктопное Windows-окно (Wails v2 дефолт, Fyne запасной), дистрибуция — один exe. Детали: `docs/09-stack-options.md`.
- Протокол скачивания портируем из Stmol (`references/01-...`, `docs/02-repo-stmol.md`), теговую схему — из nnikitochka (`docs/03-repo-nnikitochka.md`).
- Код — в `src/` (каркас создаётся следующим change'ем).

### Ключевые пути

- `docs/01-overview.md` — цель и сценарий; `docs/08-requirements.md` — требования v0.1
- `docs/02..05-*.md` — разбор 4 репо-доноров; `docs/06-comparison.md` — сравнение
- `docs/07-architecture-proposal.md` — предлагаемая архитектура (Config/Resolver/Downloader/Tagger/UI)
- `references/` — снепшоты доноров для анализа, **не зависимости, не коммитятся** (в `.gitignore`)
- `openspec/specs/` — формальные спеки (`track-download`, `audio-tagging`)
- `.env` (по `.env.example`) — токен, никогда в git

### Документация (обязательно учитывать)

- Перед изменениями: `docs/01-overview.md`, `docs/08-requirements.md`, `openspec/specs/*/spec.md`
- Протокол скачивания: `docs/02-repo-stmol.md` (MP3/lossless, fallback, `temp→tag→rename`)
- Тегирование: `docs/03-repo-nnikitochka.md` (мультиартист, totals, `YM_ID`, жанры)
- API/авторизация: `docs/05-repo-marshal.md` (`device_auth`, `download_info → direct_link`)

## Секреты

- `YANDEX_TOKEN` — только из `.env`, в git только `.env.example` без значений.

## Перед изменениями

1. **Сверяйся со спеками:** `openspec/specs/track-download/spec.md`, `openspec/specs/audio-tagging/spec.md`.
2. **Новые фичи — через OpenSpec change** (`/opsx-propose` → review → `/opsx-apply` → `/opsx-archive`).

## Приоритеты при доработках

- **FLAC first** — lossless приоритет, MP3 только как fallback с пометкой в логе.
- **Теги обязательны** — файл без тегов = брак (см. `audio-tagging/spec.md`).
- **Не держать файл в RAM** — потоковая запись; публикация атомарная (`temp → rename`).
- **Простой UX** — одно поле + кнопка, без TUI-хоткеев как у Stmol.
- **Токен никогда в git**; `references/` никогда в git.

## OpenSpec (spec-driven development)

Структура: `openspec/` (`specs/`, `changes/`, `changes/archive/`). Скиллы и команды для Cursor (`.cursor/skills/openspec-*`, `.cursor/commands/opsx-*.md`), Devin Desktop (`.devin/skills/openspec-*`, `.devin/workflows/opsx-*.md`) и OpenCode (`.opencode/skills/openspec-*`, `.opencode/commands/opsx-*.md`).

### Workflow

- `/opsx-propose <idea>` — создать change с артефактами (proposal.md, design.md, tasks.md, specs/).
- `/opsx-apply` — реализовать задачи из текущего change по спеке.
- `/opsx-archive` — заархивировать завершённый change и обновить основные specs.
- `/opsx-explore` — исследовать кодовую базу (тут: `docs/` + `references/`) перед предложением.

### Когда использовать

- Новые фичи и нетривиальные изменения — через OpenSpec (propose → review → apply → archive).
- Мелкие правки (опечатка, точечный фикс доки) — можно напрямую без OpenSpec.
- Артефакты на **русском**, normative keywords — **MUST/SHALL/MUST NOT** (не «ДОЛЖЕН»).
- Проверка: `openspec validate --all`.
