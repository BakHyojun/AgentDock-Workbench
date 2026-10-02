# 1.1.8200 한국어 표시 보완

사용자 화면의 `未归属记录`, `未识别对话 · 独立调用`, `permission.update · 修改执行权限`는 한국어 리소스가 없어서가 아니라 표시·예약 저장 경로에 연결되지 않아서 남았다. 리소스 포함 검증만으로 화면 경로 수용을 대신할 수 없었다.

## 구현 경계

- 그룹/대화 모델: 미귀속·프로젝트 미연결 synthetic ID와 빈 대화 ID의 명시적 marker를 사용한다. 서버는 이전 작업공간의 `title_source=fallback`을 추가하여 같은 이름의 사용자 작업공간을 번역하지 않는다. 원문 이름·identity·선택과 시간 경계는 유지한다.
- `activity.DescribeManagement`가 기존 permission 인식과 대화/작업 일괄 관리·연결·기본 작업·중지 문구를 담당한다. 기존 LocalizedText의 hash/args/status 보호를 유지하고 일반·예약 쓰기 모두 이를 사용한다. unknown 오류와 외부 MCP·사용자 라벨을 건드리지 않는다.
- 이전 기록 호환은 `cloneCall`의 조회 사본에서만 수행한다. descriptor가 없는 명시적 internal management tool, 빈 label/source, 정확한 제품 문구만 대상이다. list/detail/stream/export가 같은 owner를 사용하며 journal·캐시 projection·미래 descriptor를 바꾸지 않는다. 마이그레이션·삭제·Core restart가 없다.
- 일괄 관리 결과의 `message_text`를 UI에서 소비한다. skipped/failed와 원문 message는 유지한다.
- 추가 점검에서 승인 확인창의 제품 실행 사유/범위 원문 노출도 확인했다. 사용자 rule_id가 있는 설명과 독립 심사기 답변은 그대로 둔다. scope의 전체 기존 template이 확인될 때만 표시에서 번역하고 경로·대상·고정 세션 목록을 그대로 넣는다. 불완전/unknown template은 원문이다. 승인 선택·권한·정책 판정은 바꾸지 않는다.
- 리소스 29키 추가: en/zh-CN/ko-KR 각 1,171키. 모든 기존 값과 중국어 원문을 보존한다. static desktop C#/XAML의 한자 검색 결과는 주석과 기존 native permission/task recovery 기술 진단뿐이었다. 승인 원본 JSON, AI/외부 tool 응답, 로그, 사용자 제목·파일명·경로의 중국어는 데이터이므로 그대로 유지한다. 모든 출력 텍스트를 번역한 패치가 아니다.
- 사용자 지정 버전 `1.1.8200`을 적용했다. 소스 baseline은 Workbench 1.1.8 `4bd778d4077bbe58cfe19e4abb777f660694377b`이며 1.1.82 upstream을 도입한 것이 아니다. 이전 installer trial 수정과 Activity preview/reset을 유지한다.

## 검증·전달

구현 완료 후 최종 Go·Windows desktop·다국어 모델 검증을 수행한다. 실패는 보존하고 변경과 무관한 기존 실패를 숨기지 않는다. clean clone에서 Windows x64 후보 Setup을 빌드하고 실제 내부 payload/한국어 assembly/rg/CUA와 source identity·SHA256을 확인한다. 운영 PC 설치·업그레이드·복구·Core 재시작, runner guard 우회, CI 재활성화 및 GitHub Release 게시를 수행하지 않는다. 완료한 검증과 산출물은 아래에 기록한다.

- Go 전체 최종 suite: 변경한 Activity/app/HTTP, installer/shim, MCP 및 scripts 통과. 이전 기준선에서 확인한 실패 2건은 유지했다. `TestManagedTaskFirstPage1000` 2초 목표 대비 4,934.228 ms, `TestSearchTextRGExitCodes`의 예상 `SEARCH_FAILED` 대비 사전 검증 `INVALID_REGEX`. `%TEMP%/agentdock-8200-go-final.log` 보존. 실패를 삭제하거나 단언을 완화하지 않았다.
- Go formatting·vet·`verify-version v1.1.8200` 통과. Release desktop build 경고/오류 0, pure policy 923 assertions, 다국어 모델·한국어 7,307 assertions 통과. 최종 검토에서 null descriptor code의 원문 fallback을 추가한 뒤 해당 한국어/모델 시험만 한 번 다시 실행하여 **7,310 assertions** 통과. 전체 suite를 반복하지 않았다.
- 로그: `%TEMP%/agentdock-8200-desktop-build.log`, `agentdock-8200-desktop-policy.log`, `agentdock-8200-localization-final.log`, `agentdock-8200-localization-final-review.log`, `agentdock-8200-vet-final.log`. Windows desktop 빌드는 순차 실행하여 공유 obj 충돌을 피했다.
- 회귀는 explicit disposable Activity/HTTP fixture와 UI 없는 실제 모델을 사용했다. old list/detail/update query의 번역 정보, journal 원문·sequence 불변, 실제 reserved events에 metadata 저장, 외부/사용자/unknown/future metadata 보호와 skipped·failed/stop 구분을 검증했다. 승인 template 경로·세션 ID 보존과 unknown 전체 template 원문 fallback을 en/zh-CN/ko-KR에서 확인했다.
- 실제 isolated runner의 WPF 화면/Setup 설치·업그레이드·rollback, Linux/race 수용은 미실행이다. 로컬 후보 생성과 전체 installer 수용은 별도 상태다.
