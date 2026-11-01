package main

import (
	"math"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

func menuDestination(row M) error {
	kind, explicit := row["targetType"]
	if !explicit {
		kind = "system"
		if str(row["pageId"]) != "" {
			kind = "page"
		}
	}
	if !has([]string{"system", "page", "external"}, str(kind)) {
		return problem(400, "菜单打开目标无效")
	}
	row["targetType"] = kind
	if kind == "external" {
		address, ok := row["url"].(string)
		if !ok || len(address) == 0 || len(address) > 2048 || strings.ContainsAny(address, "\\") || strings.IndexFunc(address, unicode.IsSpace) >= 0 {
			return problem(400, "请填写完整的 http:// 或 https:// 网页地址，最长2048字节")
		}
		parsed, err := url.Parse(address)
		if err != nil || !has([]string{"http", "https"}, parsed.Scheme) || parsed.Hostname() == "" || parsed.User != nil || !strings.HasPrefix(strings.ToLower(address), parsed.Scheme+"://") {
			return problem(400, "菜单网页地址须为有效的 http:// 或 https:// 地址")
		}
		if port := parsed.Port(); port != "" {
			value, err := strconv.Atoi(port)
			if err != nil || value > 65535 {
				return problem(400, "菜单网页地址的端口无效")
			}
		}
		if str(row["pageId"]) != "" || str(row["categoryId"]) != "" {
			return problem(400, "外部网页不能同时绑定内置页面或 CMS 分类")
		}
		row["page"] = "home"
	} else {
		if _, exists := row["url"]; exists {
			return problem(400, "系统功能和内置页面不能同时配置网页地址")
		}
		if kind == "page" && str(row["pageId"]) == "" {
			return problem(400, "请选择已在页面编排中创建的内置页面")
		}
		if kind == "system" && str(row["pageId"]) != "" {
			return problem(400, "系统功能不能同时绑定内置页面")
		}
	}
	return nil
}

// Only display choices and stable theme slot identifiers cross the operations boundary.
// Image URLs, SVG content, font files and skeleton geometry remain theme-owned.
func navigationPresentation(row M) error {
	if value, exists := row["display"]; exists {
		mode, ok := value.(string)
		if !ok || !has([]string{"text", "icon", "both"}, mode) {
			return problem(400, "导航显示方式只能是文字、图标或图文")
		}
	}
	if value, exists := row["iconKey"]; exists {
		key, ok := value.(string)
		if !ok || (key != "" && !idPattern.MatchString(key)) {
			return problem(400, "图标位须为主题资源编号，不能填写素材地址")
		}
	}
	if value, exists := row["textColor"]; exists {
		color, ok := value.(string)
		if !ok || !navigationColor.MatchString(color) {
			return problem(400, "分类文字颜色须为 #RRGGBB 格式")
		}
	}
	return nil
}
func operationNavigation(input, out M) error {
	for _, key := range []string{"navigation", "categories", "serviceMenuItems"} {
		for _, raw := range arr(out[key]) {
			row := obj(raw)
			if err := navigationPresentation(row); err != nil {
				return err
			}
			if key == "serviceMenuItems" {
				if err := menuDestination(row); err != nil {
					return err
				}
			}
			if key == "categories" {
				row["display"] = "text"
				delete(row, "iconKey")
			}
		}
	}
	raw, exists := input["categoryShortcuts"]
	if !exists {
		return nil
	}
	groups, ok := raw.([]any)
	if !ok || len(groups) > 100 {
		return problem(400, "分类快捷导航须为数组，最多100组")
	}
	clean := []any{}
	seen := map[string]bool{}
	entries := map[string]bool{}
	sourceOwners := map[string][]string{}
	for _, rawEntry := range arr(out["categories"]) {
		entry := obj(rawEntry)
		id := str(entry["id"])
		entries[id] = true
		if source := str(entry["categoryId"]); source != "" {
			sourceOwners[source] = append(sourceOwners[source], id)
		}
	}
	explicit := map[string]bool{}
	for _, rawGroup := range groups {
		if id, ok := obj(rawGroup)["entryId"].(string); ok {
			explicit[id] = true
		}
	}
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			return problem(400, "分类快捷导航格式无效")
		}
		ownerKey := "categoryId"
		if _, exists := group["entryId"]; exists {
			ownerKey = "entryId"
			if _, mixed := group["categoryId"]; mixed {
				return problem(400, "中间导航归属不能同时使用 APP 入口和 CMS 分类")
			}
		}
		id, ok := group[ownerKey].(string)
		key := ownerKey + ":" + id
		if !ok || id == "" || len(id) > 60 || seen[key] {
			return problem(400, "分类快捷导航须绑定唯一分类编号")
		}
		seen[key] = true
		owner := id
		if ownerKey == "entryId" {
			if !entries[id] {
				return problem(400, "中间导航必须绑定已创建的 APP 一级分类")
			}
		} else {
			// Fix legacy ownership once; ambiguous or deleted owners cannot reactivate later.
			owner = ""
			if matches := sourceOwners[id]; len(matches) == 1 && !explicit[matches[0]] {
				owner = matches[0]
			}
		}
		rows, ok := group["items"].([]any)
		if !ok || len(rows) > 4 {
			return problem(400, "每个分类最多四个快捷入口")
		}
		items := []any{}
		targets := map[string]bool{}
		for _, raw := range rows {
			row, ok := raw.(map[string]any)
			if !ok {
				return problem(400, "快捷入口格式无效")
			}
			target := str(row["id"])
			if !has([]string{"all", "ranking", "schedule", "search"}, target) || targets[target] {
				return problem(400, "分类快捷入口无效或重复")
			}
			targets[target] = true
			label, ok := row["label"].(string)
			if !ok || len(label) == 0 || len(label) > 240 {
				return problem(400, "快捷入口名称须为1–240字节")
			}
			enabled, ok := row["enabled"].(bool)
			if !ok {
				return problem(400, "快捷入口开关必须为布尔值")
			}
			item := pick(row, "id", "label", "display", "iconKey")
			item["enabled"] = enabled
			if err := navigationPresentation(item); err != nil {
				return err
			}
			items = append(items, item)
		}
		validated := M{"entryId": owner, "items": items}
		if value, exists := group["enabled"]; exists {
			if _, ok := value.(bool); !ok {
				return problem(400, "分类分区开关必须为布尔值")
			}
			validated["enabled"] = value
		}
		if value, exists := group["layout"]; exists {
			layout, ok := value.(string)
			if !ok || !has([]string{"scroll", "grid"}, layout) {
				return problem(400, "分类导航排版只能是单行横滑或多行宫格")
			}
			validated["layout"] = layout
		}
		if value, exists := group["columns"]; exists {
			columns, ok := value.(float64)
			if !ok || math.IsNaN(columns) || math.IsInf(columns, 0) || columns != math.Trunc(columns) || columns < 2 || columns > 6 {
				return problem(400, "分类导航宫格列数须为2–6的整数")
			}
			validated["columns"] = columns
		}
		if value, exists := group["appearance"]; exists {
			appearance, err := operationAppearance(value, "middle")
			if err != nil {
				return err
			}
			if len(appearance) > 0 {
				validated["appearance"] = appearance
			}
		}
		if owner != "" {
			clean = append(clean, validated)
		}
	}
	out["categoryShortcuts"] = clean
	return nil
}
