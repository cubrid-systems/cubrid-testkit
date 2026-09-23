# E4 — Design (STUB)

*[English](design.md) · 한국어*

**Status:** STUB — 정식 design 은 ADR-EXT-004 incubating 정식 진입 후. 또한 N24 / N11 graduation 선결.
**Source:** ROADMAP §6a-E4, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §6
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. 모듈 위치 (의제)

```
internal/runner/isolationdist/
   ├── topology/         # HA / streaming-replication 자동 deploy (shell.DeployHA 재사용)
   ├── workload/         # randomized tx generator (KV-style 또는 SQL-style)
   ├── fault/            # network partition (iptables) / process kill / clock skew
   ├── history/          # per-client tx start/commit/abort 로그
   └── analyzer/
         ├── awdit/      # weak isolation anomaly + history graph cycle
         └── jepsen/     # external spec checker (linearizability/causal/SI) — Clojure 외부 호출
```

*단일노드 축 4* (PostgreSQL `.spec` + Hermitage) 는 본 모듈 외 — 기존 `project/analysis/isolation` 의 strangler-fig 1차 대체 (ADR-004) 검토 시 결정.

## 2. 데이터 흐름 (의제)

```
seed → topology.deploy(HA|streaming, N nodes)
       └─ workload.start(N clients) ┐
                                    ├─ history.collect()
       └─ fault.inject(seed)        ┘
                                       └─ analyzer.run(history)
                                            └─ violation? corpus.save({seed, fault seq, witness})
```

## 3. fault injection 채널 (의제)

| 채널 | 검출 가능 | 비고 |
|---|---|---|
| iptables network partition | replication split-brain | 권한 sudo |
| process kill (SIGKILL) | recovery invariant | shell 모듈 재사용 |
| clock skew (faketime) | causal / linearizability | 라이브러리 의존 |
| cgroup CPU/IO throttle | timing race | 운영 dependency |

E7 (workload) 와 *공유 가능* — 어느 항목이 owner 인가 ADR 필요.

## 4. 외부 의존

- N24 streaming-replication / N11 logical-replication-extension graduation (선결)
- shell 모듈 DeployHA + ha_repl 자산
- AWDIT 본체 (정확한 repo 확인 보강 대상)
- Jepsen (jepsen.io / Clojure)
- engine-suite C-004 책임 경계 (E7 와 결합)

## 5. 결정 보류 항목 → ADR-EXT-004

- AWDIT vs Jepsen 1차 선택
- fault injection 채널 단일화
- E7 와의 fault injector 공유 owner
- history corpus 위치 (NG1 점검)

## 6. design 작성 트리거

ADR-EXT-004 합의 후 + N24/N11 graduation 후. 현 시점 stub.
