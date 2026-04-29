# isolation — Test Corpus (testcases 디렉터리 + 케이스 형식)

**Source repo:** `cubrid-testcases/isolation/` (public)

case-formats.md §4 의 isolation 부분을 *DSL 문법 + 디렉터리 구조* 측면에서 더 깊게 정리.

---

## 1. 규모

| 항목 | 갯수 |
|------|------|
| `.ctl` 케이스 파일 | 6,778 |
| `.answer` 정답 | 6,853 (multi-stage variant 포함) |
| `.answer1` (multi-stage) | 58 |
| `.answer2` | 6 |
| `.answer_1` (변형 명명) | 3 |
| 오타 (.asnwer, .amswer) | 3 (data quality 노이즈) |

→ 1:1 매칭이 약 99%, 다단계 시나리오가 ~70 개 (`.answer1`/`.answer2` 페어).

---

## 2. 디렉터리 분류 (depth 1: 격리 수준)

```
cubrid-testcases/isolation/
├── _01_ReadCommitted/
├── _02_RepeatableRead/
├── _04_RepeatableRead_ReadCommitted/   ← 혼합 격리 시나리오
├── _05_ReadCommitted_RepeatableRead/   ← 혼합 격리 시나리오
├── _06_features/                       ← 기능별 (예: cbrd_22705_online_index_parallel)
└── config/
```

`_03_*` 자리는 비어있음 — 과거 Serializable 슬롯이었거나 이전 / 후속 격리 수준 자리로 의도된 것으로 추정. 새 시스템에서는 *번호 시스템이 아닌 명시적 enum* 으로 표현 권고.

### depth 2 (시나리오 토픽) — `_02_RepeatableRead` 예

```
_02_RepeatableRead/
├── catalog              — system catalog 동시 접근
├── dml_ddl              — DML과 DDL 의 동시성
├── foreign_key_column   — FK 컬럼 락 동작
├── function             — 함수 / stored procedure
├── index_column         — 인덱스 컬럼 락
├── issues               — 알려진 이슈 회귀
├── no_index_column      — 비인덱스 컬럼 락
├── partition_table      — 파티션 테이블 동시성
├── primary_key_column   — PK 컬럼 락
└── serial               — serial / sequence 동시성
```

→ **표준화된 시나리오 분류 트리**가 격리 수준마다 적용되어 있음. 새 시스템도 이 분류를 *first-class metadata* 로 보존.

### depth 3+ (개별 케이스 디렉터리)

각 토픽 디렉터리 아래에 `<scenario_name>.ctl` + `<scenario_name>.answer` 파일. `cases/` + `answers/` 자매 디렉터리는 *없음* (sql/medium 과 다름).

---

## 3. .ctl DSL 문법 (M0 #5 + 본 분석으로 확장)

### 3-1. 기본 구문

```
<actor>: <statement>;
```

actor 토큰:
- `MC` — Master Controller (오케스트레이션)
- `C1`, `C2`, ..., `Cn` — 클라이언트 (concurrent transactions)

statement 는 `;` 로 종료. 멀티라인 SQL은 한 줄 안에 표현 (실제 케이스에서는 한 statement 이 한 줄로 들어감).

### 3-2. 주석

```
/* multi-line comment */
```

(코퍼스 검색 결과 `--` 라인 주석 사용은 isolation 케이스에서 드뭄 — `/* */` 가 표준)

### 3-3. MC (Master Controller) 명령

| 명령 | 의미 |
|------|------|
| `MC: setup NUM_CLIENTS = N;` | N 개 클라이언트 풀 초기화 |
| `MC: wait until <Cn> ready;` | `<Cn>` 이 직전 명령 완료할 때까지 대기 |
| `MC: wait until <Cn> blocked;` | `<Cn>` 이 락 대기 상태에 진입할 때까지 대기 (concurrency 핵심) |
| `MC: pause for deadlock resolution;` | 데드락 해소 대기 (CUBRID 의 deadlock detection cycle) |
| `MC: sleep <N>;` | N 초 sleep (sleep 단위는 second 추정) |

**Data quality 관찰**:
- `MC: setup NUM_CLIENTS =1 ;` (공백 비일관) — DSL 파서가 관대함
- `MC: sleep 1` (세미콜론 누락) 일부 발견 — 마찬가지로 관대 파싱
- `MC: setup NUM_CLIENTS = 22;` (큰 N) — 22 클라이언트 동시 시나리오도 존재

새 시스템 파서는 *현재 ctltool C 파서의 관대함을 유지* 또는 lint 도구로 strict 화 (둘 중 결정 필요).

### 3-4. C<n> (클라이언트) 명령

`C<n>:` 뒤에는 임의의 SQL 문 (CREATE/DROP/INSERT/UPDATE/SELECT/COMMIT/ROLLBACK/SAVEPOINT) 또는 트랜잭션 제어:
- `C1: set transaction lock timeout INFINITE;`
- `C1: set transaction isolation level read committed;`
- `C1: set transaction isolation level repeatable read;`
- 일반 SQL: `C1: SELECT ... FROM ...;`

### 3-5. 결정론 보장 규칙

- **모든 동시성 결정 포인트는 `MC: wait until ...` 으로 표현** — 클라이언트 사이에 *implicit 순서 가정* 없음
- 락 대기를 표현해야 할 때 `wait until <Cn> blocked` 가 *필수* — 없으면 race
- 데드락 발생 가능 시 `MC: pause for deadlock resolution;` 으로 cycle 후 진행

→ 새 시스템에서도 *동일한 결정론 contract* 유지. 명시적 동기 포인트 없는 케이스는 lint 경고 후보.

---

## 4. .answer 파일 형식

```
<원시 문자열 출력>
```

isolation 의 answer 는 sql/medium 처럼 `===...===` 구분자가 *없을 수도 있음* — runone.sh 의 stdout 을 그대로 capture 한 형태. (정밀 형식은 후속 — runone.sh 정밀 분석 시.)

### multi-stage answer

`.answer1`, `.answer2` 는 *케이스가 단계별로 다른 출력을 검증* 할 때 사용. 예:
- `case.ctl` 이 phase 1, phase 2 로 나뉘는 시나리오
- runone.sh 가 phase 마다 결과를 다른 파일에 쓰도록 케이스가 지시

→ DSL 안에서 phase 표기 (예: `C1: -- phase 2 begin`) 가 있는지 corpus 검색 후속 작업으로.

---

## 5. config 디렉터리 — 코퍼스의 메타

```
cubrid-testcases/isolation/config/
```

(내용 정밀 분석 후속) — *케이스 메타데이터 / 그룹 / 알려진 실패 등* 을 담는 것으로 추정. 새 시스템 케이스 메타 디자인 시 입력.

---

## 6. 케이스 작성자 측 contract (동결 표면)

새 시스템 isolation 대체 시 케이스 작성자에게 *변경 없이 보여야* 할 것들:

1. ✅ DSL 문법 (`MC:`, `C<n>:`, `wait until ready/blocked`, `pause`, `sleep`)
2. ✅ 디렉터리 분류 트리 (`_NN_<level>/<topic>/<case>`)
3. ✅ 파일 명명 규칙 (`<name>.ctl` + `<name>.answer` + `.answer1/.answer2` for multi-stage)
4. ✅ 관대한 파싱 (whitespace / 트레일링 세미콜론 누락 등 일부 허용)
5. ✅ SQL 호환 — 케이스 안의 statement 가 CUBRID SQL 그대로

새 시스템이 ctltool 을 *흡수* 한다면 위를 직접 구현. *외부 subprocess* 로 유지하면 ctltool 그대로 동작 (단, 새 시스템이 ctltool 빌드/배포를 포함해야 함).

---

## 7. 새 시스템 corpus 운영 권고

1. **케이스 메타 metadata block** — 새 시스템에서는 케이스 헤더에 다음 메타를 명시 (예시):
   ```
   /* @level: RepeatableRead
      @topic: foreign_key_column
      @issue: CBRD-12345
      @clients: 4
      @timeout: 30
   */
   ```
   현재는 디렉터리 위치로 metadata 가 표현됨 — 명시화하면 추출/필터링 쉬워짐.

2. **lint 도구** — 오타 (.asnwer 등), 누락 세미콜론, race 가능 시퀀스 (wait until 없는 동시 명령) 자동 감지.

3. **multi-stage answer 명시화** — 현재 `.answer1`/`.answer2` 의 phase 정의가 코퍼스 외부 (runone.sh / ctltool) — DSL 안에 phase 표시를 first-class 로.

4. **결정론 검증** — 같은 .ctl 을 100회 실행해서 결과가 같은지 자동 검증 (CI 단계).

5. **`config/` 의 의미 명시화** — 후속 분석 후 새 시스템에서 *케이스 메타 디렉터리* 또는 *suite-level 그룹 정의* 로 정형화.
