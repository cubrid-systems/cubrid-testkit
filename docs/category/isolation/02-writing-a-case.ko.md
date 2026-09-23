# 2. 케이스 쓰기

*[English](02-writing-a-case.md) · 한국어*

[← isolation 카테고리로](README.ko.md)

- [케이스는 어디에 사는가](#케이스는-어디에-사는가)
- [언어](#언어)
- [기다림](#기다림)
- [답](#답)
- [케이스가 기대면 안 되는 것](#케이스가-기대면-안-되는-것)

## 케이스는 어디에 사는가

```
_01_ReadCommitted/serial/
  serial_12.ctl
  answer/serial_12.answer
```

맨 위 단계는 케이스의 클라이언트들이 쓰는 격리 수준 (`_01_ReadCommitted`, `_02_RepeatableRead`,
`_04_RepeatableRead_ReadCommitted`, `_05_ReadCommitted_RepeatableRead`, `_07_serializable`) 이나
기능 (`_06_features`) 의 이름을 딴다. 케이스는 아무 `*.ctl` 파일이고, 그 답들은 옆의 `answer/`
에 있다. 런은 `result/` 를 더한다.

케이스 옆의 선택적 파일 둘이 `runone.sh` 가 하는 일을 바꾼다: `<name>.sql` 은 케이스 전에 csql
로 돌아가고, `<name>.sh` 는 답 비교를 자기 스크립트로 대신한다. 코퍼스의 어떤 케이스도 둘 중
어느 것도 쓰지 않는다.

## 언어

`.ctl` 파일은 구문의 나열이고, 각 구문은 따옴표와 주석 바깥의 `;` 로 끝난다. `/* */` 와 `--` 는
주석이다. 구문은 컨트롤러의 것 (`MC:`) 이거나 클라이언트의 것 (`C1:`, `C2:`, …) 이다:

```
MC: setup NUM_CLIENTS = 2;
C1: set transaction lock timeout INFINITE;
C1: set transaction isolation level read committed;
C2: set transaction lock timeout INFINITE;
C2: set transaction isolation level read committed;

C1: CREATE SERIAL s1 START WITH 101 INCREMENT BY 1 MAXVALUE 20000;
C1: commit work;
MC: wait until C1 ready;

C1: SELECT SERIAL_NEXT_VALUE(s1,10);
MC: wait until C1 ready;
C2: DROP SERIAL s1;
MC: wait until C2 blocked;
C1: commit;
MC: wait until C2 ready;
C2: commit;
C2: quit;
C1: quit;
```

클라이언트 구문은 SQL 이고, 그 클라이언트의 `qacsql` 이 자기 연결로 보낸다. 컨트롤러의 구문들은
`qactl` 이 읽는 대로 (대소문자는 상관없다):

| 구문 | 무엇을 하는가 |
|---|---|
| `setup NUM_CLIENTS = <n>;` | 클라이언트 `n` 개를 시작한다. 모든 케이스의 첫 구문이다 |
| `wait until C<n> ready;` | 클라이언트 `n` 이 받은 것을 끝낼 때까지 |
| `wait until C<n> blocked;` | 클라이언트 `n` 이 락을 기다리게 될 때까지 — 시간으로 짐작하지 않고 엔진의 락 테이블에서 읽는다 |
| `wait until C<n> unblocked;` | 더 이상 그렇지 않을 때까지 |
| `wait until C<n> finished;` | 그 클라이언트가 돌릴 것을 전부 돌릴 때까지 |
| `pause for deadlock resolution;` | 3 초. 엔진이 희생자를 고르도록 |
| `sleep <n>;` | `n` **초** |
| `wait for <n>;` | `n` 초. 그동안에도 클라이언트들은 계속 돌본다 |
| `reconnect;` | 컨트롤러 자신의 연결을, 다시 |

기다림에는 100 초가 주어진다. 그때까지 더 이상 참이 될 수 없게 되면 — 막혀야 할 클라이언트가
ready 이거나, 모든 클라이언트가 막혀 있거나 — 거기서 실패한다. 그렇지 않으면 컨트롤러가 경고를
찍고 300 초까지 더 기다린다. 실패한 기다림은 락 테이블을 찍고 케이스를 끝내며, 그 케이스는 자기
답에 대해 실패한다.

## 기다림

**순서가 중요한 모든 지점은 `wait until` 이어야 한다.** 연달아 구문을 받은 두 클라이언트는 그것을
동시에 돌린다. 그 출력이 답의 순서대로 나오게 만드는 유일한 것은 그 사이에서 기다리는 컨트롤러다.
`blocked` 가 이 언어가 존재하는 이유다: 케이스가 "C2 가 이제 C1 의 락을 기다린다" 고 말하고 정확히
그때 이어가게 해준다.

`sleep` 은 동기화 지점이 아니다. 그것은 시간이고, 부하가 걸린 머신에서의 시간은 그 답이 기록될
때의 시간이 아니다.

## 답

클라이언트가 찍은 것 — 행 수, 결과, 오류 — 은 `result/<name>.result` 로 가고, 컨트롤러가 줄마다
`| ` 를 앞에 붙인다. `result/<name>.log` 는 `sed` 열다섯 단계를 거친 같은 것이고, 비교되는 것은
그쪽이다. 그 단계들은 커밋과 롤백 확인, `set transaction` 줄, 컨트롤러와 엔진의 상태 줄
(`QACTL…`, `INFO…`, `Transaction index`, `shutting down`), 구문 에코 — `;` 로 끝나는 모든 줄 —
`Ope_no` 블록과 빈 줄을 지운다. 그리고 런마다 달라지는 것을 가린다: 교착 중단에서 희생자의 이름,
죽은 pid, OID 와 `key: n` 값, 그리고 락 대기 오류 안의 호스트 이름과 숫자.

```
| 3 rows affected
| =================   Q U E R Y   R E S U L T S   =================
| 
| 
|    110  
| 1 row selected
```

케이스는 답을 둘 이상 가질 수 있다 — `.answer`, `.answer1`, `.answer2`, `.answer_1` — 전부 옳은
결과들에 대해서. 처음 맞는 것이 `ls` 순서로 이긴다. 59 케이스가 둘 이상을 갖고 있다. 철자가 틀린
이름 (`.asnwer`) 은 결코 읽히지 않는다.

## 케이스가 기대면 안 되는 것

코퍼스 전체 런 세 번과 단독 재실행으로 측정했다
([`isolation-baseline.md`](../../project/evidence/isolation-baseline.md) §4):

- **쿼리가 정렬하지 않은 행들의 순서.** 여섯 케이스가 런 안에서는 실패하고 단독으로는 통과하는데,
  그 diff 는 같은 카탈로그 행들이 다른 순서로 놓인 것이다 — 그중 셋은 네 슬롯일 때만 그렇고, 이때
  슬롯마다의 `ctldb` 는 서로 다른 케이스 순서를 겪은 상태다. `ORDER BY` 가 그 각각을 케이스 둘이
  아니라 하나로 만든다.
- **앞선 케이스들이 남긴 것.** 케이스 사이에서 `runone.sh` 는 사용자, 트리거, 시리얼, 저장
  프로시저, 뷰, 테이블을 없애고 그 밖의 것은 아무것도 하지 않는다 — `ctldb` 자체는 런 내내 살아
  있고, 크래시 뒤에만 다시 만들어진다.
- **아무것도 기다리지 않는 타이밍.** 여섯 케이스가 런마다 뒤집힌다 — 어느 클라이언트의
  `rows affected` 가 먼저 떨어지는지, 어느 구문이 유일 제약 위반을 만나는지 — 이 러너에서만큼
  CTP 에서도 그렇다.
