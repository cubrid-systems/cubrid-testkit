# isolation — ctltool `.ctl` Native Parser & DSL Grammar

**Source:** `cubrid-testtools/CTP/isolation/ctltool/`

`isolation/design.md` 의 §5-1 과 `case-formats.md` §4 에서 미파악으로 둔 *ctltool 의 정확한 DSL 문법 + 아키텍처* 정밀 분석.

**Phase 4 정밀 분석 status:** 완료

---

## 1. ctltool 의 *2-binary 아키텍처*

`ctltool/Makefile` 분석으로 확정:

```
qactl   = MC (Master Controller) — CUBRID 빌드
qactlm  = MC — MySQL 빌드
qactlo  = MC — Oracle 빌드

qacsql  = SQL Client — CUBRID
qamysql = SQL Client — MySQL  
qaoracle = SQL Client — Oracle
```

**즉 `.ctl` 케이스 1건 실행 = MC 1 프로세스 + N 클라이언트 프로세스** — 통신은 클라이언트마다 파이프 세 개(stdin·stdout·stderr)와 평문이다. 소켓이 아니다 (§5 — 2026-09-16 정정).

CTP 의 케이스 실행 경로:
```
Java Test.runTestCase
   └─► SSH: cd $ctlpath; sh runone.sh ...
              └─► qactl <args>             (MC, .ctl 파일 파서/오케스트레이터)
                    ├─► spawn qacsql       (Client 1)
                    ├─► spawn qacsql       (Client 2)
                    └─► ...                (N clients)
```

**OSS 호환 의도:** ctltool 은 *CUBRID 외 MySQL/Oracle 도 지원* — `MC: setup NUM_CLIENTS = N` 시나리오를 다중 DBMS 에서 *동일 .ctl 로* 검증할 수 있다는 설계 의도. 현재 CTP 에서는 CUBRID 만 호출. **새 시스템에서 다중 DBMS 호환을 유지할지 ADR**.

---

## 2. 소스 파일 구조 (15 파일)

```
ctltool/
├── parse.c / parse.h        — statement splitter (state machine)
├── common.c / common.h      — char class / arg parsing utility
├── qamccom.c / qamccom.h    — super controller 와의 소켓. `-slave` 로만 켜지고 아무도 켜지 않는다 (§5)
├── qactl.c                  — MC main entry + .ctl DSL 인터프리터
├── qacsql.c                 — Client main entry + SQL 실행
├── cubrid_drv.c             — CUBRID DB driver (db_drv.h 구현)
├── mysql_drv.c              — MySQL DB driver
├── oracle_drv.cpp           — Oracle DB driver (C++)
├── db_drv.h                 — DB driver 공통 인터페이스
├── Makefile                 — gcc 빌드 정의
├── runone.sh                — 케이스 1건 실행 셸 wrapper (CTP 가 호출)
├── runall.sh                — 디렉터리 일괄 실행
├── prepare.sh               — 환경 셋업
├── timeout3.sh              — 타임아웃 wrapper
├── clean.sh                 — 정리
└── SP_Sleep.java            — Java SP (sleep 시뮬레이션 용)
```

---

## 3. parse.c 의 statement splitter (state machine)

`parse.c` 의 `get_next_stmt(FILE *fp)` 가 *세미콜론 단위로 statement 추출*. 11-state state machine:

```c
typedef enum {
    ACCEPT,                      /* 종료 신호 (naked semicolon) */
    COPY,                        /* 일반 입력 → 출력 복사 */
    ONE_SLASH,                   /* '/' 본 직후 — C 주석 시작 가능 */
    IN_C_COMMENT,                /* /* ... 안 */
    ONE_STAR,                    /* C 주석 안에서 '*' 본 직후 — */ 종료 가능 */
    ONE_DASH,                    /* '-' 본 직후 — SQL 주석 시작 가능 */
    IN_SQL_COMMENT,              /* -- ... EOL 까지 */
    IN_DQ_STRING,                /* "..." 안 */
    IN_DQ_STRING_ESCAPE,         /* "..." 안에서 \ 본 직후 */
    IN_SQ_STRING,                /* '...' 안 */
    IN_SQ_STRING_ONE_SQ,         /* '...' 안에서 '' (이중 단일 따옴표) 가능 */
    IN_SQ_STRING_ESCAPE          /* '...' 안에서 \ 본 직후 */
} STATE;
```

10 character class:
```c
NL    (newline)        WS     (other whitespace)    SQ    (single quote)
DQ    (double quote)   ESCAPE (backslash)           SLASH (/)
STAR  (*)              DASH   (-)                   SEMI  (;)
OTHER
```

**핵심 동작:**
- 입력 라인을 1024-byte 청크로 읽어 *raw_buf* 에 저장
- 출력 *buf* 에 statement 쌓아 가다가 *naked semicolon* (문자열 / 주석 안이 아닌 `;`) 만나면 ACCEPT
- 4096-byte 단위 자동 grow

→ **statement 단위 분리만 함**. actor (`MC:` / `Cn:`) 인식 / 의미 처리는 `qactl.c` 가 담당.

---

## 4. qactl.c 의 .ctl DSL 인터프리터

qactl.c (MC) 가 `parse.c` 로 추출한 statement 를 받아 *DSL 토큰* 매칭. qactl.c 의 hardcoded 토큰:

```c
#define DEADLOCK_PAUSE_TOKEN     "pause for deadlock resolution;"
#define WAIT_TOKEN               "wait until c"
#define WAIT_BLOCKED_TOKEN       "blocked;"
#define WAIT_UNBLOCKED_TOKEN     "unblocked;"
#define WAIT_READY_TOKEN         "ready;"
#define WAIT_FINISHED_TOKEN      "finished;"   /* 추정 */
#define SLEEP_TOKEN              "sleep"
```

help 메시지:
```
command := <prefix><client ID> { blocked | unblocked | ready | finished };
```

→ **확정된 DSL 의미 (case-formats.md §3 보강)**:

| 명령 | 의미 |
|------|------|
| `MC: setup NUM_CLIENTS = N;` | N 개 client 프로세스 spawn |
| `MC: sleep <n>;` | MC 가 n **초** sleep (`sleepms(sleep_time*1000)`, qactl.c:2412) |
| `MC: pause for deadlock resolution;` | CUBRID deadlock detection cycle 까지 대기 |
| `MC: wait until C<n> ready;` | client n 이 *직전 statement 완료* 까지 대기 |
| `MC: wait until C<n> blocked;` | client n 이 *락 대기* 진입까지 대기 |
| `MC: wait until C<n> unblocked;` | ⭐ client n 이 *락 대기 해소* 까지 대기 (case-formats 에 없던 토큰!) |
| `MC: wait until C<n> finished;` | ⭐ client n 이 *모든 statement 완료* 까지 대기 (case-formats 에 없던 토큰!) |
| `C<n>: <SQL>` | client n 이 SQL 실행 |

**이전 분석 (case-formats.md §3) 보다 *2 토큰 추가 발견*** — `unblocked`, `finished`. corpus 에 거의 안 나오지만 사용 가능.

---

## 5. MC↔Client — 파이프와 마커 두 개 *(2026-09-16 정정)*

이 절은 소켓 메시지 교환으로 적혀 있었다. 아니다. 그것은 `qamccom.c` 이고, `qamccom.c` 는 MC 와 클라이언트
사이가 아니라 **MC 와 super controller 사이**의 것이며 `-slave` 로만 켜진다. `runone.sh` 도 `runall.sh` 도
그 플래그를 넘기지 않고, 반대편에 있어야 할 프로그램은 트리에 없다. 표본 60 케이스에서 `qamccom.c` 의 실행
라인은 8.8% 다 (`evidence/isolation-controller.md` §1).

실제 MC↔Client 는 이렇다 (`start_process`, qactl.c:1868):

```
pipe() × 3 → fork → 자식에서 dup2(r[1],1) dup2(e[1],2) dup2(w[0],0) → execvp("qacsql", {qacsql, <db>, "-cl", "<n>"})
```

- MC → Client: 문장을 클라이언트의 표준 입력에 그대로 쓴다 (qactl.c:2515). 쓰고 나서 MC 가
  `MC to C%d: %s\n` 을 자기 출력에 찍는다 (`print_mc_ope`, qactl.c:1818) — **문장을 되울리는 것은 MC 이지
  클라이언트가 아니다**.
- Client → MC: MC 가 8,192 바이트 단위로 `read()` 하고 (`qacsql_output_filter`, qactl.c:1407),
  읽은 덩어리마다 `C%d output (Transaction index = %d):\n` 한 줄을 찍은 뒤 덩어리의 줄마다 `"| "` 를 붙여
  찍는다. 그래서 **줄 앞머리의 `| ` 는 read() 경계에 걸린다** — 클라이언트 한 번의 출력이 한 번의 read 로
  들어오는 한 문제가 없지만, 경계가 줄 가운데 떨어지면 그 줄은 `| ` 가 끼어 둘로 갈린다. 코퍼스의 answer 는
  CTP 가 실제로 받은 덩어리 모양을 담고 있다.
- MC 가 클라이언트 출력에서 긁어내는 문자열은 **둘뿐**이다:
  `"Transaction index = "` (뒤의 수가 그 클라이언트의 트랜잭션 인덱스 — `tran_is_blocked` 에 넘길 값),
  `") is ready."` (하나 볼 때마다 미완 문장 수를 하나 줄이고, 0 이 되면 그 클라이언트는 READY).

blocked 검출은 클라이언트가 아니라 **서버**에 묻는다. `local_tm_isblocked` → `tran_is_blocked (tran_index)`
는 `libcubridcs` 가 내보내되 어떤 헤더에도 없는 심볼이고 (그래서 cubrid_drv.c:50 이 직접 `extern` 선언한다),
클라이언트 스텁이 `NET_SERVER_TM_ISBLOCKED` 를 보내면 서버가 `lock_is_waiting_transaction (tran_index)` 로
답한다 (`transaction_sr.c:576`, `lock_manager.c:7819`). **트랜잭션에 대한 질문이지 부르는 쪽에 대한 질문이
아니므로, 접속한 아무 클라이언트나 남의 트랜잭션을 물을 수 있다** — 별도 프로세스로 떼어낼 수 있다는 뜻이고,
`internal/ctl/native/qablocked.c` 가 그것이다 (ADR-019).

`lock_dump` 은 `wait` 가 끝내 실패했을 때 진단용으로 한 번 부른다 (qactl.c:2270, `#if defined(CUBRID)`).
blocked 검출 경로가 아니다.

---

## 6. cubrid_drv.c 의 DB 인터페이스

`db_drv.h` 가 정의하는 *공통 인터페이스* 를 cubrid_drv.c 가 구현:

```c
int init_db_client(void);
int shutdown_db(void);
void clear_db_client_related(void);
int reconnect_to_server(void);
int get_tran_id(void);
int is_client_restarted(void);
int login_db(char *host, char *username, char *password, char *db_name);
int execute_sql_statement(FILE *fp, char *statement);
int is_executing_end(int errid);
const char *error_message(void);
char *print_class_info(char *cmd_ptr, FILE *file_handler);
void print_ope(int num, const char *str);
int local_tm_isblocked(int tran_index);
```

→ **다중 DBMS 호환** 의 핵심 인터페이스. 새 시스템에서 동일 추상화 보존하면 *cross-DB 격리 테스트 가능성* 유지.

CUBRID 측 implementation 디테일:
- `dbi.h` 의 CUBRID API 사용 (cubridcs 라이브러리)
- 기본 사용자 `public`, password 빈 문자열, host `localhost`
- `tm_Tran_index` / `Client_no` 외부 심볼 의존 (CUBRID 내부)

---

## 7. runone.sh 의 호출 시그니처 정밀화

```bash
sh runone.sh [-n] -r <retry+1> <tc.ctl> <timeout> <db_name>
```

옵션:
- `-n` : core 파일 백업 *안 함* (Java 측 `backup_core_file_yn=false`)
- `-r <N>` : retry 횟수 (config testcase_retry_num + 1)

위치 인자:
- `<tc.ctl>` : 절대경로 (`/` 시작 안 하면 $HOME prefix)
- `<timeout>` : 초 단위
- `<db_name>` : `testing_database` config

**runone.sh 출력 마커** (Java 측 grep 대상, isolation/io-contract.md §4):
- `flag: OK`
- `flag: NOK <reason>`
- `found core file <path>`
- `found fatal error <msg>`

runone.sh 내부 함수:
```bash
write_ok()             — flag: OK + result 파일 작성
write_nok()            — flag: NOK + answer/result diff
format_ctl_result()    — 출력 정규화 (sed로 변동 라인 제거)
```

`format_ctl_result` 가 제거하는 라인 패턴:
- `Success to commit the transaction`
- `Success to rollback the transaction`
- `Transaction index`
- `^QACTL` (qactl 자체 출력)
- `shutting down`
- `set transaction`
- `^| Ope_no =` (와 다음 줄)
- `;$` (세미콜론으로 끝나는 라인 = SQL echo)
- `^INFO`
- `restart master client`

→ **세상 의 모든 .ctl answer 가 이 sed normalization 후에 비교됨**. 새 시스템에서 *동일 normalization* 반드시 보존.

---

## 8. 새 시스템 설계 ADR 후보

### ADR 후보 1: ctltool native 자산 처리 정책 (ADR-007?)

세 가지 옵션:

#### Option A: ctltool 흡수 (DSL 재구현)

새 시스템 안에 .ctl 파서 + MC + Client 로직 직접 구현. 새 언어로 작성.
- **Pros:** 단일 binary, 빌드 단순, 모던 동시성 모델
- **Cons:** 큰 작업 (parse + IPC + DB driver + 다중 DBMS) — 1인 6-12개월 안에 끝나기 어려움
- **위험:** sed normalization 까지 동일하게 작성하지 않으면 회귀

#### Option B: ctltool subprocess 호출 (현 모델 유지)

새 시스템이 ctltool binary 를 *그대로 빌드 + 호출*. 기존 Makefile 그대로 또는 모던 빌드 시스템에 통합.
- **Pros:** 기존 자산 재사용, 회귀 위험 최소
- **Cons:** 새 시스템이 *C 빌드 의존* (Linux gcc + CUBRID lib)
- **권고:** 1차 strangler-fig 의 isolation 부분에서 *Option B* 채택 → 점차 Option A 로 이주

#### Option C: 다중 DBMS 호환 폐기 (CUBRID-only)

`mysql_drv.c` / `oracle_drv.cpp` 제거. CUBRID 만 지원.
- **Pros:** 유지 비용 큰 폭 감소
- **Cons:** *원래 의도된 cross-DB 검증* 능력 상실
- **확정 필요:** CTP 에서 MySQL/Oracle 모드가 *현재 활성으로 사용되는지* 확인. 사용 안 되면 폐기.

### ADR 후보 2: DSL grammar 정형화 (ADR-008?)

현재 hardcoded token. 새 시스템에서 *명시적 grammar* (BNF / PEG / parser combinator) 표현:

```ebnf
ctl_file       = ( statement ";" )*
statement      = comment | mc_command | client_command
comment        = "/*" .* "*/" | "--" .* NL
mc_command     = "MC:" mc_action
client_command = "C" digit+ ":" sql_text
mc_action      = setup | wait | sleep | pause
setup          = "setup" "NUM_CLIENTS" "=" digit+
wait           = "wait until c" digit+ ( "ready" | "blocked" | "unblocked" | "finished" )
sleep          = "sleep" digit+
pause          = "pause for deadlock resolution"
sql_text       = .*    /* parse.c 의 state machine 으로 분리 */
```

→ **`unblocked` / `finished` 토큰 발견** 으로 grammar 가 case-formats.md 보다 풍부. 새 시스템에서 *문서화 필수*.

### ADR 후보 3: result normalization 정형화 (ADR-009?)

runone.sh 의 sed 패턴 (10+ 라인 제거) 을 *명시적 filter chain* 으로 정형화. 케이스 작성자가 직접 *어떤 normalization 을 적용할지* 케이스 메타에 선언:

```yaml
# case.meta.yaml
normalize:
  - remove_lines_matching: "Success to commit"
  - remove_lines_matching: "Transaction index"
  - remove_blocks: ["| Ope_no =", "...next line..."]
```

→ 현재는 *전역 sed* 라 케이스마다 다른 정책 불가. 새 시스템에서 per-case 또는 per-suite 정책 가능하게.

---

## 8a. 코퍼스가 실제로 쓰는 어휘 *(2026-09-16, 전수)*

`cubrid-testcases/isolation` 의 **`.ctl` 파일 6,790개** 전수 — 6,772 는 제외 목록을 적용한 뒤 run 이 판정하는
케이스 수이고, 여기 6,790 은 파일 수다.

| 명령 | 파일 | 등장 |
|---|---:|---:|
| `MC: setup NUM_CLIENTS = n;` | 6,790 | 6,790 |
| `MC: wait until Cn ready;` | 6,786 | 36,754 |
| `MC: wait until Cn blocked;` | 2,480 | 3,236 |
| `MC: wait until Cn unblocked;` | 150 | 153 |
| `MC: sleep n;` (초) | 804 | 921 |
| `MC: pause for deadlock resolution;` | 23 | 24 |
| 클라이언트로 가는 문장 (`Cn:` 또는 접두를 잃어 C1 으로) | 6,790 | 155,968 |

`TestCorpusVocabulary` 가 파서가 돌려준 문장을 분류해 센 값이다 — 주석 안의 명령은 세지 않으므로 같은 코퍼스를
`grep` 하면 조금 더 많이 나온다.

**한 케이스도 쓰지 않는 것:** `wait until Cn finished` · `wait for n` · `reconnect` · `rendezvous with super` ·
`execute` 세 형태 전부 (`exec_stressgen` · `exec_stressexec` 포함) · `allocate client` · `no-op` ·
`client_names =`, 그리고 클라이언트 쪽의 `save state by` · `verify state unchanged|changed` · `simulate` ·
`schema` · `print`. ADR-019 는 이것들을 **거부**한다 — 조용히 무시하지 않고, 이름을 대며 오류를 낸다.

### 문법과 코퍼스가 어긋나는 자리 둘

**1. 한 줄에 문장이 둘이면 두 번째는 C1 으로 간다 — 5개 파일 32개 문장이 엉뚱한 클라이언트로.**
`parse.c` 는 벌거벗은 `;` 에서 자르지 `Cn:` 접두를 보지 않는다. `C2: insert a; insert b;` 는 두 문장이 되고
두 번째는 접두가 없어 qactl.c:2472 의 기본값에 따라 **클라이언트 1** 로 간다.

전수 (`dumpline` — 문장마다 `parse_line_num` 을 함께 찍어 같은 줄을 판정):

| | 파일 |
|---|---:|
| 접두 없는 문장을 같은 줄에 이어 쓴 파일 | 2,256 |
| 그 줄의 주인이 **C1 이 아닌** 파일 (= 진짜 오배달) | **5** (문장 32개) |

| 파일 | 줄 | |
|---|---|---|
| `_04_.../dml_ddl/createtable_03.ctl` | 36 | C2 의 줄, 1개가 C1 으로 |
| `_04_.../index_column/function_index/basic_sql/insert_insert_03.ctl` | 40 | 26개 |
| `_04_.../index_column/multi_index/basic_sql/insert_delete_02.ctl` | 35 | 2개 |
| `_05_ReadCommitted_RepeatableRead/dml_ddl/createtable_03.ctl` | 36 | 1개 |
| `_06_features/cbrd_22705_online_index_parallel/.../insert_delete_02.ctl` | 35 | 2개 |

**줄이 다르면 오배달이 아니다.** 접두 없는 문장이 자기 줄에 홀로 선 것은 작성자가 기본값(C1)을 쓴 것이고,
그런 파일이 2,256개다. 예: `bug_bts_14165.ctl:32-35` 의 준비 블록은 접두 없이 네 줄이고 바로 뒤가
`MC: wait until C1 ready;` 다 — C1 을 의도한 것이 본문에 적혀 있다. *(2026-09-16 정정: 처음에는 "같은 줄"
조건 없이 세어 8개 파일로 보고했는데, 그중 셋은 이렇게 의도된 기본값 사용이었다.)*

**ADR-019 는 고쳤다가 되돌렸다.** 접두 없는 문장을 "그 줄을 연 클라이언트"에게 보내 다섯 파일을 모두 돌린 결과:
둘은 **차이 없음**, 하나는 answer 한 줄(`on statement number: 13`→`14`)만 어긋나고, **둘은 되돌릴 수 없이 깨진다** —
줄의 첫 문장이 바로 막히는 문장이고(그게 그 케이스가 시험하는 것이다), 막힌 클라이언트는 다음 문장을 받을 수 없다.
`insert_delete_02` 측정: qactl 라우팅 551 ms·OK → 줄의 클라이언트로 보내면 **300,328 ms·NOK**
(300초는 컨트롤러가 돌아오지 않을 클라이언트를 기다린 시간). 그래서 **라우팅은 qactl 의 것을 그대로 둔다**;
고칠 자리는 코퍼스다 (`evidence/isolation-controller.md` §5). `{ … };` 로 묶으면 한 문장으로 유지되지만
(`qamc_get_compound_stmt`, qamccom.c:881) 코퍼스에 쓰는 파일이 없다.

**2. `set transaction isolation level` 은 몰래 커밋한다.** 클라이언트가 그 문장을 보면 뒤에 `COMMIT` 을
붙인다 (qacsql.c:682). DSL 에 보이지 않는 부작용이고, 클라이언트를 그대로 두는 한 그대로 남는다.

둘 다 **보존한다**. 1번은 고쳐 봤다가 케이스들이 그것에 의존하는 것이 드러나 되돌렸고(위), 2번은 클라이언트의
행동이며 클라이언트는 그대로 두기 때문이다.

---

## 9. 미해결 후속 분석

- ~~`qamccom.c` 의 모든 메시지 타입~~ **닫힘 (2026-09-16)** — super controller 전용이고 아무도 켜지 않는다 (§5). 새 컨트롤러는 통째로 버린다
- ~~`lock_dump` 가 어디서 — *blocked* 검출의 핵심~~ **닫힘 (2026-09-16)** — blocked 검출은 `tran_is_blocked` 이고 `lock_dump` 은 실패 시 진단용이다 (§5)
- ~~mysql_drv.c / oracle_drv.cpp 의 빌드 활성 여부~~ **닫힘** — ADR-007 이 CUBRID 만으로 못 박았다
- ~~`format_ctl_result` 가 케이스마다 다른지~~ **닫힘 (2026-09-16)** — `runone.sh:48-73` 이 모든 케이스에 같은 순서로 적용한다. 순서가 의미를 가진다 (14번이 15번보다 먼저여야 `key: N(OID: …)` 가 제대로 마스킹된다)
- `qactl.c` 의 정밀 main flow — 새 컨트롤러가 포팅하며 확정한다 (ADR-019)
- `SP_Sleep.java` 의 정확한 사용처 (sleep 시뮬레이션 SP 추정)
- `prepare.sh` / `runall.sh` 의 정확한 호출자

---

## 10. 결론

```
ctltool 정체:
- 2-binary 아키텍처 (qactl MC + qacsql Client)
- 다중 DBMS 빌드 (CUBRID / MySQL / Oracle)
- parse.c 가 statement splitter (state machine)
- qactl.c 가 .ctl DSL 인터프리터 (8 토큰)
- qamccom.c 는 MC↔Client 가 아니라 super controller 용이고 꺼져 있다 (§5)

DSL 어휘 (확정, case-formats.md §3 보강):
- MC: setup NUM_CLIENTS = N
- MC: wait until C<n> { ready | blocked | unblocked | finished }
- MC: pause for deadlock resolution
- MC: sleep <n>   (초)
- C<n>: <SQL>
+ /* */ 와 -- 주석, 따옴표 문자열, naked-semicolon 종료

새 시스템 입력:
- ADR-007 (2026-09-15): 실행부를 그대로 subprocess 로 — 게이트까지
- ADR-019 (2026-09-16): 컨트롤러는 Go 로 다시 쓰고 클라이언트(`qacsql`)는 그대로 둔다.
  이 문서의 §5·§8a 가 그 명세다
- runone.sh sed normalization 동결 필수 — 컨트롤러는 건드리지 않는다 (ADR-009 예약 유지)
```
