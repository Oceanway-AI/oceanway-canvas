import json, pathlib
from decimal import Decimal

root=pathlib.Path(__file__).resolve().parent
read=lambda name:json.loads((root/(name+'.json')).read_text())
before,after=read('before-final-close'),read('after-final-reopen')
baseline=read('baseline')
run_id='agebdddd12eb9546b86f13e02d8cab915a'
task_id='ag92cdfea00afe0b4607ebc07a22675f08'
agent=next(x for x in after['agents'] if x['id']==run_id)
assert agent['status']=='completed',agent
assert before['canvas']==after['canvas']
assert before['materials']==after['materials'] and before['resources']==after['resources']
assert before['studio_tasks']==after['studio_tasks'] and before['platform_tasks']==after['platform_tasks']
assert len(after['canvas']['nodes'])==4 and len(after['canvas']['connections'])==2
old_nodes={x['id']:x for x in baseline['canvas']['nodes']}
after_nodes={x['id']:x for x in after['canvas']['nodes']}
assert all(after_nodes[k]==v for k,v in old_nodes.items())
media=[x for x in after['platform_tasks'] if x['request_id']=='studio-'+task_id]
assert len(media)==1 and len(after['platform_tasks'])==3
t=media[0];assert t['status']=='SUCCESS' and t['settled']=='true'
assert len(t['archives'])==1 and t['archives'][0]['state']=='saved' and t['archives'][0]['attempts']==1
assert t['funding'][0]['funds_state']=='settled' and t['funding'][0]['cap']==1250000
children=[x for x in after['executions'] if x['agent_run_id']==run_id]
assert children and all(x['same_authority_as_root'] is True for x in children)
bindings=[x for x in after['input_bindings'] if x['target_id']==t['task_id']]
assert len(bindings)==1 and bindings[0]['state']=='released'
assert bindings[0]['version_id']==baseline['resources'][0]['version_id'] or bindings[0]['version_id'] in [r['version_id'] for r in baseline['resources'] if r['mime']=='image/png']
assert next(x for x in after['agents'] if x['id']=='ag2668ba5c6963a004f6d0f0cb04884255')['status']=='rejected'
out={'completed_agent':run_id,'media_task':task_id,'platform_task':t['task_id'],'gateway_task':t['gateway_task'],'one_new_media_request':True,'shared_root_authority':True,'one_archive':True,'input_reference_released':True,'original_nodes_unchanged':True,'nodes':4,'edges':2,'close_reopen_same_nodes_edges_materials_tasks':True,'explicit_maximum_points':'250','settled_quota':t['quota'],'settled_points':str(Decimal(t['quota'])/5000)}
(root/'checks.json').write_text(json.dumps(out,indent=2));print(json.dumps(out))
