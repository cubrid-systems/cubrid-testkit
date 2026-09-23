# `isolation` 카테고리

*[English](README.md) · 한국어*

`isolation` 은 CTP 의 동시성 코퍼스다 — 6,790 케이스이고, 각각은 락 대기·교착·가시성 규칙이
스크립트가 정한 순서대로 일어나도록 둘 이상의 클라이언트를 한 데이터베이스에 몰아붙이는
스크립트다 — 그리고 testkit 이 세 번째로 다시 쓴 계열이다. **케이스 자체는 여전히 CTP 자신의
`runone.sh` 와 ctltool 이 실행한다**. 케이스 주위의 모든 것이 러너의 몫이다. 이 문서들이 그
as-built 가이드다.

| | |
|---|---|
| **[1. 런은 어떻게 도는가](01-how-a-run-works.ko.md)** | 단계들, `runone.sh` 가 케이스에 하는 일, 판정, 그리고 여기서 슬롯이 무엇인지 |
| **[2. 케이스 쓰기](02-writing-a-case.ko.md)** | `.ctl` 언어, 답이 어디 있는지, 케이스가 기대면 안 되는 것 |
| **[3. 돌리기](03-running-it.ko.md)** | 설치본과 코퍼스에서 판정까지, 그리고 런이 남기는 것 |
| **[4. 설정](04-configuration.ko.md)** | 러너가 읽는 모든 키와 스위치 |
| **[5. 케이스가 실패할 때](05-when-a-case-fails.ko.md)** | 판정을 읽는 법, CTP 도 재현하지 못하는 케이스들, 그리고 컨트롤러가 느릴 때만 통과하는 케이스들 |

설계와 결정, 그리고 그 과정에서 측정된 것은
[`../../project/design/module-isolation.md`](../../project/design/module-isolation.md),
[ADR-007](../../project/adr/ADR-007-isolation-executor.md) (실행기),
[ADR-018](../../project/adr/ADR-018-isolation-equivalence.md) (게이트),
[`../../project/evidence/isolation-baseline.md`](../../project/evidence/isolation-baseline.md) 이다.

## 한 문단으로

런은 머신을 점검하고, `scenario` 아래의 `.ctl` 파일을 나열하고, 슬롯을 하나 또는 여럿 연다.
슬롯은 `$CUBRID` 와 ctltool 디렉터리, 그리고 — `scenario_disk` 가 있으면 — 케이스 트리까지
각자의 오버레이 뒤에 둔 네임스페이스 묶음이고, 자기 `ctldb` 를 스스로 만든다. 이것은 다른 어떤
계열보다 여기서 더 중요하다: `runone.sh` 는 **그 사용자가 가진 모든 `cub`·`sleep`·`qactl`·
`qacsql` 을 죽이고**, 슬롯 안에서 그 사용자가 가진 것은 그 슬롯뿐이기 때문이다. 슬롯은 케이스를
하나씩 `runone.sh` 에 넘기고, 판정은 그 스크립트가 찍은 것에서 읽는다.

## 가장 짧은 런

`$CUBRID` 가 설치본을, `$CTP_HOME` 이 CTP 트리를 가리키고 `cubrid-testcases` 가 체크아웃된
상태에서:

```bash
cat > /tmp/demo/isolation.conf <<'EOF'
scenario=/path/to/cubrid-testcases/isolation
testcase_timeout_in_secs=300
testcase_retry_num=4
testcase_exclude_from_file=/path/to/cubrid-testcases/isolation/config/daily_regression_test_excluded_list_linux.conf
EOF

TESTKIT_NATIVE=isolation TESTKIT_CONTAIN=1 testkit isolation -c /tmp/demo/isolation.conf
```

```
Available Env: [local]
Build Id: 11.5.0.2574-f1ae86f
Build Bits: 64bits
BEGIN TO CHECK: 
…
The Number of Test Case : 6772
============= DEPLOY ==================
DONE
============= TEST ==================
STARTED
[ENV START] local
[ENV START] local
[ENV START] local
[ENV START] local
[TESTCASE] /path/to/cubrid-testcases/isolation/_01_ReadCommitted/…/x.ctl EnvId=local [OK]
…
============= PRINT SUMMARY ==================
Test Category:isolation
Total Case:6790
Total Execution Case:6772
Total Success Case:6758
Total Fail Case:14
Total Skip Case:18

TEST COMPLETE
```

CTP 의 출력을 줄 단위로 그대로 옮긴 것이다 — 슬롯마다 하나씩 붙는 `[ENV START]` 와
`[ENV STOP]` 만 빼고. 이 런은 기본값인 네 슬롯을 썼고 약 50 분이 걸렸다. 한 슬롯이면 3 시간
반이다. 머신이 슬롯을 몇 개 받는지는 [돌리기](03-running-it.ko.md#slots)가, 열넷 중 어느 것이
시험 대상 빌드에 관한 것이 아닌지는 [케이스가 실패할 때](05-when-a-case-fails.ko.md)가 말한다.
