# Common Subdirectory Dependencies

**Source:** `cubrid-testtools/CTP/common/` (8 하위 디렉터리)

**Phase 0 M0 status:** 완료

목적: `common/` 안의 8개 하위가 서로 어떻게 의존하는지, 그리고 외부 모듈(sql/shell/isolation/...)이 common의 어느 영역에 의존하는지 그래프로 정리. 새 시스템 설계 시 어디부터 분해/대체할 수 있는지의 근거가 됨.

---

## 1. 8 하위 디렉터리 — 정체성

| 디렉터리 | 종류 | 핵심 내용 | 산출물 |
|----------|------|-----------|--------|
| `src/` | Java 소스 (메인) | 공통 유틸 20 파일 + coreanalyzer 11 + grepo client 5 + ctp 4 + nhncorp grepo legacy 1 | `common/lib/cubridqa-common.jar` |
| `lib/` | 외부 라이브러리 + 빌드 산출물 | Apache Commons (cli/io/lang/dbcp/pool/...), log4j, slf4j, jsch, ini4j, dom4j, httpcore, mail, cubrid_jdbc.jar, **cubridqa-common.jar (자기 src 산출)** | (입력 + 출력 혼재) |
| `sched/` | 독립 서브모듈 (Java) | ActiveMQ + Quartz 기반 분산 스케줄러 (producer/consumer/agent) | `common/sched/lib/cubridqa-scheduler.jar` |
| `grepo/` | 독립 서브모듈 (Java) | jgit/osgi 기반 git 레포 서비스. `service.RepoServiceImpl` 등 5 클래스 | (잠재적으로 cubridqa-common.jar 안에 머지됨 — 아래 §3 참조) |
| `script/` | 셸 스크립트 헬퍼 (다수) | analyze_failure, run_coverage_collect_and_upload, file_core_issue, run_grepo_fetch 등 운영성 헬퍼 | (해석 시점 실행) |
| `ext/` | 셸 스크립트 (모듈별 외부 진입) | run_sql.sh / run_shell.sh / run_jdbc.sh / run_isolation.sh / run_ha_repl.sh / run_cdc_repl.sh / run_unittest.sh / run_coverage.sh / run_compat_{cci,jdbc}.sh / run_sql_by_cci.sh | (해석 시점 실행) |
| `tpl/` | 텍스트 템플릿 | issue_create.tpl, issue_create_desc.tpl, issue_comment.tpl | (script가 합치는 입력) |
| `gcov/` | 외부 바이너리 | `gcov/gcov` 단일 실행 파일 (커버리지 도구) | (script가 호출하는 입력) |

---

## 2. 내부 의존 그래프 (within common/)

```
                 ┌──────────────────────────────────────────────────┐
                 │                  common/lib/                     │
                 │  Apache Commons, log4j, jsch, ini4j, dom4j, ...  │
                 │  cubrid_jdbc.jar  +  cubridqa-common.jar (self)  │
                 └────────────────────────▲─────────────────────────┘
                                          │ (compile/runtime cp)
                                          │
              ┌───────────────────────────┴───────────────────────┐
              │                  common/src/                      │
              │  ┌──────────────────────────────────────────────┐ │
              │  │  com.navercorp.cubridqa.common (20 files)    │ │
              │  │   CommonUtils, IniData, LocalInvoker,        │ │
              │  │   SSHConnect, SFTPDownload/Upload, Log,      │ │
              │  │   MailSender, MakeFile, Constants,           │ │
              │  │   ConfigParameterConstants, ParseActionFiles,│ │
              │  │   ShareMemory, SparseFsMain, JschLogger, ... │ │
              │  └──────────────────────────────────────────────┘ │
              │  ┌──────────────────────────────────────────────┐ │
              │  │  .common.coreanalyzer (11 files)             │ │
              │  │   AnalyzerMain, CommonUtil(별도)             │ │
              │  └──────────────────────────────────────────────┘ │
              │  ┌──────────────────────────────────────────────┐ │
              │  │  .common.grepo (5 files, 구 client)          │ │
              │  │   imports CommonUtils + nhncorp.grepo (RMI)  │ │
              │  └──────────────────────────────────────────────┘ │
              │  ┌──────────────────────────────────────────────┐ │
              │  │  .ctp (4 files)                              │ │
              │  │   CTP, ComponentEnum, Version, ...           │ │
              │  │   imports common.{CommonUtils,IniData,       │ │
              │  │                    LocalInvoker}             │ │
              │  └──────────────────────────────────────────────┘ │
              │  ┌──────────────────────────────────────────────┐ │
              │  │  com.nhncorp.cubrid.common.grepo (1 file,    │ │
              │  │  legacy RMI server-side stub)                │ │
              │  └──────────────────────────────────────────────┘ │
              └────────────────────────▲────────────▲─────────────┘
                                       │            │
                                       │            │
            ┌──────────────────────────┘            └────────────┐
            │                                                    │
   ┌────────┴────────┐                            ┌──────────────┴───────────┐
   │  common/grepo/  │                            │     common/sched/        │
   │  src + lib      │  (자체 컴파일,             │  src + lib (ActiveMQ,    │
   │  (jgit, osgi)   │   common.jar에 머지)       │   Quartz, geronimo)      │
   │  pkg:           │                            │  pkg:                    │
   │  c.n.c.common.  │                            │  c.n.c.scheduler.*       │
   │  grepo.service  │                            │  → cubridqa-scheduler    │
   └─────────────────┘                            │       .jar (별도 산출)   │
                                                  └──────────────────────────┘

   ┌─────────────────┐    ┌──────────────────┐   ┌──────────────────────┐
   │  common/script/ │───►│  common/tpl/     │   │  common/gcov/gcov    │
   │  analyze_failure│    │  issue_*.tpl     │   │  (외부 바이너리)     │
   │  run_coverage_* │───────────────────────────►                      │
   │  ...            │                            └──────────────────────┘
   └─────────────────┘

   ┌─────────────────┐
   │  common/ext/    │   (외부 진입 셸 스크립트, 자체 완결)
   │  run_<module>   │   ──── 다른 common 하위에 직접 의존하지 않음
   │  .sh            │       (CTP 안 어디서도 grep으로 호출이 잡히지 않음
   │                 │        → 외부 CI/수동 실행에서 호출 추정)
   └─────────────────┘
```

### 핵심 관찰
- **`src/` 가 모든 의존의 종착점.** `lib/`의 외부 라이브러리에 의존하고, `grepo/`와 `sched/`는 src와 같은 패키지 네임스페이스(`com.navercorp.cubridqa.common.*`, `com.navercorp.cubridqa.scheduler.*`)에 속하지만 빌드 단위는 분리되어 있다.
- **`script/` 와 `tpl/`, `gcov/` 는 해석 시점에 결합** — Java 코드와는 무관. CI 운영 헬퍼.
- **`ext/` 는 외딴 섬** — 다른 common 하위와 코드 의존이 없고, CTP/ 트리 안 어디에서도 호출이 grep되지 않음. 외부 사용자가 모듈별 실행을 위한 공식 셸 진입점으로 추정. (M0 후속 조사 항목)

---

## 3. `grepo/` 와 `src/.common/grepo/` 의 이중 위치 — 빌드 트릭

build.xml 의 `cubridqa-common.jar` 타겟은:
```
<include name="com/navercorp/cubridqa/common/**/*.class" />
<include name="com/navercorp/cubridqa/ctp/**/*.class" />
<include name="com/nhncorp/cubrid/common/grepo/*.class" />
```

문자열 패턴이 **컴파일 산출 디렉터리(`${build}`) 안에서 패키지 경로로 매칭**되므로, 같은 패키지 prefix를 가진 두 소스 위치(`common/src/.../grepo/` 와 `common/grepo/src/.../grepo/service/`)가 모두 cubridqa-common.jar에 합쳐진다.

즉 `grepo/` 디렉터리는 *논리적으로는* 별도 서브모듈처럼 보이지만, *물리적 산출물* 측면에서는 cubridqa-common.jar 의 일부. 새 시스템 분리 시 이 두 위치를 합치거나 명시적으로 별 jar로 분리하는 결정이 필요하다.

---

## 4. 외부 모듈 → common 의존 방향

각 모듈이 import 하는 common Java 심볼:

| 모듈 | import 수 | 핵심 import |
|------|-----------|-------------|
| `shell` | 7 | CommonUtils, ConfigParameterConstants, Constants, LocalInvoker, Log, MailSender, MakeFile |
| `isolation` | 4 | CommonUtils, ConfigParameterConstants, **coreanalyzer.CommonUtil**, Log |
| `ha_repl` | 5 | CommonUtils, ConfigParameterConstants, Constants, LocalInvoker, Log |
| `cdc_repl` | 5 | CommonUtils, ConfigParameterConstants, Constants, LocalInvoker, Log |
| `sql` | 4 | CommonUtils, **coreanalyzer.AnalyzerMain**, IniData, **ShareMemory** |
| `jdbc` | (Java entry 없음, 셸 지배) | — (cubridqa-common.jar 가 classpath에 있는지는 jdbc/bin/run.sh 분석 시점에) |
| `sql_by_cci` | (sql 경로 공유) | sql과 동일 |

**의존 hot-spot (5+ 모듈이 import):**
- `CommonUtils` — 7/7 (path 변환, 환경변수, 헬퍼 다용도)
- `ConfigParameterConstants` — 5/7
- `Log` — 5/7

**의존 cold-spot (1~2 모듈만):**
- `coreanalyzer.*` — sql/isolation 만 (코어 덤프 분석)
- `ShareMemory` — sql 만 (Java→native shared memory IPC)
- `MailSender`, `MakeFile` — shell 만

**중요한 발견 (cli-tree와 합쳐서):**
- `ctp` 패키지(CTP, ComponentEnum)는 cubridqa-common.jar 안에 함께 패키지됨. 즉 ctp.sh 의 메인 클래스는 *common 모듈이 보유*. 새 시스템에서 entry-point와 공통 유틸을 어떻게 분리할지의 ADR 후보.
- coreanalyzer 는 common에 살지만 사용하는 모듈은 sql/isolation 둘뿐 → strangler-fig 1차 분리 후보.
- `ShareMemory` 는 sql 전용 — sql 모듈로 옮기는 게 자연스러움.

---

## 5. `sched/` 의 사용 흔적

build.xml은 `cubridqa-scheduler.jar`를 별도 산출. Java import 측면으로는 `com.navercorp.cubridqa.scheduler.*` 를 import 하는 모듈 코드가 **없음**. 하지만 `shell/init_path/run_shell.sh` 와 build.xml이 jar 이름을 참조 → 셸 레벨에서 동적으로 (java -cp ... -classpath 등) 실행될 가능성. 또 `sched/init.sh` 가 별도로 있어 외부 데몬 형태로 동작하는 것으로 추정.

새 시스템 설계 의미: scheduler는 사실상 *옵션 컴포넌트*. 핵심 테스트 실행 경로에는 직접 의존이 없으므로, 1차 strangler-fig 대상에 포함하지 않아도 무방.

---

## 6. `gcov/`, `script/`, `tpl/`, `ext/` — 해석 시점 의존만

- `script/run_coverage_collect_and_upload` → `gcov/gcov` 바이너리 호출
- `script/analyze_failure.sh` → `tpl/issue_*.tpl` 텍스트 머지
- `ext/run_<module>.sh` → 어떤 다른 common 하위도 호출하지 않음 (확인됨). 외부 CI에서 직접 호출되는 entry shell scripts.

이들은 Java 소스/빌드와 결합도 가 0. 새 시스템에서 별도 "ops/" 트리로 옮기거나, 더 작은 운영 도구 모음으로 재구성 가능.

---

## 7. 외부 의존 (third-party JAR) 정리

`common/lib/` 의 jar 분류:

| 카테고리 | jar |
|----------|-----|
| Apache Commons | commons-cli-1.2, commons-collections-3.1, commons-dbcp-1.2.2, commons-io-1.3, commons-lang-2.1, commons-logging-1.1.1, commons-logging-api-1.0.4, commons-pool-1.6 |
| 로깅 | log4j-1.2.16, slf4j-api-1.7.19, slf4j-log4j12-1.7.19 |
| 네트워크/SSH | jsch-0.1.55, httpclient-4.2.5, httpcore-4.2.4 |
| INI/XML | ini4j-0.5.4-jdk14, dom4j-1.6.1 |
| Mail | mail (디렉터리) |
| DB driver | cubrid_jdbc.jar |
| 자체 산출 | cubridqa-common.jar |

**버전 노화 주목:** commons-cli 1.2 (2009), log4j 1.2.16 (2010), commons-io 1.3 (2007), dom4j 1.6.1 (2005). 새 시스템에서는 이 라이브러리들의 모던 버전(또는 대체)을 ADR로 결정. 보안 측면에서도 log4j 1.2 는 EOL.

`grepo/lib/` 의 jgit 4.3 (2016), osgi.core 4.3 (2011) — 마찬가지로 노후.

`sched/lib/` 의 ActiveMQ 5.8 (2013), Quartz 2.2.1 (2014) — 동일.

**ADR 후보:** *common 라이브러리 현대화* (Phase 1 또는 Phase 2 진입 시).

---

## 8. 새 시스템 설계 시 권고

1. **`common/src/` 의 분해 후보**:
   - `coreanalyzer/` → 코어 덤프 분석 도구로 분리 (sql/isolation 만 사용)
   - `ShareMemory` → sql 모듈 안으로 흡수 (sql 전용)
   - `ctp` 패키지 → 새 시스템에서는 entry-point 모듈을 별도로 (e.g. `cli/`)
   - 나머지 (CommonUtils, IniData, LocalInvoker, Log, SSH/SFTP) → 진정한 공통 utility 코어

2. **`grepo/` 의 이중 위치는 통합 또는 명시 분리**. 빌드 패턴 매칭에 의존하는 현 구조는 새 시스템에서는 의도한 행동인지 ADR로 명시.

3. **`sched/` 는 옵션 모듈**로 둘 수 있음. 스케줄러 없이도 모든 모듈이 단독 실행 가능하므로, 1차 strangler-fig 대상에서 제외 가능.

4. **`ext/` 의 외부 호출자 발견**은 후속 조사 — Phase 0 의 `_overview/case-formats.md` 또는 `inventory/` 작업 진행 중에 외부 CI 스크립트 위치를 확인할 것.

5. **third-party 라이브러리 노후도** 가 심각. 새 시스템은 라이브러리 버전 정책을 ADR로 명시하고, 그에 맞춰 빌드 도구(ADR-002) 결정도 함께 정렬되어야 함.

6. **모듈 → common 의존 표면이 좁음** (대다수 모듈이 5개 이하 심볼만 import). 이는 strangler-fig에 우호적: common을 통째로 옮기지 않고, 모듈이 실제로 의존하는 좁은 인터페이스(`CommonUtils`, `Log`, `Config*Constants`, `LocalInvoker`)만 새 시스템 동등물로 제공해도 모듈 단위 대체가 가능.

---

## ROADMAP 갱신

체크리스트의 `analysis/_overview/deps-of-common.md` 항목을 완료(`- [x]`)로 갱신.

**M0 #4 출발점 (모듈별 Main 시퀀스 분석 시 입력으로 활용):**
- shell.main.Main: common 7개 import (CommonUtils/Config*/Constants/LocalInvoker/Log/MailSender/MakeFile)
- isolation.Main: common 4개 import (CommonUtils/Config*/coreanalyzer.CommonUtil/Log)
- ha_repl.Main / cdc_repl.Main: common 5개 import (Common/Config*/Constants/LocalInvoker/Log)
- sql/medium: Java entry 없음 — `sql/bin/run.sh` 27KB 가 entry. coreanalyzer.AnalyzerMain 과 IniData/ShareMemory 만 자바 측 호출.
