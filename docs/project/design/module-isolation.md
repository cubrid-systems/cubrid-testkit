# Module: isolation — 매핑 표

- **Date:** 2026-09-02 · **Status:** Accepted (Phase 2, 매핑 수준)
- **담당 task:** `isolation`
- **Phase 3 에서의 처리:** **미대체 — `legacy` Runner. Phase 4 에서 대체**

---

## 1. 왜 1차가 아닌가

native C 자산(`ctltool`)을 동반해 대체 비용이 크다. 다만 `shell.common.*` 를 공유하므로
shell 대체가 끝나면 비용이 크게 준다 — 그것이 ADR-004 가 shell 을 먼저 고른 이유다.

## 2. 신 ↔ 구 매핑 (개요)

| 구 | 신 (대체 시) |
|---|---|
| `isolation.Main` | `runner/isolation` |
| `Dispatch` · 워커 루프 | **`dispatch` 재사용** (shell 이 만들어 둔 것) |
| `SSHConnect` | **`exec.ssh` 재사용** |
| `FeedbackNull` / `FeedbackFile` | **`feedback` 재사용** |
| `FeedbackDB` | **축 O — 제외** |
| `ctltool/` (qactl MC + qacsql Client, C) | **subprocess 유지** (ADR-007 1차 방향) |
| `.ctl` 파싱 | `caseformat/ctl` |
| `runone.sh` 호출 | `caseformat/ctl` 의 `Run` |

## 3. 이 모듈만의 함정

- **케이스 레이아웃이 다르다.** `_NN_<level>/<topic>/` 안에 `.ctl` 과 `.answer` 가 **동거**한다.
  `cases/`·`answers/` 자매 디렉터리가 **없다** — `cases/` 필수 규칙은 shell 한정이다 (freeze §3-1).
- **`.ctl` DSL 은 8토큰**이다 — actor 지시, `setup NUM_CLIENTS`, `wait until <Cn> ready|blocked|unblocked|finished`,
  `sleep <ms>`, `pause for deadlock resolution`.
- **`runone.sh` 의 sed 정규화 체인을 통과한 뒤에 비교가 일어난다** (freeze §7-6).
  이 체인이 다르면 모든 isolation 판정이 달라진다. 시그니처만 맞추면 되는 것이 아니다.
- 판정은 **마지막 `flag: OK` 와 마지막 `flag: NOK` 의 위치 비교**다.
- 워커 로그의 DIFF 블록은 폭 51 구분선 + `diff -a -y -W 185`. 폭까지 F1.
- 백업 tar.gz 는 **Linux/macOS 한정, Windows skip** (이 모듈에 한해 근거가 확인됨).

## 4. 미결

| 항목 | 어디로 |
|---|---|
| `runone.sh` sed 정규화 패턴 **전수 목록** | freeze §11-15 · ADR-009 |
| `.ctl` grammar 정형화 | ADR-008 |
| ctltool 통합 형태 (흡수 / subprocess / CUBRID-only) | ADR-007 |
| ctltool 의 MySQL/Oracle 드라이버 현 활성 여부 | NG4 · ADR-007 |
| 백업 tar.gz 파일명 정확한 구분자 | freeze §11-4 |
