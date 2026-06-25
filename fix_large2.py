import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

old = '''            else if (len > 10 * 1024 * 1024)
            {
                // Large file: use SyncUploadAsync (backend supports up to 100MB)
                var syncPath = "Sync/" + relPath.Replace("\\", "/");
                using var fs = File.OpenRead(fullPath);
                var result = await _api.SyncUploadAsync(fs, fileName, syncPath);
                cloudId = result.Id;
            }'''

new = '''            else if (len > 10 * 1024 * 1024)
            {
                // Large file: chunked upload then assign to path
                var syncPath = "Sync/" + relPath.Replace("\\", "/");
                var uploadId = await _api.InitChunkUploadAsync(fileName, len, null);
                const int cs = 4 * 1024 * 1024;
                var total = (int)Math.Ceiling((double)len / cs);
                using var stream = File.OpenRead(fullPath);
                var buf = new byte[cs];
                for (int i = 0; i < total; i++)
                {
                    if (!_isRunning) return;
                    var r = (int)Math.Min(cs, len - i * cs); if (r < cs) buf = new byte[r];
                    await stream.ReadAsync(buf, 0, r); await _api.UploadChunkAsync(uploadId, i, buf);
                }
                await _api.CompleteChunkUploadAsync(uploadId);
                var result = await _api.SyncUploadAssignAsync(uploadId, syncPath);
                cloudId = result.Id;
            }'''

if old in c:
    c = c.replace(old, new)
    print('REPLACED')
else:
    print('NOT MATCHED')

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
