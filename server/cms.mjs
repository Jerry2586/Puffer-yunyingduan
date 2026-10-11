import http from 'node:http';
import https from 'node:https';
import { lookup } from 'node:dns/promises';
import { isIP } from 'node:net';
export function cmsEndpoint(value) {
  let url;
  try { url = new URL(value); } catch { throw new Error('请输入完整的 http:// 或 https:// 苹果 CMS 地址。'); }
  if (!['http:','https:'].includes(url.protocol) || url.username || url.password || url.hash) throw new Error('CMS 地址格式不支持。');
  if (!url.pathname.includes('/api.php/provide/vod')) {
    url.pathname = url.pathname.replace(/\/$/, '') + '/api.php/provide/vod/';
  }
  url.search = '';
  return url;
}
export function privateAddress(address) {
  const a = address.toLowerCase();
  if (isIP(a) === 4) {
    const p = a.split('.').map(Number);
    return p[0] === 0 || p[0] === 10 || p[0] === 127 || p[0] >= 224 || (p[0] === 169 && p[1] === 254) || (p[0] === 172 && p[1] >= 16 && p[1] <= 31) || (p[0] === 192 && p[1] === 168) || (p[0] === 100 && p[1] >= 64 && p[1] <= 127);
  }
  // Only global-unicast IPv6 addresses. Mapped IPv4 must pass its own checks.
  if (a.startsWith('::ffff:')) return true;
  return !/^[23][0-9a-f]{3}:/.test(a);
}
export async function requestCms(base, params, allowPrivate = false) {
  if (!base) throw new Error('尚未配置苹果 CMS。请在主题工作台的“内容连接”中设置地址。');
  const url = cmsEndpoint(base);
  for (const [key,value] of Object.entries(params)) if (value !== '' && value != null) url.searchParams.set(key,String(value));
  const host = url.hostname.replace(/^\[|\]$/g, '');
  const addresses = isIP(host) ? [{address:host,family:isIP(host)}] : await lookup(host,{all:true});
  if (!addresses.length || (!allowPrivate && addresses.some(a => privateAddress(a.address)))) throw new Error('CMS 地址指向本机或内网，默认禁止。内网部署需由环境管理员明确启用。');
  const selected = addresses.find(a=>a.family===4) || addresses[0];
  return new Promise((resolve,reject) => {
    const req = (url.protocol === 'https:' ? https : http).get(url, {
      headers:{'Accept':'application/json','User-Agent':'Hetun/0.1'},
      autoSelectFamily:false,
      lookup:(_host,options,callback) => options?.all ? callback(null,[selected]) : callback(null,selected.address,selected.family),
      timeout:10000
    }, response => {
      if (response.statusCode !== 200) { response.resume(); reject(new Error('CMS 返回 HTTP ' + response.statusCode + '。请确认采集接口已启用且返回 JSON。')); return; }
      let size=0; const chunks=[];
      response.on('data',chunk => { size += chunk.length; if (size > 8*1024*1024) response.destroy(new Error('CMS 响应过大。')); else chunks.push(chunk); });
      response.on('error',reject);
      response.on('end',() => {
        try { const data=JSON.parse(Buffer.concat(chunks).toString('utf8')); if (!Array.isArray(data.list)) throw new Error('没有找到 list 数组'); resolve(data); }
        catch { reject(new Error('CMS 接口不是兼容的 JSON 影片数据，请检查 /api.php/provide/vod/。')); }
      });
    });
    req.on('timeout',() => req.destroy(new Error('CMS 连接超时，请稍后重试。')));
    req.on('error',reject);
  });
}
const clean = value => String(value || '').replace(/<[^>]*>/g,'').replace(/&nbsp;|&#160;/g,' ').replace(/&amp;/g,'&').replace(/&lt;/g,'<').replace(/&gt;/g,'>');
const imageUrl = (value,base) => {try {const url=new URL(String(value || ''),base);return value && ['http:','https:'].includes(url.protocol)?url.toString():'';} catch{return '';}};
export function normalizeVod(vod,base) {
  const groups=String(vod.vod_play_url || '').split('$$$');
  const labels=String(vod.vod_play_from || '').split('$$$');
  return {id:String(vod.vod_id),title:clean(vod.vod_name),poster:imageUrl(vod.vod_pic,base),
    category:clean(vod.type_name),categoryId:String(vod.type_id || ''),year:clean(vod.vod_year),area:clean(vod.vod_area),
    note:clean(vod.vod_remarks || vod.vod_state),score:clean(vod.vod_score),description:clean(vod.vod_content || vod.vod_blurb),
    actors:clean(vod.vod_actor),director:clean(vod.vod_director),
    protected:true,sources:groups.map((group,index) => ({name:labels[index] || '播放线路 ' + (index+1),episodes:group.split('#').filter(Boolean).map((episode,n) => {
      const split=episode.indexOf('$'); const raw=split<0?episode:episode.slice(split+1);
      return {name:split<0?'第 '+(n+1)+' 集':episode.slice(0,split),url:''};
    })})).filter(s => s.episodes.length)};
}
export async function listCms(base, query, allowPrivate) {
  const raw=await requestCms(base,{ac:'detail',pg:query.page || 1,wd:query.keyword || '',t:query.category || '',ids:query.ids || ''},allowPrivate);
  return {items:raw.list.map(vod=>normalizeVod(vod,base)),page:Number(raw.page)||1,pages:Number(raw.pagecount)||1,total:Number(raw.total)||raw.list.length};
}
export async function categoriesCms(base, allowPrivate) {
  const raw=await requestCms(base,{ac:'list'},allowPrivate);
  return (raw.class || []).map(c => ({id:String(c.type_id),name:clean(c.type_name)}));
}
