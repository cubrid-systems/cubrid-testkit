# CLI Tree — `ctp.sh` Dispatch

**Source:**
- `cubrid-testtools/CTP/bin/ctp.sh` (thin launcher)
- `cubrid-testtools/CTP/common/src/com/navercorp/cubridqa/ctp/CTP.java` (실제 dispatcher)
- `cubrid-testtools/CTP/common/src/com/navercorp/cubridqa/ctp/ComponentEnum.java` (지원 task 목록)

**Phase 0 M0 status:** 완료

---

## 진입 흐름 요약

```
ctp.sh
  └─ exec: java -cp common/lib/cubridqa-common.jar com.navercorp.cubridqa.ctp.CTP "$@"
        └─ CTP.main(args)
              ├─ 옵션 파싱: -c <config>, --interactive, -h, -v
              ├─ taskList = 옵션 외 인자들 (예: "sql medium shell")
              └─ 각 task에 대해:
                    ComponentEnum.valueOf(task.toUpperCase())
                    └─ switch(component) → executeXxx(...)
```

`ctp.sh`는 18라인 셸 스크립트로, **실제 dispatch 로직은 셸이 아니라 Java 클래스 `CTP.main`**에 있다. 셸은 다음만 담당:

1. `CTP_HOME` 환경변수 설정 (`bin/`의 부모 디렉터리 절대 경로)
2. classpath 구성 (cygwin이면 `cygpath -wp` 변환)
3. `JAVA_HOME` 검사 후 java 실행, stdout/stderr를 `tee`로 임시 파일에 기록
4. 임시 파일에서 `#SCRIPTCONT` 마커가 붙은 라인을 추출해 별도 스크립트로 실행 (대화형 모드용)
5. 임시 파일 정리 후 java 종료 코드 반환

---

## 옵션 (CTP.java OPTIONS)

| Short | Long | Arg | 의미 |
|-------|------|-----|------|
| `-c` | `--config` | true | 설정 파일 경로 (생략 시 `$CTP_HOME/conf/<suite>.conf` 자동 사용) |
| | `--interactive` | false | 대화형 단일 케이스/폴더 실행 모드 (SQL 계열만 의미 있음) |
| `-h` | `--help` | false | 사용법 출력 |
| `-v` | `--version` | false | 버전 출력 |

옵션 외 위치 인자는 모두 task 이름으로 간주되며 **여러 개 나열 가능**. 예: `ctp.sh sql medium shell`.

---

## ComponentEnum (21개 선언)

```
SQL, MEDIUM, KCC, NEIS05, NEIS08,
SHELL, CCI, DOTS,
HA_REPL, ISOLATION, JDBC, NBD,
SQL_BY_CCI, SYSBENCH, TPCC, TPCW, YCSB,
WEBCONSOLE, UNITTEST, RQG, CDC_REPL
```

`ComponentEnum.valueOf`가 인식 못 하면 헬프 출력 후 해당 task만 skip.

---

## switch dispatch 트리

`CTP.main`의 task 루프 안 `switch(component)` 매핑:

| Task | Suite | 분기 | Backend | 비고 |
|------|-------|------|---------|------|
| `WEBCONSOLE` | — | utility | `executeWebConsole` → reflection `sql/lib/cubridqa-cqt.jar` :: `cqt.webconsole.Starter.exec(webconsole.conf, webRoot, "start"\|"stop")` | `taskList[1]`을 start/stop 인자로 사용. `isUtility=true` 로 task 루프 진입 전 return |
| `SQL` | sql, useCCI=false | shell-out | `executeSQL` → `sh $CTP_HOME/sql/bin/run.sh -s sql -f <conf>` (메모리릭 활성 시 `run_memory.sh`) | jdbc 인터페이스 |
| `MEDIUM` | medium | shell-out | 동일 (suite=medium) | conf로 구분, 코드 경로 동일 |
| `KCC` | kcc | shell-out | 동일 | 도메인 전용 |
| `NEIS05` | neis05 | shell-out | 동일 | 도메인 전용 |
| `NEIS08` | neis08 | shell-out | 동일 | 도메인 전용 |
| `SQL_BY_CCI` | sql_by_cci, useCCI=true | shell-out | 동일하나 `sql_interface_type=cci` 환경변수 추가 | CCI 인터페이스 |
| `SHELL` | shell, resultName=null | reflection | `executeShell` → `shell/lib/cubridqa-shell.jar` :: `shell.main.Main.exec(conf)` | |
| `RQG` | rqg, resultName="rqg" | reflection | 동일 jar/메서드, `TEST_CATEGORY=rqg` 시스템 프로퍼티 추가 | RQG는 shell 모듈 위에 얹힌 카테고리 |
| `ISOLATION` | isolation | reflection | `executeIsolation` → `isolation/lib/cubridqa-isolation.jar` :: `isolation.Main.exec(conf)` | |
| `HA_REPL` | ha_repl | reflection | `executeHaRepl` → `ha_repl/lib/cubridqa-ha_repl.jar` :: `ha_repl.Main.exec(conf)` | |
| `CDC_REPL` | cdc_repl | reflection | `executeCdcRepl` → `cdc_repl/lib/cubridqa-cdc_repl.jar` :: `cdc_repl.Main.exec(conf)` | |
| `JDBC` | jdbc | shell-out | `executeJdbc` → `sh $CTP_HOME/jdbc/bin/run.sh <conf>` | |
| `UNITTEST` | unittest | reflection | `executeUnitTest` → `shell/lib/cubridqa-shell.jar` :: `shell.main.GeneralLocalTest.exec(conf)` | **shell 모듈 jar를 공유**. config 없으면 null 인자로 호출 가능 |
| `CCI` | — | (no case) | — | enum에는 있으나 switch 분기 없음 → silent no-op |
| `DOTS` | — | (no case) | — | 동일 |
| `NBD` | — | (no case) | — | 동일 |
| `SYSBENCH` | — | (no case) | — | 동일 |
| `TPCC` | — | (no case) | — | 동일 |
| `TPCW` | — | (no case) | — | 동일 |
| `YCSB` | — | (no case) | — | 동일 |

### Dispatch 패턴 분류

CTP.main은 task를 **3가지 호출 패턴**으로 위임:

1. **Shell out (process exec)** — `LocalInvoker.exec(...)` 로 별도 셸 스크립트 실행
   - `executeSQL` (5개 + SQL_BY_CCI: 총 6개 task가 같은 메서드 공유)
   - `executeJdbc` (1개: JDBC)
2. **Reflection load (in-process)** — 모듈별 jar를 `URLClassLoader`로 로드 후 `Main.exec(configFilename)` 호출
   - `executeShell` (2개: SHELL/RQG, 같은 jar/엔트리)
   - `executeIsolation` (1개: ISOLATION)
   - `executeHaRepl` (1개: HA_REPL)
   - `executeCdcRepl` (1개: CDC_REPL)
   - `executeUnitTest` (1개: UNITTEST, shell jar 공유, 다른 엔트리)
3. **Utility (탈출 분기)** — `executeWebConsole`. `isUtility=true` 로 task 루프 진입 전에 return.
   - `WEBCONSOLE` (1개)

---

## 산출물 트리 (텍스트)

```
ctp.sh
└── java com.navercorp.cubridqa.ctp.CTP "$@"
    │
    ├── [utility] WEBCONSOLE ──► sql/lib/cubridqa-cqt.jar :: cqt.webconsole.Starter.exec(...)
    │
    ├── [shell-out] SQL ──┐
    │                MEDIUM ──┼─► executeSQL ──► sh sql/bin/run.sh -s <suite> -f <conf>
    │                  KCC ──┤              (또는 run_memory.sh, useCCI 시 sql_interface_type=cci)
    │                NEIS05 ──┤
    │                NEIS08 ──┤
    │            SQL_BY_CCI ──┘
    │
    ├── [shell-out] JDBC ──► executeJdbc ──► sh jdbc/bin/run.sh <conf>
    │
    ├── [reflection] SHELL ──┬─► executeShell ──► shell/lib/cubridqa-shell.jar
    │                   RQG ──┘                  :: shell.main.Main.exec(conf)
    │                                            (RQG는 TEST_CATEGORY=rqg 시스템 프로퍼티)
    │
    ├── [reflection] UNITTEST ──► executeUnitTest ──► shell/lib/cubridqa-shell.jar
    │                                              :: shell.main.GeneralLocalTest.exec(conf)
    │
    ├── [reflection] ISOLATION ──► executeIsolation ──► isolation/lib/cubridqa-isolation.jar
    │                                                 :: isolation.Main.exec(conf)
    │
    ├── [reflection] HA_REPL ──► executeHaRepl ──► ha_repl/lib/cubridqa-ha_repl.jar
    │                                            :: ha_repl.Main.exec(conf)
    │
    ├── [reflection] CDC_REPL ──► executeCdcRepl ──► cdc_repl/lib/cubridqa-cdc_repl.jar
    │                                              :: cdc_repl.Main.exec(conf)
    │
    └── [orphan] CCI, DOTS, NBD, SYSBENCH, TPCC, TPCW, YCSB
                  (enum 선언만, switch 분기 없음 — silent no-op)
```

---

## 새 시스템 설계 시 주목할 점

1. **CLI 표면은 21개 enum 중 14개만 살아 있음** (orphan 7개는 새 시스템에서 제거 또는 명시적 폐기 결정 필요).
2. **호출 모델이 3종 혼재**: process exec (sql/jdbc), in-process reflection (shell/isolation/ha_repl/cdc_repl/unittest), utility 분기 (webconsole). 새 시스템은 이를 통일된 plug-in 인터페이스로 흡수할 것을 권고 — 단, 외부 사용자에 보이는 CLI 인자/종료 코드/표준출력 포맷은 동결 대상이므로 내부 모델만 통일.
3. **`UNITTEST`는 shell 모듈 jar를 공유** (`GeneralLocalTest`). shell 모듈 대체 시 unittest 의존이 함께 끊기지 않도록 주의 — strangler-fig 1차 대체 후보가 shell이 아니라면 영향 적음.
4. **`#SCRIPTCONT` 인터랙티브 컨베이어** (ctp.sh): java가 `#SCRIPTCONT` 접미사 라인을 찍어 셸이 후행 실행하는 구조. 대화형 SQL 실행에서만 발현. 새 시스템에서는 IPC 방식을 단순화할 여지가 있음 (예: 상위 셸에 stdout으로 자유 명령을 흘리는 대신 명시적 named-pipe 또는 별도 RPC).
5. **conf 파일 자동 매핑**: `-c` 미지정 시 `$CTP_HOME/conf/<suite>.conf`로 fallback. 외부 표면 동결 명세에 이 규약이 포함되어야 함.
6. **`CTP_HOME` 의존**: 모든 subprocess 호출이 `${CTP_HOME}` 절대 경로 의존. 새 시스템에서도 단일 home 디렉터리 가정을 유지할지 ADR 필요.

---

## 후속 M0 작업 출발점

cli-tree 분석 과정에서 식별된 backend 진입점 (M0 #4 시퀀스 분석의 출발점):

- shell: `CTP/shell/src/com/navercorp/cubridqa/shell/main/Main.java`
- isolation: `CTP/isolation/src/com/navercorp/cubridqa/isolation/Main.java`
- ha_repl: `CTP/ha_repl/src/com/navercorp/cubridqa/ha_repl/Main.java`
- cdc_repl: `CTP/cdc_repl/src/com/navercorp/cubridqa/cdc_repl/Main.java`
- sql / medium 계열: Java entry 없음 — `CTP/sql/bin/run.sh` (27KB) 가 실제 실행기. **Java보다 셸이 지배적인 모듈.**
- jdbc: 마찬가지로 `CTP/jdbc/bin/run.sh` (8KB) 가 진입.

이 비대칭(일부는 Java 엔트리, 일부는 셸 엔트리)은 모듈별 분석에서 design.md 산출 시 다루어져야 한다.
