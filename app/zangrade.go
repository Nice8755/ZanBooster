//go:build windows

package main

// zangrade.go: ถอดจาก exe 4.11.15 · ไฟล์เอฟเฟกต์ ZanGrade (ของพรีเมียม) + ตัวสร้างค่าพรีเซ็ตจากตาราง zgv
// ตัวไฟล์ .fx เก็บแบบบีบอัด+สลับไบต์ (assets/zg.bin) ไม่วางเป็นข้อความตรง ๆ ในที่เก็บสาธารณะ

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

//go:embed assets/zg.bin
var zgBin []byte

//go:embed assets/zs.bin
var zsBin []byte

var (
	zgOnce        sync.Once
	zgSrc, zsTmpl string
)

func zanAsset(b []byte) string {
	k := []byte("ZanBooster-fx")
	x := make([]byte, len(b))
	for i, c := range b {
		x[i] = c ^ k[i%len(k)]
	}
	r, err := gzip.NewReader(bytes.NewReader(x))
	if err != nil {
		return ""
	}
	out, _ := io.ReadAll(r)
	return string(out)
}

func zgLoad() {
	zgOnce.Do(func() { zgSrc, zsTmpl = zanAsset(zgBin), zanAsset(zsBin) })
}

type v3 [3]float64

// zgv: ค่าทุกช่องของ ZanGrade (ลำดับช่องต้องตรงตัวเก่า)
type zgv struct {
	Black, Exposure, Clarity, Contrast, Saturation, Vibrance, SkinProtect, SkinLift float64
	ShadowTint, HighTint                                                            v3
	SplitAmt, Temp, Tint, BloomThresh, Bloom, Fade, Vignette, Sharp, Grain          float64
	SkyDeep, SkyClouds, LightTint, LightGlow, RoadDark, RoadCool                    float64
	LightColor, FoliageTint                                                         v3
	Filmic, AO, HorizonDehaze, SunShafts, RoadGloss, SpecPop, Atmosphere            float64
	CurveR, CurveG, CurveB, SkyGradient, NeonBoost, LensFlare, Halation, ChromaAb   float64
	RainAuto, DNRole, RoadClean, Aurora, RoadLines, RoadLift, RoadFlat              float64
	FoliageMix, FoliageSat, FoliageBright, WaterSat, CloudAmount, RainDrops         float64
	LightTwoTone, Emissive, SunWarmth, FogAmount                                    float64
	LightWarm, LightWhite, AuroraC1, AuroraC2, FogColor                             v3
}

type zgKV struct{ K, V string }

// fxSec: หนึ่งเอฟเฟกต์ในไฟล์พรีเซ็ต (ชื่อ technique, ไฟล์ .fx, ค่า)
type fxSec struct {
	Tech, File string
	Vals       []zgKV
}

func f6(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }

func v3s(v v3) string { return f6(v[0]) + "," + f6(v[1]) + "," + f6(v[2]) }

func v3or(v, def v3) v3 {
	if v == (v3{}) {
		return def
	}
	return v
}

// zgFx: ค่าทั้ง 78 ช่องของ ZanGrade.fx ตามลำดับเดิม · mode 0 = ช่องพื้นฐาน, 1 = ช่องของ ZanQV, 2 = ช่องของ ZanNVE
func zgAll(z zgv) []zgKV {
	if z.SkinProtect == 0 {
		z.SkinProtect = 0.6
	}
	if z.BloomThresh == 0 {
		z.BloomThresh = 0.8
	}
	return []zgKV{
		{"Black", f6(z.Black)}, {"White", f6(1)}, {"Exposure", f6(z.Exposure)}, {"Clarity", f6(z.Clarity)},
		{"Contrast", f6(z.Contrast)}, {"Saturation", f6(z.Saturation)}, {"Vibrance", f6(z.Vibrance)},
		{"SkinProtect", f6(z.SkinProtect)}, {"SkinLift", f6(z.SkinLift)}, {"ShadowTint", v3s(z.ShadowTint)},
		{"HighTint", v3s(z.HighTint)}, {"SplitAmt", f6(z.SplitAmt)}, {"Temp", f6(z.Temp)}, {"Tint", f6(z.Tint)},
		{"BloomThresh", f6(z.BloomThresh)}, {"Bloom", f6(z.Bloom)}, {"Fade", f6(z.Fade)}, {"Vignette", f6(z.Vignette)},
		{"Sharp", f6(z.Sharp)}, {"Grain", f6(z.Grain)}, {"SkyDeep", f6(z.SkyDeep)}, {"SkyClouds", f6(z.SkyClouds)},
		{"LightColor", v3s(v3or(z.LightColor, v3{1, 0.75, 0.45}))}, {"LightTint", f6(z.LightTint)}, {"LightGlow", f6(z.LightGlow)},
		{"RoadDark", f6(z.RoadDark)}, {"RoadCool", f6(z.RoadCool)}, {"RoadClean", f6(z.RoadClean)}, {"RoadLines", f6(z.RoadLines)},
		{"RoadLift", f6(z.RoadLift)}, {"RoadFlat", f6(z.RoadFlat)}, {"HudProtect", f6(1)}, {"DNRole", strconv.Itoa(int(z.DNRole))},
		{"SpecPop", f6(z.SpecPop)}, {"Atmosphere", f6(z.Atmosphere)}, {"AtmoColor", "0.650000,0.750000,0.900000"},
		{"CurveR", f6(z.CurveR)}, {"CurveG", f6(z.CurveG)}, {"CurveB", f6(z.CurveB)}, {"SkyGradient", f6(z.SkyGradient)},
		{"SkyTopColor", "0.250000,0.450000,0.850000"}, {"SkyHorizonColor", "0.950000,0.680000,0.550000"},
		{"NeonBoost", f6(z.NeonBoost)}, {"LensFlare", f6(z.LensFlare)}, {"Halation", f6(z.Halation)}, {"ChromaAb", f6(z.ChromaAb)},
		{"RainAuto", f6(z.RainAuto)}, {"HorizonY", f6(0.42)}, {"NightAuto", f6(0.7)}, {"HazeAuto", f6(0.7)},
		{"ShaftColor", "1.000000,0.850000,0.650000"},
		{"AuroraColor1", v3s(v3or(z.AuroraC1, v3{0.2, 1, 0.6}))}, {"AuroraColor2", v3s(v3or(z.AuroraC2, v3{0.6, 0.3, 1}))},
		{"AuroraHeight", f6(0.22)}, {"AuroraSpeed", f6(0.5)}, {"Filmic", f6(z.Filmic)}, {"AO", f6(z.AO)},
		{"HorizonDehaze", f6(z.HorizonDehaze)}, {"SunShafts", f6(z.SunShafts)}, {"RoadGloss", f6(z.RoadGloss)}, {"Aurora", f6(z.Aurora)},
		{"FoliageTint", v3s(v3or(z.FoliageTint, v3{0.25, 0.45, 0.15}))}, {"FoliageMix", f6(z.FoliageMix)}, {"FoliageSat", f6(z.FoliageSat)},
		{"FoliageBright", f6(z.FoliageBright)}, {"WaterSat", f6(z.WaterSat)}, {"CloudAmount", f6(z.CloudAmount)}, {"CloudSpeed", f6(0.5)},
		{"LightTwoTone", f6(z.LightTwoTone)}, {"LightWarmColor", v3s(v3or(z.LightWarm, v3{1, 0.6, 0.3}))},
		{"LightWhiteColor", v3s(v3or(z.LightWhite, v3{0.55, 0.75, 1}))}, {"RainDrops", f6(z.RainDrops)}, {"RainDropsMode", "0"},
		{"RainSatRef", f6(math.Max(0.3, (1+0.5*z.Vibrance)*(1+z.Saturation)))},
		{"Emissive", f6(z.Emissive)}, {"SunWarmth", f6(z.SunWarmth)}, {"FogAmount", f6(z.FogAmount)},
		{"FogColor", v3s(v3or(z.FogColor, v3{0.78, 0.82, 0.88}))},
	}
}

func zgPick(z zgv, mode int) []zgKV {
	var out []zgKV
	for _, kv := range zgAll(z) {
		if zgOwner[kv.K] == mode {
			out = append(out, kv)
		}
	}
	return out
}

func zgFx(z zgv) fxSec { return fxSec{"ZanGrade", "ZanGrade.fx", zgPick(z, 0)} }

func zgStyle(name string, z zgv) fxSec {
	return fxSec{name, name + ".fx", zgPick(z, map[string]int{"ZanQV": 1, "ZanNVE": 2}[name])}
}

// zan3: พรีเซ็ตที่ใช้ ZanGrade + ZanQV + ZanNVE
func zan3(z zgv) string {
	return buildPreset([]fxSec{zgFx(z), zgStyle("ZanQV", z), zgStyle("ZanNVE", z)})
}

// buildPreset: ข้อความไฟล์พรีเซ็ต ReShade (CRLF แบบที่ ReShade เขียนเอง)
func buildPreset(secs []fxSec) string {
	var t []string
	for _, s := range secs {
		t = append(t, s.Tech+"@"+s.File)
	}
	var b strings.Builder
	b.WriteString("Techniques=" + strings.Join(t, ",") + "\r\nTechniqueSorting=" + strings.Join(t, ",") + "\r\n")
	for _, s := range secs {
		b.WriteString("\r\n[" + s.File + "]\r\n")
		for _, kv := range s.Vals {
			b.WriteString(kv.K + "=" + kv.V + "\r\n")
		}
	}
	return b.String()
}

var zanStyles = []struct{ Name, Label, Key string }{
	{"ZanNVE", "Zan NVE (custom)", "p11"},
	{"ZanQV", "Zan QV (custom)", "p12"},
}

// zanStyleFx: ZanGrade ฉบับเปลี่ยนชื่อ (ZanQV / ZanNVE / ZanGradeN กลางคืน) ให้ ReShade เห็นเป็นเอฟเฟกต์แยก
func zanStyleFx(name, label, style string) string {
	zgLoad()
	s := strings.ReplaceAll(zgSrc, "ZanGrade", name)
	switch style {
	case "ZanQV":
		s = strings.Replace(s, "#define ZAN_MODE 0", "#define ZAN_MODE 1", 1)
	case "ZanNVE":
		s = strings.Replace(s, "#define ZAN_MODE 0", "#define ZAN_MODE 2", 1)
	}
	s = strings.Replace(s, "// "+name+".fx - Zan Booster Premium (ห้ามแจกจ่าย)",
		"// "+name+".fx - "+label+" · ปรับแต่งเองได้ทุกหมวด · Zan Booster Premium (ห้ามแจกจ่าย)", 1)
	return strings.Replace(s, `ui_label = "Zan Grade (Zan Booster)"`, `ui_label = "`+label+` (Zan Booster)"`, 1)
}

func zanFxPath(plugins, file string) string {
	return filepath.Join(plugins, "reshade-shaders", "Shaders", "Zan", file)
}

func zanGradeWrite(plugins string) {
	zgLoad()
	p := zanFxPath(plugins, "ZanGrade.fx")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	zanWriteOne(p, []byte(zgSrc))
	for _, s := range zanStyles {
		zanWriteOne(zanFxPath(plugins, s.Name+".fx"), []byte(zanStyleFx(s.Name, s.Label, s.Name)))
	}
	zanWriteOne(zanFxPath(plugins, "ZanGradeN.fx"), []byte(zanStyleFx("ZanGradeN", "Zan Grade Night", "")))
	zanHudWrite(plugins)
}

// zanGradeRemove: ตอนเกมปิด ลบไฟล์ Zan ออก (กันคัดลอกไปแจก) · เขียนทับเป็นไฟล์ว่างก่อนลบ
func zanGradeRemove(plugins string) {
	zanHudRemove(plugins)
	names := []string{"ZanGradeN.fx", "ZanGrade.fx"}
	for _, s := range zanStyles {
		names = append(names, s.Name+".fx")
	}
	for _, n := range names {
		if p := zanFxPath(plugins, n); fileExists(p) {
			zanWriteOne(p, nil)
			os.Remove(p)
		}
	}
}
