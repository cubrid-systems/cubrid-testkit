# 2. `ha-shell` 케이스 쓰기

*[English](02-writing-a-case.md) · 한국어*

[← ha-shell 카테고리로](README.ko.md)

`ha-shell` 케이스는 두 번째 노드를 가진 [`shell` 케이스](../shell/02-writing-a-case.ko.md)다. 그
문서를 먼저 읽어라: 배치, 네 줄의 틀, 판정 헬퍼, 함정들이 여기서도 전부 같고, 이 문서는 페어가
더하는 것만 다룬다.

- [두 경로](#두-경로)
- [배치](#배치)
- [두 머신 경로에서 케이스가 받는 verb 들](#두-머신-경로에서-케이스가-받는-verb-들)
- [규칙 셋, 그리고 코퍼스는 셋 다 어긴다](#규칙-셋-그리고-코퍼스는-셋-다-어긴다)
- [두 머신 경로 위의 케이스](#두-머신-경로-위의-케이스)
- [샌드박스 경로를 위해 쓰기](#샌드박스-경로를-위해-쓰기)
- [함정들](#함정들)

## 두 경로

**샌드박스 경로가 지금부터 쓰이는 모든 것의 기본이다**
([ADR-022](../../project/adr/ADR-022-topology-provider.md)). 페어는 만들어지는 대신 요청되고,
러너가 바깥에서 두 노드를 몰며, 케이스가 슬레이브로 스스로 건너가는 일은 없다.

**동결된 케이스들은 두 머신에 머무르며**, 그것은 선호 때문이 아니다. 그것들은 CTP 의 Java SSH
헬퍼를 셸로 불러서 슬레이브에 닿으므로, 노드가 QA 머신이어야 한다: 비밀번호 인증이 되는 sshd,
JVM, `expect`, 그리고 CTP 의 트리. 샌드박스 노드는 그중 어느 것도 아니고
([`ha-topology.md`](../../project/evidence/ha-topology.md) §3), 코퍼스는 동결돼 있으므로, 그
케이스에게 슬레이브에 닿는 다른 방법을 가르칠 수 없다.

**샌드박스 경로에서 무엇이 만들어졌고 무엇이 아닌가.** 이번 주에 돌아야 하는 일에 이 경로를
고르기 전에 이것을 분명히 해두라:

| | |
|---|---|
| 만들어졌고, 살아 있는 페어에 대해 증명됨 | `internal/sandbox` — 클러스터를 bind 하고, 그 토폴로지를 CTP 조각으로 읽고, 아무 노드에서나 명령을 돌리고, 복제를 기다리고, 두 노드에 걸쳐 쿼리를 비교한다. 그 `TestLive*` 테스트들이 실제 클러스터에 대고 돈다 |
| **만들어지지 않음** | **스위트를 클러스터로 라우팅하는 것 무엇이든.** 그것의 이름을 대는 설정 키가 없고, 그 패키지를 import 하는 task 가 없다. 케이스를 아직 이 방식으로 *돌릴* 수 없다 |
| 역시 없는 것 | 동결된 케이스들을 대조할 두 머신 기준선 ([`evidence/ha/`](../../project/evidence/ha/README.md)). 어느 쪽 경로든 판정이 그대로라고 주장하려면 먼저 갚아야 할 빚이다 |

그러니 오늘 샌드박스 경로를 위해 쓴 케이스는 명세된 모양과 측정된 이음매에 대고 쓰는 것이며,
라우팅을 기다린다. 그것은 실재하는 기다림이고, 이 카테고리에 *3. 돌리기* 가 없는 이유다.

## 배치

`shell` 과 동일하다. task 가 `shell` 이기 때문이다:

```
<scenario>/
  └── my_ha_case/
        └── cases/
              ├── my_ha_case.sh       ← THE CASE. Named after the directory two levels up
              └── my_ha_case.result   ← written by the run. Do not commit it
```

발견 규칙, 반복되는 이름, 헬퍼와 케이스의 구분은 전부
[shell 의 규칙](../shell/02-writing-a-case.ko.md#배치-그리고-이름이-반복되는-이유)이다. 바뀌는
것은 `shell.conf` 자리에 오는 `ha_shell.conf`, 그리고 `HA/shell` 트리를 가리키는 코퍼스다.

## 두 머신 경로에서 케이스가 받는 verb 들

`make_ha.sh` 가 `$init_path/HA.properties` 를 읽고, `make_ha_upper.sh` 를 source 해서, 이것들을
스코프에 남긴다. 개수는 동결된 373 케이스에 대한 것이고, 이 스위트가 실제로 무엇을 하는지의
지도로 읽을 값이 있다:

| verb | 케이스 | 무엇인가 |
|---|---:|---|
| `setup_ha_environment` | 370 | 두 노드에 데이터베이스를 만들고, conf 파일 넷을 다시 쓰고, 업로드하고, 하트비트를 시작하고, active 가 될 때까지 `changemode` 를 폴링한다 |
| `revert_ha_environment` | 368 | 그 역 |
| `run_on_slave` | 248 | 다른 노드에서 명령 하나 |
| `wait_for_slave` | 131 | **슬레이브의 상태가 요청된 것이 될 때까지.** 규칙 1 참조 |
| `run_upload_on_slave` · `run_download_on_slave` | 100 · 48 | 파일을 옮긴다 |
| `wait_for_active` · `wait_for_slave_active` | 81 · 13 | `changemode` 가 active 라고 할 때까지 |
| `start_slave_hb` · `stop_slave_hb` | 74 · 52 | 다른 노드에서 하트비트를 |
| `stop_slave_service` | 43 | 다른 노드에서 서비스를 |
| `slave_cmd` | 25 | `run_on_slave` 와 같되 엔진의 환경과 함께 |
| `format_hb_status` | 12 | `hb status` 를 비교 가능하게 만든다 |
| `add_ha_db` | 9 | `ha_db_list` 에 데이터베이스 하나 더 |

그중 셋이 진짜 동기화다: 한계를 두고 상태가 참이 될 때까지 폴링한다 — `wait_for_active` 는
`cubrid changemode` 를 1 초마다 120 번 돌린다.

**373 이라는 수에 대하여.** 이 페이지의 모든 개수는
[`module-ha.md`](../../project/design/module-ha.md) 가 측정한 373 케이스에 대한 것이다.
[ADR-022](../../project/adr/ADR-022-topology-provider.md) 와
[`ha-topology.md`](../../project/evidence/ha-topology.md) 는 같은 트리로 읽히는 것에 대해 367 이라고
한다. 둘은 아직 맞춰지지 않았고 이 페이지는 하나를 고르지 않는다 — 아래 규칙들이 기대고 있는 것은
비율이고, 그것은 어느 쪽이든 성립한다.

## 규칙 셋, 그리고 코퍼스는 셋 다 어긴다

이것들은 스타일이 아니다. 각각은 그러지 않았다면 베껴 왔을 트리 안의 측정된 결함이다.

### 1. 동기화하려고 `sleep` 하지 마라

| | |
|---|---:|
| 맨 `sleep N` 을 가진 케이스 | **373 중 244** |
| 맨 `sleep` 구문 | **726** |
| 코퍼스 전체에서 잠든 초를 합한 것 | **20,370 — 5 시간 39 분** |
| 진짜 기다림을 쓰는 케이스 | 136 |
| **둘 다** 쓰는 케이스 | 91 |

여기서의 sleep 은 올바른 테스트에 덧대어진 지연이 아니다. **그것이 동기화다** — 케이스가 마스터에
쓰고, 5 초 자고, 슬레이브를 읽고, 복제가 따라잡았는지는 결코 확인하지 않는다. 그것은 양쪽으로
틀렸다: 부하가 걸린 머신에서는 엔진의 것이 아닌 이유로 실패하고, 빠른 머신에서는 아무것도 기다리지
않은 채 통과한다.

**`wait_for_slave` 를 쓰라.** 그것은 타이머가 아니다: 마스터에 테이블을 만들고,
`'replication finished'` 행을 넣고, `-tillcontains` 로 슬레이브를 폴링해 그것이 도착할 때까지
기다린다 (`make_ha_upper.sh:74-87`). 그것은 몇 년 동안 모든 케이스 옆에 놓여 있었다. 네 케이스에
대해 측정한 결과, 그 기다림은 **대체한 sleep 이 5 초에서 30 초를 쓰던 자리에서 약 1.3 초**가
들었고, 판정은 하나도 바뀌지 않았다
([`evidence/ha/p1-sleep-to-wait.md`](../../project/evidence/ha/p1-sleep-to-wait.md)).

그리고 둘 다 하지는 마라. 91 케이스가 `wait_for_slave` 를 부르고 나서도 5 초를 잔다.

### 2. 케이스는 실패할 수 있어야 한다

커밋하기 전에 돌려라:

```bash
testkit check-cases <scenario> [<init_path>]
```

엔진을 읽지 않고 케이스를 돌리지도 않는다. 세 가지를 찾아낸다: NOK 로 가는 길이 아예 없는 케이스,
판정 헬퍼에서 오타 하나 떨어져 있으면서 어디에도 정의되지 않은 호출, 그리고 무언가를 자기 자신과
비교하는 것. HA 코퍼스에 대한 첫 실행에서 하나를 찾았다 — `write_nok` 자리의 `wirte_nok`, `if`
의 `else` 가지 안에서. 다시 말해 **실패를 보고했을 유일한 줄 안에서.**

답 파일 코퍼스였다면 그것이 드러났을 것이다. 이 코퍼스는 그럴 수 없다. 오라클이 케이스가 스스로
수행하는 비교이기 때문이다 — 그것이 바로 [오라클](README.ko.md#오라클-그리고-그것이-왜-좋은-부분인가)이
하는 거래이고, 이 점검이 존재하는 이유다.

### 3. 오라클은 페어이지 파일이 아니다

마스터에 쓰고, **두** 노드에서 같은 행을 읽고, 둘을 비교하라. 373 중 169 케이스가 판정에 이르는
방법이 `compare_result_between_files` 다. 기대 출력 파일을 들여오지 마라: 그 순간 케이스는 엔진의
출력 형식이 바뀔 때 다시 기록할 것을 갖게 되고, 이 스위트를 싸게 만드는 성질은 사라진다.

## 두 머신 경로 위의 케이스

```bash
#!/bin/bash
. $init_path/init.sh
init test
set -x

. $init_path/make_ha.sh
setup_ha_environment

csql -u dba -c "CREATE TABLE t(i INT PRIMARY KEY); INSERT INTO t VALUES (1),(2),(3);" $ha_db

# Rule 1. Not `sleep 5`.
wait_for_slave

csql -u dba -t -N -c "SELECT count(*) FROM t;" $ha_db          > master.log 2>&1
run_on_slave "csql -u dba -t -N -c \"SELECT count(*) FROM t;\" $ha_db" > slave.log 2>&1

# Rule 3. The two nodes are the oracle.
compare_result_between_files master.log slave.log

revert_ha_environment
finish
```

`compare_result_between_files` 가 판정을 직접 기록하고, 그래서 여기에 `write_ok` 이 없다 — 그리고
그것이 `testkit check-cases` 가 이 케이스도 실패할 수 있음을 보려면 판정을 *이행적으로* 따라가는
법을 배워야 했던 이유다.

## 샌드박스 경로를 위해 쓰기

케이스가 취하는 모양이 바뀐다. 케이스가 슬레이브에 닿는 주체이기를 그만두기 때문이다.

**페어를 세워라**, 한 번, 런 바깥에서:

```bash
make -C extensions/cluster-sandbox dist
export CSB_HOME=/somewhere
extensions/cluster-sandbox/bin/csb cluster create --name tkha --build /path/to/install.out
```

**그러면 러너가 갖는 것.** `csb cluster describe --format ctp --instance instance1` 이 토폴로지를
`ha_repl.conf` 조각으로 렌더링한다 — `env.instance1.master.ssh.host`, `.slave.ssh.host`,
`ha_db_list` — 그것은 CTP 의 동결된 키 집합이므로, 새 파서도 없고 노드가 무엇인지에 대한 두 번째
모델도 없다. `ssh.host` 는 *이 instance 가 어디 있는지* 라는 뜻을 유지하고 *sshd 가 응답하는
주소* 라는 뜻을 그만둔다: 노드들은 sshd 를 돌리지 않고, 채널은 `csb node exec` 다.

`internal/sandbox` 가 그것을 `Pair` 로 바꾸고, 케이스의 세 가지 필요가 거기에 그대로 대응된다:

| 케이스가 원하는 것 | 두 머신 경로에서 | 샌드박스 경로에서 |
|---|---|---|
| 마스터에서 무언가 돌리기 | `csql ...` | `Pair.MasterChannel().Run(ctx, ...)` |
| 슬레이브에서 무언가 돌리기 | `run_on_slave "..."` | 슬레이브의 `Node`, 네 번째 `exec.Channel` |
| 복제 기다리기 | `wait_for_slave` | `Pair.WaitForReplication(ctx, timeout)` |
| 두 노드 비교하기 | `compare_result_between_files` | `Pair.SameOnBothNodes(ctx, query)` |

`WaitForReplication` 은 세 번째 착상이 아니라 이미 존재하는 두 기다림과 의도적으로 같은 모양이다:
호출마다 만들어지는 마커 테이블, 마스터에 쓰고, 슬레이브에서 폴링하고, 한계를 둔다. CTP 의
`wait_for_slave` 와 `ha_repl` 의 `Test.java` 가 독립적으로 거기 도달했고, 그것이 이 모양이 옳다는
논거다.

**이 채널에는 `Put` 과 `Get` 이 없고, 근사되는 대신 거부된다.** csb 에는 파일 전송 verb 가 없다.
`run_upload_on_slave` 가 필요한 케이스는 오늘 샌드박스 등가물이 없다 — 그것은 실재하는 공백이지
빠뜨린 것이 아니며, `node exec` 를 통한 base64 는 인자 목록이 더 이상 들어맞지 않을 때까지만
동작하기 때문에 기각되었다.

**아직 할 수 없는 것:** 케이스를 돌리는 것. 스위트를 클러스터로 라우팅하는 것이 아무것도 없다.
그렇게 되기 전까지, 여기에 쓰는 일의 쓸모 있는 산출은 케이스의 모양과 그것이 드러내는 공백이다.

## 함정들

- **끝나지 않은 셋업 전이는 비교를 무의미하게 만들고, 초록으로 만든다.** 263 케이스가 토폴로지를
  움직이고 나서 데이터를 비교한다. 그 움직임이 아직 진행 중이면, 비교는 케이스가 생각하는 것이
  아직 아닌 노드를 재고 있는 것이다. 규칙 1 은 쓰기에만이 아니라 셋업 전이에도 적용된다.
- **`kill -9` 는 분할이 아니다.** 코퍼스가 만들어낼 수 있는 모든 fault 는 프로세스 수준이다.
  `iptables`, `ip route`, `tc qdisc` 는 373 케이스 중 **0** 개에 나온다. 사라진 노드와, 살아
  있으면서 상대가 사라졌다고 믿는 노드는 서로 다른 코드 경로다. 두 번째가 필요하다면 group B 와
  fault verb 가 필요하다 — [`module-ha.md`](../../project/design/module-ha.md) §4.
- **`to_be_active` 는 이 코퍼스에 보이지 않는다.** 그것을 기다리거나, 단언하거나, 시간을 재는
  케이스가 0 이고, 현장의 트래커 자신은 페일오버가 거기서 몇 시간 멈춘 것을 기록하고 있다. 당신
  케이스의 전제가 절반만 끝난 전이라면, 기존 트리의 무엇도 그 방법을 보여주지 않는다. 기존 트리의
  무엇도 그것을 하지 않기 때문이다.
- **하트비트 파라미터를 바꿔놓고 문서에 적힌 산술을 기대하지 마라.**
  `ha_calc_score_interval_in_msecs`, `ha_max_heartbeat_gap`, `ha_heartbeat_interval_in_msecs` 는 0
  개 케이스에 나오고, cluster-sandbox 는 열아홉 번의 런에 걸쳐 둘 중 어느 하트비트 파라미터를 네
  배로 올려도 결과가 자기 기준선 밴드 안에 머무른다는 것을 측정했다. 그것들을 바꾸는 케이스의 런
  한 번은 증거가 아니다. 증거는 분포다.
