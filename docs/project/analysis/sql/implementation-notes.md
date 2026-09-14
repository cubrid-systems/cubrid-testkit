# sql — Implementation Notes (미묘 동작 + 깨지기 쉬운 가정)

**Source:** `cubrid-testtools/CTP/sql/`

design.md 의 시퀀스 위에서, 새 시스템 작성 시 회귀 위험이 큰 미묘한 동작들.

---

## 1. `run.sh` 의 6단계 평탄 pipeline — 분기 없는 순차

```bash
do_init
do_clean
do_configure
do_create_db
do_test
do_summary_and_clean
```

- 각 단계 fail 시 *exit 1* 또는 *조용한 진행*. error handling 이 단계마다 다름.
- 한 단계 fail 이 다음 단계로 *전파되지 않을 수도* 있음 (예: do_configure fail 후에도 do_create_db 시도).

→ 새 시스템에서 *명시적 단계 게이팅* (각 단계 exit 코드 검사) 권고.

---

## 2. `scenario_repo_root=$HOME/dailyqa` 디폴트

```bash
scenario_repo_root=$HOME/dailyqa
```

config 미지정 시 `$HOME/dailyqa` 가 자동 선택. *사용자가 모르고 실행하면 의도치 않은 디렉터리* 가 testcases 로 사용됨.

→ 새 시스템에서 *명시적 conf 키 강제* + 기본값 fallback 시 경고 출력.

---

## 3. `do_init` 의 db_name 분기 — 3가지 suite

```bash
if [ "$scenario_full_name" == "medium" ]; then
    db_name="mdb"
elif [ "$scenario_full_name" == "site" ]; then
    db_name="${scenario_category}"
else
    db_name="basic"
fi
```

`site` suite 가 enum (ComponentEnum) 에 없는데 run.sh 에는 분기 존재. 즉 **legacy 코드** 또는 *외부 호출자가 -s site 로 직접 run.sh 실행* 가능. 새 시스템에서 site 의 정체 확인 후속.

---

## 4. `make_locale` Windows 한정 + medium/site 스킵

```bash
if [ "$os_type" == "Windows" ]; then
    if [ "$scenario_full_name" != "medium" -a "$scenario_full_name" != "site" ]; then
        # locale 빌드
        mv $CUBRID/conf/cubrid_locales.txt $CUBRID/conf/cubrid_locales_bak.txt
        cp $CUBRID/conf/cubrid_locales.all.txt $CUBRID/conf/cubrid_locales.txt
        ...
        $batPath/bin/make_locale.bat /release
    fi
fi
```

- Windows 에서만 동작
- medium/site 는 skip
- `cubrid_locales.txt` 를 *직접 mv/cp* — race condition 위험 (다른 케이스가 동시 사용 시)
- `cubrid_locales_bak.txt` 가 정리 시 복원되는지 후속 분석 필요

→ 새 시스템에서 locale 빌드 정책 ADR.

---

## 5. `loadjava` 명령 의존 (Java SP)

```bash
loadjava $db_name $clz 2>&1 >> $log_filename
```

`loadjava` 는 Oracle 기원 명령으로 CUBRID 도 호환 제공. `java_stored_procedure=true` config 시 케이스 시작 전 jar 적재.

→ 새 시스템에서 loadjava 호환 유지 또는 명시적 wrapper.

---

## 6. ConsoleAgent 의 stdout 인터셉트 (`StdOutJob`)

```java
stdOutJob = new StdOutJob(System.out, StdOutJob.START);
stdOutJob.start();
...
stdOutJob.setEnd(StdOutJob.END);
stdOutJob.stop();
```

`System.out` 자체를 *intercept* 해서 cqt 내부 로그 처리. **위험**: cqt 가 stop 안 부르면 stdout 이 *영구 hijack*. exception 이 finally 안에서 잡히지만, JVM 강제 종료 시 회복 불가.

→ 새 시스템에서 *명시적 logger* 사용 (System.out 인터셉트 회피).

---

## 7. `comefrom` enum (32/64 bit literal)

```java
private static final int COME_FROM_CQT_32 = 32;
private static final int COME_FROM_CQT_64 = 64;
```

매직 숫자 — 비트 수 자체를 *호출 출처 식별자* 로 사용. *32/64bit* 가 *case 결과 라벨* (testCategory) 에도 직접 들어감.

→ 새 시스템에서 *enum 또는 typed value* 로 분리.

---

## 8. `bo.runTest(test)` — 35개 console/util 클래스의 협업

`ConsoleBO.runTest` 가 cqt.console.util/* 의 35 클래스 (CommandExecutor, CubridConnection, EnvGetter/Setter, ProcessMonitor, ErrorInterrupt, ...) 를 동원. 본 분석에서 정밀 흐름 미파악 — 필요 시 design.md 갱신 + 별도 deep-dive.

→ 새 시스템에서 *deep dive* 의 우선순위는 strangler-fig 결정 (ADR-004) 후. sql 단독 대체 결정 시 35개 utility 정밀 분석 필요.

---

## 9. `lock_timeout` 등 conf 파라미터의 *원격 설정 적용 시점*

`default.cubrid.<property>=<value>` 는 *원격 cubrid.conf 에 전사*. run.sh 의 do_configure 가 이를 처리한다고 추정 — 정확한 적용 시점 (createdb 전 / 후 / 매 케이스) 후속 분석.

위험: 잘못된 timing 으로 케이스 실행 중 cubrid.conf 가 변경되면 결과가 비결정적.

---

## 10. `set -x` 활성 — 디버깅 trace 가 stdout 에 노출

run.sh 에 `set -x` 가 활성. 셸 명령이 *모두 stderr 에 출력* — log_filename 에 거대한 trace 가 누적.

→ 새 시스템에서 *trace 옵션* 으로 분리 (default off, debug 시 on).

---

## 11. `cubrid_locales_bak.txt` cleanup 미보장

위 §4 에서 mv 한 백업 파일이 *do_test 종료 후 복원되는지* 코드 상에서 명시 안 됨. cleanup 누락 시 *다음 실행* 에서 잘못된 locales 사용.

---

## 12. `do_summary_and_clean` 의 core file 탐색

```bash
coreFiles=$(find "$CUBRID" "${CTP_HOME}" -type f -name "core*")
while read -r file; do
    isCore=`file "$file"|grep 'core file'|grep -v grep|wc -l`
    if [ $isCore -ne 0 ]; then
        echo "CORE_FILE:$file"
        let "coreCount=coreCount+1"
    fi
done <<EOF
$coreFiles
EOF
```

- `find $CUBRID $CTP_HOME` — 두 디렉터리만 검색 (testcases dir 제외)
- `file` 명령으로 *진짜 core 파일* 구분 (이름이 core* 인 정상 파일 false positive 방지)
- *발견된 core 파일은 삭제 안 함* — 사용자가 수동 정리해야 함

→ 새 시스템에서 core 정책 ADR (모든 모듈 공통 후보).

---

## 13. `do_clean` 의 정확한 범위 미파악

run.sh 시작과 끝에서 `do_clean` 호출. 정리 범위 (DB 삭제 / 임시 파일 삭제 / cubrid 프로세스 kill / shared memory 정리) 정밀 분석 후속.

`remove_shared_memory()` 와 `remove_all_ipc_segments()` 함수 존재 (run.sh:246 부근) — IPC segment 정리도 do_clean 의 일부.

---

## 14. `interactive` 모드의 #SCRIPTCONT trick

run.sh do_test interactive 분기:
```bash
(export CLASSPATH=...
 export ...
 export PS1="sql> "; cd ${scenario_repo_root}; source ${CTP_HOME}/sql/bin/interactive.sh; help; bash --posix)

# do clean for interactive mode
do_clean
```

- 서브쉘 안에서 bash --posix 실행
- 사용자가 종료 (exit) 하면 do_clean 호출
- CTP.java 가 *Java 측에서 stdout 의 `#SCRIPTCONT` 마커를 추출해 셸 스크립트로 실행* — Interactive 모드에서 사용자 명령이 stdout 에 출력될 때 마커 패턴 부여 가능 (cli-tree.md §6 의 #SCRIPTCONT)

이 메커니즘은 매우 ad-hoc. 새 시스템에서 *first-class REPL* (예: Go 의 cobra interactive shell) 권고.

---

## 15. `do_test` 의 `(...)` 서브쉘 wrapping

```bash
(...
 if interactive ...
 elif cci ...
 else java ...
 fi
)
```

서브쉘 안에서 export 환경변수 — 부모 셸을 오염시키지 않음. 단, 서브쉘 종료 후 PWD/etc 복원이 *do_test 호출자 측 책임*. `cd $curDir` 가 명시적으로 두 곳에 있음.

---

## 16. `tee -a $log_filename` 의 race

`java ... | tee -a $log_filename` — Java 출력을 stdout + 파일 동시. **위험**: 다른 프로세스가 동시에 같은 파일에 쓰면 인터리빙. 1인 단일 호스트 환경에선 영향 없으나, 분산 모니터가 *log_filename 을 읽는 동안* 쓰기 진행 시 부분 읽기 가능.

---

## 17. `xargs -I{} test {} -ge 34` 부재

(이는 ROADMAP 의 verification 명령에서 본 패턴 — sql 자체 implementation 과 무관, 무시)

---

## 18. `EnvironmentCheck` 의 누락된 검사

`cqt.console.util.EnvironmentCheck` 가 실행 전 환경 검사. **무엇을 검사하는지** 정밀 분석 후속. `$CUBRID`, `$JAVA_HOME` 외에 다른 변수도 검사할 가능성 — 새 시스템에서 동일한 검사 보존 필요.

---

## 19. `loadjava` 의 *암묵적 db connection*

```bash
"$JAVA_HOME/bin/javac" -cp $CUBRID/jdbc/cubrid_jdbc.jar *.java
loadjava $db_name $clz 2>&1 >> $log_filename
```

- 케이스 자체가 javac 빌드 + loadjava 적재
- DB connection 정보 (사용자/비밀번호) 가 명시적 인자가 아님 — 환경 변수 또는 기본값 사용
- 새 시스템에서 명시적 connection string 통합 권고
