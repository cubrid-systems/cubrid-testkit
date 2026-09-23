# 2. 케이스 쓰기

*[English](02-writing-a-case.md) · 한국어*

[← shell 카테고리로](README.ko.md)

이 러너가 시험되는 코퍼스 — `cubrid-testcases-private-ex` — 는 비공개다. 그래서 이 문서는 그것을
읽지 않고도 케이스를 쓸 수 있도록 하는 것이다.

- [배치, 그리고 이름이 반복되는 이유](#배치-그리고-이름이-반복되는-이유)
- [복사해서 바로 돌릴 수 있는 케이스](#복사해서-바로-돌릴-수-있는-케이스)
- [모든 케이스가 시작하고 끝내는 네 줄](#모든-케이스가-시작하고-끝내는-네-줄)
- [판정 기록하기](#판정-기록하기)
- [기대 파일과 비교하기](#기대-파일과-비교하기)
- [케이스가 전제해도 되는 것](#케이스가-전제해도-되는-것)
- [플랫폼에서 케이스 건너뛰기](#플랫폼에서-케이스-건너뛰기)
- [함정들](#함정들)

## 배치, 그리고 이름이 반복되는 이유

```
<scenario>/
  └── my_first_case/                    ← 케이스 디렉터리. 그 이름이 케이스 이름이다
        └── cases/
              ├── my_first_case.sh      ← 케이스 본체. 두 단계 위 디렉터리의 이름을 딴다
              ├── my_first_case.result  ← 런이 쓴다. 커밋하지 말 것
              ├── expected.answer       ← 케이스가 diff 하고 싶은 무엇이든
              └── helper.sh             ← 케이스가 아니다. 다른 이름은 전부 헬퍼
```

**이름이 반복되는 것은 관례가 아니라 규칙이다.** CTP 의 `do_check_more_errors` 는 결과 파일을
*디렉터리* 이름에서 끌어내고, 러너는 *스크립트* 이름에서 끌어낸다:

```
result_file_full_name=${test_case_dir%/cases*}/cases/${case_name}.result
```

둘은 스크립트가 자기 디렉터리의 이름을 딸 때만 일치한다. 그렇게 이름 붙지 않은 스크립트는
**아무도 쓰지 않은 파일에서 판정을 읽는 케이스**가 된다. 그래서 발견 규칙은 정확히 이렇다:

```
<scenario>/**/<name>/cases/<name>.sh
```

`cases/` 아래의 다른 모든 `*.sh` 는 헬퍼다. 이것이 `PrintInfo.sh`, `common.sh`, `build.sh` 가
테스트로 실행되지 않게 하는 장치다.

**런이 읽을 수 없는 디렉터리는 건너뛰고, 이름을 댄다.** root 로 돈 런은 케이스의 `cases/` 아래에
데이터베이스와 로그를 `root:root 700` 으로 남기고, 런의 uid 는 거기 들어갈 수 없다. 발견 단계는
경로와 함께 `[WARN]` 을 찍고 나열할 수 있었던 케이스들로 계속 간다. **그런 디렉터리 안의 케이스는
발견되지 않으므로 그 경고는 읽을 값이 있다.** `testcase_workspace_dir` 이 시나리오가 아닌 다른
디렉터리를 가리킬 때의 작업공간 복사본도 마찬가지다.

케이스 디렉터리 위의 깊이는 마음대로다. 원하는 대로 묶으면 된다.

## 복사해서 바로 돌릴 수 있는 케이스

완전히 동작하는 케이스다. 데이터베이스를 만들고, 질문하고, 판정을 기록하고, 정리한다.

```bash
#!/bin/bash
. $init_path/init.sh
init test
set -x

cubrid_createdb --db-volume-size=20M --log-volume-size=20M demodb
cubrid server start demodb

csql -u dba demodb -c "CREATE TABLE t(i INT); INSERT INTO t VALUES (1),(2);"
csql -u dba demodb -c "SELECT count(*) FROM t;" > actual.log 2>&1

if grep -qE "^ +2$" actual.log; then
    write_ok
else
    write_nok "expected count(*) = 2"
fi

cubrid server stop demodb
cubrid deletedb demodb
finish
```

`<scenario>/my_first_case/cases/my_first_case.sh` 에 두고 돌리면 된다 — 두 명령은
[돌리기](03-running-it.ko.md)에 있다. 이런 것이 남는다:

```
my_first_case-1 : OK
19:11:06----<scenario>/my_first_case/cases--- time=13
```

## 모든 케이스가 시작하고 끝내는 네 줄

```bash
. $init_path/init.sh     # 헬퍼들. $init_path 는 당신을 위해 export 돼 있다
init test                # cur_path, 결과 파일, case_no = 1 을 설정한다
set -x                   # 실패가 읽히도록. 코퍼스의 모든 케이스가 이렇게 한다
...
finish                   # 서비스를 멈추고, 브로커 공유 메모리를 풀고,
                         # 케이스가 바꾼 모든 conf 를 되돌린다
```

`$init_path` 는 스크립트가 돌기 전에 러너가 설정하고, `CTP_HOME` 과 CTP 의 `bin`·
`common/script` 가 올라간 `PATH` 도 함께 설정한다. **당신이 설정하지 않으며, 해서도 안 된다.**

`finish` 는 선택이 아니다. 서비스를 멈추고, 남은 것을 `pkill cub` 하고, 브로커 공유 메모리를
풀고, conf 파일을 되돌린다 — 이것을 건너뛴 케이스는 **다음 케이스에게 더러운 슬롯을 넘긴다.**

## 판정 기록하기

한 스크립트가 판정을 **여럿** 기록할 수 있다. `case_no` 는 1 에서 시작해 호출할 때마다 오른다.

```bash
write_ok                            # my_first_case-1 : OK
write_ok "created 4 volumes"        # my_first_case-2 : OK created 4 volumes
write_nok                           # my_first_case-3 : NOK
write_nok "expected count(*) = 2"   # my_first_case-4 : NOK expected count(*) = 2
write_nok some_file.log             # 파일 이름은 결과 안에 내용이 펼쳐진다
```

`$case_name.result` 에 덧붙이고, 러너는 그것을 읽어 판정을 정한다. **한 줄이라도 NOK 면 케이스
전체가 NOK 다.**

`write_nok` 은 `$CUBRID/log/server/*.err` 에서 `Internal Error` 도 grep 해 찾은 것을 덧붙인다.
서버 쪽 결함이 그것을 드러낸 실패 바로 옆에 놓이도록.

## 기대 파일과 비교하기

```bash
compare_result_between_files actual.log expected.answer
```

둘을 diff 하고 대신 `write_ok` 나 `write_nok` 을 불러준다. 선택 인자가 둘:

```bash
compare_result_between_files actual.log expected.answer error       # 줄 번호를 무시
compare_result_between_files actual.log expected.answer error sort  # 둘 다 먼저 정렬
```

**인자 순서.** 판정은 어느 쪽이든 같다 — diff 는 diff 다 — 그러나 출력의 어느 쪽이 무엇인지를
순서가 정하고, **코퍼스는 그것에 일관되지 않다.** 전제하지 말고 케이스를 읽어라:
`compare_result_between_files a.log a.answer` 는 *실제* 를 앞에 두는 흔한 형태이고, 반대도
존재한다.

**버전별 답을 고르려면 `$CUBRID/qa.conf` 가 필요하다.** `get_best_compat_file` 이 거기서
`Server_Version` 과 `CCI_Version` 을 읽으므로, 케이스는 `expected.answer`,
`expected.answer.11.2` 같은 것을 가질 수 있고 맞는 것이 선택된다. `qa.conf` 가 없으면 그냥
평범한 파일이 쓰인다.

## 케이스가 전제해도 되는 것

| | |
|---|---|
| 작업 디렉터리 | 자기 `cases/` 디렉터리. 모든 케이스가 거기서 시작한다 |
| `$CUBRID` | 자기 데이터베이스가 하나도 없는 설치본. 이 케이스가 돌기 전에 복원됐다 |
| `$CUBRID_DATABASES` | `$CUBRID/databases`. `databases.txt` 하나가 있고 그 안은 비었다 |
| 포트 | 1523 이 비어 있고, 이 케이스만의 것이다 |
| 프로세스 | 돌고 있는 CUBRID 프로세스가 없다 |
| `$init_path`, `$CTP_HOME`, `$PATH` | 설정돼 있고 CTP 스크립트에 닿는다 |
| `/bin/sh` | bash 호환. `/bin/sh` 가 dash 인 배포판에서도 |
| 명령들 | `java`, `javac`, `diff`, `wget`, `find`, `cat`, `kill`, `tar`, `expect` — 런이 한 번 점검하고, 하나라도 없는 머신은 거부한다 |

**모든 케이스가 머신을 원래대로 돌려받는다.** 슬롯은 케이스마다 리셋되므로, 케이스는
`cubrid.conf` 를 바꾸고, 데이터베이스를 만들고, 파일을 남겨도 된다 — 다음 케이스는 그것을 보지
않는다. 그것은 동시에 **케이스가 앞선 케이스가 남긴 무엇에도 기대면 안 된다**는 뜻이다.

## 플랫폼에서 케이스 건너뛰기

스크립트에 맨 단어를 넣고 conf 에 `testcase_exclude_by_macro` 를 설정한다:

```bash
LINUX_NOT_SUPPORTED
```

`init.sh` 안의 아무 일도 하지 않는 셸 함수라서, 그 매크로로 걸러지고 있지 않을 때 이 줄은
무해하다. `WINDOWS_NOT_SUPPORTED` 와 `AIX_NOT_SUPPORTED` 도 있다.

경로로 건너뛰려면 경로 조각을 파일에 적고 `testcase_exclude_from_file` 이 그것을 가리키게 한다 —
[cubrid-testkit-patches](https://github.com/cubrid-systems/cubrid-testkit-patches)(비공개)의
`machine-exclusions/README.md` 를 보라. 이 머신의 목록이 왜 상류의 것과 따로 관리되는지도 거기
있다.

**거기 적힌 파일이 존재하지 않으면 런이 멈춘다.** CTP 는 그 파일에서 아무것도 읽지 않고, 빼놓기로
한 케이스를 전부 돌렸다.

## 함정들

**시간 줄이 문자 그대로의 diff 를 불안정하게 만든다.** `csql` 은 `1 row selected. (0.001000 sec)`
을 찍고 그 수는 매번 다르다. 예제처럼 원하는 것을 grep 하거나, 비교 전에 변덕스러운 줄을
걷어내라.

**`cubrid createdb` 는 `cubrid_createdb` 가 아니다.** 맨 유틸리티는 로케일 인자를 요구한다.
`cubrid_createdb` 는 CTP 의 래퍼이고 conf 의 `cubrid_db_charset` 에서 그것을 채워준다. 래퍼를
쓰라.

**서버를 켜둔 채 끝난 케이스는 슬롯을 붙잡는다.** `finish` 가 멈춘다. 케이스가 일찍 빠져나가야
한다면 그 전에 `finish` 를 불러라.

**없는 도구는 조용히 실패한다.** 216 개 케이스가 `expect` 스크립트로 대화형 `csql` 을 몰고 그
로그를 grep 한다. `expect` 가 없으면 케이스는 그래도 돌고, `.exp` 스크립트는 돌지 않고, grep 은
0 을 센다 — **틀린 답과 구별되지 않는 평범한 NOK.** 런이 케이스마다 그것을 발견하게 두지 않고
시작 시점에 점검하는 이유다.

**`.result` 파일을 커밋하지 마라.** 런은 그것을 저장소가 아니라 오버레이에 쓰지만, 수동으로 돌린
런에서 하나가 커밋돼 흘러들면 다음 독자를 혼란스럽게 한다.

**케이스 디렉터리 바깥에는 의도한 게 아니면 쓰지 마라.** 오버레이는 디렉터리가 끝날 때 시나리오
아래의 쓰기를 버린다. 그 바깥의 쓰기는 슬롯의 것이고 슬롯과 함께 사라진다 — 대개 그것이 원하는
바이지만 알아둘 값이 있다.
