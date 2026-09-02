# North Star — cubrid-testkit 의 정체성

- **Date:** 2026-09-02
- **Status:** Accepted (Phase 1 진입 산출물)
- **Inputs:** ROADMAP §2, `concept/phase0-retrospective.md` §4, ADR-001/002/003/004
- **Analysis baseline:** cubrid-testtools @ 86992c1b334d55800f2700d60f9809c2ceca268d

---

## 1. 한 문장

> **cubrid-testkit 은 CUBRID 기능 테스트를 *실행하는 일만* 하는 러너다.**
> 케이스를 찾고, 돌리고, 판정하고, 무슨 일이 있었는지 남긴다. 그 이상은 하지 않는다.

**CTP 대체는 목표가 아니라 경로다.** CTP 는 러너 안에 스케줄러·메일러·이슈 등록기·메시지 큐를 함께
담았고, 이 프로젝트가 하는 일은 그중 *실행*을 도로 꺼내는 것이다. 다만 기존 시스템이 그동안 계속
돌아야 하므로, 꺼내는 방식이 strangler-fig 점진 대체이고 외부 표면을 동결하는 것이다.

정체성은 **"무엇을 하는가"** 로 정의되지 **"무엇을 대체하는가"** 로 정의되지 않는다.
CTP 가 사라진 뒤에도 이 문장은 그대로 성립해야 한다.

"물려받는다"는 ADR-003 의 동결 명세(`external-surface-freeze.md`)로 정의되고,
"재작성한다"는 ADR-001(Go) · ADR-002(go build + Justfile) · ADR-004(shell 1차 대체)로 정의된다.

---

## 1a. 프로젝트의 방향성 — 축 분리 *(2026-09-02, 최우선 원칙)*

> **테스트 실행과 QA 시스템 운영은 다른 축이다. 현재 CTP 는 이 둘이 한 덩어리로 뭉쳐 있다.
> 이번 마이그레이션은 축 T 만 옮긴다. 축 O 는 제외하고, 제외 사실을 기록하고, 나중에 새로운 층으로 세운다.**

이것이 M1~M5(§2)보다 상위의 결정이다. 새 표면을 만나면 **등급을 붙이기 전에 축을 먼저 판정**한다.

| 축 | 판정 질문 | 처리 |
|---|---|---|
| **T (테스트 실행)** | 이 기능이 없으면 테스트 *결과*가 달라지는가? | `external-surface-freeze.md` 로 동결 |
| **O (QA 운영)** | 없어도 테스트 결과는 같은가? (스케줄·메일·이슈등록·큐·리포트) | `migration-exclusions.md` 로 제외 + 기록 |

축은 패키지·파일·jar 로 나뉘지 않는다. `coreanalyzer` 안에서도 `AnalyzerMain`(core 분석)은 T, `IssueMain`(이슈 등록)은 O 다. `RunShellMain` 의 CLI 옵션 13개는 **한 목록 안에서** T 8 / O 5 로 갈린다.

**왜 상위 원칙인가** — 축이 섞여 있으면 테스트 러너가 SMTP·Quartz·JIRA 클라이언트를 떠안는다. 지금 `cubridqa-scheduler.jar` 가 shell 모듈 classpath 에 실려 있는 이유가 정확히 이것이다. 축을 가르면 Phase 3 범위가 줄고, 새 운영 층은 `testkit` 의 *출력을 소비하는* 단방향 의존으로 깨끗하게 설계된다.

---

## 2. "모던 설계" 의 구체적 의미

추상어를 Phase 0 이 발견한 *구체적 결함* 에 1:1로 묶는다. 아래 5개가 이 프로젝트가 말하는 "모던"의 전부이며, 이 목록 밖의 개선은 non-goal 이다.

| # | 기존 CTP 의 결함 (Phase 0 근거) | 새 시스템의 목표 상태 |
|---|---|---|
| M1 | **호출 모델 3종 혼재** — process exec(sql/jdbc) / in-process reflection(shell·isolation·ha_repl·cdc_repl·unittest) / utility 탈출 분기(webconsole). `cli-tree.md` 의 dispatch 패턴 분류 | **단일 plug-in 인터페이스**. task 는 모두 같은 `Runner` 계약으로 등록·실행. 외부에서 보이는 CLI·종료 코드·stdout 은 불변. |
| M2 | **in-band signaling** — `#SCRIPTCONT` 접미사 라인을 stdout 에 흘려 상위 셸이 후행 실행. `cli-tree.md` 의 설계 주목점 4번 | 명령 채널과 로그 채널 **분리**. interactive 모드의 *동작*은 보존하되 전달 메커니즘은 내부 구현으로 강등. |
| M3 | **노후 third-party 의존** — log4j 1.2.16(EOL) / commons-io 1.3(2007) / dom4j 1.6.1(2005) / jgit 4.3 / ActiveMQ 5.8 / Quartz 2.2.1. `ADR-001` §1 | Go 표준 라이브러리 + `x/crypto/ssh` 중심. 외부 의존 수를 **의도적으로 최소화**하고 모두 go.mod 로 pin. |
| M4 | **암묵적 전역 상태** — `CTP_HOME` 절대경로 가정이 모든 subprocess 호출에 관통. 7개 jar 의 classpath 가 고정 경로. `cli-tree.md` 의 설계 주목점 6번 | 실행 컨텍스트를 **명시적 구조체로 전파**. `CTP_HOME` 은 호환 입력으로만 받고 내부 경로 해석은 단일 지점에 격리. |
| M5 | **dead surface** — 21 ComponentEnum 중 7개(CCI/DOTS/NBD/SYSBENCH/TPCC/TPCW/YCSB)가 switch 분기 없는 silent no-op. `orphan-enums.md` | dead 표면을 **명시적으로 폐기**(ADR-005). 침묵하는 no-op 을 남기지 않는다. |

---

## 3. "확장성" 의 구체적 의미

**단 하나의 확장점만 약속한다: case-format ingestion 인터페이스.**

Phase 0 이 확인한 사실 — 모듈마다 케이스 형식이 다르다(`.sql` / `.ctl` / `.sh`, `case-formats.md`). 이 다양성이 이미 시스템 안에 있으므로, 형식을 **plug-in 경계로 승격**하는 것이 자연스러운 확장 축이다.

```
Runner plug-in  ──┬── 형식 파서 (.sql / .ctl / .sh / 외부 포맷)
                  ├── 실행 채널 (local / SSH / subprocess)
                  └── 판정기 (expected-diff / oracle / hash)
```

이 인터페이스가 §6a 확장(E1 sqllogictest ~ E7 workload)의 **유일한 진입 슬롯**이다. `design/contracts.md`(Phase 2)에 이 계약을 명시하는 것이 §6a 항목들이 Phase 2 에 미치는 유일한 영향이다.

확장성은 여기까지다. 플러그인 DSL, 동적 로딩, 설정 기반 파이프라인 조립 같은 *추측성 유연성*은 만들지 않는다.

---

## 4. "호환성" 의 구체적 의미

호환성은 **`concept/external-surface-freeze.md` 에 열거된 것이 전부**이고, 거기에 없는 것은 호환 대상이 아니다.

- **동결한다** — `ctp.sh` CLI 문법과 task 이름, conf 81 키와 dot-notation 의미, stdout 마커, 결과 파일 포맷(`main.info` / `summary_info` / `dispatch_tc_*.txt`), 종료 코드, 원격 실행 컨트랙트(`runone.sh` 시그니처 / `init.sh` / UNITTEST 4함수), 케이스 디렉터리 규약(`cases/` + `answers/`).
- **동결하지 않는다** — `cubridqa-*.jar` 7개의 **산출물 이름과 위치**, Java 클래스/메서드 API(`CommonUtils` 40+ / `IniData` 15), RMI·ActiveMQ·grepo 데몬의 wire 프로토콜, `#SCRIPTCONT` 전달 메커니즘, dispatch 내부 모델. (ADR-003 결정)

호환성의 **판정 방법**도 함께 고정한다: 같은 testcases 입력에 대해 신/구 시스템의 동결 표면 출력이 일치하는지를 `evidence/regression-shell.md`(Phase 3 Exit)로 증명한다. 코드 리뷰나 육안 확인은 증거로 인정하지 않는다.

---

## 5. 성공 기준

1인 사이드 6~12개월 호라이즌에서 이 프로젝트가 성공했다고 말할 수 있는 조건:

1. **Phase 3 Exit** — `ctp.sh shell` / `rqg` / `unittest`(+ `jdbc`, shell.jar 공유로 딸려 옴)가 새 Go 바이너리로 실행되고, 기존 shell-format 케이스 코퍼스에 대해 회귀 동등성이 증거로 남는다.
2. **공존 무해성** — 나머지 task(sql/medium/kcc/neis05/neis08/sql_by_cci/isolation/ha_repl/cdc_repl/webconsole)는 그 기간 동안 기존 CTP 로 계속 동작하며, 사용자는 어느 task 가 신/구인지 알 필요가 없다.
3. **운영 비용 감소** — 배포가 "단일 바이너리 + conf + 셸 자산" 으로 축소된다(JVM·jar tree 불필요).

이 3개 중 1번이 최우선이다. 2·3번은 1번의 부수 효과로 따라온다.

---

## 6. 이 문서가 *아닌* 것

- 아키텍처 설계가 아니다 → `design/architecture.md` (Phase 2)
- 동결 명세가 아니다 → `concept/external-surface-freeze.md`
- 하지 않을 일의 목록이 아니다 → `concept/non-goals.md`
