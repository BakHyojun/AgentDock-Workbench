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

구현 완료 후 최종 Go·Windows desktop·다국어 모델 검증을 수행했다. 실패는 보존하고 변경과 무관한 기존 실패를 숨기지 않는다. clean clone에서 Windows x64 후보 Setup을 빌드하고 실제 내부 payload/한국어 assembly/rg/CUA와 source identity·SHA256을 확인했다. 운영 PC 설치·업그레이드·복구·Core 재시작, runner guard 우회 및 CI 재활성화는 수행하지 않았다. 후속 사용자 요청으로 같은 설치파일을 GitHub에 공개했으며 빌드·수용·공개를 아래에서 구분한다.

- Go 전체 최종 suite: 변경한 Activity/app/HTTP, installer/shim, MCP 및 scripts 통과. 이전 기준선에서 확인한 실패 2건은 유지했다. `TestManagedTaskFirstPage1000` 2초 목표 대비 4,934.228 ms, `TestSearchTextRGExitCodes`의 예상 `SEARCH_FAILED` 대비 사전 검증 `INVALID_REGEX`. `%TEMP%/agentdock-8200-go-final.log` 보존. 실패를 삭제하거나 단언을 완화하지 않았다.
- Go formatting·vet·`verify-version v1.1.8200` 통과. Release desktop build 경고/오류 0, pure policy 923 assertions, 다국어 모델·한국어 7,307 assertions 통과. 최종 검토에서 null descriptor code의 원문 fallback을 추가한 뒤 해당 한국어/모델 시험만 한 번 다시 실행하여 **7,310 assertions** 통과. 전체 suite를 반복하지 않았다.
- 로그: `%TEMP%/agentdock-8200-desktop-build.log`, `agentdock-8200-desktop-policy.log`, `agentdock-8200-localization-final.log`, `agentdock-8200-localization-final-review.log`, `agentdock-8200-vet-final.log`. Windows desktop 빌드는 순차 실행하여 공유 obj 충돌을 피했다.
- 회귀는 explicit disposable Activity/HTTP fixture와 UI 없는 실제 모델을 사용했다. old list/detail/update query의 번역 정보, journal 원문·sequence 불변, 실제 reserved events에 metadata 저장, 외부/사용자/unknown/future metadata 보호와 skipped·failed/stop 구분을 검증했다. 승인 template 경로·세션 ID 보존과 unknown 전체 template 원문 fallback을 en/zh-CN/ko-KR에서 확인했다.
- 실제 isolated runner의 WPF 화면/Setup 설치·업그레이드·rollback, Linux/race 수용은 미실행이다. 로컬 후보 생성과 전체 installer 수용은 별도 상태다.

## 설치파일 identity와 payload 확인

- 파일: `D:/Engineering/release/agentdock-1.1.8200-0383a90d/release/AgentDockSetup-amd64.exe` (Windows x64, 112,690,821 bytes).
- SHA-256: `4f8224875f68d858dd719c95657a0122c313ccf59f32d03ba13708bd3ce3621d`. 같은 폴더의 `.sha256`, `build-report.json`, `verification-scope.json`에 identity와 수행/미수행 범위를 기록했다.
- 빌드 source: `0383a90d252ec8e924a166254c7db1e09437bf90`, 깨끗한 새 일반 clone `D:/Engineering/release/agentdock-1.1.8200-0383a90d-source`. 사용자 지정 버전 `1.1.8200`, 채널 `candidate-not-released`. 이 전달 문서 커밋은 실행파일의 source identity를 변경하지 않는다.
- AgentDock/Setup은 unsigned이며 번들 cloudflared의 Authenticode는 Valid다. Inno 공식 Korean.isl의 obsolete font 경고 4개는 유지됐다. 제어판 빌드 경고/오류는 0이다.
- Setup을 실행하지 않고 innounp로 추출했다. 내장 ZIP SHA-256 `6607b6aa925b68928ab3baac58c8ca99dbef86554d6b00bf6c3737eb1d711300`이 외부 payload와 동일하다. 실제 내장 tray의 desktop assembly·ko-KR satellite bytes가 clean-clone 빌드와 일치하며, 그 satellite를 ResourceReader로 읽어 1,171키와 새 29키의 한글을 확인했다. 스크린샷 문구는 `귀속되지 않은 기록`, `식별되지 않은 대화 · 독립 호출`, `실행 권한 변경`이다.
- 내장 rg 15.2.0 파일 5개와 CUA 플러그인 파일 4개, shim bytes와 cloudflared bytes, Setup 한국어 선택을 확인했다. 패키지 검증기는 실제 packaged Core 버전/commit과 desktop source identity, PE x64, checksum을 확인하고 임시 Skill/plugin home에서 fresh/repeat bootstrap을 검증했다. 운영 runtime/설정은 사용하지 않았다.
- payload 증거: `D:/Engineering/release/agentdock-1.1.8200-0383a90d/payload-evidence.json`. 로그: `%TEMP%/agentdock-8200-release-build.log`, `agentdock-8200-assets-final.log`, `agentdock-8200-unpack.log`, `agentdock-8200-assets-metadata-final.log`.
- 저장소 Actions `enabled=false`를 유지한다. 이전 후보 설치파일 bytes는 변경하지 않았다. 아래 공개 완료도 실제 isolated Windows 설치 수용을 의미하지 않는다.

## GitHub 공개와 원격 main 반영

- 후속 사용자 요청에 따라 [v1.1.8200](https://github.com/eerraa/AgentDock-Workbench/releases/tag/v1.1.8200)을 공개했다. Draft=false, Pre-release=true, Latest=false. 실제 설치 수용 미실행 상태를 이유로 공개 사전 릴리즈를 선택했고 정식 latest 1.1.8100은 유지했다.
- [Windows x64 Setup 직접 다운로드](https://github.com/eerraa/AgentDock-Workbench/releases/download/v1.1.8200/AgentDockSetup-amd64.exe). README.md/README.zh-CN.md의 fork 다운로드·버전 안내도 이 릴리즈에 연결했다. upstream 라이선스와 attribution은 유지한다.
- annotated 태그 `v1.1.8200`은 실제 빌드 source `0383a90d252ec8e924a166254c7db1e09437bf90`을 가리킨다. 태그 메시지에 `[skip ci]`를 넣었으며 origin에만 push했다. main에는 source와 후속 문서를 fast-forward로 반영한다. 공개 태그나 이전 설치파일을 이동/덮어쓰지 않았다.
- 자산은 Setup, build-report.json, verification-scope.json과 각 `.sha256`, 정확히 6개다. Draft와 공개 후 각각 별도 폴더로 실제 다운로드하여 6개 모두의 크기·SHA-256·GitHub API digest가 로컬 원본과 동일함을 확인했다. 공개 Setup URL의 인증 없는 HEAD는 200, Content-Length는 112,690,821이었다. 공개 release 본문과 저장소 release notes도 일치한다.
- 설치파일 SHA-256과 bytes는 위 빌드 결과 그대로다. 동봉한 metadata는 제작 당시 `candidate-not-released`/게시 `not_run` 스냅샷을 보존한다. 공개 사실을 만들기 위해 metadata나 실행파일을 다시 쓰거나 빌드하지 않았다.
- 게시 증거는 `D:/Engineering/release/agentdock-1.1.8200-0383a90d/draft-publication-evidence.json`, `public-publication-evidence.json`. 다운로드는 같은 경로 아래 `github-draft-redownload`, `github-public-redownload`다. source/회귀 검사를 반복하지 않고 게시 자산만 검사했다.
- 사용자가 다음 릴리즈를 등록하는 절차는 [Windows 릴리즈 등록 안내](windows-release.ko.md)에 있다. upstream CI 문서의 자동 빌드/덮어쓰기 절차를 이 fork에 적용하지 않는다.
