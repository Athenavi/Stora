import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    lines = f.readlines()

# Fix line 336 (0-indexed: 335)
line = lines[335]
# Replace '\' (backslash+quote) with '\\' (two backslashes+quote)
# In Go source: '\' is start+escape+end = error
# Need: '\\' is start+escaped-backslash+end = backslash rune
line = line.replace("cleanPath[0] == '\\'", "cleanPath[0] == '\\\\'")
lines[335] = line
print(f"Fixed line 336")

with open(p, 'w', encoding='utf-8') as f:
    f.writelines(lines)
print('DONE')
