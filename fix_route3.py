import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

c = c.replace(
    'r.Post("/sync/upload/assign", blockHandler.SyncUploadAssign)',
    'r.Post("/sync/upload/assign", blockHandler.SyncUploadAssign)\n\t\t\tr.Post("/sync/upload/complete", blockHandler.SyncUploadComplete)'
)

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
print('DONE')
