# E5 — Parser / Protocol Fuzzing Harness (libFuzzer) (Requirements)

**Source:** survey/dbms-testing-ecosystem.md §7 + §11
**Status:** incubating (조건부 — cubrid 본 repo fuzz target build option 선결)
**축 매핑:** 축 5 (Parser / compiler fuzzing)
**사다리 위치:** ROADMAP §6a 사다리 순위 1 (SQL parser) · 3 (network packet decoder) · 4 (record serialize/unpack)
**후속 항목:** `../E9-storage-fuzzing/` — 본 항목의 인프라 위에 얹히는 *구조화 상태 fuzzing* (사다리 순위 5)
**Companion docs (후속):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`

---

## 1. 이 확장이 해결하는 문제

CUBRID 의 **frontend (lexer / parser / binder / planner)** 와 **binary protocol 진입점 (CCI / JDBC)** 의 강건성을 *byte-level coverage-guided fuzzing* 으로 검증한다.

기존 testkit 의 모든 모듈은 *valid SQL surface* 만 본다. fuzz 대상:
- SQL parser crash / UB
- heap overflow / stack corruption
- infinite recursion
- binary protocol parser corruption (CUBRID 의 CCI / JDBC 프로토콜 진입점 — 사다리 순위 3)
- **record serialize / unpack 경로** (`or_get_value` / `or_put_value` / `or_unpack_value`,
  `src/base/object_representation.h` — 사다리 순위 4). 디스크·통신에서 읽은 바이트를
  `DB_VALUE` 로 복원하는 지점으로, *손상된 record* 입력에 대한 강건성이 검증된 적 없다

위 타깃들은 모두 **stateless byte-in** 형태다 — 같은 `LLVMFuzzerTestOneInput` 시그니처를 공유하고
같은 corpus·triage 인프라를 쓴다. 반대로 *상태가 누적되는* storage operation 열은 별 항목
(**E9**) 으로 분리했다 — state reset 이라는 다른 설계 난제가 붙기 때문이다.

§6a-E2 (SQLsmith) 는 *문법 통과* random SQL — 본 항목 (libFuzzer) 은 *문법 외* byte-level mutation. 둘은 *상보적*: SQLsmith 가 못 만드는 SQL 을 libFuzzer 가 만들어 *오히려 그래서 parser crash 가 잘 잡힘*.

**잡는 버그 종류:** parser crash / heap overflow / stack corruption / infinite recursion / protocol parser corruption / 손상 record 의 unpack 시 out-of-bounds.

---

## 2. 외부 호출 형태 (제안 — incubating)

```
ctp.sh fuzz-harness [-c <fuzz.conf>] [--target parser|cci|jdbc|record] [--corpus <dir>]
   또는
testkit run fuzz [--target parser|cci|jdbc|record] [--time <sec>] [--max-len <bytes>]
```

내부 진입 (의제):
```
FuzzHarnessDriver.exec(config)
  ├─ start cubrid server (or in-process target)
  ├─ select target binary (built with -DENABLE_FUZZING by cubrid 본 repo)
  ├─ run libFuzzer / AFL with seed corpus
  ├─ on crash:
  │    save input bytes + stack hash
  │    triage: dedupe by stack hash
  └─ replay mode:
       feed past crash inputs to new build (regression seed)
```

**testkit 책임 한정:** corpus 보관 + replay + crash triage.
**cubrid 본 repo 책임:** fuzz target build option (`-DENABLE_FUZZING` 등) 추가.

**외부 표면 동결 영향:** 없음 (신규 진입점).

---

## 3. 사용자 요구사항 (incubating 추정)

1. **fuzz target layer 선택** — lexer / parser / binder / planner / executor / CCI protocol / JDBC protocol / record serialize·unpack 중 하나 이상
2. **seed corpus 관리** — 의미 있는 SQL / 의미 있는 protocol 메시지의 초기 seed
3. **crash dedup** — stack hash 로 중복 crash 통합
4. **regression seed 누적** — 과거 crash 입력을 새 빌드에서 재실행
5. **coverage feedback 통합** — libFuzzer / AFL 의 coverage 정보를 build 별로 비교
6. **time / iteration budget** — CI 안에서 정해진 시간 내 동작
7. **triage 보고** — crash 분류 (signal / address sanitizer / undefined behavior)

---

## 4. 비기능 요구

| 항목 | 의제 | 새 시스템에서의 의미 |
|------|------|---------------------|
| 도입 비용 | 중 (testkit 단독) ~ 높음 (cubrid 본 repo 협업 포함) | survey §7.3 |
| 즉시 ROI | ★★★ | 가치 큼, 단 testkit 단독 책임으로는 *부정합 위험* |
| 선결 의존 | cubrid 본 repo 의 `-DENABLE_FUZZING` 등 build option | testkit 단독 결정 불가 |
| 책임 경계 | testkit = corpus + replay + triage / cubrid = fuzz target build | **C-055** (roadmap cross-cutting, 등록됨) — 엔진 쪽은 **N66-fuzz-target-infrastructure** |
| 라이선스 | libFuzzer Apache 2.0 / AFL Apache 2.0 | 자유 |
| §6a-E2 (SQLsmith) 와의 보완 | grammar-aware vs byte-level — 둘 다 parser crash 검출 | hybrid CI (E8) 시 함께 |

---

## 5. 의존하는 외부 자원

- **cubrid 본 repo fuzz target build option** — `-DENABLE_FUZZING` 또는 등가. *선결*. testkit 단독 결정 불가
- **libFuzzer / AFL / honggfuzz** — coverage-guided fuzzer 본체
- **ASan / UBSan / MSan** — sanitizer 빌드 (cubrid 본 repo 책임)
- **seed corpus** — 의미 있는 SQL / protocol 메시지 초기 자산
- **crash corpus storage** — testkit 내부 또는 외부 (NG1 점검)

---

## 6. incubating 진입 조건 (조건부)

다음이 *충족된 후* 정식 incubating 진입 (owner: hgryoo):

1. **cubrid 본 repo PR 선결** — `-DENABLE_FUZZING` build option 추가 + sanitizer 빌드 산출물 정의. testkit 단독 시작 불가
2. **fuzz target layer 1차 선정** — SQL parser 만 / + CCI·JDBC protocol / + record serialize·unpack. **권장 순서는 ROADMAP §6a 사다리** (1 → 3 → 4)
3. **fuzzer 본체 선택** — libFuzzer (in-process) vs AFL (subprocess) vs honggfuzz
4. **seed corpus 정책** — 기존 sql 모듈 케이스를 seed 로 변환할지, 별도 seed 자산을 만들지
5. **crash corpus 위치** — testcases 레포 동결 (NG1) 점검
6. **C-055 (roadmap cross-cutting, 등록됨 2026-09-03)** — testkit §6a-E5 × **N66-fuzz-target-infrastructure** — fuzz target build 책임 위치. 엔진 쪽 작업은 roadmap repo `projects/00-pending-review/N66-fuzz-target-infrastructure/` 에 등록되어 있으며, 본 항목의 선결 조건 1 이 가리키는 실체가 그것이다. C-004 (testkit × engine-suite) 의 빌드 표면 경계와도 접한다

**ADR placeholder:**
- ADR-EXT-005 — fuzz target build option (cubrid 본 repo 측) + fuzzer 본체 선택 + corpus 위치 + 책임 경계

---

## 7. 위험 / 정합성 메모

- **책임 경계 위반 위험** — testkit 이 fuzz target build 까지 떠안으면 *모듈 경계 부정합*. cubrid 본 repo 측 PR 이 선결
- **NG1 충돌 가능** — crash corpus 가 testcases 에 들어가면 동결 위반. 외부 storage 권장
- **NG2 / NG4 충돌 없음**
- **§6a-E2 (SQLsmith) + E5 (libFuzzer) 결합** — 둘 다 parser crash 검출이지만 영역이 다름 — 함께 도입 가치 큼
- **분기 게이트 §7** — strangler-fig 우선원칙 — Phase 3·4 와 자원 충돌 시 후순위
- **E9 (storage structured fuzzing) 이 본 항목에 의존** — `-DENABLE_FUZZING`, corpus 보관, triage, coverage 보고를 그대로 재사용한다. 본 항목의 인프라 결정이 E9 를 구속하므로, ADR-EXT-005 에서 target 값 공간(`--target`)을 *확장 가능하게* 열어 둔다
