# Conf Matrix — `conf/*.conf` 키 교집합/차집합

**Source:** `cubrid-testtools/CTP/conf/` (13개 `.conf` 파일)

**Phase 0 M0 status:** 완료

추출 방식: 각 `.conf` 파일에서 `key=value` 형태(주석 처리된 템플릿 라인 포함)를 모두 키 후보로 간주. 주석 템플릿도 "문서화된 스키마"로 보고 매트릭스에 포함. 인용/괄호/공백을 포함한 비-키 토큰은 필터링.

**총 고유 키:** 81개  
**파일별 키 수:** cdc_repl 21 / ha_repl 21 / ha_shell 28 / isolation 27 / jdbc 6 / medium 21 / medium_dev 22 / sample 11 / shell 26 / shell_ci 42 / sql 25 / sql_by_cci 23 / webconsole 2

---

## 1. 핵심 발견

### 1-1. 모든 파일에 공통인 키는 거의 없다

| 공유 정도 | 키 수 |
|-----------|-------|
| 12/13 | 1 (`scenario`) |
| 6/13 | 17 |
| 5/13 | 5 |
| 4/13 | 22 |
| 3/13 | 1 |
| 2/13 | 10 |
| 1/13 (단일) | 25 |

`scenario` 만이 거의 모든 conf에 공통(webconsole 제외). 나머지는 모두 모듈 군집(cluster) 단위로만 공유된다.

### 1-2. 3개의 자연스러운 cluster

페어와이즈 공통 키 수를 보면 conf 파일들은 명확히 3개 군집으로 갈라진다:

| Cluster | 멤버 | 군집 내 공통 키 (대략) |
|---------|------|------------------------|
| **A. SQL/data 계열** | sql, sql_by_cci, medium, medium_dev, sample, jdbc | 20~23 키 (sample은 9~10) |
| **B. Shell/process 계열** | shell, shell_ci, isolation, ha_shell | 26 키 (shell_ci는 +14) |
| **C. Replication 계열** | cdc_repl, ha_repl | 20 키 |
| **D. 단독 utility** | webconsole | 2 키 (다른 cluster와 0 교집합) |

**Cluster A↔B 교집합 = 1 키 (`scenario`).** Cluster A↔C도 1, B↔C는 13 (shell ↔ ha/cdc 공통: SSH/instance 정의). webconsole은 모든 cluster와 0.

### 1-3. shell_ci.conf 가 명백히 CI 전용 슈퍼셋

shell_ci는 shell의 모든 키 + **16개 CI 전용 키**(test_platform, test_continue_yn, testcase_git_branch, testcase_update_yn, testcase_exclude_by_macro, feedback_type, default.broker*.{APPL_SERVER_SHM_ID,BROKER_PORT}, default.cubrid.cubrid_port_id, default.ha.ha_port_id, delete_testcase_after_each_execution_yn, enable_check_disk_space_yn). CI 환경(자동화/포트 충돌 회피/슬랙 피드백)에 필요한 추가 설정.


**2026-09-03 정정 — 14 가 아니라 16.** 위 열거는 두 개를 빠뜨렸다:
`testcase_exclude_from_file` 과 `test_category`. 앞의 것은 *어떤 케이스가 돌지 않는지*를
정하는 키이므로 빠뜨리기에 나쁜 쪽이었다. 실제 개수는 파일에서 직접 센 값이다
(shell.conf live 3 키, shell_ci.conf live 19 키, 차집합 16). freeze §11-11 해소.

### 1-4. medium 의 위치가 conf 레벨에서는 sql 의 자매 파일

medium.conf 의 키 21개는 **0개 exclusive** — 전부 sql.conf 또는 medium_dev.conf와 공유. 즉 conf 관점에서는 medium ≈ sql 의 변형(suite 이름만 다름)임이 확인된다. 이는 spec/cli-tree에서 추정했던 "medium은 sql 안에 통합되어 있을 가능성"을 conf 측면에서 보강한다.

medium_dev.conf 는 medium.conf + 1 exclusive(`create_table_reuseoid`).

### 1-5. webconsole.conf 의 격리

`web_port`, `sql_result_root` 두 개 키만 존재. 이 두 키는 다른 어떤 conf에도 등장하지 않음. WEBCONSOLE은 cli-tree에서도 utility 분기로 task 루프와 분리되어 있어, conf 레벨에서도 일관되게 분리되어 있다.

---

## 2. 카테고리별 키 분포

81개 키를 의미 카테고리로 분류:

### 2-1. `default.*` (14) — 모든 instance에 적용되는 기본값

```
default.ssh.pwd                        default.ssh.port
default.cubrid.<property>              default.cubrid.cubrid_port_id (shell_ci)
default.brokercommon.<property>
default.broker1.<property>             default.broker1.APPL_SERVER_SHM_ID (shell_ci)
default.broker1.BROKER_PORT (shell_ci)
default.broker2.<property>             default.broker2.APPL_SERVER_SHM_ID (shell_ci)
default.broker2.BROKER_PORT (shell_ci)
default.ha.<property>                  default.ha.ha_port_id (shell_ci)
default.cm.<property>
```

`<property>` 표기는 와일드카드 — INI 파서(IniData.java)가 dot-notation 프리픽스 매칭으로 임의의 cubrid/broker/ha 파라미터를 받아들이는 구조.

### 2-2. `env.instance*.*` (22) — 인스턴스별 오버라이드

```
env.instance{1,2}.ssh.{host,port,user,pwd,relatedhosts}
env.instance1.cubrid.<property>        env.instance1.cubrid.supplemental_log (cdc_repl)
env.instance1.broker{1,2}.<property>
env.instance1.ha.<property>            (ha_repl)
env.instance{1,2}.master.ssh.{host,user}   (ha_repl/ha_shell)
env.instance{1,2}.slave.ssh.{host,user}    (ha_repl/ha_shell)
env.instance2.broker{1,2}.BROKER_PORT      (shell_ci)
env.instance2.cubrid.cubrid_port_id        (shell_ci)
```

multi-instance(특히 HA: master/slave) 토폴로지를 conf 레벨에서 직접 표현. 새 시스템에서도 이 토폴로지 표현은 유지될 가능성이 높음(외부 인터페이스 동결 대상).

### 2-3. `testcase_*` (7) — 테스트케이스 실행 제어

```
testcase_retry_num
testcase_timeout_in_secs
testcase_exclude_from_file
testcase_exclude_by_macro          (shell_ci 전용)
testcase_git_branch                (shell_ci 전용)
testcase_update_yn                 (shell_ci 전용)
delete_testcase_after_each_execution_yn  (shell_ci 전용)
```

CI 변형이 testcase 갱신/제외/cleanup 기능을 추가로 가짐.

### 2-4. `cubrid_*` (3) — CUBRID 바이너리 자체 설정

```
cubrid_download_url
cubrid_createdb_opts
cubrid_port_id
```

### 2-5. misc (35) — 모듈별 동작 파라미터

대표 항목:
- 공통 동작: `scenario` (12/13), `test_category`, `test_mode`, `data_file`, `enable_memory_leak`
- SQL 계열: `db_charset`, `unicode_input_normalization`, `lock_timeout`, `max_plan_cache_entries`, `update_statistics_on_catalog_classes_yn`, `java_stored_procedure`, `need_make_locale`, `sql_interface_type`, `jdbc_config_file`
- HA 계열: `ha_mode`, `Onceha_mode`, `ha_apply_max_mem_size`, `ha_sync_detect_timeout_in_secs`, `ha_sync_failure_resolve_mode`
- Broker/SHM: `APPL_SERVER_SHM_ID`, `APPL_SERVER_MAX_SIZE`, `SetAPPL_SERVER_MAX_SIZE`, `BROKER_PORT`, `MASTER_SHM_ID`, `SERVICE`
- Isolation: `backup_core_file_yn`
- Webconsole: `web_port`, `sql_result_root`
- CI 한정: `test_platform`, `test_continue_yn`, `feedback_type`, `enable_check_disk_space_yn`

---

## 3. 페어와이즈 공통 키 매트릭스

각 셀은 row와 column conf 파일 사이의 공통 키 개수. 대각선은 자기 자신의 키 수.

|         | cdc_r | ha_re | ha_sh | isol | jdbc | med | med_d | samp | shell | sh_ci | sql | sql_c | webc |
|---------|------:|------:|------:|-----:|-----:|----:|------:|-----:|------:|------:|----:|------:|-----:|
| cdc_repl    | **21** | 20 | 13 | 13 | 1 | 1 | 1 | 1 | 13 | 13 | 1 | 1 | 0 |
| ha_repl     | 20 | **21** | 13 | 13 | 1 | 1 | 1 | 1 | 13 | 13 | 1 | 1 | 0 |
| ha_shell    | 13 | 13 | **28** | 26 | 1 | 1 | 1 | 1 | 26 | 26 | 1 | 1 | 0 |
| isolation   | 13 | 13 | 26 | **27** | 1 | 1 | 1 | 1 | 26 | 26 | 1 | 1 | 0 |
| jdbc        |  1 |  1 |  1 |  1 | **6** | 6 | 6 | 6 | 1 | 1 | 6 | 6 | 0 |
| medium      |  1 |  1 |  1 |  1 | 6 | **21** | 21 | 9 | 1 | 3 | 20 | 20 | 0 |
| medium_dev  |  1 |  1 |  1 |  1 | 6 | 21 | **22** | 9 | 1 | 3 | 20 | 20 | 0 |
| sample      |  1 |  1 |  1 |  1 | 6 | 9 | 9 | **11** | 1 | 1 | 10 | 10 | 0 |
| shell       | 13 | 13 | 26 | 26 | 1 | 1 | 1 | 1 | **26** | 26 | 1 | 1 | 0 |
| shell_ci    | 13 | 13 | 26 | 26 | 1 | 3 | 3 | 1 | 26 | **42** | 3 | 3 | 0 |
| sql         |  1 |  1 |  1 |  1 | 6 | 20 | 20 | 10 | 1 | 3 | **25** | 23 | 0 |
| sql_by_cci  |  1 |  1 |  1 |  1 | 6 | 20 | 20 | 10 | 1 | 3 | 23 | **23** | 0 |
| webconsole  |  0 |  0 |  0 |  0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | **2** |

### Cluster 시각화

```
        Cluster A: SQL/data         Cluster B: shell/process     Cluster C: replication
        ━━━━━━━━━━━━━━━━━━━━        ━━━━━━━━━━━━━━━━━━━━━━━     ━━━━━━━━━━━━━━━━━━━━━
        sql ──── sql_by_cci          shell ──── shell_ci          cdc_repl
         │  ╲   ╱  │                  │           │                   │
         │   ╲ ╱   │                  │           │                  20
         │    ✕    │                  │           │                   │
         │   ╱ ╲   │                  ▼           ▼                ha_repl
         │  ╱   ╲  │                ha_shell ── isolation
        medium ── medium_dev
            ╲    ╱
            sample (subset)

        jdbc는 SQL 패밀리의 미니멀 변형 (6 키, 모두 sql 계열과 공유)
        webconsole은 독립 (web_port, sql_result_root)

        Cluster A ↔ B 교집합 = 1 (`scenario`)
        Cluster A ↔ C 교집합 = 1 (`scenario`)
        Cluster B ↔ C 교집합 = 13 (SSH/instance 정의 영역)
        webconsole ↔ * = 0
```

Cluster B↔C 사이의 13키 공유는 둘 다 multi-instance SSH 토폴로지를 다루기 때문(env.instance{1,2}.{ssh,master,slave}.* 영역).

---

## 4. 단일 파일 전용 키 (총 25개)

cli-tree와 비교했을 때 **새 시스템 설계 시 폐기/통합 후보가 되는 영역**:

| 파일 | exclusive 키 |
|------|--------------|
| sql | `APPL_SERVER_MAX_SIZE`, `SetAPPL_SERVER_MAX_SIZE` |
| isolation | `backup_core_file_yn` |
| sample | `category_alias` |
| medium_dev | `create_table_reuseoid` |
| cdc_repl | `env.instance1.cubrid.supplemental_log` |
| ha_repl | `env.instance1.ha.<property>` |
| ha_shell | `env.instance1.ssh.relatedhosts`, `env.instance2.ssh.relatedhosts` |
| webconsole | `sql_result_root`, `web_port` |
| shell_ci | (14개) — CI 전용 |

shell_ci 의 14개 exclusive는 모듈 본질이 아니라 **운영 환경(CI) 차이**에서 비롯된 것이므로, 새 시스템에서는 "CI 프로필"을 별도 conf 변형으로 두기보다 **공통 conf + CI 오버레이** 모델이 더 깨끗할 가능성.

---

## 5. 새 시스템 설계 시 고려할 점

1. **3-cluster 구조는 자연스러운 모듈 경계**. SQL family, shell/process family, replication family는 conf 키 공유 정도로도 명확히 분리됨. 새 시스템에서도 이 경계를 모듈 인터페이스로 승격할 가치가 있음.

2. **`scenario`만이 진정한 공통 키**. 다른 키들은 모듈 군집 단위로만 의미가 있으므로, "global config" 영역을 거의 없게 설계하고 모듈별 conf를 명시적으로 분리하는 편이 인지 비용을 줄임.

3. **`<property>` 와일드카드는 IniData의 dot-prefix 매칭 동작에 의존**. 새 시스템 파서가 같은 의미를 보존해야 외부 인터페이스 동결 약속을 지킬 수 있음 — IniData.java 의 동작을 design.md(common)에서 정밀히 캡처할 것.

4. **shell_ci.conf 의 14개 exclusive는 CI 오버레이의 정체성**. 새 시스템에서는 base conf + CI overlay 패턴(YAML/INI override stack)이 더 깨끗함. 단, 사용자가 `shell_ci.conf` 파일명 자체에 의존(자동화 스크립트 내부)할 가능성이 있으므로 동결 후보.

5. **multi-instance(`env.instance*.*`) 표현은 conf 자산의 핵심**. cluster B/C가 공유하는 키 13개의 절반 이상이 이 영역. 새 시스템의 토폴로지 모델은 이 표현을 1:1 호환으로 받을 수 있어야 함.

6. **`<key> = ` 형태 외에 dot-notation 기반 hierarchical key**. 새 시스템 conf 포맷이 INI 외(YAML/TOML)로 가더라도 dot-notation 키 → 경로 매핑은 보존되어야 함.

---

## ROADMAP 갱신

체크리스트의 `analysis/_overview/conf-matrix.md` 항목을 완료(`- [x]`)로 갱신.
