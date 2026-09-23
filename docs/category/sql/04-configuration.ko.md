# 4. 설정

*[English](04-configuration.md) · 한국어*

[← sql 카테고리로](README.ko.md)

- [파일에는 섹션이 있다](#파일에는-섹션이-있다)
- [CTP 의 키들](#ctp-의-키들)
- [이 러너의 키들](#이-러너의-키들)
- [환경](#환경)
- [권장 설정](#권장-설정)

## 파일에는 섹션이 있다

`sql.conf` 은 서로 다른 두 가지를 담는다: 테스트 프레임워크가 읽는 것, 그리고 런 전에 **엔진
자신의 설정 파일에 써넣는** 것. 그래서 그것은 평평한 파일이 아니라 ini 파일이고, `ha_mode` 는
두 섹션에 나타나 서로 다른 두 가지를 뜻한다.

| 섹션 | 그것이 무엇인가 |
|---|---|
| 최상위 | CTP 자신의 키들, 섹션 없이 |
| `[sql]` | 이 러너의 키들 |
| `[sql/cubrid.conf]` | `do_configure` 가 `$CUBRID/conf/cubrid.conf` 에 써넣는다 |
| `[sql/cubrid_ha.conf]` | `cubrid_ha.conf` 에 써넣는다 |
| `[sql/cubrid_broker.conf/%BROKER1]` | 그 이름의 브로커 섹션에 써넣는다 |

shell 계열의 conf 는 평평하고, 그것이 두 파일 사이의 유일한 실제 차이다. 각 러너는 그 경로를
자기 형식으로 읽는다.

## CTP 의 키들

이들은 늘 그랬던 대로 동작한다.

| 키 | 기본값 | |
|---|---|---|
| `scenario` | — | 코퍼스 루트, `.../sql` 또는 `.../medium`. 필수 |
| `test_category` | task | 결과 트리와 CQT 의 alias 이름을 정한다 |
| `testcase_exclude_from_file` | — | 건너뛸 경로 조각들이 담긴 파일 하나. **없는 파일은 필터도 메시지도 아니다** — 출하되는 모든 conf 가 `${CTP_HOME}/conf/exclusions.txt` 를 가리키는데, CTP 는 그것을 출하하지 않는다 |
| `jdbc_config_file` | `test_default.xml` | `sql/configuration/test_config/` 아래에 있는 CQT 자신의 XML: 연결, 런 모드, 그리고 요약이 답을 담는지 |
| `db_charset` | `en_US.iso88591` | `run.sh` 가 그러듯, 11.5 이상에서 sql task 에는 `en_US.utf8` 로 강제된다 |
| `cubrid_createdb_opts` | — | `createdb` 에 넘겨진다 |
| `need_make_locale` | `yes` | 데이터베이스 전에 로케일 라이브러리를 만든다 |
| `data_file` | — | medium 의 `mdb.tar.gz` |
| `enable_memory_leak` | `no` | `yes` 는 task 전체를 CTP 에 되돌려준다: 그것은 valgrind 런이지 코퍼스 런이 아니다 |

## 이 러너의 키들

전부 `[sql]` 아래에 있다.

| 키 | 기본값 | 영향 | |
|---|---|---|---|
| `parallel_slots` | 크기 측정 | **큼** | 한 번에 몇 케이스가 도는지. 디렉터리를 통째로 가져가므로 이것은 슬롯 수이지 케이스 단위 분산이 아니다. 설정하지 않고 격리돼 있으면: 머신의 첫 런에서 넷(medium 은 하나), 그다음에는 그 머신이 돌려본 최대치의 두 배까지 가는데, 슬롯당 2.6 GB 인 메모리나 프로세서나 가장 긴 디렉터리나 knee 가 멈출 때까지다([슬롯과 속도](05-slots-and-speed.ko.md#슬롯을-몇-개로)). 격리되지 않았으면 하나 |
| `parallel` | `measured` | 위의 것이 자라는 방식 | `conservative` 는 자라지 않고 메모리 예산을 두 배로 잡는다; `aggressive` 는 네 배로 자라고 knee 를 무시한다 |
| `case_patch_dir` | 끔 | 직접적으로는 없음 | 이 런이 들고 가는 코퍼스 변경, 첫 케이스 전에 적용되고 끝에 되돌려진다. [케이스가 실패할 때](06-when-a-case-fails.ko.md)를 보라 |
| `status_http` | 끔 | 없음 | 진행 페이지. `on` 은 `127.0.0.1:51523`; 포트만 적으면 모든 인터페이스를 가져간다 |

여기에는 `testcase_retry_num` 도 `testcase_timeout_in_secs` 도 없다: CQT 에는 둘 다 없고, 그것을
더한 러너는 CTP 의 숫자가 답하는 것과 다른 질문에 답하는 셈이 된다.

## 환경

| | |
|---|---|
| `TESTKIT_NATIVE=sql` | `sql` 과 `medium` 을 CTP 에 넘기지 않고 여기서 돌린다. opt-in 게이트다; 계열을 쉼표로 구분해 나열하고(`shell,sql`), `all` 은 전부다. `TESTKIT_NATIVE_SQL=1` 은 옛 표기이고 여전히 동작한다 |
| `TESTKIT_CONTAIN=1` | 런을 자기만의 네임스페이스에 넣는다. 슬롯에 **필수** |
| `TESTKIT_SLOT_ROOT` | 각 슬롯의 쓰기가 떨어지는 곳. 설정하지 않으면 `/var/tmp/testkit-slots` — 그리고 **그것이 어느 디스크인지가 중요하다**: 슬롯 여덟 개가 어떤 머신의 루트 SSD 에서 1,131 초, 그 데이터 디스크에서 722 초 걸렸다 |
| `TESTKIT_SLOT_VOLATILE=1` | 슬롯 층에서의 sync 가 아무것도 하지 않고 돌아온다. 기본은 꺼짐인데, 그것이 CTP 가 도는 조건의 일부이기 때문이다. 슬롯 여덟 개에서 722 초 → 338 초의 값어치가 있다; 슬롯들이 함께 시작할 수 있게 하는 것도 이것이다 |
| `TESTKIT_CONTAIN_SH` | `/bin/sh` 위에 bind 할 셸. 찾을 수 있으면 `bash` |

그리고 CTP 자신의 것: `CUBRID`, `CUBRID_DATABASES`, `CTP_HOME`, `JAVA_HOME`.

## 권장 설정

`scripts/sizing.sh sql <your conf>` 을 돌려라 — 머신을 재서 자기 숫자와 함께 이것들을 찍는다.
빠른 디스크가 `/data` 인 30 GB·16 코어 머신이라면:

```
[sql]
parallel_slots=6
case_patch_dir=/path/to/cubrid-testkit-patches/sql
status_http=on
```

```bash
TESTKIT_NATIVE=sql TESTKIT_CONTAIN=1 \
TESTKIT_SLOT_ROOT=/data/slots TESTKIT_SLOT_VOLATILE=1 \
testkit sql -c sql.conf
```

그리고 medium 은 `parallel_slots` 를 1 로 둔 같은 환경.

`[sql/cubrid.conf]` 은 코퍼스가 기대하는 대로 두어라. 버퍼는 출하되는 모든 conf 에서 데이터
512 MB 와 로그 256 MB 이고, 슬롯의 서버가 슬롯이 치르는 2.6 GB 의 대부분이다 — 그러나 그것들은
케이스가 기록될 때 견주어진 값이기도 하고, trace 출력은 그것과 함께 움직이는 페이지 수를 담는다.
