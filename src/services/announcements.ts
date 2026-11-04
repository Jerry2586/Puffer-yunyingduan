import {request} from './api';
import type {Announcement} from '@/domain/types';

export interface AnnouncementEntry {
  id:string;
  config:Announcement;
  sources?:string[]|null;
  createdAt:string;
  updatedAt:string;
}
export interface AnnouncementState {
  version:number;
  updatedAt:string;
  items:AnnouncementEntry[];
}
export type AnnouncementMutation =
  | {action:'create';config:Announcement}
  | {action:'update';id:string;config:Announcement}
  | {action:'toggle';id:string;enabled:boolean}
  | {action:'delete';id:string};

export const announcementsApi = {
  list:()=>request<AnnouncementState>('/api/admin/announcements'),
  change:(version:number,mutation:AnnouncementMutation)=>request<AnnouncementState>('/api/admin/announcements','POST',{version,...mutation}),
};
