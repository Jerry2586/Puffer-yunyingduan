package main

import "strings"

// Carousel content is operations-owned; geometry and artwork styling stay in the theme.
// Entry IDs refer to APP primary navigation identities, never CMS source category IDs.
func operationCarousels(input, out M) error {
	raw, exists := input["carouselGroups"]
	if !exists {
		return nil
	}
	groups, ok := raw.([]any)
	if !ok || len(groups) > 40 {
		return problem(400, "轮播组须为数组，最多40组")
	}
	entries, pageIDs := map[string]bool{}, map[string]bool{}
	for _, raw := range arr(out["categories"]) {
		entries[str(obj(raw)["id"])] = true
	}
	for _, raw := range arr(out["contentPages"]) {
		pageIDs[str(obj(raw)["id"])] = true
	}
	seen, owners := map[string]bool{}, map[string]bool{}
	clean, total := []any{}, 0
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			return problem(400, "轮播组格式无效")
		}
		id := str(group["id"])
		if !idPattern.MatchString(id) || seen[id] {
			return problem(400, "轮播组编号无效或重复")
		}
		seen[id] = true
		name, ok := group["name"].(string)
		if !ok || strings.TrimSpace(name) == "" || len(name) > 240 {
			return problem(400, "轮播组名称须为1–240字节")
		}
		enabled, ok := group["enabled"].(bool)
		if !ok {
			return problem(400, "轮播组开关必须为布尔值")
		}
		bindings, ok := group["entryIds"].([]any)
		if !ok || len(bindings) > 1 {
			return problem(400, "每个轮播组最多绑定一个 APP 一级分类")
		}
		ids := []any{}
		for _, rawID := range bindings {
			entry, ok := rawID.(string)
			if !ok || !entries[entry] || owners[entry] {
				return problem(400, "轮播须绑定已创建的 APP 一级分类，一个分类只能绑定一组")
			}
			owners[entry] = true
			ids = append(ids, entry)
		}
		rows, ok := group["slides"].([]any)
		if !ok || len(rows) > 20 {
			return problem(400, "每个轮播组最多20条")
		}
		total += len(rows)
		if total > 200 {
			return problem(400, "轮播内容合计最多200条")
		}
		slides, slideIDs := []any{}, map[string]bool{}
		for _, rawSlide := range rows {
			row, ok := rawSlide.(map[string]any)
			if !ok {
				return problem(400, "轮播内容格式无效")
			}
			slideID := str(row["id"])
			if !idPattern.MatchString(slideID) || slideIDs[slideID] {
				return problem(400, "轮播内容编号无效或重复")
			}
			slideIDs[slideID] = true
			on, ok := row["enabled"].(bool)
			if !ok {
				return problem(400, "轮播内容开关必须为布尔值")
			}
			title, ok := row["title"].(string)
			if !ok || len(title) > 240 {
				return problem(400, "轮播标题最多240字节")
			}
			image, ok := row["image"].(string)
			if !ok || len(image) > 2048 {
				return problem(400, "轮播图片格式无效")
			}
			if image != "" {
				if strings.HasPrefix(image, "/api/assets/") {
					if !assetPattern.MatchString(image) || strings.HasSuffix(image, ".mp4") {
						return problem(400, "轮播须使用 PNG、JPG 或 WebP 图片")
					}
				} else if menuDestination(M{"targetType": "external", "url": image}) != nil {
					return problem(400, "轮播图片须为有效的 HTTP(S) 地址或已上传图片")
				}
			}
			source, ok := row["sourceVodId"].(string)
			if !ok || (source != "" && (!digits.MatchString(source) || len(source) > 20)) {
				return problem(400, "展示影片 ID 须为最多20位数字")
			}
			target, err := carouselDestination(row["target"], enabled && on, pageIDs)
			if err != nil {
				return err
			}
			if enabled && on && image == "" {
				return problem(400, "开启轮播前，请为每条启用内容选择图片")
			}
			slides = append(slides, M{"id": slideID, "title": title, "image": image, "sourceVodId": source, "enabled": on, "target": target})
		}
		if enabled && len(ids) == 0 {
			return problem(400, "开启轮播组前，请绑定一级分类")
		}
		clean = append(clean, M{"id": id, "name": name, "enabled": enabled, "entryIds": ids, "slides": slides})
	}
	out["carouselGroups"] = clean
	return nil
}

func carouselDestination(raw any, required bool, pageIDs map[string]bool) (M, error) {
	target, ok := raw.(map[string]any)
	if !ok {
		return nil, problem(400, "请选择轮播点击目标")
	}
	kind := str(target["type"])
	keys := map[string]string{"vod": "vodId", "page": "pageId", "external": "url", "system": "page"}
	key, valid := keys[kind]
	if !valid {
		return nil, problem(400, "轮播目标只支持影片、内置页面、外部网页或 APP 功能")
	}
	value, ok := target[key].(string)
	if !ok {
		return nil, problem(400, "轮播目标格式无效")
	}
	for _, other := range keys {
		if other != key {
			if _, exists := target[other]; exists {
				return nil, problem(400, "一条轮播只能设置一个点击目标")
			}
		}
	}
	if value == "" && !required {
		return M{"type": kind, key: value}, nil
	}
	switch kind {
	case "vod":
		if value == "" || !digits.MatchString(value) || len(value) > 20 {
			return nil, problem(400, "请选择点击后打开的影片，ID 须为最多20位数字")
		}
	case "page":
		if !pageIDs[value] {
			return nil, problem(400, "轮播绑定页面不存在，请先创建或重新选择页面")
		}
	case "external":
		if err := menuDestination(M{"targetType": "external", "url": value}); err != nil {
			return nil, err
		}
	case "system":
		if !has(pages, value) {
			return nil, problem(400, "轮播 APP 功能无效")
		}
	}
	return M{"type": kind, key: value}, nil
}
