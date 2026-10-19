import fs from 'node:fs/promises';
import path from 'node:path';
import {pathToFileURL,domainToASCII} from 'node:url';

const safePath=value=>{if(typeof value!=='string'||!/^\/[A-Za-z0-9_./-]+$/.test(value)||value.split('/').includes('..'))throw Error('服务器路径必须是无空格的绝对路径，不允许 ..');return value;};
export function deploymentConfig(input){
 const host=domainToASCII(String(input.domain||'').toLowerCase());
 if(!/^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(host))throw Error('请输入运营中心的独立域名，不带协议、端口和路径');
 const port=Number(input.port||8788);if(!Number.isInteger(port)||port<1024||port>65535)throw Error('API端口必须是1024–65535');
 const code=safePath(input.code),state=safePath(input.state||'/var/lib/hetun-operations'),config=safePath(input.config||'/etc/hetun-operations');
 for(const [a,b] of [[code,state],[code,config],[state,config]])if(a===b||a.startsWith(b+'/')||b.startsWith(a+'/'))throw Error('程序、运行数据和配置目录必须独立，不能相互嵌套');
 const node=safePath(input.node||'/usr/bin/node'),cert=safePath(input.cert),key=safePath(input.key),user=String(input.user||'hetun-ops');
 if(!/^[a-z_][a-z0-9_-]{0,31}$/.test(user))throw Error('服务用户格式无效');
 const cmsRoots=String(input.cmsRoots||'/www/wwwroot:/var/www').split(':').map(safePath);
 let cmsBase='';if(input.cmsBase){const u=new URL(input.cmsBase);if(!['http:','https:'].includes(u.protocol)||u.username||u.password||u.hash||u.search)throw Error('CMS地址格式无效');cmsBase=u.toString();}
 const allowPrivate=input.allowPrivate==='1'?'1':'0';if(input.allowPrivate&&!['0','1'].includes(input.allowPrivate))throw Error('allowPrivate只能是0或1');
 const clientOrigins=String(input.clientOrigins||'').split(',').filter(Boolean).map(v=>{const u=new URL(v);if(u.protocol!=='https:'||u.origin!==v)throw Error('网页客户端来源必须是完整HTTPS origin');return v;});
 const env=['HETUN_HOST=127.0.0.1','HETUN_PORT='+port,'HETUN_STATE_DIR='+state,'HETUN_PRODUCT_MODE=operations-center','HETUN_PUBLIC_ORIGIN=https://'+host,'HETUN_ALLOW_PRIVATE_CMS='+allowPrivate,'HETUN_CMS_ROOTS='+cmsRoots.join(':'),'HETUN_CMS_BASE='+cmsBase,'HETUN_CLIENT_ORIGINS='+clientOrigins.join(',')].join('\n')+'\n';
 const service='[Unit]\nDescription=Hetun Operations Center\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nUser='+user+'\nGroup='+user+'\nWorkingDirectory='+code+'\nEnvironmentFile='+config+'/operations.env\nExecStart='+node+' '+code+'/server/index.mjs\nRestart=on-failure\nRestartSec=5\nUMask=0077\nNoNewPrivileges=true\nPrivateTmp=true\nProtectSystem=strict\nProtectHome=true\nReadWritePaths='+state+'\n\n[Install]\nWantedBy=multi-user.target\n';
 const nginx='server {\n    listen 80;\n    server_name '+host+';\n    return 301 https://'+host+'$request_uri;\n}\n\nserver {\n    listen 443 ssl;\n    server_name '+host+';\n    ssl_certificate '+cert+';\n    ssl_certificate_key '+key+';\n    root '+code+'/frontend;\n    index index.html;\n    client_max_body_size 24m;\n    add_header X-Content-Type-Options nosniff always;\n    add_header Referrer-Policy same-origin always;\n    add_header X-Frame-Options SAMEORIGIN always;\n\n    location /api/ {\n        proxy_pass http://127.0.0.1:'+port+';\n        proxy_set_header Host $host;\n        proxy_set_header X-Forwarded-Proto $scheme;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_read_timeout 90s;\n    }\n    location = /index.html {\n        add_header Cache-Control "no-store";\n    }\n    location /assets/ {\n        try_files $uri =404;\n        expires 1y;\n        add_header Cache-Control "public, immutable";\n    }\n    location / {\n        try_files $uri $uri/ /index.html;\n    }\n}\n';
 return {env,service,nginx,domain:host,port,code,state,config,user};
}
export async function prepare(input){const result=deploymentConfig(input);await fs.mkdir(result.config,{recursive:true,mode:0o750});for(const [name,text]of [['operations.env',result.env],['hetun-operations.service',result.service],['nginx.conf',result.nginx]])await fs.writeFile(path.join(result.config,name),text,{flag:'wx',mode:0o640});return result;}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href){
 const input={};const args=process.argv.slice(2);for(let i=0;i<args.length;i+=2){const flag=args[i];if(!/^--[A-Za-z]+$/.test(flag)||!args[i+1])throw Error('参数格式：--domain ops.example.com --code /opt/hetun-operations/releases/0.2.0 --cert /path/fullchain.pem --key /path/privkey.pem');const field=flag.slice(2);if(!['domain','code','state','config','node','cert','key','user','port','cmsRoots','cmsBase','allowPrivate','clientOrigins'].includes(field))throw Error('未知参数：'+flag);input[field]=args[i+1];}
 prepare(input).then(r=>console.log('已生成独立站点配置：'+r.config+'；请核对后安装systemd服务及Nginx站点。')).catch(e=>{console.error(e.message);process.exitCode=1;});
}
