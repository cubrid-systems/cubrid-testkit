# E3 — I/O Contract (STUB)

**Status:** STUB — contract 동결은 ADR-EXT-003 incubating 정식 진입 후.
**Source:** `requirements.md` §2

---

## 1. CLI (제안)

```
ctp.sh sqlancer [-c <sqlancer.conf>] [--oracle norec|tlp|pqs]
   또는
testkit run sqlancer [-c <conf>] [--oracle <name>] [--seed <N>] [--time <sec>] [--client jdbc|cci]
```

복수 oracle 동시 지정 의제: `--oracle norec,tlp` (mismatch 분류 시 oracle 태깅).

## 2. conf 스키마 (TBD)

| 키 (의제) | 값 | 출처 |
|---|---|---|
| `seed` | `<int>` | 재현용 |
| `time_budget_sec` | `<int>` | CI 시간 통제 |
| `oracle` | csv `norec,tlp,pqs` | oracle 선택 |
| `client` | `jdbc \| cci` | SUT 구동 채널 |
| `corpus_root` | `<dir>` | mismatch corpus 위치 |
| `dialect_extensions` | (E2 와 공유) | CUBRID 확장 가산 |

ADR-EXT-003 후 키/의미 동결.

## 3. 출력 포맷 (TBD)

```
<resultDir>/
   ├── main.info
   ├── mismatch/
   │     └── <oracle>/<seed>/
   │           ├── q1.sql
   │           ├── q2.sql           # NoREC: rewrite / TLP: partition triple
   │           ├── schema.sql
   │           ├── result_q1.tsv
   │           └── result_q2.tsv
   └── stats.json                   # iterations / unique mismatches / per-oracle hit
```

## 4. 종료 코드 (TBD)

| 값 | 의미 |
|---|---|
| 0 | 시간 내 mismatch 없음 |
| 1 | mismatch 발견 |
| ≥2 | infra 오류 |

## 5. NG2 / NG4 점검

신규 진입점 — 충돌 없음. NG4 (CUBRID 가 SUT) 와도 직교.
