# 4. 설정

*[English](04-configuration.md) · 한국어*

[← shell 카테고리로](README.ko.md)

- [CTP 의 키들](#ctp-의-키들)
- [이 러너의 키들](#이-러너의-키들)
- [환경](#환경)
- [각 설정이 얼마의 값어치인가](#각-설정이-얼마의-값어치인가)
- [권장 설정](#권장-설정)

## CTP 의 키들

이들은 늘 그랬던 대로 동작한다.

| 키 | 기본값 | |
|---|---|---|
| `scenario` | — | 코퍼스 루트. 필수 |
| `test_category` | — | `shell` |
| `testcase_retry_num` | `0` | 첫 실패 뒤의 시도 횟수 |
| `testcase_timeout_in_secs` | `0` | 0 은 타임아웃 없음. CI 는 720 을 쓴다 |
| `testcase_exclude_by_macro` | — | 이 텍스트를 담은 케이스를 건너뛴다. 예: `LINUX_NOT_SUPPORTED` |
| `testcase_exclude_from_file` | — | 건너뛸 경로 조각들이 담긴 파일들, 쉼표로 구분. 존재하지 않는 파일은 런을 멈춘다 |
| `test_continue_yn` | `false` | 이어 돌린다, 이미 판정이 있는 것은 건너뛰고 |
| `cubrid_db_charset` | `en_US` | `cubrid_createdb` 가 로케일로 넘기는 것 |
| `feedback_type` | `file` | `file` 은 런의 feedback.log 와 그 옆의 파일들을 쓴다. `db` 는 이 러너가 쓰지 않는 데이터베이스를 요구한다: 그렇다고 말하고 대신 파일을 쓰므로, 이벤트는 지켜지고 그 목적지만 바뀐다. 그 밖의 무엇이든 CTP 자신의 "피드백을 전혀 하지 않음" 이다 — feedback.log 도 없고, `replay` 와 `failures` 가 읽을 것도 없다 |

## 이 러너의 키들

| 키 | 기본값 | 영향 | |
|---|---|---|---|
| `parallel_slots` | 크기 측정 | **큼** | 한 번에 몇 케이스가 도는지. 설정하지 않고 격리돼 있으면 머신의 첫 런에서 하나, 그다음에는 그 머신이 돌려본 최대치의 두 배까지 가는데, 메모리나 프로세서나 knee — 슬롯을 더 늘려도 더 빠르지 않다고 측정된 수 — 가 멈출 때까지다([ADR-020](../../project/adr/ADR-020-sizing.md)). 격리되지 않았으면 하나 |
| `parallel` | `measured` | 위의 것이 자라는 방식 | `conservative` 는 자라지 않고 메모리 예산을 두 배로 잡는다; `aggressive` 는 네 배로 자라고 knee 를 무시한다 |
| `scenario_ram_mb` | 끔 | **큼; 런을 실패시킬 수 있다** | 코퍼스 오버레이의 상위 층이 이 크기의 tmpfs 가 된다. [상한](05-the-ceiling.ko.md)을 보라 |
| `scenario_ram_high_water` | `80` | 보호용 | 그 위로는 새 케이스가 시작하지 않는, 상한에 대한 퍼센트 |
| `scenario_disk` | 끔 | 상황에 따라 | 모든 슬롯이 자기만의 오버레이를 통해 코퍼스를 보고, 그 상위 층은 `TESTKIT_SLOT_ROOT` 아래 슬롯의 디렉터리에 있으며 그것과 함께 지워진다. 디스크의 코퍼스는 바뀌지 않는다. `TESTKIT_SLOT_VOLATILE` 이 케이스가 자기 디렉터리에 만드는 데이터베이스까지 닿게 하는 것이 이것이다. `scenario_ram_mb` 나 자기만의 `testcase_workspace_dir` 과는 함께 쓰지 않는다; `TESTKIT_CONTAIN=1` 이 필요하다 |
| `heavy_in_flight_max` | `slots/4` | 보호용 | 가장 무거운 케이스를 한 번에 몇 개까지 돌려도 되는지 |
| `case_plan` | 끔 | 작음; 슬롯이 늘수록 커진다 | 케이스별 소요시간. 런의 순서를 정하려고 읽고, 측정한 것에서 쓴다 |
| `case_sizes` | 끔 | 직접적으로는 없음 | 디렉터리별 최대 발자국, 레인 키들이 읽는다 |
| `case_patch_dir` | 끔 | 없음 | 오버레이 안으로 적용되는 케이스별 패치; 디스크의 코퍼스는 바뀌지 않는다 |
| `lane_slow_secs` | 끔 | **부정적** | 슬롯을 메모리 레인과 디스크 레인으로, 소요시간으로 나눈다 |
| `lane_slow_mb` | 끔 | 상황에 따라 | 같은 분리를, 발자국으로 |
| `lane_slow_mbps` | 끔 | 상황에 따라 | 같은 분리를, 쓰기 속도로 |
| `case_logs` | 끔 | 작음 | 케이스가 쓴 것을 남긴다: `fail`, 또는 통과한 케이스에 대해서도 값싼 티어를 남기려면 `all`. [실패한 것을 남기기](06-keeping-what-failed.ko.md)를 보라 |
| `case_logs_max_mb` | 끔 | 없음 | 런 전체의 캡처에 대한 예산 |
| `status_http` | 끔 | 없음 | 진행 페이지. `on` 은 `127.0.0.1:51523`; 포트만 적으면 모든 인터페이스를 가져간다 |

## 환경

| | |
|---|---|
| `TESTKIT_NATIVE=shell` | `shell` 을 CTP 에 넘기지 않고 여기서 돌린다. opt-in 게이트. 계열을 쉼표로 구분해 나열하고(`shell,sql`), `all` 은 그 전부다; `TESTKIT_NATIVE_SHELL=1` 은 옛 표기이고 여전히 동작한다 |
| `TESTKIT_CONTAIN=1` | 런을 자기만의 네임스페이스에 넣는다. 슬롯과 `scenario_ram_mb` 에 **필수** |
| `TESTKIT_CONTAIN_SH` | `/bin/sh` 위에 bind 할 셸. 찾을 수 있으면 `bash` |
| `TESTKIT_SLOT_ROOT` | 각 런이 슬롯별 오버레이를 위한 자기 디렉터리를 만드는 곳, 슬롯이 닫힐 때 지워진다. 설정하지 않으면 `/var/tmp/testkit-slots` |
| `TESTKIT_SLOT_VOLATILE=1` | 슬롯별 오버레이를 `volatile` 로 마운트한다: 슬롯 층에서의 sync 가 아무것도 하지 않고 돌아온다. 기본은 꺼짐인데, sync 가 CTP 가 도는 조건의 일부이기 때문이다. **shell 에서 이것은 `scenario_disk` 나 디스크 레인의 오버레이(`lane_slow_*`)를 통해서만 코퍼스에 닿는다**: 그렇지 않으면 케이스는 CTP 아래에서 그러듯 코퍼스 안 제자리에서 돌고, 그 데이터베이스는 어떤 오버레이 위에도 없다 — 슬롯 여덟 개의 `_01_utility` 가 이것 없이 1,426 초, 있으면 1,401 초 걸렸고, flush 는 똑같이 139,000 번, 판정도 같았다(`project/evidence/parallel-shell.md` §5). sql 의 데이터베이스는 오버레이 위에 있고, 거기서 이것은 슬롯 여덟 개를 1,131 초에서 338–394 초로 만들었다(`project/evidence/sql-native.md` §3). Linux 5.10+ 가 필요하다; sync 가 이미 아무 비용도 들지 않는 tmpfs 에서는 아무것도 바꾸지 않는다 |

그리고 CTP 의 실패 스냅샷, 이것은 동결된 표면 바깥이고 설정할 수 있다:

| | 기본값 | |
|---|---|---|
| `CTP_ERROR_BACKUP` | 켬 | `off` 는 스냅샷을 아예 뜨지 않는다 |
| `CTP_ERROR_BACKUP_DIR` | `~/ERROR_BACKUP` | 스냅샷이 가는 곳. 전에는 하드코딩돼 있었다 |

스냅샷은 설치본 전체에 케이스 디렉터리를 더한 것 — 실패하는 케이스마다 748 MB — 이라서, 실패가
많은 런은 홈 디렉터리에 수십 기가바이트를 쓰고, 그것도 측정 안에서 쓴다.
[실패한 것을 남기기](06-keeping-what-failed.ko.md)를 보라.

CTP 자신의 환경도 여전히 적용된다 — `CUBRID`, `CUBRID_DATABASES`, `CTP_HOME`, `JAVA_HOME` —
그리고 `SKIP_CHECK_RECOVERY_ERROR`, `SKIP_CHECK_FATAL_ERROR`, `CTP_ERROR_BACKUP` 도 마찬가지다.

**`$CUBRID_DATABASES` 는 `$CUBRID` 안에 있어야 한다.** CTP 의 케이스별 리셋은 정확히
`$CUBRID/databases` 를 치운다; 변수를 다른 곳으로 가리키면 리셋은 아무도 쓰지 않는 디렉터리를
문지르고, 진짜 레지스트리는 케이스에서 케이스로 항목을 들고 간다.

## 각 설정이 얼마의 값어치인가

**`parallel_slots` 가 유일하게 큰 지렛대다.** 모든 설정이 `total work / slots` 의 1-2% 안에
떨어진다: 스케줄러는 이미 자기 바닥에 있고, 그래서 wall clock 은 일이 어떻게 정렬되는지가 아니라
일이 얼마나 있고 슬롯이 몇인지가 정한다. 코퍼스는 3,444 케이스에 22.9 시간의 일이고, 어떤 슬롯
수로도 그 아래로 내려가지 못하는 27 분짜리 최장 케이스가 있다:

| 슬롯 | 바닥 |
|---:|---:|
| 8 | 2.9 h |
| 16 | 1.4 h |
| 24 | 1.0 h |

**`case_plan` 은 슬롯 열여섯쯤부터 값을 한다.** 거기서부터 최장 케이스가 벽이 된다; 그 아래에서는
바닥이 `work / slots` 이고 순서가 그것을 움직이지 못한다. 그래도 켜두어라 — 비용이 없고, 진행
페이지의 "remaining" 은 계획이 있을 때만 믿을 만하다.

**`scenario_ram_mb` 는 시작이 아니라 지속 대역폭의 값어치가 있다.** 디스크에서 코퍼스는 약
250 MB/s 를 요구하는데 디스크는 동시 쓰기 넷 아래에서 88 을 내놓았다; 그 격차가 이득의 전부이고,
메모리를 쓰기 전에는 슬롯 넷이 한계였던 이유이기도 하다.

**레인은 메모리가 구속 제약이 아닌 머신에서는 손해인데**, 그것이 레인이 측정된 머신이었다.
`lane_slow_secs` 는 소요시간으로 고르는데, 소요시간은 메모리가 도와주는 것과 음의 상관이다 — 긴
케이스가 I/O 가 무거운 것이다 — 그래서 케이스당 1.55-1.93 배를 치렀다. 발자국과 소요시간은
무관한 것으로 드러났고, 그래서 `lane_slow_mb` 는 쓰기 속도를 무작위로 고르는 셈이다;
`lane_slow_mbps` 만이 디스크가 실제로 묶여 있는 것을 고르는 기준이다. 당신 머신에서 메모리가
구속한다고 측정하지 않았다면 **셋 다 꺼두어라.**

**지렛대가 아닌 것.** `max_clients` 와 `data_buffer_size` 는 슬롯을 사주지 않는다 — 서버에는 어떤
파라미터도 닿지 못하는 157 MB 의 바닥이 있다. 그리고 코퍼스 wall clock 의 44% 는 케이스 스크립트
안의 문자 그대로의 `sleep` 이고, 여기의 어떤 설정도 그것을 건드리지 않는다. 하네스 자신의
오버헤드는 2% 다.

## 권장 설정

### 개발자의 머신, 코퍼스의 일부

```
scenario=/path/to/testcases/shell
test_category=shell
feedback_type=file
testcase_retry_num=0
parallel_slots=4
case_plan=/path/to/plan
status_http=on
```

tmpfs 는 쓰지 않는다: 슬롯 넷에서는 디스크가 아직 한계가 아니고, 크기를 재보지 않은 상한은 런을
빠르게 하는 방법이 아니라 실패시키는 방법이다.

### 머신 하나, 코퍼스 전체

30 GB 머신에 맞춰 잰 것이다 — 다른 머신에서 이것을 믿기 전에 `scripts/sizing.sh` 을 돌려라.
잰다는 것은 세어지는 코어가 아니라 비어 있는 코어를 뜻한다: 다른 일도 함께 쓰고 있던
30 GB·16 코어 머신에서 슬롯 열여섯은 여덟보다 빠르지 않았고, `sizing.sh` 은 한꺼번에 네다섯
코어어치의 일을 측정해 그렇다고 말해두었다
([`parallel-shell.md` §6](../../project/evidence/parallel-shell.md)).

```
scenario=/path/to/testcases/shell
test_category=shell
feedback_type=file
testcase_retry_num=0
testcase_timeout_in_secs=720
parallel_slots=16
scenario_ram_mb=18432
scenario_ram_high_water=80
case_plan=/path/to/plan           # 첫 런에서 쓰이고, 다음 런에서 읽힌다
case_sizes=/path/to/sizes         # 마찬가지
case_patch_dir=/path/to/cubrid-testkit-patches/shell
case_logs=fail                    # 실패한 케이스가 쓴 것을 남긴다
status_http=on
```

이만큼 긴 런에서 `case_logs` 는 선택이 아니다. 코퍼스 전체는 두세 시간이 걸리고 실패 몇 개로
끝난다; 케이스별 출력은 `$CTP_HOME/result` 아래에 사는데 그것은 다음 런이 지우므로, 이것이
없으면 실패를 읽는 유일한 방법은 전체를 다시 돌리는 것이다. 어렵게 측정한 값: develop head
에서의 162 분짜리 런이 실패 32 개로 끝났고 그것을 읽을 것은 아무것도 남지 않았다.
[실패한 것을 남기기](06-keeping-what-failed.ko.md)를 보라.

그리고 엔진의 `cubrid.conf` 에는:

```
db_volume_size=20M
log_volume_size=20M
data_buffer_size=64M
log_buffer_size=4M
```

슬롯 16 은 `case_plan` 이 값을 하기 시작하는 지점이면서 아직 디스크가 묶는 지점 아래다; 볼륨
크기가 슬롯들을 상한 안에 들어맞게 하는 것이다. 그것을 바꾸기 전에
[상한](05-the-ceiling.ko.md#cubridconf-은-판정에-중립적이지-않다)을 읽어라 — 그 네 줄은 판정에
중립적이지 않다.

### CI

같은 것에, 흔들리는 케이스가 재시도되어 통과로 바뀌는 대신 보이는 채로 남도록
`testcase_retry_num=0`, 마지막으로 초록이던 런에서 들고 온 `case_plan`, 그리고 공개된 포트의
`status_http` 를 더한 것.
