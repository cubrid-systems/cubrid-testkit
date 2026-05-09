# E1 — I/O Contract (STUB)

**Status:** STUB — contract 동결은 ADR-EXT-001 incubating 정식 진입 후.
**Source:** `requirements.md` §2

---

## 1. CLI (제안)

```
ctp.sh sqllogictest [-c <sqllogictest.conf>]
   또는
testkit run sqllogictest [-c <conf>] [--variant sqlite|duckdb|cockroach] [--client jdbc|cci|cli]
```

옵션 / 디폴트 / 종료 코드: ADR-EXT-001 후 동결.

## 2. conf 스키마 (TBD)

| 키 (의제) | 값 | 출처 |
|---|---|---|
| `corpus_root` | `<dir>` | 외부 코퍼스 위치 (test-corpus.md) |
| `client` | `jdbc | cci | cli` | SUT 구동 채널 |
| `variant` | `sqlite | duckdb | cockroach` | spec baseline |
| `compare_mode` | `hash | values` | 결과 비교 모드 |

ADR-EXT-001 합의 전에는 *제안 수준*. 키 이름/의미/디폴트 모두 동결되지 않음.

## 3. 입력 포맷

`.slt` — sqllogictest spec record 포맷:
```
statement (ok|error)
<SQL>
----
<expected error msg, optional>

query <type> [<sort>] [label]
<SQL>
----
<hash | rows>
```

## 4. 출력 포맷 / 종료 코드 (TBD)

ADR-EXT-001 후 — `<resultDir>/main.info` 호환 여부 결정.

## 5. NG2 (외부 표면 동결) 점검

없음 — 신규 진입점. 본 항목은 *외부 표면 동결 외부* 의 신규 추가.
