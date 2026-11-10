# Shadow replay: 40 последовательных merged PR

## Вывод

**Новая before/after-карточка не доказала преимущество в предотвращении пропущенных
дефектов. Более узкая польза подтверждается: объяснять реальные изменения,
отделять их от старого debt и не путать code change с изменением measurement.**

Рекомендация после replay: сначала улучшить текущий PR feedback.
Не строить широкий proactive-refactoring product или три платных режима.

- [Визуальный replay](index.html)
- [Выборка, SHA и ручная классификация](sample.json)
- [Фактическая reproduction](reproduction.json)
- [Обновлённый replay-grounded prototype](../index.html)

## Методика и границы

После полной пагинации closed PR выбраны последние 20 merged PR каждого repository,
отсортированные по merged_at. Не по номеру и не по интересности темы.
API capture содержит 114 merged PR archfit и 172 merged PR ccgram.

- archfit: 27–29 октября 2026, #97 → #122 в merge-порядке.
- ccgram: 21–29 октября 2026, #330 → #364 в merge-порядке.
- Все 40 остались в denominator, включая docs, pins и baselines.
- Сохранены final integrated diffs, SHA, policy snapshots, comments, reviews и checks.
- Четыре read-only reviewer lanes разобрали непересекающиеся группы по 10 PR.
- Основной reviewer перепроверил сильные случаи и отклонил завышенные new_action:
  описание самой feature, известного решения или generic “проверьте semantics”
  не считается новой находкой.
- Загружены пять archived CI artifacts: ccgram #348, #341, #342, #350, #360.
- Проверены run/head bindings для #341, #350 и #360.
- На одинаковом минимальном Go fixture выполнена reproduction с binaries
  из исторических commits archfit #106 и #120.

Это **не blinded human study**, не benchmark Jev/Clef и не полный rerun
40 исторических analyzer environments. Reviewed final merged patches часто уже
содержат fixes предыдущих review rounds. #111 archfit (287 files) и #330 ccgram
(246 files) просмотрены inventory-first с focused source review, не line-by-line.

Issue comments с updated_at позже merge исключены из исходного feedback.
PR bodies получены сейчас: они не гарантируют исторический текст на момент merge.
Их intent и validation claims не заменяют source/artifact evidence.
Postmerge statements не засчитываются как prospective evidence.
Current GitNexus indexes не использовались как доказательство старого графа.

## Это не 40 полностью green PR

У final PR head до merge удалось получить:

| archfit/state | archfit | ccgram | Всего |
| --- | ---: | ---: | ---: |
| neutral | 7 | 13 | 20 |
| action_required | 1 | 5 | 6 |
| Подходящий completed check не найден в capture | 12 | 2 | 14 |

Neutral не означает complete measurement или hard-gate pass.
Не найденный check не означает, что App никогда не запускался.
Несколько archfit CI jobs завершились failure при переходе producer.
Поэтому выводы нельзя выдавать за измеренную эффективность на green-only PR.

## История 1: ccgram #341 — improvement уже было в report

[PR #341](https://github.com/alexei-led/ccgram/pull/341)
перенёс restart state в runtime и заменил private mutation callback-вызовом.

~~~text
До:
upgrade handler → imports / writes → main._restart_requested

После:
runtime owns restart state
runtime → supplies callback → Application.bot_data
upgrade handler → request → watchdog → stop
~~~

Source evidence:

- [Base upgrade.py](https://github.com/alexei-led/ccgram/blob/8828da13ccba1a3d3ded290219292bc097a9de03/src/ccgram/handlers/upgrade.py#L94-L115)
- [Head upgrade.py:94–115](https://github.com/alexei-led/ccgram/blob/64bfc0cbb8719e729638474a113c289f2f5bbf87/src/ccgram/handlers/upgrade.py#L94-L115)
- [Head main.py:325–340](https://github.com/alexei-led/ccgram/blob/64bfc0cbb8719e729638474a113c289f2f5bbf87/src/ccgram/main.py#L325-L340)

Artifact #348 содержит finding f52900402d1eaf48f57d9f4e7711160d:
ccgram.handlers.upgrade → ccgram.main.
Artifact #341 уже содержит этот ID в resolved_finding_ids, без introduced IDs.

[Исторический comment](https://github.com/alexei-led/ccgram/pull/341#issuecomment-6368107632)
показывал 87 advisories и общий призыв review dimensions.

Counterfactual card:

> Private restart-state access removed. Runtime now owns restart intent.
> Preserve callback wiring and request → watchdog → stop behavior.
> Existing repository debt remains separate.

**Польза:** легче увидеть результат refactoring и его postconditions.
**Не доказано:** что это изменило бы merge decision. Автор уже намеренно выполнил
refactoring. Устранение всех cycles не заявляется.

## История 2: ccgram #342 — цифры растут без изменения кода

[PR #342](https://github.com/alexei-led/ccgram/pull/342) меняет только .archfit.yaml:
ownership и public declarations ранее неописанных компонентов.

В соседних archived reports: 87 → 98 advisories.
Model/profile inputs различаются. Это не сравнимый quality delta.
Production source в самом PR не меняется.

Counterfactual card:

> Architecture map expanded. More relationships are now classified.
> This PR changed policy/evidence, not application code.
> Higher advisory count does not establish erosion.

Body заявляет 45 → 0 unmapped dependencies; это author-reported validation,
не независимо reproduced measurement. Сильная опора нашего вывода — config-only
diff и реальные report identities/counts.

**Польза:** избежать неправильной интерпретации.
**Classifier не нужен:** scope и measurement fingerprints решают задачу.

## История 3: ccgram #350 — хороший refactoring и generic плохой совет

[PR #350](https://github.com/alexei-led/ccgram/pull/350) убирает Telegram routing
из event monitor и вводит awaited callback, связанный с узким adapter в bootstrap.

~~~text
До:
EventStreamMonitor → TelegramClient
EventStreamMonitor → private polling/death policy

После:
EventStreamMonitor → awaited callback
bootstrap → public window-death adapter → existing query/policy
~~~

Source evidence:

- [Base monitor](https://github.com/alexei-led/ccgram/blob/bfe7bb77995870124d1a5a1a595f4ae8132ffcfe/src/ccgram/event_stream_monitor.py#L135-L153)
- [Head callback contract](https://github.com/alexei-led/ccgram/blob/6cfe0ebd37933d640239e3eb00afbd31b4259693/src/ccgram/event_stream_monitor.py#L43-L50)
- [Head invocation](https://github.com/alexei-led/ccgram/blob/6cfe0ebd37933d640239e3eb00afbd31b4259693/src/ccgram/event_stream_monitor.py#L129-L134)
- [Bootstrap wiring](https://github.com/alexei-led/ccgram/blob/6cfe0ebd37933d640239e3eb00afbd31b4259693/src/ccgram/bootstrap.py#L354-L369)
- [21-line adapter](https://github.com/alexei-led/ccgram/blob/6cfe0ebd37933d640239e3eb00afbd31b4259693/src/ccgram/handlers/polling/window_death.py#L1-L21)

Archived report: 95 findings, 7 resolved IDs, 1 introduced grouped ID.
Comparison base — 2a19f3f..., final merge parent — bfe7bb7....
Нельзя считать базы одинаковыми или приписывать каждую разницу самому PR.

Introduced group 08f2b089... содержит 51 member. Representative path —
ccgram.handlers.agent_command; среди locations — новый window_death.py:7.
Group origin не устанавливает origin каждого location.
Карточка “agent_command сломал boundary” была бы недостоверной.

Generic hypothesis — move_functionality. Без контекста она не объясняет,
что adapter→query dependency поддерживает намеренную separation of concerns.

Counterfactual card:

> Monitor is structurally more neutral. Private UI knowledge moved to an adapter.
> New query access supports recipient routing; it is not by itself a reason
> to merge modules. Verify callback/await and snapshot-before-await contracts.

[Greptile comment](https://github.com/alexei-led/ccgram/pull/350#issuecomment-6396983951)
уже объяснял изменение и показывал diagram.
[Archfit comment](https://github.com/alexei-led/ccgram/pull/350#issuecomment-6396991611)
показывал repository-wide advisories. Body сообщает, что earlier Archfit feedback
помог исправить relocated private import; это claim о предыдущем round,
не новое обнаружение нашим replay.

**Польза:** contextual interpretation и сохранение смысла refactoring.
**Не уникально:** existing code reviewer уже способен рассказать эту историю.

## История 4: ccgram #363 — выигрыш от отсутствия комментария

[PR #363](https://github.com/alexei-led/ccgram/pull/363) меняет только CHANGELOG:
28 additions, один файл.

[Bot comment](https://github.com/alexei-led/ccgram/pull/363#issuecomment-6489707536)
сообщает 0 introduced, 95 pre-existing advisories и выводит первые три
repository dependencies. Это не новые задачи этого PR.

Counterfactual:

> No observed architectural source change in this PR.
> Repository evidence/debt remains available separately.

Required evidence failures нельзя скрывать.
Quiet означает отсутствие irrelevant commentary, не отключение checks.

**Польза:** меньше interruptions. Time saving не измерялся.
**Classifier не нужен:** diff scope и existing origin/evidence data.

## История 5: archfit #106 → #120 — настоящий regression вне graph diff

В [#106](https://github.com/alexei-led/archfit/pull/106) strict lint стал требовать
rationale у blocking rules. Генератор starter config ещё не записывал rationale.

- Consumer: [LintRationales](https://github.com/alexei-led/archfit/blob/7e4d4787b6ff22c5feec58a1b83fabb232901bbd/internal/assessment/evaluation/lint.go#L114-L125)
- Producer: [writeStarterRules](https://github.com/alexei-led/archfit/blob/7e4d4787b6ff22c5feec58a1b83fabb232901bbd/internal/initcfg/starter_rules.go#L48-L64)
- Fix: [#120](https://github.com/alexei-led/archfit/pull/120)
- [Regression test](https://github.com/alexei-led/archfit/blob/8b22cfdce2b6399d4ddfc04909bc4533ce9f3d66/cmd/archfit/init_test.go#L418-L443)

Фактически выполнено в disposable snapshots, dependencies только из cache
(GOPROXY=off), без model calls:

| Историческая версия | config init | config lint --strict |
| --- | ---: | ---: |
| Commit после #106 | exit 0 | **exit 1: missing_rationale** |
| Commit после #120 | exit 0 | exit 0 |

**Hindsight-confirmed:** reproduction выбрана после чтения #120.
Reviewer lane, не получавший #120, этот mismatch не установил.
Нельзя заявлять prospective успех proposed feature.

Это candidate для другой capability: consumer validation changed → найти producer
→ выполнить producer/consumer contract check.
Classifier может выбрать investigation, но ошибку доказывает executed check.
Integration tests и обычный code review также способны решить эту задачу.

## Что строить после replay

1. **PR delta receipt:** changed/resolved facts отдельно от repository debt.
2. **Change provenance:** code change отдельно от policy/map/producer change.
3. **Contextual refactoring interpretation:** callback extraction, ownership move,
   contract use, relocation — без превращения generic hypothesis в приказ.
4. **Focused contract investigation:** отдельный следующий experiment,
   не обещание текущего before/after graph.

Jev/Clef стоит проверять на пункте 3, не на детерминированных пунктах 1–2.
Input: before/after snippets, resolved facts и intent.
Output: bounded transformation label либо insufficient_context.
Blockers и evidence completeness остаются вне model authority.

## Ручная классификация

Это оценка роли дополнительной карточки, **не accuracy, ROI или число предотвращённых bugs**.
“Пояснение” не означает новый useful finding.

| Категория | PR |
| --- | ---: |
| Без новой карточки | 12 |
| Потенциально полезное пояснение, не новый defect | 11 |
| Уже покрыто review / самим PR | 7 |
| Policy / evidence change | 8 |
| Недостаточно evidence, большой partial review | 1 |
| Regression подтверждён только hindsight | 1 |
| Всего | 40 |

Индивидуальные строки ниже — итог основного reviewer.
Несколько subagent labels new_action понижены: feature description,
known decision и generic verification request не являются новыми discoveries.

## Все 20 PR: archfit

| PR | Изменение | Роль карточки | Вывод |
| --- | --- | --- | --- |
| [#97](https://github.com/alexei-led/archfit/pull/97) | build: keep non-build inputs out of the image context and the compiler cache out of the layers | Без новой карточки | Docker context/cache hygiene; дополнительного boundary decision нет. |
| [#103](https://github.com/alexei-led/archfit/pull/103) | ci: bump actions/cache to v6.1.0 (every other action is already latest stable) | Без новой карточки | actions/cache version bump; не создавать architecture advice. |
| [#99](https://github.com/alexei-led/archfit/pull/99) | chore(deps): bump github.com/anthropics/anthropic-sdk-go/v2 from 2.2.0 to 2.3.0 in the go-minor-patch group | Без новой карточки | SDK bump без first-party source diff; влияние по go.mod не устанавливается. |
| [#104](https://github.com/alexei-led/archfit/pull/104) | feat(check): one result contract with identity, change status and repository attention (E1) | Пояснение, не новый defect | agent-result.v1 меняет shape; нужен consumer/migration context. Решение уже описано автором. |
| [#105](https://github.com/alexei-led/archfit/pull/105) | feat(check): linter-shaped text output; analyze keeps the report (E2, E10) | Пояснение, не новый defect | check text меняет contract; useful migration context, не новый defect. |
| [#106](https://github.com/alexei-led/archfit/pull/106) | feat(config): lint blocking rules without a rationale; explain the self rules (E6) | Regression подтверждён hindsight | Strict lint конфликтовал с output config init. Reproduction после чтения #120, не prospective discovery. |
| [#107](https://github.com/alexei-led/archfit/pull/107) | feat(check): resolve the change against the target branch and guard the policy (E3) | Пояснение, не новый defect | Target-scoped check и временно HEAD-scoped hooks: известный transitional behavioral contract. |
| [#111](https://github.com/alexei-led/archfit/pull/111) | feat(dotnet): C# and .NET support, end to end | Недостаточно evidence | 287 files, focused review. .NET helper исключён из self-scan; producer check failed. Не green missed-erosion case. |
| [#112](https://github.com/alexei-led/archfit/pull/112) | docs(release): finish the v3.3.0 notes for the check contract | Без новой карточки | Release notes. Ещё один пересказ migration не нужен. |
| [#113](https://github.com/alexei-led/archfit/pull/113) | feat(policy): can-import names where to go instead (E5) | Пояснение, не новый defect | Policy-checked alternative не доказывает semantic equivalence; contract уже описан и tested. |
| [#110](https://github.com/alexei-led/archfit/pull/110) | feat(config): config compare classifies the policy change (E11) | Уже покрыто review / самим PR | PR реализует policy-change classification. Описание этой feature — не новая находка review. |
| [#108](https://github.com/alexei-led/archfit/pull/108) | test(results): state-report fixtures for the five contract cases | Без новой карточки | Report fixtures; дополнительный boundary decision не подтверждён. |
| [#109](https://github.com/alexei-led/archfit/pull/109) | feat(check): files the change adds that no analysis covers are visible, unknown under change_gate: fail (E4) | Уже покрыто review / самим PR | PR реализует uncovered-source detection. Это не uncovered-source finding самого PR. |
| [#114](https://github.com/alexei-led/archfit/pull/114) | feat(hook): hook install and hook run for Claude Code, Codex and git; doctor reports the agent loop (E7) | Пояснение, не новый defect | Hook lifecycle заслуживает contract explanation; нового architectural defect не установлено. |
| [#116](https://github.com/alexei-led/archfit/pull/116) | feat(agents): a short AGENTS.md block and a worker-loop skill; owner material in docs (E8) | Без новой карточки | Agent instructions/docs; дополнительной architecture advice не требуется. |
| [#115](https://github.com/alexei-led/archfit/pull/115) | ci: run the architecture drift check in the image built from the change | Policy / evidence change | Drift check перенесён в image: measurement consistency, не code quality. |
| [#117](https://github.com/alexei-led/archfit/pull/117) | docs(release): v3.4.0 release notes | Без новой карточки | Release notes; подходящих premerge final-head checks в capture нет. |
| [#120](https://github.com/alexei-led/archfit/pull/120) | fix(init): starter rules carry a rationale so config init passes config lint --strict | Без новой карточки | Исправляет уже найденный init→lint mismatch; к merge нового действия нет. |
| [#119](https://github.com/alexei-led/archfit/pull/119) | chore(archfit): pin Action v3.1.0 with the default engine (v3.4.0) | Policy / evidence change | Producer migration требует re-anchor. App уже явно запросил его. |
| [#122](https://github.com/alexei-led/archfit/pull/122) | Update the Archfit baseline | Policy / evidence change | Baseline profile migration; нельзя объявлять устранение code debt. |

## Все 20 PR: ccgram

| PR | Изменение | Роль карточки | Вывод |
| --- | --- | --- | --- |
| [#330](https://github.com/alexei-led/ccgram/pull/330) | feat(protocol): unify Telegram and mobile on shared session core | Пояснение, не новый defect | Общий SessionService для transports. Greptile уже дал architecture summary; 246 files reviewed selectively. |
| [#331](https://github.com/alexei-led/ccgram/pull/331) | Add Archfit advisory feedback | Policy / evidence change | App/CODEOWNERS onboarding: сама feature — не новая находка review. |
| [#334](https://github.com/alexei-led/ccgram/pull/334) | fix(status): isolate optional native probe failures | Без новой карточки | Optional probe isolation исправлена в самом PR; generic architecture card не добавляет решения. |
| [#332](https://github.com/alexei-led/ccgram/pull/332) | fix(files): allow legal Unicode session paths | Без новой карточки | Локальное исправление Unicode path validation. |
| [#335](https://github.com/alexei-led/ccgram/pull/335) | perf(status): share bounded native probes across refreshers | Уже покрыто review / самим PR | Cache freshness concern уже найден review и покрыт regression tests. |
| [#345](https://github.com/alexei-led/ccgram/pull/345) | fix(relay): restore Pi hooks and require agent readiness | Уже покрыто review / самим PR | Readiness bypass уже поднят Greptile; final diff содержит fix. |
| [#347](https://github.com/alexei-led/ccgram/pull/347) | fix(protocol): allow first Claude input before its transcript exists | Уже покрыто review / самим PR | Initial missing transcript vs lost source уже объяснены Greptile. |
| [#349](https://github.com/alexei-led/ccgram/pull/349) | ci: adopt the approved Archfit v3.2.0 producer | Policy / evidence change | Producer pins: evidence recovery не равна улучшению кода. |
| [#348](https://github.com/alexei-led/ccgram/pull/348) | fix(protocol): keep internal provider records out of conversations | Пояснение, не новый defect | Canonical transcript visibility boundary; полезная invariant, но gaps уже обсуждались review. |
| [#341](https://github.com/alexei-led/ccgram/pull/341) | refactor(upgrade): make restart state runtime-owned | Пояснение, не новый defect | Private main-state access заменён runtime callback. Реальное improvement уже есть в facts и intent. |
| [#342](https://github.com/alexei-led/ccgram/pull/342) | fix(archfit): map shared core and adapter contracts | Policy / evidence change | Меняется только policy map. Рост advisories не доказывает erosion. |
| [#351](https://github.com/alexei-led/ccgram/pull/351) | fix(logs): report a multiplexer outage once, with its cause | Уже покрыто review / самим PR | Log/recovery semantics и новый import уже покрыты review. |
| [#350](https://github.com/alexei-led/ccgram/pull/350) | refactor(runtime): isolate monitor and lifecycle callbacks | Пояснение, не новый defect | Neutral monitor + awaited adapter. Existing Archfit/Greptile уже участвовали; новый вклад — presentation. |
| [#352](https://github.com/alexei-led/ccgram/pull/352) | docs: prepare v4.18.4 release notes | Без новой карточки | CHANGELOG only; bot показывает 95 advisories. Repository attention следует отделить. |
| [#353](https://github.com/alexei-led/ccgram/pull/353) | ci: adopt the qualified Archfit v3.2.2 producer | Policy / evidence change | Producer migration, не code-quality event. |
| [#359](https://github.com/alexei-led/ccgram/pull/359) | feat(topics): start a session from one typed line | Пояснение, не новый defect | Trust seeding/defaults — важный, уже заявленный behavioral decision; нового defect не установлено. |
| [#360](https://github.com/alexei-led/ccgram/pull/360) | feat(pi): support interactive command menus in Telegram | Уже покрыто review / самим PR | Menu routing concern найден Greptile и исправлен; finding IDs не меняются. |
| [#361](https://github.com/alexei-led/ccgram/pull/361) | feat(ui): mirror native command menus across coding agents | Пояснение, не новый defect | Provider menu contract: semantic/version limits явно документированы. 76 files не доказывают erosion. |
| [#363](https://github.com/alexei-led/ccgram/pull/363) | docs: prepare v4.21.0 release notes | Без новой карточки | CHANGELOG only; 0 introduced, 95 pre-existing advisories. |
| [#364](https://github.com/alexei-led/ccgram/pull/364) | chore(archfit): migrate to engine v3.4.0 and Action v3.1.0 | Policy / evidence change | Engine/Action/rationale migration; нужна provenance, не новая semantic advice. |

## Итоговое решение

**Сузить, а не расширять.** Не продавать ещё один architecture graph.
Сначала использовать имеющиеся facts для честного change-specific feedback:
resolved improvements, new facts, measurement changes и quiet result.

Новую semantic capability проверять отдельно: находит ли она producer/consumer
и behavioral-contract risks, которые существующий review пропускает.
Этот replay такого преимущества не доказал.

Не подтверждено ни одного нового missed defect как prospective успеха
именно proposed boundary diff. Это не доказательство отсутствия любых дефектов.
Есть evidence для менее шумного и более контекстного App.

## Ограничения и воспроизводимость

- Short-window, largely single-owner sample; не годовой multi-team test.
- Исторические engine/App версии различаются. Observed shortcomings
  не объявляются текущими bugs без current-implementation verification.
- Final merged patches: earlier detected-and-fixed issues не новые misses.
- Нет полного edit history PR bodies и review comments.
- Не rerun все tests/analyzers каждого PR.
- Нет измерения reviewer time, prevented incidents, model calibration или оплаты.
- Исходные main trees archfit и ccgram не менялись.
- Raw capture лежит в local temporary directory, указанном в sample.json.
  Для долговременного воспроизведения его нужно отдельно сохранить.
- GitHub CI artifacts имеют retention; они были доступны при capture.
