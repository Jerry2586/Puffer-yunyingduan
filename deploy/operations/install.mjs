import fs from 'node:fs/promises';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {deploymentConfig,prepare} from './configure.mjs';
import {pathToFileURL} from 'node:url';
export async function verifyBundle(code){
 const manifest=JSON.parse(await fs.readFile(path.join(code,'release-manifest.json'),'utf8'));
 if(manifest.product!=='hetun-operations'||manifest.mode!=='operations-center')throw Error('发布包类型错误');
 for(const [name,expected]of Object.entries(manifest.files)){const target=path.resolve(code,name);if(!target.startsWith(path.resolve(code)+path.sep))throw Error('非法发布文件路径');const real=await fs.realpath(target);if(!real.startsWith(await fs.realpath(code)+path.sep))throw Error('发布文件不能指向程序目录外');if(createHash('sha256').update(await fs.readFile(real)).digest('hex')!==expected)throw Error('发布文件校验失败：'+name);}
 return manifest;
}
export async function install(input,{apply=false,nginxDir='/etc/nginx/conf.d',nginx='/usr/sbin/nginx'}={}){
 const config=deploymentConfig(input);await verifyBundle(config.code);
 if(!/^\/[A-Za-z0-9_./-]+$/.test(nginxDir)||nginxDir.split('/').includes('..')||!/^\/[A-Za-z0-9_./-]+$/.test(nginx))throw Error('Nginx路径必须为安全绝对路径');
 const targets=[path.join(config.config,'operations.env'),'/etc/systemd/system/hetun-operations.service',path.join(nginxDir,'hetun-operations.conf')];
 for(const file of targets){try{await fs.lstat(file);throw Error('已有安装文件，未覆盖：'+file);}catch(e){if(e.code!=='ENOENT')throw e;}}
 const summary={domain:config.domain,code:config.code,state:config.state,config:config.config,nginxSite:targets[2],apply};if(!apply)return summary;
 if(process.platform!=='linux'||process.getuid?.()!==0)throw Error('正式安装需要Linux root权限');
 const run=(command,args)=>{const r=spawnSync(command,args,{encoding:'utf8',windowsHide:true});if(r.error||r.status!==0)throw Error(command+'执行失败：'+(r.stderr||r.error?.message||''));return r.stdout;};
 await fs.access(input.cert);await fs.access(input.key);await fs.access(nginxDir);
 const nodeVersion=run(input.node||'/usr/bin/node',['--version']);if(!/^v22\./.test(nodeVersion))throw Error('需要Node.js 22.x');run(nginx,['-t']);
 if(spawnSync('id',['-u',config.user],{stdio:'ignore'}).status!==0)run('useradd',['--system','--no-create-home','--shell','/usr/sbin/nologin',config.user]);
 await fs.mkdir(config.state,{recursive:true,mode:0o700});await fs.mkdir(config.config,{recursive:true,mode:0o750});run('chown',[config.user+':'+config.user,config.state]);run('chown',['root:'+config.user,config.config]);
 await prepare(input);for(const name of ['operations.env','hetun-operations.service','nginx.conf'])run('chown',['root:'+config.user,path.join(config.config,name)]);
 await fs.copyFile(path.join(config.config,'hetun-operations.service'),targets[1],fs.constants.COPYFILE_EXCL);
 await fs.copyFile(path.join(config.config,'nginx.conf'),targets[2],fs.constants.COPYFILE_EXCL);
 try{run(nginx,['-t']);}catch(e){await fs.unlink(targets[2]);await fs.unlink(targets[1]);throw Error(e.message+'；已撤下本次新增站点和服务文件，配置目录保留供检查');}
 run('systemctl',['daemon-reload']);run('systemctl',['enable','--now','hetun-operations']);run(nginx,['-s','reload']);
 return {...summary,installed:true,credentialFile:path.join(config.state,'admin-credentials.json')};
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href){
 const input={},options={},args=process.argv.slice(2),allowed=['domain','code','state','config','node','cert','key','user','port','license','publicKey','cmsRoots','cmsBase','allowPrivate','clientOrigins'];
 for(let i=0;i<args.length;i++){const flag=args[i];if(flag==='--apply'){options.apply=true;continue;}const key=flag.replace(/^--/,'');if(!args[i+1])throw Error('参数缺少值：'+flag);if(['nginxDir','nginx'].includes(key))options[key]=args[++i];else if(allowed.includes(key))input[key]=args[++i];else throw Error('未知参数：'+flag);}
 install(input,options).then(v=>console.log(JSON.stringify(v,null,2))).catch(e=>{console.error(e.message);process.exitCode=1;});
}
