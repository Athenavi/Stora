import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

# Simplify UploadFileAsync to use SyncUploadAsync
old = '''    private async Task UploadFileAsync(string relPath, string hash, long? existingCloudId)
    {
        var fullPath = Path.Combine(_store.Config.LocalPath, relPath);
        if (!File.Exists(fullPath) || _index == null) return;

        // Ensure Sync folder ID is available
        if (string.IsNullOrEmpty(_store.Config.CloudFolderId))
        {
            _index.AppendJournal(relPath, "debug_no_cloud_folder", $"CloudFolderId is null for {relPath}");
            await EnsureSyncRootAsync();
            if (string.IsNullOrEmpty(_store.Config.CloudFolderId))
            {
                _index.AppendJournal(relPath, "error_no_cloud_folder", "Still null after EnsureSyncRootAsync");
                return;
            }
        }

        try
        {
            StoreLocalBlocks(fullPath, relPath);
            var len = new FileInfo(fullPath).Length;

            // Resolve parent folder by walking path segments
            var fileName = Path.GetFileName(relPath);
            var dirPart = Path.GetDirectoryName(relPath)?.Replace('\\', '/') ?? "";
            long pid = long.TryParse(_store.Config.CloudFolderId, out var rootId) ? rootId : 0;

            if (!string.IsNullOrEmpty(dirPart))
            {
                foreach (var seg in dirPart.Split('/', StringSplitOptions.RemoveEmptyEntries))
                {
                    // Find existing folder
                    try
                    {
                        var list = await _api.ListFolderContentsAsync(pid > 0 ? pid.ToString() : null);
                        var match = list.FirstOrDefault(f => f.IsFolder && f.Name == seg);
                        if (match != null) { pid = match.Id; continue; }
                    }
                    catch { }

                    // Create folder
                    try { var c2 = await _api.CreateFolderAsync(seg, pid > 0 ? pid.ToString() : null); pid = c2.Id; }
                    catch
                    {
                        try { var list2 = await _api.ListFolderContentsAsync(pid > 0 ? pid.ToString() : null); var m2 = list2.FirstOrDefault(f => f.IsFolder && f.Name == seg); if (m2 != null) pid = m2.Id; }
                        catch { }
                    }
                }
            }

            var targetFolder = pid > 0 ? pid.ToString() : _store.Config.CloudFolderId;
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

new = '''    private async Task UploadFileAsync(string relPath, string hash, long? existingCloudId)
    {
        var fullPath = Path.Combine(_store.Config.LocalPath, relPath);
        if (!File.Exists(fullPath) || _index == null) return;

        try
        {
            StoreLocalBlocks(fullPath, relPath);
            var len = new FileInfo(fullPath).Length;

            // Use single SyncUploadAsync call (auto-creates folder hierarchy)
            var relativeForServer = relPath.Replace('\\', '/');
            long cloudId;

            if (existingCloudId != null && existingCloudId > 0)
            {
                // Update existing file content
                using var s = File.OpenRead(fullPath);
                await _api.UpdateFileContentAsync(existingCloudId.Value, s, Path.GetFileName(relPath));
                cloudId = existingCloudId.Value;
            }
            else if (len > 10 * 1024 * 1024)
            {
                // Large file: chunked upload
                var fileName = Path.GetFileName(relPath);
                var uid = await _api.InitChunkUploadAsync(fileName, len, _store.Config.CloudFolderId);
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
                // Single API call: auto-creates folders + uploads file
                using var fs = File.OpenRead(fullPath);
                var result = await _api.SyncUploadAsync(fs, Path.GetFileName(relPath), relativeForServer);
                cloudId = result.Id;
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
    print('REPLACED')
else:
    print('NOT MATCHED - checking first lines')
    s = c.find('private async Task UploadFileAsync')
    if s > 0:
        for i, line in enumerate(c[s:].split('\n')[:10]):
            print(f'{i}: [{repr(line.strip()[:100])}]')

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
