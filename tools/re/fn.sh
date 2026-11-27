#!/bin/sh
# ดู asm ของฟังก์ชันเดียวแบบเต็ม (มีข้อความกำกับ): fn.sh 'main\.reshadeCalcID'
go tool objdump -s "^$1\$" beta/ZanBooster.exe | python3 tools/re/annotate.py beta/ZanBooster.exe 2>/dev/null | awk -F'\t+' '{print $1,$4,$5}' | sed 's/^ *//' | grep -v "PCDATA\|FUNCDATA\|NOPL\|NOPW\|INT \$0x3\|morestack"
