# common — Implementation Notes (미묘 동작 + 깨지기 쉬운 가정)

**Source:** `cubrid-testtools/CTP/common/`

design.md 의 구조 위에서, 새 시스템 작성 시 회귀 위험이 큰 미묘한 동작들.

---

## 1. `cubridqa-common.jar` 안에 *세 다른 origin* 이 합쳐져 있음

build.xml 패턴 매칭으로 다음이 1 jar 에 머지 (deps-of-common.md §3, design.md §9):
1. `common/src/com/navercorp/cubridqa/common/**` (메인 src)
2. `common/grepo/src/com/navercorp/cubridqa/common/grepo/service/**` (별도 sub-module)
3. `common/src/com/navercorp/cubridqa/ctp/**` (CTP CLI 호스트)
4. `com/nhncorp/cubrid/common/grepo/*` (legacy nhn 네임스페이스)

**위험:**
- 새 시스템에서 *명시적 모듈 분리* 안 하면 같은 빌드 트릭 이어가야 함
- *클래스 충돌* 가능성: `common.LocalInvoker` 와 `common.coreanalyzer.LocalInvoker` 등 이름 중복

→ 새 시스템에서 *명시적 jar/모듈 분리* 권고.

---

## 2. `common.LocalInvoker` ≠ `coreanalyzer.LocalInvoker`

```
common/src/.../common/LocalInvoker.java
common/src/.../common/coreanalyzer/LocalInvoker.java
```

같은 *이름* 다른 *패키지*. *각자의 용도* 추정 — coreanalyzer 의 LocalInvoker 는 coreanalyzer 전용 셸 호출 utility.

새 시스템에서 *이름 충돌 방지* 로 명시적 분리 또는 단일화.

---

## 3. `IniData` 의 `<property>` dot-notation 와일드카드

IniData 가 ini4j 위에 얇게 래핑하면서 *dot-notation prefix 매칭* 을 추가. 예:
```
default.cubrid.cubrid_port_id=1523
default.cubrid.async_commit=on
```
→ `parsePropertiesByPrefix(props, "default.cubrid.")` 가 *cubrid 이하 모든 키* 를 회수.

ini4j 자체는 INI section 만 지원 — dot-notation 매칭은 *CommonUtils.parsePropertiesByPrefix* 에서 구현. 정밀 알고리즘은 후속.

**위험:** 새 시스템이 ini4j 를 다른 파서로 교체하면 *dot-notation 매칭 동작 보존* 필수. 모든 모듈이 이 동작에 의존.

---

## 4. `getPropertiesWithPriority` — 우선순위 의미 미파악

```java
public static Properties getPropertiesWithPriority(String filename) throws IOException
```

`getProperties` 와 별개. *우선순위* 가 무엇을 의미하는지 정밀 분석 필요 (시스템 프로퍼티 / 환경변수 / config 의 우선순위?).

isolation.Context.reload() 가 이 메서드 사용:
```java
this.config = CommonUtils.getPropertiesWithPriority(filename);
```

→ 새 시스템에서도 같은 우선순위 보존 필수. 정밀 동작 후속.

---

## 5. `getRadomNum` — 명백한 typo

```java
public static int getRadomNum(int max)   // Random → Radom
```

외부 호출자가 *typo 그대로* 의존하고 있을 가능성. 새 시스템 cleanup 시 *호환 alias 유지* 또는 deprecation warning 후 제거.

---

## 6. `parseInstanceParametersByRole` 의 role 매개변수

```java
public static String parseInstanceParametersByRole(Properties cfg, String instancePrefix, String role)
```

instance prefix (`env.instance1.`) + role (`broker1` / `cubrid` / `ha` / `cm`) 결합으로 *원격 cubrid.conf 에 적용할 파라미터* 추출. 이 함수가 *모듈 간 conf 모델의 핵심 로직*.

→ 새 시스템에서 *동일 한 의미* 보존. 알고리즘 정밀 후속.

---

## 7. `cleanFilesByDirectory` 의 안전성

```java
public static void cleanFilesByDirectory(String dir)
```

이름이 *디렉터리 안 모든 파일 삭제* 시사. 잘못된 인자 (`/`, `~`, 빈 문자열) 시 위험. 호출처:
- isolation.TestFactory.execute(): `CommonUtils.cleanFilesByDirectory(context.getCurrentLogDir())`

new system 에서 *path validation 강화* (예: `~/`, `/`, 절대경로 outside CTP_HOME 거부).

---

## 8. `getEnvInFile` 의 정체

```java
public static String getEnvInFile(String var)
```

`var` 환경변수를 *파일에서* 가져옴? 정밀 동작 후속. CTP 의 `CTP_HOME` 처럼 ctp.sh 가 *export* 한 변수를 *Java 프로세스가 읽는* 메커니즘.

ctp 패키지의 `CTP.java` 가 사용:
```java
private final static String ctpHome = CommonUtils.getEnvInFile("CTP_HOME");
```

`getEnvInFile` 의 구현 — System.getenv() 단순 wrapper인지 또는 *별도 파일에서 읽는 트릭* 인지 후속 조사.

---

## 9. `Log` 의 append vs truncate 모드

```java
public Log(String filename, boolean isAppend, boolean ...)
```

3-인자 생성자. Test.java 의 사용:
```java
this.dispatchLog = new Log(filename, false, laterJoined ? true : context.isContinueMode());
```

3번째 인자의 의미 정밀 후속. *fsync 정책 / autoflush / batching* 중 하나로 추정.

→ 새 시스템에서 같은 시맨틱 보존.

---

## 10. `ShareMemory` 의 1024 byte 디폴트

```java
public ShareMemory(String fname, int size) {
    this.fname = fname;
    this.size = size;
}
private int size = 1024;   // 디폴트 (생성자 인자 size 가 덮어씀)
```

MappedByteBuffer 의 size — 1024 가 디폴트 (하지만 항상 인자로 override 됨). 케이스 작성자가 *file 이름과 size* 를 약속하고 sql cqt 와 native 가 *같은 mmap 파일* 을 공유. **race / sync 정확히 어떻게 보장되는지** 후속 분석.

`wait(expectedValue, times)` 메서드:
```java
public boolean wait(String expectedValue, int times) throws Exception
```
- polling 으로 mmap 값을 N 번 체크
- expectedValue 일치하면 true return

**문제점:**
- polling = busy wait (CPU)
- `Object.wait()` 와 메서드 이름 충돌 → IDE warning
- *race condition* 가능 (Native 측이 *write 도중* Java 가 read)

→ 새 시스템에서 *명시적 동기화 primitive* (semaphore / mutex / signal) 권고.

---

## 11. `JschLogger` 의 *전역 로거 hijack*

`JschLogger` 가 jsch 의 `setLogger(myLogger)` 호출. **Process 단 1개만 hijack 가능** — 다중 SSH 세션 동시에 *각자의 logger* 못 가짐.

→ 새 시스템에서 모던 SSH 라이브러리는 per-session logger 지원 (jsch 한계 회피).

---

## 12. `SFTP*` 클래스의 *각자 main()* 보유

```
SFTPDownload.java has main()
SFTPUpload.java has main()
SFTP.java has main()
RunRemoteScript.java has main()
```

→ 각각 *standalone CLI 도구* 로도 사용 가능. 셸 스크립트가 직접 java 호출:
```
java -cp cubridqa-common.jar com.navercorp.cubridqa.common.SFTPDownload <args>
```

이 invocation 형식이 *외부 CI 에서 의존* 가능 → 동결 표면 후보.

---

## 13. `MailSender` 의 SMTP 설정 위치

`MailSender` 가 SMTP 서버 / 발신자 / 포트 등 설정을 어디서 읽는지 정밀 후속. 추정:
- `<conf>/mail.conf` 또는 환경변수
- 발신자 주소가 hardcode 가능

→ 새 시스템에서 *명시적 conf 키* 로 노출.

---

## 14. `script/run_grepo_fetch` 의 비동기 호출

추정: grepo RMI server 에 jar 패치를 *fetch* 요청. 동기/비동기 패턴 / 타임아웃 / fallback 정책 후속.

새 시스템에서 grepo 자체 폐기 시 *git CLI subprocess* 또는 *go-git* 으로 일대일 대체.

---

## 15. `CTP.java` 의 `ctpHome` 상수 — 클래스 로드 시점 단발

```java
private final static String ctpHome = CommonUtils.getEnvInFile("CTP_HOME");
```

*클래스 로드 시점* 에 단 한 번 평가. CTP_HOME 변경 시 JVM 재시작 필수. test 실행 도중에 CTP_HOME 이 바뀔 일은 없지만, 새 시스템에서도 같은 정책 명시.

---

## 16. `ComponentEnum` 의 21 멤버 중 7 orphan

cli-tree.md §3 에서 발견. CCI / DOTS / NBD / SYSBENCH / TPCC / TPCW / YCSB 는 enum 에 있으나 CTP.main 의 switch 분기 *부재*. → silent skip.

**위험:** 사용자가 `ctp.sh tpcc` 호출 시 *조용히 아무것도 안 함* — 디버깅 어려움.

→ 새 시스템에서 *명시적 폐기 또는 NotImplementedError*.

---

## 17. `Constants.LINE_SEPARATOR` 의 OS 의존

추정: System.lineSeparator() 또는 `\n` / `\r\n` 분기. 모듈이 *케이스 stdout* 처리 시 사용. 잘못 설정 시 결과 비교에서 false-fail.

새 시스템에서 *항상 normalize* (예: 모든 line ending 을 `\n` 으로 정규화) 권고.

---

## 18. `ConfigParameterConstants` 의 82 키 vs conf-matrix.md 의 81 키

1 키 차이. 어느 키가 ConfigParameterConstants 에는 있고 실제 conf 에는 없는지 (또는 반대) 확인 필요. 차이가 **legacy / unused 키** 인지 *intentional* 인지 결정 필요.

후속 분석으로 정확히 매핑.

---

## 19. coreanalyzer 의 stack digest 알고리즘

`UpdateDigestStack.java` — core dump 의 stack frame 을 *digest hash* 로 변환. 같은 hash → 같은 issue 자동 그룹화. 알고리즘 (frame 수 / 주소 normalize / symbol 매칭) 정밀 후속.

→ 새 시스템에서 *동등 알고리즘* 보존 또는 폐기 + 새 알고리즘 ADR.

---

## 20. `loadjava` 와 cubridqa-common.jar classpath

run.sh (sql 모듈) 의 javac/loadjava 호출 시 cubridqa-common.jar 를 *케이스의 classpath 에 포함* 가능성. 케이스 안에 자바 stored procedure 가 *common 의 utility* 를 import 한다면 sql 케이스가 *common API 의 일부 표면* 에 의존.

후속 분석으로 케이스 측 의존 검증.

---

## 21. `lib/cubridqa-common.jar` 의 `MANIFEST.MF` Class-Path

build.xml 에서 `manifest="common/lib/MANIFEST.MF"` 명시. 이 MANIFEST 가 *Class-Path:* 헤더로 *third-party jar 들* 을 가리킴 → URLClassLoader 가 자동 로드. 즉 isolation/ha_repl/cdc_repl 의 `URLClassLoader(isolation.jar, parentCL)` 가 cubridqa-common.jar 를 통해 *전체 third-party 전이* 의존을 자동 흡수.

→ 새 시스템에서 *런타임 classpath 모델* 을 어떻게 표현할지 ADR (특히 새 언어로 가는 경우).
