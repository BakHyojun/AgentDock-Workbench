# 내장 인터페이스 끄기 검증

## 판단과 사용자 확인

현재 소스에서 체크박스 OFF가 외부 MCP 실행을 실패로 바꾸는 결함은 재현되지 않았다. 서버 설정 저장과 도구 실행은 정상이며, **이미 연결된 ChatGPT 화면까지 즉시 깔끔하게 전환된다는 보장은 없다.** 사용자가 관찰한 “다이나믹 MCP 호출 실패”의 직접 원인은 아직 확정할 수 없다.

먼저 AgentDock의 내장 인터페이스 OFF 저장 후 ChatGPT의 해당 연결 상세에서 **Refresh**로 메타데이터를 갱신하고, 새 대화에 연결을 추가하여 같은 읽기 호출을 비교한다. 단순 웹페이지 새로고침은 MCP 메타데이터 Refresh와 다르다. Refresh 기능이 없다면 연결을 재설정하여 도구 목록을 다시 읽게 한다. AgentDock Core 재시작은 필요하지 않다. 기존 대화의 과거 카드를 제거하는 기능은 아니다.

공식 [Connect and test your plugin](https://developers.openai.com/plugins/deploy/connect-chatgpt)의 Refresh metadata 절은 UI 리소스 등 변경 후 연결 Refresh, 변경된 메타데이터 확인, 새 대화에서 재검사를 안내한다. AgentDock은 설정을 hot reload하므로 문서의 서버 배포/재시작 단계 대신 이미 동작 중인 서버 정책을 갱신한다. 개발 모드 MCP 연결에 관한 안내이며 게시된 플러그인의 갱신 방식과 구분한다.

## 소스 경로

| 단계 | 기존 소유자와 동작 |
|---|---|
| 체크박스 | `MainWindow.xaml`의 `McpUiEnabledChoice` → `MainWindow.Display.cs`의 `McpUiPreference_Click`. 저장 결과를 반영하며 실패 시 설정을 재조회한다 |
| local API | `DisplayPreferenceService.SaveAsync` → 인증된 `/internal/runtime/execution/display` POST → `RuntimeUpdateDisplaySettings` |
| 저장 | `internal/config/display.go`의 `DisplayPreferences.Update`. revision 검사, 원자적 파일 저장 후 snapshot과 listener 갱신. 실패 시 기존 정책 유지 |
| 서버 갱신 | `Server.refreshPresentationState`가 `TextOnly` generation 게시, UI 리소스 목록 제거, 기존 도구 재등록. 실행 설정이나 외부 MCP manager 재시작 없음 |
| 도구 목록 | `toolMetadata`와 `presentationMiddleware`가 UI mount 제거. 이름·schema·권한·업무 도구 유지 |
| 결과 | `finishResponse`와 middleware가 최종 응답의 UI 메타데이터 제거. `mcp_tool_call`의 nested MCP envelope에도 적용. 본문·업무 데이터·실제 오류는 유지 |
| cached URI | `legacyTemplate`은 정확히 알려진 이전 built-in URI에만 30분 동안 `disabledTemplateHTML` 제공. 목록에는 없음. 이후 resource not found |

HTTP MCP 서버는 `Stateless=true`, `JSONResponse=true`다. SDK의 목록 변경 통지는 기존 ChatGPT 캐시가 실제 갱신되었다는 증거가 아니다. local display 응답도 `server_policy_applied=true`, `host_adoption=unknown`으로 구분한다.

## 구현 한계와 미확인 경계

- **확인된 한계:** 이전 UI 주소의 30분 호환 기간이 끝나면 조회가 실패한다. 최신 목록에는 UI 주소가 없지만 오래된 목록을 계속 쓰는 host는 이 경로에 도달할 수 있다. `TestCachedTemplateGraceIsExactInertAndExpires`가 이를 검증한다.
- **확인된 구현 / 미확인 영향:** 호환 HTML은 중국어 안내문과 script/connect 차단 CSP를 포함하고, `ui/initialize` 및 initialized notification을 수행하지 않는다. 리소스 읽기 성공과 host의 App 표시 성공은 다르다. 실제 ChatGPT가 이 페이지를 초기화 오류로 처리하는지는 확인하지 못했다. 관찰된 증상의 직접 원인이나 수정 필요성을 이 사실만으로 확정하지 않는다.
- **확인된 경계:** 해당 한국어 실패 문구는 저장소 및 pinned `agentdock-protocol` renderer에서 발견되지 않았다. renderer의 초기화 실패 문구는 “MCP App을 초기화할 수 없습니다.”이다. ChatGPT의 외부 상태 표시를 AgentDock의 `isError`나 Activity RPC 상태와 동일시하지 않는다.
- **입력 관찰 / 미검증:** 실제 사용한 바이너리 버전, 설정 저장 응답, 실패한 UI resource URI, 응답 코드 및 실패 시점은 제공되지 않았다. 이 머신에서 사용자의 웹 세션과 당시 로그를 재현했다고 주장하지 않는다. 이번 세션에는 `agentdock_context` callable tool도 없어서 Git/로컬 규칙/소스를 직접 읽고 임시 fixture로 검증했다.

Refresh 후 새 대화에서도 문제가 반복되면 해당 호출의 `rpc_status`, 실제 MCP 결과의 `isError`/`code`, UI resource read 응답과 초기화 오류를 같은 호출·시점으로 비교해야 한다. 도구 실행 실패면 external MCP/권한/transport 경로를 조사하고, 실행 성공이면 host의 resource/cache/표시 경로를 조사한다. 정상 TextOnly 목록에도 UI 주소가 남아 있다면 서버 또는 중간 bridge의 metadata 결함으로 재현 범위를 좁힐 수 있다.

## 이번 변경과 검증

실행 코드와 설정은 변경하지 않았다. `internal/mcp/display_dynamic_test.go`에 실제 stateless HTTP 서버와 disposable 외부 MCP 서버를 연결하는 회귀 시험을 추가했다. 원래 도구 목록을 유지한 client로 다음을 검증한다.

1. UI ON 상태에서 실제 외부 도구 호출 성공.
2. 외부 호출이 진행 중일 때 OFF 저장 후 결과 정상 전달, UI mount 제거.
3. 목록 Refresh 없이 후속 성공 호출 정상 반환; 실제 upstream `isError=true`는 오류로 유지.
4. 본문, structured business data, non-UI metadata 유지. OFF가 외부 MCP의 재연결/재탐색을 일으키지 않음.
5. cached dynamic UI 주소는 호환 안내 HTML로 읽히고, 명시적 목록 Refresh 후 UI mount가 없어짐.
6. OFF 상태의 `agentdock_context` 성공.

- `go test ./internal/mcp ./internal/config -count=1`: config 통과. MCP의 기존 시험들은 통과했지만 새 fixture의 `mcp_manage add`에서 필수 description 누락으로 1건 실패했다. 제품 결함이 아닌 시험 준비 오류이며 실패 로그 `%TEMP%/agentdock-ui-toggle-go-final.log`를 보존했다.
- fixture에 description을 추가한 뒤 `go test ./internal/mcp -run TestDisplayToggleDynamicMCPWithoutHostRefresh -count=1 -v` 통과. 최종 로그 `%TEMP%/agentdock-ui-toggle-dynamic-final.log`. 불필요한 전체 검사 반복은 하지 않았다.
- `go vet ./internal/mcp ./internal/config`, gofmt, `git diff --check` 통과.
- Windows desktop pure-policy 검사 923개 단언 통과. `%TEMP%/agentdock-ui-toggle-desktop-final.log`. 실제 체크박스 클릭, WPF runner, ChatGPT iframe 초기화는 검증하지 않았다.

운영 Core/설정/연결 및 원본 로그는 변경하지 않았고, installer 빌드·게시·실제 설치도 실행하지 않았다. 변경은 시험/문서만이므로 기존 `1.1.8102` 후보의 source identity와 bytes는 그대로다. GitHub Actions 차단도 유지한다.
