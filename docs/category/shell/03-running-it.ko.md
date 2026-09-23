# 3. 돌리기

*[English](03-running-it.md) · 한국어*

[← shell 카테고리로](README.ko.md)

두 방법 모두 **같은 바이너리를 같은 conf 로** 돌린다. 다른 것은 환경이 무엇을 들고 오느냐뿐이다.

- [호스트 머신에서](#호스트-머신에서)
- [Docker 에서](#docker-에서)
- [런 지켜보기](#런-지켜보기)
- [결과 읽기](#결과-읽기)

## 호스트 머신에서

**root 도, 컨테이너 런타임도, 머신 변경도 필요 없다.** 네임스페이스와 오버레이 둘 다 비특권으로
쓴다. 평범한 사용자가 평범한 바이너리를 돌리는 것이다.

### 빈 디렉터리에서 판정까지

```bash
# 1. 빌드
git clone <this repo> && cd cubrid-testkit
go build -o bin/testkit ./cmd/testkit

# 2. CTP 가 늘 필요로 해온 환경
export CUBRID=/path/to/CUBRID
export CUBRID_DATABASES=$CUBRID/databases        # $CUBRID 안이어야 한다
export LD_LIBRARY_PATH=$CUBRID/lib
export PATH=$CUBRID/bin:$PATH
export CTP_HOME=/path/to/CTP
export JAVA_HOME=/path/to/jdk                    # 아직 CTP 로 넘어가는 task 를 위해

# 3. conf 하나
cat > shell.conf <<'EOF'
scenario=/path/to/testcases/shell
test_category=shell
feedback_type=file
testcase_retry_num=0
testcase_timeout_in_secs=720
parallel_slots=4
status_http=on
EOF

# 4. 머신이 가진 것보다 많은 슬롯을 요청하기 전에 크기를 재라
CUBRID=$CUBRID scripts/sizing.sh

# 5. 실행
TESTKIT_CONTAIN=1 TESTKIT_NATIVE=shell bin/testkit shell -c shell.conf
```

`TESTKIT_CONTAIN=1` 이 각 슬롯에 네임스페이스를 준다. 없으면 슬롯은 조용히 충돌하는 대신
**시작을 거부한다.** `TESTKIT_NATIVE=shell` 은 *`shell` 을 여기서 돌려라* 라고 말하는 opt-in
게이트이며, 그것이 없으면 CTP 로 넘어간다.

### 케이스 하나를, 반복해서

스위트가 어떤 케이스는 믿을 수 없다고 말해준 다음에:

```bash
bin/testkit run-shell --loop --maxloop 200 _01_utility/_38_csql/csql1
```

케이스 디렉터리에서 `touch STOP` 하면 진행 중인 시도가 끝난 뒤 루프가 멈춘다.

## Docker 에서

```bash
docker run --rm \
  --security-opt seccomp=unconfined \      # 네임스페이스를 만들기 위해
  --security-opt systempaths=unconfined \  # /proc 를 다시 마운트하기 위해
  -p 51523:51523 \                         # 페이지를 보기 위해
  -v /host/work:/home \
  -e CUBRID=/home/CUBRID \
  -e CUBRID_DATABASES=/home/CUBRID/databases \
  -e CTP_HOME=/home/CTP \
  -e TESTKIT_CONTAIN=1 \
  -e TESTKIT_NATIVE=shell \
  -e TESTKIT_SLOT_ROOT=/home/slots/run \
  <image> testkit shell -c /home/shell.conf
```

`--privileged` 는 **필요 없다.** 두 `--security-opt` 는 필요하고, 각각의 증상은 혼자서는 아무것도
지목하지 않는다:

| 증상 | 필요한 플래그 |
|---|---|
| 네임스페이스를 만들 수 없다 | `seccomp=unconfined` |
| `mount /proc: operation not permitted` | `systempaths=unconfined` |

### 컨테이너에서만 무는 것 셋

**1. 트리가 아니라 그 부모를 마운트하라.** `docker run -v` 는 주어진 경로를 마운트 지점으로
만들고, **오버레이의 하위 층은 마운트 지점일 수 없다** — overlayfs 는 모든 거부에 대해 가진
단 하나의 메시지로 거절한다.

```
   틀림                                     맞음
   -v /host/testcases:/home/testcases       -v /host/work:/home
      → /home/testcases 가 마운트 지점이라       → /home/work/testcases 는 마운트 안의
        하위 층이 될 수 없다                        평범한 디렉터리다
```

`$CUBRID`, `$CUBRID_DATABASES`, 그리고 슬롯 루트의 부모에도 똑같이 적용된다. 오버레이의 상위
층과 work 디렉터리도 컨테이너 자신의 overlayfs 위에 있을 수 없다.

**2. 트리 전체를 한 소유자로 두고, 그 uid 로 돌려라.** bind 마운트는 호스트의 uid 를 그대로
들여오는데, 네임스페이스는 정확히 하나만 매핑한다. uid 1000 이 소유한 트리는 uid 0 만 매핑하는
네임스페이스 안에서 `nobody` 이고, 실패는 가장 먼저 쓰는 무엇에게서 `permission denied` 로
드러난다:

```bash
# 호스트에서
sudo chown -R 1000:1000 /host/work
# 그리고 컨테이너를 그 uid 로
docker run --user 1000 ...
```

어느 uid 를 고르든 **마운트된 트리와 컨테이너가 합의해야 한다.**

**3. 런 사이에 결과 트리를 비워라.** `feedback.log` 와 나머지는
`$CTP_HOME/result/<category>/current_runtime_logs` 아래에 산다. `$CTP_HOME` 이 bind 마운트라면
두 번째 런이 첫 런의 파일에 덧붙이고, 나중에 그것을 읽으면 두 런이 섞인다:

```bash
rm -rf /host/work/CTP/result
```

또는 런마다 자기 `CTP_HOME` 을 주어라.

## 런 지켜보기

```
status_http=on        # 127.0.0.1:51523
status_http=8123      # 모든 인터페이스, 포트 8123
```

페이지는 슬롯들, 각자가 무엇을 얼마나 오래 돌리고 있는지, 최근 판정들, 실패들, 그리고 상한 중
얼마가 쓰이고 있는지를 보여준다.

**slots** 아래의 케이스를 클릭하면 그것이 *지금까지* 쓴 것을 보여준다 — 돌고 있는 케이스는
`feedback.log` 에 아직 아무것도 없다(그 블록은 끝날 때 쓰인다). 하지만 점검마다 자기 결과 파일에
덧붙인다. 계획상 6 초짜리인데 325 초째 슬롯을 붙잡고 있는 케이스가 **어느 점검에서 막혔는지**를
말해주는 곳이 여기다. **finished** 아래의 케이스를 클릭하면 기록된 블록을 보여준다.

## 결과 읽기

결과 파일은 동결돼 있다. 어느 러너가 만들었든 같다.

| | |
|---|---|
| `test_status.data`, `check_local.log` | 판정들 |
| `dispatch_tc_ALL.txt`, `dispatch_tc_FIN_local.txt` | 찾은 케이스들, 끝난 케이스들 |
| `feedback.log` | 케이스마다 블록 하나. 점검들과 diff 를 담아 |
| `test-shell.xml` | JUnit, CI 용 |
| `main.info`, `summary_info` | 집계 |
| `patched.txt` | **이 러너 자신의 것.** 패치가 적용되지 않았으면 아예 없다 |

**동시에 두 런은 거부된다.** 이유는 하나다: 둘이 `<CTP_HOME>/result/<category>` 를 공유하게
되는데, 거기에는 둘 사이에 `feedback.log` 하나와 `test_status.data` 하나가 있어서 **결과가 어느
쪽도 서술하지 못한다.** 런이 만드는 나머지는 이미 각자의 것이다. 두 번째 런에 다른 `CTP_HOME` 을
주면 둘은 공존한다.
