// Anchor to the server's checkedAt so a client clock offset does not alter installation age.
export function installationSeconds(installedAt:string|undefined,checkedAt:string|undefined,elapsedMs=0):number|null{
 const start=Date.parse(installedAt||''),checked=Date.parse(checkedAt||'');
 if(!Number.isFinite(start)||!Number.isFinite(checked)||!Number.isFinite(elapsedMs))return null;
 return Math.max(0,Math.floor((checked-start+Math.max(0,elapsedMs))/1000));
}
export function installationDuration(seconds:number|null):string{
 if(seconds===null)return '安装时间未记录';
 const s=Math.max(0,Math.floor(seconds)),days=Math.floor(s/86400),hours=Math.floor(s%86400/3600),minutes=Math.floor(s%3600/60);
 return days+'天 '+String(hours).padStart(2,'0')+':'+String(minutes).padStart(2,'0')+':'+String(s%60).padStart(2,'0');
}
