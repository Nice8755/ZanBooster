# Builds the corrected ZanGrade shader from the text inside the original exe (the full shader is not stored in this repo).
param([Parameter(Mandatory)][string]$Exe, [Parameter(Mandatory)][string]$Out)
$ErrorActionPreference='Stop'; $enc=[Text.UTF8Encoding]::new($false)
$lat=[Text.Encoding]::GetEncoding(28591); $raw=[IO.File]::ReadAllBytes($Exe); $s=$lat.GetString($raw); $endm='pass Grade { VertexShader = ZanGradeVS; PixelShader = ZanGradePS; }'
$i0=$s.IndexOf('// ZanGrade.fx - Zan Booster'); $i1=$s.IndexOf($endm,$i0); if($i0 -lt 0 -or $i1 -lt 0){throw 'shader not found'}; $i1=$s.IndexOf('}',$i1+$endm.Length)+1
$t=$enc.GetString($raw,$i0,$i1-$i0).Replace("`r`n","`n")+"`n"
function Rep($a,$b){ $a=$a.Replace("`r`n","`n");$b=$b.Replace("`r`n","`n"); $n=([regex]::Matches($script:t,[regex]::Escape($a))).Count; if($n -ne 1){throw "match $n : $a"}; $script:t=$script:t.Replace($a,$b) }
# พรีเซ็ตรูปแบบเดิม (11/12) เก็บค่าพื้นฐานไว้ในชุด NVE/QV
foreach($n in 'Black','White','Exposure','Contrast','Clarity','Sharp','Saturation','Vibrance','Temp','Tint','SkinProtect','ShadowTint','HighTint','SplitAmt','SkyDeep','SkyClouds','LightColor','LightTint','LightGlow','Bloom','BloomThresh','WaterSat','Vignette','RoadDark','RoadCool','FoliageTint','FoliageMix','FoliageSat','FoliageBright'){
  $rx=[regex]"(?m)^static const (float3?) $n = (.+);$"; if($rx.Matches($t).Count -ne 1){throw "const $n"}
  $t=$rx.Replace($t,('uniform $1 '+$n+' < hidden = true; > = $2;'))
}
foreach($n in 'NightAuto','HazeAuto'){ Rep "static const float $n = 0.0;" "#define $n (0.7 * step(0.0001, Black))" }
Rep "float lGlow = LightGlow + 0.35 * night, lTint = saturate(LightTint + 0.25 * night), clar = Clarity * (1.0 - 0.4 * night);" @'
float zold = 1.0;
#if ZAN_MODE != 0
	zold = step(0.0001, Black);
#endif
	float calm = ZSStep(0.2, 0.36, stat.w); // ภาพสีจัดอยู่แล้ว (เย็น/ไฟสี) ลดการเติมสี
	float lGlow = LightGlow * zold + 0.35 * night, lTint = saturate(LightTint * zold + 0.25 * night), clar = Clarity * (1.0 - 0.4 * night) * (1.0 - 0.5 * haze);
'@.TrimEnd()
Rep "#if ZAN_MODE == 0`n`tc = ZShoulder(saturate((c - blk) / (White - blk)) * exp2(Exposure) * (1.0 + 0.12 * night));`n#endif" "`tif (zold > 0.5) c = ZShoulder(saturate((c - blk) / (White - blk)) * exp2(Exposure) * (1.0 + 0.12 * night) * (1.0 - 0.02 * ZSStep(0.5, 0.72, avgL)));"
Rep "c = pow(saturate(c), 1.0 / (1.0 + 0.28 * night));`n`t`tc = saturate((c - 0.02 * night) / (1.0 - 0.02 * night));" "c = pow(saturate(c), 1.0 / (1.0 + (0.28 + 0.3 * (1.0 - ZSStep(0.06, 0.15, avgL))) * night));`n`t`tc = saturate((c - 0.004 * night) / (1.0 - 0.004 * night));"
# p11/p12 สว่างเท่าพรีเซ็ตอื่น + ขอบฟ้านุ่ม ไม่เป็นแถบตั้ง
Rep "if (zold > 0.5) c = ZShoulder(" "if (zold > 0.5) c = ZShoulder(ZAN_LIFT * "
Rep "#define ZAN_MODE 0 //" "#define ZAN_LIFT (ZAN_MODE == 0 ? 1.0 : 1.1)`n#define ZAN_MODE 0 //"
Rep "(1.0 - ZSStep(skyLn - 0.05, skyLn + 0.005, uv.y)) * skyOK" "(1.0 - ZSStep(min(skyLn, skyMean + 0.04) - 0.14, skyLn + 0.005, uv.y)) * skyOK"
# clarity ไม่เกิดขอบขาว
Rep "c = saturate(c + (L - Lb) * clar * saturate(4.0 * L * (1.0 - L)));" "c = saturate(c + clamp(L - Lb, -0.12, 0.05) * clar * saturate(4.0 * L * (1.0 - L)));"
Rep "c = saturate(c + (c * c * (3.0 - 2.0 * c) - c) * Contrast);" "c = saturate(c + (c * c * (3.0 - 2.0 * c) - c) * Contrast * (1.0 - 0.45 * nightD) * (1.0 - 0.3 * wetD));"
# เส้นถนนไม่ไปขึ้นบนน้ำ/พื้นเทาตอนฟ้าครึ้ม
Rep "lerp(c, saturate(ll * 1.35 + 0.08).xxx, saturate(lm * RoadLines * 0.8))" "lerp(c, saturate(ll * 1.35 + 0.08).xxx, saturate(lm * RoadLines * 0.8 * lerp(0.25, 1.0, ZSStep(0.06, 0.12, stat.w + nightD))))"
# ความสด: พอดีทุกสภาพแสง
Rep "float f = 1.0 + Saturation + Vibrance * (1.0 - (mx - mn));" "float f = 1.0 + Saturation + (Vibrance * (1.0 - 0.6 * calm) + 0.12 * wetD * step(0.001, Vibrance)) * (1.0 - (mx - mn));"
Rep "float3 c2 = c + (ShadowTint * (1.0 - L) * (1.0 - L) + HighTint * L * L) * SplitAmt;" @'
float tk = (1.0 - 0.5 * calm) * (1.0 - 0.35 * wetD);
	float3 c2 = c + (ShadowTint * (1.0 - L) * (1.0 - L) * lerp(1.0, 0.55, nightD) * ZSStep(0.0, 0.3, L) + HighTint * L * L * (1.0 - ZSStep(0.75, 1.0, L))) * SplitAmt * tk;
'@.TrimEnd()
Rep "c *= float3(1.0 + Temp * 0.1 + Tint * 0.03, 1.0 + Temp * 0.01 - Tint * 0.06, 1.0 - Temp * 0.1 + Tint * 0.03);" "c *= 1.0 + float3(Temp * 0.1 + Tint * 0.03, Temp * 0.01 - Tint * 0.06, Tint * 0.03 - Temp * 0.1) * tk;"
Rep "(ZFrame % 64)" "(uint(ZFrame) % 64u)"
Rep "c = lerp(c, rawIn, tex2D(ZanHudS, uv).r * HudProtect);`n`treturn saturate(c);" "c += (frac(52.9829189 * frac(dot(vpos.xy, float2(0.06711056, 0.00583715)))) - 0.5) / 255.0; // กันฟ้าเป็นริ้ว`n`tc = lerp(c, rawIn, tex2D(ZanHudS, uv).r * HudProtect);`n`treturn saturate(c);"
# ตัดคอมเมนต์ทั้งบรรทัด ให้พอดีช่อง
$t=($t.Split("`n")|Where-Object{$_ -notmatch '^\s*//' -or $_ -match '^// ZanGrade\.fx'}) -join "`n"
[IO.File]::WriteAllText($Out,$t.TrimEnd()+"`n",$enc); "shader: $($enc.GetByteCount($t)) bytes"
