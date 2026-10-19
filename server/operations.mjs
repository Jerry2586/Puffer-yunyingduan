import {defaultTheme, validateTheme} from './theme.mjs';
export const operationsContract='hetun-operations-v1';
export const platforms=['mobile','tablet','tv','web'];
export const operationKeys=['sections','features','navigation','categories','ads','announcement','clientSettings','schedule','carousel'];
const fields={
 sections:['id','enabled','page','title','subtitle','source','categoryId','limit','orderBy','manualIds'],
 navigation:['id','label','enabled','page','categoryId'],
 categories:['id','label','enabled','page','categoryId'],
 announcement:['enabled','text','title','image','frequency','actionLabel','page','url']
};
const pick=(v,keys)=>Object.fromEntries(keys.filter(k=>Object.hasOwn(v,k)).map(k=>[k,structuredClone(v[k])]));
function fail(message,status=400){throw Object.assign(Error(message),{status});}
// Explicit allowlist: geometry, styling, brand and packaged assets never travel here.
export function operationConfig(theme){
 const valid=validateTheme({...defaultTheme,...theme,carousel:{...defaultTheme.carousel,...theme?.carousel}}),out=pick(valid,['features','ads','clientSettings','schedule']);
 for(const key of ['sections','navigation','categories'])out[key]=valid[key].map(v=>pick(v,fields[key]));
 out.announcement=pick(valid.announcement,fields.announcement);
 out.carousel={slides:structuredClone(valid.carousel.slides)};
 return out;
}
export function mergeOperations(config,base=defaultTheme){
 const out=structuredClone(base),content=operationConfig({...base,...config,carousel:{...base.carousel,...config.carousel}});
 for(const key of operationKeys){
  if(!Object.hasOwn(config,key))continue;
  if(['sections','navigation','categories'].includes(key)){
   out[key]=content[key].map(item=>({...structuredClone(base[key].find(v=>v.id===item.id)||base[key][0]||{}),...item}));
  }else if(['carousel','announcement'].includes(key))out[key]={...out[key],...content[key]};
  else out[key]=content[key];
 }
 return validateTheme(out);
}
export function initializeOperations(state){
 let changed=false;
 if(!state.operations){state.operations={};changed=true;}
 for(const p of platforms){
  if(!state.operations[p]){const legacy=state.templates?.find(t=>t.id===state.activeTemplates?.[p]);state.operations[p]={version:1,publishedVersion:1,draft:operationConfig(legacy?.draft||defaultTheme),published:operationConfig(legacy?.published||defaultTheme),publishedAt:new Date().toISOString()};changed=true;}
  const r=state.operations[p];
  for(const key of ['draft','published']){const clean=operationConfig(r[key]);if(JSON.stringify(clean)!==JSON.stringify(r[key])){r[key]=clean;changed=true;}}
  if(!Array.isArray(r.history)){r.history=[];changed=true;}
 }
 return changed;
}
export function platformFor(id='runtime-mobile'){
 const value=String(id).replace(/^runtime-/,''),p=value==='desktop'?'web':value;
 if(!platforms.includes(p))fail('请选择手机、平板、电视或电脑端。');return p;
}
export function requestPlatform(url,headers){
 const query=url.searchParams.get('platform'),header=headers['x-hetun-platform'];
 if(query&&header&&platformFor(query)!==platformFor(header))fail('请求平台参数与平台标识不一致。');
 return platformFor(query||header||'mobile');
}
export function operationsStudio(state,id){
 const p=platformFor(id||'runtime-mobile'),names={mobile:'手机',tablet:'平板',tv:'电视',web:'电脑'};
 const templates=platforms.map(platform=>{const v=state.operations[platform];return {id:'runtime-'+platform,name:names[platform]+'端运营配置',platform,version:v.version,publishedVersion:v.publishedVersion,draft:mergeOperations(v.draft),published:mergeOperations(v.published)};});
 const t=templates.find(t=>t.platform===p);
 return {...t,templates,selectedTemplate:t.id,activeTemplates:Object.fromEntries(platforms.map(p=>[p,'runtime-'+p])),cmsBase:state.cmsBase,updatedAt:state.updatedAt,publishedAt:state.operations[p].publishedAt,history:state.operations[p].history.map(({version,publishedAt})=>({version,publishedAt})),contract:operationsContract};
}
function recordFor(state,input){
 const p=platformFor(input.template||'runtime-mobile'),record=state.operations[p];
 if(input.version!==record.version)fail('此端运营配置已被其他窗口修改，请重新加载。',409);
 return record;
}
export function saveOperations(state,input){
 const record=recordFor(state,input);
 if(!input.theme||typeof input.theme!=='object'||Array.isArray(input.theme))fail('运营配置必须为JSON对象。');
 const clean=operationConfig(input.theme);
 for(const key of ['androidUrl','iosUrl'])if(clean.clientSettings[key]&&!clean.clientSettings[key].toLowerCase().startsWith('https://'))fail('客户端更新地址必须使用 HTTPS。');
 record.draft=clean;record.version++;state.updatedAt=new Date().toISOString();return state;
}
function publish(record,config){
 record.history.unshift({version:record.publishedVersion,publishedAt:record.publishedAt,config:structuredClone(record.published)});record.history=record.history.slice(0,20);
 record.published=structuredClone(config);record.publishedVersion++;record.version++;record.publishedAt=new Date().toISOString();
}
export function publishOperations(state,input){const record=recordFor(state,input);publish(record,record.draft);state.updatedAt=record.publishedAt;return state;}
export function rollbackOperations(state,input){
 const record=recordFor(state,input),old=record.history.find(v=>v.version===input.targetVersion);
 if(!old)fail('历史发布版本不存在或已超过保留的20个版本。',404);
 record.draft=operationConfig(old.config);publish(record,record.draft);state.updatedAt=record.publishedAt;return state;
}
export function clientBootstrap(state,p,{os='android',version='',origin=''}={}){
 if(!['android','ios','windows','web'].includes(os))fail('不支持的客户端系统。');
 if(version&&!/^\d+\.\d+\.\d+$/.test(version))fail('客户端版本必须为三段数字。');
 const r=state.operations[p],s=r.published.clientSettings;
 const newer=(a,b)=>{const av=a.split('.').map(Number),bv=b.split('.').map(Number);for(let i=0;i<3;i++){if(av[i]!==bv[i])return av[i]>bv[i];}return false;};
 const candidate=os==='android'?s.androidUrl:os==='ios'?s.iosUrl:'';
 const download=candidate?.toLowerCase().startsWith('https://')?candidate:'';
 const available=!!(version&&s.latestVersion&&download&&newer(s.latestVersion,version));
 return {schemaVersion:1,contract:operationsContract,themeContract:'hetun-runtime-v1',platform:p,os,version:r.publishedVersion,publishedAt:r.publishedAt,config:r.published,serviceBase:origin,pollSeconds:15,cmsConfigured:!!state.cmsBase,update:{available,required:available&&s.forceUpdate,version:s.latestVersion,notes:s.updateNotes,url:download}};
}
