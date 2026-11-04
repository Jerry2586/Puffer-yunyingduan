import {request,clientRequest,captureMemberContext,captureConfigurationRequest} from './api';
import {receiveApplication,type ApplicationConfig,type PublicApplication} from './application-runtime';
export type {ApplicationConfig,PublicApplication} from './application-runtime';
export interface ApplicationSettings {version:number;updatedAt:string;config:ApplicationConfig;source:string;adapterConfigured:boolean;capabilities:{vpnDetection:boolean;history:boolean;captcha:boolean;searchSync:string;nativeTVDownload:boolean}}
export interface CaptchaInput {captchaToken?:string;captchaAnswer?:string}
export interface CaptchaChallenge {required:boolean;token?:string;image?:string;expiresIn?:number}
export interface HistoryEntry {videoId:string;episode:number;position:number;updatedAt:string;title?:string;poster?:string}
export const applicationApi={
 get:()=>request<ApplicationSettings>('/api/admin/application-settings'),
 save:(version:number,config:ApplicationConfig)=>request<ApplicationSettings>('/api/admin/application-settings','POST',{version,config}),
 config:async()=>{const current=captureConfigurationRequest();const v=await clientRequest<PublicApplication>('application/config');current();receiveApplication(v);return v;},
 captcha:(action:'login'|'register'|'comments'|'danmu')=>clientRequest<CaptchaChallenge>('application/captcha?action='+action),
 invitation:async()=>{const current=captureMemberContext();const result=await clientRequest<{code:string;text:string}>('member/invitation');current();return result;},
 avatar:async(data:{url?:string;data?:string})=>{const current=captureMemberContext();const result=await clientRequest<{ok:boolean;url:string}>('member/avatar','POST',data);current();return result;},
 history:async()=>{const current=captureMemberContext();const result=await clientRequest<{items:HistoryEntry[]}>('member/history');current();return result;},
 remember:async(videoId:string,episode:number,position:number)=>{const current=captureMemberContext();const result=await clientRequest<{ok:boolean;entry:HistoryEntry}>('member/history','POST',{videoId,episode,position});current();return result;},
};
export interface PlaybackSource {code:string;name:string;enabled:boolean;trialSeconds:number;mode:'auto'|'direct'}
export interface DanmuSettings {enabled:boolean;thirdParty:boolean;endpoint:string;match:'id'|'title';preferredDomain:string;tvDensity:number}
export interface PlaybackPreferences {version:number;updatedAt:string;source:string;sources:PlaybackSource[];danmu:DanmuSettings}
export const playbackCenterApi={
 get:()=>request<PlaybackPreferences>('/api/admin/playback-center'),
 save:(v:PlaybackPreferences)=>request<PlaybackPreferences>('/api/admin/playback-center','POST',{...v}),
 sources:()=>request<{source?:string;items:{code:string;name:string;enabled?:boolean}[]}>('/api/admin/playback-center/sources'),
 test:(config:PlaybackPreferences,videoId:string,episode:number)=>request<{ok:boolean;count:number;items:{content:string;position:number}[]}>('/api/admin/playback-center/test-danmu','POST',{config,videoId,episode}),
};
