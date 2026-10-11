import {inflateRawSync} from 'node:zlib';
import {createHash} from 'node:crypto';
import {promises as fs} from 'node:fs';
import path from 'node:path';
import {validateTheme} from './theme.mjs';
const fail=msg=>{throw new Error(msg);};
const safeName=n=>/^(manifest\.json|assets\/[a-zA-Z0-9_-]+\.(png|jpg|webp|mp4))$/.test(n);
function crc32(b){let c=0xffffffff;for(const x of b){c^=x;for(let i=0;i<8;i++)c=(c>>>1)^((c&1)?0xedb88320:0);}return (c^0xffffffff)>>>0;}
export function readPackage(buffer){
 if(buffer.length>12*1024*1024)fail('模板包最多 12 MB。');
 if(buffer[0]!==0x50){let p;try{p=JSON.parse(buffer.toString('utf8'));}catch{fail('模板包须为 ZIP 或河豚 JSON 包。');}if(!p||typeof p!=='object'||Array.isArray(p))fail('JSON 骨架包无效。');if(Object.keys(p.assets||{}).length>79)fail('模板包文件数量超过限制。');const files=new Map([['manifest.json',Buffer.from(JSON.stringify(p))]]);for(const [n,s] of Object.entries(p.assets||{})){if(!safeName(n)||n==='manifest.json'||typeof s!=='string')fail('素材路径无效。');files.set(n,Buffer.from(s,'base64'));}return files;}
 let e=-1;for(let i=buffer.length-22;i>=Math.max(0,buffer.length-65557);i--)if(buffer.readUInt32LE(i)===0x06054b50){e=i;break;}
 if(e<0)fail('ZIP 尾部损坏。');const count=buffer.readUInt16LE(e+10),offset=buffer.readUInt32LE(e+16);if(count>80||buffer.readUInt16LE(e+4)||buffer.readUInt16LE(e+6))fail('不支持多卷 ZIP 或过多文件。');let p=offset,total=0;const files=new Map();
 for(let i=0;i<count;i++){if(p+46>e||buffer.readUInt32LE(p)!==0x02014b50)fail('ZIP 目录损坏。');const flags=buffer.readUInt16LE(p+8),method=buffer.readUInt16LE(p+10),crc=buffer.readUInt32LE(p+16),size=buffer.readUInt32LE(p+20),plain=buffer.readUInt32LE(p+24),nl=buffer.readUInt16LE(p+28),ex=buffer.readUInt16LE(p+30),co=buffer.readUInt16LE(p+32),local=buffer.readUInt32LE(p+42),mode=buffer.readUInt32LE(p+38)>>>16;
 const n=buffer.subarray(p+46,p+46+nl).toString('utf8');p+=46+nl+ex+co;if(n==='assets/')continue;
 if(!safeName(n)||files.has(n)||(mode&0xf000)===0xa000||(flags&1)||![0,8].includes(method)||plain>10*1024*1024)fail('ZIP 包含不支持的路径、文件或加密内容。');
 total+=plain;if(total>16*1024*1024)fail('解压后模板包过大。');if(local+30>buffer.length||buffer.readUInt32LE(local)!==0x04034b50)fail('ZIP 文件头损坏。');const ln=buffer.readUInt16LE(local+26),le=buffer.readUInt16LE(local+28);if(buffer.subarray(local+30,local+30+ln).toString('utf8')!==n)fail('ZIP 路径不一致。');const begin=local+30+ln+le;if(begin+size>offset)fail('ZIP 文件越界。');const bytes=buffer.subarray(begin,begin+size),out=method===8?inflateRawSync(bytes,{maxOutputLength:10*1024*1024}):bytes;if(out.length!==plain||crc32(out)!==crc)fail('ZIP 校验失败。');files.set(n,out);
 }return files;
}
export async function storeAsset(bytes,name,dir){if(!bytes.length||bytes.length>10*1024*1024)fail('素材最多 10 MB。');const ext=path.extname(name).slice(1).toLowerCase();let valid=false;if(ext==='png')valid=bytes.subarray(0,8).equals(Buffer.from([137,80,78,71,13,10,26,10]));if(ext==='jpg')valid=bytes[0]===255&&bytes[1]===216&&bytes[2]===255;if(ext==='webp')valid=bytes.toString('ascii',0,4)==='RIFF'&&bytes.toString('ascii',8,12)==='WEBP';if(ext==='mp4')valid=bytes.toString('ascii',4,8)==='ftyp';if(!valid)fail('素材内容与扩展名不符，仅支持 PNG/JPG/WebP/MP4。');const file=createHash('sha256').update(bytes).digest('hex')+'.'+ext;await fs.mkdir(dir,{recursive:true});await fs.writeFile(path.join(dir,file),bytes,{flag:'wx'}).catch(e=>{if(e.code!=='EEXIST')throw e;});return '/api/assets/'+file;}
export async function installPackage(input,assetDir,authorize=async()=>{}){const files=readPackage(Buffer.from(String(input.data||''),'base64'));if(!files.has('manifest.json'))fail('包中缺少 manifest.json。');let manifest;try{manifest=JSON.parse(files.get('manifest.json').toString('utf8'));}catch{fail('manifest.json 无效。');}
 if(manifest.format!=='hetun-skeleton'||manifest.version!==2||manifest.renderer!=='hetun-v2'||!['mobile','tablet','tv','web'].includes(manifest.platform)||!manifest.theme)fail('骨架包格式、版本或渲染器不兼容。');if(!/^[a-z0-9][a-z0-9-]{2,60}$/.test(manifest.id||''))fail('骨架 ID 无效。');
 await authorize(manifest.id);
 const map={};for(const [n,b]of files)if(n!=='manifest.json')map[n]=await storeAsset(b,n,assetDir);
 function resolve(v){if(typeof v==='string'&&v.startsWith('assets/')){if(!map[v])fail('骨架缺少素材：'+v);return map[v];}if(Array.isArray(v))return v.map(resolve);if(v&&typeof v==='object')return Object.fromEntries(Object.entries(v).filter(([k])=>!['__proto__','constructor','prototype'].includes(k)).map(([k,x])=>[k,resolve(x)]));return v;}
 const theme=validateTheme(resolve(manifest.theme)),name=String(input.name||manifest.name||'').trim().slice(0,40);if(!name)fail('请输入骨架名称。');return {id:manifest.id,name,platform:manifest.platform,renderer:'hetun-v2',draft:theme,published:null,version:1,publishedVersion:0,installedAt:new Date().toISOString()};
}
