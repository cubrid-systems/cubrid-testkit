# common — Test Corpus (라이브러리이므로 *부재*)

**Source:** `cubrid-testtools/CTP/common/`

common 은 *케이스를 실행하는 모듈* 이 아니라 *라이브러리 + 부수 도구* 다. 따라서 testcases 레포의 *직접 대응 디렉터리* 가 없다.

본 문서는 (1) 부재의 *의미* 와 (2) common 의 *실질적 검증 방식* 과 (3) 새 시스템에서 *common 자체의 테스트 전략 권고* 를 다룬다.

---

## 1. testcases 레포에 common 디렉터리는 없다

```
cubrid-testcases/             — sql, medium, isolation, tool
cubrid-testcases-private-ex/  — shell, shell_heavy, shell_perf, scripts
cubrid-testcases-private/     — HA, interface, longcase, manually,
                                 random_query_generator, shell_ext
```

→ **공식 testcases 트리 어디에도 `common/` 가 없음**.

이는 의도적이다 — common 은 *케이스 실행 인프라* 이지 *케이스 실행 대상* 이 아님. CommonUtils / IniData / SSHConnect 가 *제대로 동작하는지* 는 *모듈이 케이스를 실행할 때* 함께 검증된다.

---

## 2. common 의 실질적 검증 채널

### 2-1. 모듈 케이스 실행을 통한 *간접 검증*

다른 모듈이 common 을 import 하면서 사용. 모듈 케이스가 정상 동작하면 common 의 해당 기능이 *간접적으로 검증* 됨:

- sql 케이스 17,411 개가 IniData / CommonUtils.parsePropertiesByPrefix 를 사용 → conf 파싱 검증
- isolation 케이스 6,778 개가 SSHConnect / Log / Constants 를 사용 → SSH/로깅 검증
- shell 케이스 3,658 개가 LocalInvoker / MailSender / MakeFile 사용 → 로컬 실행/통보 검증
- ha_repl/cdc_repl 케이스가 multi-instance conf 파싱 + role 매핑 (parseInstanceParametersByRole) 사용 → role-based 토폴로지 처리 검증

→ **사실상 회귀 검증** 이지만 *단위 테스트는 아님*. common 의 *경계 조건 / 엣지 케이스* 는 모듈 케이스가 우연히 cover 하지 않으면 미검증.

### 2-2. 자체 main() 보유 클래스의 *수동 검증*

common 안 다음 클래스가 자체 `main(String[])` 보유 (design.md / implementation-notes.md §12):
- `CommonUtils` (테스트/유틸 추정)
- `SparseFsMain`
- `CommitConfigFileIntoDB`
- `SFTPDownload`, `SFTPUpload`, `SFTP`, `RunRemoteScript`
- `coreanalyzer.AnalyzerMain`, `coreanalyzer.IssueMain`
- `common.grepo.UpgradeMain`
- `scheduler.producer.Main`, `scheduler.producer.crontab.SchedularMain`, `BuildMain`
- `scheduler.consumer.ConsumerAgent`

→ 이들은 *수동 / 외부 CI 호출* 을 통해 검증. *자동화된 테스트 스위트 부재* 추정.

### 2-3. JUnit 등 단위 테스트 *부재*

`common/lib/` 에 jUnit / TestNG / 어떤 단위 테스트 프레임워크 jar 가 *없음* (deps-of-common.md §7 참조). build.xml 에 `<test>` 또는 `<junit>` 타겟 *없음*.

→ **common 의 단위 테스트 인프라 자체가 없다**. 모든 검증이 *모듈 케이스 실행* 또는 *수동* 이다.

---

## 3. 새 시스템에서 common 의 *후계자* 의 테스트 전략

deps-of-common.md §8 의 권고 (kit-core / coreanalyzer / cli / grepo / scheduler 분리) 를 따르면, 각 분리된 모듈은 *자체 unit test* 를 가져야 한다:

### 3-1. `kit-core` (가장 critical)

CommonUtils / IniData / Log / SSHConnect / SFTP / LocalInvoker — *모든 모듈이 의존* 하므로 단위 테스트 필수.

권고:
- **path utility** — Linux/Windows/Cygwin 경로 변환 시나리오 테스트 (각 OS 의 의도된 입력/출력 50+ 케이스)
- **IniData** — section + dot-notation prefix 매칭 동작의 정밀 단위 테스트. 기존 conf 81 키를 *fixture* 로 사용해서 회귀 보장
- **CommonUtils.parsePropertiesByPrefix / parseInstanceParametersByRole** — conf-matrix.md 의 multi-instance 패턴이 정확히 파싱되는지 검증
- **SSHConnect / SFTP** — *mock SSH server* 에 대해 단위 테스트 (예: Apache MINA SSHD 같은 in-process server)
- **boolean / time / build URL** — 엣지 케이스 (빈 문자열 / null / Windows path 등)

### 3-2. `coreanalyzer`

- core dump 입력 fixture 다수 (실제 cubrid core 일부 + 합성 core)
- stack digest 알고리즘 동등성 검증 (구 시스템 출력과 비교)

### 3-3. `cli` (CTP CLI 후계자)

- 모든 21 task name (orphan 7 포함) 입력 → 적절한 dispatch / no-op
- `-c <conf>` / `--interactive` / `-h` / `-v` 옵션
- multi-task 호출 (`ctp.sh sql medium`)
- conf 파일 fixture 로 통합 테스트

### 3-4. `grepo` (옵션, 1차 제외)

- git CLI subprocess 또는 go-git 사용 시 *integration test* 위주 (실제 git repo 대상)

### 3-5. `scheduler` (옵션, 1차 제외)

- 폐기 권고이므로 신규 테스트 작성 우선순위 낮음

---

## 4. common 자산의 *외부 사용처 검증* — 필수 후속 분석

common 의 셸 자산 (script/, ext/, tpl/) 의 *외부 호출자* 가 누구인지 모름:

```
script/analyze_failure.sh        — 누가 호출? (CI 일정?)
script/run_coverage_collect_and_upload  — gcov/gcov 실행 + 업로드 — 어디로?
script/run_grepo_fetch           — 어떤 시점에?
ext/run_<module>.sh              — 누가 호출? (외부 CI / wrapper / docker?)
```

이들의 사용처를 확인 못 하면 *동결 표면 미파악* 상태가 됨. 새 시스템 마이그레이션 시 *외부 의존 깨짐* 위험.

→ Phase 0 → Phase 1 전이 게이트 *전에* `script/` 와 `ext/` 의 외부 호출자 식별이 권고됨. CI 레포 / 운영자 인터뷰 필요.

---

## 5. test-corpus 의미 정리 (기존 stub 대체)

| 속성 | 값 |
|------|----|
| testcases 디렉터리 | **없음** (의도적) |
| 케이스 형식 | N/A (라이브러리) |
| 단위 테스트 | **부재** (JUnit 등 없음) |
| 검증 채널 | 모듈 케이스 실행을 통한 간접 + 수동 main() |
| 새 시스템 권고 | 모듈 분리 후 *각 모듈에 자체 unit test* |

---

## 6. ROADMAP 갱신

본 문서는 case-formats.md / 모듈 test-corpus.md 와 다른 형태:
- 모듈 test-corpus 는 *케이스 입력 형식* 정리
- common test-corpus 는 *부재 의 의미 + 새 시스템 테스트 전략 권고*

Phase 0 → Phase 1 전이 시 ADR 후보:
- common 후계자 모듈들의 *unit test 정책* (ADR-006? 새 ADR 번호 부여)
- script/ + ext/ 의 외부 사용처 inventory 결과에 따른 동결/폐기 결정 (ADR-007?)
