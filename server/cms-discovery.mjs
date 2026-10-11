import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {cmsEndpoint} from './cms.mjs';
const within=(root,file)=>file===root||file.startsWith(root+path.sep);
// Read literal public site metadata only. Never execute PHP or return database credentials.
export function publicSiteMetadata(text){const pick=key=>new RegExp("['\"]"+key+"['\"]\\s*=>\\s*['\"]([^'\"\\r\\n]{0,300})['\"]").exec(text)?.[1]||'';return {name:pick('site_name'),url:pick('site_url'),installDir:pick('install_dir')};}
export async function discoverCms({roots,check,explicitBase='',maxSites=16}){
 const candidates=[],warnings=[],seen=new Set();let visited=0;
 async function add(root){const real=await fs.realpath(root);if(seen.has(real))return;const config=path.join(real,'application/extra/maccms.php'),api=path.join(real,'api.php');const realConfig=await fs.realpath(config);if(!within(real,realConfig))return;await fs.access(api);const stat=await fs.stat(realConfig);if(stat.size>1024*1024)return;const metadata=publicSiteMetadata(await fs.readFile(realConfig,'utf8'));seen.add(real);let base='';try{if(metadata.url){const address=/^https?:\/\//.test(metadata.url)?metadata.url:'https://'+metadata.url;const u=new URL(address);if(metadata.installDir&&metadata.installDir!=='/'&&u.pathname==='/')u.pathname=metadata.installDir;base=cmsEndpoint(u.toString()).toString();}}catch{}candidates.push({id:createHash('sha256').update(real).digest('hex').slice(0,20),root:real,name:metadata.name||path.basename(real),base,adapterInstalled:await fs.access(path.join(real,'application/index/controller/Hetun.php')).then(()=>true,()=>false)});}
 async function walk(dir,depth,boundary){if(++visited>200||candidates.length>=maxSites)return;try{const real=await fs.realpath(dir);if(!within(boundary,real))return;try{await add(real);return;}catch(e){if(!['ENOENT','ENOTDIR'].includes(e.code))warnings.push('部分站点配置不可读，请检查服务用户权限。');}if(depth===0)return;for(const entry of await fs.readdir(real,{withFileTypes:true})){if(entry.isDirectory()&&!entry.name.startsWith('.')&&!['node_modules','vendor','runtime','uploads','cache'].includes(entry.name))await walk(path.join(real,entry.name),depth-1,boundary);}}catch(e){if(e.code!=='ENOENT')warnings.push('扫描目录不可读：'+dir);}}
 for(const root of roots.slice(0,8)){try{const real=await fs.realpath(root);await walk(real,2,real);}catch(e){if(e.code!=='ENOENT')warnings.push('扫描目录不可读：'+root);}}
 if(explicitBase){const base=cmsEndpoint(explicitBase).toString();if(!candidates.some(v=>v.base===base))candidates.unshift({id:'configured-address',root:'',name:'安装时指定的 CMS',base,adapterInstalled:false});}
 // Limit simultaneous probes; large hosts do not open an unbounded number of sockets.
 for(let i=0;i<candidates.length;i+=4)await Promise.all(candidates.slice(i,i+4).map(async c=>{c.report=c.base?await check(c.base):null;}));
 const usable=candidates.filter(c=>c.report?.ok);return {candidates,warnings:[...new Set(warnings)],recommendedId:usable.length===1?usable[0].id:'',multiple:usable.length>1,checkedAt:new Date().toISOString()};
}
