import {request,clientRequest} from './api';
import type {CaptchaInput} from './application';
export type ManagementKind='videos'|'users'|'danmu'|'comments'|'requests'|'feedback'|'reminders';
export type CommunityKind=Exclude<ManagementKind,'videos'|'users'>;
export interface ManagementRow {
 id:number;name?:string;cover?:string;categoryId?:number;categoryName?:string;status?:number;level?:number;sources?:string;year?:string;area?:string;
 nickname?:string;groupId?:number|string;groupName?:string;points?:number;expiresAt?:number;lastLoginAt?:number;lastLoginIP?:string;
 kind?:string;userId?:number;videoId?:string;videoName?:string;content?:string;reply?:string;approved?:boolean;reports?:number;count?:number;position?:number;episode?:number;pinned?:boolean;platform?:string;ip?:string;createdAt?:string;updatedAt?:string;version?:number;
}
export interface CommunityList {items:ManagementRow[];total:number;page:number;pages:number;pageSize?:number}
export interface ManagementList extends CommunityList {source:string}
export interface ManagementMeta {source:string;adapterVersion:number;categories:{id:number;name:string;parentId:number}[];groups:{id:number;name:string}[]}
export interface ManagementConnection {base:string;configured:boolean;localAvailable:boolean}
export interface ModerationPolicy {source:string;version:number;reviewComments:boolean;reviewDanmu:boolean;keywords:string[];ips:string[]}
export interface ManagementResult {source:string;ok:boolean;affected:number;cacheInvalidated?:boolean;auditRecorded?:boolean;warnings?:string[]}
export const managementApi={
 list:(kind:ManagementKind,filters:Record<string,string|number>)=>request<ManagementList>('/api/admin/manage/'+kind+'?'+Object.entries(filters).filter(([,v])=>v!=='').map(([k,v])=>encodeURIComponent(k)+'='+encodeURIComponent(String(v))).join('&')),
 action:(kind:ManagementKind,data:Record<string,unknown>)=>request<ManagementResult>('/api/admin/manage/'+kind+'/action','POST',data),
 connection:()=>request<ManagementConnection>('/api/admin/manage/connection'),
 connect:(source:string,key:string)=>request<{source:string;ok:boolean}>('/api/admin/manage/connection','POST',{source,key}),
 install:(source:string)=>request<{source:string;ok:boolean}>('/api/admin/manage/install','POST',{source}),
 test:()=>request<ManagementMeta>('/api/admin/manage/connection/test','POST',{}),
 meta:()=>request<ManagementMeta>('/api/admin/manage/meta'),
 policy:()=>request<ModerationPolicy>('/api/admin/manage/moderation'),
 savePolicy:(policy:ModerationPolicy)=>request<ModerationPolicy>('/api/admin/manage/moderation','POST',{...policy}),
};
// Member identity is resolved by the server from the existing CMS member session.
export const communityApi={
 list:(kind:CommunityKind,videoId='',mine=false,page=1,episode=1,pageSize=20)=>clientRequest<CommunityList>('community/'+kind+'?videoId='+encodeURIComponent(videoId)+'&mine='+(mine?'1':'0')+'&page='+page+'&episode='+episode+'&pageSize='+pageSize),
 submit:(kind:CommunityKind,content:string,videoId='',position=0,episode=1,captcha:CaptchaInput={})=>clientRequest<ManagementRow>('community/'+kind,'POST',{content,videoId,position,episode,...captcha}),
 thirdParty:(videoId:string,episode:number)=>clientRequest<{items:{content:string;position:number}[]}>('playback/danmu?videoId='+encodeURIComponent(videoId)+'&episode='+episode),
 report:(kind:'comments'|'danmu',id:number)=>clientRequest('community/'+kind+'/report','POST',{id}),
};
