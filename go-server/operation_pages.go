package main

import (
	"math"
	"regexp"
)

var navigationColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// operationPages extends content configuration without accepting executable pages or theme assets.
func operationPages(input, out M) error {
	rawPages, exists := input["contentPages"]
	if !exists {
		rawPages = []any{}
	}
	rows, ok := rawPages.([]any)
	if !ok || len(rows) > 40 {
		return problem(400, "独立页面必须为数组，最多40个")
	}
	clean := []any{}
	byID := map[string]M{}
	for _, raw := range rows {
		page, ok := raw.(map[string]any)
		if !ok {
			return problem(400, "独立页面格式无效")
		}
		id := str(page["id"])
		if !idPattern.MatchString(id) || byID[id] != nil {
			return problem(400, "独立页面编号无效或重复")
		}
		title, ok := page["title"].(string)
		if !ok || len(title) == 0 || len(title) > 240 {
			return problem(400, "页面名称须为1–240字节")
		}
		kind := str(page["kind"])
		if !has([]string{"category", "campaign", "blank", "custom"}, kind) {
			return problem(400, "独立页面类型无效")
		}
		n := pick(page, "id", "title", "kind", "categoryId", "enabled", "carouselEnabled")
		if _, exists := n["categoryId"]; !exists {
			n["categoryId"] = ""
		}
		if category, ok := n["categoryId"].(string); !ok || len(category) > 60 {
			return problem(400, "页面分类绑定格式无效")
		}
		for _, key := range []string{"enabled", "carouselEnabled"} {
			if _, exists := n[key]; !exists {
				n[key] = true
			}
			if _, ok := n[key].(bool); !ok {
				return problem(400, "页面开关必须为布尔值")
			}
		}
		if kind != "category" && str(n["categoryId"]) != "" {
			return problem(400, "只有分类页面可以绑定主分类")
		}
		byID[id] = n
		clean = append(clean, n)
	}
	out["contentPages"] = clean
	for _, key := range []string{"navigation", "categories", "serviceMenuItems", "sections", "carousel"} {
		entries := arr(out[key])
		if key == "carousel" {
			entries = arr(obj(out[key])["slides"])
		}
		for _, raw := range entries {
			row := obj(raw)
			value, exists := row["pageId"]
			if !exists {
				continue
			}
			id, ok := value.(string)
			if !ok {
				return problem(400, "页面引用格式无效")
			}
			if id == "" {
				delete(row, "pageId")
				continue
			}
			target := byID[id]
			if target == nil {
				return problem(400, "页面引用不存在，请先解绑相关入口和内容")
			}
			if key == "navigation" || key == "categories" || key == "serviceMenuItems" || key == "sections" {
				row["page"] = "home"
				if target["kind"] == "category" {
					row["page"] = "category"
				}
				// Primary category entries keep their own CMS source when selecting a page.
				if key != "sections" && key != "categories" {
					row["categoryId"] = target["categoryId"]
				}
			}
			if (key == "sections" || key == "carousel") && target["kind"] == "blank" {
				return problem(400, "空白页不能包含栏目或轮播内容，请先改为自定义页面")
			}
		}
	}
	style := M{}
	if raw, exists := input["navigationStyle"]; exists {
		source, ok := raw.(map[string]any)
		if !ok {
			return problem(400, "导航样式必须为对象")
		}
		if value, exists := source["fontSize"]; exists {
			size, ok := value.(float64)
			if !ok || math.IsNaN(size) || math.IsInf(size, 0) || size != math.Trunc(size) || size < 10 || size > 28 {
				return problem(400, "导航字号须为10–28的整数")
			}
			style["fontSize"] = size
		}
		for _, key := range []string{"color", "activeColor"} {
			if value, exists := source[key]; exists {
				color, ok := value.(string)
				if !ok || !navigationColor.MatchString(color) {
					return problem(400, "导航颜色须为六位十六进制颜色")
				}
				style[key] = color
			}
		}
	}
	if err := operationNavigation(input, out); err != nil {
		return err
	}
	out["navigationStyle"] = style
	return nil
}
