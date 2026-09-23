# E3 — Test Corpus (STUB)

*[English](test-corpus.md) · 한국어*

**Status:** STUB — 코퍼스 정책은 ADR-EXT-003 incubating 정식 진입 후.
**Source:** `requirements.md` §5, ROADMAP §8 risk (E2/E3 corpus NG1)

---

## 1. 코퍼스 종류

E2 와 동일 — *입력 코퍼스 없음* (random generation). 코퍼스는 *mismatch 누적 보관소*.

| 코퍼스 | 입력/출력 | 용도 |
|---|---|---|
| seed corpus | 입력 | regression seed — 과거 mismatch query pair 를 새 빌드에서 replay |
| mismatch corpus | 출력 | oracle violation 누적 |

## 2. 보관 정책 (NG1 점검)

- ❌ testcases 레포에 두지 않음
- ✅ testkit 내부 별 트리 또는 외부 storage
- ✅ E2 와 corpus 위치 정책 *공유* (Open Question 3 와 결합)

## 3. mismatch 항목 구조 (의제)

```
mismatch/<oracle>/<witness-hash>/
   ├── seed
   ├── q1.sql
   ├── q2.sql              # NoREC: rewrite / TLP: 3-way partition
   ├── schema.sql
   ├── result_q1.tsv
   ├── result_q2.tsv
   └── reproducer.sh
```

## 4. dedup 정책 (의제)

- witness-hash = (oracle, AST shape after literal normalize, schema fingerprint) 의 hash
- 같은 witness 의 여러 mismatch 는 1 entry

ADR-EXT-003 에서 normalize 규칙 동결.

## 5. 라이선스

- SQLancer 본체 (재사용 시): MIT — vendoring 자유 (survey §12.8)
- 입력 corpus 의무 없음

## 6. 후속 작성 트리거

ADR-EXT-003 의 corpus 위치 + dedup 규칙 결정 후 본 문서 FULL 로 보강.
