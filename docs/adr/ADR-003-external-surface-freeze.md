# ADR-003: External Surface Freeze — 동결 범위와 등급 체계

- **Status:** **Accepted** (2026-09-02)
- **Date:** 2026-09-02
- **Trigger:** Phase 0 분석 종료 (ROADMAP §1 의 ADR-003 자리표시자)
- **Depends on:** ADR-001 (Go 채택 — jar 산출이 불가능해짐)
- **규범 본문:** `concept/external-surface-freeze.md`

---

## 1. Context

ROADMAP 의 제약 3개 중 하나가 *"외부 인터페이스 동결"* 이다. 그러나 Phase 0 이 끝난 시점에도 **"외부 인터페이스"가 정확히 무엇인지** 정의되지 않았다. 후보가 최소 6층이다:

1. `ctp.sh` CLI 문법과 task 이름
2. `conf/*.conf` 파일명과 81 키 스키마
3. stdout 마커 / 결과 파일 포맷 / 종료 코드
4. 원격 실행 컨트랙트 (`runone.sh` 시그니처, `init.sh`, UNITTEST 4함수)
5. `common/ext/run_*.sh` · `common/script/*` 셸 진입점
6. **`cubridqa-*.jar` 7개 산출물 이름 + Java 클래스/메서드 API**

6번을 포함하느냐가 결정의 핵심이다. `analysis/common/io-contract.md` §5 는 jar 이름이 ctp.sh 의 classpath, 모듈 jar 의 MANIFEST Class-Path, 외부 CI 스크립트에서 *고정 이름으로 참조* 될 가능성을 제기했고, §7 에서 "Option A: jar 호환 layer" 를 대안으로 열어 두었다.

동시에, 동결 범위를 정의하지 않으면 **어떤 변경이 위반인지 판정할 수 없어** Phase 3 의 회귀 동등성 게이트가 작동하지 않는다.

## 2. Decision Drivers

1. **ADR-001 = Go** — Go 바이너리는 jar 를 산출하지 않는다. 6번을 동결하려면 *별도 JVM 호환 모듈*을 영구히 유지해야 하며, 이는 단일 바이너리 배포(`north-star.md` §5-3)를 무효화한다.
2. **동결은 비용이다** — 동결한 만큼 재설계 자유도가 사라진다. 넓게 잡으면 안전하지만 프로젝트 목적(M1~M5)이 훼손된다.
3. **판정 가능해야 한다** — "동결됨/아님" 이 이분법이면 애매한 항목에서 매번 논쟁이 생긴다. 등급이 필요하다.
4. **증거 부족을 인정해야 한다** — 외부 CI·운영 스크립트가 무엇을 grep 하는지 전수 확인되지 않았다(`external-surface-freeze.md` §11-1/§11-2).

## 3. Options Considered

### Option A — 전면 동결 (jar + Java API 포함)

- **Pros:** 어떤 외부 자산도 깨지지 않는다. 공존이 가장 매끄럽다.
- **Cons:** Go 선택과 양립 불가. JVM 호환 layer 를 영구 유지해야 하고, `getRadomNum` 같은 typo 까지 물려받는다. 사실상 ADR-001 을 되돌리는 결정.

### Option B — 사용자 관측 표면만 동결 (CLI / conf / 출력 / 종료 코드 / 원격 컨트랙트)

- **Pros:** *사람과 CI 가 실제로 보는 것*과 동결 범위가 일치한다. 내부 구현 전면 재설계 가능. Go 와 정합.
- **Cons:** jar 이름을 직접 참조하는 외부 자산이 있다면 그것이 깨진다 — 다만 그 자산은 *고쳐야 할 대상*이지 *호환해야 할 대상*이 아니다.

### Option C — 동결 없이 "최선 노력 호환"

- **Pros:** 최대 자유도.
- **Cons:** Phase 3 Exit 의 *회귀 동등성* 게이트가 판정 불가능해진다. ROADMAP §8 의 2번 risk(외부 표면 동결 위반)에 대한 완화 수단이 사라진다.

## 4. Decision

**Option B 를 채택한다.** 추가로 **4등급 체계(F1 / F2 / F3 / NF)** 를 도입한다.

| 등급 | 의미 |
|---|---|
| **F1 (strict)** | 바이트 단위 동일. 외부 도구가 grep/파싱하는 표면 |
| **F2 (semantic)** | 의미 동일. 순서·공백·부가 정보는 자유 |
| **F3 (compat-input)** | 기존 입력을 수용해야 함. 내부 표현은 자유 |
| **NF (non-frozen)** | 자유 재설계 |

**기본값 규칙: 의심스러우면 F1.** §1-4 의 증거 부족(외부 grep 대상 미확인) 때문에 stdout 마커와 `ext/run_*.sh` 는 보수적으로 F1 로 두고, `external-surface-freeze.md` §11 의 확인 작업 이후 하향 조정한다.

**동결 대상에서 명시적으로 제외:** `cubridqa-*.jar` 7개 산출물 이름·경로, Java 클래스/메서드 API(`CommonUtils` 40+ / `IniData` 15 / `ConfigParameterConstants` 82 상수), RMI·ActiveMQ·grepo wire 프로토콜, `#SCRIPTCONT` 전달 메커니즘, dispatch 내부 모델, orphan 7 task.

## 5. Why

- **Go 결정의 직접 귀결.** jar 를 동결 대상에 넣는 순간 ADR-001 이 무의미해진다. 두 ADR 중 하나를 포기해야 한다면 포기할 것은 jar 호환이다 — jar 는 *구현 산출물*이지 *사용자 계약*이 아니다.
- **"외부"의 정의를 사용자 기준으로 잡는 것이 옳다.** CTP 를 쓰는 사람은 `ctp.sh` 를 치고, conf 를 고치고, 결과 파일을 읽는다. `CommonUtils.getFileContent` 를 호출하지 않는다.
- **등급 체계가 논쟁을 종결시킨다.** 새 항목이 등장하면 등급을 붙이는 것으로 결정이 끝난다. 등급이 없으면 동결 대상이 아니라는 규칙이 함께 성립한다.

## 6. Consequences

1. **NG5 발생** — Java API / jar 호환 제공은 non-goal (`concept/non-goals.md`). `analysis/common/io-contract.md` §7 Option A 는 **기각됨**으로 표시된다.
2. **공존은 subprocess 로만** — 신 Go 바이너리가 미대체 task 를 처리할 때 기존 CTP 자산을 프로세스로 호출한다. 라이브러리 링크·JNI·RMI 우회는 없다.
3. **Phase 3 회귀 동등성 게이트의 판정 기준 확정** — F1 항목은 diff 0, F2 는 필드 단위 비교, F3 는 수용 여부. `docs/evidence/regression-shell.md` 가 이 등급별로 증거를 낸다.
4. **§11 확인 작업 7건이 Phase 진입 조건으로 이월** — 특히 §11-1(ext/script 외부 호출자)과 §11-2(CI grep 대상)는 Phase 2 진입 전, §11-5·§11-6 은 Phase 3 착수 전(ADR-004 Consequence 4).
5. **jar 참조 자산 발견 시의 정책** — 호환 layer 를 만들지 않고 **그 자산을 고친다**. 이는 NG5 의 명시적 귀결이다.
6. **본 ADR 은 `concept/external-surface-freeze.md` 를 규범 문서로 위임한다** — 등급 부여의 변경은 그 문서 개정으로 이루어지며, *범위 자체*(무엇을 동결 대상 후보로 보는가)의 변경만 본 ADR 의 supersede 를 요구한다.
