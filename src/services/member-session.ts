import {ref} from 'vue';
import {applicationRuntime} from './application-runtime';

interface MemberIdentity {service?:string;source?:string;id?:string}
// A persisted identity is not proof that the current token belongs to that member.
export const memberIdentity=ref<MemberIdentity>({});
let verifiedToken='';
export function receiveMember(profile:any,service:string,source=applicationRuntime.sourceKey){
 const id=String(profile?.user_id||profile?.id||'');
 verifiedToken=id?String(uni.getStorageSync('hetun-member-token')||''):'';
 memberIdentity.value=id?{service,source,id}:{};
 uni.removeStorageSync('hetun-member-identity');
}
export function historyMember(service:string){
 const m=memberIdentity.value,token=String(uni.getStorageSync('hetun-member-token')||'');
 return !!m.id&&!!applicationRuntime.sourceKey&&m.service===service&&m.source===applicationRuntime.sourceKey&&!!verifiedToken&&token===verifiedToken;
}
export function historyScope(service:string){return JSON.stringify([service,applicationRuntime.sourceKey,historyMember(service)?memberIdentity.value.id:'guest']);}
