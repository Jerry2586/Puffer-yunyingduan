package main

import "math"

// Operations may override these bounded layout values; artwork remains theme-owned.
var appearanceNumbers = map[string]map[string][2]float64{
	"section": {"widthPercent": {40, 100}, "paddingX": {0, 40}, "paddingY": {0, 60}, "cardWidth": {72, 320}, "headingSize": {12, 32}, "textSize": {10, 24}},
	"menu":    {"widthPercent": {40, 95}, "rowHeight": {36, 100}, "gap": {0, 32}, "padding": {8, 32}, "iconSize": {12, 48}, "fontSize": {10, 28}, "radius": {0, 32}},
	"middle":  {"iconSize": {16, 64}, "fontSize": {10, 28}, "gap": {0, 32}, "contentGap": {0, 24}, "padding": {0, 32}, "itemHeight": {32, 120}, "itemWidth": {48, 200}, "radius": {0, 40}},
}
var appearanceColors = map[string][]string{
	"section": {"headingColor", "textColor", "background"},
	"menu":    {"textColor", "background", "rowBackground"},
	"middle":  {"color", "activeColor", "background", "activeBackground"},
}

func layoutInteger(value any, min, max float64) bool {
	n, ok := value.(float64)
	return ok && !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n) && n >= min && n <= max
}
func operationAppearance(value any, kind string) (M, error) {
	input, ok := value.(map[string]any)
	if !ok {
		return nil, problem(400, "排版样式必须为对象")
	}
	out := M{}
	for key, value := range input {
		if bounds, exists := appearanceNumbers[kind][key]; exists {
			if !layoutInteger(value, bounds[0], bounds[1]) {
				return nil, problem(400, "排版尺寸 "+key+" 超出范围或不是整数")
			}
			out[key] = value
		} else if has(appearanceColors[kind], key) {
			color, ok := value.(string)
			if !ok || !navigationColor.MatchString(color) {
				return nil, problem(400, "排版颜色须为 #RRGGBB 格式")
			}
			out[key] = color
		} else if kind == "middle" && key == "arrangement" {
			direction, ok := value.(string)
			if !ok || !has([]string{"row", "column"}, direction) {
				return nil, problem(400, "图文排列方式无效")
			}
			out[key] = direction
		}
	}
	return out, nil
}
func sectionPresentation(row M) error {
	for key, bounds := range map[string][2]float64{"columns": {2, 6}, "gap": {0, 32}, "radius": {0, 40}} {
		if value, exists := row[key]; exists && !layoutInteger(value, bounds[0], bounds[1]) {
			return problem(400, "栏目 "+key+" 超出范围或不是整数")
		}
	}
	for key, choices := range map[string][]string{"style": {"grid3", "grid2", "scroll", "list", "ranking", "mosaic"}, "ratio": {"2/3", "3/4", "16/9", "1/1"}} {
		if value, exists := row[key]; exists {
			text, ok := value.(string)
			if !ok || !has(choices, text) {
				return problem(400, "栏目 "+key+" 无效")
			}
		}
	}
	for _, key := range []string{"showScore", "showNote", "showYear", "showTitle", "showMore"} {
		if value, exists := row[key]; exists {
			if _, ok := value.(bool); !ok {
				return problem(400, "栏目显示开关格式无效")
			}
		}
	}
	if value, exists := row["appearance"]; exists {
		appearance, err := operationAppearance(value, "section")
		if err != nil {
			return err
		}
		delete(row, "appearance")
		if len(appearance) > 0 {
			row["appearance"] = appearance
		}
	}
	return nil
}
