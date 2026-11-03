# Tasks

## 1. Фронтенд

- [x] 1.1 В `src/frontend/dist/index.html` убрать у поля `#link` атрибут `list="linkHistory"` и удалить элемент `<datalist id="linkHistory">`; проверить, что в файле не осталось `linkHistory` и `datalist` (поиск по файлу).
- [x] 1.2 Удалить JS-блок истории (`HISTORY_KEY`, `loadHistory`, `renderHistory`, `rememberLink`, вызов `rememberLink`) и убедиться, что drag & drop, предпросмотр и кнопка «Открыть папку загрузок» не затронуты; проверить прогоном `node tests/statusbar_check.js` — сценарии статусбара проходят без ошибок.

## 2. Документация

- [x] 2.1 Убрать историю ссылок из списка возможностей в `README.md`; проверить, что упоминаний истории не осталось в README (`grep -i "истори"`).
- [x] 2.2 Добавить в `CHANGELOG.md` пользовательский пункт в `**Изменено**` с пометкой `**BREAKING:**` и инженерную деталь под `### Изменено` (change-id `remove-link-history`, состав правок, проверки); проверить `python scripts/check-version.py` — exit 0.

## 3. Проверка

- [x] 3.1 Прогнать `go test ./...`, `go vet ./...`, `go build ./...` в `src` — все команды завершаются успешно, встраивание `dist/index.html` не сломано.
- [x] 3.2 Прогнать `openspec validate remove-link-history --type change` и `openspec validate --specs` — delta и основные спеки валидны. (Полный `--all` сейчас падает из-за пустого постороннего change `enter-to-download`; его не трогать.)
- [x] 3.3 Вручную запустить `app.exe`: поле ввода не показывает выпадающий список и стрелку истории, лог не перекрывается; drag & drop и предпросмотр ссылки работают как раньше.

## Workflow follow-up

- Заархивировать change после проверки, чтобы основная спека `track-download` обновилась из delta.
- Повторно прогнать `openspec validate --all` после архивации.
