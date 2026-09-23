# Extensions — §6a Beyond Strangler-fig

*[English](README.md) · 한국어*

ROADMAP §6a "확장 영역" 의 functional requirements 모음. 각 항목은 *strangler-fig 외부* 의 *additive 작업* 으로, NG1·NG2·NG4 동결 *밖* 에서 신규 결정 자유도가 크다.

**이 디렉터리는 *결정* 이 아니라 *근거 + 의제*** — 각 항목의 정식 incubating 진입은 ADR-EXT-NNN 으로만 일어난다.

---

## 카탈로그

| ID | 축 | 이름 | 진입성 | 선결 의존 | requirements |
|---|---|---|---|---|---|
| E1 | 1 | sqllogictest 적용 | 즉시 후보 | — | [E1-sqllogictest/](E1-sqllogictest/requirements.md) |
| E2 | 2 | Random SQL fuzzing (SQLsmith) | 즉시 후보 | — | [E2-sqlsmith/](E2-sqlsmith/requirements.md) |
| E3 | 3 | Logic bug detection (SQLancer NoREC+TLP) | **진행 중 (동시 트랙)** | — | [E3-sqlancer/](E3-sqlancer/requirements.md) · [ADR-EXT-003](../../project/adr/ADR-EXT-003-sqlancer-cubrid.md) · 저장소 `cubrid-sqlancer` |
| E4 | 4 | Distributed isolation testing (AWDIT/Jepsen) | 조건부 | N24 / N11 graduation | [E4-distributed-isolation/](E4-distributed-isolation/requirements.md) |
| E5 | 5 | Parser/protocol fuzzing harness (libFuzzer) | 조건부 | cubrid 본 repo `-DENABLE_FUZZING` | [E5-parser-fuzzing/](E5-parser-fuzzing/requirements.md) |
| E6 | 6 | Differential testing (PostgreSQL pair) | 조건부 | N13 pg-wire-compat selected 이상 | [E6-differential/](E6-differential/requirements.md) |
| E7 | 7 | Stateful / randomized workload | 조건부 | C-004 책임 경계 정의 | [E7-workload/](E7-workload/requirements.md) |
| (E8) | 8 | Hybrid CI 통합 (Materialize 패턴) | 메타 | E2~E7·E9 중 둘 이상 채택 | (TBD — 카탈로그 항목 외) |
| E9 | 5 확장 × 8 | Storage-engine **concurrency** fuzzing (schedule × interleaving) | 조건부 | **E5 선행** + SERVER_MODE in-process 기동 + **E10** | [E9-storage-fuzzing/](E9-storage-fuzzing/requirements.md) |
| E10 | 보조 설비 | XASL fixture 생산·보관 (버전 식별 포함) | 즉시 후보 | — (엔진 변경 없음) | [E10-xasl-fixtures/](E10-xasl-fixtures/requirements.md) |

**번호 공간 주의.** `E8` 은 축 8 *Hybrid CI 통합* 메타 자리로 예약되어 있다. E9 가 E8 을 건너뛴 것은 결번이 아니라 이 예약 때문이다.

---

## E-번호가 아닌 것 — `extensions/cluster-sandbox`

`extensions/` 아래 submodule 이지만 **카탈로그의 항목이 아니다.** E1~E10 은 전부
*테스트 능력* — 새 오라클, 새 케이스 포맷, 새 생성기 — 이고, `cubrid-cluster-sandbox`
는 **환경 제공자**다. 무엇을 검증하는지가 아니라 어디서 도는지를 바꾼다.

E-번호를 주면 두 가지가 틀어진다. 착수 순서를 정하는 §6a 사다리에 "먼저 해야 하는
인프라"가 경쟁 항목으로 끼어들고, **여러 항목의 공통 의존**이라는 사실이 표에서
사라진다. 로드맵이 이미 세 군데에서 그 의존을 적고 있다 — E4(분산 isolation)의 경계,
E7(workload)의 경계, 사다리 순위 6(recovery/crash).

그리고 그 앞에 strangler-fig 쪽 의존이 둘 더 있다. `ha_repl` task 와 HA shell suite
는 master/slave 토폴로지가 없어 지금까지 돌지 못했고(ADR-013 이 HA 트리 367 케이스를
증거에서 제외한다), 그 토폴로지를 **러너가 만들면 안 된다**는 것이 ADR-014 다:

> **HA is not an exception.** The system under test has a topology; the runner does
> not have a fleet.

그래서 `cluster-sandbox` 를 쓰는 것은 ADR-014 를 우회하는 게 아니라 **지키는 방법**이다.
토폴로지를 세우는 일은 저쪽에 있고, 이쪽은 그 위에서 케이스를 돌린다.

| | |
|---|---|
| 저장소 | `cubrid-systems/cubrid-cluster-sandbox` (public) |
| 위치 | `extensions/cluster-sandbox` — submodule, `bot/bump-cluster-sandbox` 가 포인터를 따라 올린다 |
| 통합 형태 | subprocess + `--json` 아티팩트 (ADR-001 Consequence 4). 링크하지 않는다 |
| 이쪽 코드 | `internal/sandbox` — Channel 하나와 topology provider 하나 |
| 기록 | [ADR-022](../../project/adr/ADR-022-topology-provider.md) — 확정 (2026-09-20) |

**착수 순서는 이 표가 정하지 않는다.** fuzzing 계열(E3·E5·E9)과 미등록 후보 2건의 우선순위는 ROADMAP **§6a 사다리** 가 단일 출처다.

---

## 각 항목의 산출물 구조

`project/analysis/{module}/` 의 5 산출물 패턴을 따른다 — *단, incubating 단계라 detail 은 stub*:

```
extensions/E{N}-{name}/
├── requirements.md        — 해결 문제, 사용자 요구, 비기능, incubating 진입 조건  (FULL)
├── design.md              — 아키텍처 / 모듈 위치 / 데이터 흐름                    (STUB — ADR-EXT-NNN 후 보강)
├── io-contract.md         — CLI / conf / 출력 포맷 / 종료 코드                    (STUB — ADR-EXT-NNN 후 보강)
└── test-corpus.md         — 입력 코퍼스 출처 / 라이선스 / 보관 정책                (STUB — ADR-EXT-NNN 후 보강)
```

`implementation-notes.md` 는 *구현 진척 후* 추가. incubating 단계에는 비어 있음.

---

## ADR-EXT 자리표시자 인덱스

| ADR | 트리거 | 결정 항목 |
|---|---|---|
| ADR-EXT-001 | E1 incubating 정식 진입 | sqllogictest spec variant + 코퍼스 import 정책 + 결과 비교 모드 + SUT 클라이언트 |
| ADR-EXT-002 | E2 incubating 정식 진입 | SQLsmith 재사용/재구현 + dialect 가산 범위 + corpus 위치 + crash 판정 채널 |
| ADR-EXT-003 | ~~트리거~~ **Accepted 2026-09-02** | NoREC 1차 + 별도 저장소(ServiceLoader SPI) + 재사용 + corpus 는 testcases 밖 |
| ADR-EXT-004 | E4 incubating 정식 진입 | AWDIT/Jepsen 1차 선택 + 토폴로지 자동화 + fault injection 채널 + corpus |
| ADR-EXT-005 | E5 incubating 정식 진입 | fuzz target build option (cubrid 본 repo) + fuzzer 본체 + corpus + 책임 경계 |
| ADR-EXT-006 | E6 incubating 정식 진입 | peer DBMS + mode (canonical vs rewrite) + dialect rewrite catalog + corpus |
| ADR-EXT-007 | E7 incubating 정식 진입 | scenario 1차 선정 + invariant 카탈로그 + engine-suite 책임 경계 + corpus |
| (ADR-EXT-008) | E8 (Hybrid CI) 정식 진입 | *예약* — 축 8 메타 항목 자리 |
| ADR-EXT-009 | E9 incubating 정식 진입 | 입력 IR + **스케줄 표현** + 참가자 수 상한 + corpus 위치 + 본 repo 책임 경계(rendezvous 핸들러) |
| ADR-EXT-010 | E10 incubating 정식 진입 | 생산 경로(csql/CCI/JDBC) + 픽스처 포맷 + 버전 식별 방식 + 보관 위치 |

---

## 우선순위 (survey 결론)

1. **즉시 후보 (strangler-fig Phase 3·4 와 *병행* 가능):** E2 (SQLsmith), E3 (SQLancer NoREC+TLP)
   - 도입 비용 낮음, 의존 없음, *지금 testkit 이 비어 있는 영역* 을 직접 채움
   - PostgreSQL ecosystem 의 *de facto* 모범
2. **조건부 후보 (선결 의존 충족 후):** E5 (cubrid 본 repo PR), **E9 (E5 + E10 선행)**, E6 (N13 selected), E4 (HA graduation), E7 (C-004 정의)
   - **E10 은 즉시 후보** — 엔진 변경이 없고 선결 의존도 없다. E9 Tier 2 의 선결이면서 독립 실행 가능
   - E5 → E9 는 *같은 인프라를 공유하는 한 줄기*. 순서 역전 시 중복 구축 (ROADMAP §8 risk)
3. **장기 추적:** SQLancer++ (adaptive grammar), FoundationDB simulation 컨셉, §6a 사다리 순위 6·7 (recovery/crash framework, concurrency schedule fuzzing — 미등록)

---

## 위험 / 정합성 공통 메모

- **NG1 (testcases 레포 동결)** — fuzz / mismatch / crash / violation corpus 가 testcases 에 들어가면 위반. *외부 storage 권장* (각 항목 §6 참조)
- **E9 의 protobuf 는 프로토콜이 아니다** — CUBRID 자체 바이너리 프로토콜과 무관하다. protobuf 는 *fuzzer 내부 입력 IR* 이며 fuzz 바이너리에만 링크된다 (E9 requirements §2). 이 오해가 반복되면 항목 자체가 잘못 반려될 수 있다
- **NG2 (외부 표면 동결)** — §6a 항목은 *모두 신규 진입점* 이라 충돌 없음
- **NG4 (비-CUBRID DBMS 호환 금지)** — §6a 항목은 *CUBRID 가 SUT* — 충돌 없음
- **분기 게이트 §7** — *strangler-fig 우선원칙* (ROADMAP §8). §6a 진척을 별 행으로 분리 기재
- **case-format ingestion 인터페이스** — Phase 2 `project/design/contracts.md` 에 hybrid 합성 가능성 반영 (E1~E7 공유). **E9 는 예외** — case format 을 거치지 않고 내부 API 를 직접 호출한다

---

## 출처

- `../../project/survey/dbms-testing-ecosystem.md` — 8축 분류, 도구·연구 catalog, §6a-E2~E7 + E9 후보 도출 근거
- `../../project/ROADMAP.md` §6a — 카탈로그 / Phase 정합 / Open Questions / ADR-EXT 자리표시자
- `../../project/ROADMAP.md` §6a 부록 — **fuzzing 우선순위 사다리** (착수 순서의 단일 출처)
- `../../project/analysis/{module}/` — strangler-fig 대상 모듈의 Phase 0 산출물 (참고)
