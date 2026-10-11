import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {pathToFileURL} from 'node:url';
const hash=b=>createHash('sha256').update(b).digest('hex');
export async function installAdapter(cmsRoot,source){
 const root=await fs.realpath(cmsRoot),controller=await fs.realpath(path.join(root,'application/index/controller'));
 if(!controller.startsWith(root+path.sep))throw Error('CMS控制器目录不能指向站点外');
 await fs.access(path.join(root,'api.php'));await fs.access(path.join(root,'application/extra/maccms.php'));
 const bytes=await fs.readFile(source),target=path.join(controller,'Hetun.php');
 try{const previous=await fs.lstat(target);if(previous.isSymbolicLink())throw Error('拒绝覆盖符号链接');const existing=await fs.readFile(target);if(hash(existing)===hash(bytes))return {installed:false,unchanged:true,target,sha256:hash(bytes)};throw Error('已存在不同版本的Hetun.php；请先备份并人工核对，未覆盖');}catch(e){if(e.code!=='ENOENT')throw e;}
 await fs.writeFile(target,bytes,{flag:'wx',mode:0o644});return {installed:true,target,sha256:hash(bytes)};
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href){
 const args=process.argv.slice(2);if(args.length!==2||args[0]!=='--cmsRoot')throw Error('用法：node install-cms-adapter.mjs --cmsRoot /www/wwwroot/cms-site');
 const base=import.meta.dirname;const source=await fs.access(path.join(base,'server/maccms-extension/Hetun.php')).then(()=>path.join(base,'server/maccms-extension/Hetun.php'),()=>path.resolve(base,'../../server/maccms-extension/Hetun.php'));
 console.log(JSON.stringify(await installAdapter(args[1],source),null,2));
}
