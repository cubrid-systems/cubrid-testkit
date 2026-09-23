# E9 — Design (STUB)

*[English](design.md) · 한국어*

**Status:** STUB — 정식 design 은 ADR-EXT-009 incubating 정식 진입 후.
E5 의 `-DENABLE_FUZZING` 인프라 + state reset 스파이크가 선결.
**Source:** ROADMAP §6a-E9, `requirements.md`, `project/survey/dbms-testing-ecosystem.md` §7.4
**Companion:** `requirements.md` (FULL), `io-contract.md` (STUB), `test-corpus.md` (STUB)

---

## 1. 책임 경계 (cross-repo)

```
cubrid 본 repo                            testkit (이 모듈)
─────────────                            ───────────────
storage fuzz target                  →   corpus 보관 (seed + crash)
in-process boot / shutdown 진입점    →   replay (regression)
state reset 훅                       →   crash triage (stack hash dedup)
sanitizer 빌드 (ASan/UBSan)          →   op sequence → 사람이 읽는 reproducer 덤프
LLVMFuzzerTestOneInput 구현          →   coverage 보고
```

E5 와 **같은 인프라를 공유** 한다. 본 항목이 추가로 요구하는 것은 *reset 훅* 뿐이다.

## 2. 계층 (의제)

```
libFuzzer
   ├─ mutator: libprotobuf-mutator | FuzzedDataProvider   ← ADR-EXT-009
   ▼
StorageOpSequence (in-memory)
   ▼
translate()  ── op → heap_* / btree_* / log_* 직접 호출
   ▼
CUBRID storage engine (in-process, 임시 volume)
   ▲
reset()  ── 매 입력 경계에서 호출
```

protobuf 는 **mutator 층에만** 존재한다. `translate()` 아래로는 protobuf 가 없고,
CUBRID 자체 프로토콜/직렬화는 *전혀 경유하지 않는다* (`requirements.md` §2).

## 3. 모듈 위치 (의제)

```
internal/runner/fuzzharness/          # E5 와 공유
   ├── runner/
   ├── corpus/
   ├── triage/
   ├── coverage/
   └── storage/                       # ← 본 항목 신규
         ├── opdump/                  # op sequence → 텍스트 reproducer
         └── invariant/               # 선택적 검사점 결과 수집
```

## 4. state reset 전략 — 결정 보류

`requirements.md` §5 의 A/B/C/D 비교표. 스파이크 결과로 결정.
**본 문서의 나머지는 이 결정에 종속되므로 지금 쓸 수 없다.**

## 5. 결정 보류 항목 → ADR-EXT-009

- 입력 IR (libprotobuf-mutator vs FuzzedDataProvider vs 자체)
- state reset 전략
- operation 어휘 1차 범위 (heap 단독 / +btree / +vacuum / +checkpoint)
- corpus 위치 (NG1 점검)
- invariant 훅 유무

## 6. design 작성 트리거

E5 인프라 + state reset 스파이크 + ADR-EXT-009 후. 현 시점 stub.
