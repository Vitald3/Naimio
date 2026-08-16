export function stableIndex(value:string,count:number){let h=0;for(let i=0;i<value.length;i++)h=((h<<5)-h+value.charCodeAt(i))|0;return Math.abs(h)%count}
export function avatarFor(value:string){return `/media/avatars/avatar-${String(stableIndex(value,12)+1).padStart(2,"0")}.svg`}
export function coverFor(value:string,type?:string){const bias=type==="EDUCATION"?4:type==="MENTORING"?5:type==="CONSULTATION"?6:0;return `/media/covers/cover-${String(((stableIndex(value,8)+bias)%8)+1).padStart(2,"0")}.svg`}
export function initials(name:string){return name.split(/\s+/).filter(Boolean).slice(0,2).map(p=>p[0]).join("").toUpperCase()}
