import path from 'node:path';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const manifests=[
 ...(process.env.HETUN_UNI_DEPS?[path.resolve(process.env.HETUN_UNI_DEPS,'../package.json')]:[]),
 path.join(root,'package.json'),
 ...(process.env.LOCALAPPDATA?[path.join(process.env.LOCALAPPDATA,'Programs/UniAppTools/projects/hetun-app-c7cb6e60b06b/runtime/workspace/package.json')]:[])
];
const loaders=manifests.map(file=>createRequire(file));
export function dependency(name){
 let lastError;
 for(const load of loaders){try{return load(name);}catch(error){if(error.code!=='MODULE_NOT_FOUND')throw error;lastError=error;}}
 throw lastError;
}
