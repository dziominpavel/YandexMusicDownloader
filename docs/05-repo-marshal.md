# 05. MarshalX / yandex-music-api — фундамент

Репо: https://github.com/MarshalX/yandex-music-api
Статус: 1.3k звезд, 103 форка, 709 коммитов, 2019–2026. Стандарт де-факто. Лицензия LGPL-3.0.

## Стек

- Python ~=3.8 (3.8–3.14, CPython/PyPy).
- Sync: `requests[socks]` — единственная обязательная зависимость.
- Async extra: `aiohttp + aiofiles` (`ClientAsync`, методы `*_async`).
- Ynison extra: `websockets + betterproto`.
- Dev: `pytest, unasync, ruff, sphinx`. Доки `ym.marshal.dev`.
- Фишка сборки: sync **генерируется** из async через `unasync` (`generate_sync_version.py`). Править только `client_async.py` + `_client_async/`, иначе затрется.

## Структура

```
yandex_music/
  __init__.py
  client_async.py          # истина
  client.py                # GENERATED
  _client_async/ (22 файла) # миксины: account, albums, artists, tracks, search, playlists, likes...
  _client/                 # GENERATED
  base.py                  # YandexMusicObject: de_json, camel→snake
  device_auth/, download_info.py, track/, account/, album/...
  utils/request.py, request_async.py, request_base.py, sign_request.py
examples/ (CC0): device_auth, search, get_album_with_tracks, player, proxy...
tests/
```

`Client(token).init()`: `me=account_status()`, `account_uid`. `Track(id,title,artists,albums,duration,cover_uri,...)`, `track_id=f'{id}:{albums[0].id}'`. `TrackShort→fetch_track()` для полного.

Каноничный пример: `client.users_likes_tracks()[0].fetch_track().download('example.mp3')`.

## Auth / download_info

Токен: `Client('token').init()`, заголовок `Authorization: OAuth`. Без токена — только 30 сек.

`device_auth(on_code)` из коробки (вшитые креды Android-приложения `client_id=23cabbb...`):
1. `POST oauth.yandex.ru/device/code` → `DeviceCode{user_code, verification_url}`.
2. `on_code` показывает куда идти.
3. Poll `POST /token {device_code}` пока `authorization_pending` → `OAuthToken{access_token,refresh_token,expires_in}`. Хранение/рефреш — на нас.

Фолбэки: browser implicit `oauth.yandex.ru/authorize?response_type=token&client_id=23cab...` → `#access_token=...`, сторонний `yandex-music-token`.

`download_info`:
```python
client.tracks_download_info(track_id)  # GET /tracks/{id}/download-info
info.get_direct_link()  # XML → host,path,ts,s → md5(SIGN_SALT + path[1:] + s)
# SIGN_SALT='XGRlBW9FXlekgbPrRHuSiA'
# https://{host}/get-mp3/{sign}/{ts}{path}, TTL ~60 сек
```

Кодеки в доке: только `mp3/aac 64/128/192/320` (дефолт `mp3/192`). FLAC не документирован, но архитектура generic: `get_specific_download_info(codec,bitrate)` ищет совпадение в живом ответе. Если у аккаунта Plus и трек lossless — вернется `flac`, библиотека скачает без изменений. Нам нужен свой `pick_best(prefer=[flac,aac,mp3])`, не хардкодить `192`.

`download(filename)`: `retrieve(direct_link) + open(wb).write()`. Есть `download_bytes()` для тегирования в памяти. Нет: стриминга, прогресса, resume, ретраев, выбора best, тегов.

## Почему фундамент

Единственная зрелая reverse-обертка: весь API уже в типизированных моделях, хрупкие места инкапсулированы (подпись, XML, TTL, OAuth, `track_id`). Dual sync/async из одного источника. Stmol и другие сверяются именно с ней. Порты на C#/PHP/TS/JS тоже.

## Сильные / слабые как базы

+: минимум зависимостей, полнота метаданных (volumes, `cover_uri%%`, тексты LRC/TEXT, r128), диагностика (`report_unknown_fields`, исключения), доки + Telegram-чат.
−: примитивный `download()` (весь файл в RAM, OOM на FLAC), `direct_link` протухает за 60с (очереди обновлять), нет менеджера токенов, нет постобработки (mutagen, шаблоны, дедуп, m3u), дефолт `mp3/192` устарел.

## Что берем

1. `request_base.py`: HEADERS, `set_authorization`, `_parse`, маппинг ошибок.
2. `device_auth` целиком + `token.md` фолбэки.
3. `download_info.py`: `SIGN_SALT`, `__build_direct_link`, разделение `retrieve vs download`.
4. `tracks.py + track.py`: `track_id`, `get_download_info`, паттерн `InvalidBitrateError` → расширить до `pick_best`.
5. `Client(token)+init()->me`.
6. Паттерн миксинов + `@model/de_json`, примеры `player.py` (кеш, mkdir, retry), `proxy.py`, `search.py`.
