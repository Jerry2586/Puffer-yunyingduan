import {migrateCategoryShortcutOwners} from './operation-pages';
import {normalizeBottomNavigation} from './navigation-modules';
import defaults from './default-theme.json';import type {Theme} from './types';
export function normalizeTheme<T extends Theme>(theme:T):T {
 const memberCard=theme.features?.memberCard??theme.features?.member??true;
 theme.startup={...defaults.startup,...theme.startup};theme.clientSettings={...defaults.clientSettings,...theme.clientSettings};
 theme.features={...defaults.features,...theme.features,memberCard};theme.announcement={...defaults.announcement,...theme.announcement} as Theme['announcement'];
 theme.contentPages??=[];theme.navigationStyle??={};migrateCategoryShortcutOwners(theme);
 if(import.meta.env.VITE_HETUN_OPERATIONS_CENTER==='1')normalizeBottomNavigation(theme);
 theme.pageStyles={...defaults.pageStyles,...theme.pageStyles} as Theme['pageStyles'];return theme;
}
