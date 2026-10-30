# ADR-EXT-012: 병렬 HA applier 의 순서 검증은 testkit 의 새 진입점이다 (E12)

- **Status:** **Proposed** (2026-10-30) — 사용자 검토
- **구현 상태:** M1(`pairs`, `list`)이 구현됐고 §4 의 완료 조건을 충족했다 (2026-10-30,
  `docs/project/evidence/ha/ha-apply-gives-the-prototypes-verdicts.md`). 새 head 의 한 실행에서는
  21 개가 held 이고 `fk_ci`·`pk_move` 가 inconclusive 였다. 두 패턴은 50 라운드에서 held 였고,
  missed 는 없었다. M2(`stress`)도 구현됐고 §4 의 완료 조건을 충족했다 (2026-10-30,
  `docs/project/evidence/ha/ha-apply-stress-and-the-heap-only-row.md`). 셸 프로토타입은
  `docs/project/evidence/ha/parallel-apply-*.sh` 에 남는다
- **Date:** 2026-10-30
- **Trigger:** CBRD-27459 (writeset 과 applylogdb 병렬화 설계), 구현 CBRD-27508 / CUBRID/cubrid #8052 —
  슬레이브가 트랜잭션을 여러 worker 로 적용하면서 **충돌하는 것끼리의 순서만** 지킨다. 그 순서가
  지켜지는지를 testkit 이 물을 수단이 없다
- **Depends on:** ADR-003 (외부 표면 동결 — 새 하위 명령은 NF), ADR-015 (오라클을 더하는 항목은
  §6a, 자체 ADR-EXT), ADR-022 (토폴로지는 cluster-sandbox 가 세운다), ADR-EXT-011 (격리 앞에서
  라우팅되는 하위 명령의 선례)
- **근거 측정:** `docs/project/evidence/ha/` 의 `parallel-apply-and-the-marker.md`,
  `parallel-apply-keeps-conflict-order.md`, `parallel-apply-under-pressure.md`,
  `fk-index-insert-loses-the-victims-error.md`

---

## 1. Context

병렬 applier 아래에서 슬레이브는 마스터의 커밋 순서를 따르지 않는다. 마스터가 writeset 으로
"이 트랜잭션은 저것 뒤에 와야 한다" 를 계산하고, 슬레이브의 게이트는 그 의존성만 지킨다. 그래서
writeset 이 의존성 하나를 놓치면, 슬레이브는 뒤의 트랜잭션을 먼저 적용하고 실패하거나 조용히
갈라진다. 이 결함은 기존 HA 검사(`ha_repl`)에 보이지 않는다. 그 케이스들은 한 세션이 순서대로
쓰고 끝에서 두 노드를 비교하므로, 순서가 바뀔 기회가 거의 없다.

셸 프로토타입은 이 질문을 물을 수 있음을 보였다. 앞 트랜잭션(T1)에 무거운 ballast 를 실은 뒤
공유 키를 쓰고, 뒤 트랜잭션(T2)은 그 키에 의존하는 일과 witness 행을 쓴다. 슬레이브에서 witness 가
보이는데 T1 의 ballast 가 0 이면 T2 가 먼저 적용된 것이다. 공유 키가 없는 대조군은 같은 조건에서
20 라운드 중 19~20 번 먼저 적용되었으므로, 놓친 의존성은 라운드마다 그만큼의 확률로 드러난다.
#8052 의 옛 head 에서 이 방법은 인덱스는 같다고 보는데 writeset 은 다르게 본 키(-0.0 과 +0.0, 두
시간대의 한 시각, 상속 계층의 하위 클래스와 상위 클래스) 여섯 패턴의 누락을 모두 잡았고, 그것을
고친 새 head 에서는 24 개 패턴이 모두 순서를 지켰다.

## 2. 결정 전에 확인한 사실

| # | 확인 사항 | 결과 |
|---|---|---|
| F1 | task 이름을 더할 수 있는가 | **없다.** `external-surface-freeze` 가 14 개 task 를 F1 로 동결했다. 새 하위 명령은 NF 다 (ADR-EXT-011 F1) |
| F2 | 격리 앞에서 라우팅되는 하위 명령의 선례가 있는가 | **있다.** `perf`, `sizing`, `check-cases` (`cmd/testkit/main.go`) |
| F3 | 이것이 §6a 항목인가 | **그렇다.** 새 오라클(적용 순서)을 더한다. ADR-015: "An entry that adds an oracle belongs in §6a with its own ADR-EXT number" |
| F4 | cluster-sandbox 가 필요한 것을 갖췄는가 | **갖췄다.** `cluster create` 의 `--set-hidden` (`ha_apply_worker_count` 는 `cubrid_ha.conf` 로 간다), `--clients`, `--tools` |
| F5 | `internal/sandbox` 가 그것을 넘기는가 | **아니다.** `CreateOptions` 에 hidden 파라미터와 tools 가 없고, `Pair` 에 클라이언트 노드가 없으며, `SHOW WRITESET STATUS` 를 읽는 곳이 없다. `fail_counter` 는 `HAStatus` 가 이미 읽는다 |
| F6 | 기존 대기가 병렬 applier 아래에서도 참인가 | **참이다**, 마커가 DDL 이기 때문에. DML 마커는 20 라운드 중 18 번 앞질렀다. `replication.go` 주석과 live 테스트가 이를 고정한다 |
| F7 | 셸 프로토타입의 비교가 믿을 만한가 | **한 곳이 아니었다.** PK 로 읽는 비교는 인덱스 항목 없이 heap 에만 있는 행을 놓쳤다. heap 을 세는 검사를 더했다. 생성기의 seed 는 SQL 을 재현하지 못했다(mawk `srand`). 자체 수열로 바꿨다 |

## 3. Decision

### 3-1. 진입점 → `testkit ha-apply <verb>`, NF, 격리 없음

`perf` 와 같은 자리, 격리 **앞**에서 라우팅된다. 클러스터는 csb 가 세우고 이 명령은 코퍼스를
돌리지 않는다. CLI·conf·출력 파일은 NF 등급이다. 동결된 task 의 표면은 바뀌지 않는다.

### 3-2. 동사

- `pairs` — 충돌 쌍의 적용 순서. 패턴마다 R 라운드, 대조군 `ctl` 을 같은 실행에 포함한다.
- `stress` — N 개 클라이언트의 결정적 작업 부하, 끝에서 두 노드를 비교한다(PK 로 읽은 행, heap 행
  수, 고아 행, `fail_counter`). 같은 seed 는 셸 프로토타입과 같은 SQL 을 만든다(같은 Park–Miller
  수열). 끼어드는 순서는 실행마다 다르고 seed 가 재현하지 않는다. `--compare-only` 는 작업 부하 없이
  쌍에 남은 표를 비교만 한다. 손상을 남긴 쌍, 멈춘 실행, checkdb 뒤의 표를 지우지 않고 다시 볼 때
  쓴다.
- `list` — 패턴 목록과 각 패턴이 공유하는 키.

마커 대기 검사는 동사로 두지 않는다. live 테스트가 이미 그것을 고정한다(F6).

### 3-3. 판정

- 패턴의 판정은 **missed**(T2 가 한 번이라도 먼저 보임, 또는 `fail_counter` 가 움직임, 또는 끝의
  비교가 다름, 또는 슬레이브가 멈춤), **held**, **inconclusive** 셋이다.
- 슬레이브가 멈췄다는 판정은 읽기로만 내린다. 슬레이브에서 읽은 한 라운드의 ballast 행 수가 라운드의
  행 수에 못 미친 채 ballast 2 만 행마다 2 분(최소 2 분) 동안 그대로이면 멈춘 것이다. T1 은 한
  트랜잭션이라 행 수는 보통 0 에서 한 번에 라운드의 행 수가 되므로, 이 시간은 슬레이브가 T1 을
  적용하는 데 쓸 수 있는 시간이다. 기본값에서 새 head 의 한 라운드는 실행마다 평균 2.3~5.5 초였다. 겨냥하는 것은 영원히
  기다리는 슬레이브다(`fk-index-insert-loses-the-victims-error.md`). 클라이언트가 슬레이브를 1 분 동안
  한 번도 읽지 못하면 엔진의 결과가 아니라 실행의 실패다.
- witness 가 기다리는 시간 안에 오지 않은 라운드는, 다른 증거가 없으면 missed 가 아니라
  inconclusive 다. 의존성이 지켜질 때 T2 는 T1 을 기다리므로, 느린 슬레이브도 같은 모습이 된다.
  마스터가 쓰기를 거부한 라운드가 있는 패턴도 inconclusive 다.
- **같은 실행에서 대조군이 한 번도 앞서지 않았으면 모든 held 는 inconclusive 가 된다.** 그 쌍은
  순서를 바꾸지 않는 쌍이고(worker 하나, 또는 ballast 가 너무 작음), held 는 아무것도 증명하지
  않는다. live 테스트의 skip 과 같은 규칙이다.
- `stress` 의 판정은 실행마다 하나다. **damaged**(한 노드에서 heap 으로 센 행 수가 PK 로 읽은 행
  수와 다름: 인덱스 항목 없이 heap 에만 있는 행이 그 노드에 있다. 슬레이브가 stalled 이면 마스터만
  본다. 적용 중인 슬레이브는 두 번의 읽기가 서로 다른 상태를 볼 수 있다), **stalled**(슬레이브가 끝의
  마커를 5 분 안에 보이지 못함. 비교는 하지만 뒤처진 슬레이브는 그것만으로 다르다. 마스터가 마커를
  받지 않은 것은 실행의 실패다), **diverged**(PK
  로 읽은 표, heap 행 수, ballast, 고아 행 수가 두 노드에서 다름, 또는 `fail_counter` 가 움직임),
  **hung**(클라이언트가 `--timeout` 안에 끝나지 않아 끊었음), **same**. 여럿에 해당하면 이 순서에서
  앞의 것이고, 원장의 칸은 모두 남는다. 클라이언트를 끊은 실행도 끝에서 비교한다. 슬레이브를 읽지
  못해 비교하지 못하면 실행의 실패다. 마스터가 거부한 문장은 판정에 들지 않는다. 마스터가 최종적으로
  가진 것이 답이고, 묻는 것은 슬레이브도 그것을 가졌는가다. 다만 트랜잭션을 하나도 끝내지 못한
  클라이언트가 있으면(접속 실패 등) 실행의 실패다. 쓰이지 않은 쌍도 같게 읽히기 때문이다.
- `--compare-only` 의 비교는 실행 끝의 비교와 같되 `fail_counter` 는 보지 않는다(전과 후가 없다).
  슬레이브를 기다리는 마커 표 하나를 마스터에 만들고 지운다.
- 종료 코드는 실행이 끝났는지만 말한다(`ha_repl` 과 같다). 판정은 원장에 있다. 슬레이브가 멈추면
  그 패턴의 행을 쓴 뒤 실행을 끝낸다(뒤의 패턴이 남았으면 종료 코드 1). 뒤의 패턴은 모두 그 슬레이브를
  기다리는 것으로 시작한다.

### 3-4. 패턴은 닫힌 목록이고 Go 안에 있다

패턴은 이름, 준비 DDL, 라운드마다의 `pre`·`t1`·`t2`, 비교할 표로 이루어진 Go 데이터다. 모르는
이름은 이름을 말하며 거부된다(ADR-EXT-011 3-3 과 같은 규칙). 첫 목록은 프로토타입의 24 개다:
PK·UNIQUE·REVERSE UNIQUE·FK(단일·복합·CASCADE)·savepoint·DDL·파티션, 그리고 동치 키 8 개
(-0.0, TIMESTAMPTZ·DATETIMETZ 시간대, `utf8_en_ci`, 상속, 그 FK 짝). 케이스 파일 형식을 새로
만들지 않는다 — HA 에는 새 케이스 언어가 필요 없다는 `module-ha.md` 의 결론을 따른다.

### 3-5. 클러스터

- 기본은 명령이 쌍을 직접 세운다: `--preset ha --clients 1 --set-hidden ha_apply_worker_count=<N>`,
  필요하면 `ha_writeset_history_size`. 끝나면 지운다(`--keep` 이면 남긴다).
- `--cluster <name>` 이면 있는 쌍을 쓴다. 그 쌍의 worker 수는 원장에 적는다.
- 트랜잭션은 클라이언트 노드에서 마스터와 슬레이브로 보낸다. 노드 안에서 보내면 그 노드의 부하가
  측정에 섞인다. 한 라운드의 T1 커밋, T2 커밋, 슬레이브 조회는 클라이언트에서 한 스크립트로
  실행한다. 그 사이에 csb 왕복이 끼면 슬레이브가 T1 을 먼저 끝낼 시간이 생겨 앞지름이 가려진다.
- `internal/sandbox` 는 `CreateOptions` 에 hidden 파라미터(`SetHidden`)만 얻는다. 클라이언트 노드는
  `perf` 처럼 `sandbox.NewNode(cli, "c1")` 로 얻고, writeset 카운터를 읽는 일은 클라이언트에서
  마스터로 보내는 조회이므로 이 명령의 패키지에 둔다. **HA `Create` 와 `harepl` 경로는 바뀌지
  않는다.** M2 에서 `WaitForReplication` 의 오류가 "슬레이브가 마커를 보이지 못함"일 때
  `errors.Is(err, sandbox.ErrNotReplicated)` 가 참이 된다. 오류 문구는 그대로다. `stress` 가 멈춘
  슬레이브와 마커를 받지 않은 마스터를 가르는 데 쓴다.

### 3-6. 결과

- 원장은 TSV 한 파일이다: 패턴, 라운드(슬레이브가 멈춘 패턴은 실제로 돈 라운드), 앞선 횟수, 보이지
  않은 횟수, 마스터가 거부한 횟수, 끝의 비교(`same`, `differ:<표>`, `stalled`), `fail_counter` 변화,
  writeset 카운터(write·ref 충돌, fallback, history clear), 판정.
- 첫 줄에 클러스터, 빌드 식별자, 슬레이브의 `cubrid_ha.conf` 에서 다시 읽은
  `ha_apply_worker_count` 와 `ha_writeset_history_size`, 라운드 수, ballast 행 수를 적는다.
- 행은 패턴이 끝날 때마다 쓴다. 대조군이 먼저 돌므로 이미 쓴 판정은 바뀌지 않는다.
- 표준 출력에는 패턴마다 한 줄과 요약 한 줄을 쓴다.
- `stress` 의 원장은 실행 하나에 한 행이다: seed, 마스터 오류 수, writeset 카운터, `fail_counter`
  변화, 실행 중 마스터에서 고아 행을 본 횟수와 최대값, 클라이언트를 끊었는지, 표마다 PK 로 읽은
  비교와 행 수(heap 으로 센 마스터/슬레이브, PK 로 읽은 마스터/슬레이브), ballast, 끝의 고아
  행(마스터/슬레이브), 슬레이브가 따라왔는지, 판정. 첫 줄은 `pairs` 의 것에 `mode`, 그리고 실행이면
  클라이언트 수, 트랜잭션 수, 키 수, 연산, ballast, seed, timeout 을 더한다. `--compare-only` 에서는
  실행에만 있는 칸이 `-` 다.
- seed 는 SQL 을 재현하지만 끼어든 순서는 재현하지 않으므로 실행의 증거는 원장 옆에 남긴다: 원장
  이름에서 `.tsv` 를 뺀 것에 `.d` 를 붙인 디렉터리에 클라이언트의 스크립트와 로그, 그리고 두 노드에서
  다르게 읽힌 표의 양쪽 읽기. 쌍을 직접 세운 실행은 쌍과 함께 작업 디렉터리를 지우기 때문이다. 그
  디렉터리에 이미 파일이 있으면 쌍을 건드리기 전에 거부한다. 다른 실행의 것을 이번 실행의 것으로
  읽게 되고, 지울 권한도 이 실행에 없다.

### 3-7. Go 만

셸 프로토타입은 근거 문서 옆에 남는다. 이 명령은 그것을 부르지 않는다.

## 4. Consequences

- 동결된 표면은 건드리지 않는다.
- 카탈로그에 E12 가 생긴다. 축은 오라클(적용 순서)이다.
- 이정표: **M1** — sandbox 확장과 `pairs`, `list`. 완료 조건은 #8052 의 옛 head `bf560620e` 에서
  동치 키 여섯 패턴이 missed, 새 head `61ee0944a` 에서 24 개가 held(대조군 앞섬 포함)로, 셸
  프로토타입과 같은 판정이 나오는 것이다. **M2** — `stress`. 완료 조건은 셋이다. 같은 seed 가 셸
  프로토타입과 바이트 단위로 같은 SQL 을 만든다(golden 테스트). #8198 `6ce1c05d2` 의 쌍 ws98b 에 남은
  손상 상태(마스터 `s_child` 가 heap 9 행, PK 7 행)에서 `--compare-only` 가 damaged 를 낸다. #8052 새
  head 의 쌍에서 FK 를 뺀 연산(0–4, 9)의 실행이 same 이다. FK 연산은 #8026 이 없는 트리에서
  CBRD-27464 의 불일치를 낼 수 있으므로 이 조건에서 뺀다.

## 5. 기각한 대안

- **`ha_repl` task 의 두 번째 suite.** 쌍 관리·원장·상태 페이지를 거저 얻지만, `ha_repl` 의 표준
  출력은 F1 이고 HA `Create` 에 hidden 파라미터를 들여야 한다. ADR-EXT-011 §4 가 지킨
  "`harepl` 경로는 바뀌지 않는다" 와 부딪친다.
- **게이트된 live 테스트만.** ADR 도 표면 변경도 필요 없고 마커 검사는 이미 그렇게 들어갔다.
  그러나 원장이 없고, 쌍을 손으로 만들어야 하며, 8 클라이언트 부하는 `go test` 안에서 어색하다.
  스위트가 아니다.
- **셸 프로토타입을 testkit 이 부른다.** 두 벌의 비교 로직이 되고, 프로토타입의 두 결함(F7)이
  보여 주듯 셸의 비교는 검증하기 어렵다.
