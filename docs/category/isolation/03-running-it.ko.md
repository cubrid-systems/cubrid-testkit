# 3. 돌리기

*[English](03-running-it.md) · 한국어*

[← isolation 카테고리로](README.ko.md)

- [필요한 것](#필요한-것)
- [런 하나](#런-하나)
- [지켜보기](#지켜보기)
- [슬롯](#슬롯)
- [남기는 것](#남기는-것)

## 필요한 것

| | |
|---|---|
| 설치본 | `$CUBRID`, 그리고 헤더가 `$CUBRID/include` 에 있을 것: 모든 슬롯이 그것에 대고 ctltool 을 빌드한다 |
| CTP | `isolation/ctltool` 을 가진 CTP 트리를 가리키는 `$CTP_HOME` — `runone.sh`, 스크립트들, 그리고 `qactl` 과 `qacsql` 의 C 소스 |
| 코퍼스 | `cubrid-testcases`, 그 안의 `isolation` 디렉터리 |
| C 툴체인 | `gcc` 와 `make` |
| CTP 의 점검이 보는 것 | `$JAVA_HOME` 이 설정돼 있을 것, 그리고 `java`, `diff`, `wget`, `find`, `cat` 이 경로에 있을 것. 러너는 Java 를 쓰지 않는다. 그 점검은 CTP 의 것이고 있던 그대로 유지된다 |
| 격리 | `TESTKIT_CONTAIN=1`. **필수**: `runone.sh` 는 그 사용자가 가진 모든 `cub`·`sleep`·`qactl`·`qacsql` 을 죽이고, 그것을 이 런 안에 가둬두는 것은 네임스페이스뿐이다 |

`TESTKIT_NATIVE=isolation` 은 이 task 를 CTP 에 넘기는 대신 이 프로그램이 돌린다고 말하는
게이트다.

## 런 하나

```bash
export CUBRID=/path/to/CUBRID
export CUBRID_DATABASES=$CUBRID/databases
export CTP_HOME=/path/to/CTP
export JAVA_HOME=/usr/lib/jvm/java-8-openjdk-amd64
export PATH=$CTP_HOME/bin:$CTP_HOME/common/script:$CUBRID/bin:$JAVA_HOME/bin:$PATH

TESTKIT_NATIVE=isolation TESTKIT_CONTAIN=1 testkit isolation -c isolation.conf
```

CTP 의 데일리 런이 하는 대로 코퍼스를 돌리는, 페이지까지 켠 conf:

```
scenario=/path/to/cubrid-testcases/isolation
testcase_timeout_in_secs=300
testcase_retry_num=4
testcase_exclude_from_file=/path/to/cubrid-testcases/isolation/config/daily_regression_test_excluded_list_linux.conf
status_http=on
```

슬롯에 대해서는 아무 말도 하지 않으므로, 이 런은 기본값을 취한다 ([아래](#슬롯)).

런은 케이스가 실패했든 아니든 0 으로 끝난다. CTP 의 것이 그렇다. 실패한 머신 점검, 읽을 수 없는
빌드, 러너가 거부하는 설정은 255 로 끝난다. 없는 시나리오 디렉터리는 `[ERROR]` 를 찍고 0 으로
끝난다 — CTP 의 동작이고, exit code 가 동결된 표면의 일부라서 그대로 두었다.

## 지켜보기

`status_http=on` 은 `127.0.0.1:51523` 에 페이지를 연다 — 두 번째 런은 다음 빈 포트로 옮겨간다 —
그리고 어디인지를 표준 오류에 말한다. 표준 출력은 CTP 의 것이기 때문이다. 포트만 주면
(`status_http=51537`) 모든 인터페이스에서 듣는다. 슬롯마다 무엇을 돌리고 있는지, 속도, 그리고
떨어지는 대로의 실패들을 보여준다. 끝난 케이스를 클릭하면 그 케이스의 `feedback.log` 블록이,
실패라면 diff 와 함께 보인다. 돌고 있는 케이스는 `runone.sh` 가 쓰고 있는 원본 결과를 보여준다.

## 슬롯

**슬롯이 기본이고, 그 수는 머신이 정한다.** conf 가 `parallel_slots` 를 설정하지 않은 런은 다음
중 가장 작은 것을 취한다: 가용 메모리에서 나머지 전부를 위해 2 GB 를 뺀 것을 이 머신 자신의
런에서 슬롯 하나가 들었던 값으로 나눈 수; CPU 하나당 슬롯 하나; 머신의 첫 런에서는 넷, 그
뒤로는 지금까지 돌린 최대치의 두 배; 그리고 같은 코퍼스의 런이 가장 빨랐던 수 — 슬롯을 더 늘려도
더 빠르지 않다고 측정된 경우에 한해서. 무엇을 골랐고 왜인지는 표준 오류에 말한다:

```
[INFO] 12 slot(s): 12 by memory (23212 MB less 2048, at 1739 MB a slot, this machine's own runs)
```

이 코퍼스를 아직 돌린 적 없는 머신은 슬롯당 1,750 MB 로 계산된다. 완료된 런은 모두 자기가 쓴
것을 기록한다 — 가용 메모리가 얼마나 떨어졌는지, 벽시계 시간, 케이스 초, 첫 시도에 통과한 가장
긴 케이스, 슬롯들이 어디에 썼는지 — `~/.local/state/testkit/sizing/isolation.json` (또는
`$XDG_STATE_HOME/testkit/sizing/`, 또는 `$TESTKIT_SIZING_DIR`) 에. 그리고 그 머신의 다음 런은
적어도 이만큼 큰 코퍼스에 대한 런들 중 슬롯당 최대 피크에 15% 를 얹어 계산된다
([ADR-020](../../project/adr/ADR-020-sizing.md)). 이 파일은 체크아웃 안에 있지 않고 다른 머신에서는
무시된다. `testkit sizing isolation` 은 그것과, 세 가지 `parallel` 낱말이 각각 무엇을 정할지를
찍는다:

```
  conservative   6 slots -- 6 by memory (23212 MB less 2048, at 3478 MB a slot, this machine's own runs), sized conservative
* measured      12 slots -- 12 by memory (23212 MB less 2048, at 1739 MB a slot, this machine's own runs)
  aggressive    15 slots -- 15 by memory (23212 MB less 2048, at 1391 MB a slot, this machine's own runs), sized aggressive
```

`parallel=conservative` 는 다른 무언가가 쓰고 있는 머신을 위한 것이다: 예산을 두 배로 잡고 지금까지
돌린 것을 결코 넘지 않는다. `aggressive` 는 천장을 피하는 대신 찾아낸다: 예산의 0.8, 돌린 것의 네
배, 그리고 꺾이는 점은 없다. `parallel_slots=1` 은 CTP 가 하듯 직렬로 돌린다. 그 밖의 값은 적힌
대로 쓰인다. 슬롯마다 자기 설치본 오버레이, 자기 `ctldb`, 자기 ctltool 빌드를 갖고, 그 빌드는 자기
첫 케이스에서 만든다. 이 머신에서 측정한 값
([`isolation-baseline.md`](../../project/evidence/isolation-baseline.md)):

| | 한 슬롯 | 네 슬롯 |
|---|---:|---:|
| 60 케이스 표본 | 76 s | 38 s, 판정과 결과는 동일 |
| 코퍼스 전체, 6,772 케이스 | 12,301 s | 2,978 s |

CTP 단독은 코퍼스 전체에 11,095 s 가 걸렸다. 네 슬롯에서는 다른 모든 곳에서 통과하는 케이스 넷이
실패했는데, 각각은 슬롯의 `ctldb` 가 서로 다른 케이스 순서를 겪은 뒤 행이 다른 순서로 돌아온
목록이다. 단독으로는 두 러너 모두에서 통과한다
([케이스가 실패할 때](05-when-a-case-fails.ko.md#ctp-도-재현하지-못하는-케이스들)).

이득을 정하는 것은 케이스가 무엇을 기다리느냐다. isolation 케이스의 대부분은 기다림이다 — 락을,
`sleep` 을, 컨트롤러를 — 그래서 슬롯은 CPU 를 거의 더하지 않는다. 대신 천장을 정하는 것은 둘이다:

- **메모리.** 슬롯마다 설치본의 버퍼로 `ctldb` 의 `cub_server` 를 하나 시작한다. 출하된
  `cubrid.conf` 는 `data_buffer_size=512M` 과 `log_buffer_size=256M` 을 요구한다. conf 의
  `default.cubrid.data_buffer_size` 가 모든 슬롯에 대해 그것을 낮춘다.
- **타이밍.** 타이밍으로 뒤집히는 케이스들은 부하가 걸린 머신에서 더 뒤집힌다. 병렬 런과 직렬
  런을 비교할 때는 판정 대 판정이 아니라
  [ADR-018](../../project/adr/ADR-018-isolation-equivalence.md) 의 규칙으로 하라.

`scenario_disk=on` 은 슬롯마다 케이스 트리 위에 자기 층을 준다. `TESTKIT_SLOT_VOLATILE=1` 은 슬롯
층에 대한 sync 가 즉시 돌아오게 한다. 둘 다 [설정](04-configuration.ko.md)에 있다. volatile 은
isolation 을 더 빠르게 만들지 않는다: 여덟 슬롯으로 코퍼스 전체를 돌렸을 때 케이스 초의 1% 미만을
아꼈다. sql 의 케이스들이 반으로 줄어든 것과 달리
([`isolation-controller.md`](../../project/evidence/isolation-controller.md) §8).

## 남기는 것

- 런 디렉터리, `$CTP_HOME/result/isolation/current_runtime_logs`: `check_local.log`,
  `dispatch_tc_ALL.txt`, `dispatch_tc_FIN_local.txt`, `feedback.log`, `main_snapshot.properties`,
  `test_local.log`, `test_status.data`
- 그 옆의 `isolation_result_<build>_<bits>_0_<stamp>.tar.gz`
- 케이스 트리 안에, `scenario_disk` 가 켜져 있지 않다면: 케이스마다 `result/<name>.result`,
  `result/<name>.log`, `<name>.result` — 코퍼스 전체로 약 2 만 개 파일
- `~/error_backup` 안에, 코어를 덤프한 케이스의 백업
- 설치본과 CTP 트리 안에는 아무것도: `ctldb`, Deploy 가 덧붙인 conf 줄, ctltool 빌드와 `runone.sh`
  의 로그는 전부 슬롯의 층에 있었고 슬롯과 함께 갔다
