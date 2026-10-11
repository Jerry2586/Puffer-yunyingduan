// Rebuild only supported gateway data. Never execute CMS-returned scripts.
const decode = s => String(s).replace(/&(?:amp|quot|apos|lt|gt|#(\d+)|#x([\da-f]+));/gi,(v,n,h)=>n||h?String.fromCodePoint(Math.min(0x10ffff,parseInt(n||h,n?10:16))):({'&amp;':'&','&quot;':'"','&apos;':"'",'&lt;':'<','&gt;':'>'}[v.toLowerCase()]||v));
const attrs = s => {const out={};for(const m of s.matchAll(/([a-z][a-z0-9_-]*)\s*=\s*(?:"([^"<>]*)"|'([^'<>]*)'|([^\s<>"']+))/gi)){const k=m[1].toLowerCase();if(k in out)throw new Error('收银台字段重复。');out[k]=decode(m[2]??m[3]??m[4]);}return out;};
function webUrl(s,base){const u=new URL(String(s),base);if(!['https:','http:'].includes(u.protocol)||u.username||u.password||u.href.length>8000)throw new Error('收银台地址无效。');return u.toString();}
export function normalizePayment(data,base){
 if(data?.type==='qrcode'){const code=String(data.code_url||'');if(!/^(https?:\/\/|weixin:\/\/|alipays:\/\/)/i.test(code)||/[\u0000-\u0020]/.test(code)||code.length>2000)throw new Error('支付二维码无效。');return {kind:'qrcode',code};}
 if(data?.type==='data'){const d=data.data||{};const u=d.pay_url||d.url;if(typeof u==='string')return {kind:'redirect',url:webUrl(u,base)};throw new Error('当前网关返回了未支持的支付数据，请在 CMS 收银台完成付款。');}
 if(data?.type==='html'){
  const html=String(data.html||'');if(html.length>120000)throw new Error('收银台响应过大。');
  const form=/<form\b([^>]*)>([\s\S]*?)<\/form>/i.exec(html);
  if(form){const a=attrs(form[1]);const action=webUrl(a.action,base),method=String(a.method||'get').toLowerCase();if(!['get','post'].includes(method))throw new Error('收银台方法不支持。');const fields=[];for(const m of form[2].matchAll(/<input\b([^>]*)>/gi)){const v=attrs(m[1]);if(v.type&&v.type.toLowerCase()!=='hidden')continue;if(!v.name||v.name.length>150||String(v.value||'').length>16000)throw new Error('收银台字段无效。');fields.push({name:v.name,value:v.value||''});if(fields.length>100)throw new Error('收银台字段过多。');}if(!fields.length)throw new Error('收银台签名字段缺失。');return {kind:'form',action,method,fields};}
  const redirect=/(?:window\.)?location(?:\.href)?\s*=\s*(["'])([^"'<>\r\n]+)\1/i.exec(html);if(redirect)return {kind:'redirect',url:webUrl(decode(redirect[2]),base)};
 }
 throw new Error('当前支付网关格式未支持，请在 CMS 收银台完成付款。');
}
const escape=s=>String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
export function checkoutDocument(p){if(p.kind!=='form')throw new Error('无付款表单。');const origin=new URL(p.action).origin;return {csp:"default-src 'none'; style-src 'unsafe-inline'; form-action "+origin+"; base-uri 'none'; frame-ancestors 'none'",html:'<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>河豚 · CMS 收银台</title><style>body{background:#10151e;color:#f5f5f5;font:16px system-ui;padding:40px;max-width:600px;margin:auto}button{background:#dec799;border:0;border-radius:8px;padding:16px 24px;font-size:16px}p{line-height:1.8;color:#bbc3d0}</style><h1>继续到支付平台</h1><p>支付金额和签名由苹果 CMS 生成。完成付款后请返回河豚会员中心查询订单。</p><form action="'+escape(p.action)+'" method="'+escape(p.method)+'">'+p.fields.map(f=>'<input type="hidden" name="'+escape(f.name)+'" value="'+escape(f.value)+'">').join('')+'<button type="submit">前往付款</button></form></html>'};}
