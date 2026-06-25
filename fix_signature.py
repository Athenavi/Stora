import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

# Just fix the signature to remove folderId
c = c.replace(
    'SyncUploadFileAsync(string fullPath, string relativePath, string? folderId = null)',
    'SyncUploadFileAsync(string fullPath, string relativePath)'
)

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
print('DONE')
