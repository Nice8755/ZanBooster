#!/bin/sh
# ถอดทุกฟังก์ชันของไฟล์โค้ดเดิม (เช่น health.go) จาก exe เก่า แล้วย่อให้อ่านง่าย
# ใช้: sh tools/re/dumpfile.sh health.go > /tmp/health.txt
exe=beta/ZanBooster.exe
fns=$(go tool objdump $exe 2>/dev/null | awk -v f="zanbooster/$1" '/^TEXT main\./ && $3==f {n=$2; sub(/\(SB\)/,"",n); print n}')
re=$(echo "$fns" | sed 's/[().*]/\\&/g' | paste -sd'|')
go tool objdump -s "^($re)\$" $exe | python3 tools/re/annotate.py $exe | python3 tools/re/compact.py
