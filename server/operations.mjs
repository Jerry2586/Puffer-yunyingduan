import {defaultTheme, validateTheme} from './theme.mjs';
export const platforms=['mobile','tablet','tv','web'];
// Only these fields travel from the operations server to a packaged client.
export const operationKeys=['sections','features','navigation','categories','ads','announcement','clientSettings','schedule'];
export function operationConfig(theme){const valid=validateTheme(theme);return {...Object.fromEntries(operationKeys.map(k=>[k,valid[k]])),carousel:valid.carousel};}
export function mergeOperations(config,base=defaultTheme){return validateTheme({...structuredClone(base),...Object.fromEntries(operationKeys.filter(k=>Object.hasOwn(config,k)).map(k=>[k,config[k]])),carousel:{...base.carousel,...config.carousel}});}
export function initializeOperations(state){
 if(state.operations)return false;
 state.operations=Object.fromEntries(platforms.map(p=>{const legacy=state.templates?.find(t=>t.id===state.activeTemplates?.[p]);return [p,{version:1,publishedVersion:1,draft:operationConfig(legacy?.draft||defaultTheme),published:operationConfig(legacy?.published||defaultTheme),publishedAt:new Date().toISOString()}];}));return true;
}
export function platformFor(id='runtime-mobile'){const p=id.replace(/^runtime-/,'');if(!platforms.includes(p)){const e=Error('请选择手机、平板、电视或网页端。');e.status=400;throw e;}return p;}
export function operationsStudio(state,id){const p=platformFor(id||'runtime-mobile'),names={mobile:'手机',tablet:'平板',tv:'电视',web:'网页'};const templates=platforms.map(platform=>{const v=state.operations[platform];return {id:'runtime-'+platform,name:names[platform]+'端运营配置',platform,version:v.version,publishedVersion:v.publishedVersion,draft:mergeOperations(v.draft),published:mergeOperations(v.published)};});const t=templates.find(t=>t.platform===p);return {...t,templates,selectedTemplate:t.id,activeTemplates:Object.fromEntries(platforms.map(p=>[p,'runtime-'+p])),cmsBase:state.cmsBase,updatedAt:state.updatedAt,publishedAt:state.operations[p].publishedAt};}
export function saveOperations(state,input){const p=platformFor(input.template||'runtime-mobile'),record=state.operations[p];if(input.version!==record.version){const e=Error('此端运营配置已被其他窗口修改，请重新加载。');e.status=409;throw e;}record.draft=operationConfig(input.theme);record.version++;state.updatedAt=new Date().toISOString();return state;}
export function publishOperations(state,input){const p=platformFor(input.template||'runtime-mobile'),record=state.operations[p];if(input.version!==record.version){const e=Error('配置版本已变化，请重新加载后发布。');e.status=409;throw e;}record.published=structuredClone(record.draft);record.publishedVersion++;record.publishedAt=new Date().toISOString();return state;}
