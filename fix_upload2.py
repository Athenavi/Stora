import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

old = '''    private async Task UploadFileAsync(string relPath, string hash, long? existingCloudId)
    {
        var fullPath = Path.Combine(_store.Config.LocalPath, relPath);
        if (!File.Exists(fullPath) || _index == null) return;

        try
        {
            StoreLocalBlocks(fullPath, relPath);
            var fileHash = ComputeHash(fullPath);
            var localBlocks = GetLocalBlockHashes(fileHash);
            if (localBlocks.Count == 0) return;

            var cloudBlocks = existingCloudId != null && existingCloudId > 0
                ? await GetCloudBlockHashes(existingCloudId.Value) : new HashSet<string>();

            var toUpload = localBlocks.Where(b => !cloudBlocks.Contains(b)).ToList();
            foreach (var blockHash in toUpload)
            {
                var sub = Path.Combine(_index.StoraPath, "Objects", blockHash.Substring(0, 2));
                var bp = Path.Combine(sub, blockHash.Substring(2));
                if (File.Exists(bp)) await _api.UploadBlockAsync(File.ReadAllBytes(bp));
            }

            var parentId = await EnsureParentFolderAsync(relPath);
            var parent = parentId > 0 ? parentId.ToString() : _store.Config.CloudFolderId;
            var len = new FileInfo(fullPath).Length;

            long cloudId;
            if (existingCloudId != null && existingCloudId > 0)
            {
                using var s = File.OpenRead(fullPath); await _api.UpdateFileContentAsync(existingCloudId.Value, s, Path.GetFileName(relPath));
                cloudId = existingCloudId.Value;
            }
            else if (len > 10 * 1024 * 1024)
            {
                var uid = await _api.InitChunkUploadAsync(Path.GetFileName(relPath), len, parent);
                const int cs = 4 * 1024 * 1024;
                var total = (int)Math.Ceiling((double)len / cs);
                using var stream = File.OpenRead(fullPath);
                var buf = new byte[cs];
                for (int i = 0; i < total; i++)
                {
                    if (!_isRunning) return;
                    var r = (int)Math.Min(cs, len - i * cs); if (r < cs) buf = new byte[r];
                    await stream.ReadAsync(buf, 0, r); await _api.UploadChunkAsync(uid, i, buf);
                }
                cloudId = await _api.CompleteChunkUploadAsync(uid);
            }
            else
            {
                using var s = File.OpenRead(fullPath); var u = await _api.UploadFileAsync(s, Path.GetFileName(relPath), parent); cloudId = u.Id;
            }

            _index.MarkSynced(relPath, cloudId, hash);
            _index.AppendJournal(relPath, "synced", hash, len);
        }
        catch (Exception ex)
        {
            _index.AppendJournal(relPath, "error", ex.Message);
        }
    }'''

new = '''    private async Task UploadFileAsync(string relPath, string hash, long? existingCloudId)
    {
        var fullPath = Path.Combine(_store.Config.LocalPath, relPath);
        if (!File.Exists(fullPath) || _index == null) return;

        try
        {
            StoreLocalBlocks(fullPath, relPath);

            // Resolve target folder by walking path segments
            var fileName = Path.GetFileName(relPath);
            var dirPart = Path.GetDirectoryName(relPath)?.Replace('\\', '/') ?? "";
            long parentId = long.TryParse(_store.Config.CloudFolderId, out var rootId) ? rootId : 0;

            if (!string.IsNullOrEmpty(dirPart))
            {
                foreach (var seg in dirPart.Split('/', StringSplitOptions.RemoveEmptyEntries))
                {
                    var foundId = await FindSubFolderAsync(parentId, seg);
                    if (foundId > 0) { parentId = foundId; }
                    else
                    {
                        try { var c = await _api.CreateFolderAsync(seg, parentId > 0 ? parentId.ToString() : null); parentId = c.Id; }
                        catch { }
                    }
                }
            }

            var targetFolder = parentId > 0 ? parentId.ToString() : _store.Config.CloudFolderId;
            var len = new FileInfo(fullPath).Length;

            long cloudId;
            if (existingCloudId != null && existingCloudId > 0)
            {
                using var s = File.OpenRead(fullPath); await _api.UpdateFileContentAsync(existingCloudId.Value, s, fileName);
                cloudId = existingCloudId.Value;
            }
            else if (len > 10 * 1024 * 1024)
            {
                var uid = await _api.InitChunkUploadAsync(fileName, len, targetFolder);
                const int cs = 4 * 1024 * 1024;
                var total = (int)Math.Ceiling((double)len / cs);
                using var stream = File.OpenRead(fullPath);
                var buf = new byte[cs];
                for (int i = 0; i < total; i++)
                {
                    if (!_isRunning) return;
                    var r = (int)Math.Min(cs, len - i * cs); if (r < cs) buf = new byte[r];
                    await stream.ReadAsync(buf, 0, r); await _api.UploadChunkAsync(uid, i, buf);
                }
                cloudId = await _api.CompleteChunkUploadAsync(uid);
            }
            else
            {
                using var s = File.OpenRead(fullPath); var u = await _api.UploadFileAsync(s, fileName, targetFolder); cloudId = u.Id;
            }

            _index.MarkSynced(relPath, cloudId, hash);
            _index.AppendJournal(relPath, "synced", hash, len);
        }
        catch (Exception ex)
        {
            _index.AppendJournal(relPath, "error", ex.Message);
        }
    }'''

if old in c:
    c = c.replace(old, new)
    print('MATCHED - replaced')
else:
    print('NOT MATCHED')
    # Show first difference
    for i, (a, b) in enumerate(zip(old.split('\n'), c[c.find('    private async Task UploadFileAsync'):].split('\n'))):
        if a != b:
            print(f'Line {i} differs:')
            print(f'  old: [{repr(a[:80])}]')
            print(f'  new: [{repr(b[:80])}]')
            break

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
