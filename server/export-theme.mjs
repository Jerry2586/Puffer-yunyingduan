import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);const {makeZip}=require('./archive.cjs');
export async function exportTheme(template,assetDir){
 const files=new Map(),mapped=new Map();
 async function walk(value){if(typeof value==='string'&&value.startsWith('/api/assets/')){const name=value.slice('/api/assets/'.length);if(!/^[a-f0-9]{64}\.(png|jpg|webp|mp4)$/.test(name))throw Error('素材路径无效');if(!mapped.has(name)){const bytes=await fs.readFile(path.join(assetDir,name));const target='assets/'+name;files.set(target,bytes);mapped.set(name,target);}return mapped.get(name);}if(Array.isArray(value))return Promise.all(value.map(walk));if(value&&typeof value==='object'){const pairs=await Promise.all(Object.entries(value).map(async([k,v])=>[k,await walk(v)]));return Object.fromEntries(pairs);}return value;}
 const theme=await walk(template.draft);files.set('manifest.json',Buffer.from(JSON.stringify({format:'hetun-skeleton',version:2,renderer:'hetun-v2',id:template.id,name:template.name,platform:template.platform,theme},null,2)));const bytes=makeZip(files);if(files.size>80||bytes.length>12*1024*1024)throw Error('导出包超出模板导入限制，请减少素材');return bytes;
}
