# E2 — I/O Contract (STUB)

*[English](io-contract.md) · 한국어*

**Status:** STUB — contract 동결은 ADR-EXT-002 incubating 정식 진입 후.
**Source:** `requirements.md` §2

---

## 1. CLI (제안)

```
ctp.sh sqlsmith [-c <sqlsmith.conf>]
   또는
testkit run sqlsmith [-c <conf>] [--seed <N>] [--time <sec>] [--max-depth <D>] [--client jdbc|cci]
```

`--continue` 모드: 과거 corpus 의 crash query 를 새 빌드에서 replay.

## 2. conf 스키마 (TBD)

| 키 (의제) | 값 | 출처 |
|---|---|---|
| `seed` | `<int>` | 재현용 |
| `time_budget_sec` | `<int>` | CI 시간 통제 |
| `max_depth` | `<int>` | nested AST 최대 깊이 |
| `client` | `jdbc \| cci` | SUT 구동 채널 |
| `dialect_extensions` | csv `path,serial,connect_by,method` | CUBRID 확장 가산 토글 |
| `corpus_root` | `<dir>` | crash corpus 위치 |
| `crash_channel` | `signal \| core \| server_log \| all` | crash 판정 채널 |

ADR-EXT-002 후 키/의미 동결.

## 3. 출력 포맷 (TBD)

```
<resultDir>/
   ├── main.info              # 호환 (sql 모듈 grep 패턴 차용 검토)
   ├── crash/
   │     └── <stack-hash>/
   │           ├── seed
   │           ├── query.sql
   │           ├── stack.txt
   │           └── reproducer.sh
   └── stats.json             # runs / unique crashes / coverage
```

## 4. 종료 코드 (TBD)

| 값 | 의미 |
|---|---|
| 0 | 시간 내 crash 없음 |
| 1 | crash 발견 (corpus/ 에 보관) |
| ≥2 | infra 오류 (DB 미기동, conf 오류 등) |

ADR-EXT-002 후 동결.

## 5. NG2 / NG4 점검

신규 진입점 — 충돌 없음.
