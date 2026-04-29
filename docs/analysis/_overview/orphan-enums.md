# Orphan ComponentEnum 정밀 분석

**Source:** `cubrid-testtools/CTP/common/src/com/navercorp/cubridqa/ctp/ComponentEnum.java`

cli-tree.md §3 에서 발견한 7 orphan (CCI / DOTS / NBD / SYSBENCH / TPCC / TPCW / YCSB) 의 *진정한 dead code 여부* 를 확인. 새 시스템에서 *폐기 ADR* 의 입력.

**Phase 4 정밀 분석 status:** 완료

---

## 1. 결론 — 7 enum 모두 진짜 dead

전체 CTP 소스를 grep 한 결과:

| Enum 멤버 | Java 소스 사용 | 셸/conf 사용 | 결정 |
|-----------|----------------|--------------|------|
| `CCI` | 0 | (별개 — *CCI interface* 의미로만, enum 아님) | **dead** |
| `DOTS` | 0 | 0 | **dead** |
| `NBD` | 0 | 0 | **dead** |
| `SYSBENCH` | 0 | 0 | **dead** |
| `TPCC` | 0 | 0 | **dead** |
| `TPCW` | 0 | 0 | **dead** |
| `YCSB` | 0 | 0 | **dead** |

검증:
- `grep -rln "ComponentEnum\.<X>\|case <X>\|<X> ="` — 7 enum 모두 0 hits
- 셸/conf 의 "cci" / "CCI" 11+6 files — 모두 *CCI 인터페이스* (sql_by_cci 모듈, ext/run_compat_cci.sh, conf/sql_by_cci.conf) 의미. enum 멤버 `CCI` 와 무관.

→ **7 enum 모두 진짜로 사용되지 않는 dead code**.

---

## 2. CCI 의 *모호성* 정리

이름이 `CCI` 인 것은 두 종류:

### 2-1. ComponentEnum.CCI — 사용 안 됨
```java
public enum ComponentEnum {
    SQL, MEDIUM, KCC, NEIS05, NEIS08, SHELL, CCI, DOTS, ...
}
```
- 선언만 있고 switch 분기 없음
- `ctp.sh cci` 호출 시 silent no-op

### 2-2. CCI 인터페이스 — 활성, 별개
- `sql_by_cci` 모듈 (별도 enum: `SQL_BY_CCI`)
- `ext/run_compat_cci.sh`, `conf/sql_by_cci.conf` 등
- `sql_interface_type=cci` 환경 변수 (CTP.executeSQL 분기)
- `cci_compatibility_guide.md` doc

→ **두 CCI 는 의미가 다르다**. ComponentEnum.CCI 가 *legacy enum 슬롯* 이고, 현재 활용은 SQL_BY_CCI 의 변형 + sql_interface_type 플래그를 통해 이루어진다.

---

## 3. 7 enum 의 *추정 의도* (역사적)

명명을 보면 데이터베이스 벤치마크 / 호환성 테스트 분야의 표준 도구들:

| Enum | 추정 origin |
|------|-------------|
| `CCI` | (위 §2 — legacy 슬롯) |
| `DOTS` | DBT-2 / DOTS (Database Open source Test Suite, IBM) |
| `NBD` | (불명 — Network Block Device? Naver Big Data?) |
| `SYSBENCH` | sysbench (OSS DB benchmark, MySQL/MariaDB 표준) |
| `TPCC` | TPC-C (transaction processing benchmark) |
| `TPCW` | TPC-W (e-commerce benchmark) |
| `YCSB` | YCSB (Yahoo! Cloud Serving Benchmark) |

→ *벤치마크 / 호환성 테스트 통합 의도* 였으나 구현 안 됨. 또는 *과거 구현이 제거되었지만 enum 슬롯만 남음*.

git log 로 *제거 시점* 추적하면 의도 명확해질 수 있음 (후속 — `git log -- common/src/com/navercorp/cubridqa/ctp/`).

---

## 4. 새 시스템 ADR 권고 — *명시적 폐기*

### Option A: enum 자체 제거 (권고)

새 시스템에서 *7 enum 슬롯 제거*. 사용자가 `ctp.sh tpcc` 호출 시:
```
[ERROR] Unknown task: TPCC
       Available tasks: SQL, MEDIUM, KCC, NEIS05, NEIS08, SHELL, RQG,
                       ISOLATION, HA_REPL, CDC_REPL, JDBC, SQL_BY_CCI,
                       WEBCONSOLE, UNITTEST
```
명확한 에러로 디버깅 쉬워짐.

**위험:** 외부 CI 스크립트가 *이 enum 을 알고 있을* 가능성 — `ctp.sh tpcc` 호출이 *언젠가 작동하기를 기대* 하는 코드. 단 silent no-op 인 현 상태에서도 동작하지 않으므로 *의존이 있다면 이미 깨진 상태*.

### Option B: enum 유지 + NotImplementedError

silent no-op 대신 *명시적 NotImplementedError*. 하위 호환은 유지하되 런타임에 명확.

```
[ERROR] task TPCC is reserved but not implemented in the new system.
        Was this task historically used? Please report.
```

→ 이전 사용자에게 *fallback 신호* 전달. enum 제거 전 1 release 정도 이 mode 운영 후 제거.

### Option C: 외부 hook 인터페이스로 이행

7 enum 을 *plug-in 슬롯* 으로 정형화. 사용자가 직접 task 모듈을 등록 가능.

복잡. 실제 수요가 없으면 over-engineer.

---

## 5. 권고 결정

**Option A (제거) + grace period (Option B)** 결합:
1. 새 시스템 *0.x release* 에서 Option B (명시적 NotImplementedError)
2. 1 release 동안 외부 사용 신고 받음
3. 신고 없으면 *1.0 release* 에서 enum 자체 제거 (Option A)
4. 신고 있으면 *plug-in 슬롯 또는 별도 모듈* 으로 정형화

새 ADR 작성 권고: **ADR-005: Orphan ComponentEnum 폐기** (또는 ADR-001 의 후속 조항).

---

## 6. 결론

```
실측: 7 enum 모두 진짜 dead code
- Java 사용 0 hits (4 검색 패턴)
- 셸/conf 사용 0 hits

적용 권고:
- Option A (제거) + grace period (Option B) 단계적
- 새 시스템에서 21 → 14 active task name 으로 축소
- ADR-005 신규 작성

의문 항목:
- git log 로 enum 제거 / 추가 시점 확인 (후속)
- NBD 의 정확한 의미 (네이버 / Network Block Device?)
- 외부 CI 에 *이 enum 을 호출하는 hook* 이 있는지 인터뷰
```
