# E4 — Test Corpus (STUB)

**Status:** STUB — 코퍼스 정책은 ADR-EXT-004 incubating 정식 진입 후.
**Source:** `requirements.md` §5

---

## 1. 코퍼스 종류

| 코퍼스 | 입력/출력 | 용도 |
|---|---|---|
| anomaly catalog | 입력 | Adya/Bailis taxonomy + Hermitage — 의미 카탈로그 (지식 자산) |
| seed corpus | 입력 | regression seed — 과거 violation 의 fault sequence replay |
| violation corpus | 출력 | analyzer 가 검출한 anomaly 누적 |

## 2. anomaly catalog (Hermitage 차용)

- Adya 분류: dirty read / lost update / read skew / write skew / phantom 등
- Bailis 분류: causal / monotonic / read-your-writes 등
- testkit 자체 자산으로 보관 — 외부 license 의무 없음 (지식)

## 3. 보관 정책 (NG1 점검)

- ❌ testcases 레포에 두지 않음
- ✅ testkit 내부 또는 외부 storage
- ✅ history 가 거대해질 수 있음 (AWDIT 의 *huge history scalability* 고려) — GC 정책 명시 필요

## 4. violation 항목 구조 (의제)

```
violations/<witness-hash>/
   ├── seed
   ├── topology.json       # node count / version / config
   ├── fault_seq.json      # 재현용 fault timeline
   ├── history_excerpt.log # cycle 또는 violation 직전 / 직후
   ├── analyzer_report.txt # AWDIT 또는 Jepsen 의 진단
   └── reproducer.sh
```

## 5. 라이선스

- AWDIT: 정확한 repo / artifact 확인 필요 (incubating 진입 시 보강)
- Jepsen: Eclipse Public License 1.0 (Clojure)
- Hermitage / Elle: 지식 자산 (직접 도구 도입 아님)

## 6. 후속 작성 트리거

ADR-EXT-004 + N24/N11 graduation 후 본 문서 FULL 로 보강.
