import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

# Replace the large file path chunked+assign with chunked+complete
old = '''                await _api.CompleteChunkUploadAsync(uploadId);
                var result = await _api.SyncUploadAssignAsync(uploadId, syncPath);
                cloudId = result.Id;'''

new = '''                var result = await _api.SyncUploadCompleteAsync(uploadId, syncPath);
                cloudId = result.Id;'''

c = c.replace(old, new)

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
print('DONE')
