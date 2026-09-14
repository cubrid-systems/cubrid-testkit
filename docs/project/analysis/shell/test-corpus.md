# shell — Test Corpus

**Source repos:**
- `cubrid-testcases-private-ex/shell` (3,658 .sh + 보조)
- `cubrid-testcases-private-ex/shell_heavy`, `shell_perf` (변형)
- `cubrid-testcases-private-ex/scripts`
- `cubrid-testcases-private/HA/shell` (482 .sh — ha_repl/cdc_repl 도 사용)
- `cubrid-testcases-private/shell_ext` (342 .sh + 286 .conf — config-heavy)
- `cubrid-testcases-private/longcase` (185 .java + 131 .sh + 88 .jar)
- `cubrid-testcases-private/random_query_generator` (104 .sh + 77 .yy + 45 .zz — RQG)

case-formats.md §5/§5b/§5c/§5d/§5e 와 §6 의 shell-format 부분을 더 깊게 정리.

---

## 1. shell 모듈이 사용하는 케이스 suite 매트릭스

| Suite | Repo / 경로 | 케이스 수 | 특징 |
|-------|-------------|----------|------|
| **shell (base)** | private-ex/shell | 3,658 .sh | 기본 shell 모듈 케이스 |
| **shell_ci** | (shell + 별도 conf) | (shell 케이스 재사용) | CI 전용 conf 변형 — 케이스 자체는 shell 그대로 |
| **shell_heavy** | private-ex/shell_heavy | 142 .sh + 253 .java + 52 .class | Java-heavy 부하 시나리오 |
| **shell_perf** | private-ex/shell_perf | 50 .sh + 60 .sql | 성능 측정 + split .gz_aa/_ab/_ac 시드 |
| **shell_ext** | private/shell_ext | 342 .sh + 286 .conf | config-heavy (case당 conf) — 가능성 있는 SHELL 카테고리 |
| **HA (shell)** | private/HA/shell | 482 .sh | ha_repl/cdc_repl 도 사용. shell-format 그대로 |
| **longcase** | private/longcase | 131 .sh + 185 .java + 88 .jar | 장시간 부하 + 사전 빌드 jar |
| **RQG** | private/random_query_generator | 104 .sh + 77 .yy + 45 .zz | 외부 RQG 표준 호환 + 셸 래퍼 |
| **scripts** | private-ex/scripts | (보조) | 케이스 외 보조 스크립트 |
| **manually** | private/manually | 27 .txt + 20 .sh | *비-자동화* 절차 문서 |

→ shell 의 케이스 형식이 *다중 suite 의 표준 입력 채널* — 새 시스템에서 `shell` 모듈은 사실상 *case execution platform* 역할.

---

## 2. shell-format 케이스의 표준 구조

### 2-1. 케이스 디렉터리 패턴

```
<scenario_root>/
└── <category>/                  e.g. _28_features_844
    └── <scenario_name>/         e.g. issue_10709_statistic
        └── <subscenario>/       e.g. issue_10709_statistic_1
            ├── cases/
            │   └── <name>.sh    e.g. issue_10709_statistic_1.sh
            └── answers/
                └── <name>.answer
```

`cases/` + `answers/` 자매 디렉터리는 *반드시* 짝지어 존재. case path 의 `cases/` 세그먼트가 path 파싱의 anchor (implementation-notes.md §1).

### 2-2. 보조 자산 패키징

한 `<subscenario>/` 디렉터리 안에 `cases/`, `answers/` 외 추가:

| 디렉터리/패턴 | 용도 |
|---------------|------|
| `data/` 또는 inline `.sql`, `.txt`, `.gz` | 시드 데이터 |
| `<name>.java`, `<name>.c`, `<name>.cpp` | native/JVM 클라이언트 (케이스가 빌드 후 실행) |
| `<name>.exp` | expect 스크립트 (TUI 자동화) |
| `<name>.conf` | 케이스 전용 conf (shell_ext 에서 다용) |
| `<name>.result` | 케이스 작성자가 stdout 외에 결과를 기록하는 채널 — 워커가 회수 |

### 2-3. answer 변형

| 확장자 | 갯수 (private-ex/shell) | 의미 |
|--------|------------------------|------|
| `.answer` | 3,090 | 기본 정답 |
| `.answer_WIN` | 52 | Windows 변형 (대문자 일관성 깨짐) |
| `.answer_win` | 39 | Windows 변형 (소문자) |
| `.log` | 81 | 참조 로그 (이전 실행 또는 sample) |
| `.result` | 100 | (정답 아닌 *케이스가 쓰는* 결과 채널) |

→ Windows 변형의 *대소문자 비일관* (.answer_WIN vs .answer_win) 은 데이터 quality 노이즈. 새 시스템 lint 후보.

---

## 3. 케이스 본체 — 표준 헤더

```bash
#!/bin/bash
# 시나리오 설명 주석 (#1, #2, ...)

. $init_path/init.sh
init test
set -x

db_name=<번호 또는 이름>
cubrid service stop
cubrid server stop $db_name
cubrid deletedb  $db_name
rm -rf  $db_name

mkdir $db_name
cd $db_name
cubrid_createdb -r $db_name
sleep 2
cubrid server start $db_name
cd ..

# ... 본 시나리오 ...

# Cleanup
cubrid server stop $db_name
cubrid deletedb $db_name
```

**컨벤션:**
- 케이스 시작: `. $init_path/init.sh` + `init test` (init.sh 의 helper 함수 호출)
- `set -x` (디버깅 trace)
- DB 이름: 케이스마다 unique (보통 issue 번호 또는 시나리오 ID)
- 시작 시 *의도적 cleanup* (`cubrid service stop`, `deletedb`) — 이전 실행 잔존물 제거
- 종료 시 cleanup (의무는 아니지만 권장)

→ 새 시스템에서 init.sh 의 함수 시그니처 (`init <type>`) 동결.

---

## 4. HA 변형 — `cubrid-testcases-private/HA/`

```
HA/
└── shell/
    ├── _23_ha_enhancement
    ├── _25_features_844
    ├── _26_features_845
    ├── _28_features_930
    ├── _29_banana_qa
    ├── _38_fig
    ├── _39_fig_cake
    └── config
```

482 .sh + 328 .answer + 218 .sql + 117 .java + 9 .result.

**핵심:** HA 케이스는 *별도 형식이 아니라 shell-format 그대로* (case-formats.md §5b 의 결론). HA 토폴로지는 *case 안에서 init_path 의 `ha_*.sh` 헬퍼* 를 source 해서 셋업:
- `ha_common.sh`
- `make_ha.sh`, `make_ha_lower.sh`, `make_ha_upper.sh`
- `HA.properties`

→ HA 컨트랙트는 *init_path/ha_*.sh* 의 함수 시그니처. 새 시스템에서 동결 표면.

---

## 5. RQG 변형 — `random_query_generator/`

104 .sh + 77 .yy + 45 .zz + 6 .txt + 5 .java.

**.yy / .zz 는 외부 OSS RQG 표준** (MariaDB Random Query Generator 와 호환). CTP 측은 *셸 래퍼* (`<name>.sh`) 로 RQG runner 를 호출.

CTP.executeShell 의 RQG 분기는 `executeShell(config, "rqg", "rqg")` — 같은 메서드 + `TEST_CATEGORY=rqg` 시스템 프로퍼티만 추가. 즉 shell.main.Main 이 RQG 케이스를 *shell 케이스로 취급* + 카테고리 라벨링.

→ 새 시스템에서 RQG는 `.yy`/`.zz` 표준 형식 *그대로 동결*. shell 모듈은 RQG runner 를 invoke 하는 thin wrapper 만 제공.

---

## 6. shell_heavy / shell_perf / longcase — Java/jar 동봉

```
shell_heavy/  ─ 142 .sh + 253 .java + 52 .class + 41 .sql + 24 .jar
shell_perf/   ─ 50 .sh + 60 .sql + 23 .java + 7 .gz + split .gz_aa/_ab/_ac
longcase/     ─ 131 .sh + 185 .java + 88 .jar + 55 .sql
```

**패턴:** 케이스 디렉터리에 *사전 빌드된 .jar / .class* 가 함께 보관. 케이스 작성자가 빌드 환경 차이를 *케이스 시점에 동결* 하기 위한 의도.

**위험:**
- jar 가 노후되어 새 JVM 호환 깨질 수 있음
- jar 빌드 출처 / 재현 가능성 (빌드 스크립트가 같이 있는지) 확인 필요

→ 새 시스템에서 *케이스 시점 사전 빌드 vs 실행 시점 빌드* 정책 ADR.

---

## 7. shell_ext — config-heavy (286 .conf)

342 .sh + **286 .conf** + 164 .java + 156 .sql + 127 .txt.

case 당 별도 .conf 가 있다는 의미 (`<name>.sh` + `<name>.conf` + `<name>.answer`). doc/shell_ext_guide.md (43KB) 가 권위 있는 출처. 정밀 분석 후속.

→ 새 시스템에서 case-level conf 의 *공유 conf 와의 merge 규칙* 동결 표면.

---

## 8. 케이스 작성자 측 contract (동결 표면 요약)

새 시스템 shell 대체 시 케이스 작성자에게 *변경 없이 보여야* 할 것:

1. ✅ 디렉터리 패턴: `<...>/cases/<name>.sh` + `<...>/answers/<name>.answer`
2. ✅ `. $init_path/init.sh` + `init <type>` 헬퍼 컨트랙트
3. ✅ `<name>.result` 결과 채널 — 워커가 회수
4. ✅ `<name>.answer_win` / `.answer_WIN` 윈도우 변형 (대소문자 표준화 후속)
5. ✅ shell_ext 의 `<name>.conf` per-case 패턴
6. ✅ HA 의 ha_*.sh 헬퍼 컨트랙트
7. ✅ longcase / shell_heavy 의 jar 동봉 정책
8. ✅ RQG 의 .yy/.zz 외부 표준
9. ✅ 케이스 안의 자유로운 셸 명령 (`cubrid service`, `cubrid_createdb`, expect, ...)
10. ✅ `set -x` 디버깅 trace 활성

부수:
- `manually/` 의 .txt 절차 문서 — 자동화 대상 아님 (새 시스템에서 별도 location 으로 분리 권고)
- 데이터 quality lint (대소문자 일관, 누락 .answer 페어 등) 도입 후보

---

## 9. 새 시스템 corpus 운영 권고

1. **case metadata header 도입** — 디렉터리 위치로 표현되는 메타 (category/issue/topic) 를 케이스 헤더 주석에 명시:
   ```bash
   # @category: features_844
   # @issue: 10709
   # @topic: statistic
   # @timeout: 300
   ```
   필터링 / dispatch 우선순위 결정에 사용.

2. **lint 도구**:
   - `.answer_WIN` vs `.answer_win` 일관화
   - cases/ + answers/ 페어링 검증
   - init.sh source 누락 검출
   - `set -e` (또는 `set -o pipefail`) 권장 (현재 `set -x` 만 있음)

3. **사전 빌드 산출물 정책** (shell_heavy / longcase / commonforjdbc.jar) — 빌드 출처 / 재현성 / 보존 기간 ADR.

4. **테스트 메타 디렉터리 (`config/`)** — HA/, isolation/, medium/ 모두 가지고 있는 `config/` 하위 — 통일된 의미 확정.

5. **manually/** 분리 — 자동화 대상이 아니므로 새 시스템에서 `docs/manual-procedures/` 로 옮기는 것이 깔끔.
