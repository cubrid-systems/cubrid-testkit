# E2 — Test Corpus (STUB)

*[English](test-corpus.md) · 한국어*

**Status:** STUB — 코퍼스 정책은 ADR-EXT-002 incubating 정식 진입 후.
**Source:** `requirements.md` §5, ROADMAP §8 risk (E2/E3 corpus NG1)

---

## 1. 코퍼스 종류

본 항목은 *입력 코퍼스가 없음* — random generation 이라 코퍼스는 *생성된 crash 의 누적 보관소* 로서 의미.

| 코퍼스 | 입력/출력 | 용도 |
|---|---|---|
| seed corpus | 입력 | regression seed — 과거 crash query 를 새 빌드에서 replay |
| crash corpus | 출력 | fuzz run 의 crash 누적 — stack hash 기반 dedup |

## 2. 보관 정책 (NG1 점검)

- ❌ testcases 레포 (cubrid-testcases / -private / -private-ex) 에 두지 않음 — NG1 위반
- ✅ testkit 내부 별 트리 (예: `corpus/sqlsmith/`) 또는 외부 storage
- ✅ stack hash 디렉터리당 1 entry 보관 (dedup)

ADR-EXT-002 에서 corpus 위치 + 보관 기간 + GC 정책 명시.

## 3. crash 항목 구조 (의제)

```
crash/<stack-hash>/
   ├── seed              # 재현용 random seed
   ├── query.sql         # 정규화된 SQL
   ├── stack.txt         # 스택 / signal / register
   ├── schema.sql        # 재현 시 필요한 schema dump
   └── reproducer.sh     # 1-shot 재현 스크립트
```

## 4. dedup 정책 (의제)

- 1차: stack frame top N (signal frame 제외) 의 hash
- 2차: query AST shape (literal 제거 후 normalize) 의 hash

ADR-EXT-002 에서 N / normalize 규칙 동결.

## 5. 라이선스 / 외부 의존

- SQLsmith 본체 (재사용 시): custom 라이선스 — vendoring 정책 점검 필요 (survey §12.8)
- 입력 corpus 가 없으므로 외부 license 의무는 *재사용 결정 시점* 에만 발생

## 6. 후속 작성 트리거

ADR-EXT-002 의 corpus 위치 + dedup 규칙 결정 후 본 문서 FULL 로 보강.
