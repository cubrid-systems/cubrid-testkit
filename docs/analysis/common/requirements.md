# common — Requirements (라이브러리 + 부수 도구 + 옵션 데몬)

**Source:** `cubrid-testtools/CTP/common/`

**Companion docs:** `design.md` (src 구조), `io-contract.md` (Java API + 셸 자산), `implementation-notes.md`, `test-corpus.md` (라이브러리이므로 *부재* 의 의미)

**상위 분석:** `analysis/_overview/deps-of-common.md` (8 하위 디렉터리 의존 그래프) — 본 폴더의 5 문서는 deps-of-common.md 의 *모듈 내부* 정밀 분석.

---

## 1. common 이 해결하는 문제

CTP 의 다른 모듈 (sql / shell / isolation / ha_repl / cdc_repl / sql_by_cci) 이 *공통으로 필요한 인프라* 를 제공한다. **모듈이 아니라 라이브러리 + 부수 도구 모음** 이다.

세 가지 다른 정체성을 한 디렉터리 안에 가진다:

1. **공유 라이브러리** (`src/com/navercorp/cubridqa/common/*`) — 20+ Java 유틸 클래스 (CommonUtils / IniData / SSHConnect / SFTP / Log / ...). 다른 모듈이 import 한다.
2. **CTP 진입점** (`src/com/navercorp/cubridqa/ctp/*`) — CTP.java + ComponentEnum + Version. ctp.sh 가 호출하는 main class 가 여기 산다.
3. **부수 sub-module** —
   - `coreanalyzer/` (11 클래스) — core dump 분석 (sql/isolation 만 사용)
   - `common/grepo/` + `grepo/src/` (5+4 클래스) — git 레포 서비스 (RMI client + server)
   - `sched/` (분리된 scheduler 서브모듈, ActiveMQ + Quartz)

추가로 *비-Java 자산* 다수:
- `script/` — 셸 헬퍼 (analyze_failure / run_coverage_collect_and_upload / run_grepo_fetch / file_core_issue / ...)
- `tpl/` — issue / feedback 템플릿
- `gcov/` — 외부 바이너리 (커버리지)
- `ext/` — 모듈별 외부 진입 셸 스크립트 (run_sql.sh / run_shell.sh / ...)
- `lib/` — 외부 third-party JAR + 자체 산출 cubridqa-common.jar

---

## 2. 외부 호출 형태 — 4가지

common 은 "호출되는" 측이 아닌 "제공하는" 측이지만 다음 4가지 진입점을 갖는다:

### 2-1. 라이브러리 import (다른 모듈)

```java
// shell, isolation, ha_repl, cdc_repl, sql 등이 다음을 import
import com.navercorp.cubridqa.common.CommonUtils;
import com.navercorp.cubridqa.common.IniData;
import com.navercorp.cubridqa.common.ConfigParameterConstants;
import com.navercorp.cubridqa.common.Constants;
import com.navercorp.cubridqa.common.LocalInvoker;
import com.navercorp.cubridqa.common.Log;
import com.navercorp.cubridqa.common.coreanalyzer.AnalyzerMain;  // sql만
import com.navercorp.cubridqa.common.coreanalyzer.CommonUtil;    // isolation만
import com.navercorp.cubridqa.common.ShareMemory;                // sql만
import com.navercorp.cubridqa.common.MailSender;                 // shell만
import com.navercorp.cubridqa.common.MakeFile;                   // shell만
```

(deps-of-common.md §4 의 모듈→common 의존 표 참조)

### 2-2. CTP.main 진입 (ctp.sh 가 invoke)

```
java -cp common/lib/cubridqa-common.jar com.navercorp.cubridqa.ctp.CTP "$@"
```

CTP.java 와 ComponentEnum 은 *common src 에 있지만 패키징은 cubridqa-common.jar 에 함께* — common 의 4번째 정체성: **CLI 라우터 호스트**.

### 2-3. 부수 도구의 main()

다음 클래스들이 자체 `public static void main(String[] args)` 보유 — 셸 / CI 가 직접 java 호출:
- `common.SparseFsMain` (사용처 후속)
- `common.CommitConfigFileIntoDB` (사용처 후속)
- `common.CommonUtils.main` (테스트/유틸 추정)
- `common.coreanalyzer.AnalyzerMain` / `IssueMain`
- `common.grepo.UpgradeMain`
- `scheduler.producer.Main`
- `scheduler.producer.crontab.SchedularMain` / `BuildMain`
- `scheduler.consumer.ConsumerAgent`
- ...

### 2-4. RMI 서버 / 데몬 (옵션)

- `common.grepo.service.RepoServiceImpl` — git 레포 서비스 RMI 데몬 (옵션)
- `scheduler.*` ActiveMQ + Quartz 기반 분산 scheduler (옵션, deps-of-common.md §5)

이들은 *케이스 실행에 필수가 아닌 옵션 운영 컴포넌트*. 1차 strangler-fig 대상에서 제외 가능.

---

## 3. 사용자 요구사항 (모듈 측의 "공통 인프라가 제공해야 하는 것")

다른 모듈이 common 에 기대하는 표면을 정리:

### 3-1. 필수 코어 (모든 모듈)
- **path utilities** — Linux / Windows / Cygwin 경로 변환 (CommonUtils.getLinuxStylePath / getWindowsStylePath / getFixedPath)
- **INI parser** — `default.cubrid.<property>` 같은 dot-notation 와일드카드 매칭 (IniData)
- **conf 파라미터 상수** — 82개 키를 ConfigParameterConstants 로 type-safe 표현
- **로깅** — Log (파일 로거)
- **로컬 프로세스 실행** — LocalInvoker.exec(...)
- **boolean / 시간 / 문자열 utility** — CommonUtils.convertBoolean, dateToString, getCurrentTimeStamp

### 3-2. 분산 / 원격 (대부분 모듈)
- **SSH 클라이언트** — SSHConnect (jsch 래퍼) — *단, isolation/ha_repl/cdc_repl/shell 은 shell.common.SSHConnect 를 더 많이 사용*
- **SFTP** — SFTPDownload / SFTPUpload / SFTP (파일 전송)
- **JschLogger** — jsch 로깅 어댑터
- **RunRemoteScript** — 원격 스크립트 실행 헬퍼

### 3-3. 모듈 한정 헬퍼
- **ShareMemory** (sql 전용) — Java NIO MappedByteBuffer 기반 IPC. cqt 가 native 와 동기화 시 사용
- **MailSender** (shell 전용) — 이메일 통보
- **MakeFile** (shell 전용) — 케이스 빌드용 Makefile 생성 추정
- **MergeTemplate** (사용처 후속) — 템플릿 머지
- **ParseActionFiles** (사용처 후속) — action 파일 파서
- **CommitConfigFileIntoDB** (사용처 후속) — config 를 DB 에 적재 (어떤 DB?)

### 3-4. coreanalyzer 한정 (sql / isolation 만)
- **AnalyzerMain** (sql) — core dump 분석
- **CommonUtil** (isolation) — coreanalyzer 의 분리된 utility (이름 충돌)

### 3-5. ctp 패키지 (CTP CLI)
- **CTP.main** — ctp.sh 가 호출하는 진짜 진입점
- **ComponentEnum** — 21개 task 이름
- **Version** — 빌드 버전 표시

---

## 4. 비기능 요구

| 항목 | 현재 동작 | 새 시스템에서의 의미 |
|------|----------|---------------------|
| 패키지 prefix 매칭 빌드 | build.xml 의 `com/navercorp/cubridqa/common/**` 패턴 — common/grepo/src 를 *자동 흡수* | 빌드 트릭 명시화 또는 명시적 모듈 분리 |
| 노후 third-party | log4j 1.2 / commons-cli 1.2 / dom4j 1.6 / jsch 0.1.55 / ini4j 0.5.4 | 라이브러리 현대화 (deps-of-common.md §7) |
| jsch 의존 | SSHConnect / JschLogger | 모던 SSH 라이브러리 / 새 시스템 native (Go: x/crypto/ssh) |
| ini4j 의존 | IniData 가 래핑 | 새 conf 포맷 (TOML / YAML) ADR 후보 |
| RMI 의존 | grepo + scheduler | 옵션 컴포넌트, 1차 대체 제외 |
| coreanalyzer 위치 | common 안 (sql/isolation 사용) | 모듈-로컬 또는 별도 도구 (deps-of-common.md §8) |
| ShareMemory 의 native 가정 | Java NIO MappedByteBuffer | 새 언어에서 동등 IPC 필요 (mmap / 별 RPC) |
| ctp 패키지 위치 | common 안 | entry-point 분리 ADR (deps-of-common.md §8) |

---

## 5. 의존하는 외부 자원

(deps-of-common.md §7 참조 — 외부 third-party JAR)

새 시스템 입력:
- jsch → modern SSH library
- ini4j → modern conf parser (또는 자체 구현)
- log4j 1.2 → log4j2 / slf4j (Java) 또는 새 언어 native logger
- Apache Commons → 새 언어 표준 라이브러리
- dom4j → 새 XML 라이브러리 (또는 XML 의존 제거)
- jgit → git2go / go-git / git CLI subprocess
- ActiveMQ + Quartz → modern scheduler (또는 옵션 컴포넌트로 폐기)

---

## 6. 새 시스템 설계 입력 (요약)

common 영역의 *분해* 가 새 시스템 설계의 핵심 작업 중 하나 (deps-of-common.md §8):

1. ✅ **진정한 공통 utility 코어** (CommonUtils / IniData / Log / Constants / ConfigParameterConstants / LocalInvoker / SSHConnect / SFTP) → 별도 모듈 (가칭 `kit-core`)
2. ✅ **coreanalyzer** → 모듈-로컬 (sql/isolation 별도 도구)
3. ✅ **ShareMemory** → sql 모듈 흡수 (sql 전용)
4. ✅ **ctp 패키지** → 새 entry-point 모듈 (가칭 `cli`)
5. ✅ **grepo** → 옵션 git 통합 도구 (1차 제외)
6. ✅ **scheduler** → 옵션 (1차 제외)
7. ✅ **MailSender / MakeFile / MergeTemplate / ParseActionFiles** → 사용처 분석 후 모듈-로컬 또는 폐기
8. ✅ **script/ + tpl/ + gcov/** → ops/ 트리로 이동 (Java 와 무관한 운영 도구)
9. ✅ **ext/** → ops/ 또는 폐기 (외부 호출자 식별 후)

ADR 후보:
- 라이브러리 현대화 정책 (log4j / commons / dom4j / jgit 등) — Phase 1/2 진입 시 단번에 결정
- common/grepo 이중 위치 통합 (deps-of-common.md §3)
- ini4j → 새 conf 포맷 (또는 호환 유지)
- jsch → 모던 SSH 라이브러리
- shell.common.SSHConnect 와 common.SSHConnect 의 *통합 또는 분리* (M0 #4 발견)
