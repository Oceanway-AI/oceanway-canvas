import contextlib, io, json, runpy, pathlib, sys

# Reuse the previous whitelist-only material/task snapshot without printing it.
with contextlib.redirect_stdout(io.StringIO()):
    base = runpy.run_path('/opt/oceanway-w7-test/scripts/c2-canvas-snapshot.py')
result, rows, studio = base['result'], base['rows'], base['studio']
result['agents'] = rows(studio, None, """SELECT json_build_object(
 'id',id,'status',status,'revision',revision,'canvas_id',canvas_id,
 'parent_id',parent_id,'active_task_id',active_task_id,'media_task_id',media_task_id,
 'event_count',event_count,'message_count',message_count,
 'policy_version',state_json::jsonb->'policy'->>'systemPolicyVersion',
 'approval_id',state_json::jsonb->'approval'->>'approvalId',
 'quote_id',state_json::jsonb->'approval'->'studioQuote'->>'id',
 'maximum',state_json::jsonb->'approval'->'studioQuote'->>'walletMaxPoints',
 'has_prepared',COALESCE(length(state_json::jsonb->'approval'->>'studioPreparedEncrypted'),0)>0,
 'decisions',state_json::jsonb->'decisions',
 'task_ids',state_json::jsonb->'taskIds')
 FROM cloud_agent_executions WHERE canvas_id='sQ3Xqpn5rFRjYHy6Pjigf' ORDER BY created_at""")
result['executions'] = rows(studio, None, """SELECT json_build_object(
 'task_id',t.id,'type',t.type,'status',t.status,'agent_run_id',t.agent_run_id,
 'state',e.state,'released',e.released,
 'same_authority_as_root',e.authorization_encrypted=r.authorization_encrypted)
 FROM tasks t JOIN studio_executions e ON e.task_id=t.id
 LEFT JOIN studio_executions r ON r.task_id=t.agent_run_id
 WHERE t.project_id='sQ3Xqpn5rFRjYHy6Pjigf' ORDER BY t.created_at""")
task_ids=','.join("'"+x['task_id'].replace("'","''")+"'" for x in result['platform_tasks']) or "''"
result['input_bindings']=rows('oceanway-w7-test-postgres-1',base['db'],"SELECT json_build_object('id',id,'asset_id',asset_id,'version_id',version_id,'target_type',target_type,'target_id',target_id,'slot',slot,'state',state) FROM platform_material_bindings WHERE target_type='task_input' AND managed_task_id IN ("+task_ids+") ORDER BY id")
root=pathlib.Path('/opt/oceanway-w7-test/private/c4-agent-20261109')
root.mkdir(mode=0o700,exist_ok=True)
label=sys.argv[1] if len(sys.argv)>1 else 'latest'
assert label.replace('-','').isalnum()
(root/(label+'.json')).write_text(json.dumps(result,indent=2))
print(json.dumps(result))
