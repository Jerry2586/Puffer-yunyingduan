import { cmsEndpoint, requestCms } from './cms.mjs';

const failureMessage = error => {
  if (error?.code) return ({ENOTFOUND:'CMS 域名无法解析。',EAI_AGAIN:'CMS 域名解析暂时失败。',ECONNREFUSED:'CMS 服务拒绝连接。',ECONNRESET:'CMS 连接被中断。',ETIMEDOUT:'CMS 连接超时。',CERT_HAS_EXPIRED:'CMS HTTPS 证书已过期。',DEPTH_ZERO_SELF_SIGNED_CERT:'CMS HTTPS 证书不受信任。'})[error.code] || 'CMS 网络或 HTTPS 连接失败，请检查站点。';
  return String(error?.message || 'CMS 检测失败。').slice(0,240);
};
async function boundedRequest(base, params, allowPrivate) {
  let timer;
  try {return await Promise.race([requestCms(base,params,allowPrivate),new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('CMS 检测超时，请检查站点后重试。')),11000);})]);}
  finally {clearTimeout(timer);}
}
export function createCmsChecker(allowPrivate=false) {
  const pending=new Map();
  return async function checkCms(value) {
    const started=Date.now();
    const report=(base,configured,status,checks,message='')=>({base,configured,ok:status==='ready'||status==='empty',status,checkedAt:new Date().toISOString(),latencyMs:Date.now()-started,checks,message});
    if(!String(value||'').trim())return report('',false,'unconfigured',[],'尚未配置苹果 CMS，请填写站点地址。');
    let base;
    try {base=cmsEndpoint(String(value).trim()).toString();}
    catch(error){return report('',true,'error',[],failureMessage(error));}
    if(pending.has(base))return pending.get(base);
    const task=(async()=>{
      const checks=await Promise.all([
        {id:'categories',label:'分类接口',params:{ac:'list'},validate:raw=>{
          if(!Array.isArray(raw.class)||raw.class.some(v=>!v||v.type_id==null||typeof v.type_name!=='string'))throw new Error('CMS 分类接口缺少兼容的 class 分类数据。');
          return {count:raw.class.length};
        }},
        {id:'catalog',label:'影片接口',params:{ac:'detail',pg:1},validate:raw=>{
          if(raw.list.some(v=>!v||v.vod_id==null||typeof v.vod_name!=='string'))throw new Error('CMS 影片接口缺少兼容的影片编号和名称。');
          const total=Number(raw.total);return {count:raw.list.length,total:raw.total!==undefined&&Number.isSafeInteger(total)&&total>=raw.list.length?total:null};
        }}
      ].map(async spec=>{
        const begin=Date.now();
        try {const raw=await boundedRequest(base,spec.params,allowPrivate);if(raw.code!=null&&Number(raw.code)!==1)throw new Error('CMS 接口返回失败状态，请检查接口访问配置。');return {id:spec.id,label:spec.label,ok:true,...spec.validate(raw),latencyMs:Date.now()-begin};}
        catch(error){return {id:spec.id,label:spec.label,ok:false,latencyMs:Date.now()-begin,error:failureMessage(error)};}
      }));
      if(checks.some(v=>!v.ok))return report(base,true,'error',checks,'内容接口检测未通过。');
      if(checks.some(v=>v.count===0))return report(base,true,'empty',checks,'接口已接通，但分类或影片为空，请检查 CMS 内容。');
      return report(base,true,'ready',checks,'分类与影片接口检测通过。');
    })();
    pending.set(base,task);
    try {return await task;}finally{pending.delete(base);}
  };
}
