# common — Design (모듈 내부 구조)

**Source:** `cubrid-testtools/CTP/common/`

**상위 분석:** `analysis/_overview/deps-of-common.md` (8 하위 디렉터리 의존 그래프). 본 문서는 그 안의 *Java src 트리 정밀 분석*.

---

## 1. Java src 트리 (총 41 파일, 5 패키지)

```
com.navercorp.cubridqa
├── common (20)              ← 메인 utility 패키지
├── common.coreanalyzer (11) ← core dump 분석 (sql/isolation 만 import)
├── common.grepo (5)         ← git 레포 RMI client
└── ctp (4)                  ← CTP CLI 호스트
   plus
com.nhncorp.cubrid.common.grepo (1) ← legacy nhncorp 네임스페이스 (RMI 인터페이스)
```

추가 별도 sub-module (자체 컴파일 + 일부는 같은 jar 에 머지):
- `grepo/src/com.navercorp.cubridqa.common.grepo.service` (5 파일) — RMI server impl
- `sched/src/com.navercorp.cubridqa.scheduler` (~30 파일) — 별도 jar (cubridqa-scheduler.jar)

---

## 2. 메인 패키지 `com.navercorp.cubridqa.common` — 20 클래스

### 2-1. Utility 코어 (3) — 거의 모든 모듈이 import

| 클래스 | 책임 | 메서드 (대표) |
|--------|------|--------------|
| **CommonUtils** | 잡다 utility (40+ static method) | replace, isEmpty, rightTrim, concatFile, isAvailableURL, getFileContent, getLineList, getProperties, getPropertiesWithPriority, sleep, dateToString, getEnvInFile, isWindowsPlatform, isCygwinPlatform, getLinuxStylePath, getWindowsStylePath, getFixedPath, convertBoolean, getShellType, getSimplifiedBuildId, parsePropertiesByPrefix (dot-prefix 매칭), parseInstanceParametersByRole, ... |
| **IniData** | INI 파일 파서 (ini4j 래핑) + Section 클래스 | get(section, key), getAndTrans, getSection, put, remove, saveAs, translateValue (static) |
| **Constants** | 6 상수 | ENV_CTP_HOME_KEY, LINE_SEPARATOR, ... |

### 2-2. 설정 키 정의 (1)

| 클래스 | 책임 |
|--------|------|
| **ConfigParameterConstants** | conf 키 82개를 type-safe 상수로 정의 (예: `SCENARIO`, `TESTCASE_RETRY_NUM`, `TESTCASE_TIMEOUT_IN_SECS`, `TEST_INSTANCE_HOST_SUFFIX`, `ROLE_ENGINE`, `ROLE_BROKER1`, ...) |

→ conf-matrix.md 의 81 unique key 와 거의 일치 (1 차이는 별 의미 없음). **새 시스템에서 conf 키 카탈로그를 *모든 모듈이 참조하는 공통 표면*** 으로 유지.

### 2-3. 로깅 (2)

| 클래스 | 책임 |
|--------|------|
| **Log** | 파일 로거 (println / close, append/truncate 모드) |
| **JschLogger** | jsch 라이브러리의 로깅 어댑터 (Java SE Logger ↔ jsch logger bridge) |

→ 모듈마다 *각자의 Log 클래스가 또 있음* (shell.common.Log, scheduler.common.Log) — 이름 중복.

### 2-4. 원격 통신 (5)

| 클래스 | 책임 |
|--------|------|
| **SSHConnect** | jsch 기반 SSH 클라이언트 (Connect / execute / close) |
| **SFTP** | SFTP 베이스 클래스 |
| **SFTPDownload** | 파일 다운로드 |
| **SFTPUpload** | 파일 업로드 |
| **RunRemoteScript** | 원격 셸 스크립트 실행 헬퍼 (SSH + script staging) |

⚠️ **shell.common.SSHConnect 와 cqt.common.SSHConnect 가 별개로 존재**. M0 #4 의 발견 — common.SSHConnect 의 *진짜 사용처는 ctp 패키지 자체* 와 일부 utility 만. 모듈 측은 shell.common.SSHConnect 를 더 자주 import.

### 2-5. 로컬 실행 / 파일 (3)

| 클래스 | 책임 |
|--------|------|
| **LocalInvoker** | Process exec (sh / cmd / bash). result capture + tee 옵션 |
| **MakeFile** | Makefile 생성 추정 (shell 모듈만 사용) |
| **ShellInput** | 셸 명령 빌더 — addCommand / 멀티라인 |

### 2-6. 통보 / 보고 (1)

| 클래스 | 책임 |
|--------|------|
| **MailSender** | SMTP 이메일 (shell 모듈만 사용) |

### 2-7. 메모리 / 데이터 (3)

| 클래스 | 책임 |
|--------|------|
| **ShareMemory** | Java NIO MappedByteBuffer 기반 IPC (sql 모듈만 사용 — cqt 와 native 동기화) |
| **MergeTemplate** | 템플릿 머지 (사용처 후속 분석) |
| **ParseActionFiles** | action 파일 파서 (사용처 후속) |

### 2-8. 부수 도구 / main 보유 (3)

| 클래스 | 책임 |
|--------|------|
| **SparseFsMain** | sparse filesystem 도구 (`main(String[2])` — 2 인자) — 상세 후속 |
| **CommitConfigFileIntoDB** | config 파일을 DB 에 적재 (어떤 DB?) — 후속 |
| **CommonUtils.main** | 테스트/유틸 — 후속 |

---

## 3. coreanalyzer 패키지 (11 클래스) — core dump 분석 도구

```
.coreanalyzer/
├── AnalyzerMain        ← entry (sql 측이 import)
├── Analyzer            ← 본체
├── CheckAllStack       ← stack 검사
├── CommonUtil          ← coreanalyzer 전용 utility (isolation 측이 import)
├── Constants           ← 별도 상수
├── CoreBO              ← business object
├── IssueBean           ← 이슈 데이터 모델
├── IssueMain           ← issue 보고 entry
├── LocalInvoker        ← 별도 LocalInvoker (이름 충돌)
├── StackItem           ← 스택 프레임 데이터
└── UpdateDigestStack   ← 스택 다이제스트 갱신
```

**관찰:**
- common.LocalInvoker 와 coreanalyzer.LocalInvoker 가 둘 다 존재 — *이름 충돌* + 의도된 분리인지 의문
- AnalyzerMain 은 sql/run.sh 가 호출 가능 (do_summary_and_clean 의 core 처리 일환?). isolation 측은 CommonUtil 만 import.
- **핵심 추정:** core dump 발생 시 stack frame 디지털 다이제스트 (UpdateDigestStack) → 동일 패턴의 issue 자동 그룹화 → IssueBean 생성

→ deps-of-common.md §8 의 *모듈-로컬 분리 권고* 와 일관: 새 시스템에서 별도 도구 `coreanalyzer/` 로 분리.

---

## 4. ctp 패키지 (4 클래스) — CTP CLI 호스트

```
.ctp/
├── CTP                 ← main(String[]) — ctp.sh 가 호출
├── ComponentEnum       ← 21 task 이름
├── Version             ← 버전 표시
└── (1 더 있을 수 있음 — 정밀 후속)
```

**common src 안에 있지만 의미적으로 *완전히 다른 책임*** (라이브러리가 아니라 CLI dispatcher). 새 시스템에서 별도 entry-point 모듈로 분리 (deps-of-common.md §8).

---

## 5. grepo 패키지 (이중 위치)

### 5-1. `common/src/.../grepo/` (RMI client, 5 클래스)
```
.common.grepo/
├── EntryListener
├── GeneralEntryListener
├── RepoClient          ← RMI client
├── RepoUtil            ← utility (client 측)
└── UpgradeMain         ← main(String[]) — grepo upgrade 도구
```

### 5-2. `common/grepo/src/.../grepo/service/` (RMI server, 4 클래스)
```
.common.grepo.service/
├── EmptyCache
├── PackageInf
├── RepoServiceImpl     ← RMI server 구현
└── RepoUtil            ← server 측 (이름 충돌)
```

### 5-3. legacy nhncorp 네임스페이스 (1 클래스)
```
com.nhncorp.cubrid.common.grepo.RepoService  ← 인터페이스 (RMI stub)
```

→ build.xml 이 `com/navercorp/cubridqa/common/**` 패턴 매칭으로 5-1, 5-2 *모두* cubridqa-common.jar 에 흡수. nhncorp.* 도 명시적 include. 1 jar 에 *3 다른 origin* 이 모이는 빌드 트릭 (deps-of-common.md §3).

**해석:** grepo 는 *git 레포의 jar 산출물을 원격으로 fetch* 하는 도구로 추정 (CI 분산 환경에서 jar 배포). RMI client (모듈 측) ↔ server (별도 데몬) 분리. 새 시스템에서 *git CLI subprocess 또는 go-git 같은 모던 라이브러리* 로 대체 후보.

---

## 6. scheduler (별도 sub-module, ~30 클래스)

```
sched/src/com.navercorp.cubridqa.scheduler/
├── common/             (10 — utility, 자체 CommonUtils/Log/Constants)
│   ├── ActiveMQFactory
│   ├── MQPoolUtil
│   ├── ConsumerContext
│   ├── FileReady
│   ├── HttpUtil
│   ├── Message / SendMessage
│   └── ...
├── consumer/           (5 — message 소비)
│   ├── ConsumerAgent      ← main
│   ├── Consumer
│   ├── ConsumerTimer
│   ├── Configure
│   └── ObserverMessage
└── producer/           (10+ — message 생산 + crontab)
    ├── Main               ← main
    ├── crontab/SchedularMain  ← main
    ├── crontab/BuildMain      ← main
    ├── crontab/CUBJob / CUBJobContext
    ├── AbstractExtendedSuite / GeneralExtendedSuite
    ├── Compatibility
    ├── CompatDatabaseImage
    ├── FileItem / FileProcess / FileProcessCaller
    ├── Observer
    └── I18N
```

**역할 추정:**
- ActiveMQ broker (또는 client) + Quartz scheduler 결합
- producer 측: cron job 으로 빌드 / suite 실행 트리거
- consumer 측: 메시지 받아서 케이스 실행
- *분산 CI / 정기 회귀 인프라* 로 추정

deps-of-common.md §5: 모듈 코드는 sched 를 *직접 import 안 함*. shell/init_path/run_shell.sh 가 jar 이름으로 참조. → **옵션 컴포넌트**, 핵심 케이스 실행 경로 밖.

새 시스템에서 1차 strangler-fig 대상 *제외 가능*.

---

## 7. 클래스 간 의존 다이어그램 (메인 영역)

```
┌─────────────────────────────────────────────────┐
│  com.navercorp.cubridqa.common (메인 utility)   │
│                                                  │
│   CommonUtils ◀──── IniData ◀── ConfigParam     │
│       ▲ ▲                          Constants     │
│       │ │                                        │
│       │ └─ Log ◀── JschLogger                    │
│       │        │                                 │
│       │        └─ SSHConnect ◀── SFTP*           │
│       │                                          │
│       └─ LocalInvoker / ShellInput               │
│                                                  │
│   ShareMemory (sql 전용)                         │
│   MailSender (shell 전용)                        │
│   MakeFile (shell 전용)                          │
│   MergeTemplate / ParseActionFiles (사용처 후속) │
└──────────┬──────────────────────────────────────┘
           │ depends
           ▼
   ┌──────────────────┐    ┌─────────────────┐
   │  .coreanalyzer   │    │  .ctp           │
   │  (11 클래스)     │    │  (4 클래스)     │
   │  AnalyzerMain    │    │  CTP.main       │
   │  CommonUtil      │    │  ComponentEnum  │
   │  ...             │    │  Version        │
   └──────────────────┘    └─────────────────┘
                                   ▲
                                   │ ctp.sh 가 java -cp ... CTP
                                   │
   ┌──────────────────┐
   │  .common.grepo   │   (RMI client)
   │  (5 클래스)      │  ──RMI──▶ common/grepo/src/.../service
   │  RepoClient      │            (RMI server, 4 클래스)
   │  ...             │            RepoServiceImpl
   └──────────────────┘
```

---

## 8. 외부 라이브러리 의존 (deps-of-common.md §7 와 일관)

`common/lib/`:

```
Apache Commons:    cli-1.2, collections-3.1, dbcp-1.2.2, io-1.3, lang-2.1,
                   logging-1.1.1, logging-api-1.0.4, pool-1.6
Logging:           log4j-1.2.16 (EOL), slf4j-api-1.7.19, slf4j-log4j12-1.7.19
SSH/HTTP:          jsch-0.1.55, httpclient-4.2.5, httpcore-4.2.4
INI/XML:           ini4j-0.5.4-jdk14, dom4j-1.6.1
Mail:              mail/ (디렉터리)
DB:                cubrid_jdbc.jar
Self-built:        cubridqa-common.jar
```

`common/grepo/lib/`: jgit-4.3.1, osgi.core-4.3.0
`common/sched/lib/`: ActiveMQ 5.8 (broker/client/pool), quartz-2.2.1, geronimo-*-spec, hawtbuf-1.9

**모두 노후** (2007~2016 사이). 새 시스템에서 일괄 현대화.

---

## 9. 빌드 산출물 (deps-of-common.md §3)

```
cubridqa-common.jar (common/lib/) ←
   ├ com/navercorp/cubridqa/common/**       (20 메인 + 11 coreanalyzer + 5 grepo client)
   ├ com/navercorp/cubridqa/ctp/**          (4 클래스)
   └ com/nhncorp/cubrid/common/grepo/*      (legacy 1 클래스)

cubridqa-scheduler.jar (common/sched/lib/) ←
   └ com/navercorp/cubridqa/scheduler/**    (~30 클래스)
```

→ *3 다른 origin 이 1 jar 에 통합* (build.xml 패턴 매칭). 새 시스템에서 *명시적 모듈 분리* 권고.

---

## 10. 새 시스템 설계 시 권고 (common 관점)

deps-of-common.md §8 의 권고를 정밀화:

1. **`kit-core` 모듈** — 진정한 공통 utility:
   - CommonUtils, IniData, Log, Constants, ConfigParameterConstants, LocalInvoker, SSHConnect, SFTP*, RunRemoteScript, ShellInput, MergeTemplate, ParseActionFiles
   - 의존: 새 언어 표준 라이브러리 + 모던 SSH

2. **`coreanalyzer` 별도 도구** — sql/isolation 의 부수 도구:
   - AnalyzerMain / IssueMain 은 별도 binary
   - 새 시스템 케이스 실행 경로와 *느슨한 결합* (subprocess 호출만)

3. **`cli` 새 entry-point 모듈** — ctp 패키지 분리:
   - CTP / ComponentEnum / Version → 새 시스템의 main + dispatcher
   - 모듈 dispatch 패턴은 *plug-in* (3가지 호출 모델 통일 — cli-tree.md §3)

4. **`grepo` 옵션 git 통합 도구** — 1차 제외:
   - 새 시스템에서는 git CLI subprocess 또는 go-git 라이브러리
   - RMI 서비스 모델 폐기 ADR

5. **`scheduler` 옵션 컴포넌트** — 1차 제외:
   - ActiveMQ + Quartz 의존 무거움. 분산 CI 가 *외부 도구* (예: GitHub Actions / Jenkins) 로 대체 가능
   - 폐기 또는 별도 서브 프로젝트

6. **모듈-로컬 헬퍼 흡수**:
   - ShareMemory → sql/
   - MailSender → shell/ (또는 *공통 알림 모듈*)
   - MakeFile → shell/

7. **shell.common.SSHConnect 와의 통합** — M0 #4 의 큰 발견:
   - 두 SSHConnect 의 *기능 차이* 정밀 분석 후 통합
   - 새 시스템에서 단일 SSH 클라이언트만 유지

---

## 11. 미해결 / 후속 분석

- `MergeTemplate` / `ParseActionFiles` / `CommitConfigFileIntoDB` / `SparseFsMain` 정확한 사용처
- ConfigParameterConstants 82 키와 conf-matrix.md 81 키의 1 차이
- coreanalyzer 의 stack digest 알고리즘
- grepo RMI 가 실제 운영에서 활성인지 (현재 사용 흔적이 build.xml 외에 거의 없음)
- scheduler 의 실제 사용 (cron 으로 어떤 파이프라인을 트리거하는가)

이들은 ADR 결정 후 Phase 1 진입 시 우선순위 따라 처리.
