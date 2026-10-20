import {request} from './api';
import type {Platform} from '@/domain/types';
export interface AdSlot {id:string;enabled:boolean;order:string;dailyCap:number;cooldownSeconds:number;every:number;excludeVip:boolean}
export interface AdCreative {id:string;name:string;advertiser:string;orderNo:string;kind:string;slot:string;status:string;media:string;asset:string;text:string;destination:string;url:string;os:string[];priority:number;start:string;end:string;duration:number;skipAfter:number}
export interface AdChannel {id:string;name:string;provider:string;os:string;enabled:boolean;appId:string;units:Record<string,string>}
export interface AdConfig {slots:AdSlot[];creatives:AdCreative[];channels:AdChannel[];reward:{enabled:boolean;points:number;dailyCap:number;cooldownSeconds:number}}
export interface AdMetrics {requests:number;fills:number;impressions:number;clicks:number;completes:number;errors:number;rewards:number;points:number}
export interface AdEvent {at:string;decision:string;slot:string;source:string;creativeId:string;type:string;code:string}
export interface AdReward {id:string;transaction:string;user:string;decision:string;channel:string;points:number;at:string}
export interface AdReport {platform:Platform;version:number;publishedVersion:number;publishedAt:string;draft:AdConfig;published:AdConfig;today:AdMetrics;totals:AdMetrics;days:{date:string;metrics:AdMetrics}[];events:AdEvent[];rewards:AdReward[];slotNames:Record<string,string>;callbackUrl:string;checkedAt:string}
export interface AdDiagnosis {selected:Record<string,unknown>|null;reason:string;checks:{name:string;ok:boolean;error?:string}[];message:string;checkedAt:string}
const url=(p:Platform,s='')=>'/api/admin/ads'+s+'?platform='+p;
export const advertisingApi={get:(p:Platform)=>request<AdReport>(url(p)),save:(p:Platform,version:number,config:AdConfig)=>request<AdReport>(url(p),'PUT',{version,config}),publish:(p:Platform,version:number)=>request<AdReport>(url(p,'/publish'),'POST',{version}),diagnose:(p:Platform,os:string,slot:string)=>request<AdDiagnosis>(url(p,'/diagnose'),'POST',{os,slot})};
