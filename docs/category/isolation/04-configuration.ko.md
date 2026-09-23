# 4. 설정

*[English](04-configuration.md) · 한국어*

[← isolation 카테고리로](README.ko.md)

- [conf](#conf)
- [엔진 파라미터](#엔진-파라미터)
- [거부되거나 무시되는 것](#거부되거나-무시되는-것)
- [환경변수](#환경변수)

conf 는 CTP 의 평평한 `isolation.conf` 다: `key=value`, `#` 주석, `${VAR}` 는 확장된다. `-c` 가
그것을 지목한다. `-c` 가 없으면 `$CTP_HOME/conf/isolation.conf`.

## conf

| 키 | 기본값 | 무엇을 하는가 |
|---|---|---|
| `scenario` | — (필수) | 케이스 트리. `$HOME` 아래에 있으면 CTP 가 그랬듯 `$HOME` 기준 상대 경로로 취해지고, dispatch 파일 안의 경로들도 상대 경로가 된다 |
| `testcase_timeout_in_secs` | 2147483647 | 한 시도의 `qactl` 에 대한 `timeout3.sh` 의 한계. CTP 의 출하 conf 는 300 이라고 한다 |
| `testcase_retry_num` | 0 | `runone.sh` 가 첫 시도 뒤에 하는 시도 수. 처음 통과하는 데서 멈춘다. CTP 의 출하 conf 는 4 라고 한다 |
| `testcase_exclude_from_file` | 없음 | 경로 조각 하나짜리 파일, 주석은 `#` 과 `--`. **항목 하나는 자기를 경로에 포함하는 첫 케이스 하나만, 오직 그것만 제외한다.** 없는 파일은 런을 멈춘다 — CTP 는 그것을 빈 목록으로 읽고 전부를 돌렸다 |
| `backup_core_file_yn` | **no** (CTP 의 기본값은 yes) | `yes` 는 CTP 의 동작을 준다: `runone.sh` 가 코어를 직접 확인하고 코어와 설치본 전체의 사본을 `~/error_backup` 으로 백업한다. 설정하지 않으면 러너가 `-n` 을 넘기고 확인을 스스로 하며, 크래시 리포트와 코어의 스택을 런과 함께 남기고 백업은 쓰지 않는다 ([ADR-021](../../project/adr/ADR-021-crash-reports.md)) |
| `cubrid_testdb_name` | cubrid | 이름과 달리 클라이언트 프로그램이다: `cubrid` 는 `qacsql`, `mysql` 은 `qamysql`, 그 밖의 것은 `runone.sh` 가 그 케이스를 거부하게 만든다 |
| `test_category` | isolation | 피드백이 찍는 것. 런 디렉터리는 여기에 뭐라고 적혀 있든 `result/isolation` 이다 |
| `test_continue_yn` | no | 재개: `dispatch_tc_ALL.txt` 에서 모든 `dispatch_tc_FIN_*.txt` 를 뺀 것, 로그에는 덧붙인다 |
| `feedback_type` | file | `file` 은 `feedback.log` 와 `test_status.data` 를 쓴다. `database` 는 여기서 백엔드가 아니고 경고와 함께 파일을 쓴다. 그 밖의 것은 피드백을 남기지 않는다 |
| `parallel_slots` | 계산됨 | 동시에 도는 케이스 수. 슬롯마다 자기 `ctldb` 를 갖는다. 설정하지 않으면 러너는 다음 중 가장 작은 것을 취한다: 이 머신의 런들에서 슬롯 하나가 든 값 (코퍼스를 돌린 적이 없으면 1,750 MB) 으로 메모리가 감당하는 수, CPU 하나당 하나, 첫 런에서는 넷 그리고 그 뒤로는 지금까지 돌린 최대치의 두 배, 그리고 슬롯을 더 늘려도 값을 못 한다고 측정된 수. 무엇을 골랐는지는 표준 오류에 말한다. 값을 주면 적힌 대로 쓰이고, `1` 은 CTP 가 하듯 직렬로 돌린다 ([돌리기](03-running-it.ko.md#슬롯)) |
| `parallel` | measured | 설정하지 않은 `parallel_slots` 를 어떻게 계산할지: `conservative` 는 예산을 두 배로 잡고 늘리지 않는다. `aggressive` 는 예산의 0.8 을 취하고, 네 배까지 늘리고, 꺾이는 점을 무시한다. 그 밖의 것은 경고와 함께 `measured` 다 ([ADR-020](../../project/adr/ADR-020-sizing.md)) |
| `scenario_disk` | no | 케이스 트리를 슬롯마다의 오버레이 뒤에 둬서 `result/` 와 `<name>.result` 가 그 안에 쓰이지 않게 한다 |
| `case_patch_dir` | 꺼짐 | 이 런이 지고 가는 코퍼스 변경. 첫 케이스 전에 적용되고 끝에 되돌려진다. `qactl` 이 얼마나 느린지를 답에 기록한 케이스는 느리지 않은 어떤 컨트롤러에서도 실패한다 — [케이스가 실패할 때](05-when-a-case-fails.ko.md#컨트롤러가-느려서-통과하는-케이스들) 참조. 설정하지 않으면 아무것도 패치되지 않고, 코퍼스를 있는 그대로 보고하는 것이 일인 런에는 그것이 옳은 기본값이다 |
| `status_http` | 꺼짐 | 상태 페이지: `on`, 포트 하나, 또는 `host:port` |

## 엔진 파라미터

`DEPLOY` 에서 CTP 의 `ini.sh` 로 슬롯마다의 설치본에, 어떤 케이스도 돌기 전에 쓰인다:

| 접두사 | 파일, 섹션 |
|---|---|
| `default.cubrid.<param>` | `cubrid.conf`, `[common]` |
| `default.ha.<param>` | `cubrid_ha.conf`, `[common]` |
| `default.cm.<param>` | `cm.conf`, `[cm]` |
| `default.broker1.<param>` | `cubrid_broker.conf`, `[%query_editor]` |
| `default.broker2.<param>` | `cubrid_broker.conf`, `[%BROKER1]` |
| `default.brokercommon.<param>` | `cubrid_broker.conf`, `[broker]` |

`ctldb` 자체는 `prepare.sh` 가 자기 볼륨 크기 (각 50M) 로 만들므로 `db_volume_size` 는 거기 닿지
않는다. Deploy 는 CTP 가 하듯 매 런마다 `cubrid.conf` 에 `inquire_on_exit=3` 도 덧붙인다 —
슬롯 안에서는 그 줄이 슬롯의 오버레이와 함께 간다.

## 거부되거나 무시되는 것

| 키 | 무슨 일이 일어나는가 |
|---|---|
| `env.<instance>.*` | 거부. 이 러너는 자기가 시작된 머신에서 isolation 을 돌린다 (ADR-014). 거기서 돌리거나, CTP 를 통해 돌려라 |
| `testcase_update_yn=yes` | 거부. 코퍼스를 갱신하는 것은 러너의 일이 아니다 |
| `cubrid_download_url` | 경고. 시험되는 것은 설치된 빌드다 |
| `enable_check_disk_space_yn` | 확인하지 않는다 |

## 환경변수

| 변수 | 무엇을 하는가 |
|---|---|
| `TESTKIT_NATIVE=isolation` | 이 task 를 CTP 안이 아니라 여기서 돌린다. 없으면 `testkit isolation` 은 CTP 의 것이다 |
| `TESTKIT_CONTAIN=1` | **필수**: 자기 네임스페이스가 없으면 러너는 시작을 거부한다. `runone.sh` 가 사용자 단위로 죽이기 때문이다 |
| `TESTKIT_SLOT_ROOT` | 슬롯의 쓰기가 떨어지는 곳. 없으면 `/var/tmp/testkit-slots`. 가장 빠른 디스크에 두라 |
| `TESTKIT_SLOT_VOLATILE=1` | 슬롯 오버레이를 volatile 로 마운트한다: 그것에 대한 sync 가 즉시 돌아온다. 어차피 그 층들은 끝에 버려진다 |
| `CUBRID`, `CUBRID_DATABASES`, `CTP_HOME`, `JAVA_HOME` | CTP 에서와 같다. `CUBRID_DATABASES` 는 `$CUBRID` 아래에 두라. 거기를 슬롯의 설치본 오버레이가 덮는다 |
