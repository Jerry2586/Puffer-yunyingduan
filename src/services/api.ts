import defaults from '@/domain/default-theme.json';
import {normalizeTheme} from '@/domain/normalize-theme';
import type {Theme,StudioState,Catalog,Category,Platform} from '@/domain/types';
export interface CmsCheck {id:'categories'|'catalog';label:string;ok:boolean;latencyMs:number;count?:number;total?:number|null;error?:string;}
export interface CmsStatus {base:string;configured:boolean;ok:boolean;status:'unconfigured'|'ready'|'empty'|'error';checkedAt:string;latencyMs:number;checks:CmsCheck[];message:string;}
export interface LicenseStatus {url:string;domain:string;keyConfigured:boolean;trustConfigured:boolean;products:Record<'film-center'|'theme',{valid:boolean;message?:string;expiresAt?:string;themes?:string[];licenseId?:string}>;}
export interface SetupStatus {installed:boolean;completedAt:string;cmsConfigured:boolean;publicOrigin:string;localPreview:boolean;username:string;}
export interface CmsDiscovery {candidates:{id:string;root:string;name:string;base:string;adapterInstalled:boolean;report:CmsStatus|null}[];warnings:string[];recommendedId:string;multiple:boolean;checkedAt:string;}
const key='hetun-service-base';let clientPlatform:Platform='mobile';export function setClientPlatform(platform:Platform){clientPlatform=platform;}
export function getServiceBase():string {return String(import.meta.env.VITE_HETUN_SERVICE_BASE || '').replace(/\/$/,'');}
export function setServiceBase(value:string) { if(value && !/^https?:\/\/[^\s]+$/i.test(value)) throw new Error('请输入完整的服务地址。'); uni.setStorageSync(key,value.replace(/\/$/,'')); }
export function request<T>(endpoint:string,method:'GET'|'POST'|'PUT'='GET',data?:Record<string,unknown>):Promise<T> {
  let base=getServiceBase();
  // #ifdef APP-PLUS
  if(!base) return Promise.reject(new Error('应用未配置服务地址，请联系运营方更新安装包。'));
  // #endif
  return new Promise((resolve,reject) => uni.request({url:base+endpoint,method,data,withCredentials:true,header:{'Content-Type':'application/json','X-Hetun-Client':'studio','X-Hetun-Platform':clientPlatform,...(uni.getStorageSync('hetun-member-token')?{Authorization:'Bearer '+uni.getStorageSync('hetun-member-token')}:{})},timeout:endpoint==='/api/admin/setup/discover'?65000:25000,
    success:response => { const payload=response.data as any; if(response.statusCode>=200 && response.statusCode<300) resolve(payload); else reject(Object.assign(new Error(payload?.error || '服务连接失败：'+response.statusCode),{status:response.statusCode,code:payload?.code})); },
    fail:() => reject(new Error('无法连接河豚服务，请检查服务地址和网络。'))}));
}
export const api={
  exportTheme:(template:string)=>request<{name:string;data:string}>('/api/admin/export?template='+encodeURIComponent(template)),
  license:()=>request<LicenseStatus>('/api/admin/license'),
  activateLicense:(key:string)=>request<LicenseStatus>('/api/admin/license','PUT',{key}),
  clearLicense:()=>request<LicenseStatus>('/api/admin/license','PUT',{clear:true}),
  health:()=>request<{status:string;cmsConfigured:boolean}>('/api/health'),
  theme:async(platform:Platform='mobile'):Promise<{theme:Theme;version:number;template:string}>=>{
    setClientPlatform(platform);try{const runtime=await request<{config:Partial<Theme>;version:number}>('/api/runtime?platform='+platform);return {theme:normalizeTheme({...defaults,...runtime.config,carousel:{...defaults.carousel,...runtime.config.carousel}} as Theme),version:runtime.version,template:'bundled-client'};}
    catch(e){if((e as any).status!==404)throw e;return request('/api/theme?platform='+platform);}
  },
  setup:()=>request<SetupStatus>('/api/admin/setup'),
  discoverCms:()=>request<CmsDiscovery>('/api/admin/setup/discover','POST',{}),
  setupCheck:(base:string)=>request<CmsStatus>('/api/admin/setup/check','POST',{base}),
  completeSetup:(username:string,password:string,base:string)=>request<SetupStatus>('/api/admin/setup/complete','POST',{username,password,base}),
  account:(currentPassword:string,username:string,password:string)=>request('/api/admin/account','PUT',{currentPassword,username,password}),
  catalog:(page=1,keyword='',category='',ids='')=>request<Catalog>('/api/catalog?page='+page+'&keyword='+encodeURIComponent(keyword)+'&category='+encodeURIComponent(category)+'&ids='+encodeURIComponent(ids)),
  categories:()=>request<{items:Category[]}>('/api/catalog/categories'),
  login:(username:string,password:string)=>request('/api/admin/login','POST',{username,password}),
  logout:()=>request('/api/admin/logout','POST',{}),state:(template='')=>request<StudioState>('/api/admin/state?template='+encodeURIComponent(template)),
  save:(theme:Theme,version:number,template='')=>request<StudioState>('/api/admin/draft','PUT',{theme,version,template}),
  publish:(version:number,template='')=>request<StudioState>('/api/admin/publish','POST',{version,template}),
  install:(name:string,data:string)=>request<StudioState>('/api/admin/install','POST',{name,data}),
  renameTemplate:(template:string,name:string,version:number)=>request<StudioState>('/api/admin/template/rename','PUT',{template,name,version}),
  removeTemplate:(template:string,version:number,unpublish:boolean)=>request<StudioState>('/api/admin/template/remove','POST',{template,version,unpublish}),
  asset:(name:string,data:string)=>request<{url:string}>('/api/admin/asset','POST',{name,data}),
  playback:(id:string,sid:number,nid:number)=>request<{url:string;name:string}>('/api/playback','POST',{id,sid,nid}),
  cmsStatus:()=>request<CmsStatus>('/api/admin/cms/status'),
  checkCms:(base:string)=>request<CmsStatus>('/api/admin/cms/check','POST',{base}),
  cms:(base:string)=>request<StudioState & {cmsStatus:CmsStatus}>('/api/admin/cms','PUT',{base})
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
