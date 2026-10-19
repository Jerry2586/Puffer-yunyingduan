import fs from 'node:fs/promises';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
import {initializeOperations,operationConfig,platforms} from '../../server/operations.mjs';
export async function migrateNodeState(stateDir,output){
 if(!path.isAbsolute(stateDir)||!path.isAbsolute(output))throw Error('状态目录和导出文件均需绝对路径');
 const source=path.resolve(import.meta.dirname,'../..'),relative=path.relative(source,path.resolve(output));if(!relative||(!path.isAbsolute(relative)&&relative!=='..'&&!relative.startsWith('..'+path.sep)))throw Error('备份文件必须在源码目录外');
 const previous=JSON.parse(await fs.readFile(path.join(stateDir,'theme-state.json'),'utf8'));
 const admin=JSON.parse(await fs.readFile(path.join(stateDir,'admin-credentials.json'),'utf8'));
 if(!/^[A-Za-z0-9_]{3,40}$/.test(admin.username)||!/^[a-f0-9]{64}$/.test(admin.hash)||!/^[a-f0-9]{32,64}$/.test(admin.salt))throw Error('旧管理员摘要格式无效');
 initializeOperations(previous);
 const operations={};for(const p of platforms){const r=previous.operations[p];if(!Number.isSafeInteger(r.version)||r.version<1||!Number.isSafeInteger(r.publishedVersion)||r.publishedVersion<1)throw Error('运营版本无效');operations[p]={version:r.version,publishedVersion:r.publishedVersion,draft:operationConfig(r.draft),published:operationConfig(r.published),publishedAt:r.publishedAt||'',history:r.history.slice(0,20).map(h=>({version:h.version,publishedAt:h.publishedAt,config:operationConfig(h.config)}))};}
 const result={cmsBase:previous.cmsBase||'',updatedAt:previous.updatedAt||new Date().toISOString(),setup:previous.setup?.completedAt?{completedAt:previous.setup.completedAt}:{},operations,admin:{username:admin.username,salt:admin.salt,hash:admin.hash,algorithm:'scrypt'},revision:1};
 await fs.mkdir(path.dirname(output),{recursive:true});await fs.writeFile(output,JSON.stringify(result,null,2),{flag:'wx',mode:0o600});return {output,platforms:platforms.length};
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href){const args=process.argv.slice(2);if(args.length!==2)throw Error('用法：node deploy/go/migrate-node-state.mjs <旧状态绝对目录> <新备份绝对路径>');console.log(JSON.stringify(await migrateNodeState(...args)));}
