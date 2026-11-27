//go:build windows

package main

// reshade_data.go: สร้างด้วย tools/re/rsdata.py จาก exe 4.11.15 (ห้ามแก้มือ)

var zgOwner = map[string]int{
	"Aurora": 1,
	"AuroraColor1": 1,
	"AuroraColor2": 1,
	"AuroraHeight": 1,
	"AuroraSpeed": 1,
	"CloudAmount": 1,
	"CloudSpeed": 1,
	"LightTwoTone": 1,
	"LightWarmColor": 1,
	"LightWhiteColor": 1,
	"RoadDark": 1,
	"RoadCool": 1,
	"RoadGloss": 1,
	"RoadClean": 1,
	"RoadLines": 1,
	"RoadLift": 1,
	"RoadFlat": 1,
	"FoliageTint": 1,
	"FoliageMix": 1,
	"FoliageSat": 1,
	"FoliageBright": 1,
	"RainDrops": 1,
	"RainDropsMode": 1,
	"RainSatRef": 1,
	"Emissive": 2,
	"SunWarmth": 2,
	"FogAmount": 2,
	"FogColor": 2,
}

var zanVals = map[string]zgv{
	"p1": {Black: 0.03, Clarity: 0.45, Contrast: 0.25, Vibrance: 0.25, SkinLift: 0.04, ShadowTint: v3{-0.1, 0.0, 0.12}, HighTint: v3{0.12, 0.06, -0.08}, SplitAmt: 0.25, Temp: 0.1, Bloom: 0.15, Vignette: 0.25, Sharp: 0.4, RoadDark: 0.25, RoadCool: 0.25, RoadGloss: 0.15, RoadClean: 0.6, RoadLines: 0.6, RoadLift: 0.3, RoadFlat: 0.5, FoliageSat: 0.05, CloudAmount: 0.1, RainDrops: 0.35, LightTwoTone: 0.35, Emissive: 0.3, SunWarmth: 0.2, FogAmount: 0.05},
	"p2": {Black: 0.04, Exposure: -0.05, Clarity: 0.55, Contrast: 0.4, Saturation: -0.12, Vibrance: 0.2, ShadowTint: v3{-0.3, 0.05, 0.3}, HighTint: v3{0.3, 0.12, -0.25}, SplitAmt: 0.45, Temp: 0.05, Bloom: 0.2, Vignette: 0.45, Sharp: 0.45, Grain: 0.1, RoadDark: 0.45, RoadCool: 0.4, RoadGloss: 0.3, RoadClean: 0.6, RoadLines: 0.55, RoadFlat: 0.5, FoliageSat: -0.1, CloudAmount: 0.15, RainDrops: 0.45, LightTwoTone: 0.55, Emissive: 0.4, SunWarmth: 0.15, FogAmount: 0.12, LightWarm: v3{1.0, 0.55, 0.25}, LightWhite: v3{0.45, 0.75, 1.0}},
	"p3": {Black: 0.03, Clarity: 0.35, Contrast: 0.25, Vibrance: 0.25, ShadowTint: v3{0.05, 0.0, -0.05}, HighTint: v3{0.35, 0.2, -0.3}, SplitAmt: 0.3, Temp: 0.7, BloomThresh: 0.65, Bloom: 0.45, Vignette: 0.3, Sharp: 0.35, RoadDark: 0.15, RoadCool: 0.1, FoliageTint: v3{0.35, 0.42, 0.12}, RoadClean: 0.55, RoadLines: 0.5, RoadLift: 0.15, RoadFlat: 0.45, FoliageMix: 0.25, CloudAmount: 0.3, RainDrops: 0.3, LightTwoTone: 0.3, Emissive: 0.35, SunWarmth: 0.55, FogAmount: 0.08, LightWarm: v3{1.0, 0.7, 0.35}, LightWhite: v3{1.0, 0.85, 0.65}, FogColor: v3{0.95, 0.82, 0.68}},
	"p4": {Black: 0.04, Exposure: -0.15, Clarity: 0.4, Contrast: 0.35, Vibrance: 0.35, ShadowTint: v3{0.15, -0.1, 0.4}, HighTint: v3{0.25, -0.1, 0.25}, SplitAmt: 0.45, Temp: -0.5, Tint: 0.3, BloomThresh: 0.7, Bloom: 0.5, Vignette: 0.45, Sharp: 0.4, RoadDark: 0.4, RoadCool: 0.5, RoadGloss: 0.5, NeonBoost: 0.8, RoadClean: 0.5, Aurora: 0.35, RoadLines: 0.45, RoadFlat: 0.4, CloudAmount: 0.2, RainDrops: 0.5, LightTwoTone: 0.7, Emissive: 0.65, FogAmount: 0.1, LightWarm: v3{1.0, 0.35, 0.75}, LightWhite: v3{0.35, 0.8, 1.0}, AuroraC1: v3{0.3, 0.6, 1.0}, AuroraC2: v3{0.9, 0.3, 1.0}, FogColor: v3{0.55, 0.5, 0.8}},
	"p5": {Black: 0.02, Clarity: 0.3, Contrast: 0.22, Vibrance: 0.45, ShadowTint: v3{-0.25, 0.15, 0.25}, HighTint: v3{0.2, -0.1, 0.12}, SplitAmt: 0.35, Temp: -0.1, Tint: 0.3, Bloom: 0.4, Vignette: 0.2, Sharp: 0.35, RoadCool: 0.2, RoadGloss: 0.15, RoadClean: 0.55, RoadLines: 0.55, RoadLift: 0.3, RoadFlat: 0.45, FoliageSat: 0.2, FoliageBright: 0.05, CloudAmount: -0.2, RainDrops: 0.3, LightTwoTone: 0.6, Emissive: 0.5, SunWarmth: 0.15, LightWarm: v3{1.0, 0.4, 0.7}, LightWhite: v3{0.3, 1.0, 0.85}},
	"p6": {Black: 0.03, Exposure: 0.05, Clarity: 0.4, Contrast: 0.25, Saturation: 0.1, Vibrance: 0.4, ShadowTint: v3{-0.1, 0.0, 0.15}, HighTint: v3{0.15, 0.08, -0.1}, SplitAmt: 0.25, Temp: 0.3, Bloom: 0.25, Vignette: 0.2, Sharp: 0.4, RoadCool: 0.15, RoadGloss: 0.1, RoadClean: 0.6, RoadLines: 0.6, RoadLift: 0.25, RoadFlat: 0.5, FoliageSat: 0.2, FoliageBright: 0.05, CloudAmount: 0.25, RainDrops: 0.3, LightTwoTone: 0.25, Emissive: 0.3, SunWarmth: 0.35},
	"p7": {Black: 0.03, Clarity: 0.4, Contrast: 0.25, Vibrance: 0.45, SkinProtect: 0.9, ShadowTint: v3{-0.2, 0.05, 0.3}, HighTint: v3{0.0, 0.05, 0.1}, SplitAmt: 0.3, Temp: -0.4, Bloom: 0.2, Vignette: 0.25, Sharp: 0.4, RoadCool: 0.35, FoliageTint: v3{0.18, 0.45, 0.3}, RoadGloss: 0.15, RoadClean: 0.55, RoadLines: 0.55, RoadLift: 0.2, RoadFlat: 0.45, FoliageMix: 0.2, FoliageSat: 0.1, CloudAmount: -0.1, RainDrops: 0.35, LightTwoTone: 0.4, Emissive: 0.35, FogAmount: 0.06, LightWarm: v3{1.0, 0.75, 0.5}, LightWhite: v3{0.5, 0.85, 1.0}, FogColor: v3{0.75, 0.85, 0.95}},
	"p8": {Black: 0.01, Exposure: 0.05, Clarity: 0.15, Contrast: 0.05, Saturation: -0.05, Vibrance: 0.3, SkinLift: 0.1, HighTint: v3{0.15, -0.05, 0.1}, SplitAmt: 0.2, Tint: 0.2, BloomThresh: 0.72, Bloom: 0.35, Fade: 0.02, Vignette: 0.1, Sharp: 0.3, RoadCool: 0.15, RoadClean: 0.6, RoadLines: 0.5, RoadLift: 0.2, RoadFlat: 0.55, FoliageSat: 0.05, CloudAmount: -0.1, RainDrops: 0.25, LightTwoTone: 0.3, Emissive: 0.3, SunWarmth: 0.2, FogAmount: 0.05, LightWarm: v3{1.0, 0.7, 0.6}, LightWhite: v3{1.0, 0.85, 0.9}, FogColor: v3{0.95, 0.88, 0.9}},
	"p9": {Black: 0.02, Clarity: 0.25, Contrast: 0.12, Vibrance: 0.25, SkinLift: 0.06, ShadowTint: v3{0.2, -0.1, 0.2}, HighTint: v3{0.25, -0.08, 0.12}, SplitAmt: 0.45, Temp: 0.1, Tint: 0.35, Bloom: 0.5, Vignette: 0.15, Sharp: 0.3, Grain: 0.08, RoadCool: 0.1, FoliageTint: v3{0.5, 0.4, 0.3}, RoadClean: 0.55, Aurora: 0.15, RoadLines: 0.5, RoadLift: 0.15, RoadFlat: 0.5, FoliageMix: 0.3, FoliageSat: -0.05, CloudAmount: 0.1, RainDrops: 0.3, LightTwoTone: 0.45, Emissive: 0.35, SunWarmth: 0.15, FogAmount: 0.06, LightWarm: v3{1.0, 0.6, 0.75}, LightWhite: v3{1.0, 0.85, 0.95}, AuroraC1: v3{1.0, 0.6, 0.8}, AuroraC2: v3{0.7, 0.5, 1.0}, FogColor: v3{0.95, 0.85, 0.9}},
	"p10": {Clarity: 0.3, Contrast: 0.2, Saturation: -0.2, Vibrance: 0.1, ShadowTint: v3{0.15, -0.05, 0.25}, HighTint: v3{0.25, 0.15, -0.15}, SplitAmt: 0.4, Temp: 0.35, Tint: 0.2, Bloom: 0.35, Fade: 0.06, Vignette: 0.5, Sharp: 0.3, Grain: 0.25, RoadDark: 0.2, RoadCool: 0.1, FoliageTint: v3{0.35, 0.4, 0.18}, RoadClean: 0.45, RoadLines: 0.45, RoadLift: 0.2, RoadFlat: 0.35, FoliageMix: 0.2, FoliageSat: -0.2, CloudAmount: 0.2, RainDrops: 0.3, LightTwoTone: 0.4, Emissive: 0.3, SunWarmth: 0.25, FogAmount: 0.14, LightWarm: v3{1.0, 0.55, 0.2}, LightWhite: v3{0.9, 0.85, 0.7}, FogColor: v3{0.85, 0.8, 0.72}},
	"p11": {Black: 0.035, Exposure: -0.04, Clarity: 0.5, Contrast: 0.3, Saturation: 0.12, Vibrance: 0.15, SkinProtect: 0.7, BloomThresh: 0.8, Bloom: 0.3, Vignette: 0.1, Sharp: 0.6, SkyDeep: 0.2, SkyClouds: 0.3, LightGlow: 0.15, RoadDark: 0.3, RoadCool: 0.3, Filmic: 0.2, HorizonDehaze: 0.3, RoadGloss: 0.2, SpecPop: 0.3, NeonBoost: 0.25, RoadClean: 0.6, RoadLines: 0.6, RoadLift: 0.15, RoadFlat: 0.5, FoliageSat: 0.1, CloudAmount: 0.15, RainDrops: 0.4, LightTwoTone: 0.4, Emissive: 0.4, SunWarmth: 0.2, FogAmount: 0.06},
	"p12": {Black: 0.02, Clarity: 0.2, Contrast: 0.12, Saturation: -0.15, Vibrance: 0.12, SkinProtect: 0.8, ShadowTint: v3{-0.04, 0.02, 0.08}, HighTint: v3{0.05, 0.06, 0.08}, SplitAmt: 0.25, Temp: -0.06, BloomThresh: 0.68, Bloom: 0.3, Fade: 0.03, Vignette: 0.25, Sharp: 0.25, SkyClouds: 0.2, LightGlow: 0.5, RoadCool: 0.3, FoliageTint: v3{0.25, 0.38, 0.3}, SunShafts: 0.45, RoadGloss: 0.35, Atmosphere: 0.3, Halation: 0.2, RoadClean: 0.55, RoadLines: 0.5, RoadLift: 0.15, RoadFlat: 0.45, FoliageMix: 0.25, FoliageSat: -0.2, CloudAmount: 0.6, RainDrops: 0.3, LightTwoTone: 0.4, Emissive: 0.45, SunWarmth: 0.1, FogAmount: 0.35, LightWarm: v3{1.0, 0.75, 0.5}, LightWhite: v3{0.8, 0.9, 1.0}, FogColor: v3{0.82, 0.85, 0.9}},
}
