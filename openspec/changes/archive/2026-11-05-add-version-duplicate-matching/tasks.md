# Tasks

## 1. Снятие Radio Edit в `norm`

- [x] 1.1 Добавить `norm.RadioEditBase`: хвостовой `radio edit` без скобок, в `(…)`/`[…]`, с именем ремиксера; проверить табличным `TestRadioEditBase` (`go test ./internal/norm`).
- [x] 1.2 Покрыть тестами границы: `Song (Radio Version)`/`Radio Mix`/`Radio Remix` не снимаются, `Song (Remix)`/`(Live)`/`(Remastered)`/`(Version)` не снимаются, `m4a`/`ФАКТ` не снимаются, `Radio Edit` без названия даёт пустую базу; проверить `go test ./internal/norm`.

## 2. Fallback в `library.FindMatches`

- [x] 2.1 Добавить fallback по базовому названию: 95%, только при схожести исполнителей ≥0.90, без дублей записи; проверить `go test ./internal/library -run 'TestFindMatches'`.
- [x] 2.2 Добавить тесты: `Not Afraid` ↔ `Not Afraid Radio Edit`, `Song` ↔ `Song (Radio Edit)`/`[Radio Edit]`, `Song` ↔ `Song (2-4 Grooves Radio Edit)`, `Song` ↛ радиоверсия другого исполнителя, `Song` ↛ `(Remix)`/`(Live)`/`(Remastered)`/`m4a`; проверить `go test ./internal/library`.
- [x] 2.3 Убедиться, что `ScoreCandidate` по-прежнему не считает `Song (Live)` кандидатом на апгрейд; проверить `go test ./internal/library -run TestScoreCandidate`.

## 3. Changelog и спеки

- [x] 3.1 Добавить в `CHANGELOG.md` в `## [Unreleased]` пользовательский блок `**Исправлено**` и инженерный пункт `### Исправлено`; проверить `python scripts/check-version.py` (exit 0), версию не менять.
- [x] 3.2 Проверить дельту и проект: `openspec validate --all`; убедиться, что в тексте остались только Radio Edit и нет обещаний про remix/live/remastered.

## 4. Интеграционная проверка

- [ ] 4.1 Прогнать `go test ./...`, `go vet ./...`, `go build ./...`; новых зависимостей быть не должно.
  > Заблокировано незавершённым change `download-queue-ui` (`ui`-тесты) и `applog.test.exe: Access is denied`; scoped-прогон `norm`/`library` зелёный.
- [x] 4.2 Сверить поведение с `review.md`: радиоверсия Eminem находится, а 7 отклонённых пар (remix/remastered/russian version) и служебные пометки — нет; зафиксировать результат в ответе.

## Workflow follow-up

- После проверки и ревью — архивировать change; архивация перенесёт дельту в `openspec/specs/library-index/spec.md`.
- Пересобрать exe и перезапустить приложение, чтобы новая логика попала в работающую копию.
