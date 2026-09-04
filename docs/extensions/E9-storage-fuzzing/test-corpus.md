# E9 — Test Corpus (STUB)

**Status:** STUB — 코퍼스 정책은 ADR-EXT-009 incubating 정식 진입 후.
**Source:** `requirements.md` §8

---

## 1. 코퍼스 종류

| 코퍼스 | 입력/출력 | 용도 |
|---|---|---|
| seed corpus | 입력 | 의미 있는 operation 열 (coverage 부트스트랩) |
| crash corpus | 출력 | crash 를 낸 op 열 — stack hash dedup |
| invariant corpus | 출력 | crash 없이 invariant 를 깬 op 열 |
| coverage corpus | 출력 | libFuzzer 가 자동 관리 |

byte corpus 인 E5 와 달리 **입력이 구조체** 라는 점이 다르다. libprotobuf-mutator 를
채택하면 corpus 항목을 protobuf TextFormat 으로 저장할 수 있어 *사람이 읽고 손으로
편집할 수 있는* seed 가 된다 — 본 항목이 LPM 을 선호하는 실질적 이유 중 하나.

## 2. seed corpus 출처 (의제)

| 후보 | 비용 | 비고 |
|---|---|---|
| 손으로 쓴 경계 시나리오 | 낮음 | overflow 승격 경계 / slot 재사용 / unique 위반 후 재삽입 |
| 기존 shell·sql 케이스의 연산 흔적 추출 | 중 | SQL → 내부 op 매핑이 필요. 자동화 난이도 있음 |
| 과거 CBRD storage 결함 티켓의 재현 열 | 중 | regression seed 로 가치 최대 |
| 무작위 부트스트랩 (seed 없이 시작) | 0 | coverage 상승이 느림. 대조군으로만 |

ADR-EXT-009 에서 seed 정책 결정.

## 3. 보관 정책 (NG1 점검)

- ❌ testcases 레포에 crash / seed corpus 를 두지 않음
- ✅ testkit 내부 별 트리 또는 외부 storage — **E5 와 같은 위치를 공유**
- ✅ seed 는 텍스트(TextFormat)라 diff·리뷰 가능 → 버전 관리 부담이 byte corpus 보다 낮음

## 4. crash 항목 구조 (의제)

```
crash/<stack-hash>/
   ├── input.bin              # libFuzzer 원본 (재현 정본)
   ├── sequence.txt           # 사람이 읽는 op 열
   ├── stack.txt
   ├── sanitizer.txt
   ├── reset_strategy         # 어느 reset 전략에서 났는지 (재현성 판정에 필수)
   └── reproducer.sh
```

`reset_strategy` 를 기록하는 이유: 재현이 reset 전략에 종속되므로, 전략이 바뀌면
과거 crash 의 재현 가능성도 바뀐다 (`requirements.md` §5).

## 5. 라이선스

- libFuzzer: Apache 2.0
- protobuf: BSD-3-Clause
- libprotobuf-mutator: Apache 2.0

모두 fuzz 빌드에만 링크되므로 배포 산출물의 라이선스 구성은 불변.

## 6. 후속 작성 트리거

E5 인프라 + state reset 스파이크 + ADR-EXT-009 후 본 문서 FULL 로 보강.
