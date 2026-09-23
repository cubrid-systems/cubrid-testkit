# 1. 런은 어떻게 도는가

*[English](01-how-a-run-works.md) · 한국어*

[← isolation 카테고리로](README.ko.md)

- [단계들](#단계들)
- [케이스 하나](#케이스-하나)
- [판정](#판정)
- [여기서 슬롯이란 무엇인가](#여기서-슬롯이란-무엇인가)

## 단계들

순서는 CTP 의 `TestFactory` 의 것이고, 그것이 찍는 모든 줄도 마찬가지다.

| 단계 | 무엇을 하는가 |
|---|---|
| start | `cubrid_rel` 에서 빌드를 읽고 `scenario` 를 해석한다 — `$HOME` 아래에 있으면 `$HOME` 기준 상대 경로로. 기록 안의 케이스 경로도 그래서 상대 경로가 된다 |
| `BEGIN TO CHECK` | 변수 셋, 명령 다섯, 시나리오와 ctltool 의 디렉터리를 `check_local.log` 와 콘솔로. 이것을 통과하지 못하는 머신은 exit 255 로 멈춘다 |
| `UPDATE TEST CASES` | 아무것도 하지 않는다: 케이스를 당겨오는 것도 CTP 를 올리는 것도 이 러너의 일이 아니다. CTP 는 여기서 ctltool 의 스크립트에 실행 권한도 줬는데, 러너는 그것을 슬롯마다 한다 (아래) |
| `FETCH TEST CASES` | 시나리오 아래의 모든 `*.ctl` 을 정렬한 뒤 제외 파일의 항목들을 뺀 것 — 항목 하나는 자기를 경로에 포함하는 첫 케이스 하나만, 오직 그것만 빼낸다. `dispatch_tc_ALL.txt` |
| 슬롯 | 하나 또는 `parallel_slots` 개, 각자 자기 오버레이와 함께 ([아래](#여기서-슬롯이란-무엇인가)) |
| `DEPLOY` | 모든 슬롯에서: ctltool 스크립트에 `chmod u+x`, CTP 의 프로세스 쓸어내기, `cubrid.conf` 에 `inquire_on_exit=3` 덧붙이기, 그리고 `default.*` 엔진 파라미터를 설치본의 conf 파일들에 쓰기 |
| `TEST` | 슬롯마다 큐가 빌 때까지 케이스를 가져간다 ([케이스 하나](#케이스-하나)). 끝에 `cubrid service stop` |
| end | 요약, `TEST COMPLETE`, 그리고 런 디렉터리 옆의 `isolation_result_<build>_<bits>_0_<stamp>.tar.gz` |

재개된 런 (`test_continue_yn=yes`) 은 fetch 를 먼저 한다 — `dispatch_tc_ALL.txt` 에서 모든
`dispatch_tc_FIN_*.txt` 를 뺀 것 — 그리고 머신 점검을 그 뒤에 한다. CTP 의 것이 그랬다. 로그는
새로 시작하는 대신 덧붙인다.

## 케이스 하나

러너는 CTP 가 그랬듯 케이스마다 스크립트 하나를 보낸다 — CTP 자신의 프롤로그,
`export ctlpath=${CTP_HOME}/isolation/ctltool`, 그리고:

```
ulimit -c unlimited
export TEST_ID=0
cd $ctlpath
sh runone.sh  -r 5 /path/to/case.ctl 300 qacsql 2>&1
```

`-r` 은 `testcase_retry_num` 더하기 1 이고, 마지막 인자는 클라이언트 프로그램이며,
데이터베이스는 언제나 `ctldb` 다. 그러면 `runone.sh` 는 시도마다 이렇게 한다:

1. `$ctlpath`, `$CUBRID`, 케이스의 디렉터리 아래 코어 파일들을 **지우고**, `~/CUBRID/log` 아래의
   모든 파일을 비운다;
2. `qactl` 이 없거나 `ctldb` 가 돌고 있지 않으면 **셋업한다**: `cubrid service stop`,
   `pkill -9 cub`, `$CUBRID/databases` 에서 `ctldb` 를 지우고 다시 만들고, 시작하고, `make` 로
   `qactl` 과 `qacsql` 을 빌드한다;
3. 케이스에 `<name>.sql` 이 있으면 csql 로 **돌린다**;
4. `timeout3.sh -t <timeout> qactl ctldb <case> qacsql` 을 `result/<name>.result` 로
   **실행한다**;
5. 사본 하나를 `result/<name>.log` 로 **정규화한다** — `sed` 열다섯 단계
   ([케이스 쓰기](02-writing-a-case.ko.md#답));
6. **판정한다**: 케이스에 `<name>.sh` 가 있으면 그것으로, 없으면 `ls` 순서의
   `answer/<name>.answer*` 로, 처음 동일한 것이 이긴다 — `flag: OK` 또는 `flag: NOK`;
7. `$CUBRID/log` 에서 **코어와** `FATAL ERROR` **를 찾고**, 죽어가는 서버가 쓰는 크래시 리포트도
   찾는다 — 러너 자신의 점검이며, 케이스를 실패시키고 리포트와 코어의 스택을 런과 함께 남긴다
   ([ADR-021](../../project/adr/ADR-021-crash-reports.md)). `backup_core_file_yn=yes` 는 그 점검을
   `runone.sh` 에 되돌려주고, 그러면 `runone.sh` 가 코어를 설치본 전체와 함께 `~/error_backup` 으로
   백업하고 `ctldb` 를 다시 만든다;
8. **치운다**: 남은 `sleep`·`qactl`·`qacsql` 과 아직 열려 있는 트랜잭션을 죽이고, `ctldb` 안의
   사용자, 외래 키를 가진 테이블, 트리거, 시리얼, 함수 (`sleep` 관련은 빼고), 프로시저, 뷰,
   테이블을 `csql` 호출 열일곱 번에 걸쳐 없앤다.

통과하는 첫 시도에서 멈춘다.

## 판정

러너는 CTP 의 `Test.java` 가 그랬듯 스크립트의 출력에서 판정을 읽는다: **마지막 `flag: OK` 뒤의
마지막 `flag: NOK` 는 실패, `found core file` 이나 `found fatal error` 는 실패, `flag: OK` 가
하나도 없으면 실패** (`Not found OK word.`). 실패면 이어서 기준 답과 정규화된 결과를 185 칼럼
너비로 diff 해 `feedback.log` 에 넣는다.

스크립트가 찍은 것은 두 마커 사이의 표준 출력을 다듬은 것이다 — CTP 의 SSH 층이 로컬 머신에까지
적용하던 규칙이다. 표준 오류는 판정에 닿지 않는다. `runone.sh` 자신의 trace 는 닿는데, 명령이
`2>&1` 로 끝나기 때문이다.

## 여기서 슬롯이란 무엇인가

슬롯은 shell 의 슬롯이다 — PID·IPC·network·mount 네임스페이스, 모든 슬롯이 출하된 포트 그대로 —
여기에 다음 위로 오버레이가 얹힌다:

| | 왜 |
|---|---|
| `$CUBRID` | `ctldb`, 로그, 그리고 Deploy 가 덧붙이는 conf 줄이 슬롯의 것이라서 |
| `$CTP_HOME/isolation/ctltool` | `make clean qactl qacsql`, 그리고 `runone.sh` 가 자기 작업 디렉터리에 두는 로그들 |
| 케이스 트리, `scenario_disk` 가 있을 때 | `result/` 와 `<name>.result`. 그러지 않으면 CTP 에서처럼 트리 안에 떨어진다 |
| `~/error_backup` | 슬롯 자신의 디렉터리에 대한 bind. 슬롯이 닫힐 때 진짜 디렉터리로 복사된다 |
| `~/CUBRID/log` | 그것이 시험 대상 설치본의 것이 아닐 때, 빈 디렉터리 |

슬롯들은 큐 하나와 EnvId 하나를 공유하므로, 병렬 런은 직렬 런이 쓰는 것과 같은 파일들을 쓴다.
슬롯마다 자기 첫 케이스에서 ctltool 을 빌드하고 `ctldb` 를 만든다.
