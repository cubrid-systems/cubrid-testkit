# E6 — I/O Contract (STUB)

**Status:** STUB — contract 동결은 ADR-EXT-006 incubating 정식 진입 후.
**Source:** `requirements.md` §2

---

## 1. CLI (제안)

```
ctp.sh diff-pg [-c <diff.conf>] [--peer pg|mysql|sqlite] [--mode canonical|rewrite]
   또는
testkit run differential --peer <name> [--cases <corpus>] [--seed <N>] [--mode <name>]
```

## 2. conf 스키마 (TBD)

| 키 (의제) | 값 | 출처 |
|---|---|---|
| `peer` | `pg \| mysql \| sqlite` | peer DBMS 선택 |
| `mode` | `canonical \| rewrite` | dialect 처리 |
| `cases_root` | `<dir>` | 입력 코퍼스 (E1 .slt / E2 generated 등) |
| `peer_jdbc_url` | string | peer 접속 URL |
| `cubrid_client` | `jdbc \| cci \| pg_wire` | CUBRID 측 클라이언트 |
| `tolerance_float` | float | 부동소수점 비교 허용오차 |
| `corpus_root` | `<dir>` | mismatch corpus 위치 |
| `dialect_categories` | csv `date,null,float,collation,json,overflow` | rewrite 활성 |

ADR-EXT-006 후 키/의미 동결.

## 3. 출력 포맷 (TBD)

```
<resultDir>/
   ├── main.info
   ├── mismatch/
   │     └── <classification>/<witness-hash>/
   │           ├── case.sql
   │           ├── case_rewritten.sql      # rewrite mode 일 때
   │           ├── result_cubrid.tsv
   │           ├── result_peer.tsv
   │           ├── classification.txt      # real / dialect / float / collation
   │           └── reproducer.sh
   └── stats.json                          # cases / real wrong-results / dialect skips
```

## 4. 종료 코드 (TBD)

| 값 | 의미 |
|---|---|
| 0 | 시간 내 *real wrong-result* 없음 (dialect mismatch 제외) |
| 1 | real wrong-result 발견 |
| ≥2 | infra 오류 (peer 접속 실패 등) |

dialect mismatch 만 있는 경우는 종료 코드 0 + warning 권장 (ADR-EXT-006).

## 5. NG2 / NG4 점검

신규 진입점 — 충돌 없음. **NG4 (비-CUBRID DBMS 호환 금지) 명시 점검**: 본 항목은 *peer DBMS 와의 비교 검증* — *CUBRID 가 다른 DBMS 호환을 추가* 하는 것이 아니다.
