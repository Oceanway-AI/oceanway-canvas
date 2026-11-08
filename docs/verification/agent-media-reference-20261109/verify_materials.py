import json,pathlib,os,subprocess,urllib.request,urllib.parse,hashlib,struct
os.umask(0o077)
root=pathlib.Path('/opt/oceanway-w7-test/private/c4-agent-20261109')
state=json.loads((root/'before-final-close.json').read_text())
session=json.loads(pathlib.Path('/opt/oceanway-w7-test/private/c2-canvas-acceptance-20261108/admin-session.json').read_text())
env=dict(i.split('=',1) for i in json.loads(subprocess.check_output(['docker','inspect','oceanway-studio-preview-backend-1']))[0]['Config']['Env'] if '=' in i)
headers={'Origin':'https://test.oceanway.tech:18448','Cookie':session['cookie'],'X-Auth-Session':session['sid'],'X-Platform-Product-Key':env['STUDIO_PLATFORM_PRODUCT_KEY'],'X-Oceanway-Materials-Version':'2','Content-Type':'application/json'}
results=[]
for material in state['materials']:
 if material['operation_id']!='media_e7bc0b3194cc6eba65cfedf0d0c6117aec2e96ea93c51006061dbc893d76f097':continue
 video=material['mime'].startswith('video/')
 variants=['original','poster','playback'] if video else ['original','thumbnail','preview']
 items=[{'item_id':v,'asset_id':material['asset_id'],'version_id':material['version_id'],'variant':v,'purpose':'download' if v=='original' else 'display'} for v in variants]
 request=urllib.request.Request('https://test.oceanway.tech:18447/api/platform/v1/materials/access',data=json.dumps({'items':items}).encode(),headers=headers,method='POST')
 with urllib.request.urlopen(request,timeout=30) as r:access=json.load(r)
 data=access.get('data',access)
 for row in data['results']:
  out={'asset_id':material['asset_id'],'file_id':material['file_id'],'variant':row['actual_variant'],'state':row['status']}
  assert row['status']=='ready',out
  url=row['url'];assert urllib.parse.urlparse(url).scheme=='https'
  with urllib.request.urlopen(url,timeout=120) as r:blob=r.read(150*1024*1024);out.update(http=r.status,host=urllib.parse.urlparse(url).hostname,mime=r.headers.get('Content-Type'),bytes=len(blob),sha256=hashlib.sha256(blob).hexdigest(),cache=r.headers.get('X-Cache'))
  if row['actual_variant']=='original':
   out['digest_matches']=out['sha256']==material['sha256'];out['bytes_match']=len(blob)==material['bytes'];assert out['digest_matches'] and out['bytes_match']
   if blob[:8]==b'\x89PNG\r\n\x1a\n':out['dimensions']=list(struct.unpack('>II',blob[16:24]))
   if video:
    path=root/'generated-video.mp4';path.write_bytes(blob)
    try:
     probe=json.loads(subprocess.check_output(['ffprobe','-v','error','-show_entries','stream=codec_name,width,height:format=duration','-of','json',str(path)]))
     out['probe']=probe
    except FileNotFoundError:out['probe']='ffprobe unavailable on host'
  else:
   assert out['host']=='cdn-test.oceanway.tech'
   with urllib.request.urlopen(url,timeout=90) as r:repeat=r.read(150*1024*1024);out['second_cache']=r.headers.get('X-Cache');out['repeat_digest_matches']=hashlib.sha256(repeat).hexdigest()==out['sha256']
   assert out['repeat_digest_matches']
  results.append(out)
(root/'material-verification.json').write_text(json.dumps(results,indent=2));print(json.dumps(results))
