import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

c = c.replace(
    'var relativeForServer = relPath.Replace(\'\\\', \'/\');',
    'var relativeForServer = "Sync/" + relPath.Replace(\'\\\', \'/\');'
)

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
print('DONE')
