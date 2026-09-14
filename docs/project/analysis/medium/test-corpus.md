# medium — Test Corpus

**Source repo:** `cubrid-testcases/medium/` (public)

medium 의 testcases 는 sql 와 *같은 형식* (`.sql` + `.answer`) 이지만 별도 디렉터리. case-formats.md §3 + sql/test-corpus.md 위에서 medium 만의 차이를 정리.

---

## 1. 규모

| 확장자 | 갯수 | 의미 |
|--------|------|------|
| `.sql` | 970 | 케이스 |
| `.answer` | 982 | 정답 (1:1 + 12 변형) |
| `.api` | 10 | medium 전용 — API 호출 케이스 (추정) |
| `.0_S64_patch` | 3 | 빌드 라인 0_S64 한정 패치 (sql과 동일 메커니즘) |
| `.0_D_patch` | 3 | 빌드 라인 0_D 한정 |
| `.answer_win` | 2 | Windows 변형 |
| `.gz` | 1 | 압축 시드 데이터 |
| `.org`, `.nt`, `.ec` | 각 1 | 작은 데이터 quality 노이즈 (운영 부산물 추정) |

→ sql 의 charset 매트릭스 (`.answer_D_<DB>_C_<C>[_coll]`) 는 medium 에 *없음* — medium 은 charset 변형 검증을 안 한다는 의도.

---

## 2. 디렉터리 구조

```
cubrid-testcases/medium/
├── _04_full/{cases,answers}/        — full data scenario
├── _06_fulltests/{cases,answers}/   — full tests
├── _07_mc_dep/{cases,answers}/      — multi-class dependent
├── _08_mc_ind/{cases,answers}/      — multi-class independent
├── config/                           — 코퍼스 메타 (정밀 분석 후속)
└── (총 11 sub-dirs at depth 1, 위는 일부)
```

→ **표준 cases/+answers/ 자매 디렉터리** (sql 과 동일 패턴)

→ `mc_dep` / `mc_ind` 는 *Multi-Class Dependent / Independent* 로 추정 — 테이블 간 의존성 vs 독립성 검증. medium 의 핵심 시나리오가 *데이터 모델의 클래스 간 관계 검증* 임을 시사.

---

## 3. 케이스 형식 (sql 과 동일)

- 표준 SQL + CUBRID 확장
- `--+ pragma` 주석 지시어
- 표준 `===...===` 구분자 정답 포맷

case-formats.md §2 / sql/test-corpus.md §4-§5 의 모든 컨벤션이 medium 에 그대로 적용.

---

## 4. medium 만의 차이

### 4-1. `.api` 케이스 (10개)

- sql 에는 없는 확장자
- 추정: API 호출 (stored procedure / native API) 시나리오
- 정밀 형식 후속

### 4-2. `.gz` 시드 (1개)

데이터 적재용 압축 파일. medium 의 *대량 데이터 적재* 시나리오 입력. 새 시스템에서 *.gz seed 자동 압축 해제 / streaming 적재* 정책 결정.

### 4-3. charset 매트릭스 부재

sql 은 `.answer_D_<...>_C_<...>` 변형 80~86개씩 보유. medium 은 0개. → medium 시나리오는 *charset 변형 검증 의도가 없다* (sql 이 cover).

### 4-4. mc_dep / mc_ind 분류

표준 sql 디렉터리 (`_NN_<topic>` 의 38개) 와 다른 *데이터 모델 의존성 분류*. 새 시스템에서 *시나리오 분류 메타데이터* 를 일반화.

---

## 5. 코퍼스 운영 contract (sql 과 같으나 small-corpus 특성)

- 코퍼스가 작으므로 (970) *전체 회귀 실행* 비용이 작음 — CI 에서 매번 실행 가능
- `data_file` config 키가 *코퍼스 외부 데이터 입력* 을 가져오므로, *.gz seed + data_file 의 외부 아카이브* 두 채널 모두 지원
- mc_dep / mc_ind 분류는 *test ordering 결정* 에 사용 가능 (의존 케이스 먼저, 독립 케이스 병렬)

---

## 6. medium_dev 코퍼스

medium_dev 는 별도 testcases 디렉터리가 *없음* — 같은 `cubrid-testcases/medium/` 사용. config 만 다름 (`create_table_reuseoid` 추가).

→ medium_dev 는 *medium 의 environment overlay*. 새 시스템에서 base + overlay 패턴.

---

## 7. 케이스 작성자 측 contract (동결 표면)

새 시스템 medium 대체 시 (sql 의 contract 위에서):

1. ✅ `cubrid-testcases/medium/` 디렉터리 트리 보존
2. ✅ `_04_full` / `_06_fulltests` / `_07_mc_dep` / `_08_mc_ind` 카테고리 명명
3. ✅ `cases/` + `answers/` 자매 디렉터리
4. ✅ `.sql` + `.answer` 페어 (sql 형식 동일)
5. ✅ `.api` 케이스 처리 (정밀 형식 후속 — 동결 의무)
6. ✅ `.gz` 시드 데이터 처리
7. ✅ `data_file` config 키로 외부 데이터 아카이브 입력
8. ✅ db_name = `mdb` 약속

---

## 8. 새 시스템 corpus 운영 권고

1. **medium = sql 의 sub-suite 표현** — 별도 testcases 디렉터리는 유지하되, *case metadata* 로 medium 시나리오 분류:
   ```sql
   -- @suite: medium
   -- @category: full
   -- @data: mc_dep
   ```

2. **`.api` 케이스 정체 확정** — Phase 1 진입 전 정밀 분석. 폐기 / 일반 .sql 통합 / 별도 처리 ADR.

3. **`.gz` seed 처리 표준화** — 자동 압축 해제 / streaming / 일회성 적재 정책.

4. **mc_dep / mc_ind 메타 활성화** — 새 시스템 dispatcher 가 의존 메타를 *test ordering* 에 사용.

5. **medium_dev 의 overlay 모델 확장** — config overlay 패턴을 sql/sql_by_cci/kcc/neis*/sample 에도 적용.

6. **coexistence 기간 동안 medium 코퍼스 무수정** — 테스트 자산은 수정 불가 (ROADMAP §1).
