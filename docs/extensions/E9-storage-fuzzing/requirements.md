# E9 — Storage-engine Concurrency Fuzzing (schedule × operation interleaving) (Requirements)

**Source:** ROADMAP §6a-E9 + §6a 부록 (fuzzing 우선순위 사다리) · survey §7.4
**Status:** incubating (조건부 — E5 선행 + SERVER_MODE in-process 기동)
**축 매핑:** 축 5 확장 (engine-internal) × 축 8 (schedule/model-based) — 축 4·7 과 층이 다름
**사다리 위치:** §6a 사다리 순위 5 (원안 6행)
**Companion docs (후속):** `design.md`, `io-contract.md`, `test-corpus.md`, `implementation-notes.md`
**참조 구현:** RocksDB `fuzz/` — *입력 구조화 형태* 만 참조. 대상은 다르다 (§1.0)

> **재정의 (2026-09-03, 사용자 결정).** 본 문서는 처음에 *단일 스레드 operation sequence*
> fuzzing 으로 쓰였다. 그 형태는 값이 거의 없다 — 아래 §1.0. 재정의 이전 판의 판단 중
> 유지되는 것과 폐기되는 것을 각 절에 표시한다.

---

## 1. 이 확장이 해결하는 문제

### 1.0 왜 단일 스레드 operation sequence 가 아닌가 (2026-09-03 재정의)

두 가지가 이 항목의 형태를 바꿨다.

**(1) 찾으려는 결함이 단일 스레드에 없다.** 아래 §1.1 이 나열하는 상태들은 대부분 두 개
이상의 실행 흐름이 겹쳐야 생긴다. 한 스레드가 연산을 아무리 많은 순서로 늘어놓아도
latch 순서나 vacuum 간섭에는 도달하지 않는다.

**(2) 저장 계층은 모드별로 갈라진 코드다.** `SERVER_MODE` 분기가 `page_buffer.c` 에 **146**
개, `log_manager.c` 에 **53** 개, `vacuum.c` 에 **26** 개 있고 `vacuum_Master_daemon` 은
`cubthread::daemon` 이다. 단일 스레드 모드(SA)에서의 관측은 SERVER_MODE 서버로 이어지지
않는다. 대상 라이브러리도 **`libcubrid.so`** (SERVER_MODE) 이며 — `heap_insert_logical`,
`btree_insert`, `boot_restart_server` 가 모두 여기서 export 된다 — SA 라이브러리가 아니다.

따라서 입력은 연산 열 하나가 아니라 **`(스레드별 연산 열 × 인터리빙)`** 쌍이다.

CUBRID 의 **storage engine 내부 API** (heap / B-tree / slotted page / overflow / MVCC 가시성)
를 *구조화된 operation sequence* 로 fuzzing 한다. 입력은 byte 뭉치가 아니라
**유효한 DB 연산 열(sequence)** 이며, 그 열 자체를 mutate 한다.

### 1.1 기존 축이 못 잡는 영역

| 기존 축 | 도달 범위 | 사각지대 |
|---|---|---|
| E5 (parser/protocol fuzz) | frontend byte 진입점 | *stateless* — 엔진 내부 상태 누적이 없음 |
| E3 (SQLancer) | valid SQL 의 wrong-result | SQL 로 표현 가능한 조합만. 내부 API 직접 호출 불가 |
| E2 (SQLsmith) | grammar 통과 random SQL | 동상 |
| strangler-fig 모듈 전체 | 손으로 쓴 케이스 | 연산 *순서* 의 조합 폭발을 탐색하지 않음 |

storage engine 버그는 *하나의 연산* 이 아니라 **동시에 진행되는 연산들의 인터리빙** 이
만든 상태에서 터진다:

- slotted page 의 slot 재사용 × **동시** 가변길이 record 갱신
- overflow record 승격/강등 경계에서 두 트랜잭션이 같은 페이지를 잡는 순서
- B-tree 의 unique violation 롤백과 다른 스레드의 재삽입이 겹치는 창
- MVCC 가시성 — update 체인과 **vacuum 워커** 의 간섭
- latch 획득 순서, 그리고 그 창이 마이크로초인 race
- heap best-space 캐시가 실제 free space 와 어긋나는 상태

이런 상태는 SQL 층에서 *우연히* 도달할 수는 있어도 *체계적으로* 탐색되지 않는다.
그리고 **단일 스레드로는 도달하지 않는다** — 그것이 §1.0 의 요지다.

### 1.2 잡는 버그 종류

내부 assert 위반 / page corruption / slot 인덱스 out-of-range / OID dangling /
overflow chain 누수 / MVCC 가시성 판정 오류 / heap best-space 불일치 /
ASan heap-buffer-overflow · use-after-free / UBSan misaligned access.

---

## 2. protobuf 에 대한 오해 해소 (선결 확인 사항)

> **CUBRID 는 자체 바이너리 프로토콜을 쓰고 protobuf 를 쓰지 않는다.
> 그런데 왜 libprotobuf-mutator 인가? — 프로토콜과 무관하기 때문이다.**

여기서 protobuf 는 **fuzzer 내부의 입력 기술 언어(IR)** 일 뿐, *wire format 이 아니다*.
CUBRID 는 protobuf 바이트를 **한 번도 보지 않는다.**

```
libFuzzer
   │  (raw bytes)
   ▼
libprotobuf-mutator          ← protobuf 는 여기까지만 존재
   │  (structure-aware mutation: field 교체 / oneof 전환 / repeated 삽입·삭제)
   ▼
StorageOpSequence  (in-memory C++ 객체)
   │  harness 가 직접 번역
   ▼
heap_insert_logical() / btree_insert() / heap_get_visible_version() / ...
   ↑
   CUBRID 내부 C/C++ API 직접 호출 — 네트워크·프로토콜·직렬화 전부 경유하지 않음
```

RocksDB 의 `fuzz/db_fuzzer.cc` 도 정확히 이 형태다 — `DBOperation` protobuf 메시지를
받아 `db->Put()` / `db->Get()` / `db->Delete()` 를 *직접* 호출한다. RocksDB 역시
protobuf 를 저장 포맷이나 프로토콜로 쓰지 않는다.

**따라서 호환성 문제는 발생하지 않는다:**

| 우려 | 실제 |
|---|---|
| CUBRID 프로토콜을 protobuf 로 바꿔야 하나? | 아니오. 프로토콜은 손대지 않는다 |
| 서버/클라이언트에 protobuf 의존이 생기나? | 아니오. `cubrid-fuzz-storage` **fuzz 바이너리에만** 링크된다 |
| 3rdparty 에 protobuf 를 vendor in 해야 하나? | fuzz 빌드(`-DENABLE_FUZZING=ON`) 경로에서만. 기본 빌드 산출물은 불변 |
| 배포 산출물이 커지나? | 아니오. fuzz 타깃은 배포물이 아니다 |

*(2026-09-03 확인: cubrid 본 repo 에 protobuf 의존은 **존재하지 않는다** — 소스 트리 전역
검색 무결과, `CMakeLists.txt` / `cmake/` / `3rdparty/` 에도 언급 없음. 현행 3rdparty 는
libedit · libexpat · libjansson · libodbc · libopenssl · libtbb · lz4 · rapidjson · re2 뿐이다.
따라서 protobuf 는 **신규 fuzz-only 의존** 이며, 본 repo 3rdparty 정책과의 충돌 여부가
ADR-EXT-009 의 결정 항목이다.)*

### 2.1 protobuf 를 쓰지 않는 대안 (ADR-EXT-009 에서 비교)

| 방식 | 신규 의존 | 구조 인식 mutation | crossover 품질 | 비고 |
|---|---|---|---|---|
| **libprotobuf-mutator** | protobuf + LPM (fuzz-only) | ★★★★ | ★★★★ | RocksDB 검증된 경로. corpus 가 사람이 읽을 수 있음(TextFormat) |
| **FuzzedDataProvider** (libFuzzer 헤더 단독) | 없음 | ★★ | ★★ | 의존 0. byte→op 디코더를 손으로 씀. mutation 이 구조를 깨기 쉬움 |
| 자체 IR + 자체 mutator | 없음 | ★★★ | ★★★ | 통제력 최대, 구현·유지 비용 최대 |

**권고(미확정):** 1차는 `FuzzedDataProvider` 로 harness 골격과 *state reset* 을 먼저
검증하고, operation 어휘가 안정된 뒤 libprotobuf-mutator 로 승격. 이유는 §7 위험 1.

---

## 3. 외부 호출 형태 (제안 — incubating)

```
testkit run fuzz --target storage [--ops <n>] [--time <sec>] [--corpus <dir>]
```

내부 진입 (의제):

```
StorageFuzzDriver.exec(config)          # testkit (Go) — 오케스트레이션만
  ├─ cubrid-fuzz-storage 를 subprocess 로 구동 (ADR-001 Consequence 4)
  │
  │   ┌ cubrid-fuzz-storage (본 repo 바이너리, libFuzzer in-process) ─────────┐
  │   │ 1회: 임시 volume 생성 + boot (in-process)                             │
  │   │ 매 입력마다:                                                          │
  │   │    reset()                       ← §5 state reset 계약                │
  │   │    for op in sequence:                                                │
  │   │       translate(op) -> heap_* / btree_* / log_* 직접 호출              │
  │   │       check invariants (선택)                                         │
  │   │    rollback or drop                                                   │
  │   └───────────────────────────────────────────────────────────────────────┘
  │
  └─ 산출 아티팩트 ingest: crash 입력 → op sequence 텍스트 덤프 + stack hash dedup
```

**testkit 책임 한정:** corpus 보관 + replay + crash triage + 실행 오케스트레이션.
**cubrid 본 repo 책임:** fuzz target build option, in-process boot/shutdown 진입점,
`LLVMFuzzerTestOneInput` 구현, state reset 훅.

**외부 표면 동결(NG2) 영향:** 없음 (신규 진입점).

---

## 4. operation 어휘 (1차 의제 — 실제 API 근거)

> **재정의 2 (2026-09-04).** 아래 어휘표는 *연산을 합성한다* 는 전제로 쓰였다. 그 전제가
> 철회됐다 — **호출 규약 문제** 때문이다. `heap_insert_logical` 등은 호출자가 세워 준 전제
> (열린 트랜잭션·격리, 잡힌 락과 latch, `heap_create_insert_context ()` 로 채운
> `HEAP_OPERATION_CONTEXT`, 올바르게 중첩된 `log_sysop_start`/`end`) 위에서 돈다. 합성하면
> **실제 호출자가 만들지 않는 순서** 를 만들고, 거기서 나온 crash 는 결함이 아니라 전제
> 위반에 대한 정당한 반응일 수 있다. 발견마다 도달 가능성 triage 가 붙는다.
>
> 그래서 **연산은 합성하지 않고 미리 컴파일된 XASL 을 재생** 한다 (§4a). 질의 실행이 전제를
> 다 세우므로 모든 경로가 구성상 도달 가능하고, 퍼저는 **스케줄만** 탐색한다. 아래 표는
> Q16(b) *future work* — 내부 API 어휘 모델링 — 의 출발점으로 남긴다.

CUBRID 소스 확인 기준 (`src/storage/`, `src/base/`):

| op | 대응 내부 API | 비고 |
|---|---|---|
| `INSERT` | `heap_insert_logical` (`storage/heap_file.h`) | `HEAP_OPERATION_CONTEXT` 구성 필요 |
| `UPDATE` | `heap_update_logical` | in-place / 이동 / overflow 승격 경로 분기 |
| `DELETE` | `heap_delete_logical` | |
| `GET` | `heap_get_visible_version` | MVCC 스냅샷 인자 |
| `SCAN` | `heap_scancache_start` → `heap_next` → `heap_scancache_end` | |
| `IDX_INSERT` | `btree_insert` (`storage/btree.h`) | unique / non-unique |
| `IDX_SCAN` | `btree_range_scan` | |
| `COMMIT` / `ABORT` | transaction 경계 | §5 reset 과 직접 결합 |
| `VACUUM` | vacuum 요청 | MVCC 간섭 탐색의 핵심 |
| `CHECKPOINT` | log checkpoint | 순위 6 (recovery) 과의 접점 |

`record serialize/unpack` (`or_get_value` / `or_put_value` / `or_unpack_value`,
`src/base/object_representation.h`) 은 **stateless byte-in 타깃** 이므로 본 항목이
아니라 **E5 의 target layer** 로 귀속한다 (§6a 사다리 순위 4).

**미정:** 어휘의 1차 범위 (heap 만 / heap+btree / +vacuum / +checkpoint),
key/value 도메인 (고정 스키마 vs 가변 도메인), scan 결과의 검증 여부.

---

## 4a. 무엇이 연산을 만드는가 — XASL 재생 (2026-09-04)

**서버는 SQL 을 컴파일하지 않는다.** `libcubrid.so`(SERVER_MODE) 에 `parser_main`,
`pt_compile`, `do_prepare_select`, `xts_map_xasl_to_stream` 이 **없다** — 컴파일과 직렬화는
클라이언트 몫이다. `xqmgr_prepare_query (thrd, compile_context *, xasl_stream *)` 는 이미
만들어진 스트림을 캐시에 등록하거나 존재를 확인할 뿐 컴파일하지 않는다.

역직렬화(`stx_map_stream_to_xasl`)는 서버에 있다. 즉 **소비 절반은 있고 생산·보관 절반이
없다.** 하네스는 미리 컴파일된 XASL 픽스처를 받아 캐시에 올리고 `XASL_ID` 로 반복 실행한다.

```
질의 생성 (E3 SQLancer / 손으로 고른 것)
      ↓ 클라이언트 측 컴파일 + 직렬화
  XASL 픽스처   ← §6a-E10 이 담당 (신설). 엔진에 없는 부분
      ↓ stx_map_stream_to_xasl (엔진에 있음)
  E9 재생 — 스케줄만 탐색
```

**E3 와 겹치지 않는다.** E3 는 *질의 모양* 을 탐색하고 매번 새로 생성한다. E9 는 질의
생성기가 필요 없고 **경합을 만들도록 손으로 고른 소수의 플랜** 을 코퍼스로 쓴다. 탐색 축이
다르고(질의 vs 인터리빙) 오라클도 다르다(wrong-result vs crash·손상).

**픽스처 스키마** — 클라이언트가 실행 요청에 싣는 것이 정본이다
(`sqmgr_execute_query ()` unpack): `sql_user_text`(hash text 아님 — 그건 재작성된 해시 키),
XASL 스트림, host variable 묶음 + `data_size`, `query_flag`, `query_timeout`, 그리고
**엔진 빌드 식별자**. 마지막 항목이 필수인 이유는 §4b.

## 4b. 엔진은 스트림 버전을 검사하지 않는다

`stx_map_stream_to_xasl ()` 이 확인하는 것은 포인터 non-null 과 `xasl_stream_size > 0`
뿐이다. 곧바로 `or_unpack_int` 로 헤더 크기를 읽고 오프셋을 계산한다. **포맷·버전 검사가
없다.** 다른 빌드의 스트림은 거부되지 않고, 의미가 달라진 오프셋으로 역직렬화된다 —
조용히 이상하게 동작한다. 따라서 빌드 식별자 기록과 불일치 시 거부는 선택이 아니며,
**E10 이 구현해야 할 몫** 이다.

---

## 5. 재현성 — 상태를 되돌리는 문제가 아니라 스케줄을 재생하는 문제

> **재정의 (2026-09-03).** 이 절은 처음에 "state reset — 본 항목의 핵심 설계 난제" 였다.
> 단일 스레드 전제에서 나온 틀이고, 재정의로 문제 자체가 바뀌었다.

libFuzzer 는 **한 프로세스 안에서 입력을 수만 번 반복** 한다. 단일 스레드였다면 필요한
명제는 "같은 입력 → 같은 상태" 였다. **멀티스레드에서 그것은 얻을 수도 없고 원할 것도
아니다** — 스케줄이 비결정적인 것이 바로 탐색하려는 대상이기 때문이다.

필요한 것은 다른 형태다:

- **스케줄을 입력의 일부로 만든다.** `(연산, 스케줄)` 쌍이 재현되면 충분하고, 상태가
  비트 단위로 같을 필요는 없다.
- **reset 은 게이트가 아니라 준비 단계** 로 내려간다 — 알려진 시작 DB 를 만드는 일.
- **crash triage 의 단위가 바뀐다** — 저장할 것은 입력 바이트만이 아니라 그 입력을 재현시킨
  스케줄이다.

### 5.1 스케줄 제어 지점은 이미 엔진에 있다

`src/base/fault_injection.c` 의 `fi_handler_hold` / `fi_handler_hang` 이 지정 지점에서 창을
넓히거나 멈춘다. CBRD-27198 (2026-09-02 머지) 이 `disk_reserve_sectors_in_volume` 에
`fi_handler_hold` 훅을 넣은 이유가 정확히 이것이다 — *"The race lasts microseconds … a test
can only hit that window by luck."* **즉 스케줄 주입 원시 도구가 존재하고 이미 그 용도로
쓰이고 있다.** 본 항목은 그 위에 생성기와 불변식 검사를 얹는 일이다.

한계도 같이 기록한다: FI 지점은 **정적 enum** 이라 스케줄 공간의 커버리지가 훅이 박힌
자리에 제한된다. 지점을 늘리면 그때 본 repo 작업이 생기고, CBRD-27198 이 그 관례(NDEBUG
게이트 + 모듈당 예약 범위)를 이미 보여준다.

### 5.2 SA 모드 스파이크 (2026-09-03) — 게이트가 아니라 준비 단계에 대한 자료

스파이크: cubrid `feat/fuzz-target-infrastructure` 브랜치의 `fuzz/spike/reset_spike.cpp`.
SA 모드로 엔진을 in-process 부팅(`db_login` / `db_restart` — `compactdb`·`checksumdb` 가
이미 쓰는 경로)하고, 연산 열을 돌리고, 리셋하고, 그 실행이 관측한 모든 것을 다이제스트로
만든다. 다이제스트가 1종이면 결정적이다.

**같은 열만 반복하는 것은 약한 검증이라 쓰지 않았다.** 퍼저는 매 입력이 직전과 다르므로,
성립해야 하는 명제는 *앞에 무엇이 오든 같은 열은 같은 다이제스트를 낸다* 이다. 그래서
variant 0 사이에 다른 잔여 상태를 남기는 열 둘을 끼웠다 — in-place 로 갱신되지 않는 넓은
행, 롤백된 unique violation.

| 전략 | 실행 | 결정성 | iter/sec | median | 비고 |
|---|---:|---|---:|---:|---|
| **A. abort + 테이블 비우기** | **10,000** | **결정적** (variant-0 5,000회, 다이제스트 1종) | **126.2** | **7.6 ms** | 공개 API 만 사용. 가장 싸고 가장 빠르다 |
| A. (같은 열 반복) | 10,000 | 결정적 | 72.0 | 12.7 ms | 약한 검증. 참고용 |
| B. volume 재생성 (`DROP`/`CREATE`) | 500 | 결정적 | 41.2 | 23.6 ms | 예상대로 A 보다 느리다 |
| B'. 전체 `db_shutdown` + `db_restart` | 50 | 결정적 | 2.4 | 321.8 ms | A 의 1/50 |
| C. fork() 격리 | — | 미측정 | — | — | 결정성 확보용으로는 불필요해짐. §10 참조 |
| ~~D. 전용 reset 훅~~ | — | — | — | — | **철회.** A 가 이미 결정적이고 더 빠르다. 본 repo 작업량이 0 이 된다 |

**이 측정이 말해주는 것은 여기까지다: 트랜잭션 경계로 되돌리는 준비 단계가 성립하고 싸다.**
진입 조건으로 삼았던 "결정적 재현" 게이트는 **이 측정으로 충족되지 않는다** — SA 는 단일
스레드이고 본 항목의 대상 구성이 아니다 (§1.0). 대상 구성에서의 재현성은 §5 의 형태,
즉 스케줄 재생으로 얻어야 하며 아직 측정된 바 없다.

**측정이 덮지 않는 것:**
- **단일 스레드(SA)에서 쟀다.** 본 항목은 SERVER_MODE 멀티스레드가 대상이다. 이것이 가장 큰 간극이다.
- 리셋을 **SQL 레벨** 에서 쟀다. 본 항목은 `heap_insert_logical` 등 내부 API 를 직접 친다.
  리셋이 트랜잭션 단위라 이어질 것으로 보지만 그건 *추론* 이다.
- 초당 126 회는 libFuzzer 가 기대하는 수천 회에 못 미친다. 대부분은 리셋이 아니라 SQL 6 문장이
  전체 스택을 도는 비용인데, 이 측정은 둘을 분리하지 않았다.
- 연산 열 3 종은 임의의 연산 열이 아니다.

**그리고 하네스 형태가 확정됐다** — 이쪽은 E5 의 무상태 타깃과 달리 **자기 완결적일 수 없다.**
`db_restart` 가 설치된 `$CUBRID` 트리와 디스크 위의 DB 를 요구한다. 되돌릴 대상이 구조체가
아니라 파일과 부팅된 엔진이다.

### 5.3 SERVER_MODE in-process 기동 — 된다 (2026-09-03)

`fuzz/spike/server_boot_spike.cpp`. `net_server_start()` 에서 네트워크 절반을 뺀 순서 그대로다
— 상류에서도 `boot_restart_server()` 가 `css_init()` **앞** 에 온다. 뺀 것은 둘뿐:
`net_server_init()` (static, in-process 가 안 쓰는 요청 디스패치 테이블만 채움) 과
`css_init()` (소켓 개방).

```
boot_restart_server                            rc=0
BOOTED -- SERVER_MODE engine is up in-process with no listener.
live threads while booted                      15
xboot_shutdown_server                          ok=1
```

**스레드 15 개가 요점이다.** `rc=0` 인데 스레드가 하나였다면 이름만 다른 SA 형태였을 것이다.
데몬이 올라와 있고, 그것이 이 라이브러리를 고른 이유다.

### 5.3a 순서 공간은 이미 포화되어 있다 (2026-09-04) — 이 항목의 전제를 바꾼 측정

`noise_floor_spike.cpp`. 입력을 **고정** 하고 반복하며, 각 참가자가 연산 직전에 공유
카운터에서 티켓을 뽑는다. 티켓 순서가 관측된 인터리빙이고, 서로 다른 순서의 개수를 센다.

| 모드 | 스레드 | 반복 | distinct 순서 | 최빈 |
|---|---:|---:|---|---|
| 대조군 (티켓만, 엔진 작업 없음) | 4 | 2000 | **24 / 24 — 100%** | 5.8 · 5.3 · 4.9 · 4.8 · 4.7 % |
| `file_create_heap` | 4 | 1000 | **24 / 24 — 100%** | 5.2 · 5.1 · 5.1 · 4.9 · 4.9 % |

균등이면 4.17% 이고 둘 다 그 근처다. 엔진 작업 쪽이 오히려 *더* 균등하다 — **"엔진 latch 가
순서를 좁힌다" 는 가설은 틀렸다.**

**함의**: 스케줄 통제로 얻는 것은 탐색 커버리지가 아니다 — 반복만으로 공짜다. 얻는 것은
**재현** 이다. 따라서 **Tier 1 이 주력** 이고(시간이 곧 발견), **Tier 2 는 triage 도구** 로서
Tier 1 이 재현 안 되는 결함을 낸 *뒤* 에 착수하며, **libFuzzer 가 스케줄을 탐색한다는 구상은
폐기** 한다 — 통제 지점이 없으면 입력의 스케줄 인코딩과 실제 인터리빙 사이에 상관이 없다.

*범위 한정*: 작업 한 종류, 참가자 4 명, 관측 지점은 엔진 내부 이벤트가 아니라 티켓이다.
단일 hot page 에 실제 경합이 걸리는 작업은 더 제약될 수 있다.

### 5.3b 오라클을 넓혔다 (2026-09-04) — §5.3a 가 지목한 다음 개선

§5.3a 는 "다음 개선은 스케줄이 아니라 오라클" 로 끝났다. 그 작업의 결과다. 구현체는
`cubrid` 본 repo `feat/fuzz-target-infrastructure`, 설계 근거는 roadmap
`N66/10-design_fi-rendezvous.md` §9.2·§9.3.

**정합성 검사 — 어디에 둘 수 있는지는 비용이 정한다.**

| 검사 | 비용 | 실행 위치 |
|---|---:|---|
| `disk_check ()` | 0.000 s | 매 입력 |
| `file_tracker_check ()` | 0.007 s | 매 입력 |
| `xboot_check_db_consistency (CHECKDB_ALL_CHECK_EXCEPT_PREV_LINK)` | **7.0 s** | 세션 시작·끝 |

초당 수십 회가 목표이므로 전체 검사 1 회는 입력 수백 개어치다. "N 회마다" 가 아니라
**세션 경계에만** 둔다. 검사는 **워크로드 전후 모두** 돌린다 — 사후에만 돌리면 이 입력이
만든 결함과 DB 가 원래 갖고 있던 것을 구분할 수 없다.

**TSan 빌드는 플래그 하나다** — `-DFUZZ_SANITIZERS=thread`. ASan 과 배타적이라 엔진을 따로
빌드해야 하지만 소스 트리는 하나고, 스파이크는 대상 빌드의 `flags.make` 에서 컴파일 플래그를
되읽어 만든다. **ASan/UBSan 과 TSan 은 택일이 아니라 같은 하네스의 두 실행이다.**

**베이스라인이 없으면 새니타이저는 오라클이 아니다.** 깨끗한 4 스레드 실행이 TSan 222 건,
UBSan 10 건을 매번 낸다. `sanitizer_triage.py` 가 종류와 지점으로 묶고(TSan 은 최상위 *엔진*
프레임 기준), `--suppress` 로 억제 파일을 뽑는다.

| 파일 | 규칙 | 베이스라인 적용 시 |
|---|---:|---|
| `tsan-baseline.supp` | 42 | 222 → **0** |
| `ubsan-baseline.supp` | 3 | 10 → **0** |

**베이스라인은 판정이 아니다.** 어느 항목도 적부를 판정하지 않았다 — page buffer 와 log
append 는 손으로 짠 atomic 을 쓰므로 TSan 이 믿을 이유가 없다. 유일한 목적은 **침묵이 의미를
갖게 하는 것** 이고, 실제로 그 값을 했다: 워크로드를 넓혀 `xheap_destroy` 를 넣자
`vacuum_add_dropped_file ()` 에 닿았고 TSan 이 거기서 새 race 를 40 회 보고했다. 기존 179 건
사이였다면 안 보였을 것이 침묵 위에서는 화면에 그것 하나였다. ASan 은 **의도적으로 베이스라인
대상에서 제외** 한다 — ASan 리포트는 실행을 멈추는 메모리 오류이므로 억제하면 결함을 감춘다.

**함정 두 가지 (둘 다 조용히 실패한다).**
- `DEBUGINFOD_URLS=` 를 비워야 한다. 안 그러면 첫 리포트에서 프로세스가 **CPU 시간 0 으로**
  멈춘다 — `llvm-symbolizer` 가 디버그 정보를 HTTP 로 받으러 가서 블록되고, TSan 이 심볼라이즈
  동안 trace-part 세마포어를 쥐고 있어 나머지 스레드가 전부 뒤에 쌓인다. 엔진 데드락과
  구분되지 않는다.
- UBSan 억제는 **행 단위가 없다.** 규칙이 `<check>:<file>` 이므로 한 규칙이 그 파일 전체를
  덮는다. 조용히 넓어지는 베이스라인은 없느니만 못하므로 정밀한 도구는 컴파일 타임
  `-fsanitize-ignorelist` 라고 명시해 둔다.

**하네스 스레드는 `TT_WORKER` 를 자칭하려면 실제로 그것이어야 한다.** 엔진은 타입을 약속으로
되읽는다 — `log_tran_table.c:2896` 이 주석으로 "Only TT_WORKER threads use pl_session" 라고
적어 두었고, 연결 엔트리 없는 `TT_WORKER` 에서는 **모든 `log_sysop_start ()` 가 부수효과로
`ER_SES_SESSION_EXPIRED` 를 설정** 한다. 대부분 경로는 무시하지만 `heap_insert_logical ()` 은
반환하므로 모든 삽입이 실패한다. 해결은 다른 타입을 고르는 것이 아니라 **약속을 지키는 것**
이다: `CSS_CONN_ENTRY` 배열은 `boot_restart_server` 안에서 할당되고
`css_initialize_conn ()` 은 소켓을 건드리지 않으므로 `css_make_conn (INVALID_SOCKET)` 이
연결 없는 진짜 연결 엔트리를 준다. 이것은 **in-process 하네스 일반에 적용되는 제약** 이라
여기에 적는다.

**돌리는 방법 — `soak.sh`.** §5.3a 가 "통제할 것이 없다" 로 끝났으므로 Tier 1 의 탐색은
곧 시간이고, 남는 질문은 그 시간을 어떻게 쓰느냐뿐이다. **세션 단위** 로 쓴다 — 부팅 ·
before 검사 · 워크로드 · after 검사 · 셧다운. 한 번의 긴 실행이 아닌 이유 둘 다 위 표에서
나온다: 전체 검사는 7 s 라 경계에만 둘 수 있고, 4 스레드가 변경하는 *중* 의 검사는 틀린
상태가 아니라 **찢어진** 상태를 보고한다. 세션 반복은 부팅·셧다운 자체도 매번 재시험한다는
부수 효과가 있다. 멈추는 조건은 비정상 종료 · 오라클 실패 · **베이스라인이 덮지 않는**
새니타이저 리포트 셋뿐이고, 나머지는 침묵이다.

### 5.4 그런데 FI 로는 스케줄을 *재생* 할 수 없다 (2026-09-03)

§5.1 이 "스케줄 주입 원시 도구가 이미 있다"고 적었다. 절반만 맞다. API 를 읽고 정정한다.

**있는 것:**
- **스레드별 무장.** FI 상태는 `thread_p->fi_test_array` 에 있다 (`fi_thread_init`,
  SERVER_MODE 한정). `fi_set (thread_p, code, state)` 는 *특정 스레드* 에만 훅을 건다.
  "A 스레드만 이 지점에서 멈춰라" 가 표현된다
- **이름 붙은 주입 지점** — FI_TEST_CODE enum

**없는 것 — 그리고 이것이 게이트를 막는다:**

| 핸들러 | 구현 | 스케줄 원시 도구로서 |
|---|---|---|
| `fi_handler_hold` | `sleep (seconds)` | **시간 기반**. 창을 넓힐 뿐 순서를 정하지 않는다 |
| `fi_handler_hang` | `while (true) sleep (1);` | **영구 정지. 해제 경로가 없다** |

둘 다 sleep 중에 상태를 다시 보지 않으므로, 다른 스레드가 `fi_set` 으로 깨울 수 없다.
즉 **"A 는 B 가 지점 Y 에 도달할 때까지 여기서 기다린다" 를 표현할 수 없다.**

**결론 — 두 단계로 갈린다:**

- **Tier 1 (오늘 가능).** 창 넓히기 + 불변식 검사. race 를 *확률적으로* 노출시킨다.
  CBRD-27198 이 하는 일이 정확히 이것이고 실제로 유용하다. 다만 재생이 안 되므로 triage 는
  FI 설정과 seed 를 기록해 두는 수준에 머문다
- **Tier 2 (엔진에 핸들러 하나 추가하면).** 조건 변수에서 대기하고 하네스가 깨우는
  rendezvous 핸들러 — `fi_handler_wait` 같은 것. 그것이 생기면 인터리빙을 결정적으로 재생할
  수 있다. **작업량은 기존 네 핸들러 옆에 하나를 더하는 정도이고, CBRD-27198 이 이미 그
  관례(핸들러 + 훅 추가)를 보여준다**

따라서 이 항목이 엔진에 요구하는 것은 reset 훅도, 스케줄 지점 확대도 아닌 **rendezvous
핸들러 하나** 다. 그것이 ADR-EXT-009 의 핵심 결정 항목이 된다.

---

## 6. 사용자 요구사항 (incubating 추정)

1. **operation sequence 생성** — 구조적으로 유효한 연산 열을 mutate
2. **결정적 재현** — 같은 입력 → 같은 crash. reset 계약이 이를 보장
3. **사람이 읽는 reproducer** — crash 입력을 `INSERT/UPDATE/SCAN/COMMIT` 텍스트로 덤프
4. **invariant 훅** — crash 없이도 위반을 잡을 수 있는 검사점 (page 무결성 / OID 유효성)
5. **crash dedup** — stack hash 기반
6. **regression seed 누적** — 과거 crash 열을 새 빌드에서 재실행
7. **time / iteration budget** — CI 시간 통제
8. **E5 와의 인프라 공유** — corpus 보관·triage·coverage 보고는 같은 것을 쓴다

---

## 7. 비기능 요구

| 항목 | 의제 | 새 시스템에서의 의미 |
|------|------|---------------------|
| 도입 비용 | **높음** | E5 (중) 보다 높다 — state reset 설계가 추가됨 |
| 즉시 ROI | ★★★ | 도달 시 가치 큼. 단 선행 비용이 커서 사다리 후순위 |
| 선결 의존 | E5 의 `-DENABLE_FUZZING` 인프라 + in-process boot 진입점 | testkit 단독 결정 불가 |
| 책임 경계 | testkit = corpus + replay + triage / cubrid = target + reset 훅 | **C-055** — 엔진 쪽은 **N66-fuzz-target-infrastructure** |
| 라이선스 | libFuzzer Apache 2.0 · protobuf BSD-3 · libprotobuf-mutator Apache 2.0 | 자유. fuzz-only 링크 |
| 처리량 | 입력당 reset 비용에 지배됨 | §5 전략 선택이 곧 처리량 결정 |
| E3 (SQLancer) 와의 관계 | 상보 — E3 는 SQL 층 wrong-result, 본 항목은 내부 API 상태 | 중복 아님 |

---

## 8. 의존하는 외부 자원

- **cubrid 본 repo fuzz 인프라** — `-DENABLE_FUZZING` (E5 와 공유) + storage fuzz target + reset 훅. *선결*
- **sanitizer 빌드** — ASan / UBSan (본 repo 책임). MSan 은 서버 전체 재빌드 필요성 검토
- **libFuzzer** — in-process coverage-guided fuzzer
- **protobuf + libprotobuf-mutator** — 구조 인식 mutation (§2.1 에서 대안과 비교 후 결정)
- **seed corpus** — 의미 있는 operation 열 (§ test-corpus.md)
- **crash corpus storage** — NG1 점검 (testcases 레포 밖)

---

## 9. incubating 진입 조건 (조건부)

다음이 *충족된 후* 정식 incubating 진입 (owner: hgryoo):

1. **E5 선행** — `-DENABLE_FUZZING` 과 crash triage 인프라가 먼저 서야 한다. 본 항목은 그 위에 얹힌다.
   **부분 충족 (2026-09-03)**: `-DENABLE_FUZZING` 과 첫 타깃(사다리 순위 4, `or_get_value ()`)이
   cubrid `feat/fuzz-target-infrastructure` 에 존재하고 동작한다. corpus·replay·triage 는 아직 없다
2. ~~**SERVER_MODE in-process 기동**~~ → **충족 (2026-09-03, §5.3).** 리스너 없이 기동하고
   데몬 15 스레드가 뜨며 깨끗이 내려간다. ~~SA 모드 `db_restart`~~ 는 대상이 아니다 (§1.0)
3. **스케줄 재생 — 현행 FI 로는 불가 (2026-09-03 확인, §5.4).** 결정적 재생에는 rendezvous
   원시 도구가 필요한데 FI 에 없다. **엔진에 핸들러 하나를 추가하면 해소된다** — 그 전까지는
   Tier 1(창 넓히기, 확률적 노출)까지만 가능하다. 전략 D(전용 reset 훅)는 철회된 채로 둔다 —
   문제가 reset 이 아니게 됐다
4. **XASL 픽스처 설비 (§6a-E10) 선결** — 서버가 SQL 을 컴파일하지 않으므로 (§4a) 이것
   없이는 Tier 2 를 착수할 수 없다. **Tier 1 은 이 조건과 무관하게 진행 가능** 하며 실제로
   진행 중이다 — 다만 Tier 1 은 스토리지 내부 API 를 직접 부르므로 §4 의 호출 규약 문제를
   그대로 안고 있고, 그 발견에는 도달 가능성 triage 가 붙는다.
   **2026-09-04 에 그 비용이 측정됐다**: 워크로드를 heap 의 일생(생성 → 페이지를 걸치는
   레코드 8 개 → drop 또는 rollback)으로 넓히는 데 호출 규약 위반 3 건을 거쳤고, **셋 다
   엔진이 아니라 하네스 결함** 이었다 — `file_create_heap ()` 직접 호출(→ `xheap_create ()`),
   NULL class OID(→ root class), 그리고 세션 없는 `TT_WORKER`(§5.3b). 매번 오라클이 잡았고
   매번 "엔진인가 나인가" 를 소스로 확인해야 했다. 상세: roadmap N66 §5
5. **참가자 수 상한 확정** — 스레드 수는 입력에 포함하되 엔진이 강제하는 상한으로
   **clamp**(거부 아님). 상한은 `m_max_threads` 중 이 구성에서 노는 connection 몫이며,
   리스너가 없으므로 여유다. 설정 변경은 여전히 불필요하되 **2026-09-04 정정**: 참가자마다
   진짜 `CSS_CONN_ENTRY` 를 하나씩 쓰므로(§5.3b) `max_clients` 는 무관한 값이 아니라
   **참가자 상한** 이다 — `css_make_conn ()` 이 그 위에서 `NULL` 을 반환한다
6. **입력 IR 결정** — 연산이 아니라 **스케줄과 참가자 수** 를 기술한다. libprotobuf-mutator vs
   FuzzedDataProvider (§2.1)
7. **corpus 위치** — NG1 점검. 스케줄이 입력에 포함되므로 저장 단위가 커진다
8. **스케줄 제어 지점 범위** — 기존 FI enum 만 쓸지, 지점을 늘릴지 (§5.4)
9. **C-055** — roadmap cross-cutting 에 등록됨 (2026-09-03). 엔진 쪽 작업은
   **N66-fuzz-target-infrastructure**. 그 §9 Q1 (*testing 전용 reset 훅*) 은 **무의미해졌다** —
   측정 결과 훅 없이도 게이트를 통과하므로, 본 항목의 도달 범위를 그 답이 결정하지 않는다

**ADR placeholder:**
- **ADR-EXT-009** — 입력 IR (protobuf/LPM vs FDP) + state reset 전략 + operation 어휘 1차 범위 + corpus 위치 + 본 repo 책임 경계

> **번호 주의.** `E8` / `ADR-EXT-008` 은 축 8 *Hybrid CI 통합* 자리로 이미 예약되어 있다
> (`extensions/README.md` 카탈로그). 본 항목이 `E9` 인 이유가 이것이다 — 순위 사다리의
> 순번(5)과 카탈로그 ID(9)는 **별개 번호 공간**이다.

---

## 10. 위험 / 정합성 메모

1. **스케줄 재생이 성립하지 않을 수 있다 (최대 위험).** `(연산, 스케줄)` 쌍을 재현하려면
   FI 훅이 박힌 지점만으로 인터리빙을 충분히 결정할 수 있어야 한다. 훅 사이 구간은 여전히
   OS 스케줄러가 정하므로, 재현이 확률적으로만 될 가능성이 있다. 그러면 crash triage 가
   무의미해진다 — 진입 조건 3 이 게이트인 이유.
1b. **2026-09-03 정정 기록.** 이 자리에는 "state reset 이 안 될 수 있다"가 있었고, SA 스파이크
   결과로 "해소"라고 적었다. **범위를 넘은 주장이었다** — SA 는 단일 스레드라 대상 구성이
   아니다. 그 측정이 남기는 것은 준비 단계가 싸다는 사실뿐이다 (§5.2).
2. **protobuf 신규 의존에 대한 본 repo 반발.** fuzz-only 링크라도 3rdparty 정책상
   거부될 수 있다 → §2.1 의 FuzzedDataProvider 경로가 fallback.
3. **E5 를 건너뛰고 본 항목부터 시도.** 인프라 중복 구축이 된다 → 사다리 순서 준수.
4. **NG1 충돌.** crash corpus 를 testcases 레포에 두면 동결 위반 → 외부 storage.
5. **NG2 / NG4 충돌 없음.**
5b. **fault injection 을 연산 열에 합치면 `fork()` 격리가 필수가 된다.** 엔진의 FI 핸들러
   (`fi_handler_exit` / `fi_handler_hang`, `src/base/fault_injection.c`) 는 프로세스를 끝낸다.
   libFuzzer 의 in-process 모델과 양립하지 않으므로, crash 지점 × 연산 열을 함께 탐색하려면
   전략 C 가 차선이 아니라 전제 조건이다. §5.1 이 결정성 목적의 C 를 불필요하게 만든 것과는
   별개 사안이다.
6. **축 7 (E7 stateful workload) 과의 경계 혼동.** E7 은 *SQL/노드 레벨* long-running
   시나리오, 본 항목은 *내부 API 레벨* 단일 프로세스. C-004 경계 정의에 함께 기재.
7. **분기 게이트 §7** — strangler-fig 우선원칙. Phase 3·4 와 자원 충돌 시 후순위.

---

## 11. 참조

- RocksDB fuzzing: <https://github.com/facebook/rocksdb/tree/main/fuzz>
- libprotobuf-mutator: <https://github.com/google/libprotobuf-mutator>
- libFuzzer: <https://llvm.org/docs/LibFuzzer.html>
- libFuzzer `FuzzedDataProvider`: <https://llvm.org/docs/LibFuzzer.html#fuzzer-friendly-build-mode>
- OSS-Fuzz: <https://google.github.io/oss-fuzz/>
- 내부: `../E5-parser-fuzzing/requirements.md` · `../../ROADMAP.md` §6a 사다리 · `../../survey/dbms-testing-ecosystem.md` §7.4
