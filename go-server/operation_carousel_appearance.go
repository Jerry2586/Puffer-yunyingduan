package main

// Page overrides adjust carousel geometry without changing shared content or theme defaults.
func operationCarouselAppearance(value any) (M, error) {
	input, ok := value.(map[string]any)
	if !ok {
		return nil, problem(400, "轮播样式必须为对象")
	}
	out := M{}
	for key, bounds := range map[string][2]float64{
		"widthPercent": {40, 100}, "height": {120, 600}, "margin": {0, 48}, "radius": {0, 300},
	} {
		if value, exists := input[key]; exists {
			if !layoutInteger(value, bounds[0], bounds[1]) {
				return nil, problem(400, "轮播尺寸 "+key+" 超出范围或不是整数")
			}
			out[key] = value
		}
	}
	for key, choices := range map[string][]string{
		"style": {"full", "inset", "peek", "poster", "split"}, "fit": {"cover", "contain"},
	} {
		if value, exists := input[key]; exists {
			text, ok := value.(string)
			if !ok || !has(choices, text) {
				return nil, problem(400, "轮播样式 "+key+" 无效")
			}
			out[key] = text
		}
	}
	return out, nil
}
