## Purpose

Скачивание трека с Яндекс Музыки по вставленной ссылке: приоритет FLAC, автоматический fallback на лучший MP3, токен из `.env`, пропуск уже скачанного.

## Requirements

### Requirement: Токен из окружения

Приложение MUST читать OAuth-токен из переменной `YANDEX_TOKEN` (файл `.env`). Без токена приложение MUST предупреждать, что доступно только 30-секундное превью, и не падать.

#### Scenario: Токен задан

- **WHEN** в `.env` задан `YANDEX_TOKEN` и пользователь запускает приложение
- **THEN** приложение валидирует токен через `account/status` и готово к скачиванию

#### Scenario: Токена нет

- **WHEN** `YANDEX_TOKEN` пуст или отсутствует
- **THEN** приложение показывает предупреждение про 30-секундное превью и продолжает работу

### Requirement: Скачивание трека по ссылке

Приложение MUST принимать ссылку вида `music.yandex.ru/album/{albumId}/track/{trackId}` (позже — альбом/плейлист/чарт) и скачивать аудио в `OUTPUT_DIR` (по умолчанию `./downloads`).

#### Scenario: Вставил ссылку — получил файл

- **WHEN** пользователь вставляет ссылку на трек и нажимает «Скачать»
- **THEN** в логе появляется `[downloading] Artist — Title`, а после завершения — `[done] Artist — Title (FLAC|MP3)`

#### Scenario: Файл уже есть

- **WHEN** целевой файл уже существует в папке назначения
- **THEN** скачивание пропускается и в логе появляется `[skip] Artist — Title (already exists)`

### Requirement: FLAC с fallback на MP3

Приложение MUST сначала пробовать lossless (`get-file-info?quality=lossless`): при успехе сохранять `.flac` (или `.m4a` при контейнере `flac-mp4`). Если lossless недоступен, приложение MUST скачивать лучший доступный MP3 и помечать это в логе как fallback.

#### Scenario: Lossless доступен

- **WHEN** трек доступен в lossless
- **THEN** сохраняется `.flac` и лог содержит `[done] Artist — Title (FLAC)`

#### Scenario: Lossless недоступен

- **WHEN** трек недоступен в lossless (нет подписки или прав)
- **THEN** скачивается лучший MP3 и лог содержит `[done] Artist — Title (MP3 fallback)`

### Requirement: ALAC в M4A → FLAC на лету

Если lossless приехал в контейнере `flac-mp4` (ALAC в M4A), приложение MUST перекодировать аудио в plain FLAC через `ffmpeg.exe` рядом с exe (lossless→lossless, аудио бит-в-бит) и тегировать уже FLAC нашим тегером. В логе — `[done] Artist — Title (FLAC) [ALAC→FLAC]`. Если `ffmpeg.exe` нет рядом или задан `CONVERT_M4A=false`, приложение MUST сохранять `.m4a` как раньше. Неудачная конвертация MUST идти по обычному пути ошибок (другое зеркало, затем MP3-fallback).

#### Scenario: ALAC превратился во FLAC

- **WHEN** трек доступен в lossless только как `flac-mp4` и `ffmpeg.exe` лежит рядом с exe
- **THEN** сохраняется `.flac` с полными тегами и лог содержит `[done] Artist — Title (FLAC) [ALAC→FLAC]`

#### Scenario: Конвертера нет

- **WHEN** трек — `flac-mp4`, а `ffmpeg.exe` рядом нет
- **THEN** сохраняется `.m4a` с тегами как раньше, без ошибок

### Requirement: Потоковая запись и атомарность

Приложение MUST NOT держать весь файл в памяти: запись — потоковая, публикация — атомарная (`временный файл → переименование`). Недокачанный файл MUST NOT оставаться в папке назначения.

#### Scenario: Обрыв скачивания

- **WHEN** скачивание прерывается ошибкой сети
- **THEN** временный файл удаляется, в логе — `[error] Artist — Title`, готового файла нет
