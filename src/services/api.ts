import defaults from '@/domain/default-theme.json';
import {normalizeTheme} from '@/domain/normalize-theme';
import type {Theme,StudioState,Catalog,Category,Platform} from '@/domain/types';
export interface CmsCheck {url?:string;id:'categories'|'catalog';label:string;ok:boolean;latencyMs:number;count?:number;total?:number|null;error?:string;}
export interface CmsStatus {transport?:'http'|'local';recognized?:boolean;base:string;configured:boolean;ok:boolean;status:'unconfigured'|'ready'|'empty'|'error';checkedAt:string;latencyMs:number;checks:CmsCheck[];message:string;}
export interface SetupStatus {installed:boolean;completedAt:string;cmsConfigured:boolean;publicOrigin:string;localPreview:boolean;username:string;}
export interface CmsDiscovery {connected?:boolean;connectedBase?:string;currentReport?:CmsStatus|null;candidates:{id:string;root:string;name:string;base:string;adapterInstalled:boolean;recognized?:boolean;source?:string;attempts?:{base:string;source:string;report:CmsStatus}[];report:CmsStatus|null}[];warnings:string[];recommendedId:string;multiple:boolean;checkedAt:string;}
const key='hetun-service-base';let clientPlatform:Platform='mobile';export function setClientPlatform(platform:Platform){clientPlatform=platform;}
export function getServiceBase():string {return String(import.meta.env.VITE_HETUN_SERVICE_BASE || '').replace(/\/$/,'');}
export function setServiceBase(value:string) { if(value && !/^https?:\/\/[^\s]+$/i.test(value)) throw new Error('请输入完整的服务地址。'); uni.setStorageSync(key,value.replace(/\/$/,'')); }
export function request<T>(endpoint:string,method:'GET'|'POST'|'PUT'='GET',data?:Record<string,unknown>):Promise<T> {
  let base=getServiceBase();
  // #ifdef APP-PLUS
  if(!base) return Promise.reject(new Error('应用未配置服务地址，请联系运营方更新安装包。'));
  // #endif
  return new Promise((resolve,reject) => uni.request({url:base+endpoint,method,data,withCredentials:true,header:{'Content-Type':'application/json','X-Hetun-Client':'studio','X-Hetun-Platform':clientPlatform,...(uni.getStorageSync('hetun-member-token')?{Authorization:'Bearer '+uni.getStorageSync('hetun-member-token')}:{})},timeout:endpoint==='/api/admin/setup/discover'?65000:['/api/health','/api/admin/setup'].includes(endpoint)||endpoint.startsWith('/api/admin/state?')?10000:25000,
    success:response => { const payload=response.data as any; if(response.statusCode>=200 && response.statusCode<300) resolve(payload); else reject(Object.assign(new Error(payload?.error || '服务连接失败：'+response.statusCode),{status:response.statusCode,code:payload?.code,cmsStatus:payload?.cmsStatus})); },
    fail:() => reject(new Error('无法连接河豚服务，请检查服务地址和网络。'))}));
}
export const api={
  exportTheme:(template:string)=>request<{name:string;data:string}>('/api/admin/export?template='+encodeURIComponent(template)),
  health:()=>request<{status:string;version:string;cmsConfigured:boolean}>('/api/health'),
  theme:async(platform:Platform='mobile'):Promise<{theme:Theme;version:number;template:string}>=>{
    setClientPlatform(platform);try{const runtime=await request<{config:Partial<Theme>;version:number}>('/api/runtime?platform='+platform);return {theme:normalizeTheme({...defaults,...runtime.config,carousel:{...defaults.carousel,...runtime.config.carousel}} as Theme),version:runtime.version,template:'bundled-client'};}
    catch(e){if((e as any).status!==404)throw e;return request('/api/theme?platform='+platform);}
  },
  setup:()=>request<SetupStatus>('/api/admin/setup'),
  discoverCms:()=>request<CmsDiscovery>('/api/admin/setup/discover','POST',{}),
  setupCheck:(base:string)=>request<CmsStatus>('/api/admin/setup/check','POST',{base}),
  completeSetup:(username:string,password:string,base:string)=>request<SetupStatus>('/api/admin/setup/complete','POST',{username,password,base}),
  account:(currentPassword:string,username:string,password:string)=>request('/api/admin/account','PUT',{currentPassword,username,password}),
  catalog:(page=1,keyword='',category='',ids='',orderBy='latest')=>request<Catalog>('/api/catalog?page='+page+'&keyword='+encodeURIComponent(keyword)+'&category='+encodeURIComponent(category)+'&ids='+encodeURIComponent(ids)+'&orderBy='+encodeURIComponent(orderBy)),
  categories:()=>request<{items:Category[]}>('/api/catalog/categories'),
  login:(username:string,password:string)=>request('/api/admin/login','POST',{username,password}),
  logout:()=>request('/api/admin/logout','POST',{}),state:(template='')=>request<StudioState>('/api/admin/state?template='+encodeURIComponent(template)),
  save:(theme:Theme,version:number,template='')=>request<StudioState>('/api/admin/draft','PUT',{theme,version,template}),
  publish:(version:number,template='')=>request<StudioState>('/api/admin/publish','POST',{version,template}),
  rollback:(version:number,targetVersion:number,template='')=>request<StudioState>('/api/admin/rollback','POST',{version,targetVersion,template}),
  install:(name:string,data:string)=>request<StudioState>('/api/admin/install','POST',{name,data}),
  renameTemplate:(template:string,name:string,version:number)=>request<StudioState>('/api/admin/template/rename','PUT',{template,name,version}),
  removeTemplate:(template:string,version:number,unpublish:boolean)=>request<StudioState>('/api/admin/template/remove','POST',{template,version,unpublish}),
  asset:(name:string,data:string)=>request<{url:string}>('/api/admin/asset','POST',{name,data}),
  playback:(id:string,sid:number,nid:number)=>request<{url:string;name:string}>('/api/playback','POST',{id,sid,nid}),
  cmsStatus:()=>request<CmsStatus>('/api/admin/cms/status'),
  checkCms:(base:string)=>request<CmsStatus>('/api/admin/cms/check','POST',{base}),
  cms:(base:string,siteId?:string)=>request<StudioState & {cmsStatus:CmsStatus}>('/api/admin/cms','PUT',{base,siteId})
};

export const memberApi={
 checkout:(orderId:number,code:string,payment:string,platform:string)=>request<any>('/api/member/checkout','POST',{orderId,code,payment,platform}),
 config:()=>request<any>('/api/member/config'),plans:()=>request<any>('/api/member/plans'),profile:()=>request<any>('/api/member/profile'),
 login:async(username:string,password:string,register=false,password2='')=>{const v=await request<any>('/api/member/'+(register?'register':'login'),'POST',{username,password,password2});uni.setStorageSync('hetun-member-token',v.token);return v;},
 logout:async()=>{await request('/api/member/logout','POST',{});uni.removeStorageSync('hetun-member-token');},
 orders:()=>request<any>('/api/member/orders'),order:(code:string)=>request<any>('/api/member/order?code='+encodeURIComponent(code)),
 purchase:(groupId:number,period:string,platform:string)=>request<any>('/api/member/purchase','POST',{groupId,period,platform}),
 upgrade:(groupId:number,period:string)=>request<any>('/api/member/upgrade','POST',{groupId,period}),recharge:(price:number,platform:string)=>request<any>('/api/member/recharge','POST',{price,platform})
};
export function assetUrl(value:string){return value?.startsWith('/api/assets/')?getServiceBase()+value:value;}
export async function chooseLocalFile(accept:string):Promise<{name:string;data:string}>{
 // #ifdef H5
 return new Promise((resolve,reject)=>{const input=document.createElement('input');input.type='file';input.accept=accept;input.oncancel=()=>reject(new Error('已取消文件选择。'));input.onchange=async()=>{const file=input.files?.[0];if(!file)return reject(new Error('未选择文件。'));if(file.size>12*1024*1024)return reject(new Error('文件最大12MB。'));const bytes=new Uint8Array(await file.arrayBuffer());let text='';for(let i=0;i<bytes.length;i+=8192)text+=String.fromCharCode(...bytes.subarray(i,i+8192));resolve({name:file.name,data:btoa(text)});};input.click();});
 // #endif
 // #ifndef H5
 throw new Error('模板和素材请在网页工作台上传。');
 // #endif
}

export interface SystemOverview {version:string;goVersion:string;os:string;arch:string;startedAt:string;uptimeSeconds:number;cpuCores:number;goroutines:number;heapBytes:number;runtimeBytes:number;redis:RedisOverview;http:HttpOverview;database:{mode:string;ok:boolean;version?:string;address?:string;name?:string;username?:string;maxIdle?:number;lifetimeSeconds?:number;pool?:{open:number;inUse:number;idle:number;maxOpen:number;waitCount:number;waitDurationMs:number};latencyMs:number};cmsBase:string;host:string;pid:number;memory:{available:boolean;totalBytes?:number;usedBytes?:number;rssBytes:number};requests:{total:number;failed:number;active:number;peak:number;averageMs:number};cms:CmsStatus;revision:number;checkedAt:string;}
export interface SystemConfigValues {adminPath:string;listen:string;publicOrigin:string;clientOrigins:string[]|null;}
export interface SystemConfig {active:SystemConfigValues;saved:SystemConfigValues;restartRequired:boolean;writable:boolean;storage:string;adminPath:string;dataDir:string;auditPath:string;identity:{algorithm:string;fingerprint:string;directoryPath:string};}
export interface SystemLogs {items:{at:string;event:string;revision:number}[];truncated:boolean;source:string;runtimeLogs:string;checkedAt:string;}
export const systemApi={connections:()=>request<ConnectionConfig>('/api/admin/system/connections'),saveConnections:(v:ConnectionInput)=>request<ConnectionConfig>('/api/admin/system/connections','PUT',{...v}),testConnections:(v:ConnectionInput)=>request<ConnectionTest>('/api/admin/system/connections/test','POST',{...v}),overview:()=>request<SystemOverview>('/api/admin/system/overview'),config:()=>request<SystemConfig>('/api/admin/system/config'),saveConfig:(v:Partial<SystemConfigValues>)=>request<SystemConfig>('/api/admin/system/config','PUT',{...v}),logs:(filter='all')=>request<SystemLogs>('/api/admin/system/logs?filter='+encodeURIComponent(filter))};

export interface RedisOverview {configured:boolean;ok:boolean;message:string;address?:string;username?:string;db?:number;version?:string;ttlSeconds?:number;used_memory?:number;used_memory_peak?:number;connected_clients?:number;infoAvailable?:boolean;latencyMs?:number;cacheHits?:number;cacheMisses?:number;cacheErrors?:number;pool?:{total:number;idle:number;max:number;minIdle:number;maxIdle:number;timeouts:number;hits:number;misses:number};}
export interface HttpOverview {active:number;total:number;failed:number;averageMs:number;reusePercent:number;busiestHost:string;busiestActive:number;maxPerHost:number;timeoutSeconds:number;idleSeconds:number;maxIdle:number;maxIdlePerHost:number;sampling:string;}

export interface AppRelease {id:string;platform:string;os:string;version:string;sizeMB:number;downloadUrl:string;browserUrl:string;notes:string;force:boolean;enabled:boolean;createdAt:string;updatedAt:string;revision:number;legacy?:boolean;}
export interface AppReleaseList {items:AppRelease[];currentIds:string[];total:number;}
export const releasesApi={list:()=>request<AppReleaseList>('/api/admin/releases'),save:(row:Partial<AppRelease>,create:boolean)=>request<AppReleaseList>('/api/admin/releases',create?'POST':'PUT',{...row}),remove:(row:AppRelease)=>request<AppReleaseList>('/api/admin/releases/delete','POST',{id:row.id,revision:row.revision})};

export interface DatabaseSettings {connectSeconds:number;readSeconds:number;writeSeconds:number;maxOpen:number;maxIdle:number;lifetimeSeconds:number;idleSeconds:number;}
export interface HttpSettings {timeoutSeconds:number;maxIdle:number;maxIdlePerHost:number;maxPerHost:number;idleSeconds:number;}
export interface RedisSettings {address:string;username:string;password:string;db:number;poolSize:number;minIdle:number;maxIdle:number;ttlSeconds:number;}
export interface ConnectionValues {database:{mode:string;settings:DatabaseSettings;address?:string;name?:string;username?:string};redis:{values:RedisSettings;passwordSet:boolean}|null;http:HttpSettings;}
export interface ConnectionConfig {active:ConnectionValues;saved:ConnectionValues;revision:string;restartRequired:boolean;writable:boolean;}
export interface ConnectionInput {section:'database'|'redis'|'http';values:DatabaseSettings|RedisSettings|HttpSettings;revision?:string;passwordAction?:'keep'|'replace'|'clear';}
export interface ConnectionTest {ok:boolean;message:string;latencyMs:number;checkedAt:string;report?:CmsStatus|null;}

export interface ServiceUpdate {current:string;platform:string;repository:string;phase:string;message:string;checkedAt:string;available:boolean;canInstall:boolean;reason:string;latest:{version:string;url:string;notes:string;publishedAt:string;asset:string;size:number}|null;}
export const updateApi={status:()=>request<ServiceUpdate>('/api/admin/system/update'),check:()=>request<ServiceUpdate>('/api/admin/system/update/check','POST',{}),install:(version:string)=>request<ServiceUpdate>('/api/admin/system/update/install','POST',{version}),health:()=>request<{status:string;version:string}>('/api/health')};
