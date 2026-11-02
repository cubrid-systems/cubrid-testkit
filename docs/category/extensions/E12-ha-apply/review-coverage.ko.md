# E12 — `ha-apply` 가 #8052 의 무엇을 덮는가, 리뷰를 감당할 수 있는가

*[English](review-coverage.md) · 한국어*

**기준:** 2026-11-02, #8052 `787defb50`
**항목의 출처:** CBRD-27459 의 코멘트(설계 문서는 첨부라 읽지 못했다), develop 에 없는 #8052 의
커밋 88 개(merge 제외), `SHOW WRITESET STATUS` 의 항목
**근거:** `docs/project/evidence/ha/`, 행마다 이름을 적는다

---

## 1. 표 읽는 법

- **verified**: 그 경우를 고친 커밋보다 앞선 head 에서 실행이 missed 였고, 그 커밋이나 그 뒤에서
  held 였다. 이런 곳의 held 는 근거다. 검사가 실패할 수 있음이 보였기 때문이다. 앞선 head 가 그
  커밋의 부모가 아니면 행에 적는다.
- **check shown**: 이 행 자체의 경우는 아니지만, 같은 관찰 경로로 검사가 어떤 head 에서 실패한
  적이 있다.
- **held only**: 돌렸고 시험한 모든 head 에서 held 나 same 이었지만, 실패하는 head 는 보이지
  않았다. 이런 곳의 held 는 "아무것도 드러나지 않았다"이지 "옳다"가 아니다.
- **not run**: 여기서는 닿지 않는다.

testkit 의 모든 실행과 `61ee0944a` 부터의 모든 실행은 RelWithDebInfo 빌드다. 이 빌드에서는
applier 의 assertion 이 빠진다. 디버그 빌드로 돌린 것은 `bf560620e` 에서 셸 프로토타입 패턴 13 개
가운데 12 개를 돌린 한 번뿐이다.

## 2. 마스터: writeset 생성

| 항목 | 동사 | 상태 |
|---|---|---|
| 인덱스가 비교하는 대로 키를 해시한다: FLOAT·DOUBLE 의 -0.0, TIMESTAMPTZ·DATETIMETZ 의 시간대, 서브클래스와 그 슈퍼클래스 | `pairs`(`fp_negzero`, `fk_negzero`, `tstz_zone`, `dttz_zone`, `fk_tz`, `inherit_uk`) | **verified**: `bf560620e` 에서 missed, `61ee0944a` 에서 held. 두 head 는 커밋 24 개쯤 떨어져 있고 그 가운데 몇은 writeset 수정이다([ha-apply-gives-the-prototypes-verdicts](../../../project/evidence/ha/ha-apply-gives-the-prototypes-verdicts.md)) |
| `ignore_trailing_space` 를 켠 쌍에서 VARCHAR 키와 뒤쪽 공백을 붙인 같은 키 | `pairs`(`vc_trail`, `fk_vc_trail`) | **verified**: `17aae152e` 의 부모 `61ee0944a` 대 `3fb54e549`([ha-apply-and-the-trailing-space](../../../project/evidence/ha/ha-apply-and-the-trailing-space.md)) |
| MONETARY 통화, VARBIT 패딩, NUL 과 공백, NaN | `pairs`(`mon_cur`, `vb_len`, `str_nul`, `dbl_nan`, `fk_*`) | **verified**: `3fb54e549` 에서 missed, `fb1636c34` 에서 held. `eb31453ff` 만 가른 실행은 없다([ha-apply-and-the-reduced-keys](../../../project/evidence/ha/ha-apply-and-the-reduced-keys.md)) |
| `ignore_trailing_space` 를 끈 쌍에서 binary collation 의 뒤쪽 NUL | `pairs`(`bin_nul`, `fk_bin_nul`) | **verified**: `fb1636c34` 에서 missed, `b984637bb` 에서 held. `cf8955a91` 은 같은 부모 위에서 다시 쓴 `eb31453ff` 이고, 그것만 가른 실행은 없다([ha-apply-and-the-binary-nul](../../../project/evidence/ha/ha-apply-and-the-binary-nul.md)) |
| 대소문자를 가리지 않는 콜레이션 | `pairs`(`ci_case`, `fk_ci`) | held only. `bf560620e` 와 그 뒤의 모든 head 에서 held |
| CHAR 키와 뒤쪽 공백을 붙인 같은 키 | `pairs`(`char_pad`, `fk_char_pad`, 지금은 뺐다) | held only. `61ee0944a` 와 `3fb54e549` 에서 held. SQL 은 CHAR 를 쓰기 전에 채우므로 어떤 패턴도 둘을 다르게 만들 수 없다([ha-apply-and-the-trailing-space](../../../project/evidence/ha/ha-apply-and-the-trailing-space.md)) |
| 그 밖의 뒤쪽 NUL, 악센트를 가리지 않는 콜레이션, 함수 기반·필터 인덱스, 복합 키 안의 축약 키 | 없음 | not run |
| PK, UNIQUE, 역 UNIQUE, FK 키가 의존성을 만든다 | `pairs` 의 기본 패턴 | **check shown**: 동치 키 패턴이 같은 경로로 실패했다. 기본 패턴은 모든 head 에서 held |
| 파티션, savepoint, DML 앞의 DDL | `pairs`(`part_move`, `sp_*`, `ddl_dml`) | held only |
| history 가 차고 비워진다 | `pairs --history 50000`, `stress --history 1000` | held only. clear 는 그 뒤의 것을 자기 뒤에 세우므로 clear 가 있는 라운드는 구조상 held 다 |
| 트랜잭션당 키 한도(history 크기와 같은 수) | `stress --history 1000` 안에서 | held only. 그 fallback 은 따로 세지 않는다 |
| 문장이 커밋 순서로 fallback 한다 | `restart --statements` | 마스터 쪽은 held only(커밋 48,007 개 가운데 48,005 개가 fallback). 슬레이브의 재시작 행은 문장에서도 verified |
| DML, unique·FK 검사가 커밋 중인 트랜잭션을 만나면 그 커밋을 기다린다 | `stress` 간접, `bf560620e` 에서 돌린 프로토타입 `parallel-apply-chaser.sh`(FK 300 라운드, unique 300 라운드, 대조군 없음) | held only. 약한 "아니오"다 |
| serial fragment: AUTO_INCREMENT 와 SERIAL 의 `_db_serial` 갱신, `INCR` | 프로토타입 `parallel-apply-serial.sh` | held only. `787defb50` 과 `3fb54e549` 에서 같았고, `3fb54e549` 은 `501e5f315` 의 경우를 재현하지 못했다 |
| `COMMIT WORK RETAIN LOCK` 에서 MVCCID self-lock 유지(`787defb50`) | 없음 | not run |

## 3. 슬레이브: 병렬 applier

| 항목 | 동사 | 상태 |
|---|---|---|
| gate 가 의존하는 것이 적용될 때까지 트랜잭션을 붙잡는다 | `pairs` | **check shown**: 의존 대상보다 먼저 적용된 트랜잭션이 보인다. 동치 키 패턴의 missed 는 해시가 의존성을 빠뜨린 것이지 gate 의 결함이 아니었다 |
| 슬레이브가 멈춘다 | `stress` | **verified**. 아래 lock escalation 행 |
| 동시 클라이언트 아래에서 두 노드가 같게 끝난다(heap, PK, 고아 행) | `stress` | FK 연산 없이는 **check shown**: `--compare-only` 가 #8198 `6ce1c05d2` 의 손상된 쌍을 damaged 로 판정했고, `lock_escalation=5` 없이 돌린 `--ops 0,1,2,3,4,9` 실행은 모든 head 에서 same 이었다. FK 연산을 넣으면 `61ee0944a` 에서 9 번 가운데 6 번, `3fb54e549` 에서 3 번 모두 diverged 였다(`fail_counter`, 그리고 각 head 에서 한 번씩 슬레이브에 행 하나가 없음). #8026 이 고치는 CBRD-27464 로 읽었지만 측정하지 않았고, `fb1636c34` 이후로는 FK 연산으로 돌리지 않았다 |
| 재시작해도 커밋된 트랜잭션을 두 번 적용하지 않는다(marker) | `restart` | **verified**: applier kill 과 노드 crash 는 `72650ce0d` 의 부모 `19e83a5d9` 대 `b984637bb`·`787defb50`, 문장은 `d398beba2` 의 부모 `b84567f36` 대 `787defb50` |
| marker 삭제가 escalation 되어 생기는 교착 | `stress --set lock_escalation=5` | **verified**: `cf8955a91` 대 그 수정인 `502685e64` 하나 |
| 4 가 아닌 worker 수 | `--workers 1`, `--workers 16` | held only. 4 에서는 실패할 수 있음이 보였다 |
| 통계 문장이 적용 정보 저장을 abort 시킨다(`b984637bb`) | 없음 | not run. 그 계기, 즉 교착 victim 으로 뽑히는 통계 문장을 여기서는 만들지 못한다 |
| 시작할 때 marker 를 일부만 읽는다(`9343b73f7`) | 없음 | not run. 여기서는 그 읽기를 실패시키지 못한다 |
| 메모리 한도(`ha_apply_max_mem_size`) | `restart --set ha_apply_max_mem_size=1` | not run. 검사가 applier 의 처음 10 초를 건너뛰는데 kill 은 5 초마다 왔다 |
| 복사 로그의 경계(끝을 넘는 레코드, archive 삭제, 한 페이지를 넘는 레코드), LOB | 없음 | not run |
| standby 에서의 읽기가 마스터에 없던 상태를 본다(서로 독립인 트랜잭션이 커밋 순서와 다르게 적용됨) | `pairs` 가 그것을 보고(대조군이 앞섬) 허용된 것으로 센다 | 질문으로는 not run. 그런 읽기를 받아들일지는 설계의 몫이다 |
| SQL log, 통계 표, `applyinfo`, 종료할 때의 메모리 해제 | 없음 | not run |

## 4. 장애

| 장애 | 상태 |
|---|---|
| 슬레이브 applier kill | **verified** |
| 슬레이브 노드 crash(`csb node kill`) | **verified** |
| 마스터 crash 와 슬레이브의 승격 | held only. 프로토타입, `787defb50` 에서 세 번 |
| 슬레이브 복구 중의 crash, graceful restart, failback, 다른 복사 모드, 네트워크 분리 | not run |

## 5. 리뷰를 감당할 수 있는가

코드 리뷰와 함께 쓰는 주된 동적 검증 방법으로는 **감당할 수 있다**. 조건은 하나다. held 는
표에서 **verified** 나 **check shown** 인 곳에서만 근거로 친다.

- 병렬 applier 가 잘못되는 방식을 덮는다. 의존성을 잃어 순서가 뒤집히는 것(`pairs`), 재시작 뒤
  커밋을 두 번 적용하는 것(`restart`), 슬레이브가 멈추거나 어긋나는 것(`stress`)이다.
- 앞의 둘과 슬레이브가 멈추는 것은, 실행이 #8052 의 실제 수정보다 앞선 head 에서 실패했고 그
  뒤에서 held 였다. 그런 곳의 held 는 아무것도 말하지 않는 검사가 아니다. 어긋나는 것은 더 약하다.
  비교가 손상을 잡는다는 것은 보였지만 #8052 수정의 앞뒤로 돌린 적은 없고, FK 연산을 넣으면 앞선
  head 들에서 diverged 였으며 그 해석은 아직 확인되지 않았다.
- head 하나는 동사마다 명령 하나다. `787defb50` 은 빌드 말고 실행에 한 시간 이상 걸렸다. 근거 문서의
  평균에서 어림한 값이다.
- 판정은 원장에 남고, 판정 규칙은 ADR-EXT-012 와 requirements 에 적혀 있으므로 리뷰어가 둘 다
  확인할 수 있다.

리뷰가 여전히 따로 맡아야 하는 것:
- held only 행. 코드 리뷰가 판단하고, held 는 기껏해야 보조 근거다.
- not run 행. 코드 리뷰로 보거나, 엔진이 시험용 결함 주입을 허용해야 한다.
- FK 연산을 넣은 `stress` 의 어긋남.
- 경합. 대부분의 판정은 head 마다 한 번 돌린 것이므로 "결함 없음"은 돌린 횟수만큼의 무게다.
- 뒤의 head 들에서 assertion 이 잡았을 것. 모두 RelWithDebInfo 로 돌렸다.
- verified 행의 앞선 head 가 그 수정의 부모가 아닌 곳에서, 그 사이에 무엇이 더 있는가.
- 설계 자체의 선택(`max_committed_lsa` 를 언제 저장하는가, MySQL 과의 비교, 마스터에 없던 상태를
  보는 standby 읽기). 시험이 아니라 설계 리뷰의 몫이다.

조건 없이 주된 검증 방법이 되려면:
1. held only 행 가운데 대조군이 닿을 수 있는 것(history clear, serial fragment, failover)마다
   실패하는 head 를 찾는다. 실패하는 head 가 없으면 엔진에 결함 주입 지점을 둔다.
2. 지금 head 에서 FK 연산을 넣은 `stress` 를 #8026 과 함께, 또는 그것이 머지된 뒤에 돌린다.
3. 타이밍에 따라 갈리는 판정은 여러 번 반복해 돌린다.

## 6. 수정 커밋에 쓰는 법

#8052 에서 어떤 경우를 고친 커밋마다:
1. 그 경우에 닿는 부하가 있으면, 그 커밋의 부모에서 missed 이고 그 커밋에서 held 인 실행을
   요구한다.
2. 닿는 부하가 없으면 코드 리뷰 항목으로 남긴다.
3. 아니면 엔진에 디버그 빌드 전용 결함 주입 지점을 두어, 실행이 그 경우에 닿게 한다.
