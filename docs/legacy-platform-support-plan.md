# BREW·LGT 다운타운·GVM 지원 계획과 코드 재사용 평가

작성일: 2026-09-11. 상태: 계획 / 정적 조사 완료, 실행 호환성 미검증.
조사 기준: aram-core `66b4489`, aram-emu `159e197` 및 당시 로컬 소스.
이 문서는 제품 전체 확장 계획이므로 aram-emu가 소유한다.

## 1. 결론과 추정의 의미

지원 추가는 현재 아키텍처 안에서 가능하다. 우선순위는 **LGT 다운타운 →
BREW → GVM**으로 제안한다. 다운타운은 기존 Java 실행기를 활용할 수 있고,
BREW는 ARM CPU를 활용하지만 실행 환경이 새로 필요하며, GVM은 바이트코드
실행기 자체가 필요하다. 세 플랫폼 모두 공통 서비스와 제품 UI는 활용한다.

아래 비율은 코드 줄 수나 게임 호환률을 측정한 값이 아니다. 처음부터 각
플랫폼의 오프라인 게임 실행 경로를 만든다고 가정했을 때, 필요한 구현 책임 중
기존 구성요소로 담당할 수 있을 것으로 판단한 **설계상 재사용 범위**다.
연결·수정·검증 비용은 별도로 남으므로 개발 시간 절감률로 해석하면 안 된다.
플랫폼별 대표 게임의 첫 화면 검증 후 다시 산정한다.

| 범위 | 다운타운 | BREW | GVM |
|---|---|---|---|
| UI·공통 서비스·실행기를 포함한 전체 경로 | 약 65–80% | 약 35–50% | 약 25–40% |
| 플랫폼 고유 로더·실행기·API 연결만 비교 | 약 50–70% | 약 15–30% | 약 5–15% |
| 추정 확신도 | 중간: Java 클래스와 기존 구현 확인 | 낮음: 실제 모듈 ABI 미분석 | 낮음: 명령·리소스 형식 미완성 |

가장 확실한 판단은 비율의 정확한 숫자보다 **다운타운은 실행기까지 재사용,
BREW는 CPU 이하와 공통 서비스 재사용, GVM은 공통 서비스 중심 재사용**이라는
차이다. 신규 코어 작업은 셋 모두 aram-core에 두며 별도 에뮬레이터 제품이나
플랫폼마다 복제한 프론트엔드를 만들 필요는 없다.

## 2. 실제 입력에서 확인한 범위

사용자가 지정한 로컬 코퍼스를 원위치에서 읽었다. 입력 복사·수정·실행은 하지
않았다. ZIP 엔트리 목록과 다운타운의 내부 JAR/JAD를 메모리에서 조사했다.
공개 문서에는 개인 절대 경로, 게임 바이트, 리소스, 다운로드 URL을 넣지 않는다.

| 분류 | 조사 결과 | 해석의 한계 |
|---|---|---|
| KTF BREW | ZIP 361개. 내부 `.mod` 359개, `.mif` 362개, `.sig` 360개, `.bar` 632개 | 엔트리 개수이며 고유 게임 수가 아니다. 모듈 CPU·ABI·완전성은 미검증 |
| LGT 다운타운 | ZIP 105개에 각각 JAR/JAD 존재. JAR 105개, 클래스 엔트리 1,097개 | 정상 ZIP 열기는 클래스 검증이나 부팅 성공을 의미하지 않는다 |
| 다운타운 메타데이터 | JAD 84개에서 CLDC-1.0 및 MIDP-1.0 선언 확인 | 나머지 21개는 이번 검색에서 미확인. 다른 플랫폼이라고 단정하지 않는다 |
| SKT GVM | ZIP 9개, SGS 9개. MOD 5개, INF 3개. 하나는 SGS만 포함 | 기존 paired-descriptor 인식 조건을 만족하지 못할 변형이 있다 |
| 제외 | ALZ 1개, 중첩 ZIP의 추가 재귀 분석, 실제 게임 실행 | ZIP 475개를 475개의 고유·완전한 게임으로 주장하지 않는다 |

다운타운 클래스에서 MIDlet, Canvas, Graphics, Image, Display, Font,
RecordStore 및 Connection 계열 참조를 확인했다. 이 조사는 ASCII 이름 패턴
검색이므로 정밀한 상수 풀 의존성 분석이 아니다. 참조 횟수는 API 커버리지가
아니며, 게임에 포함된 클래스와 플랫폼에서 제공해야 할 클래스를 아직 구분하지
않았다. 특정 `com/*` 이름을 발견했다는 이유만으로 OEM API라고 분류하지 않는다.

## 3. 기존 코드와 재사용 경계

아래 경로는 워크스페이스 루트 기준이다. 문서의 과거 설계 설명보다 현재 실행
코드를 우선해 판단했다.

| 기존 위치 | 재사용 대상 | 변경 또는 신규 구현 경계 |
|---|---|---|
| `aram-core/skvm/classfile.go`, `interpreter.go`, `value.go`, `gc.go`, `threads.go` | 다운타운 클래스 파싱·Java 명령 실행·객체·GC·스레드 | 클래스 버전, 예외, 스케줄링 차이는 synthetic 재현으로 보완 |
| `aram-core/skvm/natives_cldc_*.go`, `natives_midp*.go`, `graphics_midp.go` | 다운타운 CLDC/MIDP, 그리기·라이프사이클·RMS·미디어 | 선언된 API가 존재해도 해당 게임에 필요한 의미가 맞는지는 별도 검증 |
| `aram-core/application/internal/skvmhost/machine.go`, `state.go` | 다운타운 시작·일시정지·프레임·입력·음향·상태 연결 | 생성자가 `skloader.Package`를 받고 기본 carrier=`skt`, profile=`wipi-1.2.1/skt/generic` 사용. 일반 Java 패키지와 프로필을 받도록 분리 필요 |
| `aram-core/application/skvm_factory.go` | 일반 제품 경로의 Java 머신 선택 | 현재 KTF/Raptor 우선순위 보존 후 SKT 패키지만 탐지. 일반 JAD/JAR 탐지 추가 필요 |
| `aram-core/loader/skvm/package.go` | descriptor 처리·클래스/리소스 분리의 설계와 검증 패턴 | MSD/MOD/WMR/JAR 결합 조건을 다운타운에 적용할 수 없음. 별도 J2ME 로더 필요 |
| `aram-core/cpu/backend.go` 및 CPU 구현 | BREW ARM/Thumb 실행·메모리·레지스터·CPU 문맥 | 실제 MOD 명령 집합 확인이 선행. WIPI 로드 주소·시작 규약·힙 배치를 그대로 적용하지 않음 |
| `aram-core/loader/ktf/package.go`, `loader/raptor/` | ZIP 검증·이미지 로딩 테스트 패턴 참고 | KTF WIPI의 `__adf__`/client.bin과 BREW MOD/MIF는 서로 다른 형식. 기존 로더를 억지로 확장하지 않음 |
| `aram-core/loader/gnex/header.go`, `package.go` | GVM 패키지 인식·제목 헤더 처리 | SGS 본문·명령·리소스 테이블은 미구현. 단독 SGS 지원도 별도 검증 필요 |
| `aram-core/runtime/services.go` 및 `runtime/` | 셋 모두 그래픽 표면·텍스트·저장·가상 시계·이벤트·미디어·장치·재현 서비스 | 플랫폼별 인자, 오류, 핸들, 타이머 의미를 변환하는 어댑터는 신규/수정 |
| `aram-core/core/machine.go` | 셋 모두 머신 생명주기·입력·영상·음향 계약 | 새 실행기의 내부 상태와 기능 지원 여부는 각각 구현 |
| `aram-core/skvm/state.go`, `application/internal/skvmhost/state.go` | 다운타운 Java 상태 직렬화 기반 | LGT 설정/확장 객체 추가 시 상태 버전·설정 검증 검토. BREW/GVM 상태를 Java 포맷으로 저장하지 않음 |
| `aram-frontend/frontend/backend.go`, `aram-emu` 제품 연결 | 일반 열기·제어·화면·호스트 음향·입력 전달 | 형식 표시, 파일 필터, 프로필, 지원 기능 노출 검토. VM 명령 디버거는 별도 계약 검토 |
| `aram-test/run_suite.py` | discovery·probe 실행·결과 정규화·delta·triage | 플랫폼/변형 분류, 새 synthetic fixture와 milestone 기대값 추가 |

공통 그래픽 엔진이 있다고 BREW BAR나 GVM 내부 이미지가 자동으로 해독되는
것은 아니다. 디코더와 팔레트·픽셀 배치 변환은 입력에서 확인한 형식별로 추가한다.
공통 저장 서비스도 LGT `.db/.idx` 원본 레코드 포맷의 호환을 보장하지 않는다.

## 4. 플랫폼별 설계

### 4.1 LGT 다운타운: 기존 Java 실행기의 범용화

새 `loader/j2me`(제안명)가 JAR manifest와 선택적 JAD를 검증해 main class,
properties, classes, resources를 반환하도록 한다. SKT 원본 패키지는 기존 로더가
계속 담당하고 두 로더의 결과를 공통 Java application 입력으로 변환한다.
이 공통 입력은 코어 소유로 두고 SKT record-store 부가정보는 선택적으로 표현한다.

현재 `skvmhost`의 lifecycle/frame/audio/state 코드를 활용하되 SKT 기본값,
제목별 화면 보정, 통신사 API 설치 정책을 분리한다. `skvm` 패키지 전체 이름
변경은 초기 지원의 선행조건으로 삼지 않는다. LGT 앱에 SKT 전용 클래스가
자동으로 제공되어 누락 의존성이 숨겨지는지도 native registry에서 점검한다.

첫 입력 경로는 이미 존재하는 ZIP(JAD+JAR)과 독립 JAR이다. 독립 JAD는 동반
JAR 접근이 필요하므로 데스크톱 파일 경로에만 의존하지 않는 문서 접근 계약을
검토한 뒤 지원한다. descriptor의 URL을 이용한 자동 네트워크 다운로드는 하지
않는다. 단순히 모든 ZIP을 Java로 판정하지 않고 APK·KTF·Raptor·SKVM·GNEX와
충돌하는 synthetic 입력으로 탐지 우선순위를 검증한다.

그 다음 정밀 상수 풀 분석으로 게임에 포함되지 않은 외부 클래스와 정확한
메서드 descriptor를 추출한다. 기본 MIDP만 쓰는 후보를 먼저 부팅하고 LGT
확장 의존성은 별도로 묶는다. 해상도, 소프트키, 한글 폭, 타이머/스레드,
RMS 레코드 의미를 검증한다. 외부 `.db/.idx`가 해석되지 않으면 저장 데이터
호환 미지원으로 표시하고 기존 세이브를 조용히 버린 성공으로 처리하지 않는다.

### 4.2 BREW: 기존 CPU와 공통 서비스 위에 새 실행 환경

새 `loader/brew`와 BREW runtime/host를 코어에 추가하는 방향이다. 먼저 MOD의
이미지 형식, 코드/데이터/BSS, 진입점, relocation, 요구 CPU 및 MIF 연결을
확인한다. ARM/Thumb임이 확인된 모듈에 기존 `cpu.Backend`를 연결한다.
현재 WIPI 로더가 BREW의 로딩 계약을 충족한다고 가정하지 않는다.

BREW 앱 생성·객체 인터페이스·이벤트 전달을 구현한 뒤 화면·입력·타이머·파일·
음향을 `runtime.Services`로 연결한다. 구체적인 인터페이스와 ABI는 합법적으로
접근 가능한 명세 또는 허가된 참조 동작으로 확정한다. BAR 등 리소스 처리,
SDK/기종별 확장, 서명·보호 상태 식별은 독립된 작업으로 관리한다. 보호 상태를
해석할 수 없는 입력은 정확히 보고하며 범용 실행 지원의 전제에서 제외한다.

CPU 문맥·공통 서비스 snapshot 기반은 활용하지만 BREW 객체, 참조 수명,
콜백, pending event를 포함한 전용 상태 schema가 필요하다. 이 부분을 완료하기
전에는 save/load/rewind 지원을 표시하지 않는다.

### 4.3 GVM: 기존 인식 기능 위에 새 VM

현행 [GVM 조사 문서](../../aram-core/docs/gnex-format.md)는 제목 헤더와 일부
구조만 설명한다. 제품도 `application/machine_load.go`에서 실행 미지원 오류를
반환한다. SKVM(Java)이나 ARM 실행기로 SGS 본문을 실행할 수 없다.

우선 버전별 본문·명령 인코딩·값과 스택·제어 흐름·리소스 구조에 대한 근거를
확보한다. 이후 작은 synthetic 프로그램으로 산술·분기·호출·오류 동작을
고정하고 별도 pure-Go VM을 구현한다. 그래픽·입력·타이머·저장·음향 호출은
공통 서비스에 연결한다. 원본 SGS 형식의 불확실성이 남아 있다면 synthetic VM
성공과 실제 파일 호환성을 별개의 milestone으로 기록한다.

기존 SGS+MOD/INF 로더는 보존한다. 단독 SGS는 확장자가 아닌 검증 가능한
헤더·경계 조건으로 별도 인식한다. VM 스택·PC·변수·리소스 참조·이벤트를
상태 schema에 포함하며 Java heap 상태나 ARM 디버거를 그대로 사용하지 않는다.

## 5. 실행 순서와 완료 조건

| 단계 | 소유 저장소 | 작업 | 종료 증거 |
|---|---|---|---|
| P0 코퍼스 기준선 | aram-test, loader는 core | SHA-256 중복 제거, 플랫폼/변형 분류, 현행 probe 실행, Java 외부 의존성 감사 | 고유 입력별 현행 milestone·실패 분류. ALZ·보호·불완전 입력은 명시적 미검증/실패 분류 |
| P1 다운타운 로더 | core + test | synthetic JAD/JAR/ZIP, bounded parsing, 탐지 충돌 검증 | 올바른 main class/리소스 인식, malformed 입력 거부, 기존 패키지 분류 유지 |
| P2 다운타운 최소 실행 | core + emu | 공통 Java 입력과 host 설정 분리, 일반 제품 경로 연결 | synthetic MIDlet과 선택한 원본 한 해시가 첫 화면·입력 응답에 도달 |
| P3 다운타운 확장 | core + test + 필요시 frontend | 외부 API cluster, 한글·키·타이머·음향·RMS·상태 | 복수의 서로 다른 API cluster 검증, 결정적 replay, 정확한 미지원 목록 |
| P4 BREW 로더와 bootstrap | core + test | 실제 형식 확인, synthetic 모듈 로딩, CPU 및 앱 이벤트 연결 | 원본 모듈 진입 확인과 synthetic 앱 이벤트 검증. 화면 성공과 분리 |
| P5 BREW 서비스 | core + emu + test | UI/input/time/storage/media 어댑터, 리소스, 전용 상태 | 선택 해시에서 첫 화면·조작·저장, 추가 SDK/기종별 독립 검증 |
| P6 GVM 명세 확정 | core docs + test | 버전별 SGS 의미와 synthetic 명령 fixture | 명령·리소스 해석 근거와 반복 가능한 expected 결과 |
| P7 GVM 실행 | core + emu + test | VM, 서비스 연결, 상태, 일반 열기 경로 | synthetic 의미 검증 후 원본 해시별 첫 화면·입력 증거 |

P0를 실행한 뒤 실제로 MIDP 의존성이 작은 다운타운 후보를 선택한다. 알려지지
않은 API를 무조건 성공 반환하는 방식으로 P2를 통과시키지 않는다. BREW와
GVM의 형식 조사는 다운타운 완료 전에도 가능하지만 대규모 구현 착수 판단은
각각 P4 로딩 계약과 P6 명세의 근거 확보에 둔다. 현시점에서 달력 기간이나
전체 타이틀 playable 달성 일정을 확약하지 않는다.

## 6. 검증과 출시 경계

각 behavioral change는 워크스페이스 AGENTS.md의 루프를 따른다: 네 저장소
상태 기록 → 최소 synthetic/black-box 재현 → owning test → 수정 → focused test
→ core → test-runner unit → frontend → integration/product → black-box suite.
마지막에 `report.md`, `delta.md`, `triage.md`를 확인하며 regression이 있으면
완료하지 않는다. 정상 입력 기대값을 약화하거나 예상치 못한 skip을 통과로
처리하지 않는다.

합성 게이트는 `aram-test all --force --jobs 4 --slices 4096`; 허가된 코퍼스는
프로세스 로컬 `ARAM_TEST_DATA`로 원위치 연결하고 `all --force --jobs 4
--slices 32768 --timeout 20`으로 검증한다. 로컬 참조가 있으면
`ARAM_REFERENCE_REPO`도 연결한다. 명령 실행은 `python run_suite.py`를 사용한다.
보고서에는 해시·프로필·개인정보를 제거한 상대 식별자·정규화된 결과만 남긴다.

영향받는 Windows 제품 빌드와 일반 File/Open smoke를 수행하고 테스트 프로세스를
종료한다. core Android/arm64 pure-Go 빌드와 모바일 frontend의 공식
`ebitenmobile bind` 게이트를 수행한다. SDK/NDK/binder 부재는 미검증으로 보고한다.

milestone은 `recognized`, `loads`, entry execution, `ok_frame`, `boots`,
`playable`, `complete`를 구분한다. 화면 제출만으로 정상 게임 화면이라고 판단하지
않고 예상 영상/상태와 입력 반응을 확인한다. 한 해시·프로필의 성공을 플랫폼
전체 지원으로 확대하지 않는다. 새 플랫폼의 format/profile/state identity는
WIPI/SKT로 위장하지 않고 버전이 있는 독립 식별자로 설계한다.

## 7. 이번 문서 작업에서 한 일과 남은 일

완료: ZIP 구조와 다운타운 Java 메타데이터 정적 조사, 실제 코드의 Java host/
factory/shared services/state/CPU/loader 경계 확인, 저장소별 구현 계획 작성.
이번 변경은 문서만 추가하며 실행기·로더·게임 입력을 변경하지 않는다.

미실행: P0 현행 전체 코퍼스 probe, 정확한 Java 메서드 의존성/지원률 계산,
BREW 모듈 ABI 분석, GVM 명령 의미 검증, 실제 게임 부팅·플레이 검사.
따라서 위 재사용 비율을 실측 호환률이나 완료된 구현량으로 인용하지 않는다.

다음 실행 단위는 **P0 기준선 + P1 다운타운 로더**다. 그 결과로 다운타운의
최소 실행 후보와 누락 API를 확정하고 재사용 추정 범위를 갱신한다.
