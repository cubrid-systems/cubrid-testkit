# isolation — Requirements (모듈이 해결하는 문제 + 외부 호출 형태)

**Source:** `cubrid-testtools/CTP/isolation/`

**Companion docs:** `design.md` (시퀀스), `io-contract.md` (인터페이스), `test-corpus.md` (케이스), `implementation-notes.md` (미묘 동작)

---

## 1. 이 모듈이 해결하는 문제

CUBRID 의 **트랜잭션 격리 수준 / MVCC / 동시성 락** 동작을 *재현 가능한 다중 클라이언트 시나리오* 로 검증한다.

단일 클라이언트로는 검증할 수 없는 동작 — 예:
- 두 트랜잭션이 같은 인덱스에 동시 접근할 때 락 대기/해소
- 한 트랜잭션의 DDL 중 다른 트랜잭션의 DML 차단/허용
- read committed / repeatable read / serializable 의 가시성 차이를 결정론적으로 검증
- HA 복제와 결합된 격리 동작

이를 위해 `.ctl` DSL 로 N 개 클라이언트의 명령 시퀀스를 작성하고, MC (Master Controller) 가 *동기 포인트* (`wait until C1 ready`, `wait until C2 blocked`) 를 통해 race 를 결정론적으로 재현한다.

---

## 2. 외부 호출 형태

### 2-1. 사용자 측 CLI

```
ctp.sh isolation [-c <isolation.conf>]
```

`-c` 미지정 시 `$CTP_HOME/conf/isolation.conf` 자동 사용 (cli-tree.md §3).

### 2-2. CTP.java 측 모듈 진입점

```
URLClassLoader(isolation/lib/cubridqa-isolation.jar)
  → reflection invoke
    com.navercorp.cubridqa.isolation.Main.exec(configFilename: String): void
```

### 2-3. testcases 측 케이스 진입점 (원격 SSH 세션)

```sh
cd $ctlpath
sh runone.sh [-n] -r <retry+1> <tc.ctl> <timeout_sec> <db_name> 2>&1
```

`runone.sh` (CTP/isolation/ctltool/runone.sh) 가 ctltool native binary 를 호출하여 .ctl 케이스를 파싱/실행. stdout 의 마커를 Java 측 `Test.runTestCase` 가 grep.

---

## 3. 사용자 요구사항 (M0 분석으로 추정)

CTP doc 가이드 `doc/isolation_guide.md` (62KB) 가 가장 권위 있는 출처. 정밀 인용은 Phase 1 후속.

핵심 사용자 요구 (design.md / case-formats.md 분석으로 추정):

1. **결정론적 동시성 재현** — 같은 .ctl 을 N 번 실행해도 같은 OK/NOK 결과
2. **다중 instance 지원** — env.instance{1,2}.* config 로 노드별 워커 병렬 실행 (TestFactory.concurrentTest)
3. **continue mode** — 실패 케이스만 재실행 (`testcase_retry_num`, `dispatch_tc_FIN_<env>.txt`)
4. **core file 자동 수집** — 케이스 실행 중 core dump 발생 시 보고 (`backup_core_file_yn` 옵션)
5. **scenario tree 분류** — testcases/isolation/_<NN>_<level>/<topic>/ 구조

---

## 4. 비기능 요구

| 항목 | 현재 동작 | 새 시스템에서의 의미 |
|------|----------|---------------------|
| 동시성 재현성 | MC `wait until` 동기화 | DSL 의미 동결 의무 |
| 분산 실행 | env마다 worker thread (Java ExecutorService, fixed 100) | 새 시스템도 *N env × per-env worker* 가능해야 |
| continue 실행 | dispatch_tc_FIN_<env>.txt 차집합 | 동일 형식 유지 또는 마이그레이션 도구 |
| core file 처리 | ulimit -c unlimited + find 수집 | 모던 정책 (auto-zip + 통보) ADR 후보 |
| 타임아웃 | testcase_timeout_in_secs (per-case) | runone.sh 가 제어 — Java 측 monitor 비활성 |
| 재시도 | runone.sh -r <retry+1> | dispatcher 단 retry-aware (shell의 DispatchTicket 모델 차용 후보) |

---

## 5. 의존하는 외부 자원 (요약)

상세는 `io-contract.md`.

- **CUBRID 설치**: 원격 env 의 cubrid binary + service stop/start
- **testcases 레포**: cubrid-testcases/isolation/ (6,778 .ctl + 6,853 .answer)
- **ctltool 자산**: CTP/isolation/ctltool/ (native C parser + runone.sh + Makefile) — deploy 가 원격으로 복사
- **공통 인프라**: CTP/common (CommonUtils/IniData/Log/coreanalyzer.CommonUtil) + CTP/shell/common (SSHConnect/LocalInvoker/ScriptInput/...)
- **원격 환경 변수**: $ctlpath, $CUBRID, $HOME

---

## 6. 새 시스템 설계 입력 (요약)

isolation 대체 시 *반드시 충족*:

1. ✅ `.ctl` DSL 의미 보존 (MC/C1..Cn, wait until {ready,blocked}, timeout)
2. ✅ runone.sh 호출 시그니처 동일
3. ✅ stdout 의 `flag: OK`/`flag: NOK`/`found core file`/`found fatal error` 마커
4. ✅ dispatch_tc_FIN_*.txt 형식 호환 (또는 명시적 마이그레이션 도구)
5. ✅ env 단위 병렬 + 케이스 단위 thread-safe pull (Dispatch singleton 동등)
6. ✅ core file 발견 시 동일 보고 채널 (Feedback 인터페이스 유지/호환)

ADR 후보:
- ctltool native parser 처리 (흡수 / subprocess / 재작성) — ADR-004 입력
- TestMonitor 활성화 (현 비활성)
- shell.common.* 의존 처리 — ADR-004 의 1차 대체 단위 결정과 직결
