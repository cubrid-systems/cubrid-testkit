# CUBRID Test Kit

CTP(구 cubrid-testtools)의 후계. CUBRID 의 기능 테스트를 실행하는 러너이며,
strangler-fig 로 CTP 를 점진 대체한다.

## Language

### 실행 단위 — 네 층을 구분한다

이 넷이 섞이면 실제 설계 오류가 난다. `jdbc` 는 shell **모듈**(jar)을 공유하지만 독립된
**task** 이므로, 모듈을 옮겨도 task 는 따라오지 않는다. 이 구분을 놓쳐 한 번 틀렸다.

**Task**:
`ctp.sh` 가 위치 인자로 받는 이름. `shell` `sql` `isolation` 등 14개. 동결된 외부 표면이다.
_Avoid_: command, 명령, 컴포넌트

**Suite**:
conf 파일 하나와 그것이 지정하는 실행 프로파일. `shell` 과 `shell_ci` 는 같은 task 의 다른 suite 다.
_Avoid_: 프로파일, 환경

**Module**:
**구 CTP** 의 디렉터리와 그 산출 jar. `CTP/shell/` → `cubridqa-shell.jar`. 신 시스템에는 module 이 없다.
_Avoid_: 패키지, 컴포넌트

**Runner**:
**신 시스템**에서 task 하나 이상을 담당하는 대체 단위. strangler-fig 의 대체는 이 단위로 일어난다.
_Avoid_: 모듈, 핸들러, 실행기

### 축

**축 T (테스트 실행)**:
없으면 테스트 *결과*가 달라지는 기능. 이번 마이그레이션의 대상이다.
_Avoid_: 코어, 본체

**축 O (QA 운영)**:
없어도 테스트 결과는 같은 기능 — 스케줄·메일·이슈 등록·큐·리포트. 제외하고 기록한 뒤 나중에 새 층으로 세운다.
_Avoid_: 부가 기능, 주변부

### 동결

**동결 표면**:
새 시스템이 CTP 와 똑같이 유지해야 하는 관측 가능한 것. CLI·conf·stdout 마커·결과 파일·종료 코드·원격 실행 컨트랙트.
_Avoid_: 인터페이스, API, 호환성

**F1 / F2 / F3 / NF / 미결**:
동결 등급. F1 은 바이트 단위 동일, F2 는 의미 동일, F3 는 입력 수용, NF 는 비동결.
등급을 못 붙인 것은 NF 가 아니라 **미결**이며 해소 시점을 함께 적는다.
_Avoid_: strict/loose, 필수/선택

**Legacy Runner**:
아직 대체하지 않은 task 를 기존 CTP 자산의 **subprocess 호출**로 처리하는 Runner.
_Avoid_: 어댑터, 브리지, 호환 layer

**라우팅 shim**:
task 를 native Runner 와 legacy Runner 중 어디로 보낼지 정하는 층.
NG5 가 금지하는 **jar 호환 layer**(구 모듈이 신 구현을 라이브러리로 호출)와 다른 것이다.
_Avoid_: 브리지

### 케이스

**Case**:
testcases 레포의 실행 단위 하나. 형식에 따라 `.sh` `.sql` `.ctl`.
_Avoid_: 테스트, TC, 시나리오

**Scenario**:
conf 의 `scenario` 키가 가리키는 **케이스 트리의 루트 디렉터리**. 케이스 하나가 아니다.
_Avoid_: 케이스, 테스트셋

**Answer**:
케이스의 기대 출력 파일. sql 계열은 variant 가 여럿이고 선택 알고리즘이 있다.
_Avoid_: expected, 정답 파일, 골든

**Format**:
케이스를 발견하고 실행하고 판정하는 방식. `sh` `sql` `ctl`. **모듈마다 디렉터리 레이아웃이 다르다.**
_Avoid_: 파서, 타입
