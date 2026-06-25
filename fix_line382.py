import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    lines = f.readlines()

# Fix line 382 (0-indexed: 381)
line = lines[381]
# Current: relPath.Replace("\", "/") - contains escaped quote
# Target:  relPath.Replace("\\", "/") - contains escaped backslash
line = line.replace('Replace("\\", "/")', 'Replace("\\\\", "/")')
lines[381] = line
print(f'Line 382 fixed: {line.strip()[:60]}')

with open(p, 'w', encoding='utf-8') as f:
    f.writelines(lines)
print('DONE')
