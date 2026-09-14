# isolation — I/O Contract

**Source:** `cubrid-testtools/CTP/isolation/`

외부 표면 동결 명세의 isolation 부분. Phase 1 (concept) 의 `external-surface-freeze.md` 의 모듈별 입력.

---

## 1. CLI 표면

```
ctp.sh isolation [-c <conf_path>] [--interactive] [-h] [-v]
```

- `-c <conf>` 미지정 시 `$CTP_HOME/conf/isolation.conf` (cli-tree.md §3)
- 다중 task 호출 가능: `ctp.sh isolation sql medium` — 순차 실행
- `--interactive` 는 SQL 계열 전용 — isolation 에서는 무시됨

종료 코드:
- 0 — task 완료 (실패 케이스가 있어도 0; 결과는 result 디렉터리 / Feedback 으로만 보고)
- -1 (또는 비-0) — 환경 점검 실패 / build URL 무효 / scenario 디렉터리 부재 시 `System.exit(-1)`

---

## 2. conf 스키마 — `isolation.conf` 27 키

전체 키 목록 (conf-matrix.md 참조). 카테고리별 분류:

### 2-1. multi-instance 토폴로지
```
default.ssh.pwd / default.ssh.port
default.cubrid.<property>
default.brokercommon.<property>
default.broker1.<property> / default.broker2.<property>
default.cm.<property>
default.ha.<property>
env.instance1.ssh.{host,port,pwd,user}
env.instance1.cubrid.<property>
env.instance1.broker1.<property> / .broker2.<property>
env.instance2.ssh.{host,port,pwd,user}
env.instance2.cubrid.cubrid_port_id
env.instance2.broker1.BROKER_PORT / .broker2.BROKER_PORT
```

### 2-2. 실행 제어
```
scenario                       — testcases 케이스 디렉터리 절대경로
testcase_retry_num            — 재시도 횟수 (runone.sh -r <retry+1>)
testcase_timeout_in_secs      — per-case 타임아웃 (초)
```

### 2-3. CUBRID 패키지
```
cubrid_download_url           — build URL (있으면 CommonUtils.isAvailableURL 검증)
```

### 2-4. isolation 전용
```
backup_core_file_yn           — core dump 발생 시 백업 여부 (runone.sh -n 옵션 결정)
```

### 2-5. config dot-notation 와일드카드 의미
- `<property>` 표기는 IniData 의 dot-prefix 매칭 — 임의 cubrid/broker/ha/cm 파라미터를 그대로 cubrid_<*>.conf 에 전사
- 예: `default.cubrid.async_commit=on` → 원격 cubrid.conf 의 `async_commit=on`
- 새 시스템 conf 파서가 같은 의미를 보존해야 함

---

## 3. testcases 측 입력

### 3-1. `scenario` 디렉터리
- 정의: config 의 `scenario` 키
- 형식: `<base>/_NN_<isolation_level>/<topic>/<...>/` 트리
- 케이스 파일: `*.ctl`
- 정답 파일: `*.answer` (+ multi-stage variants `.answer1`/`.answer2`)
- 예: `~/cubrid-testcases/isolation/_02_RepeatableRead/foreign_key_column/...`

### 3-2. `.ctl` 케이스 형식 (DSL 요약 — 상세는 test-corpus.md)

```
<actor>: <statement>;
```

- actor ∈ { MC, C1, C2, ..., Cn }
- 명령:
  - `MC: setup NUM_CLIENTS = N;`
  - `MC: wait until <Cn> ready;`
  - `MC: wait until <Cn> blocked;`
  - `<Cn>: <SQL or transaction control>;`
- 주석: `/* ... */`

### 3-3. testcase exclusion
- config 의 `testcase_exclude_from_file` (path) 가 가리키는 파일에 한 줄당 한 케이스 prefix 또는 substring
- exclude 파일 첫 글자가 `#` 또는 `--` 인 라인은 주석 (skip)
- excluded 케이스는 `dispatch_tc_ALL.txt` 에 들어가지 않고 `tempSkippedList` 로 분리

---

## 4. runone.sh 호출 시그니처 (원격 측)

```
sh runone.sh [-n] -r <retry+1> <tc> <timeout_sec> <db_name> 2>&1
```

- `-n` : `backup_core_file_yn=false` 인 경우 (core file 백업 안 함)
- `-r <retry+1>` : 재시도 횟수 (config 의 testcase_retry_num + 1)
- `<tc>` : 케이스 절대경로. `/` 로 시작 안 하면 `$HOME/<tc>` 로 자동 prefix
- `<timeout_sec>` : config 의 testcase_timeout_in_secs
- `<db_name>` : config 의 testing_database

stdout 마커 (Java 측 grep 대상):
- `flag: OK` — 케이스 성공
- `flag: NOK` — 케이스 실패
- `found core file` — core dump 발견 (한 줄당 한 발견)
- `found fatal error` — fatal error 발견

마지막 `flag: OK` 와 마지막 `flag: NOK` 의 위치 비교로 passFlag 결정 (Test.java 의 `extractItems`).

---

## 5. 출력 파일 (currentLogDir = `<CTP_HOME>/result/<task>/<timestamp>/`)

| 파일 | 작성자 | 용도 |
|------|--------|------|
| `main_snapshot.properties` | TestFactory.createSnapshotForConfiguration | 시작 시점 conf snapshot (Properties + AUTO_BUILD_ID/AUTO_BUILD_BITS) |
| `dispatch_tc_ALL.txt` | Dispatch.load (fresh run) | 전체 케이스 절대경로 한 줄당 한 개 — continueMode 입력 |
| `dispatch_tc_FIN_<envId>.txt` | Test.runAll (per case) | env 별 완료 케이스 — continueMode 차집합 입력 |
| `test_<envId>.log` | Test.workerLog | env 별 워커 상세 로그 (케이스 실행 + diff 출력) |
| `isolation_result_<buildId>_<bits>_<taskId>_<ts>.tar.gz` | TestFactory.backupTestResults | currentLogDir 통째 백업 (Linux/Mac만; Windows skip) |

### diff 출력 형식 (Test.showDifferenceBetweenAnswerAndResult)

실패한 케이스의 워커 로그에 다음 라인이 포함됨:
```
=================================================================== D I F F ===================================================================
<diff -a -y -W 185 의 출력>
```

- `<tc>` → `<dir>/result/<name>.log` 와 `<dir>/answer/<name>.answer` 비교
- `\n` → `\r\n` 변환 (윈도우 호환)

---

## 6. Feedback 출력 (DB / File / Null)

config 의 `feedback_type` 키로 선택:
- `file` → impl/FeedbackFile (currentLogDir 안 텍스트 파일)
- `db` → impl/FeedbackDB (구체 DB 인터페이스는 별도 분석)
- `null` → impl/FeedbackNull (no-op)

이벤트 시그니처 (Feedback 인터페이스):
```java
onTaskStartEvent(buildUrl)
onTaskContinueEvent()
onTaskStopEvent()
setTotalTestCase(total, macroSkipped, tempSkipped)
onTestCaseStartEvent(tc, envIdentify)
onTestCaseStopEvent(tc, success, elapseMs, resultText, envIdentify, isTimeOut, hasCore, skipType)
onStopEnvEvent(envId)
```

새 시스템도 동일 이벤트 + skipType 상수 (Constants.SKIP_TYPE_NO/BY_MACRO/BY_TEMP) 보존 필요.

---

## 7. 외부 의존 contract — 동결 표면 요약

새 시스템 isolation 대체 시 다음 contract 를 *완전히* 보존해야 한다:

```
입력
├── ctp.sh isolation [-c <conf>] CLI
├── isolation.conf 27 키 (multi-instance + scenario + retry/timeout + backup_core)
├── testcases/isolation/**/*.ctl + .answer (DSL 의미 + diff 입력)
└── ctltool/ 의 runone.sh + native parser (deploy 으로 원격 복사)

출력
├── stdout: flag: OK/NOK, found core file, found fatal error
├── currentLogDir/main_snapshot.properties
├── currentLogDir/dispatch_tc_{ALL,FIN_*}.txt
├── currentLogDir/test_*.log (+ DIFF section on failure)
├── currentLogDir/isolation_result_*.tar.gz (Linux 한정)
└── Feedback 이벤트 (DB/File/Null impl 중 선택)

종료 코드
├── 0  (정상 + 케이스 실패 포함)
└── -1 (환경 점검 / build URL / scenario 부재 — System.exit(-1))
```
