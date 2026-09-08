# 04. Kud1nov / yamusic-dl — минимализм и крипта

Репо: https://github.com/Kud1nov/yamusic-dl
Статус: 2 звезды, 9 коммитов. Учебная консольная утилита.

## Стек

- Go 1.23.10 (в README написано 1.16 — врет).
- Прямая зависимость одна: `google/uuid` (для `encrypted_<uuid>.raw`).
- Остальное stdlib: `net/http+cookiejar`, `aes+CTR`, `hmac+sha256`, `base64+hex+json`, `multipart`, `regexp`, `flag`.
- Логгер `zerolog` + обертка `verbose`.
- Два бинарника: `bin/yamusic-dl` + `bin/yamusic-auth`.

## Структура

```
cmd/downloader/main.go   # 65 строк: flags + DownloadTrack
cmd/authorizer/main.go   # 737 строк: весь OAuth+CSRF+2FA
internal/api/models.go   # константы + Track/Album/DownloadInfo
internal/crypto/crypto.go # sign + decrypt (+ тесты)
internal/logger/, internal/utils/ (ExtractTrackID, CleanFileName)
pkg/yamusic/client.go    # 560 строк: GetTrackInfo/GetDownloadInfo/DownloadTrack
```

Константы:
`BaseURL=https://api.music.yandex.net`, `SignKey=uz0zSpaYCLmgk6C7YLdo5F`, `Client=YandexMusicDesktopAppMacOS/5.54.0`, `Codecs=flac,flac-mp4,mp3,aac,...`, `Transport=encraw`.

## Авторизатор (самое ценное тут)

Имитация десктопа (`ClientID=97fe03033fa34407ac9bcf91d5afed5b`, Electron UA):
1. `GET passport.yandex.ru/auth` → парсинг `csrf_token` 12 паттернами + ручной ввод при смене верстки.
2. `POST multi_step/start {login}` → `track_id`.
3. `POST commit_password {password}`.
4. Если `auth_challenge` — только `push_2fa` (остальное Fatal).
5. Хождение по `302 Location` пока не найдет `access_token=` во фрагменте.

Капча не решается — Fatal + инструкция «пройди в браузере и достань токен из `innerHTML.match(/access_token=/)»`. Пароль читается с эхом (без `ReadPassword`).

## Качалка

CLI: `-track <ID|URL> -token [-quality min|normal|max] [-output] [-verbose]`.
`min→lq, normal→nq, max→lossless`.

1. `ExtractTrackID`: `^\d+$` или `/track/(\d+)`.
2. `POST /tracks {trackIds}` → title/artists/albums (есть fallback `GET /tracks/{id}`).
3. `GET /get-file-info?ts&trackId&quality&codecs&transports&sign`, где `sign=HMAC-SHA256(key, ts+trackId+quality+codecs+transports)` base64 без `=`, запятые вырезаются.
4. `http.Get(url)` → `encrypted_<uuid>.raw` → `DecryptAesCtr(hexKey, IV=0)` → `<output>/Title - Artist (Album) [ID].m4a`. Времянка чистится.

Важно: расширение всегда `.m4a`, `codec` игнорируется. Отдельного `-format flac` нет.

## FLAC? Теги?

FLAC как запрос — да (есть в `Codecs`), как фича — нет (нет маппинга `codec→расширение`). Тегов нет вообще: нет библиотек для MP4/ID3/Vorbis, обложка не качается, лирика не качается. Вся мета — в имени файла.

## Крипта

`GenerateSignature(data,key)`: убрать `,`, `HMAC-SHA256`, base64 без `=`. Есть 3 теста с эталоном.
`DecryptAesCtr`: `hex.Decode(key)`, `AES-CTR IV=0`, `XORKeyStream`. Нулевой IV — особенность API, не баг.

Нюанс: `SignKey` в коде и в тестах разные — ключ ротируется, хардкод протухает.

## Сильные / слабые

+: 9 файлов ~1900 строк, ноль зависимостей, тесты подписи, живучий CSRF-парсер, константы для копипасты.
−: только 1 трек за запуск, нет альбомов/батча/ретраев, весь файл в RAM, скачивание через голый `http.Get` без таймаута, нет хранения токена, нет `context`, нет прокси.

## Что берем

1. `GenerateSignature + DecryptAesCtr` как эталон.
2. Формулу `ts+trackId+quality+codecs+transports`, `ClientID/RedirectURI/Origin`.
3. CSRF-паттерны + фолбэк ручного ввода.
4. `ExtractTrackID`, `CleanFileName([\\/:*?"<>|]→_)`.
5. Схему имени `Title - Artist (Album) [ID]`, но чиним расширение по `codec` + пишем теги.
