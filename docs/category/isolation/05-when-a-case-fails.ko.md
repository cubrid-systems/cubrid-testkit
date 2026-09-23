# 5. 케이스가 실패할 때

*[English](05-when-a-case-fails.md) · 한국어*

[← isolation 카테고리로](README.ko.md)

- [어디를 볼 것인가](#어디를-볼-것인가)
- [실패가 아닌 줄들](#실패가-아닌-줄들)
- [어디서나 실패하는 케이스들](#어디서나-실패하는-케이스들)
- [CTP 도 재현하지 못하는 케이스들](#ctp-도-재현하지-못하는-케이스들)
- [컨트롤러가 느려서 통과하는 케이스들](#컨트롤러가-느려서-통과하는-케이스들)
- [케이스 하나를 다시 돌리기](#케이스-하나를-다시-돌리기)
- [코어와 fatal error](#코어와-fatal-error)

## 어디를 볼 것인가

실패한 케이스는 콘솔에서 `[NOK]` 다. 그 다음은 런 디렉터리
`$CTP_HOME/result/isolation/current_runtime_logs` 안이다:

| 파일 | 그 케이스에 대해 무엇을 담는가 |
|---|---|
| `feedback.log` | 판정 줄, 그리고 `D I F F` 배너 뒤에 기준 답과 정규화된 결과를 나란히 — `<` 는 답에만 있는 줄, `>` 는 결과에만 있는 줄. 케이스를 클릭하면 상태 페이지가 같은 블록을 보여준다 |
| `test_local.log` | `runone.sh` 가 찍은 전부: 그 `set -x` trace 와, 시도마다 `Testing <case> (retry count: n)`, 답에 대한 `diff`, 그리고 `flag: NOK` |

그리고 런이 `scenario_disk` 를 쓰지 않았다면 케이스 옆에:

| 파일 | 통과한 뒤 | 실패한 뒤 |
|---|---|---|
| `result/<name>.log` | 정규화된 결과 | 마지막 시도의 정규화된 결과 |
| `result/<name>.result` | 원본 결과 | `<name>:NOK` 과 나란히 놓은 diff 로 **대체된다** — 원본 출력은 사라진다 |
| `<name>.result` | `<name>:OK.` | 앞선 통과가 있었다면 거기서 온 것 |

`feedback.log` 안의 diff 는 다른 답들을 가진 케이스에 대해서도 언제나 `answer/<name>.answer` 에
대한 것이다. 판정을 정한 비교는 그 전부를 시도했다.

## 실패가 아닌 줄들

모든 케이스의 trace 는 놀라 보이지만 그렇지 않은 줄 둘을 지고 있다:

- `ERROR: Cannot remove user INFORMATION_SCHEMA from the database.` — `clean.sh` 가 DBA 와 PUBLIC
  을 뺀 모든 사용자를 없애려 하는데, 11.5 에는 없앨 수 없는 시스템 사용자가 있다.
- `find: '/home/<you>/CUBRID/log': No such file or directory` — `runone.sh` 는 케이스마다 그 전에
  `~/CUBRID/log` 를 비우는데, 그것이 있든 없든 그렇게 한다.

## 어디서나 실패하는 케이스들

상류 develop 의 head (엔진 `f1ae86ff7`, 케이스 `6ab786aa9`) 에서, 일곱 케이스가 CTP 런 두 번과
이 러너의 런 두 번 — 한 슬롯과 네 슬롯 — 모두에서 모든 시도에 실패했다
([`isolation-baseline.md`](../../project/evidence/isolation-baseline.md) §4):

- `_01_ReadCommitted/cbrd_21506/unique_index/insert_update_04`
- `_01_ReadCommitted/function/counter_function/delete_delete_rownum_01`
- `_01_ReadCommitted/partition_table/range/dml_ddl/reorganization_select_01`
- `_05_ReadCommitted_RepeatableRead/index_column/common_index/basic_sql/insert_delete_05`
- `_06_features/cbrd_22705_online_index_parallel/normal_index/insert_update_04`
- `_06_features/cbrd_22705_online_index_parallel/unique_index/insert_update_04`
- `_06_features/cbrd_22705_online_index_parallel/dml_online_index/insert_odku_online_index_01`

같은 엔진과 같은 케이스에서 CTP 에서도 실패하므로 러너의 것이 아니다 — 그리고 이제 각각에 이유가
있다 ([`isolation-always-failing.md`](../../project/evidence/isolation-always-failing.md)):

- `partition_table/range/dml_ddl/reorganization_select_01` 은 **서버를 크래시시킨다.** 클라이언트
  하나, 10 만 행짜리 range 파티션 테이블과 그것의 `group by` 면 충분하고, `runone.sh` 의 코어
  점검은 그것을 보지 못한다 (아래).
- `insert_update_04` 셋과 `insert_odku_online_index_01` 은 각각, 락 강등 하나가 풀어준 두
  클라이언트의 줄들을 이 머신이 만들어내는 순서로 찍는다. 그 답들이 가진 순서가 아니라. 모든
  시도가 인접한 두 줄이 뒤바뀐 답이다.
- `insert_delete_05` 는 select 와 insert 를 순서 없이 남기고, 컨트롤러마다 그중 서로 다른 절반을
  잃는다.
- `delete_delete_rownum_01` 은 CTP 에서 실패하고 **여기서는 통과하는** 유일한 케이스다. 코퍼스
  전체 런 네 번 모두와 단독 세 번에서 그렇다: 그 `MC: wait until C1 ready;` 가 놀고 있는
  클라이언트의 이름을 대고 있다.

## CTP 도 재현하지 못하는 케이스들

열다섯이 더 런 사이에 판정이 바뀌었고, 이 러너에서만큼 CTP 에서도 그랬다. 각각을 러너마다 세
번씩 단독으로 다시 돌렸다:

**런마다 뒤집힌다**, 런 전체에서만큼 단독으로도:

- `_01_ReadCommitted/index_column/common_index/groupby/delete_select_06`
- `_02_RepeatableRead/no_index_column/aggregate/insert_select_02_1`
- `_02_RepeatableRead/no_index_column/basic_sql/select_insert_01`
- `_02_RepeatableRead/partition_table/range/with_index/unique_with_key/insert_update_03`
- `_02_RepeatableRead/primary_key_column/basic_sql/delete_select_14` — 통과하는 것보다 훨씬 자주
  실패한다
- `_04_RepeatableRead_ReadCommitted/no_index_column/aggregate/delete_select_03`

**단독으로는 매번 통과하고, 런 안에서 실패했다:**

- `_01_ReadCommitted/catalog/db_index_04`
- `_02_RepeatableRead/catalog/db_index_key_03`
- `_02_RepeatableRead/catalog/db_index_key_05`
- `_04_RepeatableRead_ReadCommitted/index_column/common_index/aggregate/max/insert_select_01_2`
- `_06_features/cbrd_22705_online_index_parallel/dml_online_index/insert_odku_online_index_04`

**네 슬롯일 때만 빼고 어디서나 통과한다.** 이때 슬롯마다의 `ctldb` 는 서로 다른 케이스 순서를 겪은
상태다:

- `_04_RepeatableRead_ReadCommitted/dml_ddl/createindex_02`
- `_05_ReadCommitted_RepeatableRead/dml_ddl/createindex_01`
- `_06_features/cbrd_22705_online_index_parallel/dml_ddl/createindex_02`
- `_06_features/cbrd_22705_online_index_parallel/create_ddl/show_001`

이들 중 하나의 실패는 단독으로도 실패하기 전까지는 시험 대상 변경에 대한 증거가 아니다.

## 컨트롤러가 느려서 통과하는 케이스들

이것들은 `TESTKIT_ISOLATION_CTL=1` 에서만 실패한다.
[ADR-019](../../project/adr/ADR-019-isolation-controller.md) 의 컨트롤러다. 기본 런은 ctltool 의
`qactl` 을 쓰고 이것들을 결코 보지 못한다.

케이스가 두 클라이언트에게 사이에 `MC: wait until …` 없이 연달아 구문을 주고, 답은 `qactl` 의
고정된 100 ms sleep 둘이 우연히 만들어낸 순서를 기록한 것이다. 이것은 데이터베이스의 성질이
아니고, 두 컨트롤러 어느 쪽의 결함도 아니다: 빠른 쪽은 스크립트가 두 번째 구문을 보내라고 할 때
보내고, 그것은 즉시다. 이런 케이스가 스물일곱이고, 각각 단독으로 모든 시도에 실패한다.

알아보는 표시는 diff 가 **같은 줄들을 다른 순서로** 담은 실패, 또는
`ERROR! Client <n> is ready.` 로 끝나며 락 테이블을 덤프하는 `wait until C<n> blocked` 다.

스물일곱 중 다섯이 패치로 관리되고, `_01_ReadCommitted/catalog/db_index_04` 도 그렇다. 이것은 같은
종류이면서 단독으로 돌리면 통과한다. `case_patch_dir` 을 거기로 향하게 하면 케이스마다 빠져 있던
순서 구문을 갖고 돌아간다:

```
case_patch_dir=/path/to/cubrid-testkit-patches/isolation
```

그러면 런이 그렇다고 말한다 — 첫 케이스 전에 표준 출력으로, 페이지의 완료 표에, 그리고 결과
디렉터리의 `patched.txt` 에. 패치된 케이스의 판정은 패치된 케이스에 대한 주장이지 코퍼스에 대한
주장이 아니다.

나머지 스물둘은 아직 쓰이지 않았다. 스물하나는 답을 다시 기록해야 하는데, 고치는 방법이
클라이언트가 자기만의 구문에서 스냅샷을 잡는 것이고 그런 구문은 출력을 찍기 때문이다. 스물두
번째는 엔진이 두 클라이언트 중 어느 쪽을 먼저 풀어주느냐에 달려 있다. 각각이 왜 실패하는지와
무엇에 달려 있는지는
[`project/evidence/isolation-corpus-races.md`](../../project/evidence/isolation-corpus-races.md) 에
있다.

## 케이스 하나를 다시 돌리기

케이스와 그 답들을 isolation 루트 아래의 경로를 유지한 채 자기만의 트리로 복사하고, conf 를 거기로
향하게 하라:

```bash
src=/path/to/cubrid-testcases/isolation
case=_02_RepeatableRead/catalog/db_index_key_03
mkdir -p /tmp/one/$(dirname $case)/answer
cp $src/$case.ctl /tmp/one/$(dirname $case)/
cp $src/$(dirname $case)/answer/$(basename $case).answer* /tmp/one/$(dirname $case)/answer/
printf 'scenario=/tmp/one\ntestcase_timeout_in_secs=300\ntestcase_retry_num=0\n' > /tmp/one.conf
TESTKIT_NATIVE=isolation TESTKIT_CONTAIN=1 testkit isolation -c /tmp/one.conf
```

`testcase_retry_num=0` 은 다섯 번 중 최선이 아니라 첫 시도의 판정을 보여준다. 위 표들에 있는
케이스에 대해 결론을 내리기 전에 세 번 돌려라.

## 코어와 fatal error

`$ctlpath`, `$CUBRID` 또는 케이스의 디렉터리 아래의 코어 파일, `$CUBRID/log` 안의
`FATAL ERROR`, 또는 서버가 자기 죽음에 대해 쓴 크래시 리포트는 케이스를 실패시키고, 런은 그것이
일어나는 대로 표준 오류에 말한다.

**그 점검은 이 러너의 것이고, `~/error_backup` 은 기본으로 쓰이지 않는다**
([ADR-021](../../project/adr/ADR-021-crash-reports.md)). CTP 자신의 점검과 그 백업은 스위치
하나다 — `runone.sh -n` 이 둘 다 끈다 — 그리고 그 백업은 무언가를 찾은 케이스마다 서비스를 멈추고
설치본 전체를 `~/error_backup/error_<version>_<timestamp>.tar.gz` 로 복사한다. 이 러너는 `-n` 을
넘기고, 케이스마다, 그 설치본을 가진 슬롯 안에서 스스로 찾는다:

| 무엇을 찾는가 | 케이스가 무엇이라 말하는가 | 무엇이 남는가 |
|---|---|---|
| `$CUBRID/log/coredump/*.coredump`, 엔진 자신의 리포트 | `found crash report <file> (cub_server ctldb, <frame>)` | 그 리포트, `current_runtime_logs/crash/` 안에 |
| `core.*` 파일 (CTP 자신의 패턴에서 `core.log` 를 뺀 것) | `found core file <path>` | 그 gdb 스택, 같은 디렉터리 안에. 코어 자체는 떨어진 자리에 남아 슬롯과 함께 간다 |
| `$CUBRID/log` 안의 새 `FATAL ERROR` 줄 | `found fatal error in <file> (n line(s))` | 그 개수. 로그는 슬롯의 것이다 |

**바로 앞 케이스 이후에 새로 생긴 것**만 센다: 케이스 사이에 그 자리들을 쓸어내는 것이 아무것도
없어서, 이미 fatal error 를 담고 있던 로그는 그러지 않으면 그것을 쓴 케이스 이후의 모든 케이스를
실패시킬 것이기 때문이다 — CTP 의 점검이 한 일이 그것이다.

`backup_core_file_yn=yes` 는 CTP 의 동작을 되돌려 놓는다: 그 점검과, 그것과 함께 오는 백업을.

**CTP 의 점검은 크래시 하나를 놓치고, 이 러너는 자기 것을 갖고 있다.** CTP 는 `core.*` 라는
이름의 파일과 `$CUBRID/log` 안의 `FATAL ERROR` 를 찾는다. `/proc/sys/kernel/core_pattern` 이
코어를 apport 같은 크래시 핸들러에 넘기는 곳에서는, 시그널로 죽은 서버가 `core.*` 도
`FATAL ERROR` 도 남기지 않는다 — 오직
`$CUBRID/log/coredump/cub_server_<timestamp>.coredump` 안의 자기 콜 스택만 남긴다. 이 머신에서
`partition_table/range/dml_ddl/reorganization_select_01` 이 하는 일이 그것이고
([`isolation-always-failing.md`](../../project/evidence/isolation-always-failing.md) §1), CTP 에서는
평범한 diff 로 보고된다.

케이스마다 그 뒤에 이 러너는 그 디렉터리를 스스로 읽고, **전에 본 적 없는 리포트는 케이스를
실패시킨다** ([ADR-021](../../project/adr/ADR-021-crash-reports.md)):

```
[NOK] …/reorganization_select_01.ctl
 : NOK found crash report cub_server_20260917125012.888.coredump
   (cub_server ctldb, qdata_save_agg_hentry_to_list at query_aggregate.cpp:2974)
```

리포트는 슬롯이 닫히기 전에 `current_runtime_logs/crash/<case>.<report>` 로 복사되고 — 슬롯의
설치본은 슬롯과 함께 가는 오버레이다 — 런은 표준 오류에도 그렇다고 말한다. CTP 에서 통과하면서
서버를 크래시시키는 케이스는 여기서 실패하고, 그것은 이 프로젝트가 갖기로 선택한 판정 차이다.
