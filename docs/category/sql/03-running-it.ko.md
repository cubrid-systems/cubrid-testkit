# 3. 돌리기

*[English](03-running-it.md) · 한국어*

[← sql 카테고리로](README.ko.md)

- [무엇이 필요한가](#무엇이-필요한가)
- [런 하나](#런-하나)
- [지켜보기](#지켜보기)
- [medium 돌리기](#medium-돌리기)
- [무엇을 남기는가](#무엇을-남기는가)

## 무엇이 필요한가

| | |
|---|---|
| 설치본 | `$CUBRID` 가, 이 런이 멈추고 시작하고 재설정해도 되는 CUBRID 설치본을 가리킨다. 공유된 것이면 **안 된다**: `do_clean` 은 이 사용자의 `cub` 프로세스를 죽이고 데이터베이스를 지운다 |
| CTP | `$CTP_HOME` 이 CTP 트리를 가리킨다. 실행기는 그 안의 CQT jar 에 맞춰 컴파일되므로, 코퍼스가 기대하는 바로 그 CTP 여야 한다 |
| 코퍼스 | `cubrid-testcases`, `sql` 또는 `medium` 디렉터리 |
| JDK 8 | `$JAVA_HOME`. CQT 는 JDK 8 코드이고 실행기는 실행 시점에 `javac` 로 컴파일된다 |
| 설치본 안의 레지스트리 | CUBRID 자신의 기본값이 그렇듯, `$CUBRID` 아래의 `$CUBRID_DATABASES`. 그 바깥의 레지스트리는 슬롯의 오버레이가 덮지 않는다 |

두 계열 모두 `TESTKIT_NATIVE=sql` 아래에서 돈다 — 이 프로그램이 task 를 CTP 에 넘기지 않고
직접 돌린다고 말하는 게이트다 — 그리고 `TESTKIT_CONTAIN=1` 을 원한다. 이것은 런을 자기만의
네임스페이스에 넣어 머신의 다른 CUBRID 와 마주칠 수 없게 한다. 슬롯에는 그것이 필요하다.

## 런 하나

```bash
export CUBRID=/path/to/CUBRID
export CUBRID_DATABASES=$CUBRID/databases
export CTP_HOME=/path/to/CTP
export JAVA_HOME=/usr/lib/jvm/java-8-openjdk-amd64
export PATH=$CTP_HOME/bin:$CUBRID/bin:$JAVA_HOME/bin:$PATH

TESTKIT_NATIVE=sql TESTKIT_CONTAIN=1 testkit sql -c sql.conf
```

conf 는 CTP 자신의 것이고, 이 러너가 읽는 섹션 하나가 더 있다. 최소한의 것:

```
scenario=/path/to/cubrid-testcases/sql
test_category=sql
jdbc_config_file=test_default.xml
db_charset=en_US
need_make_locale=yes

[sql]
parallel_slots=6
status_http=on
case_patch_dir=/path/to/cubrid-testkit-patches/sql

[sql/cubrid.conf]
data_buffer_size=512M
log_buffer_size=256M
```

`scripts/sizing.sh sql sql.conf` 은 이 머신을 잰다 — 코어, 그것들이 한꺼번에 내놓는 양, 메모리,
그리고 동기 쓰기에 디스크가 하는 일 — 그리고 자기라면 쓸 설정을 찍는다.
[슬롯과 속도](05-slots-and-speed.ko.md)가 그 뒤의 근거이고, [설정](04-configuration.ko.md)이
모든 키다.

런은 CTP 의 것이 그러듯 케이스가 실패했든 아니든 0 으로 끝난다. 그것으로는 돌 수 없는 설정이나
없는 설치본은 1 로 끝난다.

## 지켜보기

`status_http=on` 은 `127.0.0.1:51523` 에 페이지를 낸다 — 두 번째 런은 다음 빈 포트로 옮겨 가고
어디로 갔는지 말한다. 각 슬롯이 무엇을 돌리고 있는지, 속도, 실패가 떨어지는 대로, 그리고 런이
*무엇을 뜻하는지* 를 바꾸는 스위치들을 담은 setup 패널을 보여준다: 격리돼 있는지, 슬롯들이
어디에 쓰는지, 그 sync 가 진짜인지, 그리고 패치 디렉터리가 쓰이고 있는지.

케이스를 클릭하면 그 판정을, 실패라면 result 가 answer 에서 벗어나는 첫 줄을 그 주변 줄들과
함께 보여준다. 아직 돌고 있는 케이스는 그 문장들을 보여준다 — sql 케이스는 끝날 때까지 아무것도
쓰지 않는데, 실행기가 케이스 전체를 한꺼번에 렌더링하기 때문이다.

## medium 돌리기

같은 러너, 다른 task 와 conf:

```bash
TESTKIT_NATIVE=sql TESTKIT_CONTAIN=1 testkit medium -c medium_dev.conf
```

`medium.conf` 가 아니라 `medium_dev.conf` 를 쓰라: 거기에는 `create_table_reuseoid=no` 가 있고,
그것이 없으면 11.x 에서 데이터 적재가 깨져 975 케이스 중 579 개가 그 여파로 실패한다.

**medium 은 직렬로 돌려라.** 그 케이스들은 전부 합쳐 12.6 초이고 한 디렉터리가 그 절반인데,
슬롯 하나를 띄우는 데 약 26 초가 든다 — 측정값으로 직렬 77 초, 슬롯 넷이면 77–91 초.

## 무엇을 남기는가

- `$CTP_HOME/sql/result/y<year>/m<month>/` 아래의 결과 트리, 런의 이름을 딴 것
- 코퍼스의 케이스마다 그 옆의 `.result`, 다음 런이 덮어쓴다
- `$CTP_HOME/sql/log/<category>_<version>_<epoch>.log`, 단계들 자신의 출력
- 패치 디렉터리가 쓰였을 때, 결과 트리 안의 `patched.txt`
- 그 밖에는 없다: 슬롯들과 그것들이 쓴 모든 것은 런이 끝나면 간다

`$CUBRID` 나 `$CTP_HOME` 아래에서 찾은 core 파일은 `CORE_FILE:<path>` 로 알려지고 — 동결된
코퍼스의 케이스들이 그 줄을 grep 한다 — 슬롯 안에서 찾은 것은 슬롯이 닫히기 전에 결과 트리로
복사된다.
