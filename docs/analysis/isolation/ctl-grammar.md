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

**즉 `.ctl` 케이스 1건 실행 = MC 1 프로세스 + N 클라이언트 프로세스** (Unix socket 으로 통신).

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
├── qamccom.c / qamccom.h    — MC↔Client Unix socket IPC
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
| `MC: sleep <ms>;` | MC 가 N millisecond sleep |
| `MC: pause for deadlock resolution;` | CUBRID deadlock detection cycle 까지 대기 |
| `MC: wait until C<n> ready;` | client n 이 *직전 statement 완료* 까지 대기 |
| `MC: wait until C<n> blocked;` | client n 이 *락 대기* 진입까지 대기 |
| `MC: wait until C<n> unblocked;` | ⭐ client n 이 *락 대기 해소* 까지 대기 (case-formats 에 없던 토큰!) |
| `MC: wait until C<n> finished;` | ⭐ client n 이 *모든 statement 완료* 까지 대기 (case-formats 에 없던 토큰!) |
| `C<n>: <SQL>` | client n 이 SQL 실행 |

**이전 분석 (case-formats.md §3) 보다 *2 토큰 추가 발견*** — `unblocked`, `finished`. corpus 에 거의 안 나오지만 사용 가능.

---

## 5. MC↔Client IPC (qamccom.c/h)

Unix socket 기반 메시지 교환:

```c
typedef struct qamc_msg {
    int sender_id;        /* network byte order */
    int msgtype;          /* QAMC_SMSG_* */
    int msglen;
    char msg[QAMC_MAX_MSGLEN];
} qamc_msg;
```

- 모든 정수 필드는 network byte order (`ntohl`)
- 헤더 (sender_id + msgtype + msglen) + body (msglen byte)
- msglen == 0 허용 — *signal-only* 메시지

**메시지 타입 (qamccom.c 분석):**
- `QAMC_SMSG_CONTINUE` — MC 가 client 에게 "다음 statement 실행" 신호
- (다른 타입 정밀 후속)

**핵심 패턴:**
- MC 가 `wait_blocked_command`, `wait_ready_command`, `wait_finish_command` 등을 호출
- 각 함수는 `local_tm_isblocked(tran_index)` 같은 *DB 측 트랜잭션 상태 검사* 와 *client 메시지 수신* 을 결합
- client 가 *blocked* 상태 진입 시 MC 가 detect 하는 mechanism — `lock_dump` (cubrid_drv.c 의 외부 심볼) 가 활성

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

## 9. 미해결 후속 분석

- `qamccom.c` 의 모든 메시지 타입 (`QAMC_SMSG_*`)
- `qactl.c` 의 정밀 main flow (.ctl 파일 어떻게 spawn 하고 어떻게 exit code 결정)
- `lock_dump` 가 cubrid_drv.c 의 어디서 정의되는지 — *blocked* 검출의 핵심
- mysql_drv.c / oracle_drv.cpp 의 *현재 빌드 활성* 여부 (Makefile 의 `oracle:` 타겟이 explicit)
- `format_ctl_result` 의 정규화 패턴이 *모든 .ctl 케이스에 같이 적용* 인지 *케이스마다 다른* 인지
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
- qamccom.c 가 Unix socket IPC

DSL 어휘 (확정, case-formats.md §3 보강):
- MC: setup NUM_CLIENTS = N
- MC: wait until C<n> { ready | blocked | unblocked | finished }
- MC: pause for deadlock resolution
- MC: sleep <ms>
- C<n>: <SQL>
+ /* */ 와 -- 주석, 따옴표 문자열, naked-semicolon 종료

새 시스템 입력:
- Option B (subprocess) 가 1차 strangler-fig 에 안전
- 다중 DBMS 의 *현 활성 여부* 확정 필요
- runone.sh sed normalization 동결 필수
- 새 시스템 grammar 정형화 (BNF/PEG)
```
