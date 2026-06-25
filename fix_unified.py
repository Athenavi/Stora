import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

old = '''            if (existingCloudId != null && existingCloudId > 0)
            {
                using var s = File.OpenRead(fullPath); await _api.UpdateFileContentAsync(existingCloudId.Value, s, fileName);
                cloudId = existingCloudId.Value;
            }
            else if (len > 10 * 1024 * 1024)
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
                var result = await _api.SyncUploadCompleteAsync(uploadId, syncPath);
                cloudId = result.Id;
            }
            else
            {
                // Use SyncUploadAsync - single API call with path, server creates folders
                var syncPath = "Sync/" + relPath.Replace("\\", "/");
                using var fs = File.OpenRead(fullPath);
                var result = await _api.SyncUploadAsync(fs, fileName, syncPath);
                cloudId = result.Id;
            }'''

new = '''            if (existingCloudId != null && existingCloudId > 0)
            {
                using var s = File.OpenRead(fullPath); await _api.UpdateFileContentAsync(existingCloudId.Value, s, fileName);
                cloudId = existingCloudId.Value;
            }
            else
            {
                // Single unified upload: path + file, server creates folders (supports up to 500MB)
                var syncPath = "Sync/" + relPath.Replace("\\", "/");
                using var fs = File.OpenRead(fullPath);
                var result = await _api.SyncUploadAsync(fs, fileName, syncPath);
                cloudId = result.Id;
            }'''

if old in c:
    c = c.replace(old, new)
    print('REPLACED')
else:
    print('NOT MATCHED')
    s = c.find('if (existingCloudId != null && existingCloudId > 0)')
    if s > 0:
        e = c.find('cloudId = result.Id;\n            }', s)
        print(f'From {s} to {e}: {repr(c[s:e+40])}')

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
