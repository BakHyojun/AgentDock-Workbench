using System.Globalization;
using System.Resources;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using AgentDock.ControlPanel;

internal static partial class Program
{
    private static void TestKoreanPresentation()
    {
        var count=0;
        void Check(bool yes,string message) {count++;if(!yes)throw new InvalidOperationException(message);}
        JsonElement Json(object value) => JsonSerializer.SerializeToElement(value);
        string Hash(string raw) => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(raw))).ToLowerInvariant();
        var resources=new ResourceManager("AgentDock.ControlPanel.Resources.UiStrings",typeof(ExecutionCallRow).Assembly);
        var english=resources.GetResourceSet(CultureInfo.GetCultureInfo("en"),true,true)!;
        var keys=english.Cast<System.Collections.DictionaryEntry>().ToDictionary(pair=>(string)pair.Key,pair=>(string)pair.Value!);
        Check(keys.Count>=1000,"resource inventory unexpectedly small");
        var slots=new Regex(@"\{\d+(?:,[^}:]+)?(?::[^}]+)?\}");
        foreach(var locale in new[]{"en","zh-CN","ko-KR"})
        {
            UiText.ApplyPreference(locale);
            var localized=resources.GetResourceSet(CultureInfo.GetCultureInfo(locale),true,false);
            Check(localized is not null,"missing real satellite assembly: "+locale);
            foreach(var item in keys)
            {
                var text=UiText.Get(item.Key);
                Check(localized!.GetString(item.Key) is not null && text == localized.GetString(item.Key),"unresolved key "+locale+"/"+item.Key);
                Check(slots.Matches(item.Value).Select(m=>m.Value).Order().SequenceEqual(slots.Matches(text).Select(m=>m.Value).Order()),"placeholder mismatch "+locale+"/"+item.Key);
            }
            var original="读取文件 · 사용자文件.txt";
            var descriptor=new OwnedTextDescriptor{SchemaVersion=1,Code="tool.read_file",Args=[" · 사용자文件.txt"],TextHash=Hash(original)};
            var value=Json(new {tool_name="read_file",activity_label="Read a UTF-8 text file slice",activity_label_source="tool",display_title=original,summary=original,title_text=descriptor,summary_text=descriptor,status="succeeded",updated_seq=1});
            var row=new ExecutionCallRow(value);
            Check(row.Title=="read_file · "+UiText.Format("OwnedTool_read_file"," · 사용자文件.txt"),"generated label not translated");
            Check(row.Summary==UiText.Format("OwnedTool_read_file"," · 사용자文件.txt"),"summary not translated");
            Check(new ExecutionCallRow(Json(new{tool_name="read_file",activity_label="사용자 지정 설명",activity_label_source="",display_title=original,title_text=descriptor,status="succeeded"})).Title=="read_file · 사용자 지정 설명","a generated title must not replace the user's explicit label");
            Check(row.Technical.Contains("text_hash") && value.Text("display_title")==original,"stored original was changed");
            var decoded=JsonSerializer.Deserialize<ActivityEvent>(JsonSerializer.Serialize(new{title=original,title_text=descriptor,activity_label_source="tool",tool_name="read_file",kind="call.completed",status="succeeded"}),ActivityClient.JsonOptions)!;
            var activity=new ActivityRow(decoded);Check(activity.Title==row.Summary,"activity and execution use different descriptors");
            foreach(var bad in new[]{new OwnedTextDescriptor{SchemaVersion=2,Code=descriptor.Code,Args=descriptor.Args,TextHash=descriptor.TextHash},new(){SchemaVersion=1,Code="tool.unknown",Args=[],TextHash=descriptor.TextHash},new(){SchemaVersion=1,Code=descriptor.Code,Args=null,TextHash=descriptor.TextHash},new(){SchemaVersion=1,Code=descriptor.Code,Args=["x","y"],TextHash=descriptor.TextHash},new(){SchemaVersion=1,Code=descriptor.Code,Args=["x"],TextHash="stale"}})
                Check(OwnedText.Render(bad,original,"read_file","tool","succeeded")==original,"malformed/future descriptor rewrote original");
            Check(OwnedText.Render(descriptor,original,"read_file","user","succeeded")==original,"user label translated by coincidence");
            Check(OwnedText.Render(descriptor,original,"third:read_file","tool","succeeded")==original,"third-party label translated");
            Check(ExecutionObject.From(Json(new{conversation_id="conv-user",title="新对话",title_source="user"}),"conversation").Title=="新对话","user title changed");
            var unattributed = Json(new{conversation_id="",is_unattributed=true,title="未识别对话 · 独立调用"});
            Check(ExecutionObject.From(unattributed,"conversation").Title==UiText.Get("ExecutionUnidentifiedConversation"),"unattributed navigation title is raw server text");
            Check(ExecutionObject.From(Json(new{conversation_id="conv-user",title="未识别对话 · 独立调用",title_source="user"}),"conversation").Title=="未识别对话 · 独立调用","coincidental user title translated");
            var group=new WorkspaceGroupKey("unattributed","未归属记录");
            Check(group.Title==UiText.Get("ExecutionUnattributedGroup"),"initial synthetic group title untranslated");
            group.Apply(Json(new{title="未归属记录",total=1}));
            Check(group.Title==UiText.Get("ExecutionUnattributedGroup"),"refresh replaced group translation");
            group=new WorkspaceGroupKey("unassigned","未关联项目");
            Check(group.Title==UiText.Get("ExecutionUnassignedProject"),"unassigned project title untranslated");
            group=new WorkspaceGroupKey("wsp_old","历史工作区");
            group.Apply(Json(new{title="历史工作区",title_source="fallback"}));
            Check(group.Title==UiText.Get("ExecutionHistoricalWorkspace"),"historical fallback untranslated");
            group.Apply(Json(new{title="历史工作区",title_source="workspace"}));
            Check(group.Title=="历史工作区","user workspace name translated");
            foreach(var action in new[]{"rename","pin","unpin","tags","archive","unarchive","trash","restore","delete"})
            {
                var raw=action+" · conv_fixture";
                var managed=new OwnedTextDescriptor{SchemaVersion=1,Code="management.action."+action,Args=["conv_fixture"],TextHash=Hash(raw)};
                Check(OwnedText.Render(managed,raw,"conversation."+action,"","succeeded")==UiText.Format("OwnedManagementAction_"+action,"conv_fixture"),"management action untranslated: "+action);
                Check(OwnedText.Render(managed,raw,"third:conversation."+action,"","succeeded")==raw,"external action acquired trusted presentation");
                Check(OwnedText.Render(managed,raw,"conversation."+action,"user","succeeded")==raw,"user action label translated");
            }
            var busyRaw="仍有关联的运行项或待审批请求，请先停止或处理审批。项目文件未改动。";
            var busy=new OwnedTextDescriptor{SchemaVersion=1,Code="management.busy",Args=[],TextHash=Hash(busyRaw)};
            Check(OwnedText.Render(busy,busyRaw,"conversation.trash","","skipped")==UiText.Get("OwnedManagement_busy"),"skipped batch outcome untranslated");
            Check(OwnedText.Render(busy,busyRaw,"conversation.trash","","succeeded")==busyRaw,"skipped outcome became success");
            var stopRaw="已确认命令进程退出。";
            var stop=new OwnedTextDescriptor{SchemaVersion=1,Code="management.process_exited",Args=[],TextHash=Hash(stopRaw)};
            Check(OwnedText.Render(stop,stopRaw,"call.stop","","succeeded")==UiText.Get("OwnedManagement_process_exited"),"stop result untranslated");
            Check(OwnedText.Render(stop,stopRaw,"call.stop","","failed")==stopRaw,"failed stop presented as confirmed exit");
            Check(OwnedText.Render(Json(new{schema_version=1,code=(string?)null,args=Array.Empty<string>(),text_hash=Hash(stopRaw)}),stopRaw,"call.stop","","succeeded")==stopRaw,"null descriptor code crashed or rewrote original");
            foreach(var pair in new[]{("exec_command","OwnedApprovalReasonCommand"),("session_act","OwnedApprovalReasonSession"),("file_edit","OwnedApprovalReasonFile"),("mcp_tool_call","OwnedApprovalReasonDefault")})
            {
                var raw=UiText.Original(pair.Item2);
                Check(OwnedText.ApprovalReason(pair.Item1,"",raw)==UiText.Get(pair.Item2),"approval reason untranslated");
                Check(OwnedText.ApprovalReason(pair.Item1,"user_rule",raw)==raw,"user rule explanation translated");
                Check(OwnedText.ApprovalReason(pair.Item1,"",raw+" 原文")==raw+" 原文","unknown approval reason translated");
            }
            var path="D:/原文/한국어.txt";
            var scope="工作区："+path+"\n当前进程操作系统账户权限；此模式不提升权限，也不限制任意命令内部的文件访问。\n本次停止的固定会话集合：[session_fixture]\n目标："+path+"（file）\n第三方 MCP 的内部副作用由该服务实现，未将其注释当作可信只读授权。";
            var scopeText=OwnedText.ApprovalScope(scope);
            Check(scopeText.Contains(path)&&scopeText.Contains("[session_fixture]")&&scopeText.Contains(UiText.Get("OwnedApprovalScopeAccount")),"approval scope changed selectors or failed to translate");
            Check(scopeText==string.Join("\n",UiText.Format("OwnedApprovalScopeWorkspace",path),UiText.Get("OwnedApprovalScopeAccount"),UiText.Format("OwnedApprovalScopeSessions","[session_fixture]"),UiText.Format("OwnedApprovalScopeTarget",path+"（file）"),UiText.Get("OwnedApprovalScopeExternal")),"scope template was not translated completely");
            Check(OwnedText.ApprovalScope(scope+"\n用户未知说明")==scope+"\n用户未知说明","partial template changed unknown data");
            Check(OwnedText.ApprovalScope("用户 原文")=="用户 原文","arbitrary scope changed");
            if(locale=="ko-KR") foreach(var item in keys.Where(item=>item.Key.StartsWith("OwnedManagement")||item.Key.StartsWith("OwnedApproval")))
                Check(Regex.IsMatch(UiText.Get(item.Key),"[가-힣]"),"missing Korean owned message: "+item.Key);
            var command=new ActivityEvent{Kind="task.completed",Title="task.user-title",Status="succeeded"};
            Check(ActivityPresentation.EventHeading(command,command.Title).Contains(command.Title),"user title resembling event code hidden");
            var failed="失败：원문 실제 오류";
            row.Apply(Json(new{tool_name="read_file",display_title=original,summary=failed,status="failed",updated_seq=2}));
            Check(row.Summary==failed,"new failure masked by stale success text");
            var titleRaw="修改执行权限";
            Check(OwnedText.Render(new OwnedTextDescriptor{SchemaVersion=1,Code="permission.update",TextHash=Hash(titleRaw)},titleRaw,"permission.update","","succeeded")==UiText.Get("OwnedPermissionUpdate"),"zero-argument permission descriptor failed");
            var permission="scope=workspace scope_id=wsp_fixture mode=rules revision=2；操作系统权限未改变。";
            var permissionText=new OwnedTextDescriptor{SchemaVersion=1,Code="permission.updated",Args=["workspace","wsp_fixture","rules","2"],TextHash=Hash(permission)};
            Check(OwnedText.Render(permissionText,permission,"permission.update","","succeeded")==UiText.Format("OwnedPermissionUpdated","workspace","wsp_fixture","rules","2"),"permission summary not localized");
            Check(OwnedText.Render(permissionText,permission,"permission.update","","failed")==permission,"permission failure changed to success");
            var request="原始cmd -x / 사용자.txt"; var bytes=Encoding.UTF8.GetByteCount(request);
            var payload=new ExecutionPayloadView("response");payload.Describe(Json(new{state="partial",@ref="blob",bytes,lines=2}));
            Check(payload.StateLabel==UiText.Get("ExecutionOutputPartial"),"payload state is not localized");
            Check(payload.ApplyPage(Json(new{payload=new{@ref="blob"},offset=0,next_offset=bytes,has_more=false,text=request}),"blob",false) && payload.Text==request,"payload content modified");
            var edit=new ExecutionCallRow(Json(new{tool_name="file_edit",file_edit=new{stats_state="known",insertions=26,deletions=9,path="保留/한글.txt",diff_preview="- 原文\n+ 사용자",changed=true}}));
            Check(edit.AddedLinesText=="+26"&&edit.DeletedLinesText=="−9"&&edit.FileEditDetails.Contains("- 原文\n+ 사용자"),"file statistics or original diff changed");
            var truncated=new ExecutionCallRow(Json(new{tool_name="file_edit",file_edit=new{stats_state="known",insertions=1,deletions=0,path="a.txt",diff_preview="+ 마지막 줄",diff_truncated=true,changed=true}}));
            var truncationNotice=UiText.Get("ExecutionDiffTruncated");
            Check(truncated.FileEditDetails.EndsWith("+ 마지막 줄"+truncationNotice)&&truncationNotice.Length>1&&(truncationNotice[0]=='\r'||truncationNotice[0]=='\n'),"diff truncation notice is glued to the last diff line");
            var unknown=new ExecutionCallRow(Json(new{tool_name="file_edit",file_edit=new{stats_state="unknown"}}));Check(unknown.AddedLinesText=="—","unknown edit treated as zero");
            var diagnostic="task_owner_mismatch: 原始句子 사용자";
            Check(NativeDiagnosticText.Describe(diagnostic).Contains(UiText.Get("NativeTaskOwnerMismatch"))&&NativeDiagnosticText.Describe(diagnostic).EndsWith(diagnostic),"native failure lost original");
            if(locale=="ko-KR")foreach(var key in new[]{"ExecutionInsertionHelp","ExecutionInsertionInactive","ExecutionInsertionPending","ExecutionInsertionReserved","ExecutionInsertionAttached","ExecutionInsertionExpired","ExecutionInsertionCancelled","ExecutionInsertionDeliveryUnknown","ExecutionRequestAndOutput","ActivitySummaryCounts","LocalHealthyPublicUnavailable","NativeElevatedUnavailable"})Check(Regex.IsMatch(UiText.Get(key),"[가-힣]"),"Korean message missing: "+key);
        }
        Check(UiText.ResolveLocale("system","ko") == "ko-KR" && UiText.ResolveLocale("system","ko-KR") == "ko-KR","neutral Korean not resolved");
        Check(UiText.ResolveLocale("system","fr-FR") == "en","unknown system culture did not use English");
        UiText.ApplyPreference("en");
        Console.WriteLine($"Korean presentation acceptance: {count} assertions passed. No user settings, network, runtime, window or installer were opened.");
    }
}
